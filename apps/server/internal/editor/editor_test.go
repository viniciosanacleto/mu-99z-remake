package editor

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func configDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "config")
}

var (
	catOnce sync.Once
	catVal  *Catalog
	catErr  error
)

// testCatalog builds the catalog from the real Item.txt / ItemOption.txt.
func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	catOnce.Do(func() {
		items, err := gamedata.LoadItems(configDir())
		if err != nil {
			catErr = err
			return
		}
		opts, err := gamedata.LoadItemOptions(configDir())
		if err != nil {
			catErr = err
			return
		}
		catVal = NewCatalog(items, opts)
	})
	if catErr != nil {
		t.Fatal(catErr)
	}
	return catVal
}

func byName(t *testing.T, c *Catalog, cat int, name string) CatalogItem {
	t.Helper()
	for _, it := range c.Items {
		if it.Category == cat && it.Name == name {
			return it
		}
	}
	t.Fatalf("item %q not in category %d", name, cat)
	return CatalogItem{}
}

func excLabels(ci CatalogItem) map[int]string {
	m := map[int]string{}
	for _, x := range ci.Exc {
		m[x.Bit] = x.Label
	}
	return m
}

// Values from data/Data/Item/ItemOption.txt ("General Weapon", "General
// Armor", "Wings of Spirit") and Item.txt.
func TestCatalog(t *testing.T) {
	c := testCatalog(t)
	if !slices.IsSortedFunc(c.Items, func(a, b CatalogItem) int { return a.Index - b.Index }) {
		t.Fatal("catalog not sorted by index")
	}

	kris := byName(t, c, 0, "Kris")
	if kris.Index != 0 || kris.Width != 1 || kris.Height != 2 || kris.Slot != 0 || !kris.Luck || kris.Stack {
		t.Fatalf("Kris = %+v", kris)
	}
	if kris.Option != "Dano +4 por nível da opção" {
		t.Fatalf("Kris option = %q", kris.Option)
	}
	want := map[int]string{
		1: "Mana ao matar monstro (12)", 2: "HP ao matar monstro (12)", 4: "Velocidade de ataque +7",
		8: "Dano +2%", 16: "Dano +nível/20", 32: "Chance de dano excelente +10%",
	}
	if got := excLabels(kris); !mapsEqual(got, want) {
		t.Fatalf("Kris excellent = %v", got)
	}

	boots := byName(t, c, 11, "Bronze Boots")
	if boots.Index != item.Make(11, 0) || boots.Width != 2 || boots.Height != 2 || boots.Slot != 6 {
		t.Fatalf("Bronze Boots = %+v", boots)
	}
	if boots.Option != "Defesa +4 por nível da opção" || excLabels(boots)[1] != "Zen dropado +30%" ||
		excLabels(boots)[32] != "HP máximo +4%" {
		t.Fatalf("Bronze Boots options = %q %v", boots.Option, excLabels(boots))
	}

	// Wings of Spirit: bit 32 switches the additional option (slot 2 rule).
	spirit, ok := c.Item(item.Make(12, 3))
	if !ok || excLabels(spirit)[32] != "Opção adicional vira: Recuperação de HP +1% por nível da opção" ||
		excLabels(spirit)[1] != "HP +50 +5 por nível do item" {
		t.Fatalf("Wings of Spirit = %+v", spirit)
	}

	for _, it := range c.Items {
		if it.Category == 14 && (!it.Stack || it.Luck || len(it.Exc) != 0) {
			t.Fatalf("category 14 item %+v", it)
		}
	}
}

