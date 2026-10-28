package game

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/mapitem"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func mkItem(cat, idx, level int, dur float32) item.Item {
	return item.Item{Index: item.Make(cat, idx), Level: level, Durability: dur}
}

func TestMaxDurability(t *testing.T) {
	b := newBareWorld(t)
	kris := mkItem(0, 0, 0, 0) // Kris: Durability 20
	cases := []struct {
		level    int
		exc, set bool
		want     int
	}{
		{0, false, false, 20}, {3, false, false, 23}, {7, false, false, 30}, {10, false, false, 37},
		{11, false, false, 41}, {15, false, false, 67}, {0, true, false, 35}, {0, true, true, 40},
	}
	info, _ := b.itemInfo(kris.Index)
	for _, c := range cases {
		if got := maxDurability(info, kris.Index, c.level, c.exc, c.set); got != c.want {
			t.Errorf("Kris +%d exc=%v set=%v: %d, want %d", c.level, c.exc, c.set, got, c.want)
		}
	}
	if got := maxDurability(info, 0x1c15, 5, false, false); got != 1 {
		t.Errorf("fixed-durability item = %d", got)
	}
}

func TestRequirements(t *testing.T) {
	b := newBareWorld(t)
	kris := mkItem(0, 0, 0, 20)
	if r := b.requirements(kris); r.Str != 27 || r.Dex != 27 || r.Level != 0 {
		t.Fatalf("Kris +0 = %+v", r)
	}
	kris.Level = 3
	if r := b.requirements(kris); r.Str != 38 {
		t.Fatalf("Kris +3 str = %d", r.Str)
	}
	kris.Excellent = 1 // drop level 6+25
	kris.Level = 0
	if r := b.requirements(kris); r.Str != ((31*40*3)/100 + 20) {
		t.Fatalf("exc Kris str = %d", r.Str)
	}
	// Wings of Elf: ReqLevel 150 + level*4 (FUN_0048aed0 default branch).
	wings := mkItem(12, 0, 2, 200)
	if r := b.requirements(wings); r.Level != 158 {
		t.Fatalf("wings +2 level = %d", r.Level)
	}
}

func TestItemPrices(t *testing.T) {
	b := newBareWorld(t)
	cases := []struct {
		name      string
		it        item.Item
		buy, sell int64
	}{
		{"Kris", mkItem(0, 0, 0, 20), 240, 81},
		{"Kris half durability", mkItem(0, 0, 0, 10), 240, 57},
		{"Apple x1", mkItem(14, 0, 0, 1), 20, 6},
		{"Apple x50", mkItem(14, 0, 0, 50), 1000, 330},
		{"Small Healing Potion x50", mkItem(14, 1, 0, 50), 4150, 1380},
		{"Jewel of Bless (ItemValue.txt)", mkItem(14, 13, 0, 1), 9000000, 3000000},
		{"Wings of Elf", mkItem(12, 0, 0, 200), 55400000, 18466600},
	}
	for _, c := range cases {
		got := b.itemPrice(c.it)
		if got.Buy != c.buy || got.Sell != c.sell {
			t.Errorf("%s: %+v, want buy %d sell %d", c.name, got, c.buy, c.sell)
		}
	}
}

func TestRepairCostAndWarehouseFee(t *testing.T) {
	b := newBareWorld(t)
	k := mkItem(0, 0, 0, 20)
	if c := b.repairCost(k); c != 0 {
		t.Fatalf("full durability repair = %d", c)
	}
	// buy 240 -> base 80; sqrt(80)=8.944, sqrt(8.944)=2.9907;
	// 3*8.944*2.9907*(1-10/20)+1 = 41.12 -> 41.
	k.Durability = 10
	if c := b.repairCost(k); c != 41 {
		t.Fatalf("half durability repair = %d", c)
	}
	// Broken item: (3*8.944*2.9907 + 1) * 1.4 = 113.75 -> 113 -> 110.
	k.Durability = 0
	if c := b.repairCost(k); c != 110 {
		t.Fatalf("broken repair = %d", c)
	}
	for _, c := range []struct {
		level int
		pass  bool
		want  int64
	}{{1, false, 1}, {10, false, 4}, {50, false, 100}, {100, false, 400}, {100, true, 600}, {400, false, 6400}} {
		if got := warehouseFee(c.level, c.pass); got != c.want {
			t.Errorf("fee(%d,%v) = %d, want %d", c.level, c.pass, got, c.want)
		}
	}
}

