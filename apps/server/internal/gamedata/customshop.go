package gamedata

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Data files of the last WizardTeam customs (docs/server-plan/
// 10e-customs-wizardteam.md): Custom/CustomJewel.txt (CCustomJewel),
// Custom/CustomLuckyWheel.txt (CLuckyWheel) and Custom/CustomXShop.txt
// (CCustomXShop, with the in-game GM price writer).

// --- CustomJewel ---------------------------------------------------------------------

// CustomJewelSlots is the size of the CCustomJewel table (FUN_00464b70: 50
// records of 0x98 bytes, indexed by the JewelIndex column).
const CustomJewelSlots = 50

// JewelChange is a section 2 (success) or 3 (failure) row of
// CustomJewel.txt (record +0x58 / +0x78): the values added to or removed
// from the target item. The eighth dword of the record (sockets) has no
// column and stays 0.
type JewelChange struct {
	Level, Skill, Luck, Option, Excellent, SetOption int
}

// CustomJewel is one CCustomJewel record. Index is -1 while section 0 has
// no row for the slot (FUN_00464b70 fills every slot with -1 first).
type CustomJewel struct {
	Index     int
	ItemIndex int    // -1 when out of range
	Weapon    int    // EnableSlotWeapon (categories 0..5)
	Armor     int    // EnableSlotArmor (categories 6..11)
	Wing      int    // EnableSlotWing (wings and CustomWing items)
	Rate      [4]int // Rate_AL0..3, success chance in percent
	SalePrice int

	// Section 1: the target item requirements; -1 ("*") is "any".
	MinLevel, MaxLevel   int
	Skill, Luck          int // maximum skill / luck of the target (0 = must not have it)
	MinOption, MaxOption int
	MinExc               int // minimum number of excellent options
	MaxExc               int // < 10: maximum count; > 10: forbidden bit mask (value-10)
	MaxSetOption         int // -1 any; otherwise the item must not be ancient, bit 0/1 need the set tier

	Success, Failure JewelChange
}

// CustomJewels is Data/Custom/CustomJewel.txt.
type CustomJewels struct {
	Slots [CustomJewelSlots]CustomJewel
}

// Find is FUN_004650b0: the first slot whose Index equals its position and
// whose ItemIndex is index.
func (c *CustomJewels) Find(index int) (*CustomJewel, bool) {
	if c == nil {
		return nil, false
	}
	for i := range c.Slots {
		if j := &c.Slots[i]; j.Index == i && j.ItemIndex == index {
			return j, true
		}
	}
	return nil, false
}

// itemRefAt reads an item reference at token i: "cc,iii" in one token or
// "cc iii" in two (the original tokenizer splits on the comma). It returns
// the number of tokens used; out-of-range references read as -1.
func itemRefAt(l mutext.Line, i int) (int, int, error) {
	tok, err := l.String(i)
	if err != nil {
		return 0, 0, err
	}
	if c, x, ok := strings.Cut(tok, ","); ok {
		cat, err1 := strconv.Atoi(c)
		idx, err2 := strconv.Atoi(x)
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("line %d: item reference %q", l.No, tok)
		}
		return refIndex(cat, idx), 1, nil
	}
	cat, err := l.Int(i)
	if err != nil {
		return 0, 0, err
	}
	idx, err := l.Int(i + 1)
	if err != nil {
		return 0, 0, err
	}
	return refIndex(cat, idx), 2, nil
}

func refIndex(cat, idx int) int {
	v := cat*512 + idx
	if v < 0 || v >= 0x2000 {
		return -1
	}
	return v
}

