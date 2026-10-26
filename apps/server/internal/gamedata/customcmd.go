package gamedata

import (
	"fmt"
)

// Data of the WizardTeam commands 50..65 (docs/server-plan/
// 10e-customs-wizardteam.md): the CustomAttack, CustomGift, CustomMarriage,
// CustomBuyVip and CustomPick tables of config/data/custom.toml.

// AttackSpeedRow is a section 0 row of CustomAttack.txt (CCustomAttack
// +0x04 vector, 8 ints read by FUN_00445480): the delay between two casts
// of the auto attack (FUN_004457f0). -1 is "*".
type AttackSpeedRow struct {
	Skill                      int
	MinDexterity, MaxDexterity int
	MinSpeed, MaxSpeed         int
	AttackDelay                int // ms; -1 uses BaseSpeed - Speed*MainSpeed
	BaseSpeed, MainSpeed       int
}

// AttackSkill is a section 1 row of CustomAttack.txt (std::map at +0x14
// keyed by Index, FUN_00468490 keeps the first row): Group 0 is a buff
// (Value 0 self, otherwise the party too), 1 a single target skill (0x19),
// 2 an area magic with targets (0x1E + 0x10), 3 an area skill (0x1E),
// 4 a special single target. Class[0..4] are DW DK FE MG DL.
type AttackSkill struct {
	Index, Group, Value int
	Class               [5]int
}

// CustomAttack is CustomAttack.txt.
type CustomAttack struct {
	Speeds []AttackSpeedRow
	Skills map[int]AttackSkill
	Order  []int // map order (ascending Index), the iteration order of the buff loop
}

// LoadCustomAttack reads the CustomAttack table of config/data/custom.toml.
// Section 1 indexes outside 0..213 are ignored like in FUN_00445480; the
// first row of an index wins.
func LoadCustomAttack(configDir string) (*CustomAttack, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	a := &CustomAttack{Skills: map[int]AttackSkill{}}
	for _, r := range f.CustomAttack.Speed {
		a.Speeds = append(a.Speeds, AttackSpeedRow{r.SkillIndex, r.MinDexterity, r.MaxDexterity, r.MinCharSpeed,
			r.MaxCharSpeed, r.AttackDelay, r.BaseSpeed, r.MainSpeed})
	}
	for _, r := range f.CustomAttack.Skill {
		if r.Index < 0 || r.Index >= 0xD6 {
			continue
		}
		if _, ok := a.Skills[r.Index]; ok {
			continue
		}
		a.Skills[r.Index] = AttackSkill{Index: r.Index, Group: r.Group, Value: r.Value,
			Class: [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}}
	}
	for i := 0; i < 0xD6; i++ {
		if _, ok := a.Skills[i]; ok {
			a.Order = append(a.Order, i)
		}
	}
	return a, nil
}

// AttackDelay is FUN_004457f0: the first speed row whose skill, dexterity
// and speed ranges match gives AttackDelay, or BaseSpeed - MainSpeed*speed
// when AttackDelay is -1; without a row the delay is (375 - speed) * 4.
func (a *CustomAttack) AttackDelay(skill, dexterity, speed int) int {
	if a != nil {
		for _, r := range a.Speeds {
			if (r.Skill == -1 || r.Skill == skill) &&
				(r.MinDexterity == -1 || r.MinDexterity <= dexterity) &&
				(r.MaxDexterity == -1 || dexterity <= r.MaxDexterity) &&
				(r.MinSpeed == -1 || r.MinSpeed <= speed) &&
				(r.MaxSpeed == -1 || speed <= r.MaxSpeed) {
				if r.AttackDelay == -1 {
					return r.BaseSpeed - r.MainSpeed*speed
				}
				return r.AttackDelay
			}
		}
	}
	return (0x177 - speed) * 4
}

// GiftItem is one item of a gift (section 1 of CustomGift.txt, 0x2c
// bytes): the reply creates it on the ground near the player.
type GiftItem struct {
	Index, Level, Durability int
	Skill, Luck, Option      int
	Excellent, SetOption     int
	Duration                 int // seconds, 0 = permanent
}

// Gift is a section 0 row of CustomGift.txt (the std::map node of
// FUN_00463150): GiftIndex Class AL0..AL3 MinLevel MaxLevel MinReset
// MaxReset MaxCount LevelUpPoint Money "Code". The three coin fields of the
// record (+0x44..+0x4c) have no column and stay 0.
type Gift struct {
	Index        int
	Class        int // -1 any, else the class (obj+0xe0)
	Switch       [4]int
	MinLevel     int
	MaxLevel     int
	MinReset     int
	MaxReset     int
	MaxCount     int // -1 unlimited (the DataServer compares Count < MaxCount)
	LevelUpPoint int
	Money        int
	Code         string
	Items        []GiftItem
}