func TestGridPlacement(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Grid", 0, 130, 130)
	// Kris is 1x2: first slot is 12, the next must start at 13.
	if s := b.insertInventory(o, mkItem(0, 0, 0, 20)); s != BagStart {
		t.Fatalf("first = %d", s)
	}
	if s := b.insertInventory(o, mkItem(0, 0, 0, 20)); s != BagStart+1 {
		t.Fatalf("second = %d", s)
	}
	// Bronze Armor is 2x2: needs columns 2..3 rows 0..1.
	if s := b.insertInventory(o, mkItem(8, 0, 0, 34)); s != BagStart+2 {
		t.Fatalf("armor = %d", s)
	}
	// Fill the bag with 1x1 apples: 64 cells - 2*2 (kris) - 4 (armor) = 56.
	n := 0
	for b.insertInventory(o, mkItem(14, 0, 0, 1)) >= 0 {
		n++
	}
	if n != 56 {
		t.Fatalf("apples = %d", n)
	}
	// Placement at a specific slot respects the occupancy.
	o.P.Inventory[BagStart+20] = item.Empty()
	if s := b.placeInventory(o, mkItem(0, 0, 0, 20), BagStart+20); s != -1 {
		t.Fatalf("1x2 into a 1x1 hole = %d", s)
	}
	if s := b.placeInventory(o, mkItem(14, 0, 0, 1), BagStart+20); s != BagStart+20 {
		t.Fatalf("apple into hole = %d", s)
	}
	// An item that would cross the bottom of the bag does not fit.
	g := b.occupancy(&o.P.Inventory, -1)
	if fitsAt(&g, BagStart+7*8, 1, 2) {
		t.Fatal("1x2 at the last row fits")
	}
}

