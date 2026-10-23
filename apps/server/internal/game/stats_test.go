package game

import (
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// statsPlayer places a character of the class with the DefaultClassInfo
// base stats at the given level.
func statsPlayer(b *bareWorld, index, class, level int) *Object {
	b.t.Helper()
	x, y := b.openRow(0, 2)
	o := b.player(index, "Stat"+string(rune('A'+index-PlayerStart)), 0, x, y)
	ci := b.statsData.classes[class]
	c := o.P.Char
	o.Class, c.Class, c.Level = class, byte(class<<4), level
	c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership = ci.Strength, ci.Dexterity, ci.Vitality, ci.Energy, ci.Lead
	c.Life, c.MaxLife, c.Mana, c.MaxMana, c.BP, c.MaxBP = 0, 0, 0, 0, 0, 0
	c.Experience = int64(b.data.Experience.Floor(level))
	o.P.Account.Level = 0
	return o
}

func equip(o *Object, slot, cat, idx, level int, dur float32) {
	o.P.Inventory[slot] = item.Item{Index: item.Make(cat, idx), Level: level, Durability: dur}
}

type expectStats struct {
	minR, maxR, minL, maxL, magMin, magMax, asr, as, ms, dsr, def int
	life, mana                                                    float32
	bp                                                            int
}

func checkStats(t *testing.T, w *World, o *Object, e expectStats) {
	t.Helper()
	s := w.statsOf(o)
	c := o.P.Char
	got := expectStats{s.DamageMinRight, s.DamageMaxRight, s.DamageMinLeft, s.DamageMaxLeft,
		s.MagicDamageMin, s.MagicDamageMax, s.AttackSuccessRate, s.AttackSpeed, s.MagicSpeed,
		s.DefenseSuccessRate, s.Defense, c.MaxLife, c.MaxMana, s.MaxBP}
	if got != e {
		t.Fatalf("class %d level %d:\n got %+v\nwant %+v", o.Class, c.Level, got, e)
	}
}

// Base values of each class at level 1 with the DefaultClassInfo stats and
// the Character.dat constants (doc 08 §4/§5), computed by hand:
//
//	DW 18/18/15/30: phys 18/6, 18/4; magic 30/9, 30/4; ASR 3*18/2+18/4+5;
//	    speed 18/20, 18/10; DSR 18/3; def 18/3; BP .2*18+.4*18+.3*15+.2*30
//	DK 28/20/25/10: phys 28/8, 28/4; ASR 3*20/2+28/4+5; BP .2*20+.15*28+.3*25+10
//	FE 22/25/20/15: phys (22+25)/7, /4; ASR 3*25/2+22/4+5; DSR 25/4; def 25/8
//	MG 26x4: phys 26/12+26/6, 26/8+26/4
//	DL 26/20/20/15/25: phys 15/14+26/7, 15/10+26/5; ASR 6*20/2+26/4+25/10+5
func TestCalcCharacterClassBases(t *testing.T) {
	b := newBareWorld(t)
	cases := []struct {
		class int
		e     expectStats
	}{
		{0, expectStats{3, 4, 3, 4, 3, 7, 36, 0, 1, 6, 6, 60, 60, 21}},
		{1, expectStats{3, 7, 3, 7, 1, 2, 42, 1, 1, 6, 6, 110, 20, 25}},
		{2, expectStats{6, 11, 6, 11, 1, 3, 47, 0, 0, 6, 3, 80, 30, 20}},
		{3, expectStats{6, 9, 6, 9, 2, 6, 50, 1, 1, 8, 8, 110, 60, 23}},
		{4, expectStats{4, 6, 4, 6, 1, 3, 73, 2, 2, 5, 6, 40090, 40, 23}},
	}
	for i, tc := range cases {
		o := statsPlayer(b, PlayerStart+i, tc.class, 1)
		b.calcCharacter(o)
		checkStats(t, b.World, o, tc.e)
		c := o.P.Char
		// Empty pools are filled (100% of nothing becomes 100% of max).
		if c.Life != c.MaxLife || c.Mana != c.MaxMana || int(c.BP) != tc.e.bp {
			t.Fatalf("class %d pools %v/%v %v/%v %v", tc.class, c.Life, c.MaxLife, c.Mana, c.MaxMana, c.BP)
		}
	}
	dk := b.objects[PlayerStart+1]
	if b.statsOf(dk).DKDamageMultiplier != 10/10+200 {
		t.Fatalf("DK multiplier %d", b.statsOf(dk).DKDamageMultiplier)
	}
	dl := b.objects[PlayerStart+4]
	if b.statsOf(dl).DLDamageMultiplier != 15/20+200 {
		t.Fatalf("DL multiplier %d", b.statsOf(dl).DLDamageMultiplier)
	}
}

// Life and mana per level: MaxLife + LevelLife*(level-1) +
// (Vit-VitBase)*VitalityToLife (U-COMBAT-03).
func TestCalcCharacterLifeManaPerLevel(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 50)
	o.P.Char.Vitality += 10
	o.P.Char.Energy += 4
	b.calcCharacter(o)
	c := o.P.Char
	if c.MaxLife != 110+2*49+10*3 || c.MaxMana != 20+0.5*49+4*1 {
		t.Fatalf("life %v mana %v", c.MaxLife, c.MaxMana)
	}
	// The current ratio is kept: half life stays half.
	c.Life = c.MaxLife / 2
	c.Vitality += 10
	b.calcCharacter(o)
	if c.MaxLife != 268 || c.Life != 134 {
		t.Fatalf("after vit: %v/%v", c.Life, c.MaxLife)
	}
}

