package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/mapitem"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func newItem(index, level int) item.Item {
	it := item.Empty()
	it.Index, it.Level, it.Durability = index, level, 1
	return it
}

func give(t *testing.T, b *bareWorld, o *Object, index, level, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if b.insertInventory(o, newItem(index, level)) < 0 {
			t.Fatal("inventory full")
		}
	}
}

// --- CustomStore -----------------------------------------------------------------------

func TestCustomStoreOpenAndJewelPurchase(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.safeCell(0)
	seller := b.player(PlayerStart, "Seller", 0, x, y)
	buyer := b.player(PlayerStart+1, "Buyer", 0, x, y)
	b.see(seller, buyer)
	// Not open yet: 0x21C.
	b.runCommand(seller, "/store bless")
	hasText(t, b.drain(seller), b.messageFor(seller, 0x21C))
	sword := newItem(0, 5)
	sword.PShopPrice = 35
	seller.P.Inventory[ShopStart] = sword
	pshop(seller).Open = true
	b.runCommand(seller, "/store bless")
	s := custom(seller).Store
	if !s.Active || s.Type != 0 {
		t.Fatalf("store %+v", s)
	}
	title := b.messageFor(seller, 0x21F)
	p := find(b.drain(buyer), 0x3F, 0x10)
	if p == nil || !bytes.HasPrefix(p[6:], []byte(title)) || p[4] != byte(seller.Index>>8) || p[5] != byte(seller.Index) {
		t.Fatalf("3F 10 % X", p)
	}
	// Viewing shows the jewel notice.
	view := append([]byte{0xC1, 16, 0x3F, 0x05, byte(seller.Index >> 8), byte(seller.Index)}, []byte("Seller\x00\x00\x00\x00")...)
	b.pshopView(buyer, view)
	hasText(t, b.drain(buyer), b.messageFor(buyer, 0x229))

	// 35 Bless: a 30-pack (12,30 level 2) and 5 singles out of 7.
	give(t, b, buyer, 12*512+30, 2, 1)
	give(t, b, buyer, 14*512+13, 0, 7)
	buy := append([]byte{0xC1, 17, 0x3F, 0x06, byte(seller.Index >> 8), byte(seller.Index)}, []byte("Seller\x00\x00\x00\x00")...)
	buy = append(buy, byte(ShopStart))
	zen := buyer.P.Char.Money
	b.pshopBuy(buyer, buy)
	r := find(b.drain(buyer), 0x3F, 0x06)
	if r == nil || r[4] != 1 {
		t.Fatalf("buy result % X", r)
	}
	if buyer.P.Char.Money != zen {
		t.Fatal("zen was charged")
	}
	if n := b.countItems(buyer, 14*512+13, 0); n != 2 {
		t.Fatalf("bless left %d", n)
	}
	if n := b.countItems(buyer, 12*512+30, 2); n != 0 {
		t.Fatalf("30-pack left %d", n)
	}
	if b.countItems(buyer, 0, 5) != 1 {
		t.Fatal("item not received")
	}
	b.runAsync()
	// The seller gets 35 = one 30-pack + 5 singles, and the shop closes.
	if b.countItems(seller, 12*512+30, 2) != 1 || b.countItems(seller, 14*512+13, 0) != 5 {
		t.Fatalf("seller got %d packs %d singles", b.countItems(seller, 12*512+30, 2), b.countItems(seller, 14*512+13, 0))
	}
	if pshopOpen(seller) || custom(seller).Store.Active {
		t.Fatal("shop still open")
	}
}

