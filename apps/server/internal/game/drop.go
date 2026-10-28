package game

import (
	"context"
	"fmt"
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Drops: the monster death drop (FUN_004b7290) with its option generation
// (MapRateInfo FUN_004afd90.., ItemOptionRate FUN_0049ea70..FUN_0049efc0,
// MakeNewOption FUN_0049f0d0, MakeSetOption FUN_0049f270), ItemDrop.txt
// (FUN_00492870), ItemMoneyDrop.txt (FUN_0049c920), the random ancient
// drop (FUN_004f3cb0), the item bags (CItemBagManager FUN_004913c0 /
// FUN_00491430 / FUN_00491340, CItemBagEx FUN_0048feb0), CustomMonster
// (FUN_00466d40 / FUN_00466f00), the item creation of FUN_004753e0 and the
// party zen split of the pick up (FUN_004982e0).

// dropTables are the data files of the drop module (loaded at start).
type dropTables struct {
	optionRate *gamedata.ItemOptionRate
	mapRate    []gamedata.MapRateInfo
	excRate    []gamedata.ExcellentOptionRate
	itemDrop   []gamedata.ItemDropRule
	moneyDrop  []gamedata.ItemMoneyDropRule
	guard      []int
	customMons []gamedata.CustomMonsterRow
	bags       []gamedata.BagEntry
	setTypes   *gamedata.SetItemTypes
	setOptions *gamedata.SetItemOptions
}

func loadDropTables(cfgDir string) (*dropTables, error) {
	t := &dropTables{}
	var err error
	if t.optionRate, err = gamedata.LoadItemOptionRate(cfgDir); err != nil {
		return nil, err
	}
	if t.mapRate, err = gamedata.LoadMapRateInfo(cfgDir); err != nil {
		return nil, err
	}
	if t.excRate, err = gamedata.LoadExcellentOptionRate(cfgDir); err != nil {
		return nil, err
	}
	if t.itemDrop, err = gamedata.LoadItemDrop(cfgDir); err != nil {
		return nil, err
	}
	if t.moneyDrop, err = gamedata.LoadItemMoneyDrop(cfgDir); err != nil {
		return nil, err
	}
	if t.guard, err = gamedata.LoadCustomGuard(cfgDir); err != nil {
		return nil, err
	}
	if t.customMons, err = gamedata.LoadCustomMonsterRows(cfgDir); err != nil {
		return nil, err
	}
	if t.bags, err = gamedata.LoadEventItemBags(cfgDir); err != nil {
		return nil, err
	}
	if t.setTypes, err = gamedata.LoadSetItemType(cfgDir); err != nil {
		return nil, err
	}
	if t.setOptions, err = gamedata.LoadSetItemOption(cfgDir); err != nil {
		return nil, err
	}
	return t, nil
}

// dt returns the drop tables (nil before start-up).
func (w *World) dt() *dropTables { return w.mw().tables }

func registerDrop(h *hooks) {
	// drop.go sorts before monster.go: the tables are loaded before the
	// start-up spawn so CustomMonster applies to it.
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		t, err := loadDropTables(w.configDir)
		if err != nil {
			return fmt.Errorf("drops: %w", err)
		}
		w.mw().tables = t
		return nil
	})
	h.itemBag = func(w *World, o *Object, bag, index, mp, x, y int) bool {
		return w.dropBySpecialValue(o, bag, index, mp, x, y)
	}
}

// --- option generation ----------------------------------------------------

// dropSpec is the argument list of FUN_004753e0 the drop code fills.
type dropSpec struct {
	Index, Level, Dur   int
	Skill, Luck, Option int
	Exc, Set            byte
	OptionEx            byte // parameter 14 (CItem+0xd2), set by the XShop (xshop.go)
	Duration            int  // seconds (not kept by item.Item, see doc 10b)
	ToInventory         bool
}

// pickOption is CItemOptionRate::GetItemOptionN: 0 when the row is missing.
func (w *World) pickOption(sec, row int) int {
	t := w.dt()
	if t == nil {
		return 0
	}
	v, _ := t.optionRate.Pick(sec, row, monsterRand)
	return v
}

// optionsFrom runs the six ItemOptionRate pickers on the rows of an
// ItemDrop / bag item row: the level byte starts as the row Level and the
// others as 0, each replaced only when its ItemOptionRate row exists.
func (w *World) optionsFrom(rows [6]int, level int) [6]int {
	out := [6]int{level}
	t := w.dt()
	if t == nil {
		return out
	}
	for i := range out {
		if v, ok := t.optionRate.Pick(i, rows[i], monsterRand); ok {
			out[i] = v
		}
	}
	return out
}

// maxNewOptions is the option cap of FUN_0049f0d0: 3 for 12,3 and 12,37,
// WingOption MaxNewOption (<= 6) for its wings, 4 for CustomWing items,
// otherwise 6.
func (w *World) maxNewOptions(index int) int {
	if index == 0x1a03 || index == 0x1a25 {
		return 3
	}
	if w.statsData != nil {
		if wo, ok := w.statsData.wingOptions[index]; ok {
			return min(wo.MaxNewOption, 6)
		}
		if _, ok := w.statsData.customWing(index); ok {
			return 4
		}
	}
	return 6
}

