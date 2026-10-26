package game

import (
	"fmt"
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Character stats: CItem::Convert (FUN_0048aed0) for the combat values of
// equipped items and CalcCharacter (FUN_004c9790) with its callees. The
// native formulas are authoritative (GameServerInfo - Character.dat
// CalcLuaScriptSwitch = 0). All arithmetic mirrors the C int / x87 float
// mix of the original.

// classPrefix are the class prefixes of the Character.dat keys.
var classPrefix = [5]string{"DW", "DK", "FE", "MG", "DL"}

// statsData holds the tables CalcCharacter and the experience code use.
type statsData struct {
	classes     map[int]gamedata.ClassInfo
	options     map[int][]gamedata.ItemOptionRule
	wingOptions map[int]gamedata.WingOption
	customWings map[int]gamedata.CustomWing
	attributes  map[int]gamedata.CustomAttribute
	customMons  []gamedata.CustomMonster
	k           classConsts
}

// classConsts are the CServerInfo constants of Character.dat
// (offsets 0xe8c..0x1250, see .re/cfgmap.txt) per class 0..4.
type classConsts struct {
	physMinA, physMaxA     [5]int // <C>PhysiDamageMin/MaxConstA
	physMinB, physMaxB     [5]int // MG/DL <C>PhysiDamageMin/MaxConstB
	feBowMinA, feBowMinB   int
	feBowMaxA, feBowMaxB   int
	magicMin, magicMax     [5]int
	asrA, asrB, asrC, asrD [5]int // <C>AttackSuccessRateConstA..D
	dlAsrE                 int
	physSpeed, magicSpeed  [5]int
	dsr                    [5]int // <C>DefenseSuccessRateConstA
	defense                [5]int // <C>DefenseConstA
	dkMulA, dkMulMax       int
	dlMulA, dlMulMax       int
	spirit                 [11]int // DarkSpirit min A,B,C / max A,B,C / speed A,B,C,D
	spiritRate             [3]int  // DarkSpiritAttackSuccessRateConstA..C
	damageRatePvP          [5]int  // <C>DamageRatePvP (F3 E3)
	damageRatePvM          [5]int
}

func loadClassConsts(w *World) classConsts {
	var k classConsts
	ci := func(key string) int { return w.cfg.Int("Character", key, 0) }
	for c, p := range classPrefix {
		k.physMinA[c] = ci(p + "PhysiDamageMinConstA")
		k.physMaxA[c] = ci(p + "PhysiDamageMaxConstA")
		k.physMinB[c] = ci(p + "PhysiDamageMinConstB")
		k.physMaxB[c] = ci(p + "PhysiDamageMaxConstB")
		k.magicMin[c] = ci(p + "MagicDamageMinConstA")
		k.magicMax[c] = ci(p + "MagicDamageMaxConstA")
		k.asrA[c] = ci(p + "AttackSuccessRateConstA")
		k.asrB[c] = ci(p + "AttackSuccessRateConstB")
		k.asrC[c] = ci(p + "AttackSuccessRateConstC")
		k.asrD[c] = ci(p + "AttackSuccessRateConstD")
		k.physSpeed[c] = ci(p + "PhysiSpeedConstA")
		k.magicSpeed[c] = ci(p + "MagicSpeedConstA")
		k.dsr[c] = ci(p + "DefenseSuccessRateConstA")
		k.defense[c] = ci(p + "DefenseConstA")
		k.damageRatePvP[c] = ci(p + "DamageRatePvP")
		k.damageRatePvM[c] = ci(p + "DamageRatePvM")
	}
	k.feBowMinA, k.feBowMinB = ci("FEPhysiDamageMinBowConstA"), ci("FEPhysiDamageMinBowConstB")
	k.feBowMaxA, k.feBowMaxB = ci("FEPhysiDamageMaxBowConstA"), ci("FEPhysiDamageMaxBowConstB")
	k.dlAsrE = ci("DLAttackSuccessRateConstE")
	k.dkMulA, k.dkMulMax = ci("DKDamageMultiplierConstA"), ci("DKDamageMultiplierMaxRate")
	k.dlMulA, k.dlMulMax = ci("DLDamageMultiplierConstA"), ci("DLDamageMultiplierMaxRate")
	for i, n := range []string{"MinConstA", "MinConstB", "MinConstC", "MaxConstA", "MaxConstB", "MaxConstC"} {
		k.spirit[i] = ci("DarkSpiritAttackDamage" + n)
	}
	for i, n := range []string{"A", "B", "C", "D"} {
		k.spirit[6+i] = ci("DarkSpiritAttackSpeedConst" + n)
	}
	for i, n := range []string{"A", "B", "C"} {
		k.spiritRate[i] = ci("DarkSpiritAttackSuccessRateConst" + n)
	}
	return k
}

func loadStatsData(w *World) (*statsData, error) {
	d := &statsData{}
	var err error
	if d.classes, err = gamedata.LoadClassInfo(w.configDir); err != nil {
		return nil, err
	}
	if d.options, err = gamedata.LoadItemOptions(w.configDir); err != nil {
		return nil, err
	}
	if d.wingOptions, err = gamedata.LoadWingOptions(w.configDir); err != nil {
		return nil, err
	}
	if d.customWings, err = gamedata.LoadCustomWings(w.configDir); err != nil {
		return nil, err
	}
	if d.attributes, err = gamedata.LoadCustomAttributes(w.configDir); err != nil {
		return nil, err
	}
	if d.customMons, err = gamedata.LoadCustomMonsters(w.configDir); err != nil {
		return nil, err
	}
	d.k = loadClassConsts(w)
	return d, nil
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		d, err := loadStatsData(w)
		if err != nil {
			return fmt.Errorf("stats data: %w", err)
		}
		w.statsData = d
		return nil
	})
	recalcHook = func(w *World, o *Object) { w.calcCharacter(o) }
	combatStatsHook = func(w *World, o *Object) CombatStats { return w.playerCombatStats(o) }
	f3Handlers[0x06] = func(w *World, o *Object, p []byte) { w.handleStatAdd(o, p) }
}

// ---------------------------------------------------------------------------
// CItem::Convert (combat part)

// itemOption is one of the 14 CItem option slots (+0x2a index, +0x46 value).
type itemOption struct{ Index, Value int }

// itemStats are the CItem fields CalcCharacter reads after Convert.
type itemStats struct {
	Known          bool // the Item.txt row exists (Convert ran)
	Index          int
	Level          int     // +0x06
	Slot           int     // +0x08 (Item.txt Slot)
	TwoHand        bool    // +0x0a
	AttackSpeed    int     // +0x0b (byte)
	WalkSpeed      int     // +0x0c (byte)
	DamageMin      int     // +0x0e (u16)
	DamageMax      int     // +0x10 (u16)
	DefenseSuccess int     // +0x12 (byte)
	Defense        int     // +0x14 (u16)
	MagicDefense   int     // +0x16 (u16)
	MagicPower     int     // +0x1e (u16)
	Durability     float32 // +0x20
	MaxDurability  float32 // +0x24
	DurFactor      float32 // +0xac
	Resist         [7]int  // +0x6e (bytes)
	Req            itemReq // +0x62..+0x6c
	Options        [14]itemOption
	Skill          int  // +0x98
	Luck           int  // +0x99
	OptionLevel    int  // +0x9a
	Excellent      byte // +0x9b (after the HaveOption check)
	Ancient        byte // +0xb3
	SkillWeapon    bool // +0xb0 (Item.txt Skill 0x42)
	PetLevel       int  // +0xc4
}

