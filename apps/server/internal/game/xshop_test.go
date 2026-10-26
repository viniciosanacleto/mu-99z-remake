package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// shopAccount creates the store account of a test player with credits.
func shopAccount(t *testing.T, b *bareWorld, o *Object, credits int64) *store.Memory {
	t.Helper()
	mem := b.store.(*store.Memory)
	acc, err := mem.CreateAccount(context.Background(), o.P.Account.Login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	mem.SetCredits(acc.ID, credits)
	return mem
}

func credits(t *testing.T, b *bareWorld, login string) int64 {
	t.Helper()
	n, err := b.store.(store.CreditStore).Credits(context.Background(), login)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestXShopPacketLayouts(t *testing.T) {
	if p := packetXShopCredits(0x01020304); !bytes.Equal(p, []byte{0xC1, 8, 0xFA, 0x03, 4, 3, 2, 1}) {
		t.Fatalf("FA 03 % X", p)
	}
	want := []byte{0xC1, 0x10, 0xFA, 0x04, 7, 0, 0, 0, 0x64, 0, 0, 0, 0x1d, 0x00, 0, 0}
	if p := packetXShopResult(7, 100, 29); !bytes.Equal(p, want) {
		t.Fatalf("FA 04 % X", p)
	}
	if p := packetXShopGMResult(3); len(p) != 12 || p[1] != 12 || p[3] != 6 || p[4] != 3 {
		t.Fatalf("FA 06 % X", p)
	}
	p := packetXShopPage(2, 1, 300, []xshopEntry{{0x401, 0x11223344, 5}, {0x402, 9, 1}})
	if len(p) != 0x68 || p[1] != 0x68 || p[3] != 0x09 || p[4] != 2 || p[5] != 1 || p[6] != 0xFF || p[7] != 2 {
		t.Fatalf("FA 09 header % X", p[:8])
	}
	if !bytes.Equal(p[8:20], []byte{1, 4, 0, 0, 0x44, 0x33, 0x22, 0x11, 5, 0, 0, 0}) || p[20] != 2 || p[24] != 9 || p[28] != 1 {
		t.Fatalf("FA 09 entries % X", p[8:32])
	}
	if p := packetXShopPage(0, 0, 3, nil); p[6] != 3 || p[7] != 0 {
		t.Fatalf("FA 09 pages % X", p[:8])
	}
}

func TestXShopOptionRules(t *testing.T) {
	cases := []struct {
		o     [5]byte
		flags byte
		ok    bool
	}{
		{[5]byte{}, 0, true},
		{[5]byte{1}, 0, false},          // no options without bit 0
		{[5]byte{15, 0, 1, 7}, 1, true}, // level, luck, option
		{[5]byte{16}, 7, false},
		{[5]byte{0, 1}, 1, false}, // skill needs bit 1
		{[5]byte{0, 1}, 3, true},
		{[5]byte{0, 0, 0, 0, 1}, 3, false}, // excellent needs bit 2
		{[5]byte{0, 0, 0, 0, 0x3f}, 5, true},
		{[5]byte{0, 0, 0, 0, 0x40}, 7, false},
		{[5]byte{0, 0, 2}, 7, false},
		{[5]byte{0, 0, 0, 8}, 7, false},
	}
	for i, c := range cases {
		if got := xshopOptsValid(c.o, c.flags); got != c.ok {
			t.Fatalf("case %d: %v", i, got)
		}
	}
	b := newBareWorld(t)
	for _, c := range []struct {
		idx  int
		want byte
	}{
		{0, 5},           // Kris: options, excellent, no skill (Item.txt Skill 0)
		{29, 7},          // Sonic Blade has a skill
		{0x13, 3},        // 0,19 never gets the excellent bit
		{5 * 512, 5},     // staff: no skill rule for 5,x
		{8 * 512, 5},     // armour
		{12 * 512, 0},    // Wings of Elf: HaveOption 0
		{12*512 + 3, 1},  // Wings of Spirit: options only, never excellent
		{13 * 512, 0},    // category 13 is not equipment for the XShop
		{14*512 + 13, 0}, // jewels
	} {
		if got := b.xshopFlags(c.idx); got != c.want {
			t.Fatalf("flags of %#x = %d, want %d", c.idx, got, c.want)
		}
	}
	if !b.xshopHighLevel(8, 0) || b.xshopHighLevel(7, 0) || b.xshopHighLevel(9, 14*512+13) ||
		!b.xshopHighLevel(15, 12*512) || b.xshopHighLevel(10, 14*512+15) {
		t.Fatal("option 0x40 rule")
	}
}

func TestXShopListPage(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 130, 130)
	shopAccount(t, b, o, 77)
	b.xshopList(o, []byte{0xC1, 6, 0xFA, 0x08, 0, 0})
	pk := b.drain(o)
	p := find(pk, 0xFA, 0x09)
	if p == nil {
		t.Fatal("no FA 09")
	}
	var n int
	for idx := 0; idx < 512; idx++ {
		if _, ok := b.itemInfo(idx); ok && b.cw2().prices.Price(idx) != 0 {
			n++
		}
	}
	if p[4] != 0 || p[5] != 0 || int(p[6]) != (n+7)/8 || p[7] != 8 {
		t.Fatalf("header % X (items %d)", p[:8], n)
	}
	// Kris costs 20 (CustomXShop.txt override) with flags 5.
	if binary.LittleEndian.Uint16(p[8:]) != 0 || binary.LittleEndian.Uint32(p[12:]) != 20 || p[16] != 5 {
		t.Fatalf("first entry % X", p[8:20])
	}
	b.runAsync()
	if p := find(b.drain(o), 0xFA, 0x03); !bytes.Equal(p, packetXShopCredits(77)) {
		t.Fatalf("credits % X", p)
	}
	// 120 ms limit, then a page past the end is clamped to the last one.
	b.xshopList(o, []byte{0xC1, 6, 0xFA, 0x08, 0, 200})
	if find(b.drain(o), 0xFA, 0x09) != nil {
		t.Fatal("list inside 120 ms")
	}
	b.advance(120 * time.Millisecond)
	b.xshopList(o, []byte{0xC1, 6, 0xFA, 0x08, 0, 200})
	if p := find(b.drain(o), 0xFA, 0x09); p == nil || int(p[5]) != (n+7)/8-1 || int(p[7]) != n-8*int(p[5]) {
		t.Fatalf("last page % X", p[:8])
	}
	// Category 16 is refused.
	b.advance(time.Second)
	b.xshopList(o, []byte{0xC1, 6, 0xFA, 0x08, 16, 0})
	if len(b.drain(o)) != 0 {
		t.Fatal("category 16 answered")
	}
}

func buyPacket(idx int, opts [5]byte, price uint32) []byte {
	p := []byte{0xC1, 0x10, 0xFA, 0x07, byte(idx), byte(idx >> 8)}
	p = append(p, opts[:]...)
	p = append(p, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(p[12:], price)
	return p
}

func TestXShopPurchase(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 130, 130)
	shopAccount(t, b, o, 100)
	// Sonic Blade (0,29) +15 +skill +luck +option 2 +2 excellent: the shop
	// price plus 4 credits for +15.
	price := b.cw2().prices.Price(29)
	opts := [5]byte{15, 1, 1, 2, 3}
	b.xshopBuy(o, buyPacket(29, opts, price))
	if r := find(b.drain(o), 0xFA, 0x04); !bytes.Equal(r, packetXShopResult(xshopBadPrice, 0, 29)) {
		t.Fatalf("missing +4: % X", r)
	}
	b.xshopBuy(o, buyPacket(29, opts, price+4))
	if len(b.drain(o)) != 0 || b.cw2().pending[o.Index] == nil {
		t.Fatal("purchase not pending")
	}
	// A second purchase while the first is pending is busy.
	b.advance(time.Second)
	b.xshopBuy(o, buyPacket(0, [5]byte{}, 20))
	if r := find(b.drain(o), 0xFA, 0x04); r == nil || r[4] != xshopBusy {
		t.Fatalf("pending % X", r)
	}
	b.runAsync()
	r := find(b.drain(o), 0xFA, 0x04)
	want := 100 - int64(price) - 4
	if !bytes.Equal(r, packetXShopResult(xshopOK, uint32(want), 29)) {
		t.Fatalf("result % X", r)
	}
	if credits(t, b, o.P.Account.Login) != want {
		t.Fatal("credits not debited")
	}
	var got *gamedataItemView
	for _, it := range o.P.Inventory[BagStart:BagEnd] {
		if it.Index == 29 {
			got = &gamedataItemView{it.Level, it.Skill, it.Luck, it.Option, it.Excellent, it.OptionEx}
		}
	}
	if got == nil || *got != (gamedataItemView{15, true, true, 2, 3, 0x40}) {
		t.Fatalf("item %+v", got)
	}
	if b.cw2().pending[o.Index] != nil {
		t.Fatal("pending left")
	}
}

type gamedataItemView struct {
	Level       int
	Skill, Luck bool
	Option      int
	Exc, OptEx  byte
}

func TestXShopPurchaseRefusals(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 130, 130)
	shopAccount(t, b, o, 5)
	result := func() byte {
		t.Helper()
		r := find(b.drain(o), 0xFA, 0x04)
		if r == nil {
			t.Fatal("no FA 04")
		}
		return r[4]
	}
	b.xshopBuy(o, buyPacket(0x1FFF, [5]byte{}, 25))
	if r := result(); r != xshopNoItem {
		t.Fatalf("unknown item %d", r)
	}
	b.xshopBuy(o, buyPacket(0, [5]byte{0, 1}, 20)) // Kris has no skill
	if r := result(); r != xshopBadOpts {
		t.Fatalf("skill on Kris %d", r)
	}
	// Not enough credits: the DataServer answers with the balance.
	b.xshopBuy(o, buyPacket(0, [5]byte{}, 20))
	b.runAsync()
	if r := find(b.drain(o), 0xFA, 0x04); !bytes.Equal(r, packetXShopResult(xshopNoCredit, 5, 0)) {
		t.Fatalf("no credit % X", r)
	}
	// 500 ms between purchases.
	b.xshopBuy(o, buyPacket(0, [5]byte{}, 20))
	if r := result(); r != xshopBusy {
		t.Fatalf("500 ms %d", r)
	}
	// A full bag: 2.
	b.advance(time.Second)
	for s := BagStart; s < BagEnd; s++ {
		o.P.Inventory[s] = newItem(14*512+13, 0)
	}
	b.xshopBuy(o, buyPacket(0, [5]byte{}, 20))
	if r := result(); r != xshopFailed {
		t.Fatalf("full bag %d", r)
	}
}