// makeNewOption is FUN_0049f0d0 (mode 0: the value is the bit mask; 1:
// count options; 2: rand % count options). Options are drawn from
// ExcellentOptionRate until count distinct bits are set (the original loops
// forever when it cannot find them; this stops after 1000 draws).
func (w *World) makeNewOption(index, mode, count int) byte {
	var out byte
	if mode == 0 {
		return byte(count)
	}
	if mode == 2 {
		if count <= 0 {
			return 0
		}
		count = monsterRand() % count
	}
	limit := w.maxNewOptions(index)
	var rates []gamedata.ExcellentOptionRate
	if t := w.dt(); t != nil {
		rates = t.excRate
	}
	for n, tries := 0, 0; n < count && tries < 1000; tries++ {
		if limit <= n {
			break
		}
		bit := byte(gamedata.PickExcellentOption(rates, index, monsterRand))
		if bit&out == 0 {
			out |= bit
			n++
		}
	}
	return out
}

// makeSetOption is FUN_0049f270: the ancient byte (low bits = tier 1/2,
// bits 2-3 = +5 (1) / +10 (2) stat level) for a SetOption value 1..6.
func (w *World) makeSetOption(index, val int) byte {
	t := w.dt()
	if t == nil || val == 0 {
		return 0
	}
	r, ok := t.setTypes.Get(index)
	if !ok {
		return 0
	}
	switch val {
	case 1, 2:
		var cand []byte
		if r.Option[0] > 0 {
			if val == 1 {
				cand = append(cand, 5)
			} else {
				cand = append(cand, 9)
			}
		}
		if r.Option[1] > 0 {
			if val == 2 {
				cand = append(cand, 6)
			} else {
				cand = append(cand, 10)
			}
		}
		if len(cand) == 0 {
			return 0
		}
		return cand[monsterRand()%len(cand)]
	case 3, 4:
		if r.Option[0] > 0 {
			if val == 3 {
				return 5
			}
			return 9
		}
	case 5, 6:
		if r.Option[1] > 0 {
			if val == 5 {
				return 6
			}
			return 10
		}
	}
	return 0
}

// --- item creation -----------------------------------------------------------

// createItem is the map / inventory part of FUN_004753e0: unknown indexes
// are refused, durability 0 becomes the maximum durability (FUN_00494500),
// the skill flag needs Item.txt Skill, the serial is requested later
// (0xFFFFFFFF) for items with HaveSerial. The item goes to the map with
// owner rights for owner, or into the owner's inventory (map 0xEB, C1 0C
// F3 14).
func (w *World) createItem(owner *Object, mp, x, y int, s dropSpec) bool {
	info, ok := w.data.Items[s.Index]
	if !ok {
		w.log.Debug("[CreateItem] ItemIndex does not exist", "index", s.Index)
		return false
	}
	it := item.Empty()
	it.Index = s.Index
	it.Level = s.Level
	it.Skill = s.Skill != 0 && info.Skill != 0
	it.Luck = s.Luck != 0
	it.Option = s.Option
	it.Excellent = s.Exc
	it.SetOption = s.Set
	it.OptionEx = s.OptionEx
	dur := s.Dur
	if dur == 0 {
		dur = maxDurability(info, s.Index, s.Level, s.Exc != 0, s.Set != 0)
	}
	it.Durability = float32(dur)
	if s.Duration > 0 {
		// FUN_004753e0 param_17 (time + duration), back in the DataServer
		// reply (FUN_004742b0 +0x2c): CItem+0xb7/+0xb8/+0xbc.
		it.Flag1, it.PeriodicActive, it.Expire = true, true, w.now().Unix()+int64(s.Duration)
	}
	if info.HaveSerial != 0 {
		it.Serial = math.MaxUint32
	}
	idx := -1
	if owner != nil {
		idx = owner.Index
	}
	if s.ToInventory && owner != nil && owner.P != nil {
		return w.giveInventoryItem(owner, it)
	}
	if w.hooks.dropMapItem == nil {
		return false
	}
	return w.hooks.dropMapItem(w, mp, x, y, it, idx)
}

// giveInventoryItem is the 0xEB branch of the DataServer reply
// (FUN_004742b0): serial, InventoryInsertItem (FUN_00495610) and C1 0C F3
// 14 (FUN_0049b620).
func (w *World) giveInventoryItem(o *Object, it item.Item) bool {
	if w.findBagSlot(&o.P.Inventory, it.Index) < 0 {
		return false
	}
	finish := func(it item.Item) {
		if o.State != StatePlaying || o.P == nil {
			return
		}
		if slot := w.insertInventory(o, it); slot >= 0 {
			w.send(o, packetItemModify(slot, o.P.Inventory[slot]))
			if it.Expire != 0 {
				w.send(o, packetPeriodicItem(0xFFFF, slot, it.Expire))
			}
		}
	}
	if it.Serial != math.MaxUint32 || w.store == nil {
		finish(it)
		return true
	}
	w.async(func(ctx context.Context) func() {
		serial, err := w.store.NextItemSerial(ctx)
		return func() {
			if err == nil {
				it.Serial = serial
				finish(it)
			}
		}
	})
	return true
}

