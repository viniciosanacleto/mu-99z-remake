package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// skillPlayer places a playing character of class cls with skills
// learned, outside any safe zone.
func skillPlayer(b *bareWorld, index, cls int, x, y int, skills ...int) *Object {
	o := b.player(index, "Sk"+string(rune('A'+index-PlayerStart)), 0, x, y)
	o.Class = cls
	c := o.P.Char
	c.Class = byte(cls << 4)
	c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership = 30, 20, 20, 80, 0
	c.Level = 400
	for i, s := range skills {
		o.P.Skills[i] = Skill{Index: s}
	}
	b.calcCharacter(o)
	c.Life, c.Mana, c.BP = objMaxLife(o), float32(o.P.AddMana)+c.MaxMana, float32(o.P.AddBP)+c.MaxBP
	b.drain(o)
	return o
}

func stubStats(t *testing.T, f func(o *Object) CombatStats) {
	old := combatStatsHook
	combatStatsHook = func(w *World, o *Object) CombatStats { return f(o) }
	t.Cleanup(func() { combatStatsHook = old })
}

func TestSkillPacketLayouts(t *testing.T) {
	if p := packetSkillArea(9, 0x1234, 100, 101, 3); !bytes.Equal(p, []byte{0xC3, 9, 0x1E, 9, 0x12, 0x34, 100, 101, 3}) {
		t.Fatalf("1E % X", p)
	}
	if p := packetNovaStage(0x5401, 4); !bytes.Equal(p, []byte{0xC1, 7, 0xBA, 0x54, 0x01, 0x28, 4}) {
		t.Fatalf("BA % X", p)
	}
	if p := packetKillCount(3); !bytes.Equal(p, []byte{0xC1, 5, 0xB8, 1, 3}) {
		t.Fatalf("B8 % X", p)
	}
	if p := packetSkillUse(27, 0x5400, 0x5401, true); !bytes.Equal(p, []byte{0xC3, 8, 0x19, 27, 0x54, 0, 0xD4, 1}) {
		t.Fatalf("19 % X", p)
	}
}

// FUN_004f7c00 / FUN_004f8340: angle 0 keeps the frustum unrotated.
func TestSkillFrustum(t *testing.T) {
	fx, fy := skillFrustumPoints(100, 100, 0, 6, 6)
	if fx != [4]int{94, 106, 101, 99} || fy != [4]int{106, 106, 100, 100} {
		t.Fatalf("%v %v", fx, fy)
	}
	if !inFrustum(fx, fy, 100, 103) || inFrustum(fx, fy, 100, 97) || inFrustum(fx, fy, 110, 103) {
		t.Fatal("inside test")
	}
	// 128 -> 128*360/255 = 180 degrees: the frustum points the other way.
	fx, fy = skillFrustumPoints(100, 100, 128, 6, 6)
	if !inFrustum(fx, fy, 100, 97) || inFrustum(fx, fy, 100, 103) {
		t.Fatalf("rotated %v %v", fx, fy)
	}
}

// FUN_00434360 with ComboSkill.txt (DK: 19..23 stage 0, 41..43 stage 1).
func TestComboSequence(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := skillPlayer(b, PlayerStart, 1, x, y)
	seq := []struct {
		skill int
		combo bool
	}{{19, false}, {41, false}, {42, true}, {41, false}, {19, false}, {41, false}, {41, false}}
	for i, s := range seq {
		if got := b.comboCheck(p, s.skill); got != s.combo {
			t.Fatalf("step %d skill %d combo %v", i, s.skill, got)
		}
	}
	// The second skill must come within the 3000 ms delay.
	b.comboCheck(p, 20)
	b.advance(3100 * time.Millisecond)
	if b.comboCheck(p, 41) || b.comboCheck(p, 42) {
		t.Fatal("combo after the delay")
	}
}

func TestGreaterDefenseCast(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := skillPlayer(b, PlayerStart, 2, x, y, 27)
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 27, byte(p.Index>>8), byte(p.Index)))
	// (Ene 80 / GreaterDefenseConstB 8 + ConstA 2) * GreaterDefenseRateFE 100 / 100 * 2.
	s := effectSlotOf(p, effGreaterDefense)
	if s == nil || s.Value[0] != 24 || s.Count != 300 || effOpt(p).AddDefense != 24 {
		t.Fatalf("slot %+v", s)
	}
	full := float32(p.P.AddMana) + p.P.Char.MaxMana
	if p.P.Char.Mana != full-30 {
		t.Fatalf("mana %v of %v", p.P.Char.Mana, full)
	}
	pk := b.drain(p)
	if u := find(pk, 0x19, -1); u == nil || u[3] != 27 || u[6]&0x80 == 0 {
		t.Fatalf("19 % X", u)
	}
	if m := find(pk, 0x27, 0xFF); m == nil || int(m[4])<<8|int(m[5]) != int(full)-30 {
		t.Fatalf("mana % X", m)
	}
	// Not enough mana: nothing happens.
	p.P.Char.Mana = 10
	b.removeEffect(p, effGreaterDefense)
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 27, byte(p.Index>>8), byte(p.Index)))
	if hasEffectIndex(p, effGreaterDefense) {
		t.Fatal("cast without mana")
	}
}

