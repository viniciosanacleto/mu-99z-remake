package game

import (
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

// mapItemsAt returns the valid map items of a map.
func (b *bareWorld) mapItemsAt(mp int) []mapItem {
	var out []mapItem
	t := b.mapItems(mp)
	for _, m := range t.items {
		if m.valid() {
			out = append(out, m)
		}
	}
	return out
}

// clearMapItems empties a map item table.
func (b *bareWorld) clearMapItems(mp int) {
	b.itemData.maps[mp] = newMapItemTable()
}

func TestMakeNewOptionAndSetOption(t *testing.T) {
	b := newBareWorld(t)
	// Sword (0,0) weights 1:60 2:60 4:50 8:20 16:20 32:10 (total 220).
	fixRand(t, 0, 0, 60, 200)
	if got := b.makeNewOption(0, 1, 2); got != 1|2 {
		t.Fatalf("2 options = %#x", got)
	}
	fixRand(t, 219)
	if got := b.makeNewOption(0, 1, 1); got != 32 {
		t.Fatalf("1 option = %#x", got)
	}
	if got := b.makeNewOption(0, 0, 0x21); got != 0x21 {
		t.Fatalf("mode 0 = %#x", got)
	}
	// Wings of Spirits (12,3) are capped at 3 options.
	fixRand(t, 0, 50, 100, 140, 150, 200, 0)
	if got := b.makeNewOption(0x1a03, 1, 6); countExcellent(got) != 3 {
		t.Fatalf("wing options = %#x", got)
	}
	// SetItemType 0,0: OptionIndex1 = 2, OptionIndex2 = 0.
	fixRand(t, 0)
	cases := map[int]byte{0: 0, 1: 5, 2: 9, 3: 5, 4: 9, 5: 0, 6: 0, 7: 0}
	for v, want := range cases {
		if got := b.makeSetOption(0, v); got != want {
			t.Errorf("set option %d = %d want %d", v, got, want)
		}
	}
	if got := b.makeSetOption(14*512+13, 1); got != 0 {
		t.Fatalf("non ancient = %d", got)
	}
}

func TestExcellentMonsterDrop(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	m.Mon.Level = 30
	info := b.data.Maps[0]
	info.ExcItemDropRate, info.SetItemDropRate, info.ItemDrop = 1000000, 0, 0
	b.data.Maps[0] = info
	b.clearMapItems(0)
	fixRand(t, 0)
	b.monsterItemDrop(m, p.Index)
	items := b.mapItemsAt(0)
	if len(items) != 1 {
		t.Fatalf("%d items", len(items))
	}
	it := items[0].Item
	// Level 30 - 25 = 5 table, groups 0..11 only; exc row 1: +0, skill,
	// no luck, 1 option (the first ExcellentOptionRate bit of the item).
	if it.Index >= 0x1800 || it.Excellent == 0 || it.Level != 0 || it.Luck || items[0].Owner != p.Index {
		t.Fatalf("item = %+v", it)
	}
	if countExcellent(it.Excellent) != 1 {
		t.Fatalf("exc = %#x", it.Excellent)
	}
	if want := b.itemMaxDurability(it); int(it.Durability) != want {
		t.Fatalf("durability %v want %d", it.Durability, want)
	}
}

func TestSetItemMonsterDrop(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	m.Mon.Level = 40
	info := b.data.Maps[0]
	info.SetItemDropRate, info.ItemDrop = 1000000, 0
	b.data.Maps[0] = info
	b.clearMapItems(0)
	fixRand(t, 0)
	b.monsterItemDrop(m, p.Index)
	items := b.mapItemsAt(0)
	if len(items) != 1 {
		t.Fatalf("%d items", len(items))
	}
	it := items[0].Item
	r, ok := b.dt().setTypes.Get(it.Index)
	// Set row 2: set value 1 (8000) -> tier 1 at +5 (5) for 0,0.
	if !ok || it.Index != b.dt().setTypes.Order[0] || r.Option[0] == 0 || it.SetOption != 5 || it.Excellent != 0 {
		t.Fatalf("set item = %+v", it)
	}
}

func TestBagDropByMonsterClass(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	m.Class = 78 // Golden Goblin -> Monster\Golden Goblin
	b.clearMapItems(0)
	fixRand(t, 0)
	b.monsterItemDrop(m, p.Index)
	items := b.mapItemsAt(0)
	if len(items) != 1 || items[0].Item.Index != 14*512+11 || items[0].Item.Level != 8 || items[0].X != m.X {
		t.Fatalf("goblin drop = %+v", items)
	}
}

func TestItemDropTxtJewel(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	m.Mon.Level = 20
	b.clearMapItems(0)
	// Every matching row passes rand % 1e6 < rate (0); the weighted pick
	// with 0 takes the first row (Jewel of Chaos, levels 12..150).
	fixRand(t, 0)
	if !b.itemDropRules(p, m) {
		t.Fatal("no ItemDrop")
	}
	items := b.mapItemsAt(0)
	if len(items) != 1 || items[0].Item.Index != 12*512+15 {
		t.Fatalf("drop = %+v", items)
	}
	// rand % 1e6 >= every rate: nothing.
	b.clearMapItems(0)
	fixRand(t, 999999)
	if b.itemDropRules(p, m) || len(b.mapItemsAt(0)) != 0 {
		t.Fatal("dropped with a failed roll")
	}
}

func TestPickupMoneyPartySplit(t *testing.T) {
	b := newBareWorld(t)
	a := statsPlayer(b, PlayerStart, 0, 50)
	c := statsPlayer(b, PlayerStart+1, 1, 60)
	far := statsPlayer(b, PlayerStart+2, 1, 60)
	c.X, c.Y = a.X+3, a.Y
	far.X, far.Y = a.X+30, a.Y
	b.handlePartyRequest(a, partyReq(c.Index))
	b.handlePartyAnswer(c, partyAns(1, a.Index))
	b.handlePartyRequest(a, partyReq(far.Index))
	b.handlePartyAnswer(far, partyAns(1, a.Index))
	for _, o := range []*Object{a, c, far} {
		o.P.Char.Money = 0
		b.drain(o)
	}
	old := partyMoneyDistribute
	partyMoneyDistribute = func(*World) bool { return true }
	t.Cleanup(func() { partyMoneyDistribute = old })
	b.pickupMoney(a, 1001)
	if a.P.Char.Money != 500 || c.P.Char.Money != 500 || far.P.Char.Money != 0 {
		t.Fatalf("money %d %d %d", a.P.Char.Money, c.P.Char.Money, far.P.Char.Money)
	}
	pk := find(b.drain(c), 0x22, 0xFE)
	if pk == nil || binary.BigEndian.Uint32(pk[4:]) != 500 {
		t.Fatalf("22 FE % X", pk)
	}
	partyMoneyDistribute = func(*World) bool { return false }
	b.pickupMoney(a, 100)
	if a.P.Char.Money != 600 || c.P.Char.Money != 500 {
		t.Fatal("no split without PartyMoneyDistribute")
	}
}

func TestDropItemOpensBag(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	box := item.Empty()
	box.Index, box.Level, box.Durability = 7179, 8, 1 // Box of Kundun 1
	slot := b.placeInventory(p, box, BagStart)
	b.clearMapItems(0)
	b.drain(p)
	fixRand(t, 0)
	b.handleDrop(p, []byte{0xC1, 0x06, 0x23, byte(x), byte(y), byte(slot)})
	if res := find(b.drain(p), 0x23, -1); res == nil || res[3] != 1 {
		t.Fatalf("23 = % X", res)
	}
	if !p.P.Inventory[slot].IsEmpty() {
		t.Fatal("box kept")
	}
	items := b.mapItemsAt(0)
	if len(items) == 0 {
		t.Fatal("bag dropped nothing")
	}
	for _, m := range items {
		if m.Item.Index == 7179 {
			t.Fatal("the box itself fell")
		}
	}
}

func TestCustomMonsterRates(t *testing.T) {
	b := newBareWorld(t)
	o := b.monsterByClass(0, 0)
	o.Map = 10 // CustomMonster rows: classes 274/275/293/338 on Icarus
	if !b.setMonster(o, 274) {
		t.Fatal("setMonster 274")
	}
	info := o.Mon.Info
	if o.Mon.MaxLife != float32(info.MaxLife*3) || o.Mon.Defense != info.Defense*3 ||
		o.Mon.DamageMin != info.DamageMin*250/100 || o.Mon.AttackRate != info.AttackRate*150/100 || o.Mon.SpecialBag {
		t.Fatalf("custom monster %+v info %+v", *o.Mon, *info)
	}
	o.Map = 0
	b.setMonster(o, 274)
	if o.Mon.MaxLife != float32(info.MaxLife) {
		t.Fatal("applied on another map")
	}
}

func TestAncientSetOptions(t *testing.T) {
	b := newBareWorld(t)
	o := statsPlayer(b, PlayerStart, 1, 100)
	o.P.Char.Strength, o.P.Char.Dexterity = 500, 500 // meet the requirements
	equip(o, 3, 8, 0, 0, 30)                         // Bronze Armor
	equip(o, 4, 9, 0, 0, 30)                         // Bronze Pants
	b.calcCharacter(o)
	baseMana := o.P.AddMana
	s := b.statsOf(o)
	if s.AddVit != 0 || s.AddEne != 0 || s.Ancient != (ancientBonus{}) {
		t.Fatalf("not ancient: vit %d ene %d %+v", s.AddVit, s.AddEne, s.Ancient)
	}
	// Two Hyperion pieces (set byte 5: tier 1, +5): item stat Vit +5 each,
	// tier 0 = Energy +15.
	o.P.Inventory[3].SetOption = 5
	o.P.Inventory[4].SetOption = 5
	b.calcCharacter(o)
	if s.AddVit != 10 || s.AddEne != 15 || s.AddDex != 0 || s.Ancient.Opt830 != 0 {
		t.Fatalf("2 pieces: vit %d ene %d dex %d %+v", s.AddVit, s.AddEne, s.AddDex, s.Ancient)
	}
	// Third piece: tier 1 = Agility +15 and, with both tiers, the full set
	// options 19 (+20 at obj+0x830) and 12 (+30 mana).
	equip(o, 6, 11, 0, 0, 30)
	o.P.Inventory[6].SetOption = 9 // +10 stat
	b.calcCharacter(o)
	if s.AddVit != 20 || s.AddEne != 15 || s.AddDex != 15 || s.Ancient.Opt830 != 20 || o.P.AddMana != baseMana+30 {
		t.Fatalf("full set: vit %d ene %d dex %d %+v mana %d/%d", s.AddVit, s.AddEne, s.AddDex, s.Ancient,
			o.P.AddMana, baseMana)
	}
	// An unusable piece (Valid false) does not count for the set.
	o.P.Inventory[6].Durability = 0
	if b.ancientSetCount(o, s, 3) != 3 {
		t.Fatal("durability is not a set requirement")
	}
	s.Valid[6] = false
	if b.ancientSetCount(o, s, 3) != 2 {
		t.Fatal("invalid piece counted")
	}
}

func TestItemMoneyDropGlobalRule(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	p := b.player(PlayerStart, "Alpha", 0, x, y)
	m := b.monsterNear(0, p, 1)
	m.Mon.ExpGiven = 1234
	var money int
	old := dropMapMoneyHook
	dropMapMoneyHook = func(w *World, mp, x, y, amount, owner int) bool { money = amount; return true }
	t.Cleanup(func() { dropMapMoneyHook = old })
	// Global row: rate 1000 of 1e6; amount "*" = the monster experience
	// times obj+0x878 (100) / 100.
	fixRand(t, 999)
	if !b.moneyDropRules(p, m) || money != 1234 {
		t.Fatalf("money %d", money)
	}
	money = 0
	fixRand(t, 1000)
	if b.moneyDropRules(p, m) || money != 0 {
		t.Fatal("dropped with a failed roll")
	}
}