// LoadCustomJewel reads Data/Custom/CustomJewel.txt (FUN_00464b70).
// Indexes of 50 or more are ignored; a later row overwrites the slot.
func LoadCustomJewel(dataDir string) (*CustomJewels, error) {
	c := &CustomJewels{}
	for i := range c.Slots {
		c.Slots[i].Index = -1
	}
	secs, err := loadSections(filepath.Join(dataDir, "Custom", "CustomJewel.txt"))
	if err != nil {
		return nil, err
	}
	for _, s := range secs {
		for _, l := range s.Rows {
			if err := c.loadRow(s.Index, l); err != nil {
				return nil, fmt.Errorf("CustomJewel.txt: %w", err)
			}
		}
	}
	return c, nil
}

func (c *CustomJewels) loadRow(sec int, l mutext.Line) error {
	idx, err := l.Int(0)
	if err != nil {
		return err
	}
	switch sec {
	case 0:
		item, n, err := itemRefAt(l, 1)
		if err != nil {
			return err
		}
		v, err := shopInts(l, 1+n, 8)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= CustomJewelSlots {
			return nil
		}
		j := &c.Slots[idx]
		j.Index, j.ItemIndex = idx, item
		j.Weapon, j.Armor, j.Wing = v[0], v[1], v[2]
		copy(j.Rate[:], v[3:7])
		j.SalePrice = v[7]
	case 1:
		v, err := shopInts(l, 1, 9)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= CustomJewelSlots {
			return nil
		}
		j := &c.Slots[idx]
		j.MinLevel, j.MaxLevel, j.Skill, j.Luck = v[0], v[1], v[2], v[3]
		j.MinOption, j.MaxOption, j.MinExc, j.MaxExc, j.MaxSetOption = v[4], v[5], v[6], v[7], v[8]
	case 2, 3:
		v, err := shopInts(l, 1, 6)
		if err != nil {
			return err
		}
		if idx < 0 || idx >= CustomJewelSlots {
			return nil
		}
		ch := JewelChange{v[0], v[1], v[2], v[3], v[4], v[5]}
		if sec == 2 {
			c.Slots[idx].Success = ch
		} else {
			c.Slots[idx].Failure = ch
		}
	}
	return nil
}

func shopInts(l mutext.Line, from, n int) ([]int, error) {
	v := make([]int, n)
	for i := range v {
		x, err := l.Int(from + i)
		if err != nil {
			return nil, err
		}
		v[i] = x
	}
	return v, nil
}

// --- LuckyWheel ----------------------------------------------------------------------

// WheelReward is a CustomLuckyWheel.txt row (CLuckyWheel map value,
// FUN_0044e3a0: the fifth column is stored after the sixth, so the record
// is Index Type ItemIndex Level Luck Skill Option Excellent).
type WheelReward struct {
	Index, Type, Item, Level int
	Skill, Luck              int
	Option, Excellent        int
}

// LoadLuckyWheel reads Data/Custom/CustomLuckyWheel.txt: a plain table
// keyed by IndexWheel; the first row of an index wins (std::map insert).
func LoadLuckyWheel(dataDir string) (map[int]WheelReward, error) {
	lines, err := mutext.LoadFile(filepath.Join(dataDir, "Custom", "CustomLuckyWheel.txt"))
	if err != nil {
		return nil, err
	}
	out := map[int]WheelReward{}
	for _, l := range mutext.Table(lines) {
		v, err := shopInts(l, 0, 8)
		if err != nil {
			return nil, fmt.Errorf("CustomLuckyWheel.txt: %w", err)
		}
		if _, ok := out[v[0]]; ok {
			continue
		}
		out[v[0]] = WheelReward{Index: v[0], Type: v[1], Item: v[2], Level: v[3], Skill: v[4], Luck: v[5],
			Option: v[6], Excellent: v[7]}
	}
	return out, nil
}

// --- CustomXShop ---------------------------------------------------------------------

// XShopDefaultPrice is the category price used when CustomXShop.txt cannot
// be opened (FUN_004026e0).
const XShopDefaultPrice = 25

