package gamedata

import (
	"fmt"
	"sort"
	"strings"
)

// Drop tables of the original (start-up loader FUN_004eb000 and the
// EventItemBagManager): ItemOptionRate (CItemOptionRate FUN_0049e4b0),
// MapRateInfo (CMapRateInfo), ExcellentOptionRate (CExcellentOptionRate
// FUN_0047bb40), ItemDrop (CItemDrop FUN_004925b0), ItemMoneyDrop
// (CItemMoneyDrop FUN_0049c750), CustomGuard (FUN_0044ef50), CustomMonster
// (FUN_00466b30) and the item bags (CItemBagManager FUN_00490ff0, CItemBagEx
// FUN_0048f2e0).

// --- ItemOptionRate.txt -------------------------------------------------------

// OptionRateCounts are the rate columns of each ItemOptionRate section:
// 0 level (+0..+15), 1 skill, 2 luck, 3 JoL option (0..7), 4 excellent
// option count (0..6), 5 set option (0..6) (FUN_0049ea70..FUN_0049efc0).
var OptionRateCounts = [6]int{16, 2, 2, 8, 7, 7}

// ItemOptionRate holds the six sections of Item/ItemOptionRate.txt by
// row Index.
type ItemOptionRate struct {
	Sections [6]map[int][]int
}

// LoadItemOptionRate reads the ItemOptionRate table of
// config/data/item.toml. The first row of an Index wins.
func LoadItemOptionRate(configDir string) (*ItemOptionRate, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	r := &ItemOptionRate{}
	t := f.ItemOptionRate
	for sec, rows := range [6][]OptionRateRow{t.Level, t.Skill, t.Luck, t.Option, t.Excellent, t.Set} {
		r.Sections[sec] = map[int][]int{}
		for i, row := range rows {
			if len(row.Rate) != OptionRateCounts[sec] {
				return nil, fmt.Errorf("ItemOptionRate section %d row %d: %d rates, want %d", sec, i+1, len(row.Rate), OptionRateCounts[sec])
			}
			if _, dup := r.Sections[sec][row.Index]; !dup {
				r.Sections[sec][row.Index] = append([]int(nil), row.Rate...)
			}
		}
	}
	return r, nil
}

// Pick is the weighted choice of FUN_0049ea70 and siblings: the row index
// of section sec; every non-zero rate i covers [sum, sum+rate) and
// rnd() % total selects one. ok is false when the row does not exist or all
// rates are 0 (the output byte is then left unchanged).
func (r *ItemOptionRate) Pick(sec, index int, rnd func() int) (int, bool) {
	if r == nil || sec < 0 || sec > 5 {
		return 0, false
	}
	rates, found := r.Sections[sec][index]
	if !found {
		return 0, false
	}
	return weighted(rates, rnd)
}

// weighted picks the position of a rate list with rnd() % sum.
func weighted(rates []int, rnd func() int) (int, bool) {
	total := 0
	for _, v := range rates {
		if v != 0 {
			total += v
		}
	}
	if total == 0 {
		return 0, false
	}
	x := rnd() % total
	start := 0
	for i, v := range rates {
		if v == 0 {
			continue
		}
		if start <= x && x < start+v {
			return i, true
		}
		start += v
	}
	return 0, false
}

// --- MapRateInfo.txt ------------------------------------------------------------

// MapRateInfo is a MapRateInfo.txt row (36 bytes): map (-1 any), drop
// type (0 common, 1 excellent, 2 set) and the ItemOptionRate row of each
// option (Level, Skill, Luck, JoL, Exc, Set).
type MapRateInfo struct {
	Map, DropType int
	Option        [6]int
}

// LoadMapRateInfo reads the MapRateInfo table of config/data/common.toml.
func LoadMapRateInfo(configDir string) ([]MapRateInfo, error) {
	f, err := LoadCommonConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []MapRateInfo
	for _, r := range f.MapRateInfo.Rows {
		out = append(out, MapRateInfo{Map: r.MapNumber, DropType: r.ItemDropType, Option: r.Option})
	}
	return out, nil
}

