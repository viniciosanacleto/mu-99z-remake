package game

import (
	"context"
	"path/filepath"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CCustomXShop (0x401fd0..0x405960, docs/server-plan/10e-customs.md):
// the credits shop of the original client. Prices come
// from Custom/CustomXShop.txt; the currency is MEMB_INFO.Credits, debited
// by the DataServer (DS 0x0F 0x20/0x21/0x22, store.CreditStore). Client
// packets use the private head FA (C1 only):
//
//	FA 05 GM price change (0x403660)     -> FA 06 result
//	FA 07 purchase (0x4032c0)            -> FA 04 result (after DS 0x0F 0x21)
//	FA 08 price list page (0x402e20)     -> FA 09 page, FA 03 credits
//	FA 0E recharge info (0x404a60)       -> FA 0F chunks (xshop_recharge.go)
//	FA 10 GM recharge upload (0x405030)  -> FA 11 ack   (xshop_recharge.go)
//
// The same file holds the shared state of CustomJewel (customjewel.go),
// CustomJewelBank (jewelbank.go) and CLuckyWheel (luckywheel.go).

func init() {
	worldHandlers[0xFA] = func(w *World, o *Object, p []byte) {
		if len(p) > 3 && p[0] == frame.C1 {
			if h := faHandlers[p[3]]; h != nil {
				h(w, o, p)
			}
		}
	}
	worldHandlers[0xFB] = func(w *World, o *Object, p []byte) {
		if len(p) > 3 {
			if h := fbHandlers[p[3]]; h != nil {
				h(w, o, p)
			}
		}
	}
	faHandlers[0x05] = sized(0x0C, (*World).xshopGMPrice)
	faHandlers[0x07] = sized(0x10, (*World).xshopBuy)
	faHandlers[0x08] = sized(0x06, (*World).xshopList)
	startHooks = append(startHooks, loadCashWorld)
}

// sized keeps the exact-size test of the FA dispatcher (FUN_004d6690).
func sized(n int, f func(*World, *Object, []byte)) func(*World, *Object, []byte) {
	return func(w *World, o *Object, p []byte) {
		if len(p) == n {
			f(w, o, p)
		}
	}
}

// Limits of the CCustomXShop handlers.
const (
	xshopListInterval  = 120 * time.Millisecond   // 0x77 < tick difference
	xshopBuyInterval   = 500 * time.Millisecond   // 499 < tick difference
	xshopPendingWindow = 15000 * time.Millisecond // 0x3a98
	xshopGMInterval    = 300 * time.Millisecond
	xshopMaxPrice      = 2000000000
	xshopPageSize      = 8
)

// Results of FA 04 (FUN_00404720).
const (
	xshopOK       = 0
	xshopNoCredit = 1
	xshopFailed   = 2 // no room, or the item could not be given (refunded)
	xshopNoItem   = 3 // unknown item or price 0
	xshopBusy     = 5
	xshopBadOpts  = 6
	xshopBadPrice = 7
)

// cashWorld is the per-instance state of the block 12 customs.
type cashWorld struct {
	xshopPath string // CCustomXShop +0xc4: config/data/xshop.toml (prices and recharge)
	prices    *gamedata.XShopPrices
	jewels    *gamedata.CustomJewels
	wheel     map[int]gamedata.WheelReward
	pending   map[int]*xshopPurchase  // +0x84, keyed by object index
	seq       uint32                  // +0x1c8
	uploads   map[int]*rechargeUpload // +0x04, keyed by object index
	saving    bool                    // DAT_010bc69c (FUN_00405350)
}

// cashState is the per-player state: the jewel bank counters and the tick
// maps (FUN_00405cd0) of the XShop requests.
type cashState struct {
	Bank       [store.JewelBankTypes]int // obj+0x24c4..0x24e8
	ListAt     time.Time
	BuyAt      time.Time
	PriceAt    time.Time
	RechargeAt time.Time
	UploadAt   time.Time
	Spins      []wheelSpin // lucky wheel spins waiting for their 3 s (luckywheel.go)
}

func cash(o *Object) *cashState {
	if o.P.cash == nil {
		o.P.cash = &cashState{}
	}
	return o.P.cash
}

// xshopPurchase is a pending purchase (the 0x2c-byte record of +0x84).
type xshopPurchase struct {
	Account string
	Item    int
	Price   uint32
	At      time.Time
	Seq     uint32
	Name    string
	Opts    [5]byte // level, skill, luck, option, excellent
}

func (w *World) cw2() *cashWorld {
	if w.cash == nil {
		w.cash = &cashWorld{prices: &gamedata.XShopPrices{Overrides: map[int]uint32{}},
			jewels: &gamedata.CustomJewels{}, wheel: map[int]gamedata.WheelReward{},
			pending: map[int]*xshopPurchase{}, uploads: map[int]*rechargeUpload{}}
		for i := range w.cash.prices.Category {
			w.cash.prices.Category[i] = gamedata.XShopDefaultPrice
		}
		for i := range w.cash.jewels.Slots {
			w.cash.jewels.Slots[i].Index = -1
		}
	}
	return w.cash
}

// loadCashWorld reads CustomXShop (FUN_004026e0, config/data/xshop.toml),
// CustomJewel (FUN_00464b70) and CustomLuckyWheel (FUN_0044e3a0).
func loadCashWorld(w *World) error {
	c := w.cw2()
	if w.configDir == "" {
		return nil
	}
	c.xshopPath = filepath.Join(w.configDir, "data", "xshop.toml")
	prices, ok, err := gamedata.LoadXShopPrices(c.xshopPath)
	if err != nil {
		return err
	}
	if !ok {
		w.log.Warn("[CustomXShop] Nao foi possivel abrir. Usando preco padrao 25.", "file", c.xshopPath)
	}
	c.prices = prices
	if c.jewels, err = gamedata.LoadCustomJewel(w.configDir); err != nil {
		return err
	}
	c.wheel, err = gamedata.LoadLuckyWheel(w.configDir)
	return err
}

func (w *World) creditStore() store.CreditStore {
	cs, _ := w.store.(store.CreditStore)
	return cs
}

// --- item rules ----------------------------------------------------------------------

// xshopEquip is FUN_004029f0: equipment of categories 0..11 with
// HaveOption, or wings of category 12 (Slot 7, HaveOption; indexes above
// 12,6 only as CustomWing).
func (w *World) xshopEquip(idx int) bool {
	info, ok := w.itemInfo(idx)
	if !ok || idx < 0 {
		return false
	}
	switch cat := idx >> 9; {
	case cat <= 11:
		return info.HaveOption != 0
	case cat == 12:
		if idx&0x1ff > 6 {
			if _, ok := w.statsData.customWing(idx); !ok {
				return false
			}
		}
		return info.Slot == 7 && info.HaveOption != 0
	}
	return false
}

// itemOptionAllowed is FUN_0049d390 on a probe item (skill, luck and
// option 1, every excellent bit): some ItemOption.txt rule of slot n
// matches.
func (w *World) itemOptionAllowed(idx, n int) bool {
	if w.statsData == nil {
		return false
	}
	for _, r := range w.statsData.options[n] {
		if r.Matches(idx, 1, 1, 1, 0x3f) {
			return true
		}
	}
	return false
}

// xshopFlags is FUN_00402c30, the option byte of each FA 09 entry: bit 0
// options allowed, bit 1 skill, bit 2 excellent. A CustomWing gets 1 only.
func (w *World) xshopFlags(idx int) byte {
	if !w.xshopEquip(idx) {
		return 0
	}
	if _, ok := w.statsData.customWing(idx); ok {
		return 1
	}
	if !w.itemOptionAllowed(idx, 1) || !w.itemOptionAllowed(idx, 2) {
		return 0
	}
	f := byte(1)
	if info, _ := w.itemInfo(idx); info.Skill > 0 && w.itemOptionAllowed(idx, 0) {
		f = 3
	}
	all := true
	for n := 3; n <= 8; n++ {
		if !w.itemOptionAllowed(idx, n) {
			all = false
		}
	}
	switch idx {
	case 0x13, 0x40d, 0x812, 0xa0a, 0xa24:
		all = false
	}
	if idx>>9 <= 11 && all {
		f |= 4
	}
	return f
}

// xshopOptsValid is FUN_00401000: level < 16, skill and luck < 2, option
// < 8, excellent < 0x40; without flag bit 0 everything must be 0, without
// bit 1 the skill and without bit 2 the excellent byte.
func xshopOptsValid(o [5]byte, flags byte) bool {
	if o[0] > 15 || o[1] > 1 || o[2] > 1 || o[3] > 7 || o[4] > 0x3f {
		return false
	}
	if flags&1 == 0 && o != ([5]byte{}) {
		return false
	}
	if flags&2 == 0 && o[1] != 0 {
		return false
	}
	return flags&4 != 0 || o[4] == 0
}

// xshopHighLevel is FUN_00402b20: items bought at +8..+15 get the
// CreateItem option byte 0x40 (parameter 14, CItem+0xd2) when they are not
// jewels and are weapons/armour (0..11), wings (12, Slot 7) or category 13
// items with an equipment slot.
func (w *World) xshopHighLevel(level, idx int) bool {
	if level < 8 || level > 15 || w.jewelItem(idx) {
		return false
	}
	info, ok := w.itemInfo(idx)
	if !ok {
		return false
	}
	switch cat := idx >> 9; {
	case cat >= 0 && cat <= 11:
		return true
	case cat == 12:
		return info.Slot == 7
	case cat == 13:
		return info.Slot >= 0 && info.Slot <= 11
	}
	return false
}

// --- packets -------------------------------------------------------------------------

// packetXShopCredits is C1 08 FA 03 credits(u32) (FUN_00404350).
func packetXShopCredits(credits uint32) []byte {
	p := newPacket(frame.C1, 0xFA, 0x03, 8)
	u32le(p[4:], credits)
	return p
}

// packetXShopResult is C1 10 FA 04 result pad[3] credits(u32) item(u16)
// pad[2] (FUN_00404720; credits in ECX, item in DX, the pads are stack
// bytes the original never writes).
func packetXShopResult(result byte, credits uint32, item int) []byte {
	p := newPacket(frame.C1, 0xFA, 0x04, 0x10)
	p[4] = result
	u32le(p[8:], credits)
	u16le(p[12:], item)
	return p
}

// packetXShopGMResult is C1 0C FA 06 result (FUN_00403860; bytes 5..11
// are not written by the original).
func packetXShopGMResult(result byte) []byte {
	p := newPacket(frame.C1, 0xFA, 0x06, 0x0C)
	p[4] = result
	return p
}

// xshopEntry is one item of an FA 09 page.
type xshopEntry struct {
	Item  int
	Price uint32
	Flags byte
}

// packetXShopPage is C1 68 FA 09 category page pages count + 8 entries of
// 12 bytes: item(u16) pad[2] price(u32) flags pad[3] (FUN_00402ed0).
func packetXShopPage(cat, page, pages int, list []xshopEntry) []byte {
	p := newPacket(frame.C1, 0xFA, 0x09, 0x68)
	p[4], p[5] = byte(cat), byte(page)
	p[6] = 0xFF
	if pages < 0x100 {
		p[6] = byte(pages)
	}
	p[7] = byte(len(list))
	for i, e := range list {
		b := p[8+12*i:]
		u16le(b, e.Item)
		u32le(b[4:], e.Price)
		b[8] = e.Flags
	}
	return p
}

// --- FA 08: price list --------------------------------------------------------------

// xshopList is FUN_00402e20: every 120 ms at most, the page of the items of
// a category (index>>9 clamped to 0..15) that exist in Item.txt and have a
// price, 8 per page (page clamped), then the credits (FUN_00403250, DS
// 0x0F 0x20 -> FA 03).
func (w *World) xshopList(o *Object, p []byte) {
	if !playerReady(o) || p[4] >= 16 {
		return
	}
	s, now := cash(o), w.now()
	if !s.ListAt.IsZero() && now.Sub(s.ListAt) < xshopListInterval {
		return
	}
	s.ListAt = now
	c := w.cw2()
	cat, page := int(p[4]), int(p[5])
	var items []int
	for idx := 0; idx < 0x2000; idx++ {
		if min(idx>>9, 15) != cat {
			continue
		}
		if _, ok := w.itemInfo(idx); ok && c.prices.Price(idx) != 0 {
			items = append(items, idx)
		}
	}
	pages := max((len(items)+7)>>3, 1)
	page = min(max(page, 0), pages-1)
	var list []xshopEntry
	for i := page * xshopPageSize; i < len(items) && len(list) < xshopPageSize; i++ {
		list = append(list, xshopEntry{items[i], c.prices.Price(items[i]), w.xshopFlags(items[i])})
	}
	w.send(o, packetXShopPage(cat, page, pages, list))
	w.xshopSendCredits(o)
}

// xshopSendCredits is FUN_00403250 (DS 0x0F 0x20) and its reply
// FUN_00404350: C1 08 FA 03 with the credits, 0 when the read failed.
func (w *World) xshopSendCredits(o *Object) {
	cs, idx, login := w.creditStore(), o.Index, o.P.Account.Login
	if cs == nil || !validAccountName(login) {
		w.send(o, packetXShopCredits(0))
		return
	}
	w.async(func(ctx context.Context) func() {
		n, err := cs.Credits(ctx, login)
		return func() {
			if o := w.objects[idx]; xshopOnline(o) {
				if err != nil {
					n = 0
				}
				w.send(o, packetXShopCredits(uint32(n)))
			}
		}
	})
}

// xshopOnline is gObjIsConnected (FUN_0050cd20) for the DS replies.
func xshopOnline(o *Object) bool {
	return o != nil && o.IsPlayer() && o.State == StatePlaying && o.P != nil && o.P.Char != nil
}

// --- FA 07: purchase ------------------------------------------------------------------

// xshopBuy is FUN_004032c0: [4] item(u16) [6] level skill luck option
// excellent [12] price(u32). The price the client shows must equal the
// shop price (+4 credits at level 15); the purchase is then kept pending
// and DS 0x0F 0x21 debits the credits.
func (w *World) xshopBuy(o *Object, p []byte) {
	if !playerReady(o) {
		return
	}
	idx := int(p[4]) | int(p[5])<<8
	var opts [5]byte
	copy(opts[:], p[6:11])
	fail := func(r byte) { w.send(o, packetXShopResult(r, 0, idx)) }
	if _, ok := w.itemInfo(idx); !ok {
		fail(xshopNoItem)
		return
	}
	c := w.cw2()
	price := c.prices.Price(idx)
	if price == 0 {
		fail(xshopNoItem)
		return
	}
	extra := uint32(0)
	if opts[0] == 15 {
		extra = 4
	}
	if !xshopOptsValid(opts, w.xshopFlags(idx)) || price > xshopMaxPrice || xshopMaxPrice-price < extra {
		fail(xshopBadOpts)
		return
	}
	total := price + extra
	if uint32(p[12])|uint32(p[13])<<8|uint32(p[14])<<16|uint32(p[15])<<24 != total {
		fail(xshopBadPrice)
		return
	}
	now := w.now()
	if ifBusy(o) || w.dying(o) {
		fail(xshopBusy)
		return
	}
	if r := c.pending[o.Index]; r != nil {
		if r.Account == o.P.Account.Login && now.Sub(r.At) <= xshopPendingWindow {
			fail(xshopBusy)
			return
		}
		delete(c.pending, o.Index)
	}
	s := cash(o)
	if !s.BuyAt.IsZero() && now.Sub(s.BuyAt) < xshopBuyInterval {
		fail(xshopBusy)
		return
	}
	s.BuyAt = now
	if w.findBagSlot(&o.P.Inventory, idx) < 0 {
		fail(xshopFailed)
		return
	}
	c.seq++
	if c.seq == 0 {
		c.seq = 1
	}
	rec := &xshopPurchase{Account: o.P.Account.Login, Item: idx, Price: total, At: now, Seq: c.seq,
		Name: o.P.Char.Name, Opts: opts}
	c.pending[o.Index] = rec
	w.xshopDebit(o.Index, rec)
}

// xshopDebit sends DS 0x0F 0x21 (C1 1C 0F 21 index account item price
// seq) and runs the reply FUN_00404400 on the world goroutine. A store
// without credits or an invalid account name answers "not debited".
func (w *World) xshopDebit(index int, rec *xshopPurchase) {
	cs := w.creditStore()
	login, item, price, seq := rec.Account, rec.Item, rec.Price, rec.Seq
	if cs == nil || !validAccountName(login) {
		w.post(func() { w.xshopDebitReply(index, login, item, price, seq, false, 0) })
		return
	}
	w.async(func(ctx context.Context) func() {
		ok, bal, err := cs.DebitCredits(ctx, login, int64(price))
		if err != nil {
			ok, bal = false, 0
		}
		return func() { w.xshopDebitReply(index, login, item, price, seq, ok, uint32(bal)) }
	})
}

// xshopDebitReply is FUN_00404400. The pending record must match the
// reply (account, item, price, sequence). Debited credits are given back
// (DS 0x0F 0x22, FUN_004046b0) whenever the item cannot be created: no
// record, player gone, another character, open interface or no room.
func (w *World) xshopDebitReply(index int, login string, item int, price, seq uint32, ok bool, balance uint32) {
	c := w.cw2()
	refund := func() {
		if ok && price != 0 {
			w.xshopRefund(login, price)
		}
	}
	rec := c.pending[index]
	if rec == nil || rec.Account != login || rec.Item != item || rec.Price != price || rec.Seq != seq {
		refund()
		return
	}
	delete(c.pending, index)
	o := w.objects[index]
	online := xshopOnline(o)
	if !ok {
		if online {
			w.send(o, packetXShopResult(xshopNoCredit, balance, item))
		}
		return
	}
	if !online {
		refund()
		return
	}
	if o.P.Char.Name == rec.Name && !ifBusy(o) && w.findBagSlot(&o.P.Inventory, item) >= 0 {
		dur := 0
		if info, ok := w.itemInfo(item); ok {
			dur = min(max(maxDurability(info, item, int(rec.Opts[0]), rec.Opts[4] != 0, false), 0), 255)
		}
		var optEx byte
		if w.xshopHighLevel(int(rec.Opts[0]), item) {
			optEx = 0x40
		}
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: item, Level: int(rec.Opts[0]), Dur: dur,
			Skill: int(rec.Opts[1]), Luck: int(rec.Opts[2]), Option: int(rec.Opts[3]), Exc: rec.Opts[4],
			OptionEx: optEx, ToInventory: true})
		w.send(o, packetXShopResult(xshopOK, balance, item))
		w.log.Info("[CustomXShop]", "account", login, "name", o.P.Char.Name, "item", item, "price", price)
		return
	}
	refund()
	w.send(o, packetXShopResult(xshopFailed, balance+price, item))
}

