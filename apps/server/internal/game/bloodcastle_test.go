package game

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestBCPacketLayouts(t *testing.T) {
	r := packetBCResult(1, "Hero", bcUser{Score: 4000, Exp: 20000, Money: 0x01020304})
	if len(r) != 0x1D || !bytes.Equal(r[:5], []byte{0xC1, 0x1D, 0x93, 1, 0xFF}) || string(r[5:9]) != "Hero" ||
		r[17] != 0xA0 || r[18] != 0x0F || r[21] != 0x20 || r[22] != 0x4E || !bytes.Equal(r[25:29], []byte{4, 3, 2, 1}) {
		t.Fatalf("93 = % X", r)
	}
	s := packetBCStatus(1, 0x0384, 80, 7, 0x5400, -1)
	if !bytes.Equal(s, []byte{0xC1, 0x0E, 0x9B, 1, 0x84, 0x03, 80, 0, 7, 0, 0x00, 0x54, 0xFF, 0}) {
		t.Fatalf("9B = % X", s)
	}
	if s := packetBCStatus(4, 1, 2, 3, 4, 2); s[12] != 3 {
		t.Fatalf("weapon level = % X", s)
	}
	a := packetMapAttr(8, true, bcBridgeRect)
	if !bytes.Equal(a, []byte{0xC1, 0x0B, 0x46, 0, 8, 1, 1, 13, 70, 15, 75}) {
		t.Fatalf("46 = % X", a)
	}
}

func TestBloodCastleRun(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 22, 1, 54, 0, 0, time.UTC) // next start 02:00
	for i := range b.bc().c {
		b.bc().c[i].State = dsEmpty
		b.bcEnterEmpty(&b.bc().c[i])
	}
	c := &b.bc().c[0]
	tr := b.terrain(c.Map)
	if tr.Get(14, 72)&8 == 0 || tr.Get(14, 20)&4 == 0 {
		t.Fatal("bridge / entrance not blocked while EMPTY")
	}
	o := questPlayer(b, PlayerStart, 1, 50) // DK 50 -> castle 1
	o.Map, o.X, o.Y = 2, 120, 120
	tick := func(d time.Duration) [][]byte {
		b.advance(d)
		b.bcTick()
		return b.drain(o)
	}
	tick(6 * time.Minute)
	if c.State != dsReady || !c.Open {
		t.Fatalf("ready: state %d", c.State)
	}
	slot := b.insertInventory(o, item.Item{Index: itemBCCloak, Level: 1, Durability: 1})
	b.handleBCEnter(o, frame.Build(frame.C3, 0x9A, 1, byte(slot)))
	if p := find(b.drain(o), 0x9A, -1); !bytes.Equal(p, packetBCEnterResult(0)) || o.Map != 11 {
		t.Fatalf("enter = % X map %d", p, o.Map)
	}
	o.X, o.Y = 14, 30
	tick(5 * time.Minute)
	pk := tick(time.Minute)
	if c.State != dsStart || c.Required != 40 || c.Gate < 0 || len(c.Mobs) == 0 || tr.Get(14, 20)&4 != 0 {
		t.Fatalf("start: state %d required %d gate %d mobs %d", c.State, c.Required, c.Gate, len(c.Mobs))
	}
	if p := find(pk, 0x9B, -1); p == nil {
		t.Fatal("no 9B")
	}
	// Step 0 -> 1 -> 2: the bridge opens.
	c.Kills = c.Required
	tick(time.Second)
	if c.Step != 1 || tr.Get(14, 72)&8 != 0 {
		t.Fatalf("step %d", c.Step)
	}
	pk = tick(3 * time.Second)
	if c.Step != 2 || find(pk, 0x46, -1) == nil {
		t.Fatalf("step %d bridge packet % X", c.Step, find(pk, 0x46, -1))
	}
	// Gate broken by the player.
	gate := b.objects[c.Gate]
	gate.Mon.Hits[0] = damageEntry{Index: o.Index, Damage: 1, Time: b.now()}
	b.bcMonsterKilled(gate)
	tick(time.Second)
	if c.Step != 3 || c.Required != 2 || tr.Get(14, 85)&4 != 0 {
		t.Fatalf("step %d required %d", c.Step, c.Required)
	}
	c.Kills = 2
	tick(time.Second)
	if c.Step != 4 || c.Statue < 0 {
		t.Fatalf("step %d statue %d", c.Step, c.Statue)
	}
	statue := b.objects[c.Statue]
	if statue.X != 14 || statue.Y != 95 || statue.Mon.Life != 150000 {
		t.Fatalf("statue at %d,%d life %v", statue.X, statue.Y, statue.Mon.Life)
	}
	var dropped item.Item
	b.hooks.dropMapItem = func(w *World, mp, x, y int, it item.Item, owner int) bool { dropped = it; return true }
	statue.Mon.Hits[0] = damageEntry{Index: o.Index, Damage: 1, Time: b.now()}
	b.bcMonsterKilled(statue)
	(<-b.events)() // the DataServer serial
	if dropped.Index != itemBCWeapon || dropped.Level != statue.Class-bcStatueMin || c.WeaponSerial != int64(dropped.Serial) {
		t.Fatalf("weapon %+v serial %d", dropped, c.WeaponSerial)
	}
	// Pick it up and deliver it to the Archangel.
	b.insertInventory(o, dropped)
	b.bcWeaponPicked(o, dropped)
	if c.Carrier != o.Index {
		t.Fatal("carrier not set")
	}
	b.drain(o)
	b.bcArchangelTalk(o)
	if p := find(b.drain(o), 0xF3, 0x40); !bytes.Equal(p, []byte{0xC1, 0x07, 0xF3, 0x40, 0x01, 0x17, 0x00}) || c.Winner != o.Index {
		t.Fatalf("archangel = % X winner %d", p, c.Winner)
	}
	money := o.P.Char.Money
	pk = tick(time.Second)
	r := find(pk, 0x93, -1)
	if c.State != dsClean || r == nil || r[3] != 1 || o.P.Char.Money != money+20000 {
		t.Fatalf("clean: state %d result % X money %d", c.State, r, o.P.Char.Money)
	}
	// FUN_00476c50: the score goes to RankingBloodCastle.
	if n := b.eventRank(t, store.RankingBloodCastle, o.P.Char.Name); n != int(binary.LittleEndian.Uint32(r[17:])) || n == 0 {
		t.Fatalf("ranking %d result score %d", n, binary.LittleEndian.Uint32(r[17:]))
	}
}
