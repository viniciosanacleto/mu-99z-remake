package game

import (
	"encoding/binary"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Personal shop (CPersonalShop, head 3F; docs/server-plan/09c-chaos-loja-
// pets.md §3). The shop items are the inventory slots 76..107 (rows 8..11
// of the occupancy grid); 24 flag 4 moves items there. The price of an
// item is CItem+0x84 (item.Item.PShopPrice): set by 3F 01, kept while the
// item moves, never written to the database (FUN_00496830 does not encode
// it), so it is lost on relog like in the original.

const pshopTitleLen = 36

// pshopState is obj+0x2468.. of the original.
type pshopState struct {
	Open       bool                // obj+0x2468
	Busy       bool                // obj+0x2469: a sale is being processed
	Changed    bool                // obj+0x2470: viewers must refresh (cleared every second)
	ResetKnown bool                // obj+0x2471: forget the shops already announced
	Title      [pshopTitleLen]byte // obj+0x2472
	LastBuy    time.Time           // obj+0x246c (2 s between purchases)
	Viewing    int                 // obj+0x2498 (-1 none); obj+0x2496 is Viewing >= 0
	known      []int               // obj+0x454 / obj+0x580: open shops announced to me
}

func pshop(o *Object) *pshopState {
	if o.P == nil {
		return &pshopState{Viewing: -1}
	}
	if o.P.PShop == nil {
		o.P.PShop = &pshopState{Viewing: -1}
	}
	return o.P.PShop
}

func pshopOpen(o *Object) bool { return o.P != nil && o.P.PShop != nil && o.P.PShop.Open }

func init() {
	worldHandlers[0x3F] = func(w *World, o *Object, p []byte) { w.handlePShop(o, p) }
	viewportEndHooks = append(viewportEndHooks, func(w *World) { w.pshopViewportProc() })
	beforeGateMoveHooks = append(beforeGateMoveHooks, func(w *World, o *Object) {
		if o.P != nil {
			pshop(o).ResetKnown = true // FUN_004bec30
		}
	})
}

// handlePShop dispatches 3F by sub (pc_handlers: 3F01 0x4d4ab0, 3F02
// 0x4d4be0, 3F03 0x4d4f00, 3F05 0x4d4fc0, 3F06 0x4d5140, 3F07 0x4d55b0).
func (w *World) handlePShop(o *Object, p []byte) {
	if len(p) < 4 || !playerReady(o) || o.P.Moving {
		return
	}
	switch p[3] {
	case 0x01:
		w.pshopSetPrice(o, p)
	case 0x02:
		w.pshopOpenReq(o, p)
	case 0x03:
		w.pshopClose(o)
	case 0x05:
		w.pshopView(o, p)
	case 0x06:
		w.pshopBuy(o, p)
	case 0x07:
		ps := pshop(o)
		ps.Viewing = -1
	}
}

// --- packets -------------------------------------------------------------------

// packetPShopPrice is C1 06 3F 01 result slot (FUN_004d5990).
func packetPShopPrice(result, slot byte) []byte {
	p := newPacket(frame.C1, 0x3F, 0x01, 6)
	p[4], p[5] = result, slot
	return p
}

// packetPShopOpen is C1 05 3F 02 result (FUN_004d5a30).
func packetPShopOpen(result byte) []byte {
	p := newPacket(frame.C1, 0x3F, 0x02, 5)
	p[4] = result
	return p
}

// packetPShopClose is C1 07 3F 03 01 index(BE) (FUN_004d5ad0), sent to the
// owner and to VP2.
func packetPShopClose(index int) []byte {
	p := newPacket(frame.C1, 0x3F, 0x03, 7)
	p[4] = 1
	p[5], p[6] = byte(index>>8), byte(index)
	return p
}

// packetPShopList is C2 size 3F sub result index(BE) name[10] title[36]
// count + {slot, item[7], price u32} (FUN_004d5bb0; sub 05 for 3F 05, 13
// for the refresh of a viewer). The items are only listed when result is 1.
func packetPShopList(sub, result byte, index int, name string, title []byte, list []item.Item) []byte {
	var body []byte
	n := 0
	if result == 1 {
		for slot := ShopStart; slot < InventorySlots && slot < len(list); slot++ {
			it := list[slot]
			if it.IsEmpty() {
				continue
			}
			var e [12]byte
			e[0] = byte(slot)
			it.EncodeWire(e[1:8], false)
			binary.LittleEndian.PutUint32(e[8:], uint32(it.PShopPrice))
			body = append(body, e[:]...)
			n++
		}
	}
	p := newPacket(frame.C2, 0x3F, -1, 0x37+len(body))
	p[4], p[5] = sub, result
	p[6], p[7] = byte(index>>8), byte(index)
	if result == 1 {
		putName(p[8:18], name)
		copy(p[18:18+pshopTitleLen], title)
	}
	p[54] = byte(n)
	copy(p[55:], body)
	return p
}

// packetPShopBuy is C1 0F 3F 06 result seller(BE) item[7] slot
// (FUN_004d5e40): the item is the buyer's inventory item at slot (slot 0
// on failures).
func packetPShopBuy(result byte, seller int, it item.Item, slot byte) []byte {
	p := newPacket(frame.C1, 0x3F, 0x06, 0x0F)
	p[4] = result
	p[5], p[6] = byte(seller>>8), byte(seller)
	it.EncodeWire(p[7:14], false)
	p[14] = slot
	return p
}

// packetPShopSold is C1 0F 3F 08 slot buyerName[10] (FUN_004d5f70), to
// the seller.
func packetPShopSold(slot byte, buyer string) []byte {
	p := newPacket(frame.C1, 0x3F, 0x08, 0x0F)
	p[4] = slot
	putName(p[5:15], buyer)
	return p
}

// packetPShopTitle is C1 34 3F 10 index(BE) title[36] name[10]
// (FUN_004d6040), to VP2 when an open shop changes its title.
func packetPShopTitle(index int, title []byte, name string) []byte {
	p := newPacket(frame.C1, 0x3F, 0x10, 0x34)
	p[4], p[5] = byte(index>>8), byte(index)
	copy(p[6:6+pshopTitleLen], title)
	putName(p[42:52], name)
	return p
}

// packetPShopGone is C1 06 3F 12 index(BE) (FUN_004d60c0): the shop being
// viewed closed (index FFFF).
func packetPShopGone(index int) []byte {
	p := newPacket(frame.C1, 0x3F, 0x12, 6)
	p[4], p[5] = byte(index>>8), byte(index)
	return p
}

// packetPShopViewport is C2 size 3F 00 count + {index(BE), title[36]}
// (FUN_004d5600).
func packetPShopViewport(entries []*Object) []byte {
	p := newPacket(frame.C2, 0x3F, 0, 6+len(entries)*0x26)
	p[5] = byte(len(entries))
	for i, t := range entries {
		e := p[6+i*0x26:]
		e[0], e[1] = byte(t.Index>>8), byte(t.Index)
		copy(e[2:2+pshopTitleLen], pshop(t).Title[:])
	}
	return p
}

// --- handlers -------------------------------------------------------------------

// pshopSetPrice is 3F 01 [4] slot [5..8] price (FUN_004d4ab0): 2 bad slot,
// 3 no item, 4 price <= 0, 5 level <= 5, 1 stored.
func (w *World) pshopSetPrice(o *Object, p []byte) {
	if len(p) < 9 {
		return
	}
	slot := int(p[4])
	if slot >= InventorySlots {
		w.send(o, packetPShopPrice(2, p[4]))
		return
	}
	if o.P.Inventory[slot].IsEmpty() {
		w.send(o, packetPShopPrice(3, p[4]))
		return
	}
	price := int32(binary.LittleEndian.Uint32(p[5:]))
	if price <= 0 {
		w.send(o, packetPShopPrice(4, p[4]))
		return
	}
	if o.P.Char.Level <= 5 {
		w.send(o, packetPShopPrice(5, p[4]))
		return
	}
	o.P.Inventory[slot].PShopPrice = price
	w.send(o, packetPShopPrice(1, p[4]))
}

// pshopMapAllowed is FUN_004ae910: MapManager PShopEnable, or
// PersonalShopSwitch when the map has no row or "*".
func (w *World) pshopMapAllowed(mp int) bool {
	if info, ok := w.data.Maps[mp]; ok && info.PShopEnable != -1 {
		return info.PShopEnable == 1
	}
	return w.cfg.Int("Common", "PersonalShopSwitch", 0) != 0
}

// pshopPriced is FUN_004d4100: at least one shop item and every one has a
// price.
func pshopPriced(o *Object) bool {
	n := 0
	for s := ShopStart; s < InventorySlots; s++ {
		it := o.P.Inventory[s]
		if it.IsEmpty() {
			continue
		}
		if it.PShopPrice < 1 {
			return false
		}
		n++
	}
	return n > 0
}

// pshopEmpty is FUN_004d40a0.
func pshopEmpty(o *Object) bool {
	for s := ShopStart; s < InventorySlots; s++ {
		if !o.P.Inventory[s].IsEmpty() {
			return false
		}
	}
	return true
}

// pshopOpenReq is 3F 02 [4..39] title (FUN_004d4be0).
func (w *World) pshopOpenReq(o *Object, p []byte) {
	st := items(o).If
	if st.Use && st.Type != 8 {
		return
	}
	if !w.pshopMapAllowed(o.Map) {
		w.systemMessage(o, w.messageFor(o, 0x326))
		return
	}
	if !pshopPriced(o) {
		w.send(o, packetPShopOpen(0))
		return
	}
	if o.P.Char.Level < 6 {
		w.send(o, packetPShopOpen(2))
		return
	}
	ps := pshop(o)
	var title [pshopTitleLen]byte
	if len(p) > 4 {
		copy(title[:], p[4:])
	}
	ps.Title = title
	if !ps.Open {
		ps.Open = true
	} else {
		w.broadcast(o, packetPShopTitle(o.Index, ps.Title[:], o.P.Char.Name))
	}
	w.send(o, packetPShopOpen(1))
	// The "/command" titles of the WizardTeam CustomStore (FUN_00469140)
	// are not reconstructed.
}

// pshopClose is 3F 03 (FUN_004d4f00).
func (w *World) pshopClose(o *Object) {
	ps := pshop(o)
	if !ps.Open {
		return
	}
	ps.Open = false
	ps.Title = [pshopTitleLen]byte{}
	p := packetPShopClose(o.Index)
	w.send(o, p)
	w.broadcast(o, p)
	customStoreClosed(o) // customstore.go
}

// pshopTarget returns the playing player at index, or nil.
func (w *World) pshopTarget(idx int) *Object {
	if idx < 0 || idx >= MaxObjects {
		return nil
	}
	t := w.objects[idx]
	if t.State != StatePlaying || !t.IsPlayer() || t.P.Char == nil || t.P.LogoutCountdown > 0 || t.P.Moving {
		return nil
	}
	return t
}

// nameMatches compares the 10-byte name of a request with a character
// name (strcmp on the NUL-terminated copy).
func nameMatches(raw []byte, name string) bool {
	n := 0
	for n < len(raw) && raw[n] != 0 {
		n++
	}
	return string(raw[:n]) == name
}

// pshopView is 3F 05 [4..5] index(BE) [6..15] name (FUN_004d4fc0): 3 the
// shop is closed, 2 the name does not match, 1 the list.
func (w *World) pshopView(o *Object, p []byte) {
	if len(p) < 16 {
		return
	}
	t := w.pshopTarget(int(p[4])<<8 | int(p[5]))
	if t == nil {
		return
	}
	if !pshop(t).Open {
		w.send(o, packetPShopList(5, 3, 0xFFFF, "", nil, nil))
		return
	}
	if !nameMatches(p[6:16], t.P.Char.Name) {
		w.send(o, packetPShopList(5, 2, 0xFFFF, "", nil, nil))
		return
	}
	pshop(o).Viewing = t.Index
	w.send(o, packetPShopList(5, 1, t.Index, t.P.Char.Name, pshop(t).Title[:], t.P.Inventory[:]))
	w.customStoreViewNotice(o, t) // FUN_00469500 (customstore.go)
}

// pshopBuy is 3F 06 [4..5] seller(BE) [6..15] name [16] slot
// (FUN_004d5140). Results: 3 shop closed, 4 sale in progress, 5 bad slot,
// 6 transaction / name / no item / no price, 7 not enough zen, 8 seller
// zen overflow or no room, 1 bought.
func (w *World) pshopBuy(o *Object, p []byte) {
	if len(p) < 17 {
		return
	}
	now := w.now()
	ps := pshop(o)
	if now.Sub(ps.LastBuy) < 2*time.Second {
		return
	}
	ps.LastBuy = now
	if w.customStoreBuy(o, p) { // FUN_004695d0 (customstore.go)
		return
	}
	sellerIdx := int(p[4])<<8 | int(p[5])
	t := w.pshopTarget(sellerIdx)
	if t == nil {
		return
	}
	fail := func(r byte) { w.send(o, packetPShopBuy(r, sellerIdx, o.P.Inventory[0], 0)) }
	ts := pshop(t)
	if !ts.Open {
		fail(3)
		return
	}
	if ts.Busy {
		fail(4)
		return
	}
	slot := int(p[16])
	if slot < ShopStart || slot >= InventorySlots {
		fail(5)
		return
	}
	if chaosTransactionOpen(o) {
		fail(6)
		return
	}
	if !nameMatches(p[6:16], t.P.Char.Name) {
		fail(6)
		return
	}
	it := t.P.Inventory[slot]
	if it.IsEmpty() || it.PShopPrice < 1 {
		fail(6)
		return
	}
	price := int64(it.PShopPrice)
	if uint32(o.P.Char.Money) < uint32(price) {
		fail(7)
		return
	}
	if !moneyFits(t.P.Char.Money, price) {
		fail(8)
		return
	}
	bought := it
	bought.PShopPrice = it.PShopPrice
	dst := w.insertInventory(o, bought)
	if dst < 0 {
		fail(8)
		return
	}
	ts.Busy = true
	if w.cfg.Int("Common", "WriteTradeLog", 0) != 0 {
		w.log.Info("[SellPesonalShopItem]", "log", "TRADE_LOG", "seller", t.P.Char.Name, "buyer", o.P.Char.Name,
			"value", price, "index", it.Index, "level", it.Level, "serial", it.Serial)
	}
	o.P.Char.Money -= price
	w.sendMoney(o)
	w.send(o, packetPShopBuy(1, sellerIdx, o.P.Inventory[dst], byte(dst)))
	w.saveCharacter(o, nil)
	t.P.Char.Money += price
	w.sendMoney(t)
	w.send(t, packetPShopSold(byte(slot), o.P.Char.Name))
	w.deleteInventory(t, slot)
	w.send(t, packetItemDelete(slot, 1))
	w.saveCharacter(t, nil)
	if pshopEmpty(t) {
		ts.Open = false
		ts.Title = [pshopTitleLen]byte{}
		cp := packetPShopClose(t.Index)
		w.send(t, cp)
		w.broadcast(t, cp)
		customStoreClosed(t) // FUN_004694d0 (customstore.go)
	} else {
		ts.Changed = true
	}
	ts.Busy = false
}

// pshopViewportProc is the FUN_004d5600 pass of FUN_0050a020 for every
// player, followed by the reset of the "changed" flags.
func (w *World) pshopViewportProc() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.P.Char == nil || o.P.LogoutCountdown > 0 || o.P.Moving {
			continue
		}
		w.pshopViewport(o)
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.P != nil && o.P.PShop != nil {
			o.P.PShop.Changed = false
		}
	}
}