func TestManaShieldAndGreaterDamageFormulas(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := skillPlayer(b, PlayerStart, 0, x, y, 16)
	if !b.skillManaShield(p, b.skillRefOf(16, 0), p.Index) {
		t.Fatal("mana shield refused")
	}
	// 10 + Dex 20/50 + Ene 80/200 = 10; 60 + 80/40 = 62 s.
	if s := effectSlotOf(p, effManaShield); s == nil || s.Value[0] != 10 || s.Count != 62 || effOpt(p).DamageReduction != 10 {
		t.Fatalf("slot %+v", s)
	}
	e := skillPlayer(b, PlayerStart+1, 2, x+1, y, 28)
	b.see(p, e)
	if !b.skillGreaterDamage(e, b.skillRefOf(28, 0), p.Index) {
		t.Fatal("greater damage refused")
	}
	// Ene 80/7 + 3 = 14 (rate DW 100) for 300 s on the target.
	if s := effectSlotOf(p, effGreaterDamage); s == nil || s.Value[0] != 14 || s.Count != 300 || effOpt(p).AddPhysiDamage != 14 {
		t.Fatalf("slot %+v", s)
	}
}

func TestSkillDelayAndRequirements(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 3)
	p := skillPlayer(b, PlayerStart, 4, x, y, 62)
	if !b.skillDelayOK(p, 62) || b.skillDelayOK(p, 62) {
		t.Fatal("Earthquake delay 10000 not applied")
	}
	b.advance(10 * time.Second)
	if !b.skillDelayOK(p, 62) {
		t.Fatal("delay not over")
	}
	// Stern (67) needs 4 kills and a guild status.
	if b.skillRequirementsOK(p, 67) {
		t.Fatal("guild skill without guild")
	}
	// Class column: Twisting Slash (41) is not a FE skill.
	p.Class = 2
	if b.skillRequirementsOK(p, 41) {
		t.Fatal("class column ignored")
	}
}

func TestEnergyBallMagicDamage(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := skillPlayer(b, PlayerStart, 0, x, y, 17)
	m := b.monsterNear(0, p, 2)
	b.see(p, m)
	m.Mon.DefenseRate, m.Mon.Defense, m.Mon.Life = 0, 0, 1000
	stubStats(t, func(o *Object) CombatStats {
		if o.Type == TypePlayer {
			return CombatStats{MagicDamageMin: 50, MagicDamageMax: 50, AttackSuccessRate: 100}
		}
		return b.combatStatsMonster(o)
	})
	fixRand(t, 0)
	m0 := p.P.Char.Mana
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 17, byte(m.Index>>8), byte(m.Index)))
	pk := b.drain(p)
	// Energy Ball: damage 3 -> CSkill 3..4; 50+3 = 53, DWDamageRatePvM 110 -> 58.
	d := find(pk, 0xD7, -1)
	if d == nil || int(d[5])<<8|int(d[6]) != 58 || m.Mon.Life != 942 {
		t.Fatalf("damage % X life %v", d, m.Mon.Life)
	}
	if u := find(pk, 0x19, -1); u == nil || u[3] != 17 {
		t.Fatalf("19 % X", u)
	}
	if p.P.Char.Mana != m0-1 {
		t.Fatalf("mana %v", p.P.Char.Mana)
	}
}

func (b *bareWorld) combatStatsMonster(o *Object) CombatStats {
	m := o.Mon
	return CombatStats{DamageMinRight: m.DamageMin, DamageMaxRight: m.DamageMax, Defense: m.Defense,
		AttackSuccessRate: m.AttackRate, DefenseSuccessRate: m.DefenseRate}
}