// --- monster death drop -----------------------------------------------------------

// monsterItemDrop is FUN_004b7290.
func (w *World) monsterItemDrop(o *Object, killer int) {
	m := o.Mon
	owner := w.topHitter(o)
	if owner < 0 {
		owner = killer
	}
	if w.hooks.monsterKill != nil && validIdx(owner) && !validIdx(m.Owner) { // FUN_004b8f90 (killcount.go)
		w.hooks.monsterKill(w, o, w.objects[owner])
	}
	if owner < 0 || owner >= MaxObjects || (m.Owner >= 0 && m.Owner < MaxObjects) || noDropMonster(o.Class) {
		return
	}
	t := w.dt()
	if t != nil {
		for _, g := range t.guard { // CustomGuard (FUN_0044f040)
			if g == o.Class {
				return
			}
		}
	}
	if m.SpecialBag { // obj+0x24b4: CustomMonster SpecialBag
		return
	}
	p := w.objects[owner]
	if w.dropByMonsterClass(p, o) {
		return
	}
	// Quest item drop (CQuestObjective FUN_004de9b0).
	if w.hooks.questDrop != nil && w.hooks.questDrop(w, o) {
		return
	}
	if w.dropEventDrop(p, o) { // CDropEvent FUN_00471690 (dropevent.go)
		return
	}
	if w.itemDropRules(p, o) || w.moneyDropRules(p, o) {
		return
	}
	if o.Map >= 18 && o.Map <= 23 {
		return
	}
	al := accountLevel(p)
	itemMapRate, moneyMapRate, excRate, setRate := 100, 100, 0, 0
	if info, ok := w.data.Maps[o.Map]; ok {
		itemMapRate, moneyMapRate = info.ItemDropRate, info.MoneyDropRate
		excRate, setRate = info.ExcItemDropRate, info.SetItemDropRate
	}
	// owner obj+0x874 + obj+0x770 (100 + 0 without item options / buffs);
	// CBonusManager (FUN_004188f0) types 1, 3 and 4 (bonus.go).
	ownerRate := 100
	excRate = w.bonusValue(p, BonusExcDrop, excRate)
	setRate = w.bonusValue(p, BonusSetDrop, setRate)
	itemRate := w.bonusValue(p, BonusItemDrop, 100) * (itemMapRate * (w.cfg.ALInt("Common", "ItemDropRate", al, 100) * ownerRate / 100) / 100) / 100
	level, exc := m.Level, 0
	if m.Level > 24 {
		if rnd(1000000) < setRate {
			w.randomSetItemDrop(p, o.Map, o.X, o.Y)
			return
		}
		if rnd(1000000) < excRate {
			level -= 25
			exc = 1
		}
	}
	if idx := w.pickDropItemEx(level, exc != 0); idx >= 0 {
		if rnd(max(m.ItemRate, 1)) < itemRate {
			opt := [6]int{}
			for i := range opt {
				opt[i] = w.pickOption(i, gamedata.MapRateOption(t.mapRateRows(), o.Map, exc, i))
			}
			if idx >= 0x1800 { // level, skill, luck and excellent only for groups 0..11
				opt[0], opt[1], opt[2], opt[4] = 0, 0, 0, 0
			}
			s := dropSpec{Index: idx, Level: opt[0], Skill: opt[1], Luck: opt[2], Option: opt[3]}
			s.Exc = w.makeNewOption(idx, 1, opt[4])
			s.Set = w.makeSetOption(idx, opt[5])
			w.createItem(p, o.Map, o.X, o.Y, s)
			return
		}
	}
	if m.ExpGiven == 0 {
		return
	}
	if rnd(max(m.MoneyRate, 1)) < moneyMapRate/10 {
		amount := int64(w.cfg.ALInt("Common", "MoneyAmountDropRate", al, 100)) * m.ExpGiven / 100
		amount = int64(w.moneyRateOf(p)) * amount / 100
		if amount < 1 {
			amount = 1
		}
		if w.hooks.dropMapMoney != nil {
			w.hooks.dropMapMoney(w, o.Map, o.X, o.Y, int(amount), owner)
		}
	}
}

func (t *dropTables) mapRateRows() []gamedata.MapRateInfo {
	if t == nil {
		return nil
	}
	return t.mapRate
}

// moneyRateOf is obj+0x878 (MoneyAmountDropRate, 100 + item options).
func (w *World) moneyRateOf(p *Object) int {
	if p.P != nil && p.P.Stats != nil && p.P.Stats.MoneyAmountDropRate != 0 {
		return p.P.Stats.MoneyAmountDropRate
	}
	return 100
}

