package game

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func questPlayer(b *bareWorld, index, class, level int) *Object {
	o := statsPlayer(b, index, class, level)
	o.P.Char.Quest = bytes.Repeat([]byte{0xFF}, 50)
	o.P.Char.Money = 0
	b.drain(o)
	return o
}

func TestQuestPacketLayouts(t *testing.T) {
	st := bytes.Repeat([]byte{0xFF}, 50)
	st[0] = 0xFD
	if p := packetQuestList(4, st); len(p) != 0x36 || !bytes.Equal(p[:5], []byte{0xC1, 0x36, 0xA0, 4, 0xFD}) || p[53] != 0xFF {
		t.Fatalf("A0 = % X", p)
	}
	if p := packetQuestInfo(2, 0xEF); !bytes.Equal(p, []byte{0xC1, 0x05, 0xA1, 2, 0xEF}) {
		t.Fatalf("A1 = % X", p)
	}
	if p := packetQuestResult(1, 0xFF, 0xF7); !bytes.Equal(p, []byte{0xC1, 0x06, 0xA2, 1, 0xFF, 0xF7}) {
		t.Fatalf("A2 = % X", p)
	}
	if p := packetQuestReward(0x5400, 200, 10, 0x01020304); !bytes.Equal(p,
		[]byte{0xC1, 0x0C, 0xA3, 0x54, 0x00, 200, 10, 0, 4, 3, 2, 1}) {
		t.Fatalf("A3 = % X", p)
	}
}

func TestQuestStateBits(t *testing.T) {
	o := &Object{P: &Player{}}
	o.P.Char = &store.Character{}
	o.P.Char.Quest = bytes.Repeat([]byte{0xFF}, 50)
	if questState(o, 5) != 3 || questState(o, 250) != 0 {
		t.Fatal("initial state")
	}
	setQuestState(o, 5, 1)
	if questState(o, 5) != 1 || o.P.Char.Quest[1] != 0xF7 || questState(o, 4) != 3 {
		t.Fatalf("set: % X", o.P.Char.Quest[:2])
	}
	if setQuestState(o, 200, 1) {
		t.Fatal("index 200 accepted")
	}
}

func TestEvolutionByte(t *testing.T) {
	o := &Object{P: &Player{}}
	o.P.Char = &store.Character{}
	for _, c := range []struct {
		class int
		db    byte
		want  byte
	}{{0, 0x01, 0x10}, {1, 0x11, 0x30}, {2, 0x22, 0x5F}, {1, 0x10, 0x20}} {
		o.Class, o.P.Char.Class = c.class, c.db
		if v := evolutionByte(o); v != c.want {
			t.Fatalf("class %d/%02X: %02X want %02X", c.class, c.db, v, c.want)
		}
	}
}

func TestQuestFlowScrollOfEmperor(t *testing.T) {
	b := newBareWorld(t)
	o := questPlayer(b, PlayerStart, 1, 160) // DK 160
	npc := fakeNPC(b, 100, 235, 0, o.X+1, o.Y)

	// Talking to the NPC opens quest 0 (state 3) and sends A0 once.
	if !b.npcTalk(o, npc) {
		t.Fatal("quest NPC talk not handled")
	}
	pk := b.drain(o)
	if find(pk, 0xA0, -1) == nil || !bytes.Equal(find(pk, 0xA1, -1), []byte{0xC1, 0x05, 0xA1, 0, 0xFF}) {
		t.Fatalf("talk packets % X", pk)
	}
	if st := items(o).If; !st.Use || st.Type != ifQuest {
		t.Fatalf("interface %+v", st)
	}
	accept := frame.Build(frame.C3, 0xA2, 0)
	// Accepting needs 1,000,000 zen (objective type 2, RequireState 3).
	b.handleQuestState(o, accept)
	if p := find(b.drain(o), 0xA2, -1); !bytes.Equal(p, []byte{0xC1, 0x06, 0xA2, 0, 0xFF, 0xFF}) {
		t.Fatalf("refused = % X", p)
	}
	o.P.Char.Money = 1500000
	b.handleQuestState(o, accept)
	pk = b.drain(o)
	if p := find(pk, 0xA2, -1); !bytes.Equal(p, []byte{0xC1, 0x06, 0xA2, 0, 0, 0xFD}) || o.P.Char.Money != 500000 {
		t.Fatalf("accepted = % X money %d", p, o.P.Char.Money)
	}
	if find(pk, 0x22, 0xFE) == nil {
		t.Fatal("no money update")
	}
	// State 1: the Scroll of Emperor (14,23) is required.
	b.handleQuestState(o, accept)
	if p := find(b.drain(o), 0xA2, -1); p[4] != 0xFF {
		t.Fatalf("finished without item: % X", p)
	}
	scroll := item.Item{Index: 7191, Durability: 1}
	slot := b.insertInventory(o, scroll)
	points := o.P.Char.LevelUpPoints
	b.handleQuestState(o, accept)
	pk = b.drain(o)
	if p := find(pk, 0xA2, -1); !bytes.Equal(p, []byte{0xC1, 0x06, 0xA2, 0, 0, 0xFE}) {
		t.Fatalf("completed = % X", p)
	}
	if !o.P.Inventory[slot].IsEmpty() || !bytes.Equal(find(pk, 0x28, -1), []byte{0xC1, 0x05, 0x28, byte(slot), 1}) {
		t.Fatal("scroll not consumed")
	}
	r := find(pk, 0xA3, -1)
	if o.P.Char.LevelUpPoints != points+10 || r == nil || r[5] != 200 || r[6] != 10 ||
		int(r[8])|int(r[9])<<8 != points+10 {
		t.Fatalf("reward % X points %d", r, o.P.Char.LevelUpPoints)
	}
	// Completed: the NPC now offers quest 1 (requires quest 0 state 2).
	b.npcTalk(o, npc)
	if p := find(b.drain(o), 0xA1, -1); p == nil || p[3] != 1 {
		t.Fatalf("next quest % X", p)
	}
}