func TestCustomStoreRefusesShortPayment(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.safeCell(0)
	seller := b.player(PlayerStart, "Sel", 0, x, y)
	buyer := b.player(PlayerStart+1, "Buy", 0, x, y)
	it := newItem(0, 1)
	it.PShopPrice = 15
	seller.P.Inventory[ShopStart] = it
	pshop(seller).Open = true
	b.customStoreOpen(seller, 1) // soul
	// 20 Soul in one 20-pack cannot pay exactly 15.
	give(t, b, buyer, 12*512+31, 1, 1)
	buy := append([]byte{0xC1, 17, 0x3F, 0x06, byte(seller.Index >> 8), byte(seller.Index)}, []byte("Sel\x00\x00\x00\x00\x00\x00\x00")...)
	buy = append(buy, byte(ShopStart))
	b.pshopBuy(buyer, buy)
	if r := find(b.drain(buyer), 0x3F, 0x06); r == nil || r[4] != 7 {
		t.Fatalf("result % X", r)
	}
	if b.countItems(buyer, 12*512+31, 1) != 1 || !pshopOpen(seller) {
		t.Fatal("state changed")
	}
}

func TestJewelReceiptAndPayment(t *testing.T) {
	if r := jewelReceipt(87); r != (jewelPlan{singles: 7, p10: 0, p20: 1, p30: 2}) || r.cells() != 10 {
		t.Fatalf("receipt %+v", r)
	}
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Pay", 0, 130, 130)
	give(t, b, o, 14*512+16, 0, 3)  // life singles
	give(t, b, o, 12*512+136, 0, 2) // life 10-packs
	total, plan := b.jewelPayment(o, 2, 23)
	if total != 23 || plan != (jewelPlan{singles: 3, p10: 2}) {
		t.Fatalf("%d %+v", total, plan)
	}
	if _, plan = b.jewelPayment(o, 2, 25); !plan.zero() {
		t.Fatalf("over %+v", plan)
	}
}

// --- /gift -----------------------------------------------------------------------------

