package game

// CustomStore (CCustomStore, getter FUN_00434850; docs/server-plan/
// 10e-customs.md): /store turns the open personal shop into a
// shop that sells for jewels instead of zen (index 50, FUN_00468d90 ->
// FUN_00469140) and /offstore keeps it open without the client (index 51,
// FUN_00469030). The jewel payment replaces the 3F 06 purchase
// (FUN_004695d0, called by FUN_004d5140 before the zen path).

// storeState is obj+0x249c (custom shop), 0x249d (jewel type), 0x249e
// (offline shop: 1 open, 2 closing) and 0x249f (closing countdown).
type storeState struct {
	Active    bool
	Type      int
	Offline   byte
	Countdown int
}

// storeJewels are the seven jewel kinds of FUN_004695d0 in shop type order
// (JoB, JoS, JoL, JoC, JCr, JoG, JoH): the single jewel and its
// 10/20/30-jewel pack (levels 0, 1, 2).
var storeJewels = [7]struct{ single, pack int }{
	{14*512 + 13, 12*512 + 30},
	{14*512 + 14, 12*512 + 31},
	{14*512 + 16, 12*512 + 136},
	{12*512 + 15, 12*512 + 141},
	{14*512 + 22, 12*512 + 137},
	{14*512 + 31, 12*512 + 138},
	{14*512 + 42, 12*512 + 140},
}

// storeSyntaxKeys are the Custom.dat words of the shop types 0..5.
var storeSyntaxKeys = [6]string{
	"CustomStoreCommandJoBSyntax", "CustomStoreCommandJoSSyntax", "CustomStoreCommandJoLSyntax",
	"CustomStoreCommandJoCSyntax", "CustomStoreCommandJCRSyntax", "CustomStoreCommandJoGSyntax",
}

func registerCustomstore(h *hooks) {
	h.commandHandlers[50] = (*World).commandCustomStore
	h.commandHandlers[51] = (*World).commandOffStore
}

// commandCustomStore is /store [word] (FUN_00468d90): CustomStoreSwitch and no
// interface open; no word closes the shop (FUN_004d4f00), a CustomStore
// syntax word opens the custom shop of that type. Not charged.
func (w *World) commandCustomStore(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomStoreSwitch", 0) == 0 || ifBusy(o) {
		return
	}
	word := commandToken(arg, 0, 0x20)
	if word == "" {
		w.pshopClose(o)
		return
	}
	for t, key := range storeSyntaxKeys {
		if word == w.cfg.String("Custom", key, 0x20) {
			w.customStoreOpen(o, t)
			return
		}
	}
}

// customStoreOpen is FUN_00469140: the map must allow personal shops
// (message 0x326), CustomStoreMapZone (0x23E), a safe zone (0x21D), an
// open shop (0x21C) whose items all have a price (0x21E) and level 6 (3F
// 02 result 2). The shop title becomes Message.txt 0x21F+type and C1 34
// 3F 10 goes to the owner and the viewport.
func (w *World) customStoreOpen(o *Object, typ int) {
	switch {
	case !w.pshopMapAllowed(o.Map):
		w.systemMessage(o, w.messageFor(o, 0x326))
	case !w.mapZone("CustomStoreMapZone", o.Map):
		w.customMsg(o, 0x23E)
	case !w.inSafeZone(o):
		w.customMsg(o, 0x21D)
	case !pshopOpen(o):
		w.customMsg(o, 0x21C)
	case !pshopPriced(o):
		w.customMsg(o, 0x21E)
	case o.P.Char.Level < 6:
		w.send(o, packetPShopOpen(2))
	default:
		ps := pshop(o)
		s := &custom(o).Store
		s.Active, s.Type, s.Offline = true, typ, 0
		var title [pshopTitleLen]byte
		copy(title[:], w.messageFor(o, 0x21F+typ))
		ps.Title = title
		p := packetPShopTitle(o.Index, ps.Title[:], o.P.Char.Name)
		w.send(o, p)
		w.broadcast(o, p)
	}
}

