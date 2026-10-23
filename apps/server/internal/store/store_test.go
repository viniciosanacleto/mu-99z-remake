package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestClassTemplates(t *testing.T) {
	ts, err := ClassTemplates()
	if err != nil {
		t.Fatal(err)
	}
	// MuOnline.sql DefaultClassType: DW, DK, Elf, MG, DL.
	if len(ts) != 5 {
		t.Fatalf("%d templates", len(ts))
	}
	elf := ts[32]
	if elf.Strength != 22 || elf.Dexterity != 25 || elf.Vitality != 20 || elf.Energy != 15 ||
		elf.Map != 3 || elf.X != 172 || elf.Y != 97 || elf.MaxLife != 80 || elf.MaxMana != 30 {
		t.Fatalf("elf = %+v", elf)
	}
	for c, tpl := range ts {
		if len(tpl.Inventory) != InventorySize || len(tpl.MagicList) != MagicListSize ||
			len(tpl.Quest) != QuestSize || len(tpl.EffectList) != EffectListSize {
			t.Errorf("class %d blob sizes wrong", c)
		}
	}
	// DK starts with a Short Sword (cat 0 idx 1? first item bytes 00 00 12) in slot 0.
	if !bytes.Equal(ts[16].Inventory[:3], []byte{0x00, 0x00, 0x12}) {
		t.Fatalf("DK slot 0 = % X", ts[16].Inventory[:16])
	}
}

