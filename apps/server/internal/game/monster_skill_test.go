package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/monsterskill"
)

func TestMonsterAreaAttackHitsFive(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	m := b.monsterByClass(0, 0)
	var ps []*Object
	for i := 0; i < 7; i++ {
		p := b.player(PlayerStart+i, "P"+string(rune('a'+i)), 0, x+1+i%3, y)
		p.P.Char.Life, p.P.Char.MaxLife = 5000, 5000
		ps = append(ps, p)
	}
	m = b.monsterNear(0, ps[0], -1)
	for _, p := range ps {
		b.see(p, m)
		b.drain(p)
	}
	m.Mon.Skills = []int{50}
	fixRand(t, 0)
	b.monsterAreaAttack(m)
	hit := 0
	for _, p := range ps {
		pk := b.drain(p)
		if e := find(pk, 0x1E, -1); e == nil || e[3] != 50 || int(e[4])<<8|int(e[5]) != m.Index {
			t.Fatalf("1E to %d: % X", p.Index, e)
		}
		if find(pk, 0xD7, -1) != nil {
			hit++
		}
	}
	if hit != 5 {
		t.Fatalf("%d players hit, want 5 (FUN_00500b70 cap)", hit)
	}
	// A skill outside the area list hits nobody.
	m.Mon.Skills = []int{17}
	b.monsterAreaAttack(m)
	for _, p := range ps {
		if find(b.drain(p), 0xD7, -1) != nil {
			t.Fatal("non-area skill hit")
		}
	}
}

func TestMonsterSkillUnitWitchQueen(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	p.P.Char.Life, p.P.Char.MaxLife = 1000, 1000
	m := b.monsterNear(0, p, 1)
	m.Class = 304 // Witch Queen: units 4 and 1
	b.see(p, m)
	b.drain(p)
	if !b.hasMonsterSkill(m) {
		t.Fatal("Witch Queen has MonsterSkill")
	}
	var stun []int
	b.hooks.addEffect = func(w *World, o *Object, effect, value, sec int) bool {
		stun = append(stun, o.Index, effect, sec)
		return true
	}
	// Slot 1 -> unit 1 (target, scope 0 / 6 cells, element 1 = stun 50%).
	fixRand(t, 1, 0)
	b.useMonsterSkill(m, p)
	pk := find(b.drain(p), 0x69, -1)
	if pk == nil || pk[3] != 1 || int(pk[4])|int(pk[5])<<8 != m.Index {
		t.Fatalf("69 = % X", pk)
	}
	if len(stun) != 3 || stun[0] != p.Index || stun[1] != 0x3D || stun[2] != 3 {
		t.Fatalf("stun = %v", stun)
	}
	// Failed success roll: nothing.
	stun = nil
	fixRand(t, 1, 99)
	b.useMonsterSkill(m, p)
	if stun != nil {
		t.Fatal("element applied on a failed roll")
	}
	b.drain(p)
	// Life element: -100 (type 12) and +10% (type 1), C1 26 FF.
	b.applySkillElement(&monsterskill.Element{Type: 2, IncDecType: 12, IncDecValue: 100}, m, p)
	if p.P.Char.Life != 900 || find(b.drain(p), 0x26, 0xFF) == nil {
		t.Fatalf("life %v", p.P.Char.Life)
	}
	b.applySkillElement(&monsterskill.Element{Type: 2, IncDecType: 1, IncDecValue: 10}, m, p)
	if p.P.Char.Life != 990 {
		t.Fatalf("life %v", p.P.Char.Life)
	}
	b.applySkillElement(&monsterskill.Element{Type: 2, IncDecType: 0x70, IncDecValue: 5000}, m, p)
	if p.P.Char.Life != 0 {
		t.Fatalf("life %v", p.P.Char.Life)
	}
}

func TestTrapPicksPlayerInRange(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	if !b.setMonster(m, 100) {
		t.Fatal("trap class")
	}
	m.Mon.AttackRange = 1
	b.see(p, m)
	fixRand(t, 0)
	b.trapAct(m)
	if m.Mon.Target != p.Index || !m.Mon.ActAttack || m.Mon.Delay != m.Mon.AttackSpeed {
		t.Fatalf("trap = %+v", *m.Mon)
	}
	m.Mon.ActAttack = false
	p.X += 3
	b.trapAct(m)
	if m.Mon.Target != -1 || m.Mon.ActAttack || m.Mon.Delay != m.Mon.Info.MoveSpeed {
		t.Fatalf("out of range: %+v", *m.Mon)
	}
	p.X -= 3
	mv(p).Authority = 2 // GMs are ignored
	b.trapAct(m)
	if m.Mon.Target != -1 {
		t.Fatal("GM targeted")
	}
	if !isTrap(100) || !isTrap(0x6E) || isTrap(0x6F) || !isTrap(0x20B) {
		t.Fatal("isTrap")
	}
}

func TestRegenPositionAttributes(t *testing.T) {
	b := newBareWorld(t)
	o := b.monsterByClass(0, 0)
	x0, y0 := o.X, o.Y
	o.Mon.Attribute = 6
	o.Live = LiveRemoved
	if b.regenPosition(o) || o.X != x0 || o.Live != LiveNew || o.Mon.Die != 0 {
		t.Fatalf("attribute 6: live %d die %d", o.Live, o.Mon.Die)
	}
	b.monsterOccupy(o, false)
	o.Mon.Attribute = 7
	// rand: +-(rand%6) with the sign from the next rand (odd = +).
	fixRand(t, 2, 1, 3, 0)
	if !b.regenPosition(o) {
		t.Skip("cell blocked")
	}
	if o.X != x0+2 || o.Y != y0-3 {
		t.Fatalf("attribute 7 regen at %d,%d from %d,%d", o.X, o.Y, x0, y0)
	}
}

// Invasion monsters (Attribute 5, no spawn record) regenerate near their
// death cell instead of dying for good.
func TestRegenPositionAttribute5WithoutSpawn(t *testing.T) {
	b := newBareWorld(t)
	o := b.monsterByClass(0, 0)
	b.monsterOccupy(o, false)
	x0, y0 := o.X, o.Y
	o.Mon.Attribute, o.Mon.SpawnIndex = 5, -1
	fixRand(t, 1, 1, 1, 1)
	if !b.regenPosition(o) {
		t.Skip("cell blocked")
	}
	if o.X != x0+1 || o.Y != y0+1 || o.Mon.Die != 0 {
		t.Fatalf("attribute 5 regen at %d,%d die %d", o.X, o.Y, o.Mon.Die)
	}
	// Through the state machine: dead -> removed -> alive again.
	o.Alive, o.Live = false, LiveDying
	o.Mon.Die, o.Mon.DieTime = 1, b.clock
	b.advance(time.Duration(o.Mon.RegenTime+1001) * time.Millisecond)
	b.monsterStateProc()
	b.monsterStateProc()
	if !o.Alive || o.Mon.Die != 0 || o.Live != LiveNew || o.State != StatePlaying {
		t.Fatalf("not regenerated: alive %v die %d live %d", o.Alive, o.Mon.Die, o.Live)
	}
}