func u16(v int) int { return int(uint16(v)) }
func u8(v int) int  { return int(uint8(v)) }

// isCustomWing is FUN_0046a990 on CCustomWing.
func (d *statsData) customWing(index int) (gamedata.CustomWing, bool) {
	if d == nil {
		return gamedata.CustomWing{}, false
	}
	cw, ok := d.customWings[index]
	return cw, ok
}

// convertItem reproduces the combat fields of CItem::Convert (FUN_0048aed0).
// petLevel is CItem+0xc4 (Dark Horse / Dark Raven).
func (d *statsData) convertItem(info gamedata.ItemInfo, it item.Item, petLevel int) itemStats {
	s := itemStats{Known: true, Index: it.Index, Level: it.Level & 0xf, Slot: info.Slot,
		TwoHand: infoTwoHand(info), Durability: it.Durability, PetLevel: petLevel}
	idx, lvl := it.Index, s.Level
	// +0x9b keeps the excellent byte after the HaveOption check only; the
	// local copy used by the formulas also drops it for a few items.
	s.Excellent = it.Excellent
	if info.HaveOption == 0 {
		s.Excellent = 0
	}
	exc := convertExcellent(info, it)
	if itemSetValid(it.SetOption) {
		s.Ancient = it.SetOption & 0xf
	}
	if it.Skill {
		s.Skill = 1
	}
	if it.Luck {
		s.Luck = 1
	}
	s.OptionLevel = it.Option

	if maxDurability(info, idx, 0, false, false) != 0 {
		s.MaxDurability = float32(maxDurability(info, idx, lvl, exc != 0, s.Ancient != 0))
	}
	f2 := s.MaxDurability * 0.5
	f3 := s.MaxDurability * 0.4
	f4 := s.MaxDurability * 0.3
	f5 := s.MaxDurability * 0.2
	s.DurFactor = 1
	switch {
	case s.Durability == 0:
		s.DurFactor = 0
	case s.Durability < f5:
		s.DurFactor = 0.5
	case s.Durability < f4:
		s.DurFactor = 0.6
	case s.Durability < f3:
		s.DurFactor = 0.7
	case s.Durability < f2:
		s.DurFactor = 0.8
	}
	res := info.Resistance
	for i := range s.Resist {
		s.Resist[i] = min(lvl*res[i], 0xff)
	}
	isCustom := false
	if _, ok := d.customWing(idx); ok {
		isCustom = true
	}
	s.Req = itemRequirements(info, it, isCustom, petLevel)

	ilvl := info.Level
	if s.Ancient != 0 {
		ilvl = info.Level + 30
	} else if exc != 0 {
		ilvl = info.Level + 25
	}
	special := 0
	switch idx {
	case 0x406:
		special = 15
	case 0xa07:
		special = 25
	case 0x806:
		special = 30
	}
	levelBonus := func(v int) int {
		v += lvl * 3
		if lvl > 9 {
			v += (lvl - 9) * (lvl - 8) / 2
		}
		return v
	}
	// Damage: the excellent bonus of DamageMax uses the unmodified
	// DamageMin (as in the original).
	baseMin := info.DamageMin
	if v := info.DamageMax; v != 0 {
		if s.Ancient == 0 || info.Level == 0 {
			if exc != 0 {
				if special == 0 {
					if info.Level != 0 {
						v = u16(baseMin*25/info.Level + 5 + v)
					}
				} else {
					v = u16(v + special)
				}
			}
		} else {
			v = u16(ilvl/40 + 10 + v + baseMin*25/info.Level)
		}
		s.DamageMax = u16(levelBonus(v))
	}
	if v := info.DamageMin; v != 0 {
		if s.Ancient == 0 || info.Level == 0 {
			if exc != 0 {
				if special == 0 {
					if info.Level != 0 {
						v = u16(v*25/info.Level + 5 + v)
					}
				} else {
					v = u16(v + special)
				}
			}
		} else {
			v = u16(ilvl/40 + 10 + v + v*25/info.Level)
		}
		s.DamageMin = u16(levelBonus(v))
	}
	if v := info.MagicDamageRate; v != 0 {
		if s.Ancient == 0 || info.Level == 0 {
			if exc != 0 {
				if special == 0 {
					if info.Level != 0 {
						v = u16(v*25/info.Level + 5 + v)
					}
				} else {
					v = u16(v + special)
				}
			}
		} else {
			v = u16(ilvl/60 + 7 + v + v*25/info.Level)
		}
		s.MagicPower = u16(levelBonus(v))
	}
	if v := info.DefenseSuccessRate; v != 0 {
		if s.Ancient == 0 || info.Level == 0 {
			if exc != 0 && info.Level != 0 {
				v = u8(v*25/info.Level + v + 5)
			}
		} else {
			v = u8(v*25/info.Level + ilvl/40 + v + 10)
		}
		v = u8(v + lvl*3)
		if lvl > 9 {
			v = u8((lvl-9)*(lvl-8)/2 + v)
		}
		s.DefenseSuccess = v
	}
	def := info.Defense
	if idx == 0x1a1e || idx == 0x1831 {
		def = 0xf
	}
	if def != 0 {
		if idx < 0xc00 || idx > 0xdff {
			if s.Ancient == 0 || info.Level == 0 || ilvl == 0 {
				if exc != 0 && info.Level != 0 {
					def = u16(info.Level/5 + 4 + def + def*12/info.Level)
				}
			} else {
				t := info.Level/5 + 4 + def*12/info.Level + def
				def = u16(ilvl/30 + 2 + t + (t&0xffff)*3/ilvl)
			}
			add := lvl * 3
			if idx == 0x1a04 {
				add = lvl * 2
			} else if wo, ok := d.wingOptions[idx]; ok {
				add = wo.DefenseConstA * lvl
			} else if cw, ok := d.customWing(idx); ok {
				add = cw.DefenseConstA * lvl
			}
			def = u16(def + add)
			if lvl > 9 {
				def = u16(def + (lvl-9)*(lvl-8)/2)
			}
		} else {
			def = u16(lvl + def)
			if s.Ancient != 0 && ilvl != 0 {
				def = u16(def*20/ilvl + 2 + def)
			}
		}
		s.Defense = def
	}
	if v := info.MagicDefense; v != 0 {
		s.MagicDefense = u16(levelBonus(v))
	}
	s.AttackSpeed = u8(info.AttackSpeed)
	s.WalkSpeed = u8(info.WalkSpeed)

	d.convertOptions(&s, info, it)

	if idx != 0x807 && idx != 0x80f {
		scale := func(v int) int { return int(math.RoundToEven(float64(float32(v) * s.DurFactor))) }
		s.DamageMin = u16(scale(s.DamageMin))
		s.DamageMax = u16(scale(s.DamageMax))
		s.Defense = u16(scale(s.Defense))
		s.DefenseSuccess = u8(scale(s.DefenseSuccess))
	}
	return s
}

