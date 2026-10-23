package game

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func pshopReq(sub byte, body ...byte) []byte {
	return frame.Build(frame.C3, 0x3F, append([]byte{sub}, body...)...)
}

func TestPShopPacketLayouts(t *testing.T) {
	if p := packetPShopPrice(1, 80); !bytes.Equal(p, []byte{0xC1, 6, 0x3F, 1, 1, 80}) {
		t.Fatalf("3F 01 = % X", p)
	}
	if p := packetPShopOpen(2); !bytes.Equal(p, []byte{0xC1, 5, 0x3F, 2, 2}) {
		t.Fatalf("3F 02 = % X", p)
	}
	if p := packetPShopClose(0x5400); !bytes.Equal(p, []byte{0xC1, 7, 0x3F, 3, 1, 0x54, 0}) {
		t.Fatalf("3F 03 = % X", p)
	}
	inv := make([]item.Item, InventorySlots)
	for i := range inv {
		inv[i] = item.Empty()
	}
	inv[80] = item.Item{Index: item.Make(14, 13), Durability: 1, PShopPrice: 0x01020304}
	title := []byte("Cheap jewels")
	l := packetPShopList(5, 1, 0x5401, "Seller", title, inv)
	if len(l) != 0x37+12 || l[0] != 0xC2 || int(l[1])<<8|int(l[2]) != len(l) || l[3] != 0x3F || l[4] != 5 || l[5] != 1 ||
		l[6] != 0x54 || l[7] != 1 || string(l[8:14]) != "Seller" || string(l[18:30]) != "Cheap jewels" || l[54] != 1 ||
		l[55] != 80 || l[56] != 13 || binary.LittleEndian.Uint32(l[63:]) != 0x01020304 {
		t.Fatalf("C2 3F 05 = % X", l)
	}
	if l := packetPShopList(5, 3, 0xFFFF, "", nil, inv); len(l) != 0x37 || l[5] != 3 || l[6] != 0xFF || l[54] != 0 {
		t.Fatalf("closed shop list = % X", l)
	}
	b := packetPShopBuy(1, 0x5401, item.Item{Index: item.Make(14, 13), Durability: 1}, 20)
	if len(b) != 15 || b[3] != 6 || b[4] != 1 || b[5] != 0x54 || b[6] != 1 || b[7] != 13 || b[14] != 20 {
		t.Fatalf("3F 06 = % X", b)
	}
	if s := packetPShopSold(80, "Buyer"); len(s) != 15 || s[3] != 8 || s[4] != 80 || string(s[5:10]) != "Buyer" {
		t.Fatalf("3F 08 = % X", s)
	}
	if p := packetPShopTitle(0x5401, title, "Seller"); len(p) != 0x34 || p[3] != 0x10 || p[5] != 1 ||
		string(p[6:18]) != "Cheap jewels" || string(p[42:48]) != "Seller" {
		t.Fatalf("3F 10 = % X", p)
	}
	if p := packetPShopGone(0xFFFF); !bytes.Equal(p, []byte{0xC1, 6, 0x3F, 0x12, 0xFF, 0xFF}) {
		t.Fatalf("3F 12 = % X", p)
	}
}

