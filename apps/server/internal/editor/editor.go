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

// Live is the in-memory state of playing characters (*game.Hub when the
// editor runs inside the muserver process). Without it an online account
// is read-only.
type Live interface {
	EditCharacter(ctx context.Context, name string, f func(*game.LiveEdit) error) (bool, error)
	AccountCharacter(login string) (string, bool)
	LockVault(ctx context.Context, name string) (loaded bool, release func(), err error)
}

// Errors returned to the HTTP layer.
var (
	ErrOnline   = errors.New("a conta está online: deslogue o personagem antes de editar")
	ErrNotFound = errors.New("personagem não encontrado")
	ErrInvalid  = errors.New("dados inválidos")
	ErrNoSpace  = errors.New("não há espaço livre para o item")
	ErrBusy     = errors.New("o personagem está entrando ou saindo do jogo, ou o baú está em uso: tente novamente")
)

// liveErr maps the game's transient states to ErrBusy.
func liveErr(err error) error {
	if errors.Is(err, game.ErrCharLeaving) || errors.Is(err, game.ErrVaultBusy) {
		return ErrBusy
	}
	return err
}

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

// Editor applies edits to characters: in the store, or in the game's
// memory for playing characters when it has a Live.
type Editor struct {
	st   Store
	cat  *Catalog
	lim  Limits
	live Live
}

// New returns an editor over a store and the item catalog.
func New(st Store, cat *Catalog, lim Limits) *Editor {
	return &Editor{st: st, cat: cat, lim: lim}
}