func mapsEqual(a, b map[int]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type fixture struct {
	ed   *Editor
	st   *store.Memory
	acc  *store.Account
	cat  *Catalog
	name string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	m, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	acc, err := m.CreateAccount(ctx, "arthur", "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	if res, _, err := m.CreateCharacter(ctx, acc.ID, "Heroi", 16); err != nil || res != store.CreateOK {
		t.Fatalf("create = %v, %v", res, err)
	}
	cat := testCatalog(t)
	return &fixture{ed: New(m, cat, testLimits), st: m, acc: acc, cat: cat, name: "Heroi"}
}

// testLimits mirrors 4.GameServer Common.dat (MaxStatPoint_AL* = 32767,
// ExperienceMultiplierConstA/B = 1), with AL2 raised to tell the levels apart.
var testLimits = Limits{MaxLevel: 400, MaxStat: [4]int{32767, 32767, 40000, 32767}, Exp: gamedata.NewExperience(1, 1, 1)}

func (f *fixture) vault(t *testing.T) []byte {
	t.Helper()
	w, err := f.st.LoadWarehouse(context.Background(), f.acc.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w.Items
}

func (f *fixture) inventory(t *testing.T) []byte {
	t.Helper()
	c, err := f.st.LoadCharacter(context.Background(), f.acc.ID, f.name)
	if err != nil {
		t.Fatal(err)
	}
	return c.Inventory
}

func decode(t *testing.T, blob []byte, slot int) item.Item {
	t.Helper()
	it, ok := item.DecodeDB(blob[slot*item.DBSize:])
	if !ok {
		t.Fatalf("slot %d is empty", slot)
	}
	return it
}

func TestAddItemEncodesOptions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(ctx, "heroi", NewItem{Container: Vault, Slot: -1, Index: kris.Index,
		Level: 9, Luck: true, Option: 5, Excellent: 1 | 32})
	if err != nil || slot != 0 {
		t.Fatalf("AddItem = %d, %v", slot, err)
	}
	it := decode(t, f.vault(t), 0)
	if it.Index != kris.Index || it.Level != 9 || !it.Luck || it.Skill || it.Option != 5 || it.Excellent != 33 {
		t.Fatalf("stored = %+v", it)
	}
	info := f.cat.info[kris.Index]
	if want := game.NewItemDurability(info, it); int(it.Durability) != want || want == 0 {
		t.Fatalf("durability = %v, want %d", it.Durability, want)
	}
	if info.HaveSerial != 0 && it.Serial == 0 {
		t.Fatal("serial not assigned")
	}

	// An explicit durability (potion quantity) is kept.
	potion := byName(t, f.cat, 14, "Small Healing Potion")
	slot, err = f.ed.AddItem(ctx, "Heroi", NewItem{Container: Vault, Slot: -1, Index: potion.Index, Durability: 20})
	if err != nil {
		t.Fatal(err)
	}
	if it := decode(t, f.vault(t), slot); it.Durability != 20 {
		t.Fatalf("potion quantity = %v", it.Durability)
	}
}

func TestAddItemPlacement(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boots := byName(t, f.cat, 11, "Bronze Boots") // 2x2
	sword := byName(t, f.cat, 0, "Short Sword")   // 1x3

	add := func(n NewItem) (int, error) { return f.ed.AddItem(ctx, f.name, n) }
	if s, err := add(NewItem{Container: Vault, Slot: -1, Index: boots.Index}); err != nil || s != 0 {
		t.Fatalf("boots = %d, %v", s, err)
	}
	// Columns 0..1 of rows 0..1 are taken: the sword goes to column 2.
	if s, err := add(NewItem{Container: Vault, Slot: -1, Index: sword.Index}); err != nil || s != 2 {
		t.Fatalf("sword = %d, %v", s, err)
	}
	// Slot 9 is row 1 column 1, covered by the boots.
	if _, err := add(NewItem{Container: Vault, Slot: 9, Index: sword.Index}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("occupied slot err = %v", err)
	}
	// Column 7 cannot hold a 2-wide item.
	if _, err := add(NewItem{Container: Vault, Slot: 7, Index: boots.Index}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("overflow err = %v", err)
	}
	// Last row (14) cannot hold a 3-tall sword.
	if _, err := add(NewItem{Container: Vault, Slot: 14 * 8, Index: sword.Index}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("bottom overflow err = %v", err)
	}
	if s, err := add(NewItem{Container: Vault, Slot: 12 * 8, Index: sword.Index}); err != nil || s != 96 {
		t.Fatalf("explicit slot = %d, %v", s, err)
	}

	// Equipment: boots only in slot 6, and only when it is free.
	inv := f.inventory(t)
	if _, used := item.DecodeDB(inv[6*item.DBSize:]); !used {
		if s, err := add(NewItem{Container: Inventory, Slot: 6, Index: boots.Index}); err != nil || s != 6 {
			t.Fatalf("equip boots = %d, %v", s, err)
		}
	}
	if _, err := add(NewItem{Container: Inventory, Slot: 6, Index: boots.Index}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("equip on used slot err = %v", err)
	}
	if _, err := add(NewItem{Container: Inventory, Slot: 2, Index: boots.Index}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("boots as helm err = %v", err)
	}

	// The bag is filled in reading order.
	s, err := add(NewItem{Container: Inventory, Slot: -1, Index: boots.Index})
	if err != nil || s < bagStart || s >= bagStart+64 {
		t.Fatalf("bag slot = %d, %v", s, err)
	}
	if it := decode(t, f.inventory(t), s); it.Index != boots.Index {
		t.Fatalf("bag item = %+v", it)
	}
}

func TestAddItemFull(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	boots := byName(t, f.cat, 11, "Bronze Boots")
	for i := 0; i < 4*7; i++ { // 8x15 vault holds 4 x 7 boots
		if _, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: boots.Index}); err != nil {
			t.Fatalf("boots %d: %v", i, err)
		}
	}
	if _, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: boots.Index}); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("full vault err = %v", err)
	}
}

func TestAddItemValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	kris := byName(t, f.cat, 0, "Kris")
	potion := byName(t, f.cat, 14, "Small Healing Potion")
	cases := map[string]NewItem{
		"unknown item":     {Index: 0x1FFF},
		"level 16":         {Index: kris.Index, Level: 16},
		"option 8":         {Index: kris.Index, Option: 8},
		"skill on Kris":    {Index: kris.Index, Skill: !kris.Skill},
		"luck on potion":   {Index: potion.Index, Luck: true},
		"option on potion": {Index: potion.Index, Option: 1},
		"exc on potion":    {Index: potion.Index, Excellent: 1},
		"exc bit 64":       {Index: kris.Index, Excellent: 64},
		"durability 256":   {Index: kris.Index, Durability: 256},
		"bad container":    {Index: kris.Index, Container: "bank"},
	}
	for name, n := range cases {
		if n.Container == "" {
			n.Container = Vault
		}
		n.Slot = -1
		if _, err := f.ed.AddItem(ctx, f.name, n); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := f.ed.AddItem(ctx, "Ninguem", NewItem{Container: Vault, Slot: -1, Index: kris.Index}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown character err = %v", err)
	}
}

func TestOnlineAccountIsReadOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.st.ConnectAccount(ctx, f.acc.ID, "Principal", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	kris := byName(t, f.cat, 0, "Kris")
	if _, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: kris.Index}); !errors.Is(err, ErrOnline) {
		t.Fatalf("AddItem online err = %v", err)
	}
	if err := f.ed.RemoveItem(ctx, f.name, Inventory, 0); !errors.Is(err, ErrOnline) {
		t.Fatalf("RemoveItem online err = %v", err)
	}
	if err := f.ed.UpdateStats(ctx, f.name, Stats{Level: 10, Strength: 1, Dexterity: 1, Vitality: 1, Energy: 1}); !errors.Is(err, ErrOnline) {
		t.Fatalf("UpdateStats online err = %v", err)
	}
	v, err := f.ed.Character(ctx, f.name)
	if err != nil || !v.Online || v.Vault != nil {
		t.Fatalf("view online = %+v, %v", v, err)
	}
}

func TestCharacterViewAndRemove(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Inventory, Slot: -1, Index: kris.Index, Level: 3, Excellent: 4})
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.ed.Character(ctx, "HEROI")
	if err != nil || v.Name != "Heroi" || v.Online || v.Class != 16 {
		t.Fatalf("view = %+v, %v", v, err)
	}
	i := slices.IndexFunc(v.Inventory, func(iv ItemView) bool { return iv.Slot == slot })
	if i < 0 {
		t.Fatalf("slot %d missing from %+v", slot, v.Inventory)
	}
	iv := v.Inventory[i]
	if iv.Name != "Kris" || iv.Level != 3 || iv.Width != 1 || iv.Height != 2 ||
		!slices.Equal(iv.ExcLabels, []string{"Velocidade de ataque +7"}) {
		t.Fatalf("item view = %+v", iv)
	}
	if v.Vault == nil {
		t.Fatal("vault missing for an offline account")
	}

	if err := f.ed.RemoveItem(ctx, f.name, Inventory, slot); err != nil {
		t.Fatal(err)
	}
	if _, used := item.DecodeDB(f.inventory(t)[slot*item.DBSize:]); used {
		t.Fatal("slot not cleared")
	}
	if err := f.ed.RemoveItem(ctx, f.name, Inventory, 76); !errors.Is(err, ErrInvalid) {
		t.Fatalf("remove outside bag err = %v", err)
	}
	if err := f.ed.RemoveItem(ctx, f.name, Vault, 120); !errors.Is(err, ErrInvalid) {
		t.Fatalf("remove outside vault err = %v", err)
	}
}