// XShopPrices is Custom/CustomXShop.txt (CCustomXShop +0x34 and the
// std::map at +0x78): section 0 "Category Price" (16 categories), section 1
// "Type Index Price" overrides. Negative prices read as 0.
type XShopPrices struct {
	Category  [16]uint32
	Overrides map[int]uint32
}

// Price is FUN_00402990: the override or the category price (category
// clamped to 0..15).
func (x *XShopPrices) Price(index int) uint32 {
	if p, ok := x.Overrides[index]; ok {
		return p
	}
	return x.Category[min(max(index>>9, 0), 15)]
}

func nonNegative(v int) uint32 {
	if v < 0 {
		return 0
	}
	return uint32(v)
}

// LoadXShopPrices reads path (FUN_004026e0). A missing file is not an
// error: every category costs XShopDefaultPrice and ok is false.
func LoadXShopPrices(path string) (x *XShopPrices, ok bool, err error) {
	x = &XShopPrices{Overrides: map[int]uint32{}}
	for i := range x.Category {
		x.Category[i] = XShopDefaultPrice
	}
	lines, err := mutext.LoadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return x, false, nil
		}
		return nil, false, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, false, fmt.Errorf("CustomXShop.txt: %w", err)
	}
	for _, s := range secs {
		for _, l := range s.Rows {
			switch s.Index {
			case 0:
				v, err := shopInts(l, 0, 2)
				if err != nil {
					return nil, false, fmt.Errorf("CustomXShop.txt: %w", err)
				}
				if v[0] >= 0 && v[0] < 16 {
					x.Category[v[0]] = nonNegative(v[1])
				}
			case 1:
				v, err := shopInts(l, 0, 3)
				if err != nil {
					return nil, false, fmt.Errorf("CustomXShop.txt: %w", err)
				}
				if i := v[0]*0x200 + v[1]; i >= 0 && i < 0x2000 {
					x.Overrides[i] = nonNegative(v[2])
				}
			}
		}
	}
	return x, true, nil
}

// xshopGMComment marks a row written by the GM price packet.
const xshopGMComment = "// alterado/criado ingame por GM"

// WriteXShopPrice is FUN_00403920: the GM price change rewrites
// CustomXShop.txt line by line. Inside section 1 a "Type Index Price" row
// of the item is replaced by "%d   %d   %lu" (keeping its "//" comment);
// without such a row a new "... // alterado/criado ingame por GM" line goes
// before the section's "end"; without a section 1 the lines "", "1", the
// new row and "end" are appended. The text goes to "<path>.tmp", which then
// replaces the file.
func WriteXShopPrice(path string, index int, price uint32) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	cat, idx := index>>9, index&0x1ff
	newRow := fmt.Sprintf("%d   %d   %d   %s", cat, idx, price, xshopGMComment)
	var out []string
	inSec, found, written := false, false, false
	// std::getline keeps a '\r' of CRLF lines; so does this split.
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		trimmed := strings.Trim(line, " \t\r\n")
		if trimmed == "" {
			out = append(out, line)
			continue
		}
		if !inSec {
			if trimmed == "1" {
				inSec, found = true, true
			}
			out = append(out, line)
			continue
		}
		if trimmed == "end" {
			if !written {
				out = append(out, newRow)
				written = true
			}
			inSec = false
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			out = append(out, line)
			continue
		}
		var t, i int
		var p uint64
		if n, _ := fmt.Sscanf(trimmed, "%d %d %d", &t, &i, &p); n == 3 && t == cat && i == idx {
			row := fmt.Sprintf("%d   %d   %d", cat, idx, price)
			if k := strings.Index(trimmed, "//"); k >= 0 {
				if c := strings.TrimSpace(trimmed[k:]); c != "" {
					row += "   " + c
				}
			}
			out = append(out, row)
			written = true
			continue
		}
		out = append(out, line)
	}
	if !found {
		out = append(out, "", "1", newRow, "end")
	}
	tmp := path + ".tmp"
	var buf bytes.Buffer
	for _, l := range out {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