// DK with a Short Sword (0,1: 3~7, speed 20, ReqStr 60, item level 3) and a
// Small Shield (6,0: def 1, DSR 3, ReqStr 70): requirements
// ((3)*60*3)/100+20 = 25 and ((3)*70*3)/100+20 = 26 <= 28.
func TestCalcCharacterWeaponAndShield(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	equip(o, 0, 0, 1, 0, 22)
	equip(o, 1, 6, 0, 0, 22)
	b.calcCharacter(o)
	checkStats(t, b.World, o, expectStats{6, 14, 3, 7, 1, 2, 42, 21, 21, 9, 7, 110, 20, 25})
	s := b.statsOf(o)
	if !s.Valid[0] || !s.Valid[1] || s.Items[0].Req.Str != 25 || s.Items[1].Req.Str != 26 {
		t.Fatalf("valid %v req %+v %+v", s.Valid[:2], s.Items[0].Req, s.Items[1].Req)
	}

	// A Kris (0,0) in the left hand needs Dex ((6)*40*3)/100+20 = 27 > 20:
	// its damage is not counted, but the dual-wield 55% (index test only)
	// still applies to both hands.
	equip(o, 1, 0, 0, 0, 20)
	b.calcCharacter(o)
	s = b.statsOf(o)
	if s.Valid[1] || s.DamageMinLeft != 3*55/100 || s.DamageMaxLeft != 7*55/100 || s.DamageMinRight != 6*55/100 {
		t.Fatalf("kris counted: valid %v %d~%d", s.Valid[1], s.DamageMinLeft, s.DamageMaxLeft)
	}
	// Zero durability disables the sword.
	o.P.Inventory[1] = item.Empty()
	o.P.Inventory[0].Durability = 0
	b.calcCharacter(o)
	if s := b.statsOf(o); s.Valid[0] || s.DamageMinRight != 3 {
		t.Fatalf("broken sword counted")
	}
}