// WithLive makes edits of playing characters go through l.
func (e *Editor) WithLive(l Live) *Editor {
	e.live = l
	return e
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

// load returns the character from the store and its search entry. Without
// a Live it fails with ErrOnline when write is set and the account is
// connected: the GameServer keeps online characters in memory and saves
// them back on logout, which would undo the edit. With a Live, callers
// reach the store only for characters that are not playing.
func (e *Editor) load(ctx context.Context, name string, write bool) (*store.Character, store.CharacterRef, error) {
	ref, err := e.find(ctx, name)
	if err != nil {
		return nil, ref, err
	}
	if ref.Online, err = e.st.AccountOnline(ctx, ref.AccountID); err != nil {
		return nil, ref, err
	}
	if write && ref.Online && e.live == nil {
		return nil, ref, ErrOnline
	}
	c, err := e.st.LoadCharacter(ctx, ref.AccountID, ref.Name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ref, ErrNotFound
	}
	return c, ref, err
}

// target resolves the character of an edit; like load, an online account
// is refused without a Live.
func (e *Editor) target(ctx context.Context, name string) (store.CharacterRef, error) {
	ref, err := e.find(ctx, name)
	if err != nil || e.live != nil {
		return ref, err
	}
	online, err := e.st.AccountOnline(ctx, ref.AccountID)
	if err == nil && online {
		err = ErrOnline
	}
	return ref, err
}

// editLive runs f on a playing character in the game's memory. done is
// false when the character is not playing (or there is no Live) and the
// store holds it.
func (e *Editor) editLive(ctx context.Context, name string, f func(*game.LiveEdit) error) (bool, error) {
	if e.live == nil {
		return false, nil
	}
	done, err := e.live.EditCharacter(ctx, name, f)
	return done, liveErr(err)
}

// editVault applies edit to the items of the account vault (warehouse 0):
// in the game's memory when the playing character of the account has it
// open, otherwise in the store, with the vault locked in the game so the
// player cannot open it meanwhile.
func (e *Editor) editVault(ctx context.Context, ref store.CharacterRef, edit func(items []byte) error) error {
	if e.live != nil {
		if holder, ok := e.live.AccountCharacter(ref.Login); ok {
			loaded, release, err := e.live.LockVault(ctx, holder)
			if err != nil {
				return liveErr(err)
			}
			if loaded {
				done, err := e.editLive(ctx, holder, func(le *game.LiveEdit) error {
					if !le.VaultLoaded || le.VaultNumber != 0 {
						return game.ErrVaultBusy
					}
					return edit(le.Vault)
				})
				if err == nil && !done {
					err = ErrBusy
				}
				return err
			}
			defer release()
		}
	}
	w, err := e.st.LoadWarehouse(ctx, ref.AccountID, 0)
	if err != nil {
		return err
	}
	if err := edit(w.Items); err != nil {
		return err
	}
	return e.st.SaveWarehouse(ctx, w)
}

// vault returns the items and zen of the account vault, from the game's
// memory while the playing character of the account has it open.
func (e *Editor) vault(ctx context.Context, ref store.CharacterRef) ([]byte, int64, error) {
	if e.live != nil {
		if holder, ok := e.live.AccountCharacter(ref.Login); ok {
			var items []byte
			var money int64
			if _, err := e.live.EditCharacter(ctx, holder, func(le *game.LiveEdit) error {
				if le.VaultLoaded && le.VaultNumber == 0 {
					items, money = le.Vault, le.VaultMoney
				}
				return nil
			}); err != nil {
				return nil, 0, liveErr(err)
			}
			if items != nil {
				return items, money, nil
			}
		}
	}
	w, err := e.st.LoadWarehouse(ctx, ref.AccountID, 0)
	if err != nil {
		return nil, 0, err
	}
	return w.Items, w.Money, nil
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
	Live          bool       `json:"live"` // online accounts are editable (changes reach the game)
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
	// A playing character is shown as the game holds it.
	if _, err := e.editLive(ctx, ref.Name, func(le *game.LiveEdit) error {
		lc := le.Char
		lc.Inventory = le.Inventory
		c = &lc
		return nil
	}); err != nil {
		return nil, err
	}
	online := ref.Online
	v := &CharacterView{
		Name: c.Name, AccountID: c.AccountID, Online: online, Live: e.live != nil, Class: c.Class, Level: c.Level,
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
	if online && e.live == nil {
		// Reading the vault would create it (LoadWarehouse inserts on first
		// use); skip it while the game owns the account.
		return v, nil
	}
	items, money, err := e.vault(ctx, ref)
	if err != nil {
		return nil, err
	}
	v.Vault, v.VaultMoney = e.views(items, warehouseSlots), money
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
	ref, err := e.target(ctx, name)
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
	apply := func(c *store.Character) {
		if c.Level != s.Level && e.lim.Exp != nil {
			c.Experience = int64(e.lim.Exp.Floor(s.Level))
		}
		c.Level, c.LevelUpPoints, c.Money, c.Resets = s.Level, s.LevelUpPoints, s.Money, s.Resets
		c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership = s.Strength, s.Dexterity, s.Vitality, s.Energy, s.Leadership
	}
	if done, err := e.editLive(ctx, ref.Name, func(le *game.LiveEdit) error {
		apply(&le.Char)
		return nil
	}); done || err != nil {
		return err
	}
	c, _, err := e.load(ctx, ref.Name, true)
	if err != nil {
		return err
	}
	apply(c)
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
	if n.Container != Inventory && n.Container != Vault {
		return -1, ErrInvalid
	}
	ref, err := e.target(ctx, name)
	if err != nil {
		return -1, err
	}
	if e.cat.info[n.Index].HaveSerial != 0 {
		if it.Serial, err = e.st.NextItemSerial(ctx); err != nil {
			return -1, err
		}
	}
	slot := -1
	put := func(blob []byte, start, rows int, equip bool) error {
		s, err := e.place(blob, ci, n.Slot, start, rows, equip)
		if err != nil {
			return err
		}
		it.EncodeDB(blob[s*item.DBSize:])
		slot = s
		return nil
	}
	if n.Container == Vault {
		err := e.editVault(ctx, ref, func(items []byte) error { return put(items, 0, warehouseRows, false) })
		return slot, err
	}
	if done, err := e.editLive(ctx, ref.Name, func(le *game.LiveEdit) error {
		return put(le.Inventory, bagStart, bagRows, true)
	}); done || err != nil {
		return slot, err
	}
	c, _, err := e.load(ctx, ref.Name, true)
	if err != nil {
		return -1, err
	}
	if err := put(c.Inventory, bagStart, bagRows, true); err != nil {
		return -1, err
	}
	return slot, e.st.SaveInventory(ctx, c.ID, c.Inventory)
}

// RemoveItem clears one slot.
func (e *Editor) RemoveItem(ctx context.Context, name string, ct Container, slot int) error {
	switch {
	case ct == Inventory && slot >= 0 && slot < bagStart+bagRows*gridCols:
	case ct == Vault && slot >= 0 && slot < warehouseSlots:
	default:
		return ErrInvalid
	}
	ref, err := e.target(ctx, name)
	if err != nil {
		return err
	}
	clear := func(blob []byte) error {
		item.Empty().EncodeDB(blob[slot*item.DBSize:])
		return nil
	}
	if ct == Vault {
		return e.editVault(ctx, ref, clear)
	}
	if done, err := e.editLive(ctx, ref.Name, func(le *game.LiveEdit) error {
		return clear(le.Inventory)
	}); done || err != nil {
		return err
	}
	c, _, err := e.load(ctx, ref.Name, true)
	if err != nil {
		return err
	}
	clear(c.Inventory)
	return e.st.SaveInventory(ctx, c.ID, c.Inventory)
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
