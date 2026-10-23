package gamedata

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Tables used by CalcCharacter (GameServer FUN_004c9790), CItem::Convert
// (FUN_0048aed0) and the experience formulas (FUN_004c5a30/FUN_004c5ea0).

// ClassInfo is a Character/DefaultClassInfo.txt row (CDefaultClassInfo,
// 0x30-byte records at 0x12418a4): base stats and the float life/mana
// coefficients CalcCharacter reads (+0x18 MaxLife, +0x1c MaxMana,
// +0x20 LevelLife, +0x24 LevelMana, +0x28 VitalityToLife, +0x2c
// EnergyToMana; +0x0c/+0x10 are the Vitality/Energy bases).
type ClassInfo struct {
	Class                                       int
	Strength, Dexterity, Vitality, Energy, Lead int
	MaxLife, MaxMana                            float32
	LevelLife, LevelMana                        float32
	VitalityToLife, EnergyToMana                float32
}

// LoadClassInfo reads Data/Character/DefaultClassInfo.txt.
func LoadClassInfo(dataDir string) (map[int]ClassInfo, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Character", "DefaultClassInfo.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]ClassInfo{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 12 {
			return nil, fmt.Errorf("DefaultClassInfo.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var iv [6]int
		for i := range iv {
			if iv[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		var fv [6]float32
		for i := range fv {
			f, err := strconv.ParseFloat(l.Tokens[6+i], 32)
			if err != nil {
				return nil, fmt.Errorf("DefaultClassInfo.txt line %d column %d: %w", l.No, 7+i, err)
			}
			fv[i] = float32(f)
		}
		out[iv[0]] = ClassInfo{Class: iv[0], Strength: iv[1], Dexterity: iv[2], Vitality: iv[3], Energy: iv[4],
			Lead: iv[5], MaxLife: fv[0], MaxMana: fv[1], LevelLife: fv[2], LevelMana: fv[3],
			VitalityToLife: fv[4], EnergyToMana: fv[5]}
	}
	return out, nil
}

// ItemOptionRule is an Item/ItemOption.txt row (CItemOption, FUN_0049d0b0).
// The table is keyed by Index (the CItem option slot 0..13); FUN_0049d390
// walks every rule of a slot and the last one that matches wins.
// -1 ("*") means "any".
type ItemOptionRule struct {
	Index       int // option slot 0..13
	OptionIndex int // effect code 80..127 (FUN_0049d4e0)
	Value       int
	ItemMin     int
	ItemMax     int
	Skill       int // minimum CItem+0x98
	Luck        int // minimum CItem+0x99
	Additional  int // minimum CItem+0x9a
	Excellent   int // mask tested against CItem+0x9b
}

// Matches is the rule test of FUN_0049d390.
func (r ItemOptionRule) Matches(index, skill, luck, option int, excellent byte) bool {
	return (r.ItemMin == -1 || r.ItemMin <= index) && (r.ItemMax == -1 || index <= r.ItemMax) &&
		(r.Skill == -1 || r.Skill <= skill) && (r.Luck == -1 || r.Luck <= luck) &&
		(r.Additional == -1 || r.Additional <= option) &&
		(r.Excellent == -1 || int(excellent)&r.Excellent != 0)
}

// LoadItemOptions reads Data/Item/ItemOption.txt grouped by option slot.
func LoadItemOptions(dataDir string) (map[int][]ItemOptionRule, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "ItemOption.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int][]ItemOptionRule{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 9 {
			return nil, fmt.Errorf("ItemOption.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var v [9]int
		for i := range v {
			if i == 3 || i == 4 {
				if v[i], err = ParseItemRef(l.Tokens[i]); err != nil {
					return nil, fmt.Errorf("ItemOption.txt line %d: %w", l.No, err)
				}
				if v[i] >= 0x2000 {
					v[i] = -1
				}
				continue
			}
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		r := ItemOptionRule{Index: v[0], OptionIndex: v[1], Value: v[2], ItemMin: v[3], ItemMax: v[4],
			Skill: v[5], Luck: v[6], Additional: v[7], Excellent: v[8]}
		out[r.Index] = append(out[r.Index], r)
	}
	return out, nil
}

// WingOption is an Item/WingOption.txt row (CWingOption).
type WingOption struct {
	ItemIndex       int
	DefenseConstA   int // defense per item level (CItem::Convert)
	IncDamageConstA int
	IncDamageConstB int
	DecDamageConstA int
	DecDamageConstB int
	MaxNewOption    int
}

// LoadWingOptions reads Data/Item/WingOption.txt.
func LoadWingOptions(dataDir string) (map[int]WingOption, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "WingOption.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]WingOption{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 7 {
			return nil, fmt.Errorf("WingOption.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("WingOption.txt line %d: %w", l.No, err)
		}
		var v [6]int
		for i := range v {
			if v[i], err = l.Int(1 + i); err != nil {
				return nil, err
			}
		}
		out[idx] = WingOption{ItemIndex: idx, DefenseConstA: v[0], IncDamageConstA: v[1], IncDamageConstB: v[2],
			DecDamageConstA: v[3], DecDamageConstB: v[4], MaxNewOption: v[5]}
	}
	return out, nil
}

// CustomWing is a Custom/CustomWing.txt row (CCustomWing, 100 slots).
type CustomWing struct {
	Index           int
	ItemIndex       int
	DefenseConstA   int
	IncDamageConstA int
	IncDamageConstB int
	DecDamageConstA int
	DecDamageConstB int
	Option          [3][2]int // OptionIndex/OptionValue 1..3
	NewOption       [4][2]int // NewOptionIndex/NewOptionValue 1..4
	ModelType       int
}

// LoadCustomWings reads Data/Custom/CustomWing.txt keyed by item index.
func LoadCustomWings(dataDir string) (map[int]CustomWing, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomWing.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]CustomWing{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 21 {
			return nil, fmt.Errorf("CustomWing.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var v [21]int
		for i := range v {
			if i == 1 {
				if v[i], err = ParseItemRef(l.Tokens[i]); err != nil {
					return nil, fmt.Errorf("CustomWing.txt line %d: %w", l.No, err)
				}
				continue
			}
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		if v[0] < 0 || v[0] >= 100 {
			continue
		}
		w := CustomWing{Index: v[0], ItemIndex: v[1], DefenseConstA: v[2], IncDamageConstA: v[3],
			IncDamageConstB: v[4], DecDamageConstA: v[5], DecDamageConstB: v[6]}
		for i := 0; i < 3; i++ {
			w.Option[i] = [2]int{v[7+2*i], v[8+2*i]}
		}
		for i := 0; i < 4; i++ {
			w.NewOption[i] = [2]int{v[13+2*i], v[14+2*i]}
		}
		if len(l.Tokens) > 21 {
			w.ModelType, _ = l.Int(21)
		}
		out[w.ItemIndex] = w
	}
	return out, nil
}

// CustomAttribute is a Custom/CustomAttribute.txt row (CItemAttribute,
// applied by FUN_00448070 for every equipped item).
type CustomAttribute struct {
	ItemIndex           int
	AttackSuccess       int
	Defense             int
	DefenseSuccess      int
	AddLife             int
	DamageReflect       int
	DamageMin           int
	DamageMax           int
	CriticalDamage      int
	ExcellentDamageRate int
}

// LoadCustomAttributes reads Data/Custom/CustomAttribute.txt.
func LoadCustomAttributes(dataDir string) (map[int]CustomAttribute, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomAttribute.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]CustomAttribute{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 10 {
			return nil, fmt.Errorf("CustomAttribute.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("CustomAttribute.txt line %d: %w", l.No, err)
		}
		var v [9]int
		for i := range v {
			if v[i], err = l.Int(1 + i); err != nil {
				return nil, err
			}
		}
		out[idx] = CustomAttribute{ItemIndex: idx, AttackSuccess: v[0], Defense: v[1], DefenseSuccess: v[2],
			AddLife: v[3], DamageReflect: v[4], DamageMin: v[5], DamageMax: v[6], CriticalDamage: v[7],
			ExcellentDamageRate: v[8]}
	}
	return out, nil
}

// CustomMonster is a Custom/CustomMonster.txt row (CCustomMonster); only
// ExperienceRate is used by the experience formulas (FUN_004670a0).
type CustomMonster struct {
	Monster        int // -1 = any
	Map            int // -1 = any
	ExperienceRate int // -1 = keep 100
}

// LoadCustomMonsters reads Data/Custom/CustomMonster.txt in file order.
func LoadCustomMonsters(dataDir string) ([]CustomMonster, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomMonster.txt"))
	if err != nil {
		return nil, err
	}
	var out []CustomMonster
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 9 {
			return nil, fmt.Errorf("CustomMonster.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var v [9]int
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		out = append(out, CustomMonster{Monster: v[0], Map: v[1], ExperienceRate: v[8]})
	}
	return out, nil
}

// CustomMonsterExperienceRate is FUN_004670a0: the first row whose monster
// and map match (-1 = any); 100 when none matches or its rate is "*".
func CustomMonsterExperienceRate(rows []CustomMonster, monster, mp int) int {
	for _, r := range rows {
		if (r.Monster != -1 && r.Monster != monster) || (r.Map != -1 && r.Map != mp) {
			continue
		}
		if r.ExperienceRate != -1 {
			return r.ExperienceRate
		}
		return 100
	}
	return 100
}