// convertOptions fills the 14 option slots (FUN_0049d390 per slot, then the
// CCustomWing overrides of Convert).
func (d *statsData) convertOptions(s *itemStats, info gamedata.ItemInfo, it item.Item) {
	match := func(n int) (itemOption, bool) {
		var o itemOption
		ok := false
		for _, r := range d.options[n] {
			if r.Matches(s.Index, s.Skill, s.Luck, s.OptionLevel, s.Excellent) {
				o, ok = itemOption{r.OptionIndex, r.Value}, true
			}
		}
		return o, ok
	}
	if o, ok := match(0); ok {
		if s.Skill == 0 {
			s.Skill = 1
		}
		if info.Skill != 0 {
			o.Index = info.Skill
			if info.Skill == 0x42 {
				o.Index = 0
			}
		}
		s.SkillWeapon = info.Skill == 0x42
		s.Options[0] = o
	} else {
		s.Skill = 0
	}
	if o, ok := match(1); ok {
		if s.Luck == 0 {
			s.Luck = 1
		}
		s.Options[1] = o
	} else {
		s.Luck = 0
	}
	if o, ok := match(2); ok {
		lv := s.OptionLevel
		if lv == 0 {
			lv = 1
		}
		s.OptionLevel = lv
		o.Value = u16(o.Value * lv)
		s.Options[2] = o
		switch idx := s.Index; {
		case idx < 0x1800, idx >= 0x1801 && idx <= 0x1806, idx == 0x1829, idx == 0x182a, idx == 0x1831, idx == 0x1a1e:
			s.Req.Str += lv * 4
		}
	} else {
		s.OptionLevel = 0
	}
	for n := 3; n < 14; n++ {
		if o, ok := match(n); ok {
			s.Options[n] = o
		}
	}
	cw, ok := d.customWing(s.Index)
	if !ok {
		return
	}
	if it.Luck {
		s.Luck = 1
		s.Options[1] = itemOption{0x54, 5}
	}
	if it.Option != 0 {
		s.OptionLevel = it.Option
		s.Options[2] = itemOption{cw.Option[0][0], cw.Option[0][1]}
		if s.Excellent&0x10 != 0 {
			s.Options[2] = itemOption{cw.Option[1][0], cw.Option[1][1]}
		}
		if s.Excellent&0x20 != 0 {
			s.Options[2] = itemOption{cw.Option[2][0], cw.Option[2][1]}
		}
	}
	for i := 0; i < 4; i++ {
		if s.Excellent&(1<<i) != 0 {
			s.Options[3+i] = itemOption{cw.NewOption[i][0], cw.NewOption[i][1]}
		}
	}
}

// ---------------------------------------------------------------------------
// CalcCharacter

// statsEffects are the object fields CalcCharacter reads that belong to
// buffs, master skills and the CustomAttack system (all zero until those
// modules exist). Names give the original offsets.
type statsEffects struct {
	AddStats     [5]int // +0x730..+0x740: base of +0x174..+0x184
	AddDamage    int    // +0x2a0
	AddSpeed     int    // +0x298
	PhysiSpeed   int    // +0x710
	MagicSpeed   int    // +0x714
	PhysiSpeedP  [2]int // +0x7b4 / +0x7e4 (percent up)
	MagicSpeedP  [2]int // +0x7b8 / +0x7e8
	AddLife      int    // +0x720
	AddMana      int    // +0x724
	AddBP        int    // +0x728
	AddShield    int    // +0x72c
	LifeRate     [2]int // +0x7c4 up / +0x7f4 down
	ManaRate     [2]int // +0x7c8 / +0x7f8
	BPRate       [2]int // +0x7cc / +0x7fc
	ShieldRate   [2]int // +0x7d0 / +0x800
	ExpRate      int    // +0x75c (experience rate)
	PartyExpRate int    // +0x768
}

// statsState is the CalcCharacter output of a player (object fields
// +0x328..+0x370 and the option accumulators +0x82c..+0x940).
type statsState struct {
	Items [EquipSlots]itemStats
	Valid [EquipSlots]bool // CItem+0xb5 (requirements met)

	Effects statsEffects

	AddStr, AddDex, AddVit, AddEne, AddLead int // +0x174..+0x184
	AddShield                               int // +0x1c4
	AddBP                                   int // +0x190
	MaxBP                                   int // +0x18c

	HasWeapon bool // +0x584
	Skill42   bool // +0x5d8

	DamageMinRight, DamageMaxRight int // +0x344 / +0x340
	DamageMinLeft, DamageMaxLeft   int // +0x33c / +0x338
	MagicDamageMin, MagicDamageMax int // +0x328 / +0x32c
	CurseDamageMin, CurseDamageMax int // +0x330 / +0x334
	DKDamageMultiplier             int // +0x348
	DLDamageMultiplier             int // +0x34c
	AttackSuccessRate              int // +0x35c
	AttackSpeed, MagicSpeed        int // +0x360 / +0x364
	Defense                        int // +0x368
	DefenseSuccessRate             int // +0x370
	Resistance                     [7]int
	FullSet                        bool         // +0x82c
	Ancient                        ancientBonus // drop.go: set option fields (FUN_004f3240)

	// Option accumulators (FUN_0050bc70 reset, FUN_0049d4e0 codes).
	ItemDamageMin, ItemDamageMax int    // +0x320 / +0x324 (CustomAttribute)
	IncreaseDamage               int    // +0x834 (code 116)
	CriticalDamageRate           int    // +0x844 (code 84)
	ExcellentDamageRate          int    // +0x84c (code 92)
	IgnoreDefenseRate            int    // +0x83c (code 102)
	ExperienceRate               int    // +0x86c (100 + code 117)
	ItemDropRate                 int    // +0x870 (100 + code 117)
	MoneyAmountDropRate          int    // +0x878 (100 + code 86)
	HPRecoveryRate               int    // +0x88c (code 85)
	DamageReduction              [6]int // +0x8d8..+0x8ec (code 89 adds to the first)
	DamageReflect                int    // +0x8f0 (code 88)
	HuntHP, HuntMP               int    // +0x8f4 / +0x8f8 (codes 98, 99)
	Opt109, Opt110, Opt111       int    // +0x920 / +0x924 / +0x928
	CritFull, ExcFull            int    // +0x8c8 / +0x8cc (sent in F3 E1/E3)

	lastLevelSave time.Time     // obj+0x5dc (FUN_004c6670 save throttle)
	selfDefense   pkSelfDefense // obj+0x2c4 (pk.go)

	// Dark Spirit (FUN_0046b0c0, DL with 13,5 in the left hand).
	SpiritDamageMin, SpiritDamageMax, SpiritSpeed, SpiritRate int
}

func (w *World) statsOf(o *Object) *statsState {
	if o.P.Stats == nil {
		o.P.Stats = &statsState{}
	}
	return o.P.Stats
}