func TestPShopPriceOpenBuy(t *testing.T) {
	b := newBareWorld(t)
	seller := b.player(PlayerStart, "Seller", 0, 130, 130)
	buyer := b.player(PlayerStart+1, "Buyer", 0, 131, 130)
	b.see(seller, buyer)
	for i := range buyer.VP {
		if buyer.VP[i].Index == seller.Index {
			buyer.VP[i].State = VPVisible // after the viewport send
		}
	}
	seller.P.Char.Money = 0
	buyer.P.Char.Money = 5000

	// Move a Jewel of Bless into the shop area (24 flag 4, result 0).
	seller.P.Inventory[BagStart] = item.Item{Index: idxBless, Durability: 1, Serial: 77}
	b.handleItemMove(seller, move24(0, BagStart, 4, 80))
	if m := find(b.drain(seller), 0x24, -1); m == nil || m[3] != 0 || m[4] != 80 || seller.P.Inventory[80].Index != idxBless {
		t.Fatalf("inventory -> shop = % X", m)
	}
	// Opening without a price: 3F 02 00.
	title := make([]byte, 36)
	copy(title, "Jewels")
	b.handlePShop(seller, pshopReq(2, title...))
	if r := find(b.drain(seller), 0x3F, 2); r == nil || r[4] != 0 {
		t.Fatalf("open without price = % X", r)
	}
	// 3F 01: bad slot 2, empty 3, price 0 -> 4, ok 1.
	price := func(slot byte, v uint32) byte {
		var p [4]byte
		binary.LittleEndian.PutUint32(p[:], v)
		b.handlePShop(seller, pshopReq(1, append([]byte{slot}, p[:]...)...))
		r := find(b.drain(seller), 0x3F, 1)
		if r == nil || r[5] != slot {
			t.Fatalf("3F 01 = % X", r)
		}
		return r[4]
	}
	if price(108, 10) != 2 || price(81, 10) != 3 || price(80, 0) != 4 || price(80, 0x80000000) != 4 || price(80, 3000) != 1 {
		t.Fatal("3F 01 results")
	}
	if seller.P.Inventory[80].PShopPrice != 3000 {
		t.Fatalf("price = %d", seller.P.Inventory[80].PShopPrice)
	}
	b.handlePShop(seller, pshopReq(2, title...))
	if r := find(b.drain(seller), 0x3F, 2); r == nil || r[4] != 1 || !pshopOpen(seller) {
		t.Fatalf("open = % X", r)
	}
	// Items cannot move while the shop is open.
	b.handleItemMove(seller, move24(4, 80, 0, BagStart))
	if m := find(b.drain(seller), 0x24, -1); m == nil || m[3] != 0xFF {
		t.Fatalf("move with open shop = % X", m)
	}
	// The buyer sees the title through the viewport pass.
	b.pshopViewportProc()
	v := find(b.drain(buyer), 0x3F, 0)
	if v == nil || v[5] != 1 || int(v[6])<<8|int(v[7]) != seller.Index || string(v[8:14]) != "Jewels" {
		t.Fatalf("C2 3F 00 = % X", v)
	}
	b.pshopViewportProc()
	if v := find(b.drain(buyer), 0x3F, 0); v != nil {
		t.Fatal("known shop announced twice")
	}
	// 3F 05 with the wrong name: 2; with the right one: the list.
	req := append([]byte{byte(seller.Index >> 8), byte(seller.Index)}, name10("Nobody")...)
	b.handlePShop(buyer, pshopReq(5, req...))
	if l := find(b.drain(buyer), 0x3F, 5); l == nil || l[5] != 2 {
		t.Fatalf("view wrong name = % X", l)
	}
	req = append([]byte{byte(seller.Index >> 8), byte(seller.Index)}, name10("Seller")...)
	b.handlePShop(buyer, pshopReq(5, req...))
	if l := find(b.drain(buyer), 0x3F, 5); l == nil || l[5] != 1 || l[54] != 1 || l[55] != 80 || pshop(buyer).Viewing != seller.Index {
		t.Fatalf("view = % X", l)
	}
	// Not enough zen: 7.
	buyer.P.Char.Money = 100
	b.handlePShop(buyer, pshopReq(6, append(req, 80)...))
	if r := find(b.drain(buyer), 0x3F, 6); r == nil || r[4] != 7 || r[14] != 0 {
		t.Fatalf("buy without zen = % X", r)
	}
	// Purchases are 2 s apart.
	b.handlePShop(buyer, pshopReq(6, append(req, 80)...))
	if r := find(b.drain(buyer), 0x3F, 6); r != nil {
		t.Fatal("second purchase within 2 s answered")
	}
	b.advance(2000 * 1000 * 1000)
	buyer.P.Char.Money = 5000
	b.handlePShop(buyer, pshopReq(6, append(req, 80)...))
	pk := b.drain(buyer)
	r := find(pk, 0x3F, 6)
	if r == nil || r[4] != 1 || r[14] < BagStart || buyer.P.Inventory[r[14]].Index != idxBless || buyer.P.Char.Money != 2000 {
		t.Fatalf("buy = % X money %d", r, buyer.P.Char.Money)
	}
	sp := b.drain(seller)
	if s := find(sp, 0x3F, 8); s == nil || s[4] != 80 || string(s[5:10]) != "Buyer" {
		t.Fatalf("3F 08 = % X", s)
	}
	if d := find(sp, 0x28, -1); d == nil || d[3] != 80 || d[4] != 1 || seller.P.Char.Money != 3000 {
		t.Fatalf("28 to the seller = % X money %d", d, seller.P.Char.Money)
	}
	// The shop was emptied: it closes and VP2 gets 3F 03.
	if c := find(sp, 0x3F, 3); c == nil || pshopOpen(seller) {
		t.Fatalf("shop not closed: % X", c)
	}
	if c := find(pk, 0x3F, 3); c == nil {
		t.Fatal("viewer not told the shop closed")
	}
	// The viewer is dropped on the next pass (3F 12 FFFF).
	b.pshopViewportProc()
	if g := find(b.drain(buyer), 0x3F, 0x12); g == nil || pshop(buyer).Viewing != -1 {
		t.Fatalf("3F 12 = % X", g)
	}
}

func TestPShopMoveRules(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Seller", 0, 130, 130)
	o.P.Inventory[BagStart] = item.Item{Index: idxBless, Durability: 1}
	b.handleItemMove(o, move24(0, BagStart, 4, 76))
	b.drain(o)
	b.handleItemMove(o, move24(4, 76, 4, 107))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0 || o.P.Inventory[107].Index != idxBless {
		t.Fatalf("shop -> shop = % X", m)
	}
	b.handleItemMove(o, move24(4, 107, 0, BagStart+1))
	if m := find(b.drain(o), 0x24, -1); m == nil || m[3] != 0 || o.P.Inventory[BagStart+1].Index != idxBless {
		t.Fatalf("shop -> inventory = % X", m)
	}
	// Account lock refuses with message 0x6F2.
	o.P.Account.Lock = 1
	b.handleItemMove(o, move24(0, BagStart+1, 4, 76))
	pk := b.drain(o)
	if m := find(pk, 0x24, -1); m == nil || m[3] != 0xFF || find(pk, 0x0D, -1) == nil {
		t.Fatalf("locked account = % X", pk)
	}
}
