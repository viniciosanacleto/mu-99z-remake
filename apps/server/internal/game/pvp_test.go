package game

import (
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// pvpPair places two players side by side outside the safe zone with
// fixed combat stats (attacker 100..100, target defense 40).
func pvpPair(t *testing.T, b *bareWorld, ca, ct int) (*Object, *Object) {
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	v := b.player(PlayerStart+1, "Beta", 0, x+1, y)
	a.Class, v.Class = ca, ct
	v.P.Char.Life, v.P.Char.MaxLife = 1000, 1000
	b.see(a, v)
	stubStats(t, func(o *Object) CombatStats {
		if o == a {
			return CombatStats{DamageMinLeft: 100, DamageMaxLeft: 100, AttackSuccessRate: 1000}
		}
		return CombatStats{Defense: 40}
	})
	return a, v
}

// FUN_0040b110 PvP: 100 - 40/2 = 80; GeneralDamageRatePvP 100,
// DWDamageRatePvP 110 -> 88; DWDamageRateToDK 105 -> 92. rand 0 also
// gives the DamageStuckRate flag (0 < 10).
func TestPvPDamageRates(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	fixRand(t, 0)
	b.handleAttack(a, frame.Build(frame.C1, 0xD7, byte(v.Index>>8), byte(v.Index), 0x78, 3))
	d := find(b.drain(v), 0xD7, -1)
	if d == nil || int(d[5])<<8|int(d[6]) != 92 || d[3]&0x80 == 0 || v.P.Char.Life != 908 {
		t.Fatalf("damage % X life %v", d, v.P.Char.Life)
	}
	// The attacker is recorded for self defense (pk.go).
	if !b.isSelfDefense(v, a) {
		t.Fatal("self defense not recorded")
	}
}

func TestPvPDuelRateAndDefenseSkill(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	*dl(a) = duelState{Asked: -1, AskedBy: -1, Opponent: v.Index}
	*dl(v) = duelState{Asked: -1, AskedBy: -1, Opponent: a.Index}
	fixRand(t, 0)
	b.attack(a, v, 0, false)
	// 92 * DuelDamageRate 60 / 100 = 55; no self defense in a duel.
	if v.P.Char.Life != 1000-55 || b.isSelfDefense(v, a) {
		t.Fatalf("life %v", v.P.Char.Life)
	}
	// Defense (skill 18) on the target: -DefenseConstA 50% for 4 s.
	sk(v).DefenseTime = b.now()
	v.P.Char.Life = 1000
	b.attack(a, v, 0, false)
	// 80 - 50% = 40, then 40*110/100 = 44, 44*105/100 = 46, *60/100 = 27.
	if v.P.Char.Life != 1000-27 {
		t.Fatalf("defense skill: life %v", v.P.Char.Life)
	}
	b.advance(5 * time.Second)
	v.P.Char.Life = 1000
	b.attack(a, v, 0, false)
	if v.P.Char.Life != 1000-55 {
		t.Fatalf("after defense: life %v", v.P.Char.Life)
	}
}

func TestPvPAllowedRules(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	if !b.pvpAllowed(a, v) {
		t.Fatal("Lorencia PvP refused")
	}
	v.P.Char.Level = 5
	if b.pvpAllowed(a, v) {
		t.Fatal("level 5 target allowed (EnableKillLevel1 = 0)")
	}
	v.P.Char.Level = 400
	mv(v).Authority = 8
	if b.pvpAllowed(a, v) {
		t.Fatal("GM target allowed")
	}
	mv(v).Authority = 0
	m := b.monsterNear(0, a, -1)
	if !b.pvpAllowed(a, m) {
		t.Fatal("monster target refused")
	}
}

func TestPvPReflect(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	a.P.Char.Life, a.P.Char.MaxLife = 1000, 1000
	b.statsOf(v).DamageReflect = 50
	fixRand(t, 0)
	b.attack(a, v, 0, false)
	if len(sk(v).Pending) != 1 || sk(v).Pending[0].Param != 46 {
		t.Fatalf("pending %+v", sk(v).Pending)
	}
	b.drain(a)
	b.advance(20 * time.Millisecond)
	b.skillMessages(v)
	// Fixed 46: ReflectDamageRatePvP 100, DKReflectDamageRateToDW 100,
	// then the PvP rates of the reflecting DK: 95 and DKDamageRateToDW 100.
	d := find(b.drain(a), 0xD7, -1)
	if d == nil || d[7] != 4 || int(d[5])<<8|int(d[6]) != 43 {
		t.Fatalf("reflect % X", d)
	}
}

func TestPlayerDeathPenaltyPKAndKillCount(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	c := v.P.Char
	c.Level = 100
	floor, next := b.data.Experience.Floor(100), b.data.Experience.Next(100)
	c.Experience = int64(floor) + 1000
	v.P.NextExp = next
	var killed bool
	old := playerKillHook
	playerKillHook = func(w *World, k, vic *Object) { killed = k == a && vic == v }
	t.Cleanup(func() { playerKillHook = old })
	c.Life = 1
	fixRand(t, 0)
	b.attack(a, v, 0, false)
	if v.Alive || !killed {
		t.Fatalf("alive %v pk %v", v.Alive, killed)
	}
	// ExperienceDeduceRate1 1% of the level span; MoneyDeduceRate 4%.
	loss := int64((uint64(next-floor) * 1) / 100)
	want := int64(floor) + 1000 - loss
	if want < int64(floor) {
		want = int64(floor)
	}
	if c.Experience != want || c.Money != 1000000-40000 {
		t.Fatalf("exp %d want %d money %d", c.Experience, want, c.Money)
	}
	pk := b.drain(v)
	if r := find(pk, 0xF3, 0xE0); r == nil || len(r) != 0xA4 || int(uint32(r[8])|uint32(r[9])<<8|uint32(r[10])<<16|uint32(r[11])<<24) != int(want) {
		t.Fatalf("F3 E0 % X", r)
	}
	if k := find(b.drain(a), 0xB8, -1); k == nil || k[4] != 1 {
		t.Fatalf("kill count % X", k)
	}
}

func TestCharRefreshLayout(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 2)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	c := p.P.Char
	c.Level, c.LevelUpPoints, c.Strength, c.Life, c.MaxLife = 77, 5, 70000, 123, 456
	p.Class = 3
	c.Resets, c.MasterResets, p.P.Account.Level = 260, 2, 3
	r := b.packetCharRefresh(p)
	if r[0] != 0xC1 || r[1] != 0xA4 || r[2] != 0xF3 || r[3] != 0xE0 || r[4] != 77 || r[6] != 5 ||
		int(r[16])|int(r[17])<<8 != 65000 || r[26] != 123 || int(r[28])|int(r[29])<<8 != 456 || r[46] != 3 ||
		int(r[144])|int(r[145])<<8|int(r[146])<<16 != 70000 ||
		int(r[48])|int(r[49])<<8 != 260 || r[52] != 2 || r[56] != 3 {
		t.Fatalf("% X", r)
	}
}