func TestXShopRefundWhenItemCannotBeGiven(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 130, 130)
	shopAccount(t, b, o, 100)
	b.xshopBuy(o, buyPacket(0, [5]byte{}, 20))
	// The bag fills before the DataServer answers.
	for s := BagStart; s < BagEnd; s++ {
		o.P.Inventory[s] = newItem(14*512+13, 0)
	}
	b.runAsync()
	if r := find(b.drain(o), 0xFA, 0x04); !bytes.Equal(r, packetXShopResult(xshopFailed, 100, 0)) {
		t.Fatalf("refund result % X", r)
	}
	if credits(t, b, o.P.Account.Login) != 100 {
		t.Fatal("not refunded")
	}
	// A reply without a pending record is refunded too.
	b.xshopDebitReply(o.Index, o.P.Account.Login, 0, 20, 99, true, 80)
	b.runAsync()
	if credits(t, b, o.P.Account.Login) != 120 {
		t.Fatal("orphan reply not refunded")
	}
}

func TestXShopGMPrice(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Gm", 0, 130, 130)
	src, err := os.ReadFile(repoConfig("data", "xshop.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "xshop.toml")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	b.cw2().xshopPath = path
	pkt := []byte{0xC1, 0x0C, 0xFA, 0x05, 0, 0, 0, 0, 0x2C, 0x01, 0, 0} // Kris, 300
	mv(o).Authority = 0
	b.xshopGMPrice(o, pkt)
	if p := find(b.drain(o), 0xFA, 0x06); p == nil || p[4] != 1 {
		t.Fatalf("not GM % X", p)
	}
	mv(o).Authority = 8
	b.xshopGMPrice(o, pkt)
	if p := find(b.drain(o), 0xFA, 0x06); p == nil || p[4] != 0 || b.cw2().prices.Price(0) != 300 {
		t.Fatalf("GM % X", p)
	}
	data, _ := os.ReadFile(path)
	if !regexp.MustCompile(`Type = 0, +Index = 0, +Price = 300, +Comment = "Kris"`).Match(data) {
		t.Fatal("file not rewritten")
	}
	if x, _, err := gamedata.LoadXShopPrices(path); err != nil || x.Price(0) != 300 || x.Price(1) != 10 {
		t.Fatalf("reloaded prices %v", err)
	}
	b.xshopGMPrice(o, pkt)
	if p := find(b.drain(o), 0xFA, 0x06); p == nil || p[4] != 2 {
		t.Fatalf("300 ms % X", p)
	}
	b.advance(time.Second)
	big := append([]byte(nil), pkt...)
	binary.LittleEndian.PutUint32(big[8:], 2000000001)
	b.xshopGMPrice(o, big)
	if p := find(b.drain(o), 0xFA, 0x06); p == nil || p[4] != 2 {
		t.Fatalf("price over 2e9 % X", p)
	}
	b.advance(time.Second)
	b.cw2().xshopPath = filepath.Join(t.TempDir(), "missing.toml")
	b.xshopGMPrice(o, pkt)
	if p := find(b.drain(o), 0xFA, 0x06); p == nil || p[4] != 3 {
		t.Fatalf("write fail % X", p)
	}
}

// rechargeChunks collects the FA 0F chunks into the 0x23f3-byte block.
func rechargeChunks(t *testing.T, pk [][]byte) *[rechargeSize]byte {
	t.Helper()
	d := new([rechargeSize]byte)
	n := 0
	for _, p := range pk {
		if len(p) < 4 || p[2] != 0xFA || p[3] != 0x0F {
			continue
		}
		if len(p) != 0xC6 {
			t.Fatalf("chunk size %d", len(p))
		}
		off := int(binary.LittleEndian.Uint16(p[4:]))
		if off != n*rechargeChunk {
			t.Fatalf("chunk %d offset %d", n, off)
		}
		copy(d[off:], p[6:6+min(rechargeChunk, rechargeSize-off)])
		n++
	}
	if n != 48 {
		t.Fatalf("%d chunks", n)
	}
	return d
}

func TestXShopRechargeFromOriginalFiles(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Buyer", 0, 130, 130)
	b.xshopRecharge(o, []byte{0xC1, 4, 0xFA, 0x0E})
	d := rechargeChunks(t, b.drain(o))
	// The shipped file is inactive with every field empty and no image:
	// the 48 chunks carry an all-zero block.
	if *d != ([rechargeSize]byte{}) {
		t.Fatalf("block not empty: active %d holder %q key %q side %d", d[rcActive],
			fieldText(d[rcHolder:rcHolder+0x60]), fieldText(d[rcKey:rcKey+0x80]),
			binary.LittleEndian.Uint16(d[rcQRSize:]))
	}
	// 3 s between requests.
	b.xshopRecharge(o, []byte{0xC1, 4, 0xFA, 0x0E})
	if len(b.drain(o)) != 0 {
		t.Fatal("inside 3 s")
	}
}

func TestEMVAmount(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"000201" + "540525.00" + "6304ABCD", "25.00", true},
		{"00020126", "", false},               // length past the end
		{"0002015404" + "1.00", "1.00", true}, // tag 54 last
		{"5404" + "1.00" + "5404" + "2.00", "", false},
		{"0A0201", "", false},
		{"000201", "", false}, // no tag 54
	} {
		v, ok := emvAmount(c.in)
		if v != c.want || ok != c.ok {
			t.Fatalf("%q: %q %v", c.in, v, ok)
		}
	}
	if !amountText("25.00") || amountText("2500") || amountText("2a.00") || amountText(".00") {
		t.Fatal("amount format")
	}
}

