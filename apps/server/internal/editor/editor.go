package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Store is the part of the persistence layer the editor uses.
type Store interface {
	SearchCharacters(ctx context.Context, query string, limit int) ([]store.CharacterRef, error)
	AccountByLogin(ctx context.Context, login string) (*store.Account, error)
	AccountOnline(ctx context.Context, accountID int64) (bool, error)
	LoadCharacter(ctx context.Context, accountID int64, name string) (*store.Character, error)
	SaveCharacter(ctx context.Context, c *store.Character) error
	SaveInventory(ctx context.Context, characterID int64, inventory []byte) error
	LoadWarehouse(ctx context.Context, accountID int64, number int) (*store.Warehouse, error)
	SaveWarehouse(ctx context.Context, w *store.Warehouse) error
	NextItemSerial(ctx context.Context) (uint32, error)
}

// Errors returned to the HTTP layer.
var (
	ErrOnline   = errors.New("a conta está online: deslogue o personagem antes de editar")
	ErrNotFound = errors.New("personagem não encontrado")
	ErrInvalid  = errors.New("dados inválidos")
	ErrNoSpace  = errors.New("não há espaço livre para o item")
)

// Inventory layout (game/inventory.go): 0..11 equipment, 12..75 the 8x8 bag.
// The extended rows 76..107 are not reachable in this build and are left
// untouched.
const (
	equipSlots     = 12
	bagStart       = 12
	bagRows        = 8
	gridCols       = 8
	warehouseRows  = 15 // 120 slots, CWarehouse
	warehouseSlots = 120
)

// Container selects the inventory or the account vault (warehouse 0).
type Container string

const (
	Inventory Container = "inventory"
	Vault     Container = "vault"
)

// Limits are the GameServer rules the editor applies, the same as the GM
// /edit command (game/command_gm.go commandEdit).
type Limits struct {
	MaxLevel int                  // Common.dat ServerMaxLevel
	MaxStat  [4]int               // Common.dat MaxStatPoint_AL0..AL3
	Exp      *gamedata.Experience // level table: the experience follows the level
}

// Editor applies edits to offline characters.
type Editor struct {
	st  Store
	cat *Catalog
	lim Limits
}

// New returns an editor over a store and the item catalog.
func New(st Store, cat *Catalog, lim Limits) *Editor {
	return &Editor{st: st, cat: cat, lim: lim}
}

// maxStat is MaxStatPoint_AL<accountLevel>.
func (e *Editor) maxStat(accountLevel int) int {
	if accountLevel < 0 || accountLevel >= len(e.lim.MaxStat) {
		accountLevel = 0
	}
	return e.lim.MaxStat[accountLevel]
}

// accountLevel returns the AccountLevel of a character's account.
func (e *Editor) accountLevel(ctx context.Context, login string) (int, error) {
	a, err := e.st.AccountByLogin(ctx, login)
	if err != nil {
		return 0, err
	}
	return a.Level, nil
}

// Limits returns the GameServer rules in use.
func (e *Editor) Limits() Limits { return e.lim }

// Catalog returns the item catalog.
func (e *Editor) Catalog() *Catalog { return e.cat }

// Search lists characters by name or account login.
func (e *Editor) Search(ctx context.Context, q string) ([]store.CharacterRef, error) {
	return e.st.SearchCharacters(ctx, strings.TrimSpace(q), 100)
}

// find resolves a character name (case-insensitive) to its account.
func (e *Editor) find(ctx context.Context, name string) (store.CharacterRef, error) {
	refs, err := e.st.SearchCharacters(ctx, name, 100)
	if err != nil {
		return store.CharacterRef{}, err
	}
	for _, r := range refs {
		if strings.EqualFold(r.Name, name) {
			return r, nil
		}
	}
	return store.CharacterRef{}, ErrNotFound
}

// load returns the character and its search entry, and fails with
// ErrOnline when write is set and the account is connected: the GameServer
// keeps online characters in memory and saves them back on logout, which
// would undo the edit.
func (e *Editor) load(ctx context.Context, name string, write bool) (*store.Character, store.CharacterRef, error) {
	ref, err := e.find(ctx, name)
	if err != nil {
		return nil, ref, err
	}
	if ref.Online, err = e.st.AccountOnline(ctx, ref.AccountID); err != nil {
		return nil, ref, err
	}
	if write && ref.Online {
		return nil, ref, ErrOnline
	}
	c, err := e.st.LoadCharacter(ctx, ref.AccountID, ref.Name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ref, ErrNotFound
	}
	return c, ref, err
}