// MapRateOption is FUN_004afd90..FUN_004afe80: the option row for (map,
// drop type) of the first matching line, 0 when none matches.
func MapRateOption(rows []MapRateInfo, mp, dropType, option int) int {
	for _, r := range rows {
		if r.DropType == dropType && (r.Map == -1 || r.Map == mp) {
			return r.Option[option]
		}
	}
	return 0
}

// --- ExcellentOptionRate.txt ----------------------------------------------------

// ExcellentOptionRate is a Data/Item/ExcellentOptionRate.txt row (0x30
// bytes): the option bit, the item index range (-1 any) and its weight.
type ExcellentOptionRate struct {
	Option   int // +0x00
	Min, Max int // +0x04 / +0x08
	Name     string
	Rate     int // +0x2c
}

// LoadExcellentOptionRate reads the ExcellentOptionRate table of
// config/data/item.toml.
func LoadExcellentOptionRate(configDir string) ([]ExcellentOptionRate, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ExcellentOptionRate
	for i, r := range f.ExcellentOptionRate.Rows {
		lo, err := ParseItemRef(r.ItemMinIndex)
		if err != nil {
			return nil, fmt.Errorf("ExcellentOptionRate row %d: %w", i+1, err)
		}
		hi, err := ParseItemRef(r.ItemMaxIndex)
		if err != nil {
			return nil, fmt.Errorf("ExcellentOptionRate row %d: %w", i+1, err)
		}
		out = append(out, ExcellentOptionRate{Option: r.Index, Min: lo, Max: hi, Name: r.Name, Rate: r.DropRate})
	}
	return out, nil
}

// PickExcellentOption is FUN_0047bd30: a weighted choice among the rows
// whose range holds the item (rate != 0); 0 when none applies.
func PickExcellentOption(rows []ExcellentOptionRate, index int, rnd func() int) int {
	var cand []ExcellentOptionRate
	var rates []int
	for _, r := range rows {
		if (r.Min == -1 || r.Min <= index) && (r.Max == -1 || index <= r.Max) && r.Rate != 0 {
			cand = append(cand, r)
			rates = append(rates, r.Rate)
		}
	}
	i, ok := weighted(rates, rnd)
	if !ok {
		return 0
	}
	return cand[i].Option
}

// --- ItemDrop.txt -----------------------------------------------------------

// ItemDropRule is a Data/Item/ItemDrop.txt row (0x48 bytes, FUN_00492870).
type ItemDropRule struct {
	Index    int    // +0x00
	Level    int    // +0x04
	Grade    int    // +0x08 (passed as the durability)
	Option   [6]int // +0x0c..+0x20: ItemOptionRate rows
	Duration int    // +0x24 (seconds, 0 = permanent)
	Map      int    // +0x28 (-1 any)
	Monster  int    // +0x2c (-1 any)
	MinLevel int    // +0x30 (-1 any)
	MaxLevel int    // +0x34 (-1 any)
	Rate     [4]int // +0x38: DropRate per account level, of 1,000,000 (-1 = always)
}

// LoadItemDrop reads the ItemDrop table of config/data/item.toml.
func LoadItemDrop(configDir string) ([]ItemDropRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ItemDropRule
	for i, r := range f.ItemDrop.Rows {
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemDrop row %d: %w", i+1, err)
		}
		out = append(out, ItemDropRule{Index: cat*512 + idx, Level: r.ItemLevel, Grade: r.ItemGrade,
			Option:   [6]int{r.LevelOption, r.SkillOption, r.LuckOption, r.JoLOption, r.ExcOption, r.SetOption},
			Duration: r.Duration, Map: r.MapNumber, Monster: r.MonsterIndex, MinLevel: r.MonsterMinLevel,
			MaxLevel: r.MonsterMaxLevel, Rate: r.DropRate})
	}
	return out, nil
}

// --- ItemMoneyDrop.txt ------------------------------------------------------

