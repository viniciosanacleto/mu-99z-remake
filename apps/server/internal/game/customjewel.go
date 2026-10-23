package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// CCustomJewel (0x464b40..0x465810, item use 0x4c8000): the jewels of
// Custom/CustomJewel.txt. Using one on an item (head 26, after every fixed
// item of FUN_00499a20) checks the section 1 requirements of the target,
// rolls Rate_AL and applies the section 2 (success) or section 3
// (failure) changes; either way the jewel is consumed. The table also
// feeds the jewel test FUN_00496bf0 (auto pick, XShop) and the item price
// (FUN_0048c7a0: SalePrice).

// jewelItem is FUN_00496bf0: a CustomJewel.txt jewel or one of (12,15),
// (14,13), (14,14), (14,16), (14,22), (14,31), (14,41..44), (14,160),
// (14,161).
func (w *World) jewelItem(index int) bool {
	if _, ok := w.customJewel(index); ok {
		return true
	}
	switch index {
	case 12*512 + 15, 14*512 + 13, 14*512 + 14, 14*512 + 16, 14*512 + 22, 14*512 + 31,
		14*512 + 41, 14*512 + 42, 14*512 + 43, 14*512 + 44, 14*512 + 160, 14*512 + 161:
		return true
	}
	return false
}

// customJewel is FUN_004650b0.
func (w *World) customJewel(index int) (*gamedata.CustomJewel, bool) {
	if w.cash == nil {
		return nil, false
	}
	return w.cash.jewels.Find(index)
}

// customJewelPrice is the SalePrice branch of FUN_0048c7a0.
func (w *World) customJewelPrice(index int) (int64, bool) {
	j, ok := w.customJewel(index)
	if !ok {
		return 0, false
	}
	return int64(j.SalePrice), true
}

// useCustomJewel is the CustomJewel branch of FUN_00499a20: when the
// source item is a custom jewel and FUN_004c8000 accepts the target, the
// jewel is deleted (C1 05 28 src 01) and the target sent again (C1 0C F3
// 14). It reports whether the source was a custom jewel.
func (w *World) useCustomJewel(o *Object, src, dst int) bool {
	j, ok := w.customJewel(o.P.Inventory[src].Index)
	if !ok {
		return false
	}
	if w.applyCustomJewel(o, j, src, dst) {
		w.deleteInventory(o, src)
		w.send(o, packetItemDelete(src, 1))
		w.send(o, packetItemModify(dst, o.P.Inventory[dst]))
	}
	return true
}