// CItem::Convert level bonuses: +3 per level and (l-9)(l-8)/2 above +9;
// excellent: dmg*25/ItemLevel + 5 (DamageMax uses DamageMin, as in the
// original); requirements with ItemLevel+25.
func TestConvertItemDamage(t *testing.T) {
	b := newBareWorld(t)
	d := b.statsData
	info := b.data.Items[item.Make(0, 1)]
	s := d.convertItem(info, item.Item{Index: item.Make(0, 1), Level: 11, Durability: 50}, 0)
	if s.DamageMin != 3+33+3 || s.DamageMax != 7+33+3 || s.MaxDurability != 22+21 {
		t.Fatalf("+11 short sword %d~%d dur %v", s.DamageMin, s.DamageMax, s.MaxDurability)
	}
	s = d.convertItem(info, item.Item{Index: item.Make(0, 1), Durability: 37, Excellent: 1}, 0)
	if s.DamageMin != 3*25/3+5+3 || s.DamageMax != 3*25/3+5+7 || s.Req.Str != (28*60*3)/100+20 || s.MaxDurability != 37 {
		t.Fatalf("exc short sword %d~%d req %d dur %v", s.DamageMin, s.DamageMax, s.Req.Str, s.MaxDurability)
	}
	// Durability factors: +9 max 22+14 = 36; 16 < 50% -> 0.8; 4 < 20% of
	// 22 -> 0.5 (rounded half to even).
	s = d.convertItem(info, item.Item{Index: item.Make(0, 1), Level: 9, Durability: 16}, 0)
	if s.DurFactor != 0.8 || s.DamageMin != 24 || s.DamageMax != 27 { // (3+27)*0.8, (7+27)*0.8=27.2
		t.Fatalf("worn sword factor %v %d~%d", s.DurFactor, s.DamageMin, s.DamageMax)
	}
	s = d.convertItem(info, item.Item{Index: item.Make(0, 1), Durability: 4}, 0)
	if s.DurFactor != 0.5 || s.DamageMin != 2 || s.DamageMax != 4 { // 1.5 -> 2, 3.5 -> 4 (half even)
		t.Fatalf("broken sword factor %v %d~%d", s.DurFactor, s.DamageMin, s.DamageMax)
	}
	// Shields add the item level once: Small Shield +5 = 1+5.
	sh := d.convertItem(b.data.Items[item.Make(6, 0)], item.Item{Index: item.Make(6, 0), Level: 5, Durability: 40}, 0)
	if sh.Defense != 6 || sh.DefenseSuccess != 3+15 {
		t.Fatalf("shield def %d dsr %d", sh.Defense, sh.DefenseSuccess)
	}
	// Wings use WingOption DefenseConstA (3 per level for 12,0).
	wg := d.convertItem(b.data.Items[item.Make(12, 0)], item.Item{Index: item.Make(12, 0), Level: 4, Durability: 250}, 0)
	if wg.Defense != 10+12 {
		t.Fatalf("wings def %d", wg.Defense)
	}
	// Ring of Ice +2: ice resistance = level * 1.
	rg := d.convertItem(b.data.Items[item.Make(13, 8)], item.Item{Index: item.Make(13, 8), Level: 2, Durability: 50}, 0)
	if rg.Resist[0] != 2 {
		t.Fatalf("ring resist %v", rg.Resist)
	}
}

// ItemOption.txt: additional option (slot 2, code 80 x level) adds damage
// and raises ReqStr by 4 per level; luck (slot 1, code 84) adds critical
// damage rate; excellent masks 4 (code 97 speed +7) and 8 (code 94 +2%).
func TestCalcCharacterItemOptions(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	o.P.Char.Strength = 80
	o.P.Inventory[0] = item.Item{Index: item.Make(0, 1), Durability: 40, Option: 2, Luck: true, Excellent: 4 | 8}
	b.calcCharacter(o)
	s := b.statsOf(o)
	it := s.Items[0]
	if it.Options[2] != (itemOption{80, 8}) || it.Options[1] != (itemOption{84, 5}) || it.Req.Str != (28*60*3)/100+20+8 {
		t.Fatalf("options %+v req %d", it.Options, it.Req.Str)
	}
	// Item: exc 3*25/3+5+3 = 33 ~ 37 (needs Str 70+8). Base 80/8=10 ~ 80/4=20.
	// Right hand 43~57, +8 (code 80) = 51~65, +2% (code 94) = 52~66.
	if s.DamageMinRight != 52 || s.DamageMaxRight != 66 || s.DamageMinLeft != 18 {
		t.Fatalf("damage %d~%d left %d", s.DamageMinRight, s.DamageMaxRight, s.DamageMinLeft)
	}
	// Speed 20/15=1 + sword 20, +7.
	if s.AttackSpeed != 28 || s.CriticalDamageRate != 5 {
		t.Fatalf("speed %d crit %d", s.AttackSpeed, s.CriticalDamageRate)
	}
}

