package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// pushScene is an attacker at x and a monster at x+2 on an open row of
// Lorencia (cells x..x+8 free).
func pushScene(t *testing.T) (*bareWorld, *Object, *Object, int, int) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Pusher", 0, x, y)
	m := b.monsterNear(0, p, 2)
	b.see(p, m)
	return b, p, m, x, y
}

func TestPushBackDirection(t *testing.T) {
	b, p, m, x, _ := pushScene(t)
	p.Dir = 3 // east (pathDirs[3] = +1, 0)
	fixRand(t, 1)
	if !b.pushBack(m, p) || m.X != x+3 {
		t.Fatalf("push = %d, want %d", m.X, x+3)
	}
	if r := find(b.drain(p), 0xDF, -1); r == nil {
		t.Fatal("no position packet to the viewer")
	}
	if b.terrain(0).Get(x+3, m.Y)&gamedata.AttrCharacter == 0 || b.terrain(0).Get(x+2, m.Y)&gamedata.AttrCharacter != 0 {
		t.Fatal("occupancy not moved")
	}
	// rand % 3 == 0 reverses the direction.
	fixRand(t, 0)
	if !b.pushBack(m, p) || m.X != x+2 {
		t.Fatalf("reversed push = %d", m.X)
	}
	// A blocked cell stops it.
	tr := b.terrain(0)
	tr.Set(x+3, m.Y, tr.Get(x+3, m.Y)|gamedata.AttrBlock)
	fixRand(t, 1)
	if b.pushBack(m, p) || m.X != x+2 {
		t.Fatal("pushed into a wall")
	}
	// Castle/event classes never move.
	m.Class = 0x83
	if b.pushBack(m, p) {
		t.Fatal("pushed a class of the list")
	}
}

func TestPushBackMobComboAndCrown(t *testing.T) {
	b, p, m, _, _ := pushScene(t)
	if l := b.q().noPush; len(l) != 1 || l[0] != 500 {
		t.Fatalf("CustomMobCombo = %v", l)
	}
	m.Class = 500
	if b.pushAllowed(m, false) {
		t.Fatal("CustomMobCombo class pushed")
	}
	// A player on a teleport or with authority 8 is not pushed.
	v := b.player(PlayerStart+1, "Victim", 0, p.X+1, p.Y)
	v.Teleporting = true
	if b.pushAllowed(v, false) {
		t.Fatal("teleporting player pushed")
	}
	v.Teleporting = false
	mv(v).Authority = 8
	if b.pushAllowed(v, false) {
		t.Fatal("authority 8 pushed")
	}
	mv(v).Authority = 0
	if !b.pushAllowed(v, false) {
		t.Fatal("player not pushable")
	}
}

func TestKnockBack(t *testing.T) {
	b, p, m, x, _ := pushScene(t)
	// Three steps: the third only moves classes 1, 3 and 4.
	if !b.knockBack(m, p, 3) || m.X != x+4 {
		t.Fatalf("knock = %d, want %d", m.X, x+4)
	}
	m.Class = 1
	b.knockBack(m, p, 3)
	if m.X != x+7 {
		t.Fatalf("class 1 knock = %d, want %d", m.X, x+7)
	}
	// Mercenaries are spared by the knock-back only.
	m.Class = 0x11E
	if b.knockBack(m, p, 2) || !b.pushAllowed(m, false) {
		t.Fatal("mercenary rule")
	}
}

func TestSkillPushBackMessages(t *testing.T) {
	b, p, m, x, y := pushScene(t)
	p.Dir = 3
	// Monster: message 2 after 150 ms (FUN_00510550), then FUN_005110e0.
	b.skillHitEffect(p, m, 19, 10)
	found := false
	for _, msg := range m.Mon.Msgs {
		if msg.Used && msg.Code == msgPushBack && msg.Sender == p.Index {
			found = true
		}
	}
	if !found {
		t.Fatal("no push message")
	}
	fixRand(t, 1)
	b.advance(151 * time.Millisecond)
	b.monsterMessages(m)
	if m.X != x+3 {
		t.Fatalf("monster push = %d", m.X)
	}
	// Player: the push queue of FUN_004c4120 case 2.
	v := b.player(PlayerStart+1, "Victim", 0, x+5, y)
	b.queuePushBack(v, p, 150)
	b.pushTick()
	if v.X != x+5 {
		t.Fatal("pushed before the delay")
	}
	b.advance(151 * time.Millisecond)
	b.pushTick()
	if v.X != x+6 || len(b.q().pushes) != 0 {
		t.Fatalf("player push = %d", v.X)
	}
	// A dead sender pushes nothing.
	b.queuePushBack(v, p, 0)
	p.Alive = false
	b.advance(time.Millisecond)
	b.pushTick()
	if v.X != x+6 {
		t.Fatal("dead sender pushed")
	}
}
