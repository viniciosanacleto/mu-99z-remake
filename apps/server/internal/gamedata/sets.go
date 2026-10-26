package gamedata

import (
	"fmt"
	"sort"
)

// Ancient (set) items: SetItemType.txt (CSetItemType, loader
// FUN_004f3910, lookup FUN_004f42b0) and Item/SetItemOption.txt
// (CSetItemOption, 100 rows of 0xac bytes, loader FUN_004f2b50).

// SetItemType is a SetItemType.txt row (map value: +0 index, +4 StatType,
// +8/+0xc set index for set option 1/2, +0x10 drop weight).
type SetItemType struct {
	Index    int
	StatType int    // option code of the item's own +5/+10 stat (-1 none)
	Option   [2]int // SetItemOption row of tier 1 / tier 2 (0 = none)
	DropRate int    // weight of FUN_004f3ad0
}

// SetItemTypes is the CSetItemType map with its keys in ascending order
// (std::map iteration order).
type SetItemTypes struct {
	ByIndex map[int]SetItemType
	Order   []int
}

// LoadSetItemType reads the SetItemType table of config/data/item.toml;
// the first row of an item wins.
func LoadSetItemType(configDir string) (*SetItemTypes, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	s := &SetItemTypes{ByIndex: map[int]SetItemType{}}
	for i, row := range f.SetItemType.Rows {
		cat, idx, err := SplitItemRef(row.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("SetItemType row %d: %w", i+1, err)
		}
		r := SetItemType{Index: cat*512 + idx, StatType: row.StatType,
			Option: [2]int{row.OptionIndex1, row.OptionIndex2}, DropRate: row.ItemDropRate}
		if _, dup := s.ByIndex[r.Index]; dup {
			continue
		}
		s.ByIndex[r.Index] = r
		s.Order = append(s.Order, r.Index)
	}
	sort.Ints(s.Order)
	return s, nil
}

// Get is FUN_004f42b0.
func (s *SetItemTypes) Get(index int) (SetItemType, bool) {
	if s == nil {
		return SetItemType{}, false
	}
	r, ok := s.ByIndex[index]
	return r, ok
}

// SetOptionIndex returns the SetItemOption row of an ancient item: the
// tier selected by the low two bits of the set byte (FUN_004f2d50); -1 when
// the item has none.
func (s *SetItemTypes) SetOptionIndex(index int, set byte) int {
	r, ok := s.Get(index)
	t := int(set&3) - 1
	if !ok || t < 0 || t > 1 {
		return -1
	}
	return r.Option[t]
}

// SetOptionPair is one (option code, value) of SetItemOption.txt; Code -1
// is "*".
type SetOptionPair struct{ Code, Value int }

// SetItemOption is a SetItemOption.txt row: 6 tiers of 2 options (+0x24)
// and the 5 full-set options (+0x84).
type SetItemOption struct {
	Index int
	Name  string
	Tier  [6][2]SetOptionPair
	Full  [5]SetOptionPair
}

// SetItemOptions is the 100-entry table (Index -1 = empty slot).
type SetItemOptions [100]SetItemOption

// LoadSetItemOption reads the SetItemOption table of config/data/item.toml;
// rows with an index outside 0..99 are ignored like the original.
func LoadSetItemOption(configDir string) (*SetItemOptions, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	t := &SetItemOptions{}
	for i := range t {
		t[i].Index = -1
	}
	for _, row := range f.SetItemOption.Rows {
		r := SetItemOption{Index: row.Index, Name: row.Name}
		for k, p := range row.Option {
			r.Tier[k/2][k%2] = SetOptionPair{p[0], p[1]}
		}
		for k, p := range row.FullOption {
			r.Full[k] = SetOptionPair{p[0], p[1]}
		}
		if row.Index >= 0 && row.Index < 100 {
			t[row.Index] = r
		}
	}
	return t, nil
}

// TierCount is the number of tiers holding at least one option (the full
// set test of FUN_004f30f0).
func (r SetItemOption) TierCount() int {
	n := 0
	for _, tier := range r.Tier {
		if tier[0].Code != -1 || tier[1].Code != -1 {
			n++
		}
	}
	return n
}