// LoadCustomGift reads the CustomGift table of config/data/custom.toml in
// file order (the code lookup of FUN_00463780 walks the list and takes the
// first match). Items go to the last gift with their GiftIndex.
func LoadCustomGift(configDir string) ([]Gift, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var gifts []Gift
	byIndex := map[int]int{}
	for _, r := range f.CustomGift.Gift {
		byIndex[r.GiftIndex] = len(gifts)
		gifts = append(gifts, Gift{Index: r.GiftIndex, Class: r.Class, Switch: [4]int{r.AL0, r.AL1, r.AL2, r.AL3},
			MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, MaxCount: r.MaxCount,
			LevelUpPoint: r.LevelUpPoint, Money: r.Money, Code: cutString(r.Code, 0x1F)})
	}
	for i, r := range f.CustomGift.Item {
		it, err := customItem(r.CustomItemColumns)
		if err != nil {
			return nil, fmt.Errorf("CustomGift.Item row %d: %w", i+1, err)
		}
		if k, ok := byIndex[r.GiftIndex]; ok {
			gifts[k].Items = append(gifts[k].Items, it)
		}
	}
	return gifts, nil
}

// customItem converts the "cc,iii Level Dur Skill Luck Option Exc Set
// Duration" columns shared by CustomGift and CustomMarriage.
func customItem(c CustomItemColumns) (GiftItem, error) {
	idx, err := ParseItemRef(c.ItemIndex)
	if err != nil {
		return GiftItem{}, err
	}
	return GiftItem{Index: idx, Level: c.ItemLevel, Durability: c.Durability, Skill: c.SkillSwitch, Luck: c.LuckSwitch,
		Option: c.Aditional, Excellent: c.Excellent, SetOption: c.SetOption, Duration: c.Duration}, nil
}

// MarriageItem is the requirement of sections 0 (marry) and 1 (divorce)
// of CustomMarriage.txt: Enable ItemIndex ItemLevel ItemCount.
type MarriageItem struct {
	Enable, Index, Level, Count int
}

// CustomMarriage is CustomMarriage.txt (CCustomMarriage +0x28
// marry item, +0x38 divorce item, +0x48 the reward items).
type CustomMarriage struct {
	Marry, Divorce MarriageItem
	Rewards        []GiftItem
}

// LoadCustomMarriage reads the CustomMarriage table of
// config/data/custom.toml (the last marry / divorce row wins).
func LoadCustomMarriage(configDir string) (*CustomMarriage, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := f.CustomMarriage
	m := &CustomMarriage{}
	for k, rows := range [2][]CustomMarriageItemRow{t.Marry, t.Divorce} {
		for i, r := range rows {
			idx, err := ParseItemRef(r.ItemIndex)
			if err != nil {
				return nil, fmt.Errorf("CustomMarriage row %d: %w", i+1, err)
			}
			it := MarriageItem{Enable: r.Enable, Index: idx, Level: r.ItemLevel, Count: r.ItemCount}
			if k == 0 {
				m.Marry = it
			} else {
				m.Divorce = it
			}
		}
	}
	for i, r := range t.Reward {
		it, err := customItem(r.CustomItemColumns)
		if err != nil {
			return nil, fmt.Errorf("CustomMarriage.Reward row %d: %w", i+1, err)
		}
		m.Rewards = append(m.Rewards, it)
	}
	return m, nil
}

// VipPlan is a CustomBuyVip.txt row (std::map keyed by Code, FUN_00448e00):
// AccountLevel (-1 any, else the minimum current level) NewAccountLevel
// Days Money "Code". The three coin prices (+0x38..+0x40) have no column.
type VipPlan struct {
	AccountLevel, NewAccountLevel, Days, Money int
	Code                                       string
}

// LoadCustomBuyVip reads the CustomBuyVip table of config/data/custom.toml
// (first code wins).
func LoadCustomBuyVip(configDir string) (map[string]VipPlan, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[string]VipPlan{}
	for _, r := range f.CustomBuyVip.Rows {
		p := VipPlan{AccountLevel: r.AccountLevel, NewAccountLevel: r.NewAccountLevel, Days: r.Days, Money: r.Money,
			Code: cutString(r.Code, 0x1F)}
		if _, ok := out[p.Code]; !ok {
			out[p.Code] = p
		}
	}
	return out, nil
}

// PickBlock is a CustomPick.txt row (CCustomPick +0x1c0 vector of 3 ints,
// FUN_00456660): an item the auto pick never takes for accounts whose
// level is at most AccountLevel (-1 all); Level -1 is any level.
type PickBlock struct {
	Index, Level, AccountLevel int
}

// LoadCustomPick reads the CustomPick table of config/data/custom.toml.
func LoadCustomPick(configDir string) ([]PickBlock, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []PickBlock
	for i, r := range f.CustomPick.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomPick row %d: %w", i+1, err)
		}
		out = append(out, PickBlock{Index: idx, Level: r.ItemLevel, AccountLevel: r.AccountLevel})
	}
	return out, nil
}
