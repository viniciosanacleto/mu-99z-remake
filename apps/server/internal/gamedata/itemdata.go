package gamedata

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// ParseItemRef parses the "cc,iii" item reference used by the item, shop
// and drop tables into category*512 + index. "*" is -1.
func ParseItemRef(s string) (int, error) {
	if s == "*" {
		return -1, nil
	}
	cat, idx, ok := strings.Cut(s, ",")
	if !ok {
		return 0, fmt.Errorf("item reference %q: want cc,iii", s)
	}
	c, err := strconv.Atoi(cat)
	if err != nil {
		return 0, fmt.Errorf("item reference %q: %w", s, err)
	}
	i, err := strconv.Atoi(idx)
	if err != nil {
		return 0, fmt.Errorf("item reference %q: %w", s, err)
	}
	if c < 0 || c > 15 || i < 0 || i >= 512 {
		return 0, fmt.Errorf("item reference %q out of range", s)
	}
	return c*512 + i, nil
}

// ItemStackRule is a section 0 row of Item/ItemStack.txt (CItemStack):
// ItemIndex ItemLevel(-1 any) MaxStack.
type ItemStackRule struct {
	Index, Level, MaxStack int
}

// ItemStackPack is a section 1 row: ItemIndex ItemCount NextItemIndex.
type ItemStackPack struct {
	Index, Count, Next int
}

// ItemStack holds Item/ItemStack.txt.
type ItemStack struct {
	Rules []ItemStackRule
	Packs []ItemStackPack
}

// Max returns the stack limit of (index, level) (FUN_00495960 / FUN_004a0df0
// lookup: first rule whose level is -1 or equal), 0 when not stackable.
func (s *ItemStack) Max(index, level int) int {
	if s == nil {
		return 0
	}
	for _, r := range s.Rules {
		if r.Index == index && (r.Level == -1 || r.Level == level) {
			return r.MaxStack
		}
	}
	return 0
}

// LoadItemStack reads Data/Item/ItemStack.txt.
func LoadItemStack(dataDir string) (*ItemStack, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Item", "ItemStack.json"))
	if err != nil {
		return nil, err
	}
	secs, err := mutext.Sections(lines)
	if err != nil {
		return nil, fmt.Errorf("ItemStack.txt: %w", err)
	}
	out := &ItemStack{}
	for _, s := range secs {
		for _, l := range s.Rows {
			if len(l.Tokens) < 3 {
				return nil, fmt.Errorf("ItemStack.txt line %d: %d columns", l.No, len(l.Tokens))
			}
			idx, err := ParseItemRef(l.Tokens[0])
			if err != nil {
				return nil, fmt.Errorf("ItemStack.txt line %d: %w", l.No, err)
			}
			switch s.Index {
			case 0:
				lvl, err1 := l.Int(1)
				mx, err2 := l.Int(2)
				if err1 != nil || err2 != nil {
					return nil, fmt.Errorf("ItemStack.txt line %d: bad row", l.No)
				}
				out.Rules = append(out.Rules, ItemStackRule{idx, lvl, mx})
			case 1:
				cnt, err1 := l.Int(1)
				next, err2 := ParseItemRef(l.Tokens[2])
				if err1 != nil || err2 != nil {
					return nil, fmt.Errorf("ItemStack.txt line %d: bad row", l.No)
				}
				out.Packs = append(out.Packs, ItemStackPack{idx, cnt, next})
			}
		}
	}
	return out, nil
}

// ItemValueRule is an Item/ItemValue.txt row (CItemValue): ItemIndex
// ItemLevel(-1 any) ItemGrade(-1 any, compared with the excellent byte)
// MoneyValue.
type ItemValueRule struct {
	Index, Level, Grade int
	Value               int64
}