// pickDropItemEx is FUN_004ba8c0: a random item of the level table; the
// excellent drop only considers groups 0..11 (index < 0x1800).
func (w *World) pickDropItemEx(level int, exc bool) int {
	if level < 0 || level >= 150 {
		return -1
	}
	var list []int
	for _, idx := range w.mw().drops[level] {
		if !exc || idx < 0x1800 {
			list = append(list, idx)
		}
	}
	if len(list) == 0 {
		return -1
	}
	return list[rnd(len(list))]
}

// randomSetItemDrop is FUN_004f3cb0: a SetItemType item chosen by DropRate
// (FUN_004f3ad0, only Item.txt rows with DropItem), options of the
// MapRateInfo "set" line (drop type 2).
func (w *World) randomSetItemDrop(p *Object, mp, x, y int) {
	t := w.dt()
	if t == nil {
		return
	}
	idx := w.randomSetItem()
	if idx < 0 {
		return
	}
	opt := [6]int{}
	for i := range opt {
		opt[i] = w.pickOption(i, gamedata.MapRateOption(t.mapRate, mp, 2, i))
	}
	s := dropSpec{Index: idx, Level: opt[0], Skill: opt[1], Luck: opt[2], Option: opt[3]}
	s.Exc = w.makeNewOption(idx, 1, opt[4])
	s.Set = w.makeSetOption(idx, opt[5])
	w.createItem(p, mp, x, y, s)
}

// randomSetItem is FUN_004f3ad0.
func (w *World) randomSetItem() int {
	t := w.dt()
	var cand, rates []int
	for _, idx := range t.setTypes.Order {
		r := t.setTypes.ByIndex[idx]
		info, ok := w.data.Items[idx]
		if ok && info.DropItem != 0 && r.DropRate != 0 {
			cand = append(cand, idx)
			rates = append(rates, r.DropRate)
		}
	}
	total := 0
	for _, r := range rates {
		total += r
	}
	if total == 0 {
		return -1
	}
	x := monsterRand() % total
	for i, r := range rates {
		if x < r {
			return cand[i]
		}
		x -= r
	}
	return -1
}

// itemDropRules is FUN_00492870 (ItemDrop.txt, only on maps whose
// MapManager ItemDrop flag is set, value+0x30): every matching row passes
// its own rand % 1,000,000 < DropRate[AL] test (-1 always passes) and
// weighs 1,000,000 / DropRate; one of them drops.
func (w *World) itemDropRules(p, o *Object) bool {
	t := w.dt()
	if t == nil {
		return false
	}
	info, ok := w.data.Maps[o.Map]
	if !ok || info.ItemDrop == 0 {
		return false
	}
	al := accountLevel(p)
	var cand []gamedata.ItemDropRule
	var weights []int
	for _, r := range t.itemDrop {
		if _, ok := w.data.Items[r.Index]; !ok {
			continue
		}
		if (r.Map != -1 && r.Map != o.Map) || (r.Monster != -1 && r.Monster != o.Class) ||
			(r.MinLevel != -1 && o.Mon.Level < r.MinLevel) || (r.MaxLevel != -1 && o.Mon.Level > r.MaxLevel) {
			continue
		}
		rate := r.Rate[al]
		if rate == -1 {
			rate = 1000000
		} else if rate <= rnd(1000000) {
			continue
		}
		if wt := 1000000 / max(rate, 1); wt != 0 {
			cand = append(cand, r)
			weights = append(weights, wt)
		}
	}
	total := 0
	for _, v := range weights {
		total += v
	}
	if total == 0 {
		return false
	}
	x := monsterRand() % total
	for i, v := range weights {
		if x >= v {
			x -= v
			continue
		}
		r := cand[i]
		opt := w.optionsFrom(r.Option, r.Level)
		s := dropSpec{Index: r.Index, Level: opt[0], Dur: r.Grade, Skill: opt[1], Luck: opt[2],
			Option: opt[3], Duration: r.Duration}
		s.Exc = w.makeNewOption(r.Index, 1, opt[4])
		s.Set = w.makeSetOption(r.Index, opt[5])
		w.createItem(p, o.Map, o.X, o.Y, s)
		return true
	}
	return false
}