// conformance runs the same scenario against every implementation.
func conformance(t *testing.T, s Store) {
	ctx := context.Background()
	a, err := s.CreateAccount(ctx, "tester", "plain:pw", "1234567890123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAccount(ctx, "TESTER", "x", ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate login err = %v", err)
	}
	got, err := s.AccountByLogin(ctx, "Tester")
	if err != nil || got.ID != a.ID || got.PersonalID != "1234567890123" {
		t.Fatalf("AccountByLogin = %+v, %v", got, err)
	}
	if _, err := s.AccountByLogin(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account err = %v", err)
	}
	if err := s.SetAccountLock(ctx, a.ID, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.SetExtClass(ctx, a.ID, 4); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := s.SetAccountLevel(ctx, a.ID, 2, exp); err != nil {
		t.Fatal(err)
	}
	got, _ = s.AccountByLogin(ctx, "tester")
	if got.Lock != 3 || got.ExtClass != 4 || got.Level != 2 || !got.LevelExpiresAt.Equal(exp) {
		t.Fatalf("account after updates = %+v", got)
	}
	if err := s.ConnectAccount(ctx, a.ID, "Principal", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if err := s.DisconnectAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	// Characters: create in slot order, duplicate names, no template, full slots.
	names := []string{"Alpha", "Beta", "Gamma", "Delta", "Eps"}
	for i, n := range names {
		r, slot, err := s.CreateCharacter(ctx, a.ID, n, byte(16*(i%5)))
		if err != nil || r != CreateOK || slot != i {
			t.Fatalf("create %s = %d slot %d err %v", n, r, slot, err)
		}
	}
	if r, _, _ := s.CreateCharacter(ctx, a.ID, "alpha", 0); r != CreateNameExists {
		t.Fatalf("duplicate name result %d", r)
	}
	if r, _, _ := s.CreateCharacter(ctx, a.ID, "Zeta", 0); r != CreateNoSlot {
		t.Fatalf("full slots result %d", r)
	}
	b, _ := s.CreateAccount(ctx, "other", "x", "")
	if r, _, _ := s.CreateCharacter(ctx, b.ID, "Summ", 0x50); r != CreateError {
		t.Fatalf("missing template result %d", r)
	}

	list, err := s.ListCharacters(ctx, a.ID)
	if err != nil || len(list) != 5 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	if list[1].Name != "Beta" || list[1].Class != 16 || list[1].Level != 1 ||
		len(list[1].Inventory) != SummaryItems*ItemDBSize || list[1].GuildStatus != GuildStatusNone {
		t.Fatalf("summary = %+v", list[1])
	}

	ok, err := s.DeleteCharacter(ctx, a.ID, "gamma")
	if err != nil || !ok {
		t.Fatalf("delete = %v %v", ok, err)
	}
	if ok, _ := s.DeleteCharacter(ctx, b.ID, "Alpha"); ok {
		t.Fatal("deleted a character of another account")
	}
	if r, slot, _ := s.CreateCharacter(ctx, a.ID, "Gamma2", 32); r != CreateOK || slot != 2 {
		t.Fatalf("reuse slot: %d %d", r, slot)
	}

	c, err := s.LoadCharacter(ctx, a.ID, "beta")
	if err != nil {
		t.Fatal(err)
	}
	if c.Strength != 28 || c.Map != 0 || c.X != 182 || c.Y != 128 || c.PKLevel != 3 || !c.IsNew {
		t.Fatalf("loaded = %+v", c)
	}
	c.Level, c.Experience, c.Money, c.Map, c.X, c.Y = 50, 123456, 999, 2, 10, 20
	c.Inventory[0] = 0x05
	c.IsNew = false
	if err := s.SaveCharacter(ctx, c); err != nil {
		t.Fatal(err)
	}
	c2, _ := s.LoadCharacter(ctx, a.ID, "Beta")
	if c2.Level != 50 || c2.Experience != 123456 || c2.Money != 999 || c2.Map != 2 ||
		c2.Inventory[0] != 0x05 || c2.IsNew {
		t.Fatalf("after save = %+v", c2)
	}
	inv := EmptyItems(InventorySize / ItemDBSize)
	if err := s.SaveInventory(ctx, c.ID, inv); err != nil {
		t.Fatal(err)
	}
	c3, _ := s.LoadCharacter(ctx, a.ID, "Beta")
	if !bytes.Equal(c3.Inventory, inv) {
		t.Fatal("SaveInventory not persisted")
	}
	if ok, _ := s.CharacterExists(ctx, "BETA"); !ok {
		t.Fatal("CharacterExists")
	}

	w, err := s.LoadWarehouse(ctx, a.ID, 0)
	if err != nil || len(w.Items) != WarehouseSize || w.Items[0] != 0xFF {
		t.Fatalf("warehouse = %v", err)
	}
	w.Money, w.Password = 500, 1234
	w.Items[0] = 1
	if err := s.SaveWarehouse(ctx, w); err != nil {
		t.Fatal(err)
	}
	w2, _ := s.LoadWarehouse(ctx, a.ID, 0)
	if w2.Money != 500 || w2.Password != 1234 || w2.Items[0] != 1 {
		t.Fatalf("warehouse after save = %+v", w2)
	}

	if _, err := s.LoadOptions(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("options err = %v", err)
	}
	o := &Options{SkillKeys: make([]byte, SkillKeysSize), GameOption: 1, QKey: 2, WKey: 3, EKey: 4, ChatWindow: 5}
	if err := s.SaveOptions(ctx, c.ID, o); err != nil {
		t.Fatal(err)
	}
	o2, err := s.LoadOptions(ctx, c.ID)
	if err != nil || o2.EKey != 4 || o2.ChatWindow != 5 {
		t.Fatalf("options = %+v %v", o2, err)
	}

	s1, _ := s.NextItemSerial(ctx)
	s2, _ := s.NextItemSerial(ctx)
	if s2 != s1+1 {
		t.Fatalf("serials %d %d", s1, s2)
	}
}

func TestMemoryConformance(t *testing.T) {
	m, err := NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	conformance(t, m)
}

// TestPostgresConformance needs MU_TEST_DATABASE_URL pointing at a server
// where the user may create databases, e.g.
// postgres://mu:mu@localhost:55432/mu?sslmode=disable
func TestPostgresConformance(t *testing.T) {
	base := os.Getenv("MU_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("mu_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")

	u, _ := url.Parse(base)
	u.Path = "/" + name
	p, err := OpenPostgres(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	conformance(t, p)

	// Migrations are idempotent.
	if err := p.migrate(ctx); err != nil {
		t.Fatal(err)
	}
}
