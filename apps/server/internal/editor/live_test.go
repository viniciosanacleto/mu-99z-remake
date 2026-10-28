package editor

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// fakeLive stands for game.Hub: the callbacks change the LiveEdit of the
// playing characters in place.
type fakeLive struct {
	playing  map[string]*game.LiveEdit // lower(name)
	accounts map[string]string         // login -> playing character
	editErr  error
	lockErr  error
	locked   bool
	released bool
}

func (l *fakeLive) EditCharacter(_ context.Context, name string, f func(*game.LiveEdit) error) (bool, error) {
	le, ok := l.playing[strings.ToLower(name)]
	if !ok {
		return false, nil
	}
	if l.editErr != nil {
		return true, l.editErr
	}
	return true, f(le)
}

func (l *fakeLive) AccountCharacter(login string) (string, bool) {
	name, ok := l.accounts[login]
	return name, ok
}

func (l *fakeLive) LockVault(_ context.Context, name string) (bool, func(), error) {
	if l.lockErr != nil {
		return false, func() {}, l.lockErr
	}
	if l.playing[strings.ToLower(name)].VaultLoaded {
		return true, func() {}, nil
	}
	l.locked = true
	return false, func() { l.released = true }, nil
}

// liveFixture is the fixture with the account online and, when playing is
// set, the character in the game.
func liveFixture(t *testing.T, playing bool) (*fixture, *fakeLive) {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.st.ConnectAccount(ctx, f.acc.ID, "Principal", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	l := &fakeLive{playing: map[string]*game.LiveEdit{}, accounts: map[string]string{}}
	if playing {
		c, err := f.st.LoadCharacter(ctx, f.acc.ID, f.name)
		if err != nil {
			t.Fatal(err)
		}
		le := &game.LiveEdit{Char: *c, Inventory: bytes.Clone(c.Inventory)}
		le.Char.Inventory = nil
		l.playing[strings.ToLower(f.name)] = le
		l.accounts[f.acc.Login] = f.name
	}
	f.ed.WithLive(l)
	return f, l
}

func (l *fakeLive) char(name string) *game.LiveEdit { return l.playing[strings.ToLower(name)] }

func TestLiveEditsPlayingCharacter(t *testing.T) {
	f, l := liveFixture(t, true)
	ctx := context.Background()
	le := l.char(f.name)
	if err := f.ed.UpdateStats(ctx, f.name, Stats{Level: 10, Strength: 50, Dexterity: 1, Vitality: 1, Energy: 1, Money: 9, Resets: 2}); err != nil {
		t.Fatal(err)
	}
	if le.Char.Level != 10 || le.Char.Strength != 50 || le.Char.Money != 9 || le.Char.Resets != 2 ||
		le.Char.Experience != int64(testLimits.Exp.Floor(10)) {
		t.Fatalf("live char %+v", le.Char)
	}
	if c, _ := f.st.LoadCharacter(ctx, f.acc.ID, f.name); c.Level == 10 {
		t.Fatal("playing character written to the store")
	}

	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Inventory, Slot: -1, Index: kris.Index, Level: 2})
	if err != nil || slot < bagStart {
		t.Fatalf("slot %d err %v", slot, err)
	}
	if it := decode(t, le.Inventory, slot); it.Index != kris.Index || it.Level != 2 {
		t.Fatalf("live item %+v", it)
	}
	if _, used := item.DecodeDB(f.inventory(t)[slot*item.DBSize:]); used {
		t.Fatal("item written to the store")
	}

	v, err := f.ed.Character(ctx, f.name)
	if err != nil || !v.Online || !v.Live || v.Level != 10 || v.Vault == nil || !slices.ContainsFunc(v.Inventory, func(iv ItemView) bool { return iv.Slot == slot }) {
		t.Fatalf("view %+v, %v", v, err)
	}

	if err := f.ed.RemoveItem(ctx, f.name, Inventory, slot); err != nil {
		t.Fatal(err)
	}
	if _, used := item.DecodeDB(le.Inventory[slot*item.DBSize:]); used {
		t.Fatal("live item not removed")
	}
}