func TestQuestEvolutionAndHero(t *testing.T) {
	b := newBareWorld(t)
	o := questPlayer(b, PlayerStart, 2, 230) // Elf 230
	setQuestState(o, 0, 2)
	setQuestState(o, 1, 1)
	b.insertInventory(o, item.Item{Index: 7193, Durability: 1}) // Tear of Elf (Elf only)
	b.handleQuestState(o, frame.Build(frame.C3, 0xA2, 1))
	pk := b.drain(o)
	if changeUp(o) != 1 || questState(o, 1) != 2 {
		t.Fatalf("change up %d state %d", changeUp(o), questState(o, 1))
	}
	var evo []byte
	for _, p := range pk {
		if frame.Head(p) == 0xA3 && p[5] == 201 {
			evo = p
		}
	}
	if evo == nil || evo[6] != 0x50 {
		t.Fatalf("evolution reward % X", evo)
	}
	// Quest 2 (state 1 -> 2) gives Hero Status: skill 0x4D for the Elf and
	// PlusStatPoint * (230 - PlusStatMinLevel 220) points.
	setQuestState(o, 2, 1)
	b.insertInventory(o, item.Item{Index: 7191, Level: 1, Durability: 1}) // Ring of Honor
	before := o.P.Char.LevelUpPoints
	b.handleQuestState(o, frame.Build(frame.C3, 0xA2, 2))
	pk = b.drain(o)
	if o.P.Char.LevelUpPoints != before+10 || questState(o, 2) != 2 {
		t.Fatalf("hero points %d -> %d", before, o.P.Char.LevelUpPoints)
	}
	learned := false
	for _, s := range o.P.Skills {
		learned = learned || s.Index == 0x4D
	}
	if !learned || find(pk, 0xF3, 0x11) == nil {
		t.Fatal("Elf did not learn skill 0x4D")
	}
}

func TestQuestItemDrop(t *testing.T) {
	b := newBareWorld(t)
	o := questPlayer(b, PlayerStart, 1, 160)
	setQuestState(o, 0, 1) // Scroll of Emperor: monsters level 45..60, 10/10000
	m := b.monsterNear(0, o, 1)
	m.Mon.Level = 50
	m.Mon.Hits[0] = damageEntry{Index: o.Index, Damage: 10, Time: b.now()}
	var dropped item.Item
	old := dropMapItemHook
	dropMapItemHook = func(w *World, mp, x, y int, it item.Item, owner int) bool { dropped = it; return owner == o.Index }
	t.Cleanup(func() { dropMapItemHook = old })
	fixRand(t, 10)
	if b.questItemDrop(m) {
		t.Fatal("dropped with rand 10 >= rate 10")
	}
	fixRand(t, 9)
	if !b.questItemDrop(m) || dropped.Index != 7191 {
		t.Fatalf("no drop: %+v", dropped)
	}
	m.Mon.Level = 61
	if b.questItemDrop(m) {
		t.Fatal("dropped outside the level range")
	}
	m.Mon.Level = 50
	b.insertInventory(o, item.Item{Index: 7191, Durability: 1})
	if b.questItemDrop(m) {
		t.Fatal("dropped while the player already has the quantity")
	}
}

func TestQuestListSentOncePerEntry(t *testing.T) {
	b := newBareWorld(t)
	o := questPlayer(b, PlayerStart, 0, 10)
	b.handleQuestList(o, nil)
	b.handleQuestList(o, nil)
	n := 0
	for _, p := range b.drain(o) {
		if frame.Head(p) == 0xA0 {
			n++
			if p[3] != 4 {
				t.Fatalf("count %d", p[3])
			}
		}
	}
	if n != 1 {
		t.Fatalf("A0 sent %d times", n)
	}
}
