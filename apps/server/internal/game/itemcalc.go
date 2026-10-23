package game

import (
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Item formulas of CItem (CItem::Convert FUN_0048aed0, durability
// FUN_00494500, value FUN_0048c7a0, repair FUN_004946a0). They work on
// gamedata.ItemInfo, whose fields mirror the 0xd8-byte CItemInfo record the
// original copies before each computation.

// itemInfo returns the Item.txt row of an index.
func (w *World) itemInfo(index int) (gamedata.ItemInfo, bool) {
	info, ok := w.data.Items[index]
	return info, ok
}

// infoDurability is the CItemInfo durability: Durability + MagicDurability
// for weapons (FUN_00493990: local_78 += local_74).
func infoDurability(info gamedata.ItemInfo) int {
	if info.Category < 6 {
		return info.Durability + info.MagicDurability
	}
	if info.Category == 14 {
		return 1
	}
	return info.Durability
}

// infoTwoHand is CItemInfo+0x64 (FUN_00493990: Width > 1, weapons only).
func infoTwoHand(info gamedata.ItemInfo) bool { return info.Category < 6 && info.Width > 1 }

// convertExcellent applies the Convert rules that drop the excellent byte
// (no HaveOption, a few fixed indexes, wings slot 7, Dinorant/Fenrir).
func convertExcellent(info gamedata.ItemInfo, it item.Item) byte {
	exc := it.Excellent
	if info.HaveOption == 0 {
		exc = 0
	}
	switch it.Index {
	case 0x13, 0x40d, 0x812, 0xa0a, 0xa24, 0x1a03, 0x1a25:
		exc = 0
	}
	if info.Slot == 7 {
		exc = 0
	}
	return exc
}

// itemSetValid is CItem+0xb3: the ancient option survives only when both
// 2-bit fields are 1 or 2.
func itemSetValid(set byte) bool {
	a, b := set&3, set>>2&3
	return (a == 1 || a == 2) && (b == 1 || b == 2)
}

// maxDurability is FUN_00494500(index, level, excellent, set).
func maxDurability(info gamedata.ItemInfo, index, level int, exc, set bool) int {
	if index == 0x29 {
		return 0
	}
	switch index {
	case 0x1c15, 0x1c1d, 0x1c64, 0x1cd7, 0x1a12, 0x1a1d, 0x1a33, 0x1c13, 0x1c66, 0x1c6d, 0x1c6e:
		return 1
	}
	d := infoDurability(info)
	switch {
	case level < 5:
		d += level
	case level == 10:
		d += 0x11
	case level == 11:
		d += 0x15
	case level == 12:
		d += 0x1a
	case level == 13:
		d += 0x20
	case level == 14:
		d += 0x27
	case level == 15:
		d += 0x2f
	default:
		d += level*2 - 4
	}
	switch index {
	case 0x13, 0x40d, 0x812, 0xa0a, 0xa24:
	default:
		if info.Slot != 7 {
			if set {
				d += 0x14
			} else if exc {
				d += 0xf
			}
		}
	}
	if d > 0xff {
		return 0xff
	}
	return d
}

// itemMaxDurability is maxDurability for an item (Convert inputs).
func (w *World) itemMaxDurability(it item.Item) int {
	info, ok := w.itemInfo(it.Index)
	if !ok {
		return 0
	}
	return maxDurability(info, it.Index, it.Level, convertExcellent(info, it) != 0, itemSetValid(it.SetOption))
}

// itemReq are the requirements CItem::Convert computes (CItem+0x62..0x6c).
type itemReq struct {
	Str, Dex, Ene, Vit, Lead, Level int
}

// itemRequirements reproduces the requirement part of FUN_0048aed0.
// petLevel is CItem+0xc4 (Dark Horse / Dark Raven level).
func itemRequirements(info gamedata.ItemInfo, it item.Item, isCustomWing bool, petLevel int) itemReq {
	var r itemReq
	exc := convertExcellent(info, it) != 0
	set := itemSetValid(it.SetOption)
	lvl := it.Level
	base := info.Level
	if exc || set {
		base += 25
	}
	f := func(req, mul int) int { return ((base+lvl*3)*req*mul)/100 + 20 }
	if info.ReqStrength != 0 {
		r.Str = f(info.ReqStrength, 3)
	}
	if info.ReqDexterity != 0 {
		r.Dex = f(info.ReqDexterity, 3)
	}
	if info.ReqVitality != 0 {
		r.Vit = f(info.ReqVitality, 3)
	}
	if info.ReqEnergy != 0 {
		if info.Category == 5 && info.Slot == 1 {
			r.Ene = ((lvl+base)*info.ReqEnergy*3)/100 + 20
		} else {
			r.Ene = f(info.ReqEnergy, 4)
		}
	}
	if info.ReqLeadership != 0 {
		r.Lead = f(info.ReqLeadership, 3)
	}
	idx := it.Index
	if idx == 0x1a05 {
		r.Lead = petLevel*15 + 185
	}
	if info.ReqLevel != 0 {
		req := info.ReqLevel
		r.Level = req
		if idx >= 0x1800 {
			orb := (idx >= 0x1807 && idx <= 0x1818 && idx != 0x180f) || (idx >= 0x182c && idx <= 0x1830) ||
				(idx >= 0x1824 && idx <= 0x1828) || idx == 0x182b || idx == 0x1832 || (idx >= 0x1882 && idx <= 0x1887)
			switch {
			case (idx > 0x1802 && idx < 0x1807) || idx == 0x182a:
				r.Level = lvl*5 + req
			case orb:
			case idx > 0x1905 && idx < 0x190a:
				r.Level = lvl*4 + req
			case idx == 0x190a || idx == 0x190b:
			case isCustomWing:
			case idx == 0x1a04:
				r.Level = (petLevel + 0x6d) * 2
			default:
				r.Level = lvl*4 + req
			}
		}
	}
	if idx == 0x1a0a {
		if base < 3 {
			r.Level = 0x14
		} else {
			r.Level = 0x32
		}
	}
	if exc && r.Level != 0 && idx >= 0x1800 {
		r.Level += 0x14
	}
	return r
}

// requirements of an inventory item.
func (w *World) requirements(it item.Item) itemReq {
	info, _ := w.itemInfo(it.Index)
	_, custom := w.data.Custom.WingSlot(it.Index)
	return itemRequirements(info, it, custom, it.PetLevel)
}

// countExcellent is FUN_00512270.
func countExcellent(exc byte) int {
	n := 0
	for b := byte(1); b < 0x40; b <<= 1 {
		if exc&b != 0 {
			n++
		}
	}
	return n
}

// roundPrice applies the ">99: /10*10, >999: /100*100" rounding (the
// threshold of the first step is 9 for consumables).
func roundPrice(v int64, first int64) int64 {
	if v > first {
		v = v / 10 * 10
	}
	if v > 999 {
		v = v / 100 * 100
	}
	return v
}

// itemPrices are CItem+0x7c (buy) and CItem+0x78 (sell).
type itemPrices struct {
	Buy, Sell int64
}

// itemValueLookup is FUN_004a0df0 (CItemValue): index, level (-1 any),
// grade (-1 any, the excellent byte). For stackable items (except bolts
// and arrows) the value is multiplied by the durability.
func (w *World) itemValueLookup(it item.Item, exc byte) (int64, bool) {
	d := w.itemData
	if d == nil {
		return 0, false
	}
	for _, r := range d.values {
		if r.Index != it.Index || (r.Level != -1 && r.Level != it.Level) || (r.Grade != -1 && r.Grade != int(exc)) {
			continue
		}
		if d.stack.Max(it.Index, it.Level) != 0 && it.Index != 0x807 && it.Index != 0x80f {
			return int64(float64(r.Value) * float64(it.Durability)), true
		}
		return r.Value, true
	}
	return 0, false
}

// isWing reproduces the wing ranges FUN_0048c7a0 tests.
func isWingRange(idx int) bool {
	return (idx >= 0x1800 && idx <= 0x1806) || (idx >= 0x1824 && idx <= 0x182b) || idx == 0x1832 ||
		(idx >= 0x1906 && idx <= 0x1909)
}

// itemPrice is FUN_0048c7a0.
func (w *World) itemPrice(it item.Item) itemPrices {
	if it.IsEmpty() {
		return itemPrices{}
	}
	if p, ok := petPrice(it); ok {
		return p
	}
	info, ok := w.itemInfo(it.Index)
	if !ok {
		return itemPrices{}
	}
	exc := convertExcellent(info, it)
	maxDur := float32(w.itemMaxDurability(it))
	if v, ok := w.itemValueLookup(it, exc); ok || info.BuyMoney != 0 {
		if !ok {
			v = int64(info.BuyMoney)
		}
		return itemPrices{Buy: roundPrice(v, 99), Sell: roundPrice(v/3, 99)}
	}
	idx := it.Index
	var gold int64
	if price, ok := w.customJewelPrice(idx); ok {
		// CustomJewel.txt SalePrice (FUN_004650b0 branch of FUN_0048c7a0).
		gold = price
	} else if info.Value > 0 {
		gold = int64(info.Value*info.Value*10) / 12
		if idx >= 0x1c00 && idx <= 0x1c08 {
			if idx == 0x1c03 || idx == 0x1c06 {
				gold *= 2
			}
			gold = gold * (int64(1) << uint(it.Level&0x1f)) * int64(it.Durability)
			if gold > 2000000000 || gold < 0 {
				gold = 2000000000
			}
			buy := gold
			if buy > 9 {
				buy = buy / 10 * 10
			}
			sell := gold / 3
			if sell > 9 {
				sell = sell / 10 * 10
			}
			return itemPrices{Buy: buy, Sell: sell}
		}
	} else {
		lv := info.Level + it.Level*3
		if exc != 0 && idx < 0x1800 {
			lv += 25
		}
		cat := idx / 512
		_, custom := w.data.Custom.WingSlot(idx)
		cubic := false
		if cat == 12 && ((idx > 0x1806 && idx < 0x1824) || idx > 0x182b) && idx != 0x1832 &&
			(idx < 0x1906 || idx > 0x1909) && !custom {
			cubic = true
		} else if cat == 13 || cat == 15 {
			cubic = true
		}
		if cubic {
			gold = int64(lv*lv*lv) + 100
			if it.Option != 0 {
				gold *= int64(it.Option + 1)
			}
		} else {
			switch it.Level {
			case 5:
				lv += 4
			case 6:
				lv += 10
			case 7:
				lv += 25
			case 8:
				lv += 45
			case 9:
				lv += 65
			case 10:
				lv += 95
			case 11:
				lv += 135
			case 12:
				lv += 185
			case 13:
				lv += 245
			case 14:
				lv += 305
			case 15:
				lv += 365
			}
			l := int64(lv)
			if isWingRange(idx) || custom {
				gold = (l+40)*l*l*11 + 40000000
			} else {
				gold = (l+40)*l*l/8 + 100
			}
			if idx < 0xc00 && !infoTwoHand(info) {
				gold = gold * 80 / 100
			}
			if it.Luck {
				gold += gold * 25 / 100
			}
			if it.Option != 0 {
				gold += gold * int64(it.Option) * 60 / 100
			}
		}
		for n := countExcellent(exc); n > 0; n-- {
			if idx < 0x1800 {
				gold += gold * 100 / 100
			} else {
				gold += gold * 25 / 100
			}
		}
	}
	if gold > 2000000000 || gold < 0 {
		gold = 2000000000
	}
	buy := roundPrice(gold, 99)
	sell := gold / 3
	if info.Slot >= 0 && info.Slot < 12 && maxDur > 0 {
		cut := int64(math.RoundToEven(float64(float32(sell) * 0.6 * (1 - it.Durability/maxDur))))
		sell -= cut
	}
	return itemPrices{Buy: buy, Sell: roundPrice(sell, 99)}
}

// repairCost is FUN_004946a0 (repair of one item at an NPC; the caller
// multiplies nothing else). buy is the item's buy price (CItem+0x7c).
func (w *World) repairCost(it item.Item) int64 {
	if it.IsEmpty() {
		return 0
	}
	maxDur := float32(w.itemMaxDurability(it))
	if maxDur == it.Durability {
		return 0
	}
	info, ok := w.itemInfo(it.Index)
	if !ok || infoDurability(info) == 0 {
		return 0
	}
	buy := w.itemPrice(it).Buy
	base := buy / 3
	if it.Index == 0x1a04 || it.Index == 0x1a05 {
		base = buy
	}
	if base > 2000000000 {
		base = 2000000000
	}
	if base >= 100 {
		base = base / 10 * 10
	}
	if base >= 1000 {
		base = base / 100 * 100
	}
	s := float32(math.Sqrt(float64(base)))
	s2 := float32(math.Sqrt(float64(s)))
	cost := float32(float64(s2)*float64(s)*3.0*(1-float64(it.Durability)/float64(maxDur)) + 1)
	if it.Durability <= 0 {
		if it.Index == 0x1a04 || it.Index == 0x1a05 {
			cost += cost
		} else {
			cost = float32(float64(cost) * float64(float32(1.4)))
		}
	}
	c := int64(cost)
	if c >= 100 {
		c = c / 10 * 10
	}
	if c >= 1000 {
		c = c / 100 * 100
	}
	return c
}