func TestUpdateStats(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s := Stats{Level: 350, LevelUpPoints: 100, Strength: 500, Dexterity: 200, Vitality: 300, Energy: 50,
		Leadership: 0, Money: 2000000000, Resets: 3}
	if err := f.ed.UpdateStats(ctx, f.name, s); err != nil {
		t.Fatal(err)
	}
	c, _ := f.st.LoadCharacter(ctx, f.acc.ID, f.name)
	if c.Level != 350 || c.LevelUpPoints != 100 || c.Strength != 500 || c.Dexterity != 200 || c.Vitality != 300 ||
		c.Energy != 50 || c.Money != 2000000000 || c.Resets != 3 {
		t.Fatalf("saved = %+v", c)
	}
	for name, mod := range map[string]func(*Stats){
		"level above ServerMaxLevel": func(s *Stats) { s.Level = 401 },
		"level 0":                    func(s *Stats) { s.Level = 0 },
		"zen above cap":              func(s *Stats) { s.Money = 2000000001 },
		"negative points":            func(s *Stats) { s.LevelUpPoints = -1 },
		"strength 0":                 func(s *Stats) { s.Strength = 0 },
		"energy above MaxStatPoint":  func(s *Stats) { s.Energy = 32768 },
		"leadership above max":       func(s *Stats) { s.Leadership = 32768 },
	} {
		bad := s
		mod(&bad)
		if err := f.ed.UpdateStats(ctx, f.name, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestUpdateStatsLimitsAndExperience(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	exp := testLimits.Exp
	s := Stats{Level: 250, Strength: 32767, Dexterity: 25, Vitality: 25, Energy: 25}
	if err := f.ed.UpdateStats(ctx, f.name, s); err != nil {
		t.Fatal(err)
	}
	c, _ := f.st.LoadCharacter(ctx, f.acc.ID, f.name)
	if c.Level != 250 || c.Experience != int64(exp.Floor(250)) || c.Experience == 0 {
		t.Fatalf("level %d exp %d, want exp %d", c.Level, c.Experience, exp.Floor(250))
	}
	// Same level: the experience earned inside the level is kept.
	c.Experience += 1000
	if err := f.st.SaveCharacter(ctx, c); err != nil {
		t.Fatal(err)
	}
	s.LevelUpPoints = 7
	if err := f.ed.UpdateStats(ctx, f.name, s); err != nil {
		t.Fatal(err)
	}
	if c, _ = f.st.LoadCharacter(ctx, f.acc.ID, f.name); c.Experience != int64(exp.Floor(250))+1000 {
		t.Fatalf("exp changed without a level change: %d", c.Experience)
	}
	// Lowering the level drops the experience to the new floor.
	s.Level = 100
	if err := f.ed.UpdateStats(ctx, f.name, s); err != nil {
		t.Fatal(err)
	}
	if c, _ = f.st.LoadCharacter(ctx, f.acc.ID, f.name); c.Experience != int64(exp.Floor(100)) {
		t.Fatalf("exp %d want %d", c.Experience, exp.Floor(100))
	}

	// The cap follows the account level.
	s.Strength = 40000
	if err := f.ed.UpdateStats(ctx, f.name, s); !errors.Is(err, ErrInvalid) {
		t.Fatalf("AL0 40000 err = %v", err)
	}
	if err := f.st.SetAccountLevel(ctx, f.acc.ID, 2, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := f.ed.UpdateStats(ctx, f.name, s); err != nil {
		t.Fatalf("AL2 40000 err = %v", err)
	}
	v, err := f.ed.Character(ctx, f.name)
	if err != nil {
		t.Fatal(err)
	}
	if v.MaxStat != 40000 || v.MaxLevel != 400 || v.Strength != 40000 {
		t.Fatalf("view maxStat %d maxLevel %d str %d", v.MaxStat, v.MaxLevel, v.Strength)
	}
}