// xshopRefund is DS 0x0F 0x22 (C1 14 0F 22 account price; FUN_00401610
// adds the credits back and logs "[CustomXShop] Refund").
func (w *World) xshopRefund(login string, price uint32) {
	cs := w.creditStore()
	if cs == nil {
		return
	}
	w.async(func(ctx context.Context) func() {
		if err := cs.RefundCredits(ctx, login, int64(price)); err != nil {
			w.log.Error("[CustomXShop] Refund", "account", login, "value", price, "err", err)
		}
		return nil
	})
}

// --- FA 05: GM price ------------------------------------------------------------------

// xshopGMPrice is FUN_00403660: [4] item(u16) [8] price(u32). Only
// Authority 8 (else result 1, "GM-PRICE DENIED"); one change per 300 ms
// (2); the item must exist and the price be at most 2,000,000,000 (2).
// The file is rewritten (FUN_00403920; failure 3) and the override
// changes (0).
func (w *World) xshopGMPrice(o *Object, p []byte) {
	if !playerReady(o) {
		return
	}
	idx := int(p[4]) | int(p[5])<<8
	price := uint32(p[8]) | uint32(p[9])<<8 | uint32(p[10])<<16 | uint32(p[11])<<24
	if !gmFull(o) {
		w.send(o, packetXShopGMResult(1))
		w.log.Info("[CustomXShop][GM-PRICE][DENIED]", "account", o.P.Account.Login, "name", o.P.Char.Name,
			"item", idx, "price", price)
		return
	}
	s, now := cash(o), w.now()
	if !s.PriceAt.IsZero() && now.Sub(s.PriceAt) < xshopGMInterval {
		w.send(o, packetXShopGMResult(2))
		return
	}
	s.PriceAt = now
	if _, ok := w.itemInfo(idx); !ok || price > xshopMaxPrice {
		w.send(o, packetXShopGMResult(2))
		return
	}
	c := w.cw2()
	old := c.prices.Price(idx)
	if c.xshopPath == "" || gamedata.WriteXShopPrice(c.xshopPath, idx, price) != nil {
		w.send(o, packetXShopGMResult(3))
		w.log.Warn("[CustomXShop][GM-PRICE][WRITE-FAIL]", "account", o.P.Account.Login, "name", o.P.Char.Name,
			"item", idx, "old", old, "new", price)
		return
	}
	c.prices.Overrides[idx] = price
	w.send(o, packetXShopGMResult(0))
	w.log.Info("[CustomXShop][GM-PRICE][OK]", "account", o.P.Account.Login, "name", o.P.Char.Name,
		"item", idx, "old", old, "new", price)
}