// isWingIndex is the wing list of FUN_004650e0: 12,0..6, 12,36..43,
// 12,49, 12,50, 12,262..265 and 13,30.
func isWingIndex(idx int) bool {
	return (idx >= 0x1800 && idx <= 0x1806) || (idx >= 0x1824 && idx <= 0x182b) || idx == 0x1831 ||
		idx == 0x1832 || (idx >= 0x1906 && idx <= 0x1909) || idx == 0x1a1e
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// customJewelAccepts is FUN_004650e0 (with FUN_004653c0 for the
// excellent options and FUN_00465460 for the set option).
func (w *World) customJewelAccepts(j *gamedata.CustomJewel, t item.Item) bool {
	idx := t.Index
	if idx == 0x807 || idx == 0x80f {
		return false
	}
	_, cwing := w.statsData.customWing(idx)
	if idx > 0x1806 && !isWingIndex(idx) && !cwing {
		return false
	}
	if j.Weapon == 0 && idx >= 0 && idx < 0xc00 {
		return false
	}
	if j.Armor == 0 && idx >= 0xc00 && idx < 0x1800 {
		return false
	}
	if j.Wing == 0 && (isWingIndex(idx) || cwing) {
		return false
	}
	switch {
	case j.MinLevel != -1 && t.Level < j.MinLevel,
		j.MaxLevel != -1 && j.MaxLevel < t.Level,
		j.Skill != -1 && j.Skill < boolInt(t.Skill),
		j.Luck != -1 && j.Luck < boolInt(t.Luck),
		j.MinOption != -1 && t.Option < j.MinOption,
		j.MaxOption != -1 && j.MaxOption < t.Option:
		return false
	}
	n := countExcellent(t.Excellent)
	if j.MinExc != -1 && n < j.MinExc {
		return false
	}
	if j.MaxExc != -1 {
		if j.MaxExc > 10 && t.Excellent&byte(j.MaxExc-10) != 0 {
			return false
		}
		if j.MaxExc < 10 && j.MaxExc < n {
			return false
		}
	}
	if j.MaxSetOption != -1 {
		if t.SetOption&3 != 0 {
			return false
		}
		for k := 0; k < 2; k++ {
			if j.MaxSetOption&(1<<k) == 0 {
				continue
			}
			r, ok := w.setType(idx)
			if !ok || r.Option[k] < 1 {
				return false
			}
		}
	}
	return true
}

func (w *World) setType(idx int) (gamedata.SetItemType, bool) {
	t := w.dt()
	if t == nil {
		return gamedata.SetItemType{}, false
	}
	return t.setTypes.Get(idx)
}

// applyCustomJewel is FUN_004c8000: both slots in the bag range, a
// non-periodic target accepted by FUN_004650e0, then Rate_AL<account
// level> decides between the success and the failure changes.
func (w *World) applyCustomJewel(o *Object, j *gamedata.CustomJewel, src, dst int) bool {
	if src < 0 || src >= BagEnd || dst < 0 || dst >= BagEnd {
		return false
	}
	inv := &o.P.Inventory
	if inv[src].IsEmpty() || inv[dst].IsEmpty() || inv[dst].Flag1 {
		return false
	}
	t := &inv[dst]
	if !w.customJewelAccepts(j, *t) {
		return false
	}
	old := w.itemMaxDurability(*t)
	if w.rand(100) < j.Rate[min(max(o.P.Account.Level, 0), 3)] {
		s := j.Success
		t.Level = min(t.Level+s.Level, w.cfg.Int("Common", "MaxItemLevel", 0))
		t.Skill = byte(min(boolInt(t.Skill)+s.Skill, 1)) != 0
		t.Luck = byte(min(boolInt(t.Luck)+s.Luck, 1)) != 0
		t.Option = int(byte(min(t.Option+s.Option, w.cfg.Int("Common", "MaxItemOption", 0))))
		w.jewelExcellent(t, s.Excellent, true)
		w.jewelSetOption(t, s.SetOption)
	} else {
		f := j.Failure
		t.Level = max(t.Level-f.Level, 0)
		t.Skill = max(boolInt(t.Skill)-f.Skill, 0) != 0
		t.Luck = max(boolInt(t.Luck)-f.Luck, 0) != 0
		t.Option = max(t.Option-f.Option, 0)
		w.jewelExcellent(t, f.Excellent, false)
		if t.SetOption&3 != 0 && f.SetOption > 0 {
			t.SetOption = 0
		}
	}
	w.rescaleDurability(t, old)
	w.rebuildCharSet(o) // FUN_004c6ba0
	return true
}

// jewelExcellent is FUN_004655c0. Adding draws ExcellentOptionRate
// options until n new bits are set or the item has its maximum
// (maxNewOptions); removing clears n random bits among the first maximum
// bits, or every bit when the item has n or fewer. The original loops
// until it succeeds; this stops after 1000 draws.
func (w *World) jewelExcellent(t *item.Item, n int, add bool) {
	limit := w.maxNewOptions(t.Index)
	if !add {
		if countExcellent(t.Excellent) <= n {
			t.Excellent = 0
			return
		}
		for removed, tries := 0, 0; removed < n && tries < 1000; tries++ {
			bit := byte(1) << uint(chaosRand()%limit)
			if t.Excellent&bit != 0 {
				t.Excellent ^= bit
				removed++
			}
		}
		return
	}
	var rates []gamedata.ExcellentOptionRate
	if d := w.dt(); d != nil {
		rates = d.excRate
	}
	for added, tries := 0, 0; added < n && tries < 1000; tries++ {
		if countExcellent(t.Excellent) >= limit {
			return
		}
		bit := byte(gamedata.PickExcellentOption(rates, t.Index, monsterRand))
		if bit != 0 && t.Excellent&bit == 0 {
			t.Excellent |= bit
			added++
		}
	}
}

// jewelSetOption is FUN_00465810 in add mode. SetOption is a mask: bits
// 0/1 offer the set tiers 1/2 the item has in SetItemType (a random one of
// the two when both fit), bit 2 the +5 stat and bit 3 the +10 stat level.
// An item that is already ancient keeps its tier.
func (w *World) jewelSetOption(t *item.Item, val int) {
	if val == 0 {
		return
	}
	tier, stat := t.SetOption&3, t.SetOption&0xc
	if tier == 0 {
		r, ok := w.setType(t.Index)
		for k := 0; k < 2; k++ {
			if val&(1<<k) == 0 || !ok || r.Option[k] < 1 {
				continue
			}
			if tier == 0 || chaosRand()%2 == 0 {
				tier = byte(k + 1)
			}
		}
	}
	switch stat {
	case 0:
		if tier != 0 && val&4 != 0 {
			stat = 4
			if val&8 != 0 {
				stat = 8
			}
		}
	case 4:
		if tier != 0 && val&8 != 0 {
			stat = 8
		}
	}
	t.SetOption = stat + tier
}
