package game

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// fixChaosRand makes chaosRand return vals in a loop.
func fixChaosRand(t *testing.T, vals ...int) {
	t.Helper()
	old := chaosRand
	i := 0
	chaosRand = func() int {
		v := vals[i%len(vals)]
		i++
		return v
	}
	t.Cleanup(func() { chaosRand = old })
}

// chaosPlayer is a player with the chaos box open (Chaos Goblin talk).
func chaosPlayer(b *bareWorld) *Object {
	o := b.player(PlayerStart, "Mixer", 0, 130, 130)
	o.Class = 0
	o.P.Char.Class = 0
	o.P.Char.Money = 100000000
	npcTalkHooks[chaosGoblin](b.World, o, fakeNPC(b, 50, chaosGoblin, 0, 131, 130))
	b.drain(o)
	return o
}

func move24(sf, ss, tf, ts byte) []byte {
	p := frame.Build(frame.C3, 0x24, append([]byte{sf, ss}, append(make([]byte, 7), tf, ts)...)...)
	return p
}

func TestChaosPacketLayouts(t *testing.T) {
	if p := packetChaosResult(0, nil); !bytes.Equal(p, []byte{0xC1, 0x0B, 0x86, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("86 fail = % X", p)
	}
	it := item.Item{Index: item.Make(13, 4), Durability: 255}
	p := packetChaosResult(1, &it)
	if len(p) != 11 || p[3] != 1 || p[4] != 4 || p[6] != 255 || p[9]>>4 != 13 {
		t.Fatalf("86 ok = % X", p)
	}
	var box [chaosBoxSlots]item.Item
	for i := range box {
		box[i] = item.Empty()
	}
	box[9] = item.Item{Index: idxChaos, Durability: 1}
	l := packetChaosList(3, box[:])
	if len(l) != 14 || l[0] != 0xC2 || l[1] != 0 || l[2] != 14 || l[3] != 0x31 || l[4] != 3 || l[5] != 1 || l[6] != 9 || l[7] != 15 {
		t.Fatalf("C2 31 03 = % X", l)
	}
	r := packetChaosRate(60, 2000000, 7)
	if len(r) != 16 || r[0] != 0xC1 || r[1] != 0x10 || r[2] != 0x88 || binary.LittleEndian.Uint32(r[4:]) != 60 ||
		binary.LittleEndian.Uint32(r[8:]) != 2000000 || binary.LittleEndian.Uint32(r[12:]) != 7 {
		t.Fatalf("88 = % X", r)
	}
	if p := packetTaxInfo(1, 3); !bytes.Equal(p, []byte{0xC1, 0x06, 0xB2, 0x1A, 1, 3}) {
		t.Fatalf("B2 1A = % X", p)
	}
}

func TestChaosTalkOpensBoxAndTransaction(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Mixer", 0, 130, 130)
	h := npcTalkHooks[chaosGoblin]
	if h == nil || !h(b.World, o, fakeNPC(b, 50, chaosGoblin, 0, 131, 130)) {
		t.Fatal("chaos goblin not handled")
	}
	pk := b.drain(o)
	if w := find(pk, 0x30, 3); w == nil || w[0] != 0xC3 || len(w) != 0x0B {
		t.Fatalf("no C3 0B 30 03: % X", pk)
	}
	if tax := find(pk, 0xB2, 0x1A); tax == nil || tax[4] != 1 || tax[5] != 0 {
		t.Fatalf("tax = % X", tax)
	}
	st := items(o).If
	if !st.Use || st.Type != ifChaosBox || chaos(o).Trans != 1 {
		t.Fatalf("interface %+v trans %d", st, chaos(o).Trans)
	}
	// A second talk while the transaction is open does nothing.
	h(b.World, o, fakeNPC(b, 50, chaosGoblin, 0, 131, 130))
	if len(b.drain(o)) != 0 {
		t.Fatal("talk during an open box answered")
	}
}

func TestChaosBoxMoves(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	o.P.Inventory[BagStart] = item.Item{Index: idxChaos, Durability: 1}
	b.handleItemMove(o, move24(0, BagStart, 3, 5))
	m := find(b.drain(o), 0x24, -1)
	if m == nil || m[3] != 3 || m[4] != 5 || m[5] != 15 {
		t.Fatalf("inventory -> chaos = % X", m)
	}
	c := chaos(o)
	if c.Box[5].Index != idxChaos || !o.P.Inventory[BagStart].IsEmpty() {
		t.Fatal("item not in the box")
	}
	b.handleItemMove(o, move24(3, 5, 3, 6))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 3 || m[4] != 6 || c.Box[6].Index != idxChaos {
		t.Fatalf("chaos -> chaos = % X", m)
	}
	b.handleItemMove(o, move24(3, 6, 0, BagStart+3))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0 || m[4] != BagStart+3 || o.P.Inventory[BagStart+3].Index != idxChaos {
		t.Fatalf("chaos -> inventory = % X", m)
	}
	// Flag 16 (lucky items) refuses everything; a running mix locks moves.
	b.handleItemMove(o, move24(0, BagStart+3, 0x10, 0))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0xFF {
		t.Fatalf("flag 16 = % X", m)
	}
	c.Lock = true
	b.handleItemMove(o, move24(0, BagStart+3, 3, 0))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0xFF {
		t.Fatalf("move during a mix = % X", m)
	}
	c.Lock = false
	// Without the chaos box open the flags fail.
	items(o).If = ifState{}
	b.handleItemMove(o, move24(0, BagStart+3, 3, 0))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0xFF {
		t.Fatalf("move without the box = % X", m)
	}
}

func TestChaosItemMixSuccess(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	c := chaos(o)
	c.Box[0] = item.Item{Index: idxChaos, Durability: 1}
	c.Box[1] = item.Item{Index: item.Make(0, 1), Level: 4, Option: 1, Durability: 20}
	money := o.P.Char.Money
	// rand 0: roll succeeds, the bag picks its first row/section/item and
	// ItemOptionRate row 3 picks level 0 / no skill / no luck / no JoL.
	fixChaosRand(t, 0)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 1})
	pk := b.drain(o)
	m := find(pk, 0x22, 0xFE)
	if m == nil {
		t.Fatal("no money packet")
	}
	value := int64(40000) + b.itemPrice(item.Item{Index: item.Make(0, 1), Level: 4, Option: 1, Durability: 20}).Buy
	rate := min(int(value/20000), 100)
	if c.Rate != rate || c.Money != int64(rate)*10000 || o.P.Char.Money != money-c.Money {
		t.Fatalf("rate %d money %d (value %d), left %d", c.Rate, c.Money, value, o.P.Char.Money)
	}
	if !c.Lock {
		t.Fatal("lock released before the serial arrived")
	}
	b.pump()
	r := find(b.drain(o), 0x86, -1)
	if r == nil || r[3] != 1 || r[4] != 6 || r[9]>>4 != 2 {
		t.Fatalf("86 01 = % X", r)
	}
	if c.Box[0].Index != item.Make(2, 6) || c.Box[0].Serial == 0 || !c.Box[1].IsEmpty() || c.Lock {
		t.Fatalf("box = %+v lock %v", c.Box[0], c.Lock)
	}
}