// commandOffStore is /offstore (FUN_00469030): CustomStoreOfflineSwitch,
// no interface, an open shop (0x21C), CustomStoreOfflineMapZone (0x23E),
// a safe zone (0x21D) outside Chaos Castle 18..23 (0x23E). The client is
// closed and the shop stays (obj+0x249e = 1); CustomStoreSkinChange sets
// the skin (obj+0x314). Not charged.
func (w *World) commandOffStore(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomStoreOfflineSwitch", 0) == 0 || ifBusy(o) || detached(o) {
		return
	}
	if !pshopOpen(o) {
		w.customMsg(o, 0x21C)
		return
	}
	if !w.mapZone("CustomStoreOfflineMapZone", o.Map) {
		w.customMsg(o, 0x23E)
		return
	}
	if !w.inSafeZone(o) {
		w.customMsg(o, 0x21D)
		return
	}
	if o.Map >= 18 && o.Map <= 23 {
		w.customMsg(o, 0x23E)
		return
	}
	s := &custom(o).Store
	s.Offline, s.Countdown = 1, 0
	if skin := w.cfg.Int("Custom", "CustomStoreSkinChange", 0); skin != 0 {
		gmOf(o).Skin = skin
	}
	w.reconnectSave(o) // FUN_004e1350 (reconnect.go)
	w.detachClient(o)
	w.log.Info("[CustomStore] Offline store", "account", o.P.Account.Login, "name", o.P.Char.Name)
}

// customStoreClosed is the CustomStore part of FUN_004d4f00 and of a shop
// that sold its last item (FUN_004694d0): the custom flags are cleared
// and an offline shop starts its 5-second close (0x249e = 0x502).
func customStoreClosed(o *Object) {
	if o.P == nil || o.P.cust == nil {
		return
	}
	s := &o.P.cust.Store
	s.Active, s.Type = false, 0
	if s.Offline == 1 {
		s.Offline, s.Countdown = 2, 5
	}
}

// customStoreViewNotice is FUN_00469500 (from 3F 05, FUN_004d4fc0): the
// viewer of a custom shop gets Message.txt 0x229+type as a gold notice.
func (w *World) customStoreViewNotice(viewer, owner *Object) {
	if owner.P == nil || owner.P.cust == nil || !owner.P.cust.Store.Active {
		return
	}
	if t := owner.P.cust.Store.Type; t >= 0 && t <= 9 {
		w.goldMessage(viewer, w.messageFor(viewer, 0x229+t))
	}
}

// jewelPlan is how a buyer pays one jewel kind: singles and 10/20/30
// packs.
type jewelPlan struct{ singles, p10, p20, p30 int }

func (p jewelPlan) zero() bool { return p == jewelPlan{} }

// jewelPayment is one block of FUN_004695d0: the jewels of a kind the
// buyer holds (singles + 10 * (p10 + 2 p20 + 3 p30)) and, when the price
// can be paid exactly, the greedy plan from the 30-packs down to the
// singles (zero plan otherwise).
func (w *World) jewelPayment(o *Object, kind, price int) (total int, plan jewelPlan) {
	j := storeJewels[kind]
	s := w.countItems(o, j.single, 0)
	p10 := w.countItems(o, j.pack, 0)
	p20 := w.countItems(o, j.pack, 1)
	p30 := w.countItems(o, j.pack, 2)
	total = s + (p30*3+p10+p20*2)*10
	rem := price
	if p30 > 0 {
		p30 = min(p30, rem/30)
		rem -= p30 * 30
	}
	if p20 > 0 {
		p20 = min(p20, rem/20)
		rem -= p20 * 20
	}
	if p10 > 0 {
		p10 = min(p10, rem/10)
		rem -= p10 * 10
	}
	if s > 0 {
		s = min(s, rem)
		rem -= s
	}
	if rem == 0 {
		plan = jewelPlan{s, p10, p20, p30}
	}
	return total, plan
}

