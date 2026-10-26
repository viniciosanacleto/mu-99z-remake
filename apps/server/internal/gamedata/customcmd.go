package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Data files of the custom commands 50..65 (docs/server-plan/
// 10e-customs.md): Custom/CustomAttack.txt, CustomGift.txt,
// CustomMarriage.txt, CustomBuyVip.txt and CustomPick.txt.

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

// CustomAttack is Data/Custom/CustomAttack.txt.
type CustomAttack struct {
	Speeds []AttackSpeedRow
	Skills map[int]AttackSkill
	Order  []int // map order (ascending Index), the iteration order of the buff loop
}

// LoadCustomAttack reads Data/Custom/CustomAttack.txt. Section 1 indexes
// outside 0..213 are ignored like in FUN_00445480.
func LoadCustomAttack(dataDir string) (*CustomAttack, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Custom", "CustomAttack.txt"))
	if err != nil {
		return nil, err
	}
	a := &CustomAttack{Skills: map[int]AttackSkill{}}
	for _, s := range secs {
		for _, l := range s.Rows {
			v, err := rowInts(l, 8)
			if err != nil {
				return nil, fmt.Errorf("CustomAttack.txt: %w", err)
			}
			switch s.Index {
			case 0:
				a.Speeds = append(a.Speeds, AttackSpeedRow{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7]})
			case 1:
				if v[0] < 0 || v[0] >= 0xD6 {
					continue
				}
				if _, ok := a.Skills[v[0]]; ok {
					continue
				}
				sk := AttackSkill{Index: v[0], Group: v[1], Value: v[2]}
				copy(sk.Class[:], v[3:8])
				a.Skills[v[0]] = sk
			}
		}
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

// LoadCustomGift reads Data/Custom/CustomGift.txt in file order (the code
// lookup of FUN_00463780 walks the list and takes the first match).
func LoadCustomGift(dataDir string) ([]Gift, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Custom", "CustomGift.txt"))
	if err != nil {
		return nil, err
	}
	var gifts []Gift
	byIndex := map[int]int{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := rowInts(l, 13)
				if err != nil {
					return nil, fmt.Errorf("CustomGift.txt: %w", err)
				}
				if len(l.Tokens) < 14 {
					return nil, fmt.Errorf("CustomGift.txt line %d: missing code", l.No)
				}
				g := Gift{Index: v[0], Class: v[1], MinLevel: v[6], MaxLevel: v[7], MinReset: v[8],
					MaxReset: v[9], MaxCount: v[10], LevelUpPoint: v[11], Money: v[12], Code: cutString(l.Tokens[13], 0x1F)}
				copy(g.Switch[:], v[2:6])
				byIndex[g.Index] = len(gifts)
				gifts = append(gifts, g)
			case 1:
				it, gi, err := giftItemRow(l)
				if err != nil {
					return nil, err
				}
				if k, ok := byIndex[gi]; ok {
					gifts[k].Items = append(gifts[k].Items, it)
				}
			}
		}
	}
	return gifts, nil
}

// giftItemRow parses "Index cc,iii Level Dur Skill Luck Option Exc Set
// Duration" (the item columns shared by CustomGift and CustomMarriage).
func giftItemRow(l mutext.Line) (GiftItem, int, error) {
	if len(l.Tokens) < 10 {
		return GiftItem{}, 0, fmt.Errorf("line %d: %d columns", l.No, len(l.Tokens))
	}
	gi, err := l.Int(0)
	if err != nil {
		return GiftItem{}, 0, err
	}
	it, err := customItemColumns(mutext.Line{No: l.No, Tokens: l.Tokens[1:]})
	return it, gi, err
}