func TestChaosItemMixFailureAndErrors(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	c := chaos(o)
	// No chaos jewel: 7.
	c.Box[1] = item.Item{Index: item.Make(0, 1), Level: 4, Option: 1, Durability: 20}
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 1})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixWrongItems || c.Lock {
		t.Fatalf("no chaos = % X", r)
	}
	c.Box[0] = item.Item{Index: idxChaos, Durability: 1}
	// Charms of Luck worth more than 10: F0.
	c.Box[2] = item.Item{Index: idxCharmLuck, Durability: 11}
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 1})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixCharms {
		t.Fatalf("charms = % X", r)
	}
	c.Box[2] = item.Empty()
	// Not enough zen: 2.
	o.P.Char.Money = 10
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 1})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixNoMoney || o.P.Char.Money != 10 {
		t.Fatalf("no zen = % X", r)
	}
	// Failure: the box is cleared, the empty list and 86 00 are sent.
	o.P.Char.Money = 100000000
	fixChaosRand(t, 99)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 1})
	pk := b.drain(o)
	if l := find(pk, 0x31, -1); l == nil || l[4] != 3 || l[5] != 0 {
		t.Fatalf("list after failure = % X", l)
	}
	if r := find(pk, 0x86, -1); r == nil || r[3] != mixFail || r[4] != 0xFF {
		t.Fatalf("86 00 = % X", r)
	}
	for _, it := range c.Box {
		if !it.IsEmpty() {
			t.Fatal("box not cleared")
		}
	}
}