// ItemView is one occupied slot.
type ItemView struct {
	Slot       int      `json:"slot"`
	Index      int      `json:"index"`
	Name       string   `json:"name"`
	Level      int      `json:"level"`
	Skill      bool     `json:"skill"`
	Luck       bool     `json:"luck"`
	Option     int      `json:"option"`
	Excellent  int      `json:"excellent"`
	ExcLabels  []string `json:"excLabels,omitempty"`
	Durability int      `json:"dur"`
	Serial     uint32   `json:"serial"`
	Width      int      `json:"w"`
	Height     int      `json:"h"`
}

// CharacterView is the editor page of one character.
type CharacterView struct {
	Name          string     `json:"name"`
	AccountID     int64      `json:"accountId"`
	Online        bool       `json:"online"`
	Class         byte       `json:"class"`
	Level         int        `json:"level"`
	LevelUpPoints int        `json:"points"`
	Strength      int        `json:"str"`
	Dexterity     int        `json:"dex"`
	Vitality      int        `json:"vit"`
	Energy        int        `json:"ene"`
	Leadership    int        `json:"cmd"`
	MaxLevel      int        `json:"maxLevel"`
	MaxStat       int        `json:"maxStat"` // MaxStatPoint of the account level
	Money         int64      `json:"zen"`
	Resets        int        `json:"resets"`
	Map           int        `json:"map"`
	X             int        `json:"x"`
	Y             int        `json:"y"`
	Inventory     []ItemView `json:"inventory"`
	Vault         []ItemView `json:"vault"`
	VaultMoney    int64      `json:"vaultZen"`
}

// Character returns the full view of a character and its account vault.
func (e *Editor) Character(ctx context.Context, name string) (*CharacterView, error) {
	c, ref, err := e.load(ctx, name, false)
	if err != nil {
		return nil, err
	}
	online := ref.Online
	v := &CharacterView{
		Name: c.Name, AccountID: c.AccountID, Online: online, Class: c.Class, Level: c.Level,
		LevelUpPoints: c.LevelUpPoints, Strength: c.Strength, Dexterity: c.Dexterity, Vitality: c.Vitality,
		Energy: c.Energy, Leadership: c.Leadership, Money: c.Money, Resets: c.Resets,
		Map: c.Map, X: c.X, Y: c.Y, MaxLevel: e.lim.MaxLevel,
	}
	al, err := e.accountLevel(ctx, ref.Login)
	if err != nil {
		return nil, err
	}
	v.MaxStat = e.maxStat(al)
	v.Inventory = e.views(c.Inventory, bagStart+bagRows*gridCols)
	if online {
		// Reading the vault would create it (LoadWarehouse inserts on first
		// use); skip it while the game owns the account.
		return v, nil
	}
	w, err := e.st.LoadWarehouse(ctx, c.AccountID, 0)
	if err != nil {
		return nil, err
	}
	v.Vault, v.VaultMoney = e.views(w.Items, warehouseSlots), w.Money
	return v, nil
}

func (e *Editor) views(blob []byte, n int) []ItemView {
	out := []ItemView{}
	for s := 0; s < n && (s+1)*item.DBSize <= len(blob); s++ {
		it, ok := item.DecodeDB(blob[s*item.DBSize:])
		if !ok {
			continue
		}
		iv := ItemView{
			Slot: s, Index: it.Index, Level: it.Level, Skill: it.Skill, Luck: it.Luck, Option: it.Option,
			Excellent: int(it.Excellent), Durability: int(it.Durability), Serial: it.Serial, Width: 1, Height: 1,
			Name: fmt.Sprintf("Item %d,%d", it.Index/512, it.Index%512),
		}
		if ci, ok := e.cat.Item(it.Index); ok {
			iv.Name, iv.Width, iv.Height = ci.Name, ci.Width, ci.Height
			for _, x := range ci.Exc {
				if int(it.Excellent)&x.Bit != 0 {
					iv.ExcLabels = append(iv.ExcLabels, x.Label)
				}
			}
		}
		out = append(out, iv)
	}
	return out
}

