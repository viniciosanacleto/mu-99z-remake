package game

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// runAsync drains the async work and its continuations of a bare world.
func (b *bareWorld) runAsync() {
	b.wg.Wait()
	for len(b.events) > 0 {
		(<-b.events)()
		b.wg.Wait()
	}
}

func TestGoldenArcherFlow(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Archer", 0, 130, 130)
	npc := fakeNPC(b, 101, gaNPC, 0, 131, 130)
	if !b.hooks.npcTalk[gaNPC](b.World, o, npc) {
		t.Fatal("talk not handled")
	}
	b.runAsync()
	if p := find(b.drain(o), 0x94, -1); !bytes.Equal(p, packetRenaCount(0)) {
		t.Fatalf("count = % X", p)
	}
	if st := items(o).If; st.Type != ifArcher || !st.Use {
		t.Fatalf("interface %+v", st)
	}
	// Ten Renas handed one by one.
	for i := 0; i < 10; i++ {
		b.insertInventory(o, item.Item{Index: itemRena, Durability: 1})
		b.handleGiveRena(o, frame.Build(frame.C1, 0x95))
		b.runAsync()
	}
	pk := b.drain(o)
	var last []byte
	for _, p := range pk {
		if frame.Head(p) == 0x94 {
			last = p
		}
	}
	if !bytes.Equal(last, packetRenaCount(10)) || b.countItems(o, itemRena, 0) != 0 {
		t.Fatalf("after giving: % X", last)
	}
	// Group 1 needs 20: refused, count unchanged.
	b.handleArcherReward(o, frame.Build(frame.C1, 0x96, 1))
	b.runAsync()
	if p := find(b.drain(o), 0xF3, 0x40); p != nil {
		t.Fatal("rewarded without enough Renas")
	}
	// Group 0 (10 Renas): a jewel into the inventory and the firework.
	b.handleArcherReward(o, frame.Build(frame.C1, 0x96, 0))
	b.runAsync()
	pk = b.drain(o)
	if p := find(pk, 0xF3, 0x40); !bytes.Equal(p, []byte{0xC1, 0x07, 0xF3, 0x40, 0, 130, 130}) {
		t.Fatalf("firework % X", p)
	}
	if find(pk, 0xF3, 0x14) == nil {
		t.Fatal("reward not given to the inventory")
	}
	b.handleArcherClose(o, frame.Build(frame.C1, 0x97))
	if items(o).If.Use {
		t.Fatal("interface not closed")
	}
}