func TestXShopRechargeUploadRoundTrip(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Gm", 0, 130, 130)
	dir := t.TempDir()
	path := filepath.Join(dir, "xshop.toml")
	b.cw2().xshopPath = path
	b.dataDir = dir // the QR image goes to <data>/Custom
	if err := os.MkdirAll(filepath.Join(dir, "Custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := new([rechargeSize]byte)
	d[rcActive] = 1
	copy(d[rcHolder:], "Fulano")
	copy(d[rcKey:], "chave@pix")
	copy(d[rcCopy:], "000201"+"540510.50"+"6304ABCD")
	binary.LittleEndian.PutUint16(d[rcQRSize:], 16)
	for i := 0; i < 16*16; i += 3 {
		d[rcQRBits+i/8] |= 1 << (i % 8)
	}
	send := func() [][]byte {
		var acks [][]byte
		for off := 0; off < rechargeSize; off += rechargeChunk {
			p := packetRechargeChunk(off, d)
			p[3] = 0x10
			b.xshopRechargeUpload(o, p)
			acks = append(acks, b.drain(o)...)
		}
		return acks
	}
	mv(o).Authority = 0
	if acks := send(); len(acks) != 48 {
		t.Fatalf("not GM: %d acks", len(acks))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("saved without GM")
	}
	mv(o).Authority = 8
	acks := send()
	if len(acks) != 1 || !bytes.Equal(acks[0], []byte{0xC1, 5, 0xFA, 0x11, 0}) {
		t.Fatalf("acks %d % X", len(acks), acks)
	}
	f, err := gamedata.LoadXShopConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	r := f.CustomXShopRecharge
	if r.Ativo != 1 || r.Titular != "Fulano" || r.ChavePix != "chave@pix" || r.Valor != "10.50" ||
		!strings.HasPrefix(r.ImagemBMP, "pix") || !strings.HasSuffix(r.ImagemBMP, ".tmp.bmp") {
		t.Fatalf("config %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "Custom", r.ImagemBMP)); err != nil {
		t.Fatalf("image not in <data>/Custom: %v", err)
	}
	// The saved file reads back as the same block.
	b.xshopRecharge(o, []byte{0xC1, 4, 0xFA, 0x0E})
	got := rechargeChunks(t, b.drain(o))
	copy(d[rcValue:], "10.50")
	if *got != *d {
		t.Fatal("round trip differs")
	}
	// A chunk out of order breaks the upload (ack) and a new start inside
	// 3 s is refused.
	p := packetRechargeChunk(0, d)
	p[3] = 0x10
	b.xshopRechargeUpload(o, p)
	if len(b.drain(o)) != 1 {
		t.Fatal("restart inside 3 s not acknowledged")
	}
}

func TestRechargeImageRoundTrip(t *testing.T) {
	d := new([rechargeSize]byte)
	binary.LittleEndian.PutUint16(d[rcQRSize:], 5)
	d[rcQRBits] = 0b10100101
	d[rcQRBits+3] = 0x01 // bit 24, the last of 5x5
	path := filepath.Join(t.TempDir(), "q.bmp")
	if err := os.WriteFile(path, encodeRechargeBMP(d), 0o644); err != nil {
		t.Fatal(err)
	}
	var back [rechargeSize]byte
	if !loadRechargeImage(path, &back) || back != *d {
		t.Fatalf("round trip % X", back[rcQRSize:rcQRSize+8])
	}
	// A 24-bit bottom-up 4x2 image scales into a 4x4 matrix, centred.
	img := []byte{'B', 'M', 0, 0, 0, 0, 0, 0, 0, 0, 0x36, 0, 0, 0, 0x28, 0, 0, 0, 4, 0, 0, 0, 2, 0, 0, 0, 1, 0, 24, 0}
	img = append(img, make([]byte, 0x36-len(img))...)
	// bottom row (y=1): white, black, white, black; top row (y=0): black x4
	img = append(img, 0xff, 0xff, 0xff, 0, 0, 0, 0xff, 0xff, 0xff, 0, 0, 0)
	img = append(img, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatal(err)
	}
	clear(back[:])
	if !loadRechargeImage(path, &back) || binary.LittleEndian.Uint16(back[rcQRSize:]) != 4 {
		t.Fatal("24-bit image")
	}
	// rows 1 and 2 of the 4x4 matrix: 1111 then 0101 (bits 4..11).
	if back[rcQRBits] != 0xF0 || back[rcQRBits+1] != 0x0A {
		t.Fatalf("bits % X", back[rcQRBits:rcQRBits+2])
	}
	if loadRechargeImage(filepath.Join(t.TempDir(), "none.bmp"), &back) {
		t.Fatal("missing file loaded")
	}
}

func TestCashHeadRouting(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Router", 0, 130, 130)
	// FA: C1 only and the exact size of each sub-code (FUN_004d6690).
	worldHandlers[0xFA](b.World, o, []byte{0xC1, 7, 0xFA, 0x08, 0, 0, 0})
	worldHandlers[0xFA](b.World, o, []byte{0xC2, 0, 6, 0xFA, 0x08, 0})
	if len(b.drain(o)) != 0 {
		t.Fatal("wrong size or kind answered")
	}
	worldHandlers[0xFA](b.World, o, []byte{0xC1, 6, 0xFA, 0x08, 0, 0})
	if find(b.drain(o), 0xFA, 0x09) == nil {
		t.Fatal("FA 08 not routed")
	}
	worldHandlers[0xFB](b.World, o, []byte{0xC1, 4, 0xFB, 0x16})
	if len(cash(o).Spins) != 1 {
		t.Fatal("FB 16 not routed")
	}
	if f3Handlers[0xF5] == nil || f3Handlers[0xF6] == nil {
		t.Fatal("jewel bank handlers")
	}
}