// Stats are the editable character fields.
type Stats struct {
	Level         int   `json:"level"`
	LevelUpPoints int   `json:"points"`
	Strength      int   `json:"str"`
	Dexterity     int   `json:"dex"`
	Vitality      int   `json:"vit"`
	Energy        int   `json:"ene"`
	Leadership    int   `json:"cmd"`
	Money         int64 `json:"zen"`
	Resets        int   `json:"resets"`
}

// maxZen is the inventory zen cap (game/inventory.go moneyFits).
const maxZen = 2000000000

// UpdateStats writes level, points, stats, zen and resets. Stats stop at
// MaxStatPoint of the account level. When the level changes the experience
// is set to the start of the new level, like /edit (FUN_0050be30 raises it
// to the floor; lowering the level would otherwise leave experience above
// the next level and chain level-ups on the next kill).
func (e *Editor) UpdateStats(ctx context.Context, name string, s Stats) error {
	if s.Level < 1 || s.Level > e.lim.MaxLevel || s.LevelUpPoints < 0 || s.Money < 0 || s.Money > maxZen || s.Resets < 0 {
		return ErrInvalid
	}
	c, ref, err := e.load(ctx, name, true)
	if err != nil {
		return err
	}
	al, err := e.accountLevel(ctx, ref.Login)
	if err != nil {
		return err
	}
	maxStat := e.maxStat(al)
	for _, v := range []int{s.Strength, s.Dexterity, s.Vitality, s.Energy} {
		if v < 1 || v > maxStat {
			return ErrInvalid
		}
	}
	if s.Leadership < 0 || s.Leadership > maxStat {
		return ErrInvalid
	}
	if c.Level != s.Level && e.lim.Exp != nil {
		c.Experience = int64(e.lim.Exp.Floor(s.Level))
	}
	c.Level, c.LevelUpPoints, c.Money, c.Resets = s.Level, s.LevelUpPoints, s.Money, s.Resets
	c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership = s.Strength, s.Dexterity, s.Vitality, s.Energy, s.Leadership
	return e.st.SaveCharacter(ctx, c)
}

// NewItem describes an item to add.
type NewItem struct {
	Container  Container `json:"container"`
	Slot       int       `json:"slot"` // -1 = first free bag / vault position
	Index      int       `json:"index"`
	Level      int       `json:"level"`
	Skill      bool      `json:"skill"`
	Luck       bool      `json:"luck"`
	Option     int       `json:"option"`
	Excellent  int       `json:"excellent"`
	Durability int       `json:"dur"` // 0 = maximum (quantity for potions)
}

// build validates the request against the catalog and makes the item.
func (e *Editor) build(n NewItem) (item.Item, CatalogItem, error) {
	ci, ok := e.cat.Item(n.Index)
	if !ok || n.Level < 0 || n.Level > 15 || n.Option < 0 || n.Option > 7 ||
		n.Durability < 0 || n.Durability > 255 || n.Excellent < 0 || n.Excellent > 63 {
		return item.Item{}, ci, ErrInvalid
	}
	if (n.Skill && !ci.Skill) || (n.Luck && !ci.Luck) || (n.Option > 0 && ci.Option == "") {
		return item.Item{}, ci, ErrInvalid
	}
	allowed := 0
	for _, x := range ci.Exc {
		allowed |= x.Bit
	}
	if n.Excellent&^allowed != 0 {
		return item.Item{}, ci, ErrInvalid
	}
	it := item.Item{
		Index: n.Index, Level: n.Level, Skill: n.Skill, Luck: n.Luck, Option: n.Option,
		Excellent: byte(n.Excellent),
	}
	dur := n.Durability
	if dur == 0 {
		dur = game.NewItemDurability(e.cat.info[n.Index], it)
	}
	it.Durability = float32(dur)
	return it, ci, nil
}