func TestCommandGift(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := b.player(PlayerStart, "Gifty", 0, x, y)
	b.cw().gifts = []gamedata.Gift{
		{Index: 1, Class: 1, Switch: [4]int{1, 1, 1, 1}, MinLevel: -1, MaxLevel: -1, MinReset: -1, MaxReset: -1, Code: "dk"},
		{Index: 2, Class: -1, Switch: [4]int{1, 1, 1, 1}, MinLevel: 500, MaxLevel: -1, MinReset: -1, MaxReset: -1, Code: "high"},
		{Index: 3, Class: -1, Switch: [4]int{1, 1, 1, 1}, MinLevel: -1, MaxLevel: -1, MinReset: -1, MaxReset: -1, MaxCount: 1,
			LevelUpPoint: 7, Money: 100, Code: "beginner", Items: []gamedata.GiftItem{{Index: 14*512 + 13}}},
	}
	// Switch off: nothing.
	b.runCommand(o, "/gift beginner")
	if len(b.drain(o)) != 0 {
		t.Fatal("answered with CustomGiftSwitch 0")
	}
	withConfig(t, b, "Custom", map[string]string{"CustomGiftSwitch": "1"})
	b.runCommand(o, "/gift dk") // class mismatch: not found
	hasText(t, b.drain(o), b.messageFor(o, 0x25D))
	b.runCommand(o, "/gift high")
	hasText(t, b.drain(o), b.messageFor(o, 0x25E))
	pts, zen := o.P.Char.LevelUpPoints, o.P.Char.Money
	b.runCommand(o, "/gift beginner")
	b.runAsync()
	pk := b.drain(o)
	hasText(t, pk, b.messageFor(o, 0x264))
	if find(pk, 0xF3, 0x40) == nil || find(pk, 0xF3, 0xE0) == nil {
		t.Fatal("no firework or F3 E0")
	}
	if o.P.Char.LevelUpPoints != pts+7 || o.P.Char.Money != zen+100 {
		t.Fatalf("points %d zen %d", o.P.Char.LevelUpPoints, o.P.Char.Money)
	}
	n := 0
	for _, m := range b.mapItemsAt(0) {
		if m.Item.Index == 14*512+13 && m.Owner == o.Index {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d gift items on the ground", n)
	}
	// MaxCount 1: the DataServer refuses the second time.
	b.runCommand(o, "/gift beginner")
	b.runAsync()
	hasText(t, b.drain(o), b.messageFor(o, 0x263))
}

// --- marriage --------------------------------------------------------------------------

func marriagePair(t *testing.T, b *bareWorld) (*Object, *Object) {
	t.Helper()
	withConfig(t, b, "Custom", map[string]string{"CustomMarriageSwitch": "1"})
	a := b.player(PlayerStart, "Romeo", 2, 12, 25)
	c := b.player(PlayerStart+1, "Juliet", 2, 13, 26)
	a.Class, c.Class = 1, 2
	b.see(a, c)
	give(t, b, a, 12*512+15, 0, 2)
	give(t, b, c, 12*512+15, 0, 2)
	return a, c
}

func TestMarriageFlow(t *testing.T) {
	b := newBareWorld(t)
	a, c := marriagePair(t, b)
	// (13,42) of the original file is not in Item.txt and is never created.
	b.cw().marriage.Rewards = []gamedata.GiftItem{{Index: 13*512 + 8}}
	b.runCommand(a, "/marry Nobody")
	hasText(t, b.drain(a), b.messageFor(a, 0x171))
	b.runCommand(a, "/marry Romeo")
	hasText(t, b.drain(a), b.messageFor(a, 0x28A))
	b.runCommand(a, "/marry Juliet")
	hasText(t, b.drain(a), b.messageFor(a, 0x291))
	hasText(t, b.drain(c), b.messageFor(c, 0x292, "Romeo"))
	if r := custom(c).Marry; !r.Pending || r.From != a.Index {
		t.Fatalf("request %+v", r)
	}
	b.runCommand(c, "/accept")
	b.runAsync()
	if !a.P.Char.Married || a.P.Char.MarryCouple != "Juliet" || !c.P.Char.Married || c.P.Char.MarryCouple != "Romeo" {
		t.Fatal("not married")
	}
	// The marry item count is the divorce section's (1).
	if b.countItems(a, 12*512+15, 0) != 1 || b.countItems(c, 12*512+15, 0) != 1 {
		t.Fatal("items not taken")
	}
	if b.countItems(a, 13*512+8, 0) != 1 || b.countItems(c, 13*512+8, 0) != 1 {
		t.Fatal("no reward")
	}
	pc := b.drain(c)
	hasText(t, pc, b.messageFor(c, 0x295, "Romeo"))
	n := 0
	for _, p := range pc {
		if find([][]byte{p}, 0xF3, 0x40) != nil {
			n++
		}
	}
	if n != 10 {
		t.Fatalf("%d fireworks", n)
	}
	// Already married.
	b.runCommand(a, "/marry Juliet")
	hasText(t, b.drain(a), b.messageFor(a, 0x28C, "Juliet"))
	// Divorce with the partner online.
	b.runCommand(a, "/divorce")
	if a.P.Char.Married || c.P.Char.Married || a.P.Char.MarryCouple != "" {
		t.Fatal("not divorced")
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x299))
	if b.countItems(a, 12*512+15, 0) != 0 {
		t.Fatal("divorce item not taken")
	}
}

func TestMarriageRules(t *testing.T) {
	b := newBareWorld(t)
	a, c := marriagePair(t, b)
	c.Class = 1 // same gender
	b.runCommand(a, "/marry Juliet")
	hasText(t, b.drain(a), b.messageFor(a, 0x28E))
	c.Class = 2
	c.X = 40 // outside the altar
	b.runCommand(a, "/marry Juliet")
	hasText(t, b.drain(a), b.messageFor(a, 0x28F))
	c.X = 13
	b.deleteItems(c, 12*512+15, 0, 2)
	b.runCommand(a, "/marry Juliet")
	hasText(t, b.drain(a), b.messageFor(a, 0x290, 1, b.itemName(12*512+15, 0)))
	give(t, b, c, 12*512+15, 0, 1)
	b.runCommand(a, "/marry Juliet")
	b.advance(61 * time.Second)
	b.runCommand(c, "/accept")
	hasText(t, b.drain(c), b.messageFor(c, 0x293))
	if c.P.Char.Married || custom(c).Marry.Pending {
		t.Fatal("accepted after 60 seconds")
	}
}

