package gamedata

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

// Data of the last WizardTeam customs (docs/server-plan/
// 10e-customs-wizardteam.md): CustomJewel (CCustomJewel) and
// CustomLuckyWheel (CLuckyWheel) in config/data/custom.toml, CustomXShop
// (CCustomXShop, with the in-game GM price writer) in config/data/xshop.toml.

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

// CustomJewels is CustomJewel.txt.
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

func refIndex(cat, idx int) int {
	v := cat*512 + idx
	if v < 0 || v >= 0x2000 {
		return -1
	}
	return v
}

// LoadCustomJewel reads the CustomJewel table of config/data/custom.toml
// (FUN_00464b70). Indexes of 50 or more are ignored; a later row overwrites
// the slot.
func LoadCustomJewel(configDir string) (*CustomJewels, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	c := &CustomJewels{}
	for i := range c.Slots {
		c.Slots[i].Index = -1
	}
	ok := func(i int) bool { return i >= 0 && i < CustomJewelSlots }
	t := f.CustomJewel
	for i, r := range t.Jewel {
		cat, idx, err := SplitItemRef(r.ItemIndex)
		if err != nil {
			return nil, fmt.Errorf("CustomJewel.Jewel row %d: %w", i+1, err)
		}
		if !ok(r.JewelIndex) {
			continue
		}
		j := &c.Slots[r.JewelIndex]
		j.Index, j.ItemIndex = r.JewelIndex, refIndex(cat, idx)
		j.Weapon, j.Armor, j.Wing = r.EnableSlotWeapon, r.EnableSlotArmor, r.EnableSlotWing
		j.Rate = [4]int{r.Rate_AL0, r.Rate_AL1, r.Rate_AL2, r.Rate_AL3}
		j.SalePrice = r.SalePrice
	}
	for _, r := range t.Requirement {
		if !ok(r.JewelIndex) {
			continue
		}
		j := &c.Slots[r.JewelIndex]
		j.MinLevel, j.MaxLevel, j.Skill, j.Luck = r.MinItemLevel, r.MaxItemLevel, r.ItemWithSkill, r.ItemWithLuck
		j.MinOption, j.MaxOption = r.ItemMinAditional, r.ItemMaxAditional
		j.MinExc, j.MaxExc, j.MaxSetOption = r.MinItemExcellent, r.MaxItemExcellent, r.MaxItemSetOption
	}
	for k, rows := range [2][]CustomJewelChangeRow{t.Success, t.Failure} {
		for _, r := range rows {
			if !ok(r.JewelIndex) {
				continue
			}
			ch := JewelChange{r.ItemLevel, r.SkillSwitch, r.LuckSwitch, r.Aditional, r.Excellent, r.SetOption}
			if k == 0 {
				c.Slots[r.JewelIndex].Success = ch
			} else {
				c.Slots[r.JewelIndex].Failure = ch
			}
		}
	}
	return c, nil
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

// LoadLuckyWheel reads the CustomLuckyWheel table of config/data/custom.toml
// keyed by IndexWheel; the first row of an index wins (std::map insert).
func LoadLuckyWheel(configDir string) (map[int]WheelReward, error) {
	f, err := LoadCustomConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]WheelReward{}
	for _, r := range f.CustomLuckyWheel.Rows {
		if _, ok := out[r.IndexWheel]; ok {
			continue
		}
		out[r.IndexWheel] = WheelReward{Index: r.IndexWheel, Type: r.Type, Item: r.Index, Level: r.Level, Skill: r.Skill,
			Luck: r.Luck, Option: r.Option, Excellent: r.Excellent}
	}
	return out, nil
}

// --- CustomXShop ---------------------------------------------------------------------

// XShopDefaultPrice is the category price used when CustomXShop.txt cannot
// be opened (FUN_004026e0).
const XShopDefaultPrice = 25

// XShopPrices is CustomXShop.txt (CCustomXShop +0x34 and the
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

// LoadXShopPrices reads the CustomXShop table of the xshop.toml at path
// (FUN_004026e0). A missing file is not an error: every category costs
// XShopDefaultPrice and ok is false.
func LoadXShopPrices(path string) (x *XShopPrices, ok bool, err error) {
	x = &XShopPrices{Overrides: map[int]uint32{}}
	for i := range x.Category {
		x.Category[i] = XShopDefaultPrice
	}
	f, err := LoadXShopConfig(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return x, false, nil
		}
		return nil, false, err
	}
	for _, r := range f.CustomXShop.Category {
		if r.Type >= 0 && r.Type < 16 {
			x.Category[r.Type] = nonNegative(r.Price)
		}
	}
	for _, r := range f.CustomXShop.Item {
		if i := r.Type*0x200 + r.Index; i >= 0 && i < 0x2000 {
			x.Overrides[i] = nonNegative(r.Price)
		}
	}
	return x, true, nil
}

// xshopGMComment marks a row written by the GM price packet.
const xshopGMComment = "alterado/criado ingame por GM"

// WriteXShopPrice is FUN_00403920 on xshop.toml: the GM price change sets
// the Price of every CustomXShop.Item row of the item (keeping its
// comment); without such a row a new one commented "alterado/criado ingame
// por GM" is added. The file is rewritten through a temporary file.
func WriteXShopPrice(path string, index int, price uint32) error {
	var f XShopConfig
	if err := tomlconf.LoadUTF8(path, &f); err != nil {
		return err
	}
	cat, idx := index>>9, index&0x1ff
	found := false
	for i := range f.CustomXShop.Item {
		if r := &f.CustomXShop.Item[i]; r.Type == cat && r.Index == idx {
			r.Price = int(price)
			found = true
		}
	}
	if !found {
		f.CustomXShop.Item = append(f.CustomXShop.Item, XShopItemRow{Type: cat, Index: idx, Price: int(price),
			Comment: xshopGMComment})
	}
	return tomlconf.Save(path, &f)
}

// WriteXShopRecharge is the file part of FUN_00405350 on xshop.toml: the
// CustomXShopRecharge table is replaced (strings in Windows-1252, as the
// client sends them).
func WriteXShopRecharge(path string, r XShopRechargeFile) error {
	var f XShopConfig
	if err := tomlconf.LoadUTF8(path, &f); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, s := range []*string{&r.Titular, &r.ChavePix, &r.CopiaECola, &r.ImagemURL, &r.ImagemBMP, &r.Valor} {
		*s = tomlconf.UTF8(*s)
	}
	f.CustomXShopRecharge = r
	return tomlconf.Save(path, &f)
}