// moneyDropRules is FUN_0049c920 (ItemMoneyDrop.txt, same map flag): zen
// = MoneyAmount (or the monster experience, at least 1) * owner
// obj+0x878 / 100.
func (w *World) moneyDropRules(p, o *Object) bool {
	t := w.dt()
	if t == nil {
		return false
	}
	info, ok := w.data.Maps[o.Map]
	if !ok || info.ItemDrop == 0 {
		return false
	}
	al := accountLevel(p)
	var cand []gamedata.ItemMoneyDropRule
	var weights []int
	for _, r := range t.moneyDrop {
		if (r.Map != -1 && r.Map != o.Map) || (r.Monster != -1 && r.Monster != o.Class) ||
			(r.MinLevel != -1 && o.Mon.Level < r.MinLevel) || (r.MaxLevel != -1 && o.Mon.Level > r.MaxLevel) ||
			(r.AccountLevel != -1 && r.AccountLevel != al) {
			continue
		}
		rate := r.Rate
		if rate == -1 {
			rate = 1000000
		} else if rate <= rnd(1000000) {
			continue
		}
		if wt := 1000000 / max(rate, 1); wt != 0 {
			cand = append(cand, r)
			weights = append(weights, wt)
		}
	}
	total := 0
	for _, v := range weights {
		total += v
	}
	if total == 0 {
		return false
	}
	x := monsterRand() % total
	for i, v := range weights {
		if x >= v {
			x -= v
			continue
		}
		amount := int64(cand[i].Amount)
		if amount == -1 {
			amount = o.Mon.ExpGiven
			if amount == 0 {
				amount = 1
			}
		}
		amount = int64(w.moneyRateOf(p)) * amount / 100
		if amount < 1 {
			amount = 1
		}
		if w.hooks.dropMapMoney != nil {
			w.hooks.dropMapMoney(w, o.Map, o.X, o.Y, int(amount), p.Index)
		}
		return true
	}
	return false
}

// --- item bags ------------------------------------------------------------------

// dropByMonsterClass is FUN_004913c0: the first bag whose MonsterClass is
// the monster's class drops at the monster.
func (w *World) dropByMonsterClass(p, o *Object) bool {
	t := w.dt()
	if t == nil {
		return false
	}
	for i := range t.bags {
		if t.bags[i].MonsterClass == o.Class {
			w.bagDrop(t.bags[i].Bag, p, o.Map, o.X, o.Y)
			return true
		}
	}
	return false
}

// dropBySpecialValue is FUN_00491430: SpecialValue = bag type, SpecialLevel
// = index (-1 any). The SpecialState key (register EDX) is not decoded and
// matches any value.
func (w *World) dropBySpecialValue(p *Object, value, level, mp, x, y int) bool {
	t := w.dt()
	if t == nil {
		return false
	}
	for i := range t.bags {
		b := &t.bags[i]
		if b.SpecialValue == value && (b.SpecialLevel == -1 || b.SpecialLevel == level) {
			w.bagDrop(b.Bag, p, mp, x, y)
			return true
		}
	}
	return false
}

// dropByItemIndex is FUN_00491340: the bag opened by dropping an item
// (boxes, starter rings, medals...).
func (w *World) dropByItemIndex(p *Object, index, level, mp, x, y int) bool {
	t := w.dt()
	if t == nil {
		return false
	}
	for i := range t.bags {
		b := &t.bags[i]
		if b.ItemIndex == index && (b.ItemLevel == -1 || b.ItemLevel == level) {
			w.bagDrop(b.Bag, p, mp, x, y)
			return true
		}
	}
	return false
}

// bagDrop is CItemBagEx::DropItem (FUN_0048feb0): for every bag row of the
// map that passes rand % 10000 < DropRate, a section 4 row allowed for the
// player's class is chosen by SectionRate, and MinCount..MaxCount drops
// follow: set item (rand % 10000 < SetDropRate or Section -1), zen
// (rand % 100 >= ItemRate) or a random item of the section. Ends with the
// F3 40 firework of BagUseEffect.
func (w *World) bagDrop(b *gamedata.ItemBag, p *Object, mp, x, y int) {
	if b == nil || p == nil {
		return
	}
	cls := min(max(p.Class, 0), 4)
	changeUp := 0
	if p.P != nil && p.P.Char != nil {
		changeUp = int(p.P.Char.Class & 0x0F) // obj+0xe4
	}
	for bi, row := range b.Bags {
		if row.Map != -1 && row.Map != mp {
			continue
		}
		if rnd(10000) >= row.DropRate {
			continue
		}
		var cand []gamedata.BagDrop
		var rates []int
		for _, d := range row.Drops {
			c := d.Class[cls]
			if c != 0 && c <= changeUp+1 && d.SectionRate != 0 {
				cand = append(cand, d)
				rates = append(rates, d.SectionRate)
			}
		}
		total := 0
		for _, r := range rates {
			total += r
		}
		if total == 0 {
			continue
		}
		pick := monsterRand() % total
		var d gamedata.BagDrop
		for i, r := range rates {
			if pick < r {
				d = cand[i]
				break
			}
			pick -= r
		}
		if d.MinCount < 0 || d.MaxCount < d.MinCount || d.MaxCount > 1000 {
			continue
		}
		count := monsterRand()%(d.MaxCount-d.MinCount+1) + d.MinCount
		for n := 0; n < count; n++ {
			dx, dy := x, y
			if count > 1 || bi != 0 || !w.cellDroppable(mp, x, y) {
				var ok bool
				if dx, dy, ok = w.randomNearCell(mp, x, y, 2, 2, 10); !ok {
					dx, dy = p.X, p.Y
				}
			}
			if rnd(10000) < d.SetDropRate || d.Section == -1 {
				w.randomSetItemDrop(p, mp, dx, dy)
				continue
			}
			if rnd(100) >= b.ItemRate {
				if w.hooks.dropMapMoney != nil {
					w.hooks.dropMapMoney(w, mp, dx, dy, b.MoneyDrop, -1)
				}
				continue
			}
			items := b.Sections[d.Section]
			if len(items) == 0 {
				continue
			}
			it := items[uint32(monsterRand())%uint32(len(items))]
			if it.Index < 0 {
				continue
			}
			opt := w.optionsFrom(it.Option, it.Level)
			s := dropSpec{Index: it.Index, Level: opt[0], Skill: opt[1], Luck: opt[2], Option: opt[3],
				Duration: it.Duration, ToInventory: b.DropType != 0}
			s.Exc = w.makeNewOption(it.Index, 1, opt[4])
			s.Set = w.makeSetOption(it.Index, opt[5])
			if s.Exc == 0 {
				s.Exc = byte(it.Grade)
			}
			if w.isStackable(it.Index, opt[0]) {
				s.Dur = it.Grade
			}
			if s.ToInventory && p.P != nil && w.findBagSlot(&p.P.Inventory, it.Index) < 0 {
				s.ToInventory = false // FUN_004952a0 failed: ground drop
			}
			w.createItem(p, mp, dx, dy, s)
		}
	}
	switch b.BagUseEffect {
	case 1, 2:
		eff := byte(0)
		if b.BagUseEffect == 2 {
			eff = 2
		}
		pk := packetFirework(eff, byte(p.X), byte(p.Y))
		if p.IsPlayer() {
			w.send(p, pk)
		}
		w.broadcast(p, pk)
	case 3:
		pk := packetFirework(0x3A, byte(p.Index>>8), byte(p.Index))
		if p.IsPlayer() {
			w.send(p, pk)
		}
		w.broadcast(p, pk)
	}
}