func plusBox(o *Object, level int, luck bool) {
	c := chaos(o)
	for i := range c.Box {
		c.Box[i] = item.Empty()
	}
	c.Box[0] = item.Item{Index: idxChaos, Durability: 1}
	c.Box[1] = item.Item{Index: idxBless, Durability: 1}
	c.Box[2] = item.Item{Index: idxSoul, Durability: 1}
	c.Box[3] = item.Item{Index: item.Make(0, 1), Level: level, Luck: luck, Durability: 10}
}

func TestPlusItemMix(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	c := chaos(o)
	plusBox(o, 9, false)
	fixChaosRand(t, 59) // PlusCommonItemLevelMixRate1_AL0 = 60
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 3})
	pk := b.drain(o)
	r := find(pk, 0x86, -1)
	if r == nil || r[3] != 1 || c.Rate != 60 || c.Money != 2000000 {
		t.Fatalf("+10 = % X rate %d money %d", r, c.Rate, c.Money)
	}
	up := c.Box[0]
	oldMax := float32(b.itemMaxDurability(item.Item{Index: item.Make(0, 1), Level: 9}))
	newMax := float32(b.itemMaxDurability(item.Item{Index: item.Make(0, 1), Level: 10}))
	if up.Level != 10 || up.Durability != 10*newMax/oldMax || !c.Box[3].IsEmpty() || c.Lock {
		t.Fatalf("upgraded = %+v (max %v -> %v)", up, oldMax, newMax)
	}
	// Luck adds AddLuckSuccessRate2 (20); rand 80 fails with FailMixMode 0.
	plusBox(o, 9, true)
	fixChaosRand(t, 80)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 3})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != 0 || c.Rate != 80 {
		t.Fatalf("luck failure = % X rate %d", r, c.Rate)
	}
	for _, it := range c.Box {
		if !it.IsEmpty() {
			t.Fatal("FailMixMode 0 must clear everything")
		}
	}
	// Account level 1 uses FailMixMode 2: the item survives untouched.
	o.P.Account.Level = 1
	plusBox(o, 9, false)
	fixChaosRand(t, 99)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 3})
	if c.Box[3].Level != 9 || !c.Box[0].IsEmpty() || !c.Box[1].IsEmpty() {
		t.Fatalf("FailMixMode 2 box = %+v", c.Box[:4])
	}
	// +14 reads a rate offset no key loads: rate 0.
	o.P.Account.Level = 0
	plusBox(o, 13, false)
	c.Box[4] = item.Item{Index: idxBless, Durability: 1}
	c.Box[5] = item.Item{Index: idxSoul, Durability: 1}
	c.Box[6] = item.Item{Index: idxBless, Durability: 1}
	c.Box[7] = item.Item{Index: idxSoul, Durability: 1}
	c.Box[8] = item.Item{Index: idxBless, Durability: 1}
	c.Box[9] = item.Item{Index: idxSoul, Durability: 1}
	c.Box[10] = item.Item{Index: idxBless, Durability: 1}
	c.Box[11] = item.Item{Index: idxSoul, Durability: 1}
	fixChaosRand(t, 0)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0x31})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != 0 || c.Rate != 0 || c.Money != 10000000 {
		t.Fatalf("+14 = % X rate %d money %d", r, c.Rate, c.Money)
	}
}

func TestPairedTicketMixes(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	c := chaos(o)
	set := func(a, bb, la, lb int) {
		for i := range c.Box {
			c.Box[i] = item.Empty()
		}
		c.Box[0] = item.Item{Index: idxChaos, Durability: 1}
		c.Box[1] = item.Item{Index: a, Level: la, Durability: 1}
		c.Box[2] = item.Item{Index: bb, Level: lb, Durability: 1}
	}
	set(idxDSEye, idxDSKey, 2, 3)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 2})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixWrongItems {
		t.Fatalf("DS mismatched levels = % X", r)
	}
	set(idxArchangel, idxBloodBone, 9, 9)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 8})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixWrongLevel {
		t.Fatalf("BC level 9 = % X", r)
	}
	// Devil Square 2: rate 80, 200,000 zen, Devil's Invitation +2 dur 1.
	set(idxDSEye, idxDSKey, 2, 2)
	fixChaosRand(t, 79)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 2})
	b.pump()
	r := find(b.drain(o), 0x86, -1)
	if r == nil || r[3] != 1 || c.Rate != 80 || c.Money != 200000 || c.Box[0].Index != idxDSInvite ||
		c.Box[0].Level != 2 || c.Box[0].Durability != 1 {
		t.Fatalf("DS2 = % X box %+v", r, c.Box[0])
	}
	// Blood Castle: rand%100 == rate still succeeds (fails only when rate < r).
	set(idxArchangel, idxBloodBone, 3, 3)
	fixChaosRand(t, 80)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 8})
	b.pump()
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != 1 || c.Money != 150000 || c.Box[0].Index != idxCloak {
		t.Fatalf("BC3 = % X", r)
	}
}