// customItemColumns parses "cc,iii Level Dur Skill Luck Option Exc Set Duration".
func customItemColumns(l mutext.Line) (GiftItem, error) {
	if len(l.Tokens) < 9 {
		return GiftItem{}, fmt.Errorf("line %d: %d columns", l.No, len(l.Tokens))
	}
	idx, err := ParseItemRef(l.Tokens[0])
	if err != nil {
		return GiftItem{}, fmt.Errorf("line %d: %w", l.No, err)
	}
	v := make([]int, 8)
	for i := range v {
		if v[i], err = l.Int(i + 1); err != nil {
			return GiftItem{}, err
		}
	}
	return GiftItem{Index: idx, Level: v[0], Durability: v[1], Skill: v[2], Luck: v[3], Option: v[4],
		Excellent: v[5], SetOption: v[6], Duration: v[7]}, nil
}

// MarriageItem is the requirement of sections 0 (marry) and 1 (divorce)
// of CustomMarriage.txt: Enable ItemIndex ItemLevel ItemCount.
type MarriageItem struct {
	Enable, Index, Level, Count int
}

// CustomMarriage is Data/Custom/CustomMarriage.txt (CCustomMarriage +0x28
// marry item, +0x38 divorce item, +0x48 the reward items).
type CustomMarriage struct {
	Marry, Divorce MarriageItem
	Rewards        []GiftItem
}

// LoadCustomMarriage reads Data/Custom/CustomMarriage.txt.
func LoadCustomMarriage(dataDir string) (*CustomMarriage, error) {
	secs, err := loadSections(filepath.Join(dataDir, "Custom", "CustomMarriage.txt"))
	if err != nil {
		return nil, err
	}
	m := &CustomMarriage{}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0, 1:
				if len(l.Tokens) < 4 {
					return nil, fmt.Errorf("CustomMarriage.txt line %d: %d columns", l.No, len(l.Tokens))
				}
				en, err := l.Int(0)
				if err != nil {
					return nil, err
				}
				idx, err := ParseItemRef(l.Tokens[1])
				if err != nil {
					return nil, fmt.Errorf("CustomMarriage.txt line %d: %w", l.No, err)
				}
				lvl, err := l.Int(2)
				if err != nil {
					return nil, err
				}
				cnt, err := l.Int(3)
				if err != nil {
					return nil, err
				}
				r := MarriageItem{Enable: en, Index: idx, Level: lvl, Count: cnt}
				if s.Index == 0 {
					m.Marry = r
				} else {
					m.Divorce = r
				}
			case 2:
				it, err := customItemColumns(l)
				if err != nil {
					return nil, fmt.Errorf("CustomMarriage.txt: %w", err)
				}
				m.Rewards = append(m.Rewards, it)
			}
		}
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

// LoadCustomBuyVip reads Data/Custom/CustomBuyVip.txt (first code wins).
func LoadCustomBuyVip(dataDir string) (map[string]VipPlan, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomBuyVip.txt"))
	if err != nil {
		return nil, err
	}
	out := map[string]VipPlan{}
	for _, l := range mutext.Table(lines) {
		v, err := rowInts(l, 4)
		if err != nil {
			return nil, fmt.Errorf("CustomBuyVip.txt: %w", err)
		}
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("CustomBuyVip.txt line %d: missing code", l.No)
		}
		p := VipPlan{AccountLevel: v[0], NewAccountLevel: v[1], Days: v[2], Money: v[3], Code: cutString(l.Tokens[4], 0x1F)}
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

// LoadCustomPick reads Data/Custom/CustomPick.txt.
func LoadCustomPick(dataDir string) ([]PickBlock, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomPick.txt"))
	if err != nil {
		return nil, err
	}
	var out []PickBlock
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 3 {
			return nil, fmt.Errorf("CustomPick.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("CustomPick.txt line %d: %w", l.No, err)
		}
		lvl, err := l.Int(1)
		if err != nil {
			return nil, err
		}
		al, err := l.Int(2)
		if err != nil {
			return nil, err
		}
		out = append(out, PickBlock{Index: idx, Level: lvl, AccountLevel: al})
	}
	return out, nil
}