func TestDivorceOfflinePartnerAndTrack(t *testing.T) {
	b := newBareWorld(t)
	withConfig(t, b, "Custom", map[string]string{"CustomMarriageSwitch": "1", "CustomMarriageTracePK": "1"})
	ctx := context.Background()
	acc, _ := b.store.CreateAccount(ctx, "accOther", "plain:x", "")
	b.store.CreateCharacter(ctx, acc.ID, "Other", 0)
	ch, _ := b.store.LoadCharacter(ctx, acc.ID, "Other")
	ch.Married, ch.MarryCouple = true, "Solo"
	b.store.SaveCharacter(ctx, ch)
	x, y := b.openRow(0, 2)
	o := b.player(PlayerStart, "Solo", 0, x, y)
	o.P.Char.Married, o.P.Char.MarryCouple = true, "Other"
	b.runCommand(o, "/trackmarry")
	hasText(t, b.drain(o), b.messageFor(o, 0x29A))
	give(t, b, o, 12*512+15, 0, 1)
	b.runCommand(o, "/divorce")
	b.runAsync()
	if ch, _ = b.store.LoadCharacter(ctx, acc.ID, "Other"); ch.Married {
		t.Fatal("offline partner still married")
	}
	// /trackmarry warps to the partner; a PK 5 cannot with TracePK.
	p := b.player(PlayerStart+1, "Mate", 0, x+2, y)
	o.P.Char.Married, o.P.Char.MarryCouple = true, "Mate"
	o.PKLevel = 5
	b.runCommand(o, "/trackmarry")
	hasText(t, b.drain(o), b.messageFor(o, 0x29C))
	o.PKLevel = 3
	p.Map = 11
	b.runCommand(o, "/trackmarry")
	hasText(t, b.drain(o), b.messageFor(o, 0x29B))
	p.Map = 0
	b.runCommand(o, "/trackmarry")
	if o.X != p.X || o.Y != p.Y {
		t.Fatalf("at %d,%d want %d,%d", o.X, o.Y, p.X, p.Y)
	}
}

// --- /buyvip ---------------------------------------------------------------------------

func TestCommandBuyVip(t *testing.T) {
	b := newBareWorld(t)
	o := offlinePlayer(t, b, PlayerStart, "Vippy")
	b.cw().vip = map[string]gamedata.VipPlan{
		"month": {AccountLevel: 0, NewAccountLevel: 2, Days: 31, Money: 500, Code: "month"},
		"gold":  {AccountLevel: 2, NewAccountLevel: 3, Days: 7, Code: "gold"},
	}
	b.runCommand(o, "/buyvip nope")
	hasText(t, b.drain(o), b.messageFor(o, 0x2A2))
	b.runCommand(o, "/buyvip gold")
	hasText(t, b.drain(o), b.messageFor(o, 0x2A3))
	o.P.Char.Money = 100
	b.runCommand(o, "/buyvip month")
	hasText(t, b.drain(o), b.messageFor(o, 0x2A4, 500))
	o.P.Char.Money = 1000
	b.runCommand(o, "/buyvip month")
	hasText(t, b.drain(o), b.messageFor(o, 0x2A8))
	b.runAsync()
	if o.P.Char.Money != 500 || o.P.Account.Level != 2 {
		t.Fatalf("zen %d level %d", o.P.Char.Money, o.P.Account.Level)
	}
	// The auth service uses the real clock.
	if d := time.Until(o.P.Account.LevelExpiresAt); d < 30*24*time.Hour || d > 32*24*time.Hour {
		t.Fatalf("expires in %v", d)
	}
}

// --- /pack4155, /unpack ----------------------------------------------------------------