func TestTwistingSlashHitsAround(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := skillPlayer(b, PlayerStart, 1, x+2, y, 41)
	p.P.Inventory[0] = item.Item{Index: item.Make(0, 0), Durability: 20}
	m1 := b.monsterNear(0, p, 1)
	m2 := b.monsterNear(0, p, -2)
	b.see(p, m1)
	b.see(p, m2)
	for _, m := range []*Object{m1, m2} {
		m.Mon.DefenseRate, m.Mon.Defense, m.Mon.Life = 0, 0, 5000
	}
	stubStats(t, func(o *Object) CombatStats {
		if o.Type == TypePlayer {
			return CombatStats{DamageMinRight: 100, DamageMaxRight: 100, AttackSuccessRate: 100}
		}
		return b.combatStatsMonster(o)
	})
	fixRand(t, 0)
	m0, bp0 := p.P.Char.Mana, p.P.Char.BP
	b.handleSkillArea(p, frame.Build(frame.C3, 0x1E, 41, byte(p.X), byte(p.Y), 2, 0, 0, 0, 0))
	pk := b.drain(p)
	if a := find(pk, 0x1E, -1); a == nil || a[3] != 41 || int(a[4])<<8|int(a[5]) != p.Index {
		t.Fatalf("1E % X", a)
	}
	// Both monsters within Radio 3 are hit.
	if m1.Mon.Life == 5000 || m2.Mon.Life == 5000 {
		t.Fatalf("lives %v %v", m1.Mon.Life, m2.Mon.Life)
	}
	if p.P.Char.Mana != m0-10 || p.P.Char.BP != bp0-10 {
		t.Fatalf("mana %v bp %v", p.P.Char.Mana, p.P.Char.BP)
	}
}

func TestAreaMagicTargetsBy10(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := skillPlayer(b, PlayerStart, 0, x+2, y, 9)
	m1 := b.monsterNear(0, p, 1)
	b.see(p, m1)
	m1.Mon.DefenseRate, m1.Mon.Defense, m1.Mon.Life = 0, 0, 5000
	stubStats(t, func(o *Object) CombatStats {
		if o.Type == TypePlayer {
			return CombatStats{MagicDamageMin: 100, MagicDamageMax: 100, AttackSuccessRate: 100}
		}
		return b.combatStatsMonster(o)
	})
	fixRand(t, 0)
	hit := frame.Build(frame.C3, 0x10, 9, byte(p.X), byte(p.Y), 0, 1, byte(m1.Index>>8), byte(m1.Index), 0)
	// 0x10 before the 0x1E cast is ignored.
	b.handleSkillMulti(p, hit)
	if m1.Mon.Life != 5000 {
		t.Fatal("hit without a cast")
	}
	m0 := p.P.Char.Mana
	b.handleSkillArea(p, frame.Build(frame.C3, 0x1E, 9, byte(p.X), byte(p.Y), 2, 0, 0, 0, 0))
	if sk(p).AreaSkill != 9 || p.P.Char.Mana != m0-90 {
		t.Fatalf("area %d mana %v", sk(p).AreaSkill, p.P.Char.Mana)
	}
	b.handleSkillMulti(p, hit)
	// Evil Spirit 45 -> 45..67: 100+45 = 145, DW PvM 110 -> 159.
	if m1.Mon.Life != 5000-159 {
		t.Fatalf("life %v", m1.Mon.Life)
	}
	for i := 0; i < 6; i++ {
		b.handleSkillMulti(p, hit)
	}
	if want := float32(5000 - 159*5); m1.Mon.Life != want {
		t.Fatalf("more than 5 packets: life %v want %v", m1.Mon.Life, want)
	}
}

func TestNovaChargeAndRelease(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := skillPlayer(b, PlayerStart, 0, x+2, y, 40)
	p.P.Char.Mana, p.P.Char.BP = 500, 100
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	m.Mon.DefenseRate, m.Mon.Defense, m.Mon.Life = 0, 0, 50000
	stubStats(t, func(o *Object) CombatStats {
		if o.Type == TypePlayer {
			return CombatStats{MagicDamageMin: 100, MagicDamageMax: 100, AttackSuccessRate: 100}
		}
		return b.combatStatsMonster(o)
	})
	fixRand(t, 0)
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 0x3a, byte(m.Index>>8), byte(m.Index)))
	if !sk(p).NovaCharging || p.P.Char.Mana != 320 {
		t.Fatalf("charging %v mana %v", sk(p).NovaCharging, p.P.Char.Mana)
	}
	if u := find(b.drain(p), 0x19, -1); u == nil || u[3] != 0x3a {
		t.Fatalf("19 % X", u)
	}
	b.advance(600 * time.Millisecond)
	b.novaTick(p)
	if ba := find(b.drain(p), 0xBA, -1); ba == nil || ba[6] != 1 {
		t.Fatalf("BA % X", ba)
	}
	b.handleSkillTarget(p, frame.Build(frame.C3, 0x19, 40, byte(m.Index>>8), byte(m.Index)))
	if sk(p).NovaCharging || len(sk(p).Pending) != 1 || p.P.Char.Mana != 320 {
		t.Fatalf("release: %+v mana %v", sk(p), p.P.Char.Mana)
	}
	b.advance(700 * time.Millisecond)
	b.skillMessages(p)
	// Stage 1: magic 100 + Str 30/NovaC 2 + 1*NovaB 5*NovaA 20 = 215, PvM 110 -> 236.
	if m.Mon.Life != 50000-236 {
		t.Fatalf("life %v", m.Mon.Life)
	}
}
