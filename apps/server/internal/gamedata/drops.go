package gamedata

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Drop tables of the original (start-up loader FUN_004eb000 and the
// EventItemBagManager): ItemOptionRate (CItemOptionRate FUN_0049e4b0),
// MapRateInfo (CMapRateInfo), ExcellentOptionRate (CExcellentOptionRate
// FUN_0047bb40), ItemDrop (CItemDrop FUN_004925b0), ItemMoneyDrop
// (CItemMoneyDrop FUN_0049c750), CustomGuard (FUN_0044ef50), CustomMonster
// (FUN_00466b30) and the item bags (CItemBagManager FUN_00490ff0, CItemBagEx
// FUN_0048f2e0).

// numTok reproduces the number reader of the original tokenizer
// (FUN_004e0d70 + _ftol): a token made of digits, '.', '-' or '*' is a
// number ("*" = -1); any other token (a word such as "N") leaves the
// previous number in place, so it reads as the value of the previous
// numeric column.
func numTok(tok string, prev int) int {
	if tok == "" {
		return prev
	}
	c := tok[0]
	if !(c >= '0' && c <= '9' || c == '.' || c == '-' || c == '*') {
		return prev
	}
	end := 0
	for end < len(tok) && (tok[end] >= '0' && tok[end] <= '9' || tok[end] == '.' || tok[end] == '-' || tok[end] == '*') {
		end++
	}
	s := tok[:end]
	if s == "*" {
		return -1
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0 // atof of a malformed number
	}
	return int(f) // _ftol truncates
}

// numFields splits the tokens of a line the way the original reads them:
// a comma ends a number ("07,000" and "0," "8" are two numbers each).
func numFields(l mutext.Line) []string {
	var out []string
	for _, t := range l.Tokens {
		for _, p := range strings.Split(t, ",") {
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// numRow reads n numbers from fields starting at from (missing columns read
// as the previous value, like the original reading past the line).
func numRow(f []string, from, n int) []int {
	out := make([]int, n)
	prev := 0
	for i := 0; i < n; i++ {
		tok := ""
		if from+i < len(f) {
			tok = f[from+i]
		}
		prev = numTok(tok, prev)
		out[i] = prev
	}
	return out
}

func dropSections(path string) ([]mutext.Section, error) {
	lines, err := mutext.LoadFile(path)
	if err != nil {
		return nil, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return secs, nil
}

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

// LoadItemOptionRate reads Data/Item/ItemOptionRate.txt.
func LoadItemOptionRate(dataDir string) (*ItemOptionRate, error) {
	secs, err := dropSections(filepath.Join(dataDir, "Item", "ItemOptionRate.txt"))
	if err != nil {
		return nil, err
	}
	r := &ItemOptionRate{}
	for i := range r.Sections {
		r.Sections[i] = map[int][]int{}
	}
	for _, s := range secs {
		if s.Index < 0 || s.Index > 5 {
			continue
		}
		n := OptionRateCounts[s.Index]
		for _, l := range s.Rows {
			v := numRow(numFields(l), 0, n+1)
			if _, dup := r.Sections[s.Index][v[0]]; !dup {
				r.Sections[s.Index][v[0]] = v[1:]
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

// MapRateInfo is a Data/MapRateInfo.txt row (36 bytes): map (-1 any), drop
// type (0 common, 1 excellent, 2 set) and the ItemOptionRate row of each
// option (Level, Skill, Luck, JoL, Exc, Set).
type MapRateInfo struct {
	Map, DropType int
	Option        [6]int
}

// LoadMapRateInfo reads Data/MapRateInfo.txt.
func LoadMapRateInfo(dataDir string) ([]MapRateInfo, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "MapRateInfo.txt"))
	if err != nil {
		return nil, err
	}
	var out []MapRateInfo
	for _, l := range mutext.Table(lines) {
		v := numRow(numFields(l), 0, 8)
		r := MapRateInfo{Map: v[0], DropType: v[1]}
		copy(r.Option[:], v[2:])
		out = append(out, r)
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

// LoadExcellentOptionRate reads Data/Item/ExcellentOptionRate.txt.
func LoadExcellentOptionRate(dataDir string) ([]ExcellentOptionRate, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "ExcellentOptionRate.txt"))
	if err != nil {
		return nil, err
	}
	var out []ExcellentOptionRate
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("ExcellentOptionRate.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		opt := numTok(l.Tokens[0], 0)
		lo, err := ParseItemRef(l.Tokens[1])
		if err != nil {
			return nil, fmt.Errorf("ExcellentOptionRate.txt line %d: %w", l.No, err)
		}
		hi, err := ParseItemRef(l.Tokens[2])
		if err != nil {
			return nil, fmt.Errorf("ExcellentOptionRate.txt line %d: %w", l.No, err)
		}
		out = append(out, ExcellentOptionRate{Option: opt, Min: lo, Max: hi, Name: l.Tokens[3],
			Rate: numTok(l.Tokens[4], 0)})
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

// LoadItemDrop reads Data/Item/ItemDrop.txt.
func LoadItemDrop(dataDir string) ([]ItemDropRule, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "ItemDrop.txt"))
	if err != nil {
		return nil, err
	}
	var out []ItemDropRule
	for _, l := range mutext.Table(lines) {
		f := numFields(l)
		v := numRow(f, 0, 19)
		r := ItemDropRule{Index: v[0]*512 + v[1], Level: v[2], Grade: v[3], Duration: v[10], Map: v[11],
			Monster: v[12], MinLevel: v[13], MaxLevel: v[14]}
		copy(r.Option[:], v[4:10])
		copy(r.Rate[:], v[15:19])
		out = append(out, r)
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

// LoadItemMoneyDrop reads Data/Item/ItemMoneyDrop.txt.
func LoadItemMoneyDrop(dataDir string) ([]ItemMoneyDropRule, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Item", "ItemMoneyDrop.txt"))
	if err != nil {
		return nil, err
	}
	var out []ItemMoneyDropRule
	for _, l := range mutext.Table(lines) {
		v := numRow(numFields(l), 0, 7)
		out = append(out, ItemMoneyDropRule{v[0], v[1], v[2], v[3], v[4], v[5], v[6]})
	}
	return out, nil
}

// --- Custom/CustomGuard.txt and CustomMonster.txt -----------------------------

// LoadCustomGuard reads Data/Custom/CustomGuard.txt section 0: up to 100
// monster classes that never drop (FUN_0044ef50 / FUN_0044f040).
func LoadCustomGuard(dataDir string) ([]int, error) {
	return monsterClassList(filepath.Join(dataDir, "Custom", "CustomGuard.txt"))
}

// CustomMonsterRow is a full Data/Custom/CustomMonster.txt row (0x3c bytes,
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

// LoadCustomMonsterRows reads Data/Custom/CustomMonster.txt.
func LoadCustomMonsterRows(dataDir string) ([]CustomMonsterRow, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomMonster.txt"))
	if err != nil {
		return nil, err
	}
	var out []CustomMonsterRow
	for _, l := range mutext.Table(lines) {
		v := numRow(numFields(l), 0, 11)
		out = append(out, CustomMonsterRow{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10]})
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

// BagEntry is a Data/EventItemBagManager.txt row (0x1a8 bytes): the keys
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

// LoadItemBag reads one EventItemBag file.
func LoadItemBag(path string) (*ItemBag, error) {
	secs, err := dropSections(path)
	if err != nil {
		return nil, err
	}
	b := &ItemBag{Sections: map[int][]BagItem{}}
	find := func(idx int) *BagRow {
		for i := range b.Bags {
			if b.Bags[i].Index == idx {
				return &b.Bags[i]
			}
		}
		return nil
	}
	for _, s := range secs {
		for _, l := range s.Rows {
			f := numFields(l)
			switch {
			case s.Index == 2:
				v := numRow(f, 0, 4)
				b.ItemRate, b.MoneyDrop, b.BagUseEffect, b.DropType = v[0], v[1], v[2], v[3]
			case s.Index == 3:
				v := numRow(f, 0, 3)
				if find(v[0]) == nil { // std::map insert keeps the first key
					b.Bags = append(b.Bags, BagRow{Index: v[0], DropRate: v[1], Map: v[2]})
				}
			case s.Index == 4:
				v := numRow(f, 0, 11)
				d := BagDrop{Section: v[1], SectionRate: v[2], SetDropRate: v[3], MinCount: v[4], MaxCount: v[5]}
				copy(d.Class[:], v[6:11])
				if d.MinCount < 0 || d.MaxCount < d.MinCount || d.MaxCount > 1000 {
					continue // "[ItemBagEx] Invalid drop count"
				}
				if r := find(v[0]); r != nil {
					r.Drops = append(r.Drops, d)
				}
			case s.Index > 4:
				v := numRow(f, 0, 12)
				it := BagItem{Index: v[0]*512 + v[1], Level: v[2], Grade: v[3], Duration: v[10]}
				copy(it.Option[:], v[4:10])
				if v[0]*512+v[1] < 0 || v[0]*512+v[1] >= 0x2000 {
					it.Index = -1
				}
				b.Sections[s.Index] = append(b.Sections[s.Index], it)
			}
		}
	}
	sort.SliceStable(b.Bags, func(i, j int) bool { return b.Bags[i].Index < b.Bags[j].Index })
	return b, nil
}

// LoadEventItemBags reads Data/EventItemBagManager.txt and every bag it
// names (EventItemBag\<path>.txt, FUN_00491220).
func LoadEventItemBags(dataDir string) ([]BagEntry, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "EventItemBagManager.txt"))
	if err != nil {
		return nil, err
	}
	var out []BagEntry
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 7 {
			return nil, fmt.Errorf("EventItemBagManager.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := numRow(l.Tokens, 0, 6)
		e := BagEntry{ItemIndex: v[0], ItemLevel: v[1], MonsterClass: v[2], SpecialValue: v[3],
			SpecialLevel: v[4], SpecialState: v[5], Path: l.Tokens[6]}
		p := filepath.Join(append([]string{dataDir, "EventItemBag"}, strings.Split(e.Path+".txt", "\\")...)...)
		if e.Bag, err = LoadItemBag(p); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