func TestJewelPackUnpack(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Packer", 0, 130, 130)
	b.runCommand(o, "/pack4155")
	hasText(t, b.drain(o), b.messageFor(o, 0x184))
	b.runCommand(o, "/pack4155 bless 15")
	hasText(t, b.drain(o), b.messageFor(o, 0x185))
	b.runCommand(o, "/pack4155 bless 10")
	hasText(t, b.drain(o), b.messageFor(o, 0x186))
	give(t, b, o, 14*512+13, 0, 12)
	b.runCommand(o, "/pack4155 bless 20")
	hasText(t, b.drain(o), b.messageFor(o, 0x187))
	b.runCommand(o, "/pack4155 bless 10")
	b.runAsync()
	if b.countItems(o, 14*512+13, 0) != 2 || b.countItems(o, 12*512+30, 0) != 1 {
		t.Fatalf("after pack: %d bless %d packs", b.countItems(o, 14*512+13, 0), b.countItems(o, 12*512+30, 0))
	}
	b.runCommand(o, "/unpack bless 20")
	hasText(t, b.drain(o), b.messageFor(o, 0x186))
	b.runCommand(o, "/unpack bless 10")
	b.runAsync()
	if b.countItems(o, 14*512+13, 0) != 12 || b.countItems(o, 12*512+30, 0) != 0 {
		t.Fatal("after unpack")
	}
	// An unknown word does nothing.
	b.drain(o)
	b.runCommand(o, "/pack4155 xyz 10")
	if len(b.drain(o)) != 0 {
		t.Fatal("unknown type answered")
	}
}

// --- CustomPick ------------------------------------------------------------------------

func TestCustomPickCommandsAndLoot(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := b.player(PlayerStart, "Picker", 0, x, y)
	b.runCommand(o, "/pick")
	hasText(t, b.drain(o), b.messageFor(o, 0x1C3))
	b.runCommand(o, "/pickset ab")
	hasText(t, b.drain(o), b.messageFor(o, 0x1CB))
	b.runCommand(o, "/pickset Bless")
	hasText(t, b.drain(o), b.messageFor(o, 0x1C7, 1, "Bless"))
	if pickOf(o).Texts[0] != "bless" {
		t.Fatalf("filter %q", pickOf(o).Texts[0])
	}
	b.runCommand(o, "/pick 0 0 0 0")
	hasText(t, b.drain(o), b.messageFor(o, 0x1C4, 1, 0, 0, 0, 0))
	if !pickOf(o).On {
		t.Fatal("not on")
	}
	// A jewel of bless next to the player is taken by name; a blocked
	// Jewel of Creation (CustomPick.txt) stays.
	n := b.addMapItem(0, x+1, y, newItem(14*512+13, 0), -1)
	m := b.addMapItem(0, x+1, y, newItem(14*512+22, 0), -1)
	b.pickTick()
	if !b.mapItems(0).Items[n].Valid() {
		t.Fatal("picked before one second")
	}
	b.advance(2 * time.Second)
	b.pickTick()
	b.runAsync() // the item serial
	if b.mapItems(0).Items[n].State != mapitem.StateRemoved || b.countItems(o, 14*512+13, 0) != 1 {
		t.Fatal("not picked")
	}
	if b.mapItems(0).Items[m].State == mapitem.StateRemoved {
		t.Fatal("blocked item picked")
	}
	b.runCommand(o, "/pick")
	hasText(t, b.drain(o), b.messageFor(o, 0x1C5))
	b.runCommand(o, "/pickclear")
	if pickOf(o).Count != 0 || pickOf(o).Texts[0] != "" {
		t.Fatal("not cleared")
	}
}

func TestCustomPickPersistence(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Saver", 0, 130, 130)
	cs := b.customStore()
	var d store.PickData
	copy(d.List[store.PickTextSize:], "excellent")
	d.Jewel = 1
	cs.SavePick(context.Background(), "Saver", d)
	b.loadPick(o)
	b.runAsync()
	pk := pickOf(o)
	if !pk.Loaded || pk.Count != 1 || pk.Texts[1] != "excellent" || pk.Jewel != 1 {
		t.Fatalf("%+v", *pk)
	}
	pk.Texts[0], pk.Exc = "wings", 1
	b.saveCharacter(o, nil)
	b.runAsync()
	got, _ := cs.LoadPick(context.Background(), "Saver")
	if got.Exc != 1 || string(bytes.TrimRight(got.List[:store.PickTextSize], "\x00")) != "wings" {
		t.Fatalf("saved %+v", got)
	}
}

