package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// fixRand makes monsterRand return vals in a loop for the test.
func fixRand(t *testing.T, vals ...int) {
	t.Helper()
	old := monsterRand
	i := 0
	monsterRand = func() int {
		v := vals[i%len(vals)]
		i++
		return v
	}
	t.Cleanup(func() { monsterRand = old })
}

func TestMonsterViewportEntryLayout(t *testing.T) {
	o := &Object{Index: 0x1234, Class: 0x0102, X: 10, Y: 20, TX: 11, TY: 21, Dir: 5, PKLevel: 3,
		ViewSkillState: 0xA1B2C3D4, Live: LiveNew, Teleporting: true}
	b := make([]byte, 16)
	monsterViewportEntry(b, o)
	want := []byte{0x12 | 0xC0, 0x34, 0x01, 0x02, 0xD4, 0xC3, 0xB2, 0xA1, 10, 20, 11, 21, 0x53, 0, 0, 0}
	if !bytes.Equal(b, want) {
		t.Fatalf("entry = % X", b)
	}
	o.Live = LiveNormal
	monsterViewportEntry(b, o)
	if b[0] != 0x12 {
		t.Fatalf("not new: % X", b)
	}
	p := packetViewportMonsters([][]byte{b, b})
	if p[0] != 0xC2 || int(p[1])<<8|int(p[2]) != 5+32 || p[3] != 0x13 || p[4] != 2 {
		t.Fatalf("C2 13 = % X", p)
	}
}

func TestStartupSpawnsLorencia(t *testing.T) {
	b := newBareWorld(t)
	npc := b.monsterByClass(0, 240)
	if npc == nil || npc.Type != TypeNPC || npc.Index >= SummonStart {
		t.Fatalf("NPC 240 = %+v", npc)
	}
	if !(npc.X == 147 && npc.Y == 145) && !(npc.X == 146 && npc.Y == 110) {
		t.Fatalf("NPC 240 at %d,%d", npc.X, npc.Y)
	}
	bull := b.monsterByClass(0, 0)
	if bull == nil || bull.Type != TypeMonster || bull.Mon.Life != 110 || bull.Mon.Info.Name != "Bull Fighter" {
		t.Fatalf("bull = %+v", bull)
	}
	r := b.mw().spawns[bull.Mon.SpawnIndex]
	if bull.X < r.X || bull.X >= r.TX || bull.Y < r.Y || bull.Y >= r.TY {
		t.Fatalf("bull at %d,%d outside %+v", bull.X, bull.Y, r)
	}
	if b.terrain(0).Get(bull.X, bull.Y)&gamedata.AttrCharacter == 0 {
		t.Fatal("cell not occupied")
	}
	// Index allocation: monsters and NPCs use 0..0x5013 in order.
	if b.objects[0].State != StatePlaying || b.objects[SummonStart].State != StateEmpty {
		t.Fatal("index layout")
	}
}

func TestAllocMonsterIndexReusesAfterTenSeconds(t *testing.T) {
	b := newBareWorld(t)
	b.mw().freed = nil // spawn records that found no cell at start-up
	o := b.monsterByClass(0, 0)
	idx := o.Index
	b.deleteMonster(o)
	first := b.allocMonsterIndex()
	if first == idx {
		t.Fatal("freed index reused before 10 s")
	}
	b.objects[first].State = StatePlaying
	b.advance(11 * time.Second)
	if got := b.allocMonsterIndex(); got != idx {
		t.Fatalf("alloc = %d, want freed %d", got, idx)
	}
}

func TestSendMonsterViewport(t *testing.T) {
	b := newBareWorld(t)
	p := b.player(PlayerStart, "Alpha", 0, 182, 128)
	b.viewportRemove(p)
	b.viewportAdd(p)
	b.viewportSend(p)
	pk := find(b.drain(p), 0x13, -1)
	if pk == nil {
		t.Fatal("no C2 13")
	}
	n := int(pk[4])
	found := false
	for i := 0; i < n; i++ {
		e := pk[5+16*i:]
		idx := int(e[0]&0x3F)<<8 | int(e[1])
		o := b.objects[idx]
		if e[0]&0x80 == 0 || int(e[2])<<8|int(e[3]) != o.Class || int(e[8]) != o.X || int(e[9]) != o.Y {
			t.Fatalf("entry % X for %+v", e[:16], o)
		}
		if o.Class == 236 {
			found = true
		}
	}
	if !found {
		t.Fatal("NPC 236 (175,120) not sent")
	}
}

func TestDirToAndDistance(t *testing.T) {
	for d, v := range pathDirs {
		if got := dirTo(v[0], v[1]); got != d {
			t.Fatalf("dirTo(%v) = %d, want %d", v, got, d)
		}
	}
	if distance(&Object{X: 0, Y: 0}, &Object{X: 3, Y: 4}) != 5 || distance(&Object{X: 0, Y: 0}, &Object{X: 2, Y: 2}) != 2 {
		t.Fatal("distance")
	}
}

func TestFindPath(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	path := b.findPath(0, x, y, x+6, y, true)
	if len(path) != 7 || path[0] != [2]int{x, y} || path[6] != [2]int{x + 6, y} {
		t.Fatalf("path = %v", path)
	}
	// More than 15 nodes is refused.
	if p := b.findPath(0, x, y, x+40, y, true); p != nil && len(p) >= 15 {
		t.Fatalf("long path = %v", p)
	}
}