// isStackable is FUN_004a09e0 on CItemStack (index, level -1 any).
func (w *World) isStackable(index, level int) bool {
	if w.itemData == nil || w.itemData.stack == nil {
		return false
	}
	return w.itemData.stack.Max(index, level) > 0
}

// packetFirework is C1 07 F3 40 [4] type [5] [6] (X/Y, or the index for
// type 0x3A).
func packetFirework(typ, a, b byte) []byte {
	p := newPacket(frame.C1, 0xF3, 0x40, 7)
	p[4], p[5], p[6] = typ, a, b
	return p
}

// randomNearCell is FUN_0050c860: up to tries draws of (x +- rand%(rx+1),
// y +- rand%(ry+1)) (the sign from another rand: odd = +), accepted when
// the cell has neither 0x04 nor 0x08.
func (w *World) randomNearCell(mp, x, y, rx, ry, tries int) (int, int, bool) {
	rx, ry = max(rx, 1), max(ry, 1)
	sign := func() int {
		if monsterRand()&1 != 0 {
			return 1
		}
		return -1
	}
	for i := 0; i < tries; i++ {
		a := monsterRand() % (rx + 1)
		nx := sign()*a + x
		b := monsterRand() % (ry + 1)
		ny := sign()*b + y
		if w.cellDroppable(mp, nx, ny) {
			return nx, ny, true
		}
	}
	return x, y, false
}

// --- dropping an item that opens a bag -----------------------------------------

// dropSpecialItem is the bag / Lost Map / firecracker branch of the 23
// handler (FUN_004988f0): consumed is true when the item was used instead
// of being placed on the map; refused when the drop must fail (a Lost Map
// that could not open its gate).
func (w *World) dropSpecialItem(o *Object, it item.Item, x, y int) (consumed, refused bool) {
	if w.dropByItemIndex(o, it.Index, it.Level, o.Map, x, y) {
		return true, false
	}
	if w.hooks.castleDrop != nil { // mercenary / life stone scrolls (castlesiege_battle.go)
		if handled, ok := w.hooks.castleDrop(w, o, it, x, y); handled {
			return ok, !ok
		}
	}
	if it.Index == itemLostMap { // FUN_004a2f20 (kalima.go)
		ok := w.lostMapDrop(o, it.Level, x, y)
		return ok, !ok
	}
	if it.Index == 0x1c3f || it.Index == 0x1c63 { // firecrackers: C1 07 F3 40 00 X Y
		pk := packetFirework(0, byte(x), byte(y))
		w.send(o, pk)
		w.broadcast(o, pk)
		return true, false
	}
	if w.hooks.summonScroll != nil { // SummonScroll.txt (summonscroll.go)
		if handled, ok := w.hooks.summonScroll(w, o, it, x, y); handled {
			return ok, !ok
		}
	}
	return false, false
}

// --- zen pick up -------------------------------------------------------------------