// --- CustomAttack ----------------------------------------------------------------------

func TestAttackHardCodedAndPackets(t *testing.T) {
	if p := packetAttackStatus(true); !bytes.Equal(p, []byte{0xC1, 8, 0xF3, 0xEB, 1, 0, 0, 0}) {
		t.Fatalf("F3 EB % X", p)
	}
	if p := packetAttackHelperOpen(); !bytes.Equal(p, []byte{0xC1, 4, 0xF3, 0xF6}) {
		t.Fatalf("F3 F6 % X", p)
	}
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Atk", 0, 130, 130)
	b.runCommand(o, "/attack")
	if find(b.drain(o), 0xF3, 0xF6) == nil {
		t.Fatal("no F3 F6")
	}
	b.runCommand(o, "/attack off")
	if len(b.drain(o)) != 0 {
		t.Fatal("off while not active answered")
	}
}

func TestAttackManualHitsNearestMonster(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 6)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1) // Poison
	o.Map = 0
	m := b.monsterNear(0, o, 2)
	b.see(o, m)
	b.handleAttackManual(o, []byte{0xC1, 8, 0xF3, 0x0B, 1, 0, 0, 0})
	pk := b.drain(o)
	if p := find(pk, 0xF3, 0xEB); p == nil || p[4] != 1 {
		t.Fatal("no status")
	}
	a := attackOf(o)
	if !a.Manual || !a.Active || a.Skill != 1 {
		t.Fatalf("%+v", *a)
	}
	b.attackTick()
	if a.Target != m.Index || !a.NextCast.After(b.clock) {
		t.Fatalf("target %d next %v", a.Target, a.NextCast)
	}
	// F3 0B again stops it (message 0x23C).
	b.handleAttackManual(o, []byte{0xC1, 8, 0xF3, 0x0B, 1, 0, 0, 0})
	if a.Active || a.Manual {
		t.Fatal("still active")
	}
	if !bytes.Contains(bytes.Join(b.drain(o), nil), []byte(b.messageFor(o, 0x23C))) {
		t.Fatal("no 0x23C")
	}
}

func TestAttackManualRefusals(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.safeCell(0)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1)
	b.handleAttackManual(o, []byte{0xC1, 8, 0xF3, 0x0B, 1, 0, 0, 0})
	hasText(t, b.drain(o), "Use /attack fora da cidade, perto dos monstros.")
	o.X, o.Y = b.openRow(0, 1)
	b.handleAttackManual(o, []byte{0xC1, 8, 0xF3, 0x0B, 9, 0, 0, 0})
	hasText(t, b.drain(o), "A magia selecionada nao esta na lista do personagem. Selecione uma habilidade disponivel.")
}

func TestOffAttackAndExperienceRate(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1)
	withConfig(t, b, "Custom", map[string]string{"CustomAttackExperienceRate_AL0": "150", "CustomAttackOfflineExperienceRate_AL0": "50"})
	if b.customAttackExpRate(o) != 100 {
		t.Fatal("rate without the attack")
	}
	b.runCommand(o, "/offattack")
	hasText(t, b.drain(o), b.messageFor(o, 0x241))
	a := attackOf(o)
	a.Skill = 1
	b.attackBegin(o)
	if b.customAttackExpRate(o) != 150 {
		t.Fatal("online rate")
	}
	b.runCommand(o, "/offattack")
	if !a.Offline || !detached(o) || !isOffline(o) {
		t.Fatal("not offline")
	}
	if b.customAttackExpRate(o) != 50 {
		t.Fatal("offline rate")
	}
	// Without skill or buffs the offline object is removed.
	a.Skill, a.Buff = 0, 0
	b.attackTick()
	if o.State != StateEmpty {
		t.Fatal("not removed")
	}
}