// ItemMoneyDropRule is a Data/Item/ItemMoneyDrop.txt row (7 ints,
// FUN_0049c920).
type ItemMoneyDropRule struct {
	Map, Monster, MinLevel, MaxLevel, AccountLevel int
	Amount                                         int // -1 = the monster's experience
	Rate                                           int // of 1,000,000 (-1 = always)
}

// LoadItemMoneyDrop reads the ItemMoneyDrop table of config/data/item.toml.
func LoadItemMoneyDrop(configDir string) ([]ItemMoneyDropRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ItemMoneyDropRule
	for _, r := range f.ItemMoneyDrop.Rows {
		out = append(out, ItemMoneyDropRule{r.MapNumber, r.MonsterIndex, r.MonsterMinLevel, r.MonsterMaxLevel,
			r.AccountLevel, r.MoneyAmount, r.MoneyDropRate})
	}
	return out, nil
}

// --- Custom/CustomGuard.txt and CustomMonster.txt -----------------------------

// LoadCustomGuard reads the CustomGuard table of config/data/custom.toml:
// up to 100 monster classes that never drop (FUN_0044ef50 / FUN_0044f040).
func LoadCustomGuard(configDir string) ([]int, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	return monsterClassList(f.CustomGuard), nil
}

// CustomMonsterRow is a full CustomMonster.txt row (0x3c bytes,
// FUN_00466b30): the percentages FUN_00466d40 applies at creation and the
// kill message / special bag of FUN_00466f00. -1 = "*".
type CustomMonsterRow struct {
	Monster, Map                     int
	MaxLife, DamageMin, DamageMax    int
	Defense, AttackRate, DefenseRate int
	ExperienceRate                   int
	KillMessage                      int
	SpecialBag                       int
}

// LoadCustomMonsterRows reads the CustomMonster table of
// config/data/custom.toml.
func LoadCustomMonsterRows(configDir string) ([]CustomMonsterRow, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []CustomMonsterRow
	for _, r := range f.CustomMonster.Rows {
		out = append(out, CustomMonsterRow{r.MonsterIndex, r.MapNumber, r.MaxLife, r.DamageMin, r.DamageMax, r.Defense,
			r.AttackRate, r.DefenseRate, r.ExperienceRate, r.KillMessage, r.SpecialBag})
	}
	return out, nil
}

// FindCustomMonster returns the first row matching class and map (-1 any).
func FindCustomMonster(rows []CustomMonsterRow, class, mp int) (CustomMonsterRow, bool) {
	for _, r := range rows {
		if (r.Monster == -1 || r.Monster == class) && (r.Map == -1 || r.Map == mp) {
			return r, true
		}
	}
	return CustomMonsterRow{}, false
}

// --- EventItemBag ---------------------------------------------------------------

// BagEntry is an EventItemBagManager.txt row (0x1a8 bytes): the keys
// that select a bag and the bag itself.
type BagEntry struct {
	ItemIndex    int // +0x00
	ItemLevel    int // +0x04
	MonsterClass int // +0x08
	SpecialValue int // +0x0c
	SpecialLevel int // +0x10
	SpecialState int // +0x14
	Path         string
	Bag          *ItemBag
}

// ItemBag is a CItemBagEx (FUN_0048f2e0): section 2 settings, section 3
// bag rows, section 4 drop rows per bag and the item lists (sections >= 5).
type ItemBag struct {
	ItemRate     int // +0x20 (%): item instead of zen
	MoneyDrop    int // +0x24
	BagUseEffect int // +0x28: 1/2 firework F3 40, 3 = F3 40 0x3A
	DropType     int // +0x30: 0 on the ground, else into the inventory
	Bags         []BagRow
	Sections     map[int][]BagItem
}

// BagRow is a section 3 row with its section 4 drops.
type BagRow struct {
	Index    int
	DropRate int // of 10000
	Map      int // -1 any
	Drops    []BagDrop
}

