package game

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// customTables implements charset.Custom from CustomWing.txt (CCustomWing,
// 100 slots indexed by the Index column) and CustomPet.txt (CCustomPet,
// keyed by item index).
type customTables struct {
	wings map[int]int // item index -> CustomWing Index
	pets  map[int]bool
}

func (c customTables) WingSlot(itemIndex int) (int, bool) {
	s, ok := c.wings[itemIndex]
	return s, ok
}

func (c customTables) IsPet(itemIndex int) bool { return c.pets[itemIndex] }

// parseItemRef parses "cc,iii".
func parseItemRef(s string) (int, error) {
	cat, idx, ok := strings.Cut(s, ",")
	if !ok {
		return 0, fmt.Errorf("item reference %q", s)
	}
	c, err := strconv.Atoi(cat)
	if err != nil {
		return 0, err
	}
	i, err := strconv.Atoi(idx)
	if err != nil {
		return 0, err
	}
	return item.Make(c, i), nil
}

// loadCustomTables reads the CustomWing and CustomPet tables of
// config/data/custom.toml.
func loadCustomTables(configDir string) (customTables, error) {
	t := customTables{wings: map[int]int{}, pets: map[int]bool{}}
	f, err := gamedata.LoadCustomConfig(configDir)
	if err != nil {
		return t, err
	}
	for i, r := range f.CustomWing.Rows {
		idx, err := parseItemRef(r.ItemIndex)
		if err != nil {
			return t, fmt.Errorf("CustomWing row %d: %w", i+1, err)
		}
		if r.Index >= 0 && r.Index < 100 {
			t.wings[idx] = r.Index
		}
	}
	for i, r := range f.CustomPet.Rows {
		idx, err := parseItemRef(r.ItemIndex)
		if err != nil {
			return t, fmt.Errorf("CustomPet row %d: %w", i+1, err)
		}
		t.pets[idx] = true
	}
	return t, nil
}