// pshopViewport is FUN_004d5600 for one player: announce the open shops of
// the visible players not announced yet (all of them when the list is
// empty), then refresh or drop the shop being viewed.
func (w *World) pshopViewport(o *Object) {
	ps := pshop(o)
	if ps.ResetKnown {
		ps.known = nil
		ps.ResetKnown = false
	}
	var found []int
	var announce []*Object
	for _, e := range o.VP {
		if e.State != VPVisible || e.Type != byte(TypePlayer) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if !pshopOpen(t) {
			continue
		}
		found = append(found, t.Index)
		known := false
		for _, k := range ps.known {
			if k == t.Index {
				known = true
				break
			}
		}
		if len(ps.known) == 0 || !known {
			announce = append(announce, t)
		}
	}
	if len(announce) > 0 {
		w.send(o, packetPShopViewport(announce))
	}
	ps.known = found
	if ps.Viewing < 0 {
		return
	}
	t := w.pshopTarget(ps.Viewing)
	if t == nil || !pshopOpen(t) {
		ps.Viewing = -1
		w.send(o, packetPShopGone(0xFFFF))
		return
	}
	if pshop(t).Changed {
		w.send(o, packetPShopList(0x13, 1, t.Index, t.P.Char.Name, pshop(t).Title[:], t.P.Inventory[:]))
	}
}