// resetOptions is FUN_0050bc70.
func (s *statsState) resetOptions() {
	s.FullSet = false
	s.IncreaseDamage, s.CriticalDamageRate, s.ExcellentDamageRate, s.IgnoreDefenseRate = 0, 0, 0, 0
	s.ExperienceRate, s.ItemDropRate, s.MoneyAmountDropRate = 100, 100, 100
	s.HPRecoveryRate = 0
	s.DamageReduction = [6]int{}
	s.DamageReflect, s.HuntHP, s.HuntMP = 0, 0, 0
	s.Opt109, s.Opt110, s.Opt111 = 0, 0, 0
	s.CritFull, s.ExcFull = 0, 0
	s.ItemDamageMin, s.ItemDamageMax = 0, 0
}

// equipSlot returns the converted item of an equipment slot.
func (w *World) convertEquipment(o *Object) [EquipSlots]itemStats {
	var out [EquipSlots]itemStats
	for i := 0; i < EquipSlots; i++ {
		it := o.P.Inventory[i]
		out[i].Index = -1
		if it.IsEmpty() {
			continue
		}
		info, ok := w.data.Items[it.Index]
		if !ok {
			out[i].Index = it.Index
			continue
		}
		out[i] = w.statsData.convertItem(info, it, it.PetLevel) // CItem+0xc4 (pet.go, U-STATS-03)
	}
	return out
}

func validIndex(i int) bool { return i >= 0 && i < 0x2000 }

// meetsRequirements is FUN_00496d00 (JoH requirement reductions are 0).
func (w *World) meetsRequirements(o *Object, s *statsState, it itemStats) bool {
	if !validIndex(it.Index) || it.Durability == 0 || !it.Known {
		return false
	}
	c := o.P.Char
	if it.Req.Level > c.Level || it.Req.Str > c.Strength+s.AddStr || it.Req.Dex > c.Dexterity+s.AddDex ||
		it.Req.Vit > c.Vitality+s.AddVit || it.Req.Ene > c.Energy+s.AddEne || it.Req.Lead > c.Leadership+s.AddLead {
		return false
	}
	info := w.data.Items[it.Index]
	return o.Class >= 0 && o.Class < 5 && info.Class[o.Class] != 0 // FUN_00494b50
}

// applyOptions is FUN_0049d470 / FUN_0049d4e0: pass 1 applies only code
// 105 (leadership), pass 0 everything else.
func (s *statsState) applyOptions(first bool, level, masterLevel int, maxLife, maxMana float32, addLife, addMana *int) {
	for slot := 0; slot < EquipSlots; slot++ {
		it := s.Items[slot]
		if !validIndex(it.Index) || !s.Valid[slot] || it.Durability == 0 {
			continue
		}
		for _, op := range it.Options {
			if (op.Index == 0x69) != first {
				continue
			}
			s.applyOption(op, it.Level, level+masterLevel, maxLife, maxMana, addLife, addMana)
		}
	}
}

func (s *statsState) applyOption(op itemOption, itemLevel, lvl int, maxLife, maxMana float32, addLife, addMana *int) {
	v := op.Value
	div := func() int {
		if v == 0 {
			return 0 // the original faults on a zero divisor; no row has one
		}
		return lvl / v
	}
	pct := func(x int) int { return x*v/100 + x }
	switch op.Index {
	case 80:
		s.DamageMinRight += v
		s.DamageMaxRight += v
		s.DamageMinLeft += v
		s.DamageMaxLeft += v
	case 81:
		s.MagicDamageMin += v
		s.MagicDamageMax += v
	case 82:
		s.DefenseSuccessRate += v
	case 83:
		s.Defense += v
	case 84:
		s.CriticalDamageRate += v
	case 85:
		s.HPRecoveryRate += v
	case 86:
		s.MoneyAmountDropRate += v
	case 87:
		s.DefenseSuccessRate = pct(s.DefenseSuccessRate)
	case 88:
		s.DamageReflect += v
	case 89:
		s.DamageReduction[0] += v
	case 90:
		*addMana += int(float64(maxMana) * float64(v) / 100)
	case 91:
		*addLife += int(float64(maxLife) * float64(v) / 100)
	case 92:
		s.ExcellentDamageRate += v
	case 93:
		d := div()
		s.DamageMinRight += d
		s.DamageMaxRight += d
		s.DamageMinLeft += d
		s.DamageMaxLeft += d
	case 94:
		s.DamageMinRight = pct(s.DamageMinRight)
		s.DamageMaxRight = pct(s.DamageMaxRight)
		s.DamageMinLeft = pct(s.DamageMinLeft)
		s.DamageMaxLeft = pct(s.DamageMaxLeft)
	case 95:
		d := div()
		s.MagicDamageMin += d
		s.MagicDamageMax += d
	case 96:
		s.MagicDamageMin = pct(s.MagicDamageMin)
		s.MagicDamageMax = pct(s.MagicDamageMax)
	case 97:
		s.AttackSpeed += v
		s.MagicSpeed += v
	case 98:
		s.HuntHP += v
	case 99:
		s.HuntMP += v
	case 100:
		*addLife += itemLevel*v + 50
	case 101:
		*addMana += itemLevel*v + 50
	case 102:
		s.IgnoreDefenseRate += v
	case 103:
		s.AddBP += v
	case 104:
		s.AddBP += s.MaxBP * v / 100
	case 105:
		s.AddLead += itemLevel*v + 10
	case 109:
		s.Opt109 += v
	case 110:
		s.Opt110 += v
	case 111:
		s.Opt111 += v
	case 113:
		s.CurseDamageMin += v
		s.CurseDamageMax += v
	case 114:
		d := div()
		s.CurseDamageMin += d
		s.CurseDamageMax += d
	case 115:
		s.CurseDamageMin = pct(s.CurseDamageMin)
		s.CurseDamageMax = pct(s.CurseDamageMax)
	case 116:
		s.IncreaseDamage += v
	case 117:
		s.ExperienceRate += v
		s.ItemDropRate += v
	case 118:
		// Applies 117 only with a 13,80 / 13,123 pet in slot 8 (FUN_0048ad60
		// validity check); those items do not exist in this Item.txt.
		if p := s.Items[8]; s.Valid[8] && (p.Index == 0x1a50 || p.Index == 0x1a7b) {
			s.ExperienceRate += v
			s.ItemDropRate += v
		}
	case 119:
		*addLife += v
	case 120:
		*addMana += v
	case 121:
		*addLife += div()
	case 122:
		*addMana += div()
	case 123:
		s.DamageMinRight += v
		s.DamageMaxRight += v
		s.DamageMinLeft += v
		s.DamageMaxLeft += v
		s.MagicDamageMin += v
		s.MagicDamageMax += v
		s.CurseDamageMin += v
		s.CurseDamageMax += v
	case 124:
		d := div()
		s.DamageMinRight += d
		s.DamageMaxRight += d
		s.DamageMinLeft += d
		s.DamageMaxLeft += d
		s.MagicDamageMin += d
		s.MagicDamageMax += d
		s.CurseDamageMin += d
		s.CurseDamageMax += d
	case 125:
		s.DamageMinRight = pct(s.DamageMinRight)
		s.DamageMaxRight = pct(s.DamageMaxRight)
		s.DamageMinLeft = pct(s.DamageMinLeft)
		s.DamageMaxLeft = pct(s.DamageMaxLeft)
		s.MagicDamageMin = pct(s.MagicDamageMin)
		s.MagicDamageMax = pct(s.MagicDamageMax)
		s.CurseDamageMin = pct(s.CurseDamageMin)
		s.CurseDamageMax = pct(s.CurseDamageMax)
	case 126:
		s.Defense += div()
	case 127:
		s.Defense = pct(s.Defense)
	}
}

