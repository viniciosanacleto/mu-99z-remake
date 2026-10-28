package game

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// live runs a Hub call that posts to the world while the test plays the
// world goroutine.
func (b *bareWorld) live(f func() error) error {
	done := make(chan error, 1)
	go func() { done <- f() }()
	for {
		select {
		case ev := <-b.events:
			ev()
		case err := <-done:
			return err
		}
	}
}

// onlinePlayer is a playing character registered in the hub.
func (b *bareWorld) onlinePlayer(index int, name string) *Object {
	o := b.player(index, name, 0, 130, 130)
	b.hub.Enter(OnlineChar{Name: name, Account: o.P.Account.Login, ServerCode: b.cfg.ServerCode, Index: index})
	return o
}

func (b *bareWorld) edit(name string, f func(*LiveEdit) error) (bool, error) {
	var online bool
	err := b.live(func() error {
		var err error
		online, err = b.hub.EditCharacter(context.Background(), name, f)
		return err
	})
	return online, err
}

func hasPacket(pkts [][]byte, want []byte) bool {
	for _, p := range pkts {
		if bytes.Equal(p, want) {
			return true
		}
	}
	return false
}

func testItem(index int) item.Item {
	it := item.Empty()
	it.Index, it.Durability = index, 20
	return it
}

func TestLiveEditStatsAndZen(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	online, err := b.edit("ann", func(e *LiveEdit) error {
		if e.Char.Name != "Ann" || e.Char.Map != 0 || e.Char.X != 130 {
			t.Fatalf("snapshot %+v", e.Char)
		}
		e.Char.Level, e.Char.Experience = 10, int64(b.data.Experience.Floor(10))
		e.Char.Strength, e.Char.LevelUpPoints, e.Char.Resets = 50, 7, 3
		e.Char.Money = 5
		e.Char.Name = "Changed" // not an editable field
		return nil
	})
	if err != nil || !online {
		t.Fatalf("online=%v err=%v", online, err)
	}
	c := o.P.Char
	if c.Name != "Ann" || c.Level != 10 || c.Strength != 50 || c.LevelUpPoints != 7 || c.Resets != 3 || c.Money != 5 {
		t.Fatalf("char %+v", c)
	}
	if o.P.NextExp != b.data.Experience.Next(10) {
		t.Fatalf("NextExp %d", o.P.NextExp)
	}
	pkts := b.drain(o)
	if p := find(pkts, 0xF3, 0xE0); !bytes.Equal(p, b.packetCharRefresh(o)) {
		t.Fatalf("refresh % X", p)
	}
	if !hasPacket(pkts, packetMoney(5)) {
		t.Fatal("zen not sent")
	}
}