func TestPetAndAlwaysSuccessMixes(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	c := chaos(o)
	fill := func(spirit, bless, soul int) {
		for i := range c.Box {
			c.Box[i] = item.Empty()
		}
		n := 0
		add := func(idx, lvl int) { c.Box[n] = item.Item{Index: idx, Level: lvl, Durability: 1}; n++ }
		add(idxChaos, 0)
		add(idxSpirit, spirit)
		add(idxCreation, 0)
		for i := 0; i < bless; i++ {
			add(idxBless, 0)
		}
		for i := 0; i < soul; i++ {
			add(idxSoul, 0)
		}
	}
	// Dark Raven: spirit +1, 2 bless, 2 soul, 1,000,000 zen.
	fill(1, 2, 2)
	fixChaosRand(t, 0)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0xE})
	b.pump()
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != 1 || c.Money != 1000000 || c.Box[0].Index != idxDarkRaven ||
		c.Box[0].PetLevel != 1 {
		t.Fatalf("raven mix = % X box %+v", r, c.Box[0])
	}
	// Dark Horse with the raven counts: 7.
	fill(0, 2, 2)
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0xD})
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != mixWrongItems {
		t.Fatalf("horse with 2 jewels = % X", r)
	}
	// Siege potion: 3 bless -> 30 potions, 100,000 zen, no rate.
	for i := range c.Box {
		c.Box[i] = item.Empty()
	}
	for i := 0; i < 3; i++ {
		c.Box[i] = item.Item{Index: idxBless, Durability: 1}
	}
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0xF})
	b.pump()
	if r := find(b.drain(o), 0x86, -1); r == nil || r[3] != 1 || c.Money != 100000 || c.Box[0].Index != idxSiegePot ||
		c.Box[0].Durability != 30 || c.Box[0].Level != 0 {
		t.Fatalf("siege potion = % X box %+v", r, c.Box[0])
	}
	// Senior mix: no castle owner -> 0x2E7 notice and 86 00.
	b.handleChaosMix(o, []byte{0xC1, 4, 0x86, 0x12})
	pk := b.drain(o)
	if r := find(pk, 0x86, -1); r == nil || r[3] != 0 || find(pk, 0x0D, -1) == nil {
		t.Fatalf("senior = % X", pk)
	}
}

func TestChaosRatePreview(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	plusBox(o, 9, true)
	b.handleChaosRate(o, []byte{0xC1, 8, 0x88, 0, 3, 0, 0, 0})
	r := find(b.drain(o), 0x88, -1)
	if r == nil || binary.LittleEndian.Uint32(r[4:]) != 80 || binary.LittleEndian.Uint32(r[8:]) != 2000000 ||
		binary.LittleEndian.Uint32(r[12:]) != 0 {
		t.Fatalf("88 +10 = % X", r)
	}
	// +11 needs two bless and two soul: no answer.
	b.handleChaosRate(o, []byte{0xC1, 8, 0x88, 0, 4, 0, 0, 0})
	if r := find(b.drain(o), 0x88, -1); r != nil {
		t.Fatalf("88 +11 with one jewel each = % X", r)
	}
	// Wrong recipe: no answer; unknown type: zeros.
	b.handleChaosRate(o, []byte{0xC1, 8, 0x88, 0, 5, 0, 0, 0})
	if r := find(b.drain(o), 0x88, -1); r != nil {
		t.Fatalf("88 dinorant without items = % X", r)
	}
	b.handleChaosRate(o, []byte{0xC1, 8, 0x88, 0, 0x31, 0, 0, 0})
	if r := find(b.drain(o), 0x88, -1); r == nil || binary.LittleEndian.Uint32(r[4:]) != 0 {
		t.Fatalf("88 0x31 = % X", r)
	}
}

