package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestPeriodicPacketLayouts(t *testing.T) {
	if p := packetPeriodicItem(0xFFFF, 13, 0x01020304); !bytes.Equal(p, []byte{0xC1, 0x0C, 0xD5, 0x01, 0xFF, 0xFF, 13, 0, 4, 3, 2, 1}) {
		t.Fatalf("D5 01 % X", p)
	}
	if p := packetPeriodicDelete(20); !bytes.Equal(p, []byte{0xC1, 0x05, 0xD5, 0x02, 20}) {
		t.Fatalf("D5 02 % X", p)
	}
}

func periodicItem(serial uint32) item.Item {
	it := mkItem(14, 13, 0, 1) // Jewel of Bless
	it.Serial, it.Flag1 = serial, true
	return it
}

// A new item with a duration (FUN_004753e0 param_17) is active at once and
// announced with D5 01 FFFF.
func TestPeriodicCreatedItem(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Unix(1_800_000_000, 0)
	o := b.player(PlayerStart, "Timed", 0, 130, 130)
	b.drain(o)
	if !b.createItem(o, 0, 130, 130, dropSpec{Index: item.Make(14, 13), Duration: 600, ToInventory: true}) {
		t.Fatal("create")
	}
	b.runAsync()
	slot := -1
	for s := BagStart; s < BagEnd; s++ {
		if !o.P.Inventory[s].IsEmpty() {
			slot = s
		}
	}
	if slot < 0 {
		t.Fatal("not in inventory")
	}
	it := o.P.Inventory[slot]
	if !it.Flag1 || !it.PeriodicActive || it.Expire != 1_800_000_600 {
		t.Fatalf("item %+v", it)
	}
	p := find(b.drain(o), 0xD5, 0x01)
	if p == nil || p[4] != 0xFF || p[6] != byte(slot) || u32(p[8:]) != 1_800_000_600 {
		t.Fatalf("D5 01 % X", p)
	}
}

func u32(p []byte) uint32 {
	return uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
}

// FUN_004d36e0: an active item past its time is deleted with 28 and D5 02;
// inactive and unexpired items stay.
func TestPeriodicExpiry(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Unix(1_800_000_000, 0)
	o := b.player(PlayerStart, "Expire", 0, 130, 130)
	gone, kept, idle := periodicItem(1), periodicItem(2), periodicItem(3)
	gone.PeriodicActive, gone.Expire = true, 1_800_000_000
	kept.PeriodicActive, kept.Expire = true, 1_800_000_001
	o.P.Inventory[BagStart] = gone
	o.P.Inventory[BagStart+1] = kept
	o.P.Inventory[BagStart+2] = idle
	b.drain(o)
	b.periodicSecond()
	pk := b.drain(o)
	if !o.P.Inventory[BagStart].IsEmpty() || o.P.Inventory[BagStart+1].IsEmpty() || o.P.Inventory[BagStart+2].IsEmpty() {
		t.Fatal("wrong items deleted")
	}
	if d := find(pk, 0x28, -1); d == nil || d[3] != BagStart || d[4] != 1 {
		t.Fatalf("28 % X", d)
	}
	if d := find(pk, 0xD5, 0x02); d == nil || d[4] != BagStart {
		t.Fatalf("D5 02 % X", d)
	}
	// A pending logout skips the check.
	o.P.LogoutCountdown = 3
	b.advance(time.Second)
	b.periodicSecond()
	if o.P.Inventory[BagStart+1].IsEmpty() {
		t.Fatal("deleted during logout")
	}
}

// FUN_004d3a70 / FUN_004d38e0 on login and FUN_004d3c00 on save: the time
// round-trips through PeriodicItem; an unknown serial gets 0 and expires.
func TestPeriodicLoadAndSave(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Unix(1_800_000_000, 0)
	ps := b.store.(store.PeriodicStore)
	ps.SavePeriodic(context.Background(), map[uint32]int64{70: 1_800_000_500})
	o := b.player(PlayerStart, "Login", 0, 130, 130)
	o.P.Inventory[BagStart] = periodicItem(70)
	o.P.Inventory[BagStart+1] = periodicItem(71)
	b.drain(o)
	b.periodicLoad(o)
	b.runAsync()
	a, c := o.P.Inventory[BagStart], o.P.Inventory[BagStart+1]
	if !a.PeriodicActive || a.Expire != 1_800_000_500 || !c.PeriodicActive || c.Expire != 0 {
		t.Fatalf("loaded %+v %+v", a, c)
	}
	n := 0
	for _, p := range b.drain(o) {
		if len(p) > 3 && p[2] == 0xD5 && p[3] == 1 {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d D5 01", n)
	}
	o.P.Inventory[BagStart].Expire = 1_800_000_900
	b.periodicSave(o)
	b.runAsync()
	got, _ := ps.PeriodicTimes(context.Background(), []uint32{70, 71})
	if got[70] != 1_800_000_900 || got[71] != 0 {
		t.Fatalf("saved %v", got)
	}
	b.periodicSecond()
	if !o.P.Inventory[BagStart+1].IsEmpty() || o.P.Inventory[BagStart].IsEmpty() {
		t.Fatal("time 0 item not expired")
	}
}

// FUN_00499410: a periodic item moved inside the inventory reports the
// source and target slots.
func TestPeriodicMove(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Mover", 0, 130, 130)
	it := periodicItem(9)
	it.PeriodicActive, it.Expire = true, 1_900_000_000
	o.P.Inventory[BagStart] = it
	b.drain(o)
	b.handleItemMove(o, move24(0, BagStart, 0, BagStart+5))
	p := find(b.drain(o), 0xD5, 0x01)
	if p == nil || p[4] != BagStart || p[6] != BagStart+5 || u32(p[8:]) != 1_900_000_000 {
		t.Fatalf("D5 01 % X", p)
	}
}