func TestLiveEditNoChangeSendsNothing(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	if _, err := b.edit("Ann", func(*LiveEdit) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if pkts := b.drain(o); len(pkts) != 0 {
		t.Fatalf("sent %d packets", len(pkts))
	}
}

func TestLiveEditCallbackErrorAppliesNothing(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	boom := errors.New("boom")
	_, err := b.edit("Ann", func(e *LiveEdit) error {
		e.Char.Money = 1
		return boom
	})
	if !errors.Is(err, boom) || o.P.Char.Money != 1000000 {
		t.Fatalf("err=%v money=%d", err, o.P.Char.Money)
	}
}

func TestLiveEditInventory(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	viewer := b.player(PlayerStart+1, "Bob", 0, 131, 130)
	b.see(o, viewer)
	sword := testItem(0)
	if _, err := b.edit("Ann", func(e *LiveEdit) error {
		sword.EncodeDB(e.Inventory[12*store.ItemDBSize:])
		sword.EncodeDB(e.Inventory[0:])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if o.P.Inventory[12].Index != 0 || o.P.Inventory[0].Index != 0 {
		t.Fatal("items not placed")
	}
	pkts := b.drain(o)
	if !hasPacket(pkts, packetItemModify(12, o.P.Inventory[12])) || !hasPacket(pkts, packetItemModify(0, o.P.Inventory[0])) {
		t.Fatal("F3 14 not sent")
	}
	if !hasPacket(b.drain(viewer), packetEquipChange(o.Index, 0, o.P.Inventory[0])) {
		t.Fatal("equipment not broadcast")
	}

	if _, err := b.edit("Ann", func(e *LiveEdit) error {
		item.Empty().EncodeDB(e.Inventory[12*store.ItemDBSize:])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !o.P.Inventory[12].IsEmpty() || o.P.Inventory[0].IsEmpty() {
		t.Fatal("remove")
	}
	if pkts := b.drain(o); len(pkts) != 1 || !bytes.Equal(pkts[0], packetItemDelete(12, 1)) {
		t.Fatalf("remove packets % X", pkts)
	}
}

func TestLiveEditVault(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	if _, err := b.edit("Ann", func(e *LiveEdit) error {
		if e.VaultLoaded || e.Vault != nil {
			t.Fatal("closed vault in the snapshot")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	st := items(o)
	st.If = ifState{Use: true, State: 1, Type: ifWarehouse}
	st.WarehouseLoaded = true
	if _, err := b.edit("Ann", func(e *LiveEdit) error {
		if !e.VaultLoaded || len(e.Vault) != store.WarehouseSize {
			t.Fatal("open vault missing")
		}
		testItem(0).EncodeDB(e.Vault[3*store.ItemDBSize:])
		e.VaultMoney = 77
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if st.Warehouse[3].Index != 0 || st.WarehouseMoney != 77 {
		t.Fatal("vault not applied")
	}
	pkts := b.drain(o)
	if !hasPacket(pkts, packetItemList(st.Warehouse[:])) || !hasPacket(pkts, packetWarehouseMoney(1, 77, o.P.Char.Money)) {
		t.Fatal("vault not sent")
	}
}

func TestLiveEditOfflineAndLeaving(t *testing.T) {
	b := newBareWorld(t)
	called := false
	online, err := b.hub.EditCharacter(context.Background(), "Nobody", func(*LiveEdit) error { called = true; return nil })
	if online || err != nil || called {
		t.Fatalf("online=%v err=%v called=%v", online, err, called)
	}
	// The hub still lists a session whose object is gone.
	b.hub.Enter(OnlineChar{Name: "Ghost", ServerCode: b.cfg.ServerCode, Index: PlayerStart})
	online, err = b.edit("Ghost", func(*LiveEdit) error { called = true; return nil })
	if !online || !errors.Is(err, ErrCharLeaving) || called {
		t.Fatalf("online=%v err=%v called=%v", online, err, called)
	}
}

func TestAccountCharacter(t *testing.T) {
	b := newBareWorld(t)
	b.onlinePlayer(PlayerStart, "Ann")
	if name, ok := b.hub.AccountCharacter("ACCANN"); !ok || name != "Ann" {
		t.Fatalf("%q %v", name, ok)
	}
	if _, ok := b.hub.AccountCharacter("other"); ok {
		t.Fatal("found other account")
	}
}

func TestLockVault(t *testing.T) {
	b := newBareWorld(t)
	o := b.onlinePlayer(PlayerStart, "Ann")
	lock := func() (bool, func(), error) {
		var loaded bool
		var release func()
		err := b.live(func() error {
			var err error
			loaded, release, err = b.hub.LockVault(context.Background(), "Ann")
			return err
		})
		return loaded, release, err
	}
	loaded, release, err := lock()
	if err != nil || loaded || !items(o).warehouseLoading {
		t.Fatalf("loaded=%v err=%v", loaded, err)
	}
	b.openWarehouse(o)
	if items(o).If.Type == ifWarehouse {
		t.Fatal("vault opened while locked")
	}
	if _, _, err := lock(); !errors.Is(err, ErrVaultBusy) {
		t.Fatalf("second lock err=%v", err)
	}
	b.live(func() error { release(); return nil })
	if items(o).warehouseLoading {
		t.Fatal("not released")
	}
	items(o).WarehouseLoaded = true
	if loaded, _, err := lock(); err != nil || !loaded || items(o).warehouseLoading {
		t.Fatalf("open vault loaded=%v err=%v", loaded, err)
	}
	if _, _, err := b.hub.LockVault(context.Background(), "Nobody"); !errors.Is(err, ErrCharLeaving) {
		t.Fatalf("offline err=%v", err)
	}
}