// Full Bronze set +10 for a DK (level 100, Str 200, Dex 60): each piece
// gets +31 defense; the full set adds 10% DSR and 5% defense.
func TestCalcCharacterFullSet(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 100)
	o.P.Char.Strength, o.P.Char.Dexterity = 200, 60
	for slot := 2; slot <= 6; slot++ {
		equip(o, slot, slot+5, 0, 10, 51)
	}
	b.calcCharacter(o)
	s := b.statsOf(o)
	if !s.FullSet {
		t.Fatalf("not a full set: valid %v", s.Valid)
	}
	// def 60/3=20 + (40+45+41+35+35) = 216, +5% = 226; DSR 20 + 10% = 22;
	// speed 60/15=4 + gloves 4, 60/20=3 + 4.
	checkStats(t, b.World, o, expectStats{25, 50, 25, 50, 1, 2, 640, 8, 7, 22, 226, 308, 69.5, 59})
	// One piece at +11: still 5 pieces >= +10, no 5x +11 -> +5%.
	o.P.Inventory[6].Level = 11
	b.calcCharacter(o)
	if s := b.statsOf(o); s.Defense != (20+40+45+41+35+(4+33+3))*105/100 {
		t.Fatalf("mixed set defense %d", s.Defense)
	}
}

// FE with a Short Bow (4,0 in the left hand) uses Dex/7+Str/14,
// Dex/4+Str/8; +1 arrows (4,15) in the right hand add (2*1+1)% + 1.
func TestCalcCharacterBowAndArrows(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 2, 1)
	equip(o, 1, 4, 0, 0, 20)
	equip(o, 0, 4, 15, 0, 255)
	b.calcCharacter(o)
	s := b.statsOf(o)
	if s.DamageMinLeft != 25/7+22/14+3 || s.DamageMaxLeft != 25/4+22/8+5 || s.AttackSpeed != 30 {
		t.Fatalf("bow %d~%d speed %d valid %v", s.DamageMinLeft, s.DamageMaxLeft, s.AttackSpeed, s.Valid[:2])
	}
	o.P.Inventory[0].Level = 1
	b.calcCharacter(o)
	s = b.statsOf(o)
	if s.DamageMinLeft != 8 || s.DamageMaxLeft != 14 {
		t.Fatalf("arrows +1: %d~%d", s.DamageMinLeft, s.DamageMaxLeft)
	}
}

// DK dual wield: both hands at 55%.
func TestCalcCharacterDualWield(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	equip(o, 0, 0, 1, 0, 22)
	equip(o, 1, 0, 1, 0, 22)
	b.calcCharacter(o)
	s := b.statsOf(o)
	if s.DamageMinRight != 6*55/100 || s.DamageMaxRight != 14*55/100 || s.DamageMinLeft != 3 || s.AttackSpeed != 1+20 {
		t.Fatalf("dual %d~%d left %d~%d speed %d", s.DamageMinRight, s.DamageMaxRight, s.DamageMinLeft, s.DamageMaxLeft, s.AttackSpeed)
	}
}

func TestMaxBPFormula(t *testing.T) {
	cases := []struct{ cls, s, d, v, e, l, want int }{
		{0, 18, 18, 15, 30, 0, 21},       // 3.6+7.2+4.5+6
		{1, 28, 20, 25, 10, 0, 25},       // 4.2+4+7.5+10
		{2, 22, 25, 20, 15, 0, 20},       // 6.6+5+6+3
		{3, 26, 26, 26, 26, 0, 23},       // 5.2+6.5+7.8+3.9
		{4, 26, 20, 20, 15, 25, 23},      // 7.8+4+2+2.25+7.5
		{1, 1000, 500, 300, 200, 0, 540}, // 100+150+90+200
	}
	for _, c := range cases {
		if got := maxBPFormula(c.cls, c.s, c.d, c.v, c.e, c.l); got != c.want {
			t.Errorf("class %d: %d, want %d", c.cls, got, c.want)
		}
	}
}