// LoadItemValue reads Data/Item/ItemValue.txt.
func LoadItemValue(dataDir string) ([]ItemValueRule, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Item", "ItemValue.json"))
	if err != nil {
		return nil, err
	}
	var out []ItemValueRule
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 4 {
			return nil, fmt.Errorf("ItemValue.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("ItemValue.txt line %d: %w", l.No, err)
		}
		lvl, err1 := l.Int(1)
		grade, err2 := l.Int(2)
		v, err3 := strconv.ParseInt(l.Tokens[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("ItemValue.txt line %d: bad row", l.No)
		}
		out = append(out, ItemValueRule{idx, lvl, grade, v})
	}
	return out, nil
}

// ItemMoveRule is an Item/ItemMove.txt row (CItemMove).
type ItemMoveRule struct {
	AllowDrop, AllowSell, AllowTrade, AllowVault bool
}

// LoadItemMove reads Data/Item/ItemMove.txt; items without a row are
// allowed everywhere (FUN_004f42b0 lookup returning end()).
func LoadItemMove(dataDir string) (map[int]ItemMoveRule, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Item", "ItemMove.json"))
	if err != nil {
		return nil, err
	}
	out := map[int]ItemMoveRule{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 5 {
			return nil, fmt.Errorf("ItemMove.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("ItemMove.txt line %d: %w", l.No, err)
		}
		var v [4]int
		for i := range v {
			if v[i], err = l.Int(1 + i); err != nil {
				return nil, err
			}
		}
		out[idx] = ItemMoveRule{v[0] != 0, v[1] != 0, v[2] != 0, v[3] != 0}
	}
	return out, nil
}

// ItemLootRule is an Item/ItemLoot.txt row (CItemLoot): ItemIndex
// ItemLevel(-1 any) AllowLoot.
type ItemLootRule struct {
	Index, Level int
	AllowLoot    bool
}

// LoadItemLoot reads Data/Item/ItemLoot.txt.
func LoadItemLoot(dataDir string) ([]ItemLootRule, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Item", "ItemLoot.json"))
	if err != nil {
		return nil, err
	}
	var out []ItemLootRule
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 3 {
			return nil, fmt.Errorf("ItemLoot.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		idx, err := ParseItemRef(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("ItemLoot.txt line %d: %w", l.No, err)
		}
		lvl, err1 := l.Int(1)
		allow, err2 := l.Int(2)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("ItemLoot.txt line %d: bad row", l.No)
		}
		out = append(out, ItemLootRule{idx, lvl, allow != 0})
	}
	return out, nil
}

// ShopItem is one Data/Shop/*.txt row.
type ShopItem struct {
	Index      int
	Level      int
	Durability int // 0 = the item's maximum durability
	Skill      bool
	Luck       bool
	Option     int
	Excellent  int
	SetOption  int
}

// ShopInfo is one Data/ShopManager.txt row (CShopManager, FUN_004f5300).
// Number is the row index (stored in the NPC's obj+0x31c).
type ShopInfo struct {
	Number   int
	NPCClass int
	Map      int
	X, Y     int
	Dir      int
	AL       [4]bool
	GMLevel  int // -1 = "*"
	Path     string
	Items    []ShopItem
}

// LoadShops reads Data/ShopManager.txt and every Data/Shop/<path>.txt it
// names.
func LoadShops(dataDir string) ([]ShopInfo, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "ShopManager.json"))
	if err != nil {
		return nil, err
	}
	var out []ShopInfo
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) != 11 {
			return nil, fmt.Errorf("ShopManager.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		var v [10]int
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		s := ShopInfo{Number: len(out), NPCClass: v[0], Map: v[1], X: v[2], Y: v[3], Dir: v[4],
			AL: [4]bool{v[5] != 0, v[6] != 0, v[7] != 0, v[8] != 0}, GMLevel: v[9], Path: l.Tokens[10]}
		if s.Items, err = loadShopItems(filepath.Join(dataDir, "Shop", s.Path+".json")); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// loadShopItems is FUN_004f4aa0: ItemIndex ItemLevel Durability
// SkillSwitch LuckSwitch Aditional Excellent SetOption.
func loadShopItems(path string) ([]ShopItem, error) {
	lines, err := jsonconf.LoadLines(path)
	if err != nil {
		return nil, err
	}
	var out []ShopItem
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < 8 {
			return nil, fmt.Errorf("%s line %d: %d columns", filepath.Base(path), l.No, len(l.Tokens))
		}
		idx, err := parseShopIndex(l.Tokens[0])
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", filepath.Base(path), l.No, err)
		}
		var v [7]int
		for i := range v {
			if v[i], err = l.Int(1 + i); err != nil {
				return nil, err
			}
		}
		out = append(out, ShopItem{Index: idx, Level: v[0], Durability: v[1], Skill: v[2] != 0,
			Luck: v[3] != 0, Option: v[4], Excellent: v[5], SetOption: v[6]})
	}
	return out, nil
}

// parseShopIndex accepts "cc,iii"; an out-of-range index becomes -1 like
// the original (local_18 = -1).
func parseShopIndex(s string) (int, error) {
	cat, idx, ok := strings.Cut(s, ",")
	if !ok {
		return 0, fmt.Errorf("item reference %q", s)
	}
	c, err1 := strconv.Atoi(cat)
	i, err2 := strconv.Atoi(idx)
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("item reference %q", s)
	}
	v := c*512 + i
	if v < 0 || v >= 0x2000 {
		return -1, nil
	}
	return v, nil
}