// partyMoneyDistribute is CServerInfo 0x5c4 (PartyMoneyDistribute).
var partyMoneyDistribute = func(w *World) bool { return w.cfg.Int("Common", "PartyMoneyDistribute", 0) != 0 }

// pickupMoney is the zen branch of FUN_004982e0: with PartyMoneyDistribute
// the amount is split among the party members on the same map within 10
// cells (picker included); each one gets C3 10 22 FE money.
func (w *World) pickupMoney(o *Object, amount int64) {
	pt := w.partyOf(o)
	if pt == nil || !partyMoneyDistribute(w) {
		addMoney(o, amount)
		w.sendMoney(o)
		return
	}
	var near []*Object
	for _, idx := range pt.members {
		if idx < 0 || idx >= MaxObjects {
			continue
		}
		m := w.objects[idx]
		if m.Map == o.Map && objDistance(o, m) < 10 && m.P != nil && m.P.Char != nil {
			near = append(near, m)
		}
	}
	if len(near) == 0 {
		return
	}
	share := amount / int64(len(near))
	for _, m := range near {
		addMoney(m, share)
		w.sendMoney(m)
	}
}

// --- CustomMonster -----------------------------------------------------------------

// applyCustomMonster is FUN_00466d40 (called by setMonster): percentages of
// life, damage, defense and rates; a SpecialBag marks obj+0x24b4 (no normal
// drop).
func (w *World) applyCustomMonster(o *Object) {
	t := w.dt()
	if t == nil || o.Mon == nil {
		return
	}
	r, ok := gamedata.FindCustomMonster(t.customMons, o.Class, o.Map)
	if !ok {
		return
	}
	m := o.Mon
	pct := func(v int) int64 {
		if v == -1 {
			return 100
		}
		return int64(v)
	}
	m.Life = float32(float64(int64(m.Life)*pct(r.MaxLife)) / 100)
	m.MaxLife = float32(float64(int64(m.MaxLife)*pct(r.MaxLife)) / 100)
	m.DamageMin = int(int64(m.DamageMin) * pct(r.DamageMin) / 100)
	m.DamageMax = int(int64(m.DamageMax) * pct(r.DamageMax) / 100)
	m.Defense = int(int64(m.Defense) * pct(r.Defense) / 100)
	m.AttackRate = int(int64(m.AttackRate) * pct(r.AttackRate) / 100)
	m.DefenseRate = int(int64(m.DefenseRate) * pct(r.DefenseRate) / 100)
	if r.SpecialBag != -1 {
		m.SpecialBag = true
	}
}

// customMonsterDeath is FUN_00466f00 (monster death): the SpecialBag drops
// through FUN_00491430(SpecialBag, -1) at the monster for the top hitter.
// The KillMessage notice (FUN_004c0c70) is left to the notice module.
func (w *World) customMonsterDeath(o *Object, killer int) {
	t := w.dt()
	if t == nil || o.Mon == nil {
		return
	}
	r, ok := gamedata.FindCustomMonster(t.customMons, o.Class, o.Map)
	if !ok || r.SpecialBag == -1 {
		return
	}
	owner := w.topHitter(o)
	if owner < 0 {
		owner = killer
	}
	if owner < 0 || owner >= MaxObjects {
		return
	}
	w.dropBySpecialValue(w.objects[owner], r.SpecialBag, -1, o.Map, o.X, o.Y)
}

// --- ancient (set) options in CalcCharacter ----------------------------------

// ancientBonus holds the object fields only the set options write
// (FUN_004f3240 codes 0x0e, 0x10, 0x12, 0x13).
type ancientBonus struct {
	Opt884          int // +0x884 (code 14)
	CriticalDamage  int // +0x848 (code 16)
	ExcellentDamage int // +0x850 (code 18)
	Opt830          int // +0x830 (code 19)
}

// ancientItemStats is FUN_004f2f70: every equipped ancient item (set bits
// & 3 != 0) with a SetItemType StatType adds ((set >> 2) & 3) * 5 to that
// stat (codes 0..4 only; no requirement check).
func (w *World) ancientItemStats(o *Object, s *statsState) {
	t := w.dt()
	if t == nil || o.P == nil {
		return
	}
	for slot := 0; slot < EquipSlots; slot++ {
		it := o.P.Inventory[slot]
		if it.IsEmpty() || it.SetOption&3 == 0 {
			continue
		}
		r, ok := t.setTypes.Get(it.Index)
		if !ok || r.StatType == -1 {
			continue
		}
		w.applyAncientOption(o, s, r.StatType, int(it.SetOption>>2&3)*5, true)
	}
}