// F3 06: C1 05 F3 06 type -> C1 30 F3 06 (type+0x10, max life/mana u16,
// max BP u16, then u32 points, life, mana, BP, str, agi, vit, ene, cmd).
func TestStatAddPacket(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	o.P.Char.LevelUpPoints = 2
	b.calcCharacter(o)
	b.drain(o)
	b.handleStatAdd(o, frame.Build(frame.C1, 0xF3, 0x06, 2))
	p := find(b.drain(o), 0xF3, 0x06)
	if p == nil || len(p) != 0x30 || p[4] != 0x12 {
		t.Fatalf("F3 06 % X", p)
	}
	u32 := func(off int) int { return int(binary.LittleEndian.Uint32(p[off:])) }
	if binary.LittleEndian.Uint16(p[6:]) != 113 || u32(12) != 1 || u32(16) != 113 || u32(20) != 20 ||
		u32(28) != 28 || u32(32) != 20 || u32(36) != 26 || u32(40) != 10 {
		t.Fatalf("F3 06 % X", p)
	}
	if bp := maxBPFormula(1, 28, 20, 26, 10, 0); u32(24) != bp || int(binary.LittleEndian.Uint16(p[8:])) != bp {
		t.Fatalf("BP % X", p)
	}
	// MaxStatPoint (32767) and missing points fail with result 0.
	o.P.Char.Energy = 32767
	b.handleStatAdd(o, frame.Build(frame.C1, 0xF3, 0x06, 3))
	if p := find(b.drain(o), 0xF3, 0x06); p == nil || p[4] != 0 {
		t.Fatalf("over max % X", p)
	}
	o.P.Char.LevelUpPoints = 0
	b.handleStatAdd(o, frame.Build(frame.C1, 0xF3, 0x06, 0))
	if p := find(b.drain(o), 0xF3, 0x06); p == nil || p[4] != 0 || o.P.Char.Strength != 28 {
		t.Fatalf("no points % X", p)
	}
}

// CalcCharacter ends with the WizardTeam C1 CC F3 E1 and C1 3C F3 E3.
func TestCharStatsPackets(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 1)
	equip(o, 0, 0, 1, 0, 22)
	b.calcCharacter(o)
	pk := b.drain(o)
	e1 := find(pk, 0xF3, 0xE1)
	e3 := find(pk, 0xF3, 0xE3)
	if len(e1) != 0xCC || len(e3) != 0x3C {
		t.Fatalf("E1 %d E3 %d", len(e1), len(e3))
	}
	u32 := func(p []byte, off int) int { return int(int32(binary.LittleEndian.Uint32(p[off:]))) }
	if u32(e1, 12) != 110 || u32(e1, 32) != 28 || u32(e1, 72) != 6 || u32(e1, 76) != 14 ||
		u32(e1, 128) != 21 || u32(e1, 152) != 42 || u32(e1, 160) != 6 || u32(e1, 172) != 201 || u32(e1, 176) != 100 {
		t.Fatalf("E1 % X", e1)
	}
	if u32(e3, 52) != 100 {
		t.Fatalf("E3 % X", e3)
	}
}

// Rebuilding the CharSet (FUN_004c6ba0) recomputes the character.
func TestRebuildCharSetRecalculates(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 3, 1)
	b.rebuildCharSet(o)
	if o.P.Char.MaxLife != 110 || o.P.Stats == nil {
		t.Fatalf("max life %v", o.P.Char.MaxLife)
	}
	if cs := b.combatStatsHook(o); cs.DamageMinRight != 6 || cs.AttackSuccessRate != 50 {
		t.Fatalf("combat stats %+v", cs)
	}
}

func (b *bareWorld) combatStatsHook(o *Object) CombatStats { return combatStatsHook(b.World, o) }
