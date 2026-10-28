package game

import (
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// onWorld runs f on the world goroutine and waits for it.
func onWorld(e *testEnv, f func(w *World)) {
	done := make(chan struct{})
	e.world.post(func() { f(e.world); close(done) })
	<-done
}

// TestItemsEndToEnd speaks the original client protocol: an item dropped
// with hooks.dropMapItem appears in the viewport (C2 20), is picked up
// (C3 22 -> C3 10 22 slot item), moved inside the bag (C3 24) and dropped
// again (C3 23 -> C1 05 23 01 slot).
func TestItemsEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc.ID, "Picker", 0x10)

	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login = %d", r)
	}
	name := make([]byte, 10)
	copy(name, "Picker")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	info := c.next(0xF3, 0x03)
	x, y, mp := int(info[4]), int(info[5]), int(info[6])

	var idx int
	onWorld(env, func(w *World) {
		for i := PlayerStart; i < MaxObjects; i++ {
			if o := w.objects[i]; o.IsPlayer() && o.P.Char != nil && o.P.Char.Name == "Picker" {
				idx = i
			}
		}
		// A 1x1 Small Healing Potion (droppable per ItemMove.txt).
		it := item.Item{Index: item.Make(14, 1), Durability: 1}
		for dx := -1; dx <= 1; dx++ {
			if w.hooks.dropMapItem(w, mp, x+dx, y, it, idx) {
				return
			}
		}
		t.Error("could not drop an item next to the player")
	})
	add := c.next(0x20, -1)
	if add[4] != 1 || add[9] != 1 || add[14]&0xF0 != 0xE0 {
		t.Fatalf("C2 20 = % X", add)
	}
	num := int(add[5]&0x7F)<<8 | int(add[6])

	c.send(frame.Build(frame.C3, 0x22, byte(num>>8), byte(num)))
	pick := c.next(0x22, -1)
	if !c.lastEncrypted || len(pick) != 16 || pick[3] < BagStart || pick[4] != 1 {
		t.Fatalf("pick up = % X", pick)
	}
	slot := pick[3]
	if gone := c.next(0x21, -1); int(gone[5])<<8|int(gone[6]) != num {
		t.Fatalf("C2 21 = % X", gone)
	}

	// Move it to the last bag cell.
	body := append([]byte{0, slot}, make([]byte, 7)...)
	body = append(body, 0, BagEnd-1)
	c.send(frame.Build(frame.C3, 0x24, body...))
	mv := c.next(0x24, -1)
	if mv[3] != 0 || mv[4] != BagEnd-1 || mv[5] != 1 {
		t.Fatalf("move = % X", mv)
	}
	onWorld(env, func(w *World) {
		o := w.objects[idx]
		if !o.P.Inventory[slot].IsEmpty() || o.P.Inventory[BagEnd-1].Index != item.Make(14, 1) {
			t.Error("inventory not updated by the move")
		}
	})

	// Drop it back on our own cell.
	c.send(frame.Build(frame.C3, 0x23, byte(x), byte(y), BagEnd-1))
	if d := c.next(0x23, -1); d[3] != 1 || d[4] != BagEnd-1 {
		t.Fatalf("drop = % X", d)
	}
	if again := c.next(0x20, -1); again[9] != 1 {
		t.Fatalf("dropped item not shown: % X", again)
	}
}

// TestShopEndToEnd talks to Potion Girl Amy (a fake NPC object at her
// ShopManager position), buys the first item and closes the window.
func TestShopEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc.ID, "Shopper", 0x10)
	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login = %d", r)
	}
	name := make([]byte, 10)
	copy(name, "Shopper")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	c.next(0xF3, 0x03)
	const npcIndex = 7
	onWorld(env, func(w *World) {
		var o *Object
		for i := PlayerStart; i < MaxObjects; i++ {
			if x := w.objects[i]; x.IsPlayer() && x.P.Char != nil && x.P.Char.Name == "Shopper" {
				o = x
			}
		}
		w.occupy(o, false)
		o.Map, o.X, o.Y, o.TX, o.TY = 0, 127, 88, 127, 88
		o.P.Char.Money = 5000
		n := w.objects[npcIndex]
		n.reset()
		n.State, n.Type, n.Class = StatePlaying, TypeNPC, 253
		n.Map, n.X, n.Y = 0, 127, 86
		n.Live, n.Alive = LiveNormal, true
	})
	c.send(frame.Build(frame.C3, 0x30, 0, npcIndex))
	if win := c.next(0x30, 0); len(win) != 0x0B {
		t.Fatalf("30 00 = % X", win)
	}
	if list := c.next(0x31, -1); list[5] == 0 {
		t.Fatalf("empty shop list % X", list)
	}
	c.send(frame.Build(frame.C3, 0x32, 0))
	if m := c.next(0x22, 0xFE); int(m[4])<<24|int(m[5])<<16|int(m[6])<<8|int(m[7]) != 4000 {
		t.Fatalf("money = % X", m)
	}
	if buy := c.next(0x32, -1); buy[3] < BagStart || buy[3] >= BagEnd || buy[6] != 50 {
		t.Fatalf("buy = % X", buy)
	}
	c.send(frame.Build(frame.C1, 0x31))
	c.send(frame.Build(frame.C3, 0x32, 0))
	if buy := c.next(0x32, -1); buy[3] != 0xFF {
		t.Fatalf("buy after close = % X", buy)
	}
}