// ancientSetCount is FUN_004f2d50: the valid equipped pieces of a set; two
// identical weapons (slots 0/1) or rings (10/11) of the same set count once.
func (w *World) ancientSetCount(o *Object, s *statsState, set int) int {
	t := w.dt()
	n, weapon, ring := 0, 0, 0
	inv := &o.P.Inventory
	for slot := 0; slot < EquipSlots; slot++ {
		it := inv[slot]
		if it.IsEmpty() || it.SetOption&3 == 0 || !s.Valid[slot] {
			continue
		}
		idx := t.setTypes.SetOptionIndex(it.Index, it.SetOption)
		if idx <= 0 || idx != set {
			continue
		}
		switch slot {
		case 0, 1:
			if weapon != 0 && weapon == idx && inv[0].Index == inv[1].Index {
				continue
			}
			if weapon == 0 {
				weapon = idx
			}
		case 10, 11:
			if ring != 0 && ring == idx && inv[10].Index == inv[11].Index {
				continue
			}
			if ring == 0 {
				ring = idx
			}
		}
		n++
	}
	return n
}

// ancientSetOptions is FUN_004f30f0: for every set with n pieces the
// options of tiers 0..n-2 apply; when n-1 equals the number of tiers with
// options the 5 full options apply too (obj+0x5d1). stats selects the
// pass: codes 0..4 (stats) or the others.
func (w *World) ancientSetOptions(o *Object, s *statsState, stats bool) {
	t := w.dt()
	if t == nil || o.P == nil {
		return
	}
	if !stats {
		s.Ancient = ancientBonus{}
	}
	for i := range t.setOptions {
		row := t.setOptions[i]
		if row.Index != i {
			continue
		}
		n := w.ancientSetCount(o, s, i)
		if n == 0 {
			continue
		}
		tiers := n - 1
		for k := 0; k < tiers && k < len(row.Tier); k++ {
			for _, op := range row.Tier[k] {
				if op.Code != -1 {
					w.applyAncientOption(o, s, op.Code, op.Value, stats)
				}
			}
		}
		if tiers == row.TierCount() {
			for _, op := range row.Full {
				if op.Code != -1 {
					w.applyAncientOption(o, s, op.Code, op.Value, stats)
				}
			}
		}
	}
}

// applyAncientOption is FUN_004f3240 (code, value, pass).
func (w *World) applyAncientOption(o *Object, s *statsState, code, v int, stats bool) {
	if (code >= 0 && code <= 4) != stats {
		return
	}
	pct := func(x int) int { return x*v/100 + x }
	div := func(x int) int {
		if v == 0 {
			return 0
		}
		return x / v
	}
	c := o.P.Char
	switch code {
	case 0:
		s.AddStr += v
	case 1:
		s.AddDex += v
	case 2:
		s.AddEne += v
	case 3:
		s.AddVit += v
	case 4:
		s.AddLead += v
	case 5:
		s.DamageMinLeft += v
		s.DamageMinRight += v
	case 8:
		s.DamageMinLeft += v
		s.DamageMinRight += v
		fallthrough
	case 6:
		s.DamageMaxLeft += v
		s.DamageMaxRight += v
	case 7:
		s.MagicDamageMin = pct(s.MagicDamageMin)
		s.MagicDamageMax = pct(s.MagicDamageMax)
	case 9:
		s.AttackSuccessRate = pct(s.AttackSuccessRate)
	case 10:
		s.Defense += v
	case 11:
		o.P.AddLife += v
	case 12:
		o.P.AddMana += v
	case 13:
		s.AddBP += v
	case 14:
		s.Ancient.Opt884 += v
	case 15:
		s.CriticalDamageRate += v
	case 16:
		s.Ancient.CriticalDamage += v
	case 17:
		s.ExcellentDamageRate += v
	case 18:
		s.Ancient.ExcellentDamage += v
	case 19:
		s.Ancient.Opt830 += v
	case 20:
		s.IncreaseDamage += v
	case 21:
		s.IgnoreDefenseRate += v
	case 22: // shield in the left hand (groups 6)
		if l := o.P.Inventory[1]; l.Index >= 0xc00 && l.Index < 0xe00 {
			s.Defense = pct(s.Defense)
		}
	case 23: // two-handed weapon in either hand
		if s.Items[0].TwoHand && s.Valid[0] || s.Items[1].TwoHand && s.Valid[1] {
			s.DamageMinLeft = pct(s.DamageMinLeft)
			s.DamageMinRight = pct(s.DamageMinRight)
			s.DamageMaxLeft = pct(s.DamageMaxLeft)
			s.DamageMaxRight = pct(s.DamageMaxRight)
		}
	case 24:
		d := div(c.Strength)
		s.DamageMinLeft += d
		s.DamageMinRight += d
		s.DamageMaxLeft += d
		s.DamageMaxRight += d
	case 25:
		d := div(c.Dexterity)
		s.DamageMinLeft += d
		s.DamageMinRight += d
		s.DamageMaxLeft += d
		s.DamageMaxRight += d
	case 26:
		s.Defense += div(c.Dexterity)
	case 27:
		s.Defense += div(c.Vitality)
	case 28:
		d := div(c.Energy)
		s.MagicDamageMin += d
		s.MagicDamageMax += d
	case 29, 30, 31, 32, 33, 34, 35:
		s.Resistance[code-29] = int(byte(s.Resistance[code-29] + v))
	}
}
