package gamedata

import (
	"fmt"
	"strconv"
	"strings"
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

// ItemStackRule is a section 0 row of ItemStack.txt (CItemStack):
// ItemIndex ItemLevel(-1 any) MaxStack.
type ItemStackRule struct {
	Index, Level, MaxStack int
}

// ItemStackPack is a section 1 row: ItemIndex ItemCount NextItemIndex.
type ItemStackPack struct {
	Index, Count, Next int
}

// ItemStack holds ItemStack.txt.
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

// LoadItemStack reads the ItemStack table of config/data/item.toml.
func LoadItemStack(configDir string) (*ItemStack, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := &ItemStack{}
	for i, r := range f.ItemStack.Stack {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemStack.Stack row %d: %w", i+1, err)
		}
		out.Rules = append(out.Rules, ItemStackRule{idx, r.ItemLevel, r.MaxStack})
	}
	for i, r := range f.ItemStack.Pack {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemStack.Pack row %d: %w", i+1, err)
		}
		next, err := ParseItemRef(r.NextItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemStack.Pack row %d: %w", i+1, err)
		}
		out.Packs = append(out.Packs, ItemStackPack{idx, r.ItemCount, next})
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

// LoadItemValue reads the ItemValue table of config/data/item.toml.
func LoadItemValue(configDir string) ([]ItemValueRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ItemValueRule
	for i, r := range f.ItemValue.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemValue row %d: %w", i+1, err)
		}
		out = append(out, ItemValueRule{idx, r.ItemLevel, r.ItemGrade, r.MoneyValue})
	}
	return out, nil
}

// ItemMoveRule is an Item/ItemMove.txt row (CItemMove).
type ItemMoveRule struct {
	AllowDrop, AllowSell, AllowTrade, AllowVault bool
}

// LoadItemMove reads the ItemMove table of config/data/item.toml; items
// without a row are allowed everywhere (FUN_004f42b0 lookup returning end()).
func LoadItemMove(configDir string) (map[int]ItemMoveRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]ItemMoveRule{}
	for i, r := range f.ItemMove.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemMove row %d: %w", i+1, err)
		}
		out[idx] = ItemMoveRule{r.AllowDrop != 0, r.AllowSell != 0, r.AllowTrade != 0, r.AllowVault != 0}
	}
	return out, nil
}

// ItemLootRule is an Item/ItemLoot.txt row (CItemLoot): ItemIndex
// ItemLevel(-1 any) AllowLoot.
type ItemLootRule struct {
	Index, Level int
	AllowLoot    bool
}

// LoadItemLoot reads the ItemLoot table of config/data/item.toml.
func LoadItemLoot(configDir string) ([]ItemLootRule, error) {
	f, err := loadItemConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ItemLootRule
	for i, r := range f.ItemLoot.Rows {
		idx, err := ParseItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("ItemLoot row %d: %w", i+1, err)
		}
		out = append(out, ItemLootRule{idx, r.ItemLevel, r.AllowLoot != 0})
	}
	return out, nil
}

// ShopItem is one Shop/*.txt row.
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

// ShopInfo is one ShopManager.txt row (CShopManager, FUN_004f5300).
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

// LoadShops reads config/data/shop.toml: the ShopManager rows and the item
// list of the shop each one names (FUN_004f5300, items FUN_004f4aa0).
func LoadShops(configDir string) ([]ShopInfo, error) {
	f, err := LoadShopConfig(configDir)
	if err != nil {
		return nil, err
	}
	var out []ShopInfo
	for _, r := range f.ShopManager.Rows {
		s := ShopInfo{Number: len(out), NPCClass: r.NPCIndex, Map: r.MapNumber, X: r.LocationX, Y: r.LocationY,
			Dir: r.Direction, AL: [4]bool{r.AL0 != 0, r.AL1 != 0, r.AL2 != 0, r.AL3 != 0}, GMLevel: r.GMLevel, Path: r.ShopPath}
		shop, ok := f.Shop[r.ShopPath]
		if !ok {
			return nil, fmt.Errorf("ShopManager: shop %q not found in Shop", r.ShopPath)
		}
		for i, it := range shop.Rows {
			idx, err := parseShopIndex(it.ItemIndex)
			if err != nil {
				return nil, fmt.Errorf("Shop.%q row %d: %w", r.ShopPath, i+1, err)
			}
			s.Items = append(s.Items, ShopItem{Index: idx, Level: it.ItemLevel, Durability: it.Durability,
				Skill: it.SkillSwitch != 0, Luck: it.LuckSwitch != 0, Option: it.Aditional, Excellent: it.Excellent,
				SetOption: it.SetOption})
		}
		out = append(out, s)
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
