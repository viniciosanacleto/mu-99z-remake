package game

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// TestChaosAndPShopEndToEnd speaks the original client protocol: talk to
// the Chaos Goblin (C3 30 -> C3 0B 30 03, C1 06 B2 1A), put two jewels in
// the box (C3 24 flag 3), ask the Devil Square rate (C1 88), mix with the
// wrong items (C1 86 02 -> 86 07), close the box (C1 87 -> C1 03 87), then
// price an item for the personal shop (C3 3F 01).
func TestChaosAndPShopEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc.ID, "Alchemist", 0x10)
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login = %d", r)
	}
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name10("Alchemist")...)...))
	c.next(0xF3, 0x03)
	const npcIndex = 9
	onWorld(env, func(w *World) {
		var o *Object
		for i := PlayerStart; i < MaxObjects; i++ {
			if x := w.objects[i]; x.IsPlayer() && x.P.Char != nil && x.P.Char.Name == "Alchemist" {
				o = x
			}
		}
		w.occupy(o, false)
		o.Map, o.X, o.Y, o.TX, o.TY = 0, 130, 130, 130, 130
		o.P.Char.Level = 50
		o.Live = LiveNormal
		o.P.Inventory[BagStart] = item.Item{Index: idxChaos, Durability: 1}
		o.P.Inventory[BagStart+1] = item.Item{Index: idxDSEye, Level: 1, Durability: 1}
		o.P.Inventory[BagStart+2] = item.Item{Index: idxBless, Durability: 1}
		n := w.objects[npcIndex]
		n.reset()
		n.State, n.Type, n.Class = StatePlaying, TypeNPC, chaosGoblin
		n.Map, n.X, n.Y = 0, 131, 130
		n.Live, n.Alive = LiveNormal, true
	})
	c.send(frame.Build(frame.C3, 0x30, 0, npcIndex))
	if win := c.next(0x30, 3); len(win) != 0x0B || !c.lastEncrypted {
		t.Fatalf("30 03 = % X", win)
	}
	if tax := c.next(0xB2, 0x1A); tax[4] != 1 {
		t.Fatalf("B2 1A = % X", tax)
	}
	for i, slot := range []byte{BagStart, BagStart + 1} {
		c.send(frame.Build(frame.C3, 0x24, append([]byte{0, slot}, append(make([]byte, 7), 3, byte(i))...)...))
		if m := c.next(0x24, -1); m[3] != 3 || m[4] != byte(i) {
			t.Fatalf("24 to the box = % X", m)
		}
	}
	// Devil Square preview without a key: no answer; the mix says 7.
	c.send(frame.Build(frame.C1, 0x88, 0, 2, 0, 0, 0))
	c.send(frame.Build(frame.C1, 0x86, 2))
	if r := c.next(0x86, -1); r[3] != mixWrongItems {
		t.Fatalf("86 = % X", r)
	}
	// The Dinorant preview with the same box: no answer either; type 0x31
	// has no preview branch and answers zeros.
	c.send(frame.Build(frame.C1, 0x88, 0, 0x31, 0, 0, 0))
	if r := c.next(0x88, -1); binary.LittleEndian.Uint32(r[4:]) != 0 || binary.LittleEndian.Uint32(r[8:]) != 0 {
		t.Fatalf("88 = % X", r)
	}
	c.send(frame.Build(frame.C1, 0x87))
	if r := c.next(0x87, -1); len(r) != 3 {
		t.Fatalf("87 = % X", r)
	}
	// Personal shop price on the remaining bless (level 50 > 5).
	c.send(frame.Build(frame.C3, 0x3F, 1, BagStart+2, 0x10, 0x27, 0, 0))
	if r := c.next(0x3F, 1); r[4] != 1 || r[5] != BagStart+2 {
		t.Fatalf("3F 01 = % X", r)
	}
	onWorld(env, func(w *World) {
		for i := PlayerStart; i < MaxObjects; i++ {
			if x := w.objects[i]; x.IsPlayer() && x.P.Char != nil && x.P.Char.Name == "Alchemist" {
				if x.P.Inventory[BagStart+2].PShopPrice != 10000 {
					t.Errorf("price = %d", x.P.Inventory[BagStart+2].PShopPrice)
				}
			}
		}
	})
}