// --- item moves (24 flag 4) ---------------------------------------------------------

// pshopItemAllowed is FUN_00494ee0: a valid item without the lock flag,
// ItemMove AllowTrade, the TradeItemBlock rules (FUN_00495140) and the
// account lock (message 0x6F2).
func (w *World) pshopItemAllowed(o *Object, it item.Item) bool {
	if it.IsEmpty() || it.Flag1 || !w.itemMoveRule(it.Index).AllowTrade {
		return false
	}
	if !w.tradeBlockAllowed(o, it) {
		return false
	}
	return !w.accountLockRefused(o)
}

// tradeBlockAllowed is FUN_00495140 (TradeItemBlock, TradeItemBlockExc,
// TradeItemBlockSet; GMs with Authority 8 are exempt). The extra tests on
// CServerInfo+0x3a8/0x3ac/0x3b0 read keys this build does not load.
func (w *World) tradeBlockAllowed(o *Object, it item.Item) bool {
	if w.cfg.Int("Common", "TradeItemBlock", 0) == 0 || mv(o).Authority == 8 {
		return true
	}
	if n := w.cfg.Int("Common", "TradeItemBlockExc", 0); n != 0 && isExcellent(it) && countExcellent(it.Excellent) >= n {
		return false
	}
	if w.cfg.Int("Common", "TradeItemBlockSet", 0) != 0 && isSet(it) {
		return false
	}
	return true
}