func TestPacketLayouts(t *testing.T) {
	m := &mapitem.Item{X: 130, Y: 131, State: mapitem.StateNew, Money: 0x01020304}
	m.Item = item.Empty()
	m.Item.Index = mapitem.MoneyIndex
	e := mapitem.Entry(0x123, m)
	want := []byte{0x81, 0x23, 130, 131, 0x0F, 0x02, 0x03, 0x00, 0x04, 0xE0, 0x01}
	if !bytes.Equal(e, want) {
		t.Fatalf("money entry = % X", e)
	}
	p := mapitem.PacketItems([][]byte{e})
	if p[0] != 0xC2 || int(p[1])<<8|int(p[2]) != 16 || p[3] != 0x20 || p[4] != 1 {
		t.Fatalf("C2 20 = % X", p)
	}
	d := mapitem.PacketDestroy([]int{5, 0x101})
	if !bytes.Equal(d, []byte{0xC2, 0, 9, 0x21, 2, 0, 5, 1, 1}) {
		t.Fatalf("C2 21 = % X", d)
	}
	it := mkItem(0, 0, 7, 20)
	eq := packetEquipChange(0x53FC, 0, it)
	if eq[0] != 0xC1 || eq[1] != 0x0C || eq[2] != 0x25 || eq[3] != 0x53 || eq[4] != 0xFC || eq[5] != 0 ||
		eq[6] != 0<<4|3 {
		t.Fatalf("C1 0C 25 = % X", eq)
	}
	if l := packetLife(0xFF, 70000); !bytes.Equal(l[:8], []byte{0xC1, 0x0C, 0x26, 0xFF, 0xFD, 0xE8, 0, 0}) ||
		binary.LittleEndian.Uint32(l[8:]) != 70000 {
		t.Fatalf("life = % X", l)
	}
	if mp := packetMana(300, 70000); !bytes.Equal(mp[:8], []byte{0xC1, 0x10, 0x27, 0xFF, 0x01, 0x2C, 0xFD, 0xE8}) ||
		binary.LittleEndian.Uint32(mp[8:]) != 300 || binary.LittleEndian.Uint32(mp[12:]) != 70000 {
		t.Fatalf("mana = % X", mp)
	}
	if r := packetMoveResult(0xFF, 20, nil); !bytes.Equal(r, []byte{0xC3, 0x0C, 0x24, 0xFF, 20, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("move fail = % X", r)
	}
	if s := packetSellResult(1, 1234); !bytes.Equal(s, []byte{0xC1, 0x08, 0x33, 1, 0xD2, 0x04, 0, 0}) {
		t.Fatalf("sell = % X", s)
	}
	if w := packetWarehouseMoney(1, 5, 7); !bytes.Equal(w, []byte{0xC1, 0x0C, 0x81, 1, 5, 0, 0, 0, 7, 0, 0, 0}) {
		t.Fatalf("81 = % X", w)
	}
	if n := packetNpcWindow(2); !bytes.Equal(n, []byte{0xC3, 0x0B, 0x30, 2, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("30 = % X", n)
	}
	list := packetItemList([]item.Item{item.Empty(), mkItem(14, 0, 0, 3)})
	if list[0] != 0xC2 || list[3] != 0x31 || list[4] != 0 || list[5] != 1 || list[6] != 1 || list[7] != 0 || list[9] != 3 {
		t.Fatalf("31 = % X", list)
	}
	if tr := packetTradeResponse(1, "Bob", 300, 7); tr[1] != 0x14 || tr[3] != 1 || string(tr[4:7]) != "Bob" ||
		binary.LittleEndian.Uint16(tr[14:]) != 300 || binary.LittleEndian.Uint32(tr[16:]) != 7 {
		t.Fatalf("37 = % X", tr)
	}
}

func TestItemDataLoaders(t *testing.T) {
	dir := repoConfig()
	st, err := gamedata.LoadItemStack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Max(item.Make(14, 0), 3) != 255 || st.Max(item.Make(14, 21), 3) != 255 || st.Max(item.Make(14, 21), 0) != 0 {
		t.Fatalf("stack rules wrong: %+v", st.Rules[:3])
	}
	if len(st.Packs) != 1 || st.Packs[0].Next != item.Make(14, 28) {
		t.Fatalf("packs = %+v", st.Packs)
	}
	mv, err := gamedata.LoadItemMove(dir)
	if err != nil {
		t.Fatal(err)
	}
	if r := mv[item.Make(12, 0)]; r.AllowDrop || !r.AllowSell || !r.AllowTrade || !r.AllowVault {
		t.Fatalf("wings move = %+v", r)
	}
	shops, err := gamedata.LoadShops(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(shops) != 21 || shops[0].NPCClass != 230 || shops[11].NPCClass != 253 || shops[11].X != 127 {
		t.Fatalf("shops = %d, %+v", len(shops), shops[0])
	}
	if _, err := gamedata.ParseItemRef("14,013"); err != nil {
		t.Fatal(err)
	}
}

// --- handler tests on a bare world ---------------------------------------

func TestMoveInventory(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Mover", 0, 130, 130)
	o.Class = 1
	o.P.Char.Strength, o.P.Char.Dexterity = 1000, 1000
	o.P.Inventory[BagStart] = mkItem(0, 0, 0, 20) // Kris
	move := func(sf, ss, tf, ts byte) []byte {
		p := frame.Build(frame.C3, 0x24, append([]byte{sf, ss}, append(make([]byte, 7), tf, ts)...)...)
		b.handleItemMove(o, p)
		return find(b.drain(o), 0x24, -1)
	}
	r := move(0, BagStart, 0, BagStart+30)
	if r[3] != 0 || r[4] != BagStart+30 || r[5] != 0 || !o.P.Inventory[BagStart].IsEmpty() ||
		o.P.Inventory[BagStart+30].Index != 0 {
		t.Fatalf("bag move = % X", r)
	}
	// Equip it (right hand) and check the equipment broadcast to a viewer.
	v := b.player(PlayerStart+1, "Viewer", 0, 131, 130)
	b.see(o, v)
	r = move(0, BagStart+30, 0, 0)
	if r[3] != 0 || r[4] != 0 || o.P.Inventory[0].IsEmpty() {
		t.Fatalf("equip = % X", r)
	}
	pk := b.drain(v)
	if eq := find(pk, 0x25, -1); eq == nil || eq[6]>>4 != 0 {
		t.Fatalf("no C1 0C 25 to the viewer: %d packets", len(pk))
	}
	if find(pk, 0xF3, 0x13) == nil {
		t.Fatal("no F3 13 to the viewer")
	}
	// Armor cannot go into the weapon slot.
	o.P.Inventory[BagStart] = mkItem(8, 0, 0, 34)
	if r = move(0, BagStart, 0, 1); r[3] != 0xFF {
		t.Fatalf("armor in left hand = % X", r)
	}
	// Bag item onto an occupied cell fails.
	o.P.Inventory[BagStart+40] = mkItem(14, 0, 0, 1)
	if r = move(0, BagStart+40, 0, BagStart+8); r[3] != 0xFF {
		t.Fatalf("overlap = % X", r)
	}
}

func TestStackMergeOnMove(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Stack", 0, 130, 130)
	o.P.Inventory[BagStart] = mkItem(14, 1, 0, 200)
	o.P.Inventory[BagStart+1] = mkItem(14, 1, 0, 100)
	b.handleItemMove(o, frame.Build(frame.C3, 0x24, append([]byte{0, BagStart}, append(make([]byte, 7), 0, BagStart+1)...)...))
	pk := b.drain(o)
	if r := find(pk, 0x24, -1); r[3] != 0xFF {
		t.Fatalf("merge result = % X", r)
	}
	if o.P.Inventory[BagStart+1].Durability != 255 || o.P.Inventory[BagStart].Durability != 45 {
		t.Fatalf("stacks = %v %v", o.P.Inventory[BagStart].Durability, o.P.Inventory[BagStart+1].Durability)
	}
	if d := find(pk, 0x2A, -1); d == nil || d[3] != BagStart+1 || d[4] != 255 {
		t.Fatalf("durability packet = % X", d)
	}
}

func fakeNPC(b *bareWorld, index, class, mp, x, y int) *Object {
	n := b.objects[index]
	n.reset()
	n.State, n.Type, n.Class = StatePlaying, TypeNPC, class
	n.Map, n.X, n.Y = mp, x, y
	n.Live, n.Alive = LiveNormal, true
	return n
}

func TestShopBuySellRepair(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 127, 88)
	o.P.Char.Money = 100000
	amy := fakeNPC(b, 100, 253, 0, 127, 86) // Potion Girl Amy (ShopManager row 11)
	h := b.hooks.npcTalk[253]
	if h == nil || !h(b.World, o, amy) {
		t.Fatal("shop talk not handled")
	}
	pk := b.drain(o)
	if p := find(pk, 0x30, 0); p == nil || p[0] != 0xC3 {
		t.Fatal("no C3 0B 30 00")
	}
	list := find(pk, 0x31, -1)
	if list == nil || list[5] == 0 || list[6] != 0 || list[7] != 0 || list[8]>>3 != 0 || list[12]&0xF0 != 0xE0 {
		t.Fatalf("shop list = % X", list)
	}
	// Buy the apple at position 0 (50 apples, 1000 zen).
	b.handleBuy(o, frame.Build(frame.C3, 0x32, 0))
	pk = b.drain(o)
	r := find(pk, 0x32, -1)
	if r == nil || r[3] != BagStart || r[4] != 0 || r[6] != 50 {
		t.Fatalf("buy = % X", r)
	}
	if m := find(pk, 0x22, 0xFE); m == nil || binary.BigEndian.Uint32(m[4:]) != 99000 || o.P.Char.Money != 99000 {
		t.Fatalf("money after buy = %d (% X)", o.P.Char.Money, m)
	}
	// Buying again merges into the stack (255 max): result 0xFF, money paid.
	b.handleBuy(o, frame.Build(frame.C3, 0x32, 0))
	pk = b.drain(o)
	if r := find(pk, 0x32, -1); r[3] != 0xFF || o.P.Inventory[BagStart].Durability != 100 || o.P.Char.Money != 98000 {
		t.Fatalf("stack buy = % X, dur %v money %d", r, o.P.Inventory[BagStart].Durability, o.P.Char.Money)
	}
	// Sell it back: 100 apples -> 2000/3 = 666 -> 660.
	b.handleSell(o, frame.Build(frame.C3, 0x33, BagStart))
	r = find(b.drain(o), 0x33, -1)
	if r[3] != 1 || binary.LittleEndian.Uint32(r[4:]) != 98660 || !o.P.Inventory[BagStart].IsEmpty() {
		t.Fatalf("sell = % X", r)
	}
	// Repair a damaged Kris: 41 zen.
	o.P.Inventory[BagStart] = mkItem(0, 0, 0, 10)
	b.handleRepair(o, frame.Build(frame.C3, 0x34, BagStart, 0))
	pk = b.drain(o)
	if r := find(pk, 0x34, -1); r == nil || binary.LittleEndian.Uint32(r[3+1:]) != 98619 ||
		o.P.Inventory[BagStart].Durability != 20 {
		t.Fatalf("repair = % X dur %v", r, o.P.Inventory[BagStart].Durability)
	}
	// Close the window (31): buying is refused afterwards.
	b.closeNpcWindow(o)
	b.handleBuy(o, frame.Build(frame.C3, 0x32, 0))
	if r := find(b.drain(o), 0x32, -1); r[3] != 0xFF {
		t.Fatalf("buy after close = % X", r)
	}
}

func TestDropAndPickup(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := b.player(PlayerStart, "Looter", 0, x, y)
	// Jewels are AllowDrop 0 in ItemMove.txt; drop a Jewel of Chaos-free
	// item: Small Healing Potion x3.
	o.P.Inventory[BagStart] = mkItem(14, 1, 0, 3)
	o.P.Inventory[BagStart+1] = mkItem(14, 13, 0, 1)
	b.handleDrop(o, frame.Build(frame.C3, 0x23, byte(x+1), byte(y), BagStart+1))
	if r := find(b.drain(o), 0x23, -1); r[3] != 0 {
		t.Fatalf("jewel drop = % X", r)
	}
	o.P.Inventory[BagStart+1] = item.Empty()
	b.handleDrop(o, frame.Build(frame.C3, 0x23, byte(x+1), byte(y), BagStart))
	if r := find(b.drain(o), 0x23, -1); !bytes.Equal(r, []byte{0xC1, 0x05, 0x23, 1, BagStart}) {
		t.Fatalf("drop = % X", r)
	}
	// The viewport cycle shows it with the "new" flag.
	b.viewportProc()
	pk := b.drain(o)
	c := find(pk, 0x20, -1)
	if c == nil || c[4] != 1 || c[5]&0x80 == 0 || c[7] != byte(x+1) || c[9] != 1 || c[11] != 3 || c[14]&0xF0 != 0xE0 {
		t.Fatalf("C2 20 = % X", c)
	}
	num := int(c[5]&0x7F)<<8 | int(c[6])
	// Another player cannot take it while the owner protection lasts.
	other := b.player(PlayerStart+1, "Thief", 0, x+2, y)
	b.handlePickup(other, frame.Build(frame.C3, 0x22, byte(num>>8), byte(num)))
	if r := find(b.drain(other), 0x22, -1); r[3] != 0xFF {
		t.Fatalf("thief pick = % X", r)
	}
	b.handlePickup(o, frame.Build(frame.C3, 0x22, byte(num>>8), byte(num)))
	pk = b.drain(o)
	r := find(pk, 0x22, -1)
	if len(r) != 16 || r[3] != BagStart || r[4] != 1 || r[6] != 3 || o.P.Inventory[BagStart].Index != item.Make(14, 1) {
		t.Fatalf("pick = % X", r)
	}
	// Next cycle: the map item is removed (C2 21) and then cleared.
	b.mapItemProc()
	b.viewportProc()
	if d := find(b.drain(o), 0x21, -1); d == nil || int(d[5])<<8|int(d[6]) != num {
		t.Fatalf("C2 21 = % X", d)
	}
	// Excellent items and +5 weapons cannot be dropped.
	o.P.Inventory[BagStart+1] = mkItem(0, 0, 5, 20)
	b.handleDrop(o, frame.Build(frame.C3, 0x23, byte(x+1), byte(y), BagStart+1))
	if r := find(b.drain(o), 0x23, -1); r[3] != 0 {
		t.Fatalf("+5 drop = % X", r)
	}
}

func TestMoneyPickupAndExpiry(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := b.player(PlayerStart, "Rich", 0, x, y)
	o.P.Char.Money = 10
	if !b.hooks.dropMapMoney(b.World, 0, x, y, 5000, -1) {
		t.Fatal("money drop failed")
	}
	b.viewportProc()
	c := find(b.drain(o), 0x20, -1)
	num := int(c[5]&0x7F)<<8 | int(c[6])
	b.handlePickup(o, frame.Build(frame.C3, 0x22, byte(num>>8), byte(num)))
	if m := find(b.drain(o), 0x22, 0xFE); m == nil || binary.BigEndian.Uint32(m[4:]) != 5010 {
		t.Fatalf("money pick = % X", m)
	}
	// An item left on the ground expires after ItemDropTime seconds.
	b.hooks.dropMapItem(b.World, 0, x+1, y, mkItem(14, 0, 0, 1), -1)
	tbl := b.mapItems(0)
	b.advance(time.Duration(b.cfg.Int("Common", "ItemDropTime", 0)+1) * time.Second)
	b.mapItemProc() // new -> normal
	b.mapItemProc() // expired -> removed
	found := false
	for _, m := range tbl.Items {
		if m.Valid() && m.Item.Index == item.Make(14, 0) && m.State == mapitem.StateRemoved {
			found = true
		}
	}
	if !found {
		t.Fatal("item did not expire")
	}
}

func TestUsePotionAndJewel(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Drinker", 0, 130, 130)
	c := o.P.Char
	c.MaxLife, c.Life = 1000, 100
	o.P.Inventory[BagStart] = mkItem(14, 3, 0, 2) // Large Healing Potion x2: DW rate 30
	b.handleUseItem(o, frame.Build(frame.C3, 0x26, BagStart, 0, 0))
	pk := b.drain(o)
	if l := find(pk, 0x26, 0xFD); l == nil || l[5] != 100 {
		t.Fatalf("unlock packet = % X", l)
	}
	if l := find(pk, 0x26, 0xFF); l == nil || binary.LittleEndian.Uint32(l[8:]) != 400 || c.Life != 400 {
		t.Fatalf("life = %v (% X)", c.Life, l)
	}
	if d := find(pk, 0x2A, -1); d == nil || d[3] != BagStart || d[4] != 1 {
		t.Fatalf("durability = % X", d)
	}
	// The anti auto-potion check blocks a second potion within 250 ms.
	b.handleUseItem(o, frame.Build(frame.C3, 0x26, BagStart, 0, 0))
	if find(b.drain(o), 0x26, 0xFF) != nil {
		t.Fatal("potion within the tolerance")
	}
	b.advance(300 * time.Millisecond)
	b.handleUseItem(o, frame.Build(frame.C3, 0x26, BagStart, 0, 0))
	pk = b.drain(o)
	if d := find(pk, 0x28, -1); d == nil || d[3] != BagStart || !o.P.Inventory[BagStart].IsEmpty() || c.Life != 700 {
		t.Fatalf("last potion: life %v % X", c.Life, d)
	}
	// Jewel of Bless on a +0 Kris: +1, jewel deleted, F3 14 sent.
	o.P.Inventory[BagStart] = mkItem(14, 13, 0, 1)
	o.P.Inventory[BagStart+1] = mkItem(0, 0, 0, 20)
	b.handleUseItem(o, frame.Build(frame.C3, 0x26, BagStart, BagStart+1, 0))
	pk = b.drain(o)
	if o.P.Inventory[BagStart+1].Level != 1 || !o.P.Inventory[BagStart].IsEmpty() {
		t.Fatalf("bless: %+v", o.P.Inventory[BagStart+1])
	}
	// durability rescaled to the new maximum (21).
	if o.P.Inventory[BagStart+1].Durability != 21 {
		t.Fatalf("bless durability %v", o.P.Inventory[BagStart+1].Durability)
	}
	if m := find(pk, 0xF3, 0x14); m == nil || m[4] != BagStart+1 || m[6]>>3&0xF != 1 {
		t.Fatalf("F3 14 = % X", m)
	}
}

func TestWarehouseMoneyAndMoves(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Banker", 0, 130, 130)
	o.P.Char.Money, o.P.Char.Level = 10000, 100
	st := items(o)
	st.If = ifState{Use: true, Type: ifWarehouse}
	b.warehouseLoaded(o, &store.Warehouse{Items: store.EmptyItems(WarehouseSlots), Money: 0})
	pk := b.drain(o)
	if find(pk, 0x30, 2) == nil || find(pk, 0x31, -1) == nil || find(pk, 0x81, 1) == nil || find(pk, 0x83, -1) == nil {
		t.Fatalf("open packets: %d", len(pk))
	}
	deposit := func(typ byte, amount uint32) []byte {
		p := make([]byte, 5)
		p[0] = typ
		binary.LittleEndian.PutUint32(p[1:], amount)
		b.handleWarehouseMoney(o, frame.Build(frame.C1, 0x81, p...))
		return find(b.drain(o), 0x81, -1)
	}
	if r := deposit(0, 6000); r[3] != 1 || st.WarehouseMoney != 6000 || o.P.Char.Money != 4000 {
		t.Fatalf("deposit = % X", r)
	}
	// Withdraw 1000: fee for level 100 is 400, paid from the inventory zen.
	if r := deposit(1, 1000); r[3] != 1 || st.WarehouseMoney != 5000 || o.P.Char.Money != 4600 {
		t.Fatalf("withdraw = % X wh %d inv %d", r, st.WarehouseMoney, o.P.Char.Money)
	}
	if r := deposit(0, 999999); r[3] != 0 {
		t.Fatalf("over-deposit = % X", r)
	}
	// Inventory -> warehouse -> inventory.
	o.P.Inventory[BagStart] = mkItem(0, 0, 0, 20)
	b.handleItemMove(o, frame.Build(frame.C3, 0x24, append([]byte{0, BagStart}, append(make([]byte, 7), 2, 5)...)...))
	if r := find(b.drain(o), 0x24, -1); r[3] != 2 || st.Warehouse[5].IsEmpty() {
		t.Fatalf("to warehouse = % X", r)
	}
	b.handleItemMove(o, frame.Build(frame.C3, 0x24, append([]byte{2, 5}, append(make([]byte, 7), 0, BagStart+9)...)...))
	if r := find(b.drain(o), 0x24, -1); r[3] != 0 || o.P.Inventory[BagStart+9].IsEmpty() || o.P.Char.Money != 4200 {
		t.Fatalf("from warehouse = % X money %d", r, o.P.Char.Money)
	}
	// Password: set, lock blocks withdrawal, unlock with the right one.
	pw := func(typ byte, pass uint16) []byte {
		p := make([]byte, 3)
		p[0] = typ
		binary.LittleEndian.PutUint16(p[1:], pass)
		b.handleWarehousePassword(o, frame.Build(frame.C3, 0x83, p...))
		return find(b.drain(o), 0x83, -1)
	}
	if r := pw(1, 1234); r[3] != 1 || st.WarehouseLock != 1 {
		t.Fatalf("set password = % X", r)
	}
	if r := deposit(1, 10); r[3] != 0 {
		t.Fatalf("withdraw while locked = % X", r)
	}
	if r := pw(0, 1); r[3] != 0x0a {
		t.Fatalf("wrong password = % X", r)
	}
	if r := pw(0, 1234); r[3] != 0x0c || st.WarehouseLock != 0 {
		t.Fatalf("unlock = % X", r)
	}
	// Close saves and sends C1 03 82.
	b.closeWarehouse(o)
	if r := find(b.drain(o), 0x82, -1); r == nil || st.WarehouseLoaded {
		t.Fatalf("close = % X", r)
	}
}

func TestTradeFlow(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alice", 0, x, y)
	c := b.player(PlayerStart+1, "Carol", 0, x+1, y)
	a.P.Char.Money, c.P.Char.Money = 5000, 100
	a.P.Inventory[BagStart] = mkItem(0, 0, 0, 20)
	b.handleTradeRequest(a, frame.Build(frame.C3, 0x36, byte(c.Index>>8), byte(c.Index)))
	if r := find(b.drain(c), 0x36, -1); r == nil || string(r[3:8]) != "Alice" {
		t.Fatalf("36 = % X", r)
	}
	b.handleTradeResponse(c, frame.Build(frame.C1, 0x37, 1))
	if r := find(b.drain(a), 0x37, -1); r[3] != 1 || string(r[4:9]) != "Carol" {
		t.Fatalf("37 to Alice = % X", r)
	}
	b.drain(c)
	// Alice puts the Kris into the box and 1000 zen.
	b.handleItemMove(a, frame.Build(frame.C3, 0x24, append([]byte{0, BagStart}, append(make([]byte, 7), 1, 0)...)...))
	if r := find(b.drain(a), 0x24, -1); r[3] != 1 {
		t.Fatalf("to trade box = % X", r)
	}
	if r := find(b.drain(c), 0x39, -1); r == nil || r[3] != 0 || r[4] != 0 {
		t.Fatalf("39 = % X", r)
	}
	money := make([]byte, 5)
	binary.LittleEndian.PutUint32(money[1:], 1000)
	b.handleTradeMoney(a, frame.Build(frame.C1, 0x3A, money...))
	if r := find(b.drain(c), 0x3B, -1); r == nil || binary.LittleEndian.Uint32(r[4:]) != 1000 {
		t.Fatalf("3B = % X", r)
	}
	b.drain(a)
	b.handleTradeOK(a, frame.Build(frame.C3, 0x3C, 1))
	if r := find(b.drain(c), 0x3C, -1); r[3] != 1 {
		t.Fatalf("3C = % X", r)
	}
	b.handleTradeOK(c, frame.Build(frame.C3, 0x3C, 1))
	if r := find(b.drain(a), 0x3D, -1); r == nil || r[3] != 1 {
		t.Fatalf("3D Alice = % X", r)
	}
	if a.P.Char.Money != 4000 || c.P.Char.Money != 1100 || c.P.Inventory[BagStart].Index != 0 ||
		!a.P.Inventory[BagStart].IsEmpty() || items(a).If.Use || items(c).If.Use {
		t.Fatalf("after trade: a %d c %d", a.P.Char.Money, c.P.Char.Money)
	}
	// A cancelled trade restores the box items.
	c.P.Inventory[BagStart+5] = mkItem(14, 0, 0, 1)
	b.handleTradeRequest(c, frame.Build(frame.C3, 0x36, byte(a.Index>>8), byte(a.Index)))
	b.handleTradeResponse(a, frame.Build(frame.C1, 0x37, 1))
	b.handleItemMove(c, frame.Build(frame.C3, 0x24, append([]byte{0, BagStart + 5}, append(make([]byte, 7), 1, 3)...)...))
	if !c.P.Inventory[BagStart+5].IsEmpty() {
		t.Fatal("item not moved to the box")
	}
	b.drain(a)
	b.drain(c)
	b.tradeCancelBoth(a)
	if c.P.Inventory[BagStart+5].IsEmpty() || items(c).If.Use {
		t.Fatal("cancel did not restore the inventory")
	}
	if r := find(b.drain(c), 0x3D, -1); r == nil || r[3] != 0 {
		t.Fatalf("3D cancel = % X", r)
	}
}

func TestTradeRefusesLockedAccount(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	a := b.player(PlayerStart, "Alice", 0, x, y)
	c := b.player(PlayerStart+1, "Carol", 0, x+1, y)
	a.P.Inventory[BagStart] = mkItem(0, 0, 0, 20)
	b.handleTradeRequest(a, frame.Build(frame.C3, 0x36, byte(c.Index>>8), byte(c.Index)))
	b.handleTradeResponse(c, frame.Build(frame.C1, 0x37, 1))
	b.drain(a)
	a.P.Account.Lock = 1234
	b.handleItemMove(a, frame.Build(frame.C3, 0x24, append([]byte{0, BagStart}, append(make([]byte, 7), 1, 0)...)...))
	if r := find(b.drain(a), 0x24, -1); r == nil || r[3] == 1 || a.P.Inventory[BagStart].IsEmpty() {
		t.Fatalf("locked move to trade = % X", r)
	}
}