// fullSetBonus reproduces the "full set" test and the +10..+15 defense
// bonus of CalcCharacter: helm, armor, pants, gloves and boots (slots 2..6)
// of the same type, all valid; MG skips the helm and class 6 the gloves
// (counted as +15).
func (s *statsState) fullSet(class int) (ok bool, counts [6]int) {
	typ := -1
	ok = false
	for slot := 2; slot < 7; slot++ {
		if (slot == 2 && class == 3) || (slot == 5 && class == 6) {
			continue
		}
		it := s.Items[slot]
		if !validIndex(it.Index) || !s.Valid[slot] {
			return false, counts
		}
		t := it.Index % 512
		if typ != -1 && t != typ {
			return false, counts
		}
		ok, typ = true, t
	}
	if !ok {
		return false, counts
	}
	count := func(slot int) {
		if l := s.Items[slot].Level; l >= 10 && l <= 15 {
			counts[l-10]++
		}
	}
	if class == 3 {
		counts[5]++
	} else {
		count(2)
	}
	count(3)
	count(4)
	if class == 6 {
		counts[5]++
	} else {
		count(5)
	}
	count(6)
	return true, counts
}

// calcCharacter is CalcCharacter (FUN_004c9790).
func (w *World) calcCharacter(o *Object) {
	if o.Type != TypePlayer || o.P == nil || o.P.Char == nil || w.statsData == nil {
		return
	}
	p, c, d := o.P, o.P.Char, w.statsData
	k := &d.k
	s := w.statsOf(o)
	cls := o.Class
	ci := d.classes[cls]

	// Current life/mana/shield/BP ratios, preserved through the recompute.
	pct := func(cur float32, add int, max float32) float32 {
		t := float32(add) + max
		if t == 0 {
			return 100
		}
		return float32(float64(cur) * 100 / float64(t))
	}
	lifePct := pct(c.Life, p.AddLife, c.MaxLife)
	manaPct := pct(c.Mana, p.AddMana, c.MaxMana)
	shieldPct := float32(100)
	if t := s.AddShield + int(c.MaxShield); t != 0 {
		shieldPct = float32(int(c.Shield)*100) / float32(t)
	}
	bpPct := float32(100)
	if t := s.AddBP + s.MaxBP; t != 0 {
		bpPct = float32(int(c.BP)*100) / float32(t)
	}

	s.Items = w.convertEquipment(o)
	r, l := s.Items[0], s.Items[1]
	s.HasWeapon = !((!validIndex(r.Index) && !validIndex(l.Index)) ||
		(!validIndex(l.Index) && r.Index == 0x80f) ||
		(!validIndex(r.Index) && (l.Index == 0x807 || (l.Index > 0xbff && l.Index < 0xe00))))
	for i := range s.Valid {
		s.Valid[i] = validIndex(s.Items[i].Index)
	}
	p.AddLife, p.AddMana, s.AddBP, s.AddShield = 0, 0, 0, 0
	s.Skill42 = false
	s.resetOptions()

	// Requirement loop: options and stat bonuses can make other items wearable.
	masterLevel := 0 // obj+0x5e4, only set by Lua in this build (U-COMBAT-05)
	for {
		e := s.Effects.AddStats
		s.AddStr, s.AddDex, s.AddVit, s.AddEne, s.AddLead = e[0], e[1], e[2], e[3], e[4]
		w.ancientItemStats(o, s) // FUN_004f2f70 (drop.go)
		s.applyOptions(true, c.Level, masterLevel, c.MaxLife, c.MaxMana, &p.AddLife, &p.AddMana)
		w.ancientSetOptions(o, s, true) // FUN_004f30f0(.., 1): stat options
		changed := false
		for i := 0; i < EquipSlots; i++ {
			if s.Valid[i] && !w.meetsRequirements(o, s, s.Items[i]) {
				s.Valid[i] = false
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	str := c.Strength + s.AddStr
	dex := c.Dexterity + s.AddDex
	vit := c.Vitality + s.AddVit
	ene := c.Energy + s.AddEne
	lead := c.Leadership + s.AddLead
	if cls < 0 || cls > 4 {
		return // classes 5/6 have no constants in this installation
	}

	// Base damage per class.
	switch cls {
	case 0, 1:
		s.DamageMinRight, s.DamageMaxRight = str/k.physMinA[cls], str/k.physMaxA[cls]
		s.DamageMinLeft, s.DamageMaxLeft = s.DamageMinRight, s.DamageMaxRight
	case 2:
		bow := func(i int) bool { return i >= 0x800 && i <= 0x9ff }
		if (bow(r.Index) && r.Index != 0x80f) || (bow(l.Index) && l.Index != 0x807) {
			s.DamageMinRight = dex/k.feBowMinB + str/k.feBowMinA
			s.DamageMaxRight = dex/k.feBowMaxB + str/k.feBowMaxA
		} else {
			s.DamageMinRight = (dex + str) / k.physMinA[2]
			s.DamageMaxRight = (dex + str) / k.physMaxA[2]
		}
		s.DamageMinLeft, s.DamageMaxLeft = s.DamageMinRight, s.DamageMaxRight
	case 3, 4:
		s.DamageMinRight = ene/k.physMinB[cls] + str/k.physMinA[cls]
		s.DamageMaxRight = ene/k.physMaxB[cls] + str/k.physMaxA[cls]
		s.DamageMinLeft, s.DamageMaxLeft = s.DamageMinRight, s.DamageMaxRight
	}
	s.MagicDamageMin, s.MagicDamageMax = ene/k.magicMin[cls], ene/k.magicMax[cls]
	s.CurseDamageMin, s.CurseDamageMax = s.MagicDamageMin, s.MagicDamageMax
	switch cls {
	case 1:
		s.DKDamageMultiplier = min(ene/k.dkMulA+200, k.dkMulMax)
	case 4:
		s.DLDamageMultiplier = min(ene/k.dlMulA+200, k.dlMulMax)
	}

	// Weapons: staffs (category 5) add half of their damage.
	hand := func(it itemStats, valid bool, minv, maxv *int) {
		if !validIndex(it.Index) {
			return
		}
		dmin, dmax := 0, 0
		if valid && it.Durability != 0 {
			dmin, dmax = it.DamageMin, it.DamageMax
		}
		if it.Index >= 0xa00 && it.Index <= 0xbff {
			dmin, dmax = int(uint32(dmin)/2), int(uint32(dmax)/2)
		}
		*minv += dmin
		*maxv += dmax
		if it.SkillWeapon {
			s.Skill42 = true
		}
	}
	hand(r, s.Valid[0], &s.DamageMinRight, &s.DamageMaxRight)
	hand(l, s.Valid[1], &s.DamageMinLeft, &s.DamageMaxLeft)
	add := s.Effects.AddDamage
	s.DamageMinRight += add
	s.DamageMaxRight += add
	s.DamageMinLeft += add
	s.DamageMaxLeft += add
	s.MagicDamageMin += add
	s.MagicDamageMax += add
	s.CurseDamageMin += add
	s.CurseDamageMax += add

	// Attack success rate (PvM).
	s.AttackSuccessRate = k.asrB[cls]*dex/k.asrC[cls] + str/k.asrD[cls] + (c.Level+masterLevel)*k.asrA[cls]
	if cls == 4 {
		s.AttackSuccessRate = k.asrB[4]*dex/k.asrC[4] + str/k.asrD[4] + lead/k.dlAsrE + (c.Level+masterLevel)*k.asrA[4]
	}

	// Speeds.
	ef := &s.Effects
	as := ef.PhysiSpeed + ef.AddSpeed + dex/k.physSpeed[cls]
	ms := ef.MagicSpeed + ef.AddSpeed + dex/k.magicSpeed[cls]
	as += ef.PhysiSpeedP[0] * as / 100
	ms += ef.MagicSpeedP[0] * ms / 100
	as += ef.PhysiSpeedP[1] * as / 100
	ms += ef.MagicSpeedP[1] * ms / 100
	isWeapon := func(it itemStats, valid bool) bool {
		return it.Index >= 0 && it.Index < 0xc00 && it.Index != 0x807 && it.Index != 0x80f && valid
	}
	rw, lw := isWeapon(r, s.Valid[0]), isWeapon(l, s.Valid[1])
	s.AttackSpeed, s.MagicSpeed = as, ms
	switch {
	case rw && lw:
		v := int(uint32(l.AttackSpeed+r.AttackSpeed) / 2)
		s.AttackSpeed, s.MagicSpeed = v+as, v+ms
	case rw:
		s.AttackSpeed, s.MagicSpeed = r.AttackSpeed+as, r.AttackSpeed+ms
	case lw:
		s.AttackSpeed, s.MagicSpeed = l.AttackSpeed+as, l.AttackSpeed+ms
	}
	for _, slot := range []int{5, 8, 9} {
		if it := s.Items[slot]; validIndex(it.Index) && s.Valid[slot] {
			s.AttackSpeed += it.AttackSpeed
			s.MagicSpeed += it.AttackSpeed
		}
	}

	// Defense success rate and defense: slots 1..7.
	s.DefenseSuccessRate = dex / k.dsr[cls]
	s.Defense = dex / k.defense[cls]
	for slot := 1; slot <= 7; slot++ {
		if it := s.Items[slot]; validIndex(it.Index) && s.Valid[slot] && it.Durability != 0 {
			s.DefenseSuccessRate += it.DefenseSuccess
			s.Defense += it.Defense
		}
	}
	full, counts := s.fullSet(cls)
	s.FullSet = full
	if full {
		s.DefenseSuccessRate = s.DefenseSuccessRate*10/100 + s.DefenseSuccessRate
	}
	if h := s.Items[8]; h.Index == 0x1a04 && s.Valid[8] && h.Durability != 0 {
		s.Defense = dex/20 + s.Defense + 5 + h.PetLevel*2
	}
	if full {
		c15, c14, c13, c12, c11, c10 := counts[5], counts[4], counts[3], counts[2], counts[1], counts[0]
		a14 := c15 + c14
		a13 := a14 + c13
		a12 := a13 + c12
		a11 := a12 + c11
		if c10+a11 > 4 {
			switch {
			case c15 == 5:
				s.Defense = s.Defense*30/100 + s.Defense
			case c14 == 5 || a14 == 5:
				s.Defense = s.Defense*25/100 + s.Defense
			case c13 == 5 || a13 == 5:
				s.Defense = s.Defense*20/100 + s.Defense
			case c12 == 5 || a12 == 5:
				s.Defense = s.Defense*15/100 + s.Defense
			case c11 == 5 || a11 == 5:
				s.Defense = s.Defense*10/100 + s.Defense
			case c10 == 5 || c10+a11 == 5:
				s.Defense = s.Defense*5/100 + s.Defense
			}
		}
	}

	w.calcDarkSpirit(o, s, lead)
	w.ravenSync(o, s) // the CDarkSpirit side of FUN_0046b0c0 (pet_raven.go)

	// Resistances: best of pendant and rings (no validity check).
	for i := range s.Resistance {
		a, b, cc := s.Items[9].Resist[i], s.Items[10].Resist[i], s.Items[11].Resist[i]
		s.Resistance[i] = max(a, b, cc)
	}

	// Experience bounds (same as FUN_0050be30).
	if floor := w.data.Experience.Floor(c.Level); uint32(c.Experience) < floor {
		c.Experience = int64(floor)
	}
	p.NextExp = w.data.Experience.Next(c.Level)

	s.applyOptions(false, c.Level, masterLevel, c.MaxLife, c.MaxMana, &p.AddLife, &p.AddMana)
	w.ancientSetOptions(o, s, false) // FUN_004f30f0(.., 0): other set options (drop.go)

	// Arrows / bolts level bonus.
	bowIdx := func(i int) bool { return i >= 0x800 && i <= 0x9ff }
	if !bowIdx(r.Index) || r.Index == 0x80f || r.Slot != 0 {
		if bowIdx(l.Index) && l.Index != 0x807 && l.Slot == 1 && r.Index == 0x80f && r.Level > 0 {
			m := r.Level*2 + 1
			v := m*s.DamageMinLeft/100 + 1 + s.DamageMinLeft
			s.DamageMinLeft = v
			s.DamageMaxLeft += m*v/100 + 1
		}
	} else if l.Index == 0x807 && l.Level > 0 {
		m := l.Level*2 + 1
		v := m*s.DamageMinRight/100 + 1 + s.DamageMinRight
		s.DamageMinRight = v
		s.DamageMaxRight += m*v/100 + 1
	}
	// Dual wield (DK, MG, DL).
	if (cls == 1 || cls == 3 || cls == 4) && validIndex(r.Index) && validIndex(l.Index) && r.Index < 0x800 && l.Index < 0x800 {
		s.DamageMinRight = s.DamageMinRight * 55 / 100
		s.DamageMaxRight = s.DamageMaxRight * 55 / 100
		s.DamageMinLeft = s.DamageMinLeft * 55 / 100
		s.DamageMaxLeft = s.DamageMaxLeft * 55 / 100
	}

	// Life and mana (U-COMBAT-03), float32 like obj+0x114 / +0x120.
	maxLife := float32(float64(ci.LevelLife)*float64(c.Level-1) + float64(ci.MaxLife))
	maxMana := float32(float64(ci.LevelMana)*float64(c.Level-1) + float64(ci.MaxMana))
	maxLife = float32(float64(c.Vitality+s.AddVit-ci.Vitality)*float64(ci.VitalityToLife) + float64(maxLife))
	maxMana = float32(float64(c.Energy+s.AddEne-ci.Energy)*float64(ci.EnergyToMana) + float64(maxMana))
	c.MaxLife, c.MaxMana = maxLife, maxMana
	s.MaxBP = maxBPFormula(cls, str, dex, vit, ene, lead)

	ftol := func(f float64) int { return int(f) }
	p.AddLife += ftol(float64(ef.LifeRate[0])*float64(maxLife))/100 - ftol(float64(ef.LifeRate[1])*float64(maxLife))/100 + ef.AddLife
	p.AddMana += ftol(float64(ef.ManaRate[0])*float64(maxMana))/100 - ftol(float64(ef.ManaRate[1])*float64(maxMana))/100 + ef.AddMana
	s.AddBP += ef.BPRate[0]*s.MaxBP/100 - ef.BPRate[1]*s.MaxBP/100 + ef.AddBP
	ms0 := int(c.MaxShield)
	s.AddShield += ef.ShieldRate[0]*ms0/100 - ef.ShieldRate[1]*ms0/100 + ef.AddShield

	// CustomAttribute.txt (FUN_00448070), every equipped item.
	for i := 0; i < EquipSlots; i++ {
		it := p.Inventory[i]
		if it.IsEmpty() {
			continue
		}
		a, ok := d.attributes[it.Index]
		if !ok {
			continue
		}
		if a.AttackSuccess > 0 {
			s.AttackSuccessRate += a.AttackSuccess
		}
		if a.Defense > 0 {
			s.Defense += a.Defense
		}
		if a.DefenseSuccess > 0 {
			s.DefenseSuccessRate += a.DefenseSuccess
		}
		if a.AddLife > 0 {
			p.AddLife += a.AddLife
		}
		if a.DamageReflect > 0 {
			s.DamageReflect += a.DamageReflect
		}
		if a.DamageMin > 0 {
			s.ItemDamageMin += a.DamageMin
			s.DamageMinRight += a.DamageMin
			s.DamageMinLeft += a.DamageMin
		}
		if a.DamageMax > 0 {
			s.ItemDamageMax += a.DamageMax
			s.DamageMaxRight += a.DamageMax
			s.DamageMaxLeft += a.DamageMax
		}
		if a.CriticalDamage > 0 {
			s.CriticalDamageRate += a.CriticalDamage
		}
		if a.ExcellentDamageRate > 0 {
			s.ExcellentDamageRate += a.ExcellentDamageRate
		}
	}

	// Restore the ratios on the new maximums.
	scale := func(pc float32, add int, max float32) float32 {
		t := float64(add) + float64(max)
		v := float32(float64(pc) * t / 100)
		if t < float64(v) {
			v = float32(t)
		}
		return v
	}
	c.Life = scale(lifePct, p.AddLife, maxLife)
	c.Mana = scale(manaPct, p.AddMana, maxMana)
	if t := s.AddShield + ms0; true {
		v := int(float64(shieldPct) * float64(t) / 100)
		c.Shield = float32(min(v, t))
	}
	if t := s.AddBP + s.MaxBP; true {
		v := int(float64(bpPct) * float64(t) / 100)
		c.BP = float32(min(v, t))
	}
	c.MaxBP = float32(s.MaxBP)
	p.AddBP = s.AddBP

	w.send(o, w.packetCharStats(o))
	w.send(o, w.packetCharOptions(o))
}

// maxBPFormula is FUN_004c9410 (x87 sums truncated by _ftol).
func maxBPFormula(cls, str, dex, vit, ene, lead int) int {
	f := func(v int, k float64) float64 { return float64(v) * k }
	var v float64
	switch cls {
	case 0:
		v = f(str, 0.2) + f(dex, 0.4) + f(vit, 0.3) + f(ene, 0.2)
	case 1:
		v = f(dex, 0.2) + f(str, 0.15) + f(vit, 0.3) + float64(ene)
	case 2:
		v = f(str, 0.3) + f(dex, 0.2) + f(vit, 0.3) + f(ene, 0.2)
	case 3:
		v = f(str, 0.2) + f(dex, 0.25) + f(vit, 0.3) + f(ene, 0.15)
	case 4:
		v = f(str, 0.3) + f(dex, 0.2) + f(vit, 0.1) + f(ene, 0.15) + f(lead, 0.3)
	}
	return int(v)
}

// calcDarkSpirit is FUN_0046b0c0 for a Dark Lord with the Dark Raven
// (13,5) in the left hand.
func (w *World) calcDarkSpirit(o *Object, s *statsState, lead int) {
	s.SpiritDamageMin, s.SpiritDamageMax, s.SpiritSpeed, s.SpiritRate = 0, 0, 0, 0
	l := s.Items[1]
	if o.Class != 4 || l.Index != 0x1a05 || !s.Valid[1] {
		return
	}
	k := &w.statsData.k
	sp := k.spirit
	if sp[2] == 0 || sp[5] == 0 || sp[8] == 0 || sp[9] == 0 || k.spiritRate[2] == 0 {
		return
	}
	pl := l.PetLevel
	s.SpiritDamageMin = lead/sp[2] + sp[1]*pl + sp[0]
	s.SpiritDamageMax = lead/sp[5] + sp[4]*pl + sp[3]
	s.SpiritSpeed = sp[7]*pl/sp[8] + sp[6] + lead/sp[9]
	s.SpiritRate = k.spiritRate[0] + k.spiritRate[1]*pl/k.spiritRate[2]
}

// playerCombatStats exposes the CalcCharacter results to the combat module.
func (w *World) playerCombatStats(o *Object) CombatStats {
	if o.P == nil {
		return CombatStats{}
	}
	if o.P.Stats == nil {
		w.calcCharacter(o)
	}
	s := w.statsOf(o)
	return CombatStats{
		DamageMinRight: s.DamageMinRight, DamageMaxRight: s.DamageMaxRight,
		DamageMinLeft: s.DamageMinLeft, DamageMaxLeft: s.DamageMaxLeft,
		MagicDamageMin: s.MagicDamageMin, MagicDamageMax: s.MagicDamageMax,
		Defense: s.Defense, MagicDefense: 0,
		AttackSuccessRate: s.AttackSuccessRate, DefenseSuccessRate: s.DefenseSuccessRate,
		AttackSpeed: s.AttackSpeed, MagicSpeed: s.MagicSpeed,
		CriticalDamage: s.CriticalDamageRate, ExcellentDamage: s.ExcellentDamageRate,
		CriticalDamageBonus: s.Ancient.CriticalDamage, ExcellentDamageBonus: s.Ancient.ExcellentDamage,
	}
}

// ---------------------------------------------------------------------------
// Packets

// maxFruit returns the CFruit limit shown in F3 03/F3 05/F3 E1.
func maxFruit(class, level int) int {
	if level < 1 || level > 1000 {
		return 0
	}
	return fruitLimit(class, level)
}

// packetCharStats is the WizardTeam C1 CC F3 E1 (FUN_004dc150) sent at the
// end of every CalcCharacter. Buff/master-skill fields are zero.
func (w *World) packetCharStats(o *Object) []byte {
	p, c, s := o.P, o.P.Char, w.statsOf(o)
	ef := &s.Effects
	b := newPacket(frame.C1, 0xF3, 0xE1, 0xCC)
	put := func(off, v int) { u32le(b[off:], uint32(int32(v))) }
	put(4, c.LevelUpPoints)
	put(8, int(c.Life))
	put(12, int(float64(p.AddLife)+float64(c.MaxLife)))
	put(16, int(c.Mana))
	put(20, int(float64(p.AddMana)+float64(c.MaxMana)))
	put(24, int(c.BP))
	put(28, s.MaxBP+s.AddBP)
	put(32, c.Strength)
	put(36, c.Dexterity)
	put(40, c.Vitality)
	put(44, c.Energy)
	put(48, c.Leadership)
	put(52, s.AddStr)
	put(56, s.AddDex)
	put(60, s.AddVit)
	put(64, s.AddEne)
	put(68, s.AddLead)
	// FUN_00410500: physical damage shown (both hands for dual wield).
	r, l := s.Items[0], s.Items[1]
	var pmin, pmax int
	if (o.Class == 1 || o.Class == 3 || o.Class == 4 || o.Class == 6) && validIndex(r.Index) && r.Index < 0x800 &&
		validIndex(l.Index) && l.Index < 0x800 && s.Valid[0] && s.Valid[1] {
		pmin, pmax = s.DamageMinRight+s.DamageMinLeft, s.DamageMaxRight+s.DamageMaxLeft
	} else if (r.Index < 0 || r.Index > 0x7ff) && (r.Index < 0xa00 || r.Index > 0xbff) &&
		((r.Index < 0x800 || r.Index > 0x9ff) || r.Index == 0x80f || r.Slot != 0) {
		pmin, pmax = s.DamageMinLeft, s.DamageMaxLeft
	} else {
		pmin, pmax = s.DamageMinRight, s.DamageMaxRight
	}
	put(72, pmin)
	put(76, pmax)
	put(80, s.MagicDamageMin)
	put(84, s.MagicDamageMax)
	put(88, s.CurseDamageMin)
	put(92, s.CurseDamageMax)
	// 96..124: buff rates (+0x7a4.. +0x7dc) and the staff / book rise.
	rise := func(it itemStats, valid bool) int {
		return int(math.RoundToEven(float64(float32(int(uint32(it.MagicPower)>>1)+it.Level*2) * it.DurFactor)))
	}
	if validIndex(r.Index) && s.Valid[0] && (r.Index < 0x200 || (r.Index > 0x9ff && r.Index < 0xc00)) {
		put(120, rise(r, true))
	}
	if validIndex(l.Index) && s.Valid[1] && l.Index > 0xa14 && l.Index < 0xa18 {
		put(124, rise(l, true))
	}
	put(128, s.AttackSpeed)
	put(132, s.MagicSpeed)
	put(136, c.FruitAddPoints)
	put(140, maxFruit(o.Class, c.Level))
	put(144, c.FruitSubPoints)
	put(148, maxFruit(o.Class, c.Level))
	put(152, s.AttackSuccessRate)
	put(156, s.CritFull)
	put(160, s.Defense)
	put(164, s.DefenseSuccessRate)
	put(168, s.ExcFull)
	mul := 200
	switch o.Class {
	case 1:
		mul = s.DKDamageMultiplier
	case 4:
		mul = s.DLDamageMultiplier
	}
	put(172, mul)
	put(176, 100)
	put(180, 100)
	put(184, 100)
	// Dark Spirit (FUN_0046c9d0): a scepter in the right hand raises the
	// raven damage by its magic rise.
	smin, smax := s.SpiritDamageMin, s.SpiritDamageMax
	if validIndex(r.Index) && s.Valid[0] && r.MagicPower != 0 && r.Index >= 0x400 && r.Index < 0x600 {
		pc := int(float64(float32(int(uint32(r.MagicPower)>>1)+r.Level*2)) * float64(r.DurFactor))
		smin = int(uint32(smin*pc)/100) + smin
		smax = int(uint32(smax*pc)/100) + smax
	}
	put(188, smin)
	put(192, smax)
	put(196, s.SpiritSpeed)
	put(200, s.SpiritRate)
	_ = ef
	return b
}

// packetCharOptions is the WizardTeam C1 3C F3 E3 (FUN_004dcc20).
func (w *World) packetCharOptions(o *Object) []byte {
	s := w.statsOf(o)
	k := &w.statsData.k
	cls := min(max(o.Class, 0), 4)
	b := newPacket(frame.C1, 0xF3, 0xE3, 0x3C)
	put := func(off, v int) { u32le(b[off:], uint32(int32(v))) }
	put(4, s.DamageReflect)
	put(8, s.CriticalDamageRate)
	put(12, k.damageRatePvM[cls]*s.CriticalDamageRate)
	put(16, k.damageRatePvP[cls]*s.CriticalDamageRate)
	put(20, s.ExcellentDamageRate)
	put(24, k.damageRatePvM[cls]*s.ExcellentDamageRate)
	put(28, k.damageRatePvP[cls]*s.ExcellentDamageRate)
	put(32, s.IncreaseDamage)
	put(36, 0) // +0x838
	put(40, s.IgnoreDefenseRate)
	put(44, s.CritFull)
	put(48, s.ExcFull)
	put(52, s.MoneyAmountDropRate)
	sum := 0
	for _, v := range s.DamageReduction {
		sum += v
	}
	put(56, sum)
	return b
}

// ---------------------------------------------------------------------------
// F3 06: add a stat point (FUN_004d98f0 -> FUN_004c68f0).

// addStatPoint is FUN_004c68f0: false when the type is unknown, there are
// no points or the stat reached MaxStatPoint.
func (w *World) addStatPoint(o *Object, typ, n int) bool {
	c := o.P.Char
	if o.Type != TypePlayer || n > c.LevelUpPoints {
		return false
	}
	var v *int
	switch typ {
	case 0:
		v = &c.Strength
	case 1:
		v = &c.Dexterity
	case 2:
		v = &c.Vitality
	case 3:
		v = &c.Energy
	case 4:
		v = &c.Leadership
	default:
		return false
	}
	maxStat := w.cfg.ALInt("Common", "MaxStatPoint", o.P.Account.Level, 0)
	if *v+n > maxStat {
		w.systemMessage(o, w.messageFor(o, 0x10c, maxStat))
		return false
	}
	*v += n
	c.LevelUpPoints -= n
	w.calcCharacter(o)
	return true
}

// addLevelUpPoints gives free points (quest rewards, commands).
func (w *World) addLevelUpPoints(o *Object, n int) {
	if o.P != nil && o.P.Char != nil {
		o.P.Char.LevelUpPoints += n
	}
}

// handleStatAdd is the F3 06 handler (FUN_004d98f0); the request is
// C1 05 F3 06 type.
func (w *World) handleStatAdd(o *Object, pkt []byte) {
	if len(pkt) < 5 || o.P.LogoutCountdown > 0 {
		return
	}
	typ := int(pkt[4])
	b := newPacket(frame.C1, 0xF3, 0x06, 0x30)
	if w.addStatPoint(o, typ, 1) {
		p, c, s := o.P, o.P.Char, w.statsOf(o)
		b[4] = byte(typ + 0x10)
		maxLife := int(math.RoundToEven(float64(float32(p.AddLife) + c.MaxLife)))
		maxMana := int(math.RoundToEven(float64(float32(p.AddMana) + c.MaxMana)))
		switch typ {
		case 2:
			u16le(b[6:], clamp65000(maxLife))
		case 3:
			u16le(b[6:], clamp65000(maxMana))
		}
		u16le(b[8:], clamp65000(s.AddBP+s.MaxBP))
		for i, v := range []int{c.LevelUpPoints, maxLife, maxMana, s.AddBP + s.MaxBP,
			c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership} {
			u32le(b[12+4*i:], uint32(int32(v)))
		}
	}
	w.send(o, b)
}