func TestAttackHelperPacketRunsCommandChecks(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1)
	b.handleAttackHelper(o, []byte{0xC1, 6, 0xF3, 0x0A, 1, 0})
	a := attackOf(o)
	if !a.Active || a.Skill != 1 || a.Manual {
		t.Fatalf("%+v", *a)
	}
	if mv(o).CommandTicks[52].IsZero() {
		t.Fatal("index 52 not charged")
	}
	b.handleAttackHelper(o, []byte{0xC1, 6, 0xF3, 0x0A, 1, 0})
	if a.Active {
		t.Fatal("second F3 0A must stop")
	}
	// A skill outside the table gives 0x23F.
	b.drain(o)
	b.handleAttackHelper(o, []byte{0xC1, 6, 0xF3, 0x0A, 9, 0})
	hasText(t, b.drain(o), b.messageFor(o, 0x23F))
}

func TestAttackPotionsReuseSlot(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 2)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1)
	withConfig(t, b, "Custom", map[string]string{"CustomAttackPotion_AL0": "1", "CustomAttackPotionRate1": "50"})
	o.P.Char.Life = 1
	give(t, b, o, 14*512+3, 0, 1)
	attackOf(o).Active = true
	b.attackPotions(o)
	if b.countItems(o, 14*512+3, 0) != 0 {
		t.Fatal("potion not used")
	}
	if !attackOf(o).NextPotion.After(b.clock) {
		t.Fatal("potion delay not set")
	}
}

// --- hard-coded ------------------------------------------------------------------------

func TestVendoCompro(t *testing.T) {
	if p := packetAd(0x5401, "COMPRO: x"); !bytes.Equal(p, append([]byte{0xC1, 15, 0x01, 0x54, 0x01}, []byte("COMPRO: x\x00")...)) {
		t.Fatalf("% X", p)
	}
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Vend", 0, 130, 130)
	v := b.player(PlayerStart+1, "View", 0, 131, 130)
	b.see(o, v)
	b.runCommand(o, "/vendo asas +13")
	want := packetAd(o.Index, "[ VENDO asas +13 ]")
	if p := find(b.drain(v), 0x01, -1); !bytes.Equal(p, want) {
		t.Fatalf("viewer % X", p)
	}
	b.drain(o)
	b.advance(time.Second)
	b.adSecond()
	if find(b.drain(v), 0x01, -1) != nil {
		t.Fatal("repeated before 3 seconds")
	}
	b.advance(2 * time.Second)
	b.adSecond()
	if find(b.drain(v), 0x01, -1) == nil {
		t.Fatal("not repeated")
	}
	b.runCommand(o, "/compro "+string(bytes.Repeat([]byte("x"), 52)))
	hasText(t, b.drain(o), "Use /compro mensagem (ate 51 caracteres), ou /compro off.")
	b.runCommand(o, "/vendo off")
	hasText(t, b.drain(o), "Anuncio acima do personagem desativado.")
	if p := find(b.drain(v), 0x01, -1); !bytes.Equal(p, packetAd(o.Index, "")) || custom(o).Ad.Type != 0 {
		t.Fatalf("off % X", p)
	}
}

func TestSuperDrop(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Gm", 0, 130, 130)
	v := b.player(PlayerStart+1, "Usr", 0, 131, 130)
	b.runCommand(o, "/superdrop")
	hasText(t, b.drain(o), "/superdrop: comando exclusivo para Game Master.")
	mv(o).Authority = 8
	b.runCommand(o, "/superdrop")
	if p := find(b.drain(v), 0xF3, 0xFA); !bytes.Equal(p, []byte{0xC1, 5, 0xF3, 0xFA, 1}) || !b.superDropOn() {
		t.Fatalf("% X", p)
	}
	// An excellent item can be dropped now.
	x, y := b.openRow(0, 2)
	v.X, v.Y = x, y
	exc := newItem(0, 0)
	exc.Excellent = 1
	v.P.Inventory[12] = exc
	b.handleDrop(v, []byte{0xC1, 6, 0x23, byte(x + 1), byte(y), 12})
	if p := find(b.drain(v), 0x23, -1); p == nil || p[3] != 1 {
		t.Fatalf("drop % X", p)
	}
	b.runCommand(o, "/superdrop")
	hasText(t, b.drain(o), "SuperDrop DESATIVADO.")
}

