package game

import (
	"bytes"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestCombatPacketLayouts(t *testing.T) {
	p := packetDamage(0x1234, 70000, 0x103, 99.6, true)
	want := []byte{0xC1, 0x10, 0xD7, 0x92, 0x34, 0xFD, 0xE8, 0x03, 100, 0, 0, 0, 0x70, 0x11, 0x01, 0}
	if !bytes.Equal(p, want) {
		t.Fatalf("damage = % X", p)
	}
	if p := packetDie(0x0102, 5, 0x53FC); !bytes.Equal(p, []byte{0xC1, 0x08, 0x17, 0x01, 0x02, 5, 0x53, 0xFC}) {
		t.Fatalf("die = % X", p)
	}
	p = packetKillExp(0x0005, 0x00012345, 300, 1000, 2000)
	if len(p) != 0x18 || p[0] != 0xC3 || p[2] != 0x9C || p[3] != 0 || p[4] != 5 || p[6] != 0x01 || p[7] != 0 ||
		p[8] != 0x45 || p[9] != 0x23 || p[10] != 0x01 || p[11] != 0x2C || p[16] != 0xE8 || p[17] != 0x03 || p[20] != 0xD0 {
		t.Fatalf("exp = % X", p)
	}
	if p := packetSkillUse(2, 0x10, 0x53FC, true); !bytes.Equal(p, []byte{0xC3, 0x08, 0x19, 2, 0, 0x10, 0xD3, 0xFC}) {
		t.Fatalf("skill = % X", p)
	}
	if p := packetAttackAction(7, 3, 0x78, 0x53FC); !bytes.Equal(p, []byte{0xC1, 0x09, 0x18, 0, 7, 3, 0x78, 0x53, 0xFC}) {
		t.Fatalf("action = % X", p)
	}
	p = packetRespawn(125, 126, 0, 1, 70000, 50, 30, 1234, 5678)
	if len(p) != 0x24 || p[3] != 0x04 || p[4] != 125 || p[5] != 126 || p[7] != 1 || p[8] != 0xE8 || p[9] != 0xFD ||
		p[10] != 50 || p[12] != 30 || p[16] != 0xD2 || p[20] != 0x2E || p[24] != 0x70 || p[32] != 30 {
		t.Fatalf("respawn = % X", p)
	}
}

func TestHitCheck(t *testing.T) {
	fixRand(t, 4, 5)
	// Attack rate below defense rate: 5% hits (rand%100 <= 4) with 30%.
	if r, h := hitCheck(CombatStats{AttackSuccessRate: 10}, CombatStats{DefenseSuccessRate: 20}); !r || !h {
		t.Fatal("4 should hit")
	}
	if _, h := hitCheck(CombatStats{AttackSuccessRate: 10}, CombatStats{DefenseSuccessRate: 20}); h {
		t.Fatal("5 should miss")
	}
	// Otherwise miss when rand % attack < defense.
	fixRand(t, 105, 106)
	if _, h := hitCheck(CombatStats{AttackSuccessRate: 100}, CombatStats{DefenseSuccessRate: 6}); h {
		t.Fatal("105%100=5 < 6 must miss")
	}
	if r, h := hitCheck(CombatStats{AttackSuccessRate: 100}, CombatStats{DefenseSuccessRate: 6}); r || !h {
		t.Fatal("106%100=6 must hit")
	}
}

func TestAttackDamageFormulas(t *testing.T) {
	mon := &Object{Type: TypeMonster}
	pl := &Object{Type: TypePlayer, P: &Player{}}
	for i := range pl.P.Inventory {
		pl.P.Inventory[i] = item.Empty()
	}
	var eff int
	// Monster: min + rand%(max-min) - defense.
	fixRand(t, 7)
	if d := attackDamage(mon, pl, CombatStats{DamageMinRight: 16, DamageMaxRight: 21}, CombatStats{}, &eff, 10); d != 16+2-10 {
		t.Fatalf("monster damage %d", d)
	}
	// Player without weapon uses the left range; critical then excellent.
	as := CombatStats{DamageMinLeft: 10, DamageMaxLeft: 20, DamageMinRight: 50, DamageMaxRight: 60,
		CriticalDamage: 10, ExcellentDamage: 10, CriticalDamageBonus: 5, ExcellentDamageBonus: 7}
	fixRand(t, 3, 50, 50)
	eff = 0
	if d := attackDamage(pl, mon, as, CombatStats{}, &eff, 4); d != 13-4 || eff != 0 {
		t.Fatalf("plain %d eff %d", d, eff)
	}
	fixRand(t, 3, 9, 50)
	eff = 0
	if d := attackDamage(pl, mon, as, CombatStats{}, &eff, 4); d != 20+5-4 || eff != effectCritical {
		t.Fatalf("critical %d eff %d", d, eff)
	}
	fixRand(t, 3, 9, 9)
	eff = 0
	if d := attackDamage(pl, mon, as, CombatStats{}, &eff, 4); d != 20*120/100+7-4 || eff != effectExcellent {
		t.Fatalf("excellent %d eff %d", d, eff)
	}
	// A sword in the right hand switches to the right-hand range.
	pl.P.Inventory[0] = item.Item{Index: item.Make(0, 1)}
	fixRand(t, 0, 99, 99)
	if d := attackDamage(pl, mon, as, CombatStats{}, &eff, 0); d != 50 {
		t.Fatalf("right hand %d", d)
	}
	// Defense: halved for players, ignored with IgnoreDefenseRate.
	fixRand(t, 99)
	eff = 0
	if d := targetDefense(mon, pl, CombatStats{Defense: 31}, CombatStats{}, &eff); d != 15 {
		t.Fatalf("player defense %d", d)
	}
	fixRand(t, 0)
	if d := targetDefense(pl, mon, CombatStats{Defense: 31}, CombatStats{IgnoreDefenseRate: 1}, &eff); d != 0 || eff != effectIgnoreDefense {
		t.Fatalf("ignore defense %d %d", d, eff)
	}
}

func TestKillExperienceFormula(t *testing.T) {
	b := newBareWorld(t)
	p := b.player(PlayerStart, "Alpha", 0, 130, 130)
	p.P.Char.Level = 1
	m := b.monsterByClass(0, 0) // Bull Fighter, level 6, MaxLife 110
	// base = (6+25)*6/3 = 62; exp0 = 62/4+62 = 77; full kill:
	// 110*77/110 * 100 (player rate) * AddExperienceRate_AL0 (500) / 100 * map 100 / 100.
	if e := b.killExperience(p, m, 110); e != 77*100*500/100 {
		t.Fatalf("exp = %d", e)
	}
	// Damage above MaxLife counts as MaxLife; half damage half experience.
	if e := b.killExperience(p, m, 500); e != 77*500 {
		t.Fatalf("capped exp = %d", e)
	}
	if e := b.killExperience(p, m, 55); e != 38*500 {
		t.Fatalf("half exp = %d", e)
	}
	// Player 20 levels above: base = (6+10)*62/26.
	p.P.Char.Level = 26
	if e := b.killExperience(p, m, 110); e != int64((16*62/26)/4+16*62/26)*500 {
		t.Fatalf("high level exp = %d", e)
	}
}

func TestDropTableAndDamageTable(t *testing.T) {
	b := newBareWorld(t)
	for _, idx := range b.mw().drops[10] {
		ii := b.data.Items[idx]
		if ii.DropItem == 0 || 10 < ii.Level-2 || 10 > ii.Level+4 {
			t.Fatalf("item %d level %d in level 10 table", idx, ii.Level)
		}
	}
	if len(b.mw().drops[10]) == 0 {
		t.Fatal("empty drop table")
	}
	p := b.player(PlayerStart, "Alpha", 0, 130, 130)
	if r := b.damageTableRate(0, p, nil); r != 100 {
		t.Fatalf("damage table rate %d", r)
	}
}

func TestPlayerKillsMonster(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	p.Class = 1
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	var gained int64
	oldGain, oldStats, oldDrop := b.hooks.gainExperience, b.hooks.combatStats, b.hooks.dropMapMoney
	b.hooks.gainExperience = func(w *World, o *Object, exp int64) { gained += exp; o.P.Char.Experience += exp }
	b.hooks.combatStats = func(w *World, o *Object) CombatStats {
		return CombatStats{DamageMinLeft: 40, DamageMaxLeft: 50, AttackSuccessRate: 1000}
	}
	var money int
	b.hooks.dropMapMoney = func(w *World, mp, x, y, amount, owner int) bool { money = amount; return true }
	t.Cleanup(func() { b.hooks.gainExperience, b.hooks.combatStats, b.hooks.dropMapMoney = oldGain, oldStats, oldDrop })
	fixRand(t, 100)
	m.Mon.Life = 100
	b.handleAttack(p, frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
	pk := b.drain(p)
	// The 18 action goes to the attacker's viewers only (FUN_004da300).
	if a := find(pk, 0x18, -1); a != nil {
		t.Fatalf("action sent to attacker % X", a)
	}
	// hit: 100%1000=100 >= 6; damage 40 + 100%10 = 40 - defense 6 = 34,
	// below the level-400 minimum 40: 40 + 100%40 = 60, then the DK PvM
	// rate 95: 95*(100*60/100)/100 = 57.
	d := find(pk, 0xD7, -1)
	if d == nil || int(d[3])<<8|int(d[4]) != m.Index || int(d[5])<<8|int(d[6]) != 57 || d[8] != 43 || d[12] != 57 {
		t.Fatalf("damage % X", d)
	}
	if m.Mon.Life != 43 || m.Mon.LastAttacker != p.Index {
		t.Fatalf("life %v", m.Mon.Life)
	}
	m.Mon.Life = 10
	b.handleAttack(p, frame.Build(frame.C1, 0xD7, byte(m.Index>>8), byte(m.Index), 0x78, 3))
	pk = b.drain(p)
	die := find(pk, 0x17, -1)
	if die == nil || int(die[3])<<8|int(die[4]) != m.Index || int(die[6])<<8|int(die[7]) != p.Index {
		t.Fatalf("die % X", die)
	}
	if find(pk, 0xD7, -1) != nil {
		t.Fatal("damage packet sent on death")
	}
	ex := find(pk, 0x9C, -1)
	if ex == nil || gained != 0 && int64(ex[6])<<24|int64(ex[7])<<16|int64(ex[9])<<8|int64(ex[8]) != gained || gained == 0 {
		t.Fatalf("exp % X gained %d", ex, gained)
	}
	if m.Alive || m.Live != LiveDying || m.Mon.Die != 1 {
		t.Fatal("monster not dead")
	}
	// 500 ms later the die message drops money/items for the top hitter.
	// rand 100: no item (100 % ItemRate 130 = 100 is not < 100), money
	// (100 % MoneyRate 20 = 0 < 100/10) = MoneyAmountDropRate_AL0 (50) *
	// experience given / 100.
	// MapManager ItemDrop = 1 on Lorencia would let ItemDrop.txt /
	// ItemMoneyDrop.txt (FUN_00492870 / FUN_0049c920) win with rand 100;
	// they are switched off here to reach the base formula of FUN_004b7290
	// (drop_test.go covers those tables).
	info := b.data.Maps[0]
	info.ItemDrop = 0
	b.data.Maps[0] = info
	b.advance(600 * time.Millisecond)
	b.monsterMessages(m)
	if money != int(50*gained/100) || money == 0 {
		t.Fatalf("money %d for exp %d", money, gained)
	}
}

func TestMonsterKillsPlayerAndRespawn(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	p.P.Char.Life, p.P.Char.MaxLife = 5, 100
	m := b.monsterNear(0, p, 1)
	b.see(p, m)
	b.hooks.combatStats = func(w *World, o *Object) CombatStats { return CombatStats{} }
	fixRand(t, 0)
	m.Mon.Life = m.Mon.MaxLife
	b.attack(m, p, 0, false)
	pk := b.drain(p)
	if die := find(pk, 0x17, -1); die == nil || int(die[3])<<8|int(die[4]) != p.Index {
		t.Fatalf("player die % X", die)
	}
	if p.Alive || p.Live != LiveDying {
		t.Fatal("player alive")
	}
	b.advance(1100 * time.Millisecond)
	b.playerDeathProc()
	if p.Live != LiveRemoved {
		t.Fatalf("live %d", p.Live)
	}
	b.playerDeathProc()
	r := find(b.drain(p), 0xF3, 0x04)
	if r == nil || !p.Alive || p.P.Char.Life != 100 || p.Live != LiveNew || int(r[4]) != p.X || int(r[6]) != p.Map {
		t.Fatalf("respawn % X alive %v life %v", r, p.Alive, p.P.Char.Life)
	}
	if g := b.data.Gates[b.respawnGate(0)]; p.X < g.X1 || p.X > g.X2 || p.Y < g.Y1 || p.Y > g.Y2 {
		t.Fatalf("respawn at %d,%d", p.X, p.Y)
	}
}
