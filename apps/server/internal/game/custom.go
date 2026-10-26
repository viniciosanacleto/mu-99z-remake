package game

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

// customTables implements charset.Custom from Custom/CustomWing.txt
// (CCustomWing, 100 slots indexed by the Index column) and
// Custom/CustomPet.txt (CCustomPet, keyed by item index).
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

func loadCustomTables(dataDir string) (customTables, error) {
	t := customTables{wings: map[int]int{}, pets: map[int]bool{}}
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Custom", "CustomWing.json"))
	if err != nil {
		return t, err
	}
	for _, l := range mutext.Table(lines) {
		slot, err := l.Int(0)
		if err != nil {
			return t, err
		}
		idx, err := parseItemRef(l.Tokens[1])
		if err != nil {
			return t, fmt.Errorf("CustomWing.txt line %d: %w", l.No, err)
		}
		if slot >= 0 && slot < 100 {
			t.wings[idx] = slot
		}
	}
	lines, err = jsonconf.LoadLines(filepath.Join(dataDir, "Custom", "CustomPet.json"))
	if err != nil {
		return t, err
	}
	for _, l := range mutext.Table(lines) {
		idx, err := parseItemRef(l.Tokens[1])
		if err != nil {
			return t, fmt.Errorf("CustomPet.txt line %d: %w", l.No, err)
		}
		t.pets[idx] = true
	}
	return t, nil
}
