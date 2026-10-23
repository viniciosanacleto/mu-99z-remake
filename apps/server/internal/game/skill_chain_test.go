package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// FUN_004fdbd0: C1 10 BF 0A skillHi skillLo caster(LE) 3 0 targets(LE).
func TestChainLightningPacket(t *testing.T) {
	p := packetChainLightning(215, 0x5401, [3]int{0x1234, 0x0010, 0x1234})
	want := []byte{0xC1, 0x10, 0xBF, 0x0A, 0x00, 0xD7, 0x01, 0x54, 3, 0, 0x34, 0x12, 0x10, 0x00, 0x34, 0x12}
	if !bytes.Equal(p, want) {
		t.Fatalf("% X", p)
	}
	// FUN_0040b110: 100% / 70% / 50% by hit number.
	for hit, r := range map[int]int{0: 100, 1: 100, 2: 70, 3: 50, 4: 100} {
		if got := chainLightningRate(hit); got != r {
			t.Fatalf("hit %d rate %d", hit, got)
		}
	}
}

// chainWorld prepares a DW with skill 215 (absent from SkillList.txt, so
// it is injected) and four monsters: the target at +1, two in Radio of
// the target (+3 and +2) and one outside it (-2), the latter first in VP2.
func chainWorld(t *testing.T) (*bareWorld, *Object, [4]*Object) {
	b := newBareWorld(t)
	b.data.Skills[215] = gamedata.SkillInfo{Index: 215, Name: "Chain Lightning", Damage: 0, Mana: 5, Range: 6,
		Radio: 2, Type: -1, Class: [5]int{1, 0, 0, 0, 0}}
	x, y := b.openRow(0, 8)
	p := skillPlayer(b, PlayerStart, 0, x+2, y, 215)
	var m [4]*Object
	for i, dx := range []int{1, 3, 2, -2} {
		m[i] = b.nthMonsterNear(i, p, dx)
		m[i].Mon.DefenseRate, m[i].Mon.Defense, m[i].Mon.Life = 0, 0, 50000
	}
	for _, i := range []int{3, 0, 1, 2} {
		b.see(p, m[i])
	}
	stubStats(t, func(o *Object) CombatStats {
		if o.Type == TypePlayer {
			return CombatStats{MagicDamageMin: 1000, MagicDamageMax: 1000, AttackSuccessRate: 100}
		}
		return b.combatStatsMonster(o)
	})
	fixRand(t, 0)
	return b, p, m
}

func TestChainLightningChainAndDamage(t *testing.T) {
	b, p, m := chainWorld(t)
	m0 := p.P.Char.Mana
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 215, byte(m[0].Index>>8), byte(m[0].Index)))
	if p.P.Char.Mana != m0-5 {
		t.Fatalf("mana %v", p.P.Char.Mana)
	}
	bf := find(b.drain(p), 0xBF, 0x0A)
	want := packetChainLightning(215, p.Index, [3]int{m[0].Index, m[1].Index, m[2].Index})
	if !bytes.Equal(bf, want) {
		t.Fatalf("BF 0A % X want % X", bf, want)
	}
	st := sk(p)
	if len(st.Pending) != 3 {
		t.Fatalf("pending %+v", st.Pending)
	}
	for i, h := range st.Pending {
		if h.Code != 0x34 || h.Skill != 215 || h.Param != i+1 || h.Due != b.clock.Add(time.Duration(200*(i+1))*time.Millisecond) {
			t.Fatalf("hit %d %+v", i, h)
		}
	}
	// The hits land one by one, 200 ms apart.
	b.advance(201 * time.Millisecond)
	b.skillMessages(p)
	d1 := 50000 - m[0].Mon.Life
	if d1 <= 0 || m[1].Mon.Life != 50000 {
		t.Fatalf("first hit %v %v", m[0].Mon.Life, m[1].Mon.Life)
	}
	b.advance(200 * time.Millisecond)
	b.skillMessages(p)
	d2 := 50000 - m[1].Mon.Life
	b.advance(200 * time.Millisecond)
	b.skillMessages(p)
	d3 := 50000 - m[2].Mon.Life
	if m[3].Mon.Life != 50000 {
		t.Fatal("monster outside the target's Radio hit")
	}
	if d2 != float32(int(d1)*70/100) || d3 != float32(int(d1)*50/100) {
		t.Fatalf("damage %v %v %v", d1, d2, d3)
	}
}

// Without other objects near the target, the three slots stay on it.
func TestChainLightningSingleTarget(t *testing.T) {
	b, p, m := chainWorld(t)
	b.data.Skills[215] = gamedata.SkillInfo{Index: 215, Mana: 5, Range: 6, Radio: 0, Type: -1, Class: [5]int{1}}
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 215, byte(m[0].Index>>8), byte(m[0].Index)))
	bf := find(b.drain(p), 0xBF, 0x0A)
	if !bytes.Equal(bf, packetChainLightning(215, p.Index, [3]int{m[0].Index, m[0].Index, m[0].Index})) {
		t.Fatalf("BF 0A % X", bf)
	}
}

// The target out of Range or a HackSkillSpeedCheck refusal cancels the
// cast without taking mana.
func TestChainLightningRangeAndSpeedGate(t *testing.T) {
	b, p, m := chainWorld(t)
	m0 := p.P.Char.Mana
	b.data.Skills[215] = gamedata.SkillInfo{Index: 215, Mana: 5, Range: 0, Radio: 2, Type: -1, Class: [5]int{1}}
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 215, byte(m[0].Index>>8), byte(m[0].Index)))
	if find(b.drain(p), 0xBF, 0x0A) != nil || p.P.Char.Mana != m0 || len(sk(p).Pending) != 0 {
		t.Fatal("cast out of range")
	}
	b.data.Skills[215] = gamedata.SkillInfo{Index: 215, Mana: 5, Range: 6, Radio: 2, Type: -1, Class: [5]int{1}}
	b.dus().speedRules = []gamedata.SkillSpeedRule{{Skill: 215, Class: -1, MinLevel: -1, MaxLevel: -1, MinReset: -1,
		MaxReset: -1, MinSpeed: -1, MaxSpeed: -1, MaxDelay: -1, MinCount: 0, MaxCount: 0, Action: 0}}
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 215, byte(m[0].Index>>8), byte(m[0].Index)))
	if find(b.drain(p), 0xBF, 0x0A) != nil || p.P.Char.Mana != m0 || len(sk(p).Pending) != 0 {
		t.Fatal("cast despite the speed check")
	}
}

// nthMonsterNear is monsterNear with the k-th playing monster of map 0
// (monsterByClass always returns the same object).
func (b *bareWorld) nthMonsterNear(k int, p *Object, dx int) *Object {
	for i := 0; i < SummonStart; i++ {
		o := b.objects[i]
		if o.State != StatePlaying || o.Mon == nil || o.Map != 0 || o.Type != TypeMonster {
			continue
		}
		if k--; k >= 0 {
			continue
		}
		b.monsterOccupy(o, false)
		o.Move = nil
		o.X, o.Y, o.TX, o.TY = p.X+dx, p.Y, p.X+dx, p.Y
		o.Mon.StartX, o.Mon.StartY = o.X, o.Y
		o.Mon.SpawnIndex = -1
		o.Live = LiveNormal
		b.monsterOccupy(o, true)
		return o
	}
	b.t.Fatal("not enough monsters")
	return nil
}