// jewelReceipt is FUN_004d41b0: the seller gets the price as 30, 20 and
// 10 packs and singles; it needs one bag cell for each.
func jewelReceipt(price int) jewelPlan {
	return jewelPlan{p30: price / 30, p20: price % 30 / 20, p10: price % 30 % 20 / 10, singles: price % 30 % 20 % 10}
}

func (p jewelPlan) cells() int { return p.singles + p.p10 + p.p20 + p.p30 }

// customStoreBuy is FUN_004695d0: a purchase from a custom shop paid in
// the shop's jewel kind. It returns false when the seller's shop is not a
// custom shop, so the zen path of FUN_004d5140 runs. 3F 06 results (CL of
// the FUN_004d5e40 calls): 3 closed, 4 busy, 5 bad slot, 6 transaction /
// name / item / price, 7 jewels missing or no exact payment or no room
// for the seller's jewels, 8 no room for the item, 1 bought.
func (w *World) customStoreBuy(o *Object, p []byte) bool {
	sellerIdx := int(p[4])<<8 | int(p[5])
	t := w.pshopTarget(sellerIdx)
	if t == nil || t == o {
		return false
	}
	fail := func(r byte) bool { w.send(o, packetPShopBuy(r, sellerIdx, o.P.Inventory[0], 0)); return true }
	ts := pshop(t)
	if !ts.Open {
		return fail(3)
	}
	if t.P.cust == nil || !t.P.cust.Store.Active {
		return false
	}
	if ts.Busy {
		return fail(4)
	}
	slot := int(p[16])
	if slot < ShopStart || slot >= InventorySlots {
		return fail(5)
	}
	if chaosTransactionOpen(o) || !nameMatches(p[6:16], t.P.Char.Name) {
		return fail(6)
	}
	it := t.P.Inventory[slot]
	if it.IsEmpty() || it.PShopPrice < 1 {
		return fail(6)
	}
	typ := t.P.cust.Store.Type
	if typ < 0 || typ > 6 {
		return true // types 7..9 (FUN_004695d0 tail): nothing happens
	}
	var price [7]int
	price[typ] = int(it.PShopPrice)
	var plans [7]jewelPlan
	cells := 0
	for k := range storeJewels {
		total, plan := w.jewelPayment(o, k, price[k])
		if price[k] > total || (price[k] > 0 && plan.zero()) {
			return fail(7)
		}
		plans[k] = plan
		cells += jewelReceipt(price[k]).cells()
	}
	if w.freeBagCells(t) < cells {
		return fail(7)
	}
	ts.Busy = true
	bought := it
	dst := w.insertInventory(o, bought)
	if dst < 0 {
		ts.Busy = false
		return fail(8)
	}
	w.log.Info("[SellPesonalShopItem]", "seller", t.P.Char.Name, "buyer", o.P.Char.Name, "type", typ,
		"value", it.PShopPrice, "index", it.Index, "level", it.Level, "serial", it.Serial)
	for k, j := range storeJewels {
		pl := plans[k]
		w.deleteItems(o, j.single, 0, pl.singles)
		w.deleteItems(o, j.pack, 0, pl.p10)
		w.deleteItems(o, j.pack, 1, pl.p20)
		w.deleteItems(o, j.pack, 2, pl.p30)
	}
	w.send(o, packetPShopBuy(1, sellerIdx, o.P.Inventory[dst], byte(dst)))
	w.saveCharacter(o, nil)
	for k, j := range storeJewels {
		r := jewelReceipt(price[k])
		give := func(index, level, n int) {
			for i := 0; i < n; i++ {
				w.createItem(t, 0xEB, 0, 0, dropSpec{Index: index, Level: level, ToInventory: true})
			}
		}
		give(j.single, 0, r.singles)
		give(j.pack, 0, r.p10)
		give(j.pack, 1, r.p20)
		give(j.pack, 2, r.p30)
	}
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
		customStoreClosed(t)
	} else {
		ts.Changed = true
	}
	ts.Busy = false
	return true
}
