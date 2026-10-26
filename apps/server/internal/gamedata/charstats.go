package gamedata

import (
	"fmt"
)

// Tables used by CalcCharacter (GameServer FUN_004c9790), CItem::Convert
// (FUN_0048aed0) and the experience formulas (FUN_004c5a30/FUN_004c5ea0).

// ClassInfo is a DefaultClassInfo.txt row (CDefaultClassInfo,
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

// LoadClassInfo reads the DefaultClassInfo table of
// config/data/character.toml.
func LoadClassInfo(configDir string) (map[int]ClassInfo, error) {
	f, err := LoadCharacterConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]ClassInfo{}
	for _, r := range f.DefaultClassInfo.Rows {
		out[r.Class] = ClassInfo{Class: r.Class, Strength: r.Strength, Dexterity: r.Dexterity, Vitality: r.Vitality,
			Energy: r.Energy, Lead: r.Leadership, MaxLife: float32(r.MaxLife), MaxMana: float32(r.MaxMana),
			LevelLife: float32(r.LevelLife), LevelMana: float32(r.LevelMana), VitalityToLife: float32(r.VitalityToLife),
			EnergyToMana: float32(r.EnergyToMana)}
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

// LoadItemOptions reads the ItemOption table of config/data/item.toml,
// grouped by option slot.
func LoadItemOptions(configDir string) (map[int][]ItemOptionRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int][]ItemOptionRule{}
	for i, row := range f.ItemOption.Rows {
		var rng [2]int
		for k, ref := range [2]string{row.ItemIndexMin, row.ItemIndexMax} {
			if rng[k], err = ParseItemRef(ref); err != nil {
				return nil, fmt.Errorf("ItemOption row %d: %w", i+1, err)
			}
		}
		r := ItemOptionRule{Index: row.Index, OptionIndex: row.OptionIndex, Value: row.OptionValue,
			ItemMin: rng[0], ItemMax: rng[1], Skill: row.SkillSwitch, Luck: row.LuckSwitch,
			Additional: row.Aditional, Excellent: row.Excellent}
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

// LoadWingOptions reads the WingOption table of config/data/item.toml.
func LoadWingOptions(configDir string) (map[int]WingOption, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]WingOption{}
	for i, r := range f.WingOption.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("WingOption row %d: %w", i+1, err)
		}
		out[idx] = WingOption{ItemIndex: idx, DefenseConstA: r.DefenseConstA, IncDamageConstA: r.IncDamageConstA,
			IncDamageConstB: r.IncDamageConstB, DecDamageConstA: r.DecDamageConstA, DecDamageConstB: r.DecDamageConstB,
			MaxNewOption: r.MaxNewOption}
	}
	return out, nil
}

// CustomWing is a CustomWing.txt row (CCustomWing, 100 slots).
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

// LoadCustomWings reads the CustomWing table of config/data/custom.toml
// keyed by item index (Index outside 0..99 is ignored).
func LoadCustomWings(configDir string) (map[int]CustomWing, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]CustomWing{}
	for i, r := range f.CustomWing.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomWing row %d: %w", i+1, err)
		}
		if r.Index < 0 || r.Index >= 100 {
			continue
		}
		out[idx] = CustomWing{Index: r.Index, ItemIndex: idx, DefenseConstA: r.DefenseConstA,
			IncDamageConstA: r.IncDamageConstA, IncDamageConstB: r.IncDamageConstB, DecDamageConstA: r.DecDamageConstA,
			DecDamageConstB: r.DecDamageConstB,
			Option: [3][2]int{{r.OptionIndex1, r.OptionValue1}, {r.OptionIndex2, r.OptionValue2},
				{r.OptionIndex3, r.OptionValue3}},
			NewOption: [4][2]int{{r.NewOptionIndex1, r.NewOptionValue1}, {r.NewOptionIndex2, r.NewOptionValue2},
				{r.NewOptionIndex3, r.NewOptionValue3}, {r.NewOptionIndex4, r.NewOptionValue4}},
			ModelType: r.ModelType}
	}
	return out, nil
}

// CustomAttribute is a CustomAttribute.txt row (CItemAttribute,
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

// LoadCustomAttributes reads the CustomAttribute table of
// config/data/custom.toml.
func LoadCustomAttributes(configDir string) (map[int]CustomAttribute, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]CustomAttribute{}
	for i, r := range f.CustomAttribute.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomAttribute row %d: %w", i+1, err)
		}
		out[idx] = CustomAttribute{ItemIndex: idx, AttackSuccess: r.AttackSuccess, Defense: r.Defense,
			DefenseSuccess: r.SucessDefense, AddLife: r.AddLife, DamageReflect: r.DamageReflect, DamageMin: r.DamageMin,
			DamageMax: r.DamageMax, CriticalDamage: r.CriticalDamage, ExcellentDamageRate: r.ExcellentDamageRate}
	}
	return out, nil
}

// CustomMonster is a CustomMonster.txt row (CCustomMonster); only
// ExperienceRate is used by the experience formulas (FUN_004670a0).
type CustomMonster struct {
	Monster        int // -1 = any
	Map            int // -1 = any
	ExperienceRate int // -1 = keep 100
}

// LoadCustomMonsters reads the CustomMonster table of
// config/data/custom.toml in file order.
func LoadCustomMonsters(configDir string) ([]CustomMonster, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []CustomMonster
	for _, r := range f.CustomMonster.Rows {
		out = append(out, CustomMonster{Monster: r.MonsterIndex, Map: r.MapNumber, ExperienceRate: r.ExperienceRate})
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