// BagDrop is a section 4 row (0x2c bytes).
type BagDrop struct {
	Section     int    // +0x04 (-1 = set item)
	SectionRate int    // +0x08 (weight)
	SetDropRate int    // +0x0c (of 10000)
	MinCount    int    // +0x10
	MaxCount    int    // +0x14
	Class       [5]int // +0x18: required change-up + 1 per class (0 = not allowed)
}

// BagItem is an item row of a section >= 5 (0x28 bytes).
type BagItem struct {
	Index    int    // +0x00
	Level    int    // +0x04
	Grade    int    // +0x08
	Option   [6]int // +0x0c..+0x20
	Duration int    // +0x24
}

// BuildItemBag converts the sections of a bag file (FUN_0048f2e0).
func BuildItemBag(f ItemBagFile) (*ItemBag, error) {
	b := &ItemBag{Sections: map[int][]BagItem{}}
	find := func(idx int) *BagRow {
		for i := range b.Bags {
			if b.Bags[i].Index == idx {
				return &b.Bags[i]
			}
		}
		return nil
	}
	for _, r := range f.Settings {
		b.ItemRate, b.MoneyDrop, b.BagUseEffect, b.DropType = r.ItemRate, r.MoneyDrop, r.BagUseEffect, r.DropType
	}
	for _, r := range f.Bag {
		if find(r.BagIndex) == nil { // std::map insert keeps the first key
			b.Bags = append(b.Bags, BagRow{Index: r.BagIndex, DropRate: r.DropRate, Map: r.MapNumber})
		}
	}
	for _, r := range f.Drop {
		d := BagDrop{Section: r.Section, SectionRate: r.SectionRate, SetDropRate: r.SetDropRate, MinCount: r.MinDropCount,
			MaxCount: r.MaxDropCount, Class: r.Class}
		if d.MinCount < 0 || d.MaxCount < d.MinCount || d.MaxCount > 1000 {
			continue // "[ItemBagEx] Invalid drop count"
		}
		if bag := find(r.BagIndex); bag != nil {
			bag.Drops = append(bag.Drops, d)
		}
	}
	for _, s := range f.Items {
		for i, r := range s.Rows {
			cat, idx, err := SplitItemRef(r.ItemIndex)
			if err != nil {
				return nil, fmt.Errorf("section %d row %d: %w", s.Section, i+1, err)
			}
			it := BagItem{Index: cat*512 + idx, Level: r.ItemLevel, Grade: r.GradeCount, Option: r.Option, Duration: r.Duration}
			if it.Index < 0 || it.Index >= 0x2000 {
				it.Index = -1
			}
			b.Sections[s.Section] = append(b.Sections[s.Section], it)
		}
	}
	sort.SliceStable(b.Bags, func(i, j int) bool { return b.Bags[i].Index < b.Bags[j].Index })
	return b, nil
}

// BagKey is the Bag key of a manager BagPath ("Item\Box of Luck" ->
// "Item/Box of Luck").
func BagKey(path string) string { return strings.ReplaceAll(path, "\\", "/") }

// LoadEventItemBags reads config/data/eventitembag.toml: the manager rows
// and the bag each one names (FUN_00491220).
func LoadEventItemBags(configDir string) ([]BagEntry, error) {
	f, err := LoadEventItemBagConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []BagEntry
	for _, r := range f.EventItemBagManager.Rows {
		e := BagEntry{ItemIndex: r.ItemIndex, ItemLevel: r.ItemLevel, MonsterClass: r.MonsterClass,
			SpecialValue: r.SpecialValue, SpecialLevel: r.SpecialLevel, SpecialState: r.SpecialState, Path: r.BagPath}
		bag, ok := f.Bag[BagKey(r.BagPath)]
		if !ok {
			return nil, fmt.Errorf("EventItemBagManager: bag %q not found in Bag", r.BagPath)
		}
		if e.Bag, err = BuildItemBag(bag); err != nil {
			return nil, fmt.Errorf("Bag.%q: %w", BagKey(r.BagPath), err)
		}
		out = append(out, e)
	}
	return out, nil
}
