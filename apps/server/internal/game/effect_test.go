package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
)

func TestLoadEffectList(t *testing.T) {
	b := newBareWorld(t)
	tbl := b.effectTable()
	if e := tbl[55]; e.Group != 27 || e.Name != "Poison" || e.Flag != 1 || e.Duration != -1 || e.Item != -1 {
		t.Fatalf("poison %+v", e)
	}
	if e := tbl[28]; e.Type != 1 || e.Group != 20 {
		t.Fatalf("GM %+v", e)
	}
	if len(tbl) != 32 {
		t.Fatalf("%d effects", len(tbl))
	}
}

func TestAddEffectBuffExpiresAndPackets(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	q := b.player(PlayerStart+1, "Beta", 0, x+1, y)
	b.see(p, q)
	if !b.addEffect(p, effect.GreaterDefense, 2, [4]int{24}, true) {
		t.Fatal("not added")
	}
	if effOpt(p).AddDefense != 24 || p.ViewSkillState&8 == 0 {
		t.Fatalf("opt %+v vss %#x", effOpt(p), p.ViewSkillState)
	}
	e := find(b.drain(p), 0x07, -1)
	if e == nil || e[3] != 1 || e[7] != 0 || e[4] != 8 {
		t.Fatalf("self % X", e)
	}
	if e := find(b.drain(q), 0x07, -1); e == nil || int(e[8])<<8|int(e[9]) != p.Index {
		t.Fatalf("viewer % X", e)
	}
	// A weaker copy is refused (EffectOverwriteMode 0), a stronger one replaces.
	if b.addEffect(p, effect.GreaterDefense, 2, [4]int{10}, true) {
		t.Fatal("weaker copy accepted")
	}
	if !b.addEffect(p, effect.GreaterDefense, 2, [4]int{30}, true) || effOpt(p).AddDefense != 30 {
		t.Fatalf("stronger copy: %+v", effOpt(p))
	}
	b.drain(p)
	b.effectSecond(p)
	if !hasEffectIndex(p, effect.GreaterDefense) {
		t.Fatal("expired early")
	}
	b.effectSecond(p)
	if hasEffectIndex(p, effect.GreaterDefense) || effOpt(p).AddDefense != 0 || p.ViewSkillState&8 != 0 {
		t.Fatalf("not expired: %+v", effOpt(p))
	}
	if e := find(b.drain(p), 0x07, -1); e == nil || e[3] != 0 {
		t.Fatalf("removal % X", e)
	}
}

func TestPoisonTickDamage(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	m.Mon.Life = 1000
	// Poison: value[0] attacker, every 2 s (ConstA) 3% (ConstB) of life.
	b.addEffect(m, effect.Poison, 20, [4]int{p.Index, 2, 3, 0}, true)
	b.drain(p)
	b.effectSecond(m) // count 19: odd, no damage
	if m.Mon.Life != 1000 {
		t.Fatalf("life %v", m.Mon.Life)
	}
	b.effectSecond(m) // count 18
	if m.Mon.Life != 970 {
		t.Fatalf("life %v", m.Mon.Life)
	}
	d := find(b.drain(p), 0xD7, -1)
	if d == nil || int(d[5])<<8|int(d[6]) != 30 || d[7] != 5 {
		t.Fatalf("damage % X", d)
	}
}

func TestIceSlowsAndSternImmobilizes(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	b.addEffect(p, effect.Ice, 10, [4]int{}, true)
	if !mv(p).Slow {
		t.Fatal("ice did not slow")
	}
	b.removeEffect(p, effect.Ice)
	if mv(p).Slow {
		t.Fatal("still slow")
	}
	b.addEffect(p, effect.Stern, 10, [4]int{}, true)
	if !b.immobilized(p) {
		t.Fatal("stern does not block walking")
	}
	b.clearEffects(p)
	if b.immobilized(p) {
		t.Fatal("clear kept stern")
	}
}

func TestGreaterLifeRaisesMaxLife(t *testing.T) {
	b := newBareWorld(t)
	p := statsPlayer(b, PlayerStart, 1, 50)
	b.calcCharacter(p)
	before := p.P.Char.MaxLife + float32(p.P.AddLife)
	b.addEffect(p, effect.GreaterLife, 60, [4]int{20}, true)
	after := p.P.Char.MaxLife + float32(p.P.AddLife)
	if int(after) != int(before)+int(before)*20/100 {
		t.Fatalf("max life %v -> %v", before, after)
	}
	b.leaveWorld(p)
	if s := b.statsOf(p).Effects; s.LifeRate[0] != 0 {
		t.Fatalf("rate kept after leaving: %+v", s)
	}
}

func TestCancelEffectPacket(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	b.addEffect(p, effect.InfinityArrow, 0, [4]int{}, true)
	b.advance(time.Hour)
	b.effectSecond(p)
	if !hasEffectIndex(p, effect.InfinityArrow) {
		t.Fatal("duration 0 must never expire")
	}
	b.handleCancelEffect(p, []byte{0xC1, 0x04, 0x1B, 77})
	if hasEffectIndex(p, effect.InfinityArrow) {
		t.Fatal("1B did not cancel")
	}
}