func TestLiveOnlineAccountCharacterNotPlaying(t *testing.T) {
	f, l := liveFixture(t, false)
	ctx := context.Background()
	if err := f.ed.UpdateStats(ctx, f.name, Stats{Level: 10, Strength: 1, Dexterity: 1, Vitality: 1, Energy: 1}); err != nil {
		t.Fatal(err)
	}
	if c, _ := f.st.LoadCharacter(ctx, f.acc.ID, f.name); c.Level != 10 {
		t.Fatal("store not updated")
	}
	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: kris.Index})
	if err != nil {
		t.Fatal(err)
	}
	decode(t, f.vault(t), slot)
	if l.locked {
		t.Fatal("vault locked without a playing character")
	}
	v, err := f.ed.Character(ctx, f.name)
	if err != nil || !v.Online || !v.Live || len(v.Vault) != 1 {
		t.Fatalf("view %+v, %v", v, err)
	}
}

func TestLiveVaultOpenInGame(t *testing.T) {
	f, l := liveFixture(t, true)
	ctx := context.Background()
	le := l.char(f.name)
	le.VaultLoaded, le.Vault, le.VaultMoney = true, bytes.Repeat([]byte{0xFF}, store.WarehouseSize), 55
	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: kris.Index})
	if err != nil {
		t.Fatal(err)
	}
	decode(t, le.Vault, slot)
	if _, used := item.DecodeDB(f.vault(t)[slot*item.DBSize:]); used {
		t.Fatal("open vault written to the store")
	}
	v, err := f.ed.Character(ctx, f.name)
	if err != nil || len(v.Vault) != 1 || v.VaultMoney != 55 {
		t.Fatalf("view %+v, %v", v, err)
	}
	if err := f.ed.RemoveItem(ctx, f.name, Vault, slot); err != nil {
		t.Fatal(err)
	}
	if _, used := item.DecodeDB(le.Vault[slot*item.DBSize:]); used {
		t.Fatal("vault item not removed")
	}

	le.VaultNumber = 1 // another vault is open: vault 0 is not in memory
	if _, err := f.ed.AddItem(ctx, f.name, NewItem{Container: Vault, Slot: -1, Index: kris.Index}); !errors.Is(err, ErrBusy) {
		t.Fatalf("other vault err = %v", err)
	}
}

func TestLiveVaultLockedWhileWritingStore(t *testing.T) {
	f, l := liveFixture(t, true)
	kris := byName(t, f.cat, 0, "Kris")
	slot, err := f.ed.AddItem(context.Background(), f.name, NewItem{Container: Vault, Slot: -1, Index: kris.Index})
	if err != nil {
		t.Fatal(err)
	}
	decode(t, f.vault(t), slot)
	if !l.locked || !l.released {
		t.Fatalf("locked=%v released=%v", l.locked, l.released)
	}
}

func TestLiveBusy(t *testing.T) {
	f, l := liveFixture(t, true)
	ctx := context.Background()
	l.editErr = game.ErrCharLeaving
	if err := f.ed.UpdateStats(ctx, f.name, Stats{Level: 10, Strength: 1, Dexterity: 1, Vitality: 1, Energy: 1}); !errors.Is(err, ErrBusy) {
		t.Fatalf("leaving err = %v", err)
	}
	if _, err := f.ed.Character(ctx, f.name); !errors.Is(err, ErrBusy) {
		t.Fatalf("view leaving err = %v", err)
	}
	l.editErr, l.lockErr = nil, game.ErrVaultBusy
	if err := f.ed.RemoveItem(ctx, f.name, Vault, 0); !errors.Is(err, ErrBusy) {
		t.Fatalf("vault busy err = %v", err)
	}

	rec := httptest.NewRecorder()
	writeError(rec, nil, ErrBusy)
	if rec.Code != http.StatusConflict {
		t.Fatalf("ErrBusy status %d", rec.Code)
	}
}