// AddItem puts a new item in the character's inventory or account vault
// and returns its slot.
func (e *Editor) AddItem(ctx context.Context, name string, n NewItem) (int, error) {
	it, ci, err := e.build(n)
	if err != nil {
		return -1, err
	}
	c, _, err := e.load(ctx, name, true)
	if err != nil {
		return -1, err
	}
	if e.cat.info[n.Index].HaveSerial != 0 {
		if it.Serial, err = e.st.NextItemSerial(ctx); err != nil {
			return -1, err
		}
	}
	switch n.Container {
	case Inventory:
		slot, err := e.place(c.Inventory, ci, n.Slot, bagStart, bagRows, true)
		if err != nil {
			return -1, err
		}
		it.EncodeDB(c.Inventory[slot*item.DBSize:])
		return slot, e.st.SaveInventory(ctx, c.ID, c.Inventory)
	case Vault:
		w, err := e.st.LoadWarehouse(ctx, c.AccountID, 0)
		if err != nil {
			return -1, err
		}
		slot, err := e.place(w.Items, ci, n.Slot, 0, warehouseRows, false)
		if err != nil {
			return -1, err
		}
		it.EncodeDB(w.Items[slot*item.DBSize:])
		return slot, e.st.SaveWarehouse(ctx, w)
	}
	return -1, ErrInvalid
}

// RemoveItem clears one slot.
func (e *Editor) RemoveItem(ctx context.Context, name string, ct Container, slot int) error {
	c, _, err := e.load(ctx, name, true)
	if err != nil {
		return err
	}
	switch ct {
	case Inventory:
		if slot < 0 || slot >= bagStart+bagRows*gridCols {
			return ErrInvalid
		}
		item.Empty().EncodeDB(c.Inventory[slot*item.DBSize:])
		return e.st.SaveInventory(ctx, c.ID, c.Inventory)
	case Vault:
		if slot < 0 || slot >= warehouseSlots {
			return ErrInvalid
		}
		w, err := e.st.LoadWarehouse(ctx, c.AccountID, 0)
		if err != nil {
			return err
		}
		item.Empty().EncodeDB(w.Items[slot*item.DBSize:])
		return e.st.SaveWarehouse(ctx, w)
	}
	return ErrInvalid
}

// place picks the target slot of a new item. In the inventory, slot 0..11
// equips it when Item.txt allows that slot (rings use 10 and 11); slot -1
// takes the first grid position where it fits, scanning rows then columns
// like FUN_00495610.
func (e *Editor) place(blob []byte, ci CatalogItem, slot, start, rows int, equip bool) (int, error) {
	if equip && slot >= 0 && slot < equipSlots {
		ok := ci.Slot == slot || (ci.Slot == 10 && slot == 11) || (ci.Slot == 0 && slot == 1 && ci.Width == 1)
		if !ok {
			return -1, ErrInvalid
		}
		if _, used := item.DecodeDB(blob[slot*item.DBSize:]); used {
			return -1, ErrNoSpace
		}
		return slot, nil
	}
	g := e.grid(blob, start, rows)
	fits := func(s int) bool {
		x, y := (s-start)%gridCols, (s-start)/gridCols
		if x+ci.Width > gridCols || y+ci.Height > rows {
			return false
		}
		for dy := 0; dy < ci.Height; dy++ {
			for dx := 0; dx < ci.Width; dx++ {
				if g[y+dy][x+dx] {
					return false
				}
			}
		}
		return true
	}
	if slot >= 0 {
		if slot < start || slot >= start+rows*gridCols || !fits(slot) {
			return -1, ErrNoSpace
		}
		return slot, nil
	}
	for s := start; s < start+rows*gridCols; s++ {
		if fits(s) {
			return s, nil
		}
	}
	return -1, ErrNoSpace
}

// grid is the occupancy of a rows x 8 area starting at slot start.
func (e *Editor) grid(blob []byte, start, rows int) [][gridCols]bool {
	g := make([][gridCols]bool, rows)
	for s := start; s < start+rows*gridCols; s++ {
		it, ok := item.DecodeDB(blob[s*item.DBSize:])
		if !ok {
			continue
		}
		w, h := 1, 1
		if ci, ok := e.cat.Item(it.Index); ok {
			w, h = ci.Width, ci.Height
		}
		x, y := (s-start)%gridCols, (s-start)/gridCols
		for dy := 0; dy < h && y+dy < rows; dy++ {
			for dx := 0; dx < w && x+dx < gridCols; dx++ {
				g[y+dy][x+dx] = true
			}
		}
	}
	return g
}