func TestChaosCloseAndLogout(t *testing.T) {
	b := newBareWorld(t)
	o := chaosPlayer(b)
	o.P.Inventory[BagStart] = item.Item{Index: idxChaos, Durability: 1}
	b.World.chaosBegin(o) // already open: no new backup
	chaos(o).backup[BagStart] = o.P.Inventory[BagStart]
	b.handleItemMove(o, move24(0, BagStart, 3, 0))
	b.drain(o)
	// Logout without a mix: the inventory from before the box comes back.
	b.World.chaosLogout(o)
	if o.P.Inventory[BagStart].Index != idxChaos || chaos(o).Trans != 3 {
		t.Fatalf("rollback: %+v trans %d", o.P.Inventory[BagStart], chaos(o).Trans)
	}
	// 87 clears and commits.
	items(o).If = ifState{}
	npcTalkHooks[chaosGoblin](b.World, o, fakeNPC(b, 50, chaosGoblin, 0, 131, 130))
	b.handleItemMove(o, move24(0, BagStart, 3, 0))
	b.drain(o)
	b.handleChaosClose(o)
	pk := b.drain(o)
	if p := find(pk, 0x87, -1); p == nil || len(p) != 3 || chaos(o).Trans != 2 || items(o).If.Use {
		t.Fatalf("87 = % X trans %d", p, chaos(o).Trans)
	}
}

func TestChaosRollBag(t *testing.T) {
	d := &chaosData{rates: &gamedata.ItemOptionRate{}}
	for i := range d.rates.Sections {
		d.rates.Sections[i] = map[int][]int{}
	}
	d.rates.Sections[0][3] = []int{0, 0, 0, 0, 0, 0, 0, 1}
	d.rates.Sections[1][1] = []int{0, 1}
	bag := &gamedata.ItemBag{
		Bags: []gamedata.BagRow{{Index: 0, DropRate: 10000, Drops: []gamedata.BagDrop{
			{Section: 5, SectionRate: 1, Class: [5]int{1, 2, 1, 1, 1}},
			{Section: 6, SectionRate: 1, Class: [5]int{1, 1, 1, 1, 1}},
		}}},
		Sections: map[int][]gamedata.BagItem{
			5: {{Index: 0x10, Level: 2, Option: [6]int{3, 1, -1, -1, -1, -1}}},
			6: {{Index: 0x20, Level: 1, Grade: 4, Option: [6]int{-1, -1, -1, -1, -1, -1}}},
		},
	}
	// A DK without change up only qualifies for section 6.
	m, ok := chaosRollBag(d, bag, 1, 0, func() int { return 0 })
	if !ok || m.Index != 0x20 || m.Level != 1 || m.Excellent != 4 {
		t.Fatalf("DK roll = %+v %v", m, ok)
	}
	// A DW gets section 5: level from row 3 (7), skill from row 1.
	m, ok = chaosRollBag(d, bag, 0, 0, func() int { return 0 })
	if !ok || m.Index != 0x10 || m.Level != 7 || !m.Skill {
		t.Fatalf("DW roll = %+v %v", m, ok)
	}
	// No bag row passes its DropRate: no item.
	bag.Bags[0].DropRate = 0
	if _, ok := chaosRollBag(d, bag, 0, 0, func() int { return 0 }); ok {
		t.Fatal("rolled with DropRate 0")
	}
}

func TestChaosMakeSet(t *testing.T) {
	sets := &gamedata.SetItemTypes{ByIndex: map[int]gamedata.SetItemType{
		1: {Index: 1, Option: [2]int{2, 0}},
		2: {Index: 2, Option: [2]int{0, 3}},
	}}
	zero := func() int { return 0 }
	cases := []struct {
		idx, v int
		want   byte
	}{{1, 1, 5}, {1, 2, 9}, {1, 3, 5}, {1, 4, 9}, {1, 5, 0}, {2, 1, 10}, {2, 2, 6}, {2, 5, 6}, {2, 6, 10}, {3, 1, 0}, {1, 0, 0}}
	for _, c := range cases {
		if got := chaosMakeSet(sets, c.idx, c.v, zero); got != c.want {
			t.Errorf("set(%d,%d) = %d, want %d", c.idx, c.v, got, c.want)
		}
	}
}