// inventoryToPShop is FUN_00498160: any inventory slot to 76..107; the
// result byte is 0 (the shop is part of the inventory).
func (w *World) inventoryToPShop(o *Object, src, dst int) int {
	if src < 0 || src >= InventorySlots || dst < ShopStart || dst >= InventorySlots {
		return -1
	}
	it := o.P.Inventory[src]
	if !w.pshopItemAllowed(o, it) {
		return -1
	}
	// FUN_004956f0 places the copy while the source still occupies its
	// cells, then FUN_00495880 deletes the source.
	if w.placeInventory(o, it, dst) < 0 {
		return -1
	}
	w.deleteInventory(o, src)
	w.equipmentChanged(o, src)
	return moveInventory
}

// pshopToInventory is FUN_00497a70.
func (w *World) pshopToInventory(o *Object, src, dst int) int {
	if src < ShopStart || src >= InventorySlots || dst < 0 || dst >= BagEnd {
		return -1
	}
	if w.accountLockRefused(o) {
		return -1
	}
	it := o.P.Inventory[src]
	if it.IsEmpty() || !w.canEquip(o, it, dst) {
		return -1
	}
	if w.placeInventory(o, it, dst) < 0 {
		return -1
	}
	w.deleteInventory(o, src)
	w.equipmentChanged(o, dst)
	return moveInventory
}

// pshopToPShop is FUN_00498200.
func (w *World) pshopToPShop(o *Object, src, dst int) int {
	if src < ShopStart || src >= InventorySlots || dst < ShopStart || dst >= InventorySlots || src == dst {
		return -1
	}
	it := o.P.Inventory[src]
	if it.IsEmpty() {
		return -1
	}
	o.P.Inventory[src] = item.Empty()
	if w.placeInventory(o, it, dst) < 0 {
		o.P.Inventory[src] = it
		return -1
	}
	return moveInventory
}