func TestCustomCombo(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Knight", 0, 130, 130)
	o.Class = 1
	b.runCommand(o, "/combo now")
	hasText(t, b.drain(o), "Use apenas /combo ou /combo off.")
	b.runCommand(o, "/combo")
	hasText(t, b.drain(o), "Aprenda Twisting Slash e Death Stab para ativar /combo.")
	o.P.Skills[0] = Skill{Index: 0x29}
	o.P.Skills[1] = Skill{Index: 0x2B}
	o.P.Char.Energy = 1000
	b.runCommand(o, "/combo")
	pk := b.drain(o)
	if find(pk, 0xF3, 0xF8) == nil {
		t.Fatal("no F3 F8")
	}
	cb := custom(o).Combo
	if !cb.On || cb.Skills != [3]int{0x16, 0x29, 0x2B} {
		t.Fatalf("%+v", cb)
	}
	if b.findSkill(o, 0x16) != nil && find(pk, 0xF3, 0x11) == nil {
		t.Fatal("Cyclone learned without F3 11")
	}
	// Sequence: 22, 41, 43 within 3 seconds each.
	if b.customComboCheck(o, 0x16) || b.customComboCheck(o, 0x29) || !b.customComboCheck(o, 0x2B) {
		t.Fatal("sequence")
	}
	b.customComboCheck(o, 0x16)
	b.advance(4 * time.Second)
	if b.customComboCheck(o, 0x29) {
		t.Fatal("expired step completed")
	}
	if !customWeaponSkillBypass(o, 0x16) {
		t.Fatal("Cyclone needs a weapon with /combo")
	}
	b.runCommand(o, "/combo off")
	if custom(o).Combo.On {
		t.Fatal("still on")
	}
}

func TestMyInfo(t *testing.T) {
	b := newBareWorld(t)
	o := offlinePlayer(t, b, PlayerStart, "Info")
	b.store.(*store.Memory).SetCredits(o.P.Account.ID, 77)
	o.P.Account.Level = 2
	o.P.Account.LevelExpiresAt = time.Date(2026, 12, 31, 23, 0, 0, 0, time.Local)
	b.runCommand(o, "/minhainfo")
	b.runAsync()
	pk := b.drain(o)
	hasText(t, pk, "[Minha info] Info | Saldo: 77 creditos")
	hasText(t, pk, "Assinatura: "+b.messageFor(o, 0x2EC))
	hasText(t, pk, "Validade VIP: 2026-12-31 23:00:00")
	hasText(t, pk, "Online acumulado: 0 h 0 min")
	// Three seconds between requests.
	b.runCommand(o, "/minhainfo")
	b.runAsync()
	if len(b.drain(o)) != 0 {
		t.Fatal("answered within 3 seconds")
	}
}

func TestAttackIndex52AndStop(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 4)
	o := skillPlayer(b, PlayerStart, 0, x, y, 1)
	c := command{Index: 52}
	// "" uses CustomAttackBuffEnable_AL (0) and the first table skill.
	b.commandAttack(o, c, "")
	a := attackOf(o)
	if !a.Active || a.Skill != 1 || a.Buff != 0 {
		t.Fatalf("%+v", *a)
	}
	// /attack off stops it with 0x23C.
	b.drain(o)
	b.runCommand(o, "/attack off")
	pk := b.drain(o)
	if a.Active || find(pk, 0xF3, 0xEB) == nil {
		t.Fatal("not stopped")
	}
	hasText(t, pk, b.messageFor(o, 0x23C))
	b.commandAttack(o, c, "0 9")
	hasText(t, b.drain(o), b.messageFor(o, 0x23F))
	// Leaving the 5-cell range of the start stops the attack.
	b.commandAttack(o, c, "0 1")
	o.X += 6
	b.attackTick()
	if a.Active {
		t.Fatal("still active out of range")
	}
	// Outside CustomAttackMapZone or in a safe zone: 0x23E.
	o.X, o.Y = b.safeCell(0)
	b.drain(o)
	b.commandAttack(o, c, "")
	hasText(t, b.drain(o), b.messageFor(o, 0x23E))
}