// monsterNear places monster o next to player p on an open row.
func (b *bareWorld) monsterNear(class int, p *Object, dx int) *Object {
	o := b.monsterByClass(0, class)
	b.monsterOccupy(o, false)
	o.Move = nil
	o.X, o.Y, o.TX, o.TY = p.X+dx, p.Y, p.X+dx, p.Y
	o.Mon.StartX, o.Mon.StartY = o.X, o.Y
	o.Mon.SpawnIndex = -1
	o.Live = LiveNormal
	b.monsterOccupy(o, true)
	return o
}

func TestMonsterAIAggroAndAttack(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	p.P.Char.Life, p.P.Char.MaxLife = 1000, 1000
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	fixRand(t, 0)
	// Emotion 0: rand even -> no random move, search finds the player.
	m.Mon.Delay = 0
	b.monsterBaseAct(m)
	if m.Mon.Target != p.Index || m.Mon.Emotion != emotionAttack || m.Mon.EmotionCount != 30 {
		t.Fatalf("after search: %+v", *m.Mon)
	}
	// Emotion 1 inside AttackRange: attack with AttackSpeed delay.
	b.monsterBaseAct(m)
	if !m.Mon.ActAttack || m.Mon.Delay != m.Mon.AttackSpeed || m.Dir != dirTo(p.X-m.X, p.Y-m.Y) {
		t.Fatalf("after approach: %+v dir %d", *m.Mon, m.Dir)
	}
	b.drain(p)
	b.advance(2 * time.Second)
	m.Mon.ActAttack = false
	m.Mon.Delay = 0
	b.monsterProcess(m)
	pk := b.drain(p)
	act := find(pk, 0x18, -1)
	if act == nil || int(act[3])<<8|int(act[4]) != m.Index || act[6] != 0x78 || int(act[7])<<8|int(act[8]) != p.Index {
		t.Fatalf("action = % X (%d packets)", act, len(pk))
	}
	dmg := find(pk, 0xD7, -1)
	if dmg == nil || int(dmg[3]&0x7F)<<8|int(dmg[4]) != p.Index {
		t.Fatalf("damage = % X", dmg)
	}
	if p.P.Char.Life >= 1000 {
		t.Fatalf("player life %v", p.P.Char.Life)
	}
}

func TestMonsterChasesThroughSetPath(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 5)
	b.see(p, m)
	var gotDirs []byte
	b.hooks.setPath = func(w *World, o *Object, sx, sy int, dirs []byte) {
		if o == m {
			gotDirs = dirs
		}
	}
	fixRand(t, 0)
	m.Mon.Target, m.Mon.Emotion, m.Mon.EmotionCount = p.Index, emotionAttack, 30
	m.Mon.Delay = 0
	b.monsterProcess(m)
	// Target (x, y), monster at x+5: the chase cell is x+AttackRange.
	if m.Mon.MTX != x+1 || m.Mon.MTY != y || len(gotDirs) != 4 {
		t.Fatalf("MT=%d,%d dirs=%v", m.Mon.MTX, m.Mon.MTY, gotDirs)
	}
	for _, d := range gotDirs {
		if d != 7 {
			t.Fatalf("dirs = %v", gotDirs)
		}
	}
}

func TestMonsterDeathRegen(t *testing.T) {
	b := newBareWorld(t)
	o := b.monsterByClass(0, 0)
	o.Live = LiveNormal
	o.Alive = false
	o.Live = LiveDying
	o.Mon.Die, o.Mon.DieTime = 1, b.clock
	o.Mon.Life = 0
	b.advance(time.Duration(o.Mon.RegenTime) * time.Millisecond)
	b.monsterStateProc()
	if o.Mon.Die != 1 {
		t.Fatal("removed too early")
	}
	b.advance(1001 * time.Millisecond)
	b.monsterStateProc()
	if o.Mon.Die != 2 || o.Live != LiveRemoved {
		t.Fatalf("die=%d live=%d", o.Mon.Die, o.Live)
	}
	b.monsterStateProc()
	if o.Mon.Die != 0 || !o.Alive || o.Live != LiveNew || o.Mon.Life != o.Mon.MaxLife || o.Mon.Delay != 5000 {
		t.Fatalf("after regen: %+v", *o.Mon)
	}
}

func TestNpcTalkMessage(t *testing.T) {
	b := newBareWorld(t)
	npc := b.monsterByClass(0, 240)
	p := b.player(PlayerStart, "Alpha", 0, npc.X+1, npc.Y)
	b.handleNpcTalk(p, frame.Build(frame.C3, 0x30, byte(npc.Index>>8), byte(npc.Index)))
	pk := find(b.drain(p), 0x01, -1)
	if pk == nil || int(pk[3])<<8|int(pk[4]) != npc.Index || !bytes.HasPrefix(pk[5:], []byte("Seus Items")) ||
		int(pk[1]) != len(pk) || pk[len(pk)-1] != 0 {
		t.Fatalf("talk = % X", pk)
	}
	// Too far: nothing.
	p.X += 10
	b.handleNpcTalk(p, frame.Build(frame.C3, 0x30, byte(npc.Index>>8), byte(npc.Index)))
	if find(b.drain(p), 0x01, -1) != nil {
		t.Fatal("talk from 11 cells")
	}
	long := packetObjectChat(5, string(bytes.Repeat([]byte("a"), 100)))
	if len(long) != 0x3B+6 {
		t.Fatalf("long text size %d", len(long))
	}
}
