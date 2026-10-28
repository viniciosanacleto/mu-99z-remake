package game

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Trade (CTrade, FUN_00507cf0..FUN_00508d30). The trade box is 32 CItems
// in an 8x4 grid (obj+0x3f8 / map obj+0x3fc).

const tradeSlots = 32

// tradeState is the per-player trade data.
type tradeState struct {
	Partner int // obj+0x318 (-1 none)
	Box     [tradeSlots]item.Item
	Money   int64 // obj+0x400
	OK      bool  // obj+0x404

	// Inventory backup taken when the trade starts (FUN_0050d300 /
	// FUN_0050d380), restored on cancel (FUN_0050d750).
	backup      [InventorySlots]item.Item
	backupValid bool
}

func trade(o *Object) *tradeState {
	if o.P.Trade == nil {
		o.P.Trade = &tradeState{Partner: -1}
		clearTradeBox(o.P.Trade)
	}
	return o.P.Trade
}

// clearTradeBox is FUN_005078d0.
func clearTradeBox(t *tradeState) {
	for i := range t.Box {
		t.Box[i] = item.Empty()
	}
}

func registerTrade(h *hooks) {
	h.worldHandlers[0x36] = func(w *World, o *Object, p []byte) { w.handleTradeRequest(o, p) }
	h.worldHandlers[0x37] = func(w *World, o *Object, p []byte) { w.handleTradeResponse(o, p) }
	h.worldHandlers[0x3A] = func(w *World, o *Object, p []byte) { w.handleTradeMoney(o, p) }
	h.worldHandlers[0x3C] = func(w *World, o *Object, p []byte) { w.handleTradeOK(o, p) }
	h.worldHandlers[0x3D] = func(w *World, o *Object, p []byte) {
		if playerReady(o) {
			w.tradeCancelBoth(o)
		}
	}
	h.tradeCancel = func(w *World, o *Object) { w.tradeCancelBoth(o) }
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		if o.P == nil || o.P.Trade == nil || o.P.Trade.Partner < 0 {
			return
		}
		w.tradeCancelBoth(o)
		if p := w.tradePartner(o); p != nil {
			w.tradeReset(p)
		}
		w.tradeReset(o)
	})
}

// tradePartner returns the partner object when it is a playing player.
func (w *World) tradePartner(o *Object) *Object {
	t := trade(o)
	if t.Partner < 0 || t.Partner >= MaxObjects {
		return nil
	}
	p := w.objects[t.Partner]
	if !p.IsPlayer() || p.State != StatePlaying || p.P.Char == nil {
		return nil
	}
	return p
}

// inTrade is the "use && type 1 && state != 0" test.
func inTrade(o *Object) bool {
	s := items(o).If
	return s.Use && s.Type == ifTrade && s.State != 0
}

// tradeAllowed is the map rule of FUN_004ae8a0: MapManager TradeEnable
// when set, otherwise TradeSwitch.
func (w *World) tradeAllowed(mp int) bool {
	if info, ok := w.data.Maps[mp]; ok && info.TradeEnable != -1 {
		return info.TradeEnable == 1
	}
	return w.cfg.Int("Common", "TradeSwitch", 0) != 0
}

func near2(a, b *Object) bool {
	return b.X-2 <= a.X && a.X <= b.X+2 && b.Y-2 <= a.Y && a.Y <= b.Y+2
}

// packetTradeRequest is C3 0D 36 name[10] (FUN_005088e0).
func packetTradeRequest(name string) []byte {
	p := newPacket(frame.C3, 0x36, -1, 0x0D)
	putName(p[3:], name)
	return p
}

// packetTradeResponse is C1 14 37 result name[10] level(u16) guild(u32)
// (FUN_005089a0).
func packetTradeResponse(result byte, name string, level int, guild uint32) []byte {
	p := newPacket(frame.C1, 0x37, -1, 0x14)
	p[3] = result
	putName(p[4:], name)
	binary.LittleEndian.PutUint16(p[14:], uint16(level))
	binary.LittleEndian.PutUint32(p[16:], guild)
	return p
}

// packetTradeItemDelete is C1 04 38 slot (FUN_00508a80).
func packetTradeItemDelete(slot int) []byte {
	p := newPacket(frame.C1, 0x38, -1, 4)
	p[3] = byte(slot)
	return p
}

// packetTradeItemAdd is C1 0B 39 slot item[7] (FUN_00508b20).
func packetTradeItemAdd(slot int, it item.Item) []byte {
	p := newPacket(frame.C1, 0x39, -1, 0x0B)
	p[3] = byte(slot)
	it.EncodeWire(p[4:11], false)
	return p
}

// packetTradeMoney is C1 08 3B money (FUN_00508bf0).
func packetTradeMoney(money int64) []byte {
	p := newPacket(frame.C1, 0x3B, -1, 8)
	binary.LittleEndian.PutUint32(p[4:], uint32(money))
	return p
}

// packetTradeOK is C1 04 3C flag (FUN_00508c90).
func packetTradeOK(flag byte) []byte {
	p := newPacket(frame.C1, 0x3C, -1, 4)
	p[3] = flag
	return p
}

// packetTradeResult is C1 04 3D result (FUN_00508d30).
func packetTradeResult(result byte) []byte {
	p := newPacket(frame.C1, 0x3D, -1, 4)
	p[3] = result
	return p
}

func guildNumber(w *World, o *Object) uint32 {
	if w.hooks.guildNumber != nil {
		return w.hooks.guildNumber(w, o)
	}
	return 0
}

// handleTradeRequest is FUN_00507cf0: 36 [3..4] target index (BE).
func (w *World) handleTradeRequest(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 5 {
		return
	}
	ti := int(p[3])<<8 | int(p[4])
	if ti < PlayerStart || ti >= MaxObjects {
		return
	}
	t := w.objects[ti]
	if t == o || !t.IsPlayer() || t.State != StatePlaying || t.P.Char == nil {
		return
	}
	// Russian roulette shot, hide and seek win and the obj+0x24f0 refusal
	// (FUN_004e4e70, FUN_0045fed0 and FUN_00507cf0; customevent.go).
	if w.hooks.tradeRequest != nil && w.hooks.tradeRequest(w, o, t) {
		return
	}
	if items(o).If.Use || items(t).If.Use {
		return
	}
	if !w.tradeAllowed(o.Map) || o.Map != t.Map || !near2(o, t) {
		w.send(o, packetTradeResponse(0, t.P.Char.Name, 0, 0))
		return
	}
	items(o).If = ifState{Use: true, Type: ifTrade}
	items(t).If = ifState{Use: true, Type: ifTrade}
	trade(o).Partner = ti
	trade(t).Partner = o.Index
	w.send(t, packetTradeRequest(o.P.Char.Name))
}

// tradeReset clears the trade part of an object (interface, partner, OK,
// money).
func (w *World) tradeReset(o *Object) {
	st := items(o)
	if st.If.Type == ifTrade {
		st.If = ifState{}
	}
	t := trade(o)
	t.Partner, t.OK, t.Money = -1, false, 0
	t.backupValid = false
	clearTradeBox(t)
}

// handleTradeResponse is FUN_00507f80: 37 [3] accept.
func (w *World) handleTradeResponse(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 4 {
		return
	}
	st := items(o)
	if !(st.If.Use && st.If.Type == ifTrade && st.If.State == 0) {
		return
	}
	pt := w.tradePartner(o)
	if pt == nil {
		w.tradeReset(o)
		return
	}
	ps := items(pt)
	if !(ps.If.Use && ps.If.Type == ifTrade && ps.If.State == 0) {
		return
	}
	if p[3] != 0 && o.Map == pt.Map && w.tradeAllowed(o.Map) && near2(o, pt) {
		for _, x := range []*Object{o, pt} {
			t := trade(x)
			clearTradeBox(t)
			t.backup = x.P.Inventory
			t.backupValid = true
			t.OK, t.Money = false, 0
			items(x).If = ifState{Use: true, State: 1, Type: ifTrade}
		}
		w.send(o, packetTradeResponse(1, pt.P.Char.Name, pt.P.Char.Level, guildNumber(w, pt)))
		w.send(pt, packetTradeResponse(1, o.P.Char.Name, o.P.Char.Level, guildNumber(w, o)))
		return
	}
	w.tradeReset(o)
	w.send(o, packetTradeResponse(0, pt.P.Char.Name, 0, 0))
	w.tradeReset(pt)
	w.send(pt, packetTradeResponse(0, o.P.Char.Name, 0, 0))
}

// handleTradeMoney is FUN_005082d0: 3A [4..7] amount.
func (w *World) handleTradeMoney(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 8 || !inTrade(o) {
		return
	}
	pt := w.tradePartner(o)
	if pt == nil || !inTrade(pt) {
		return
	}
	amount := int64(binary.LittleEndian.Uint32(p[4:]))
	if amount >= 1000000000 || amount > o.P.Char.Money {
		return
	}
	w.send(o, packetMoney(uint32(o.P.Char.Money-amount)))
	p3a := newPacket(frame.C1, 0x3A, -1, 4)
	p3a[3] = 1
	w.send(o, p3a)
	trade(o).OK = false
	w.send(o, packetTradeOK(0))
	trade(pt).OK = false
	w.send(pt, packetTradeOK(2))
	trade(o).Money = amount
	w.send(pt, packetTradeMoney(amount))
}

// handleTradeOK is FUN_00508460: 3C [3] 0/1.
func (w *World) handleTradeOK(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 4 || !inTrade(o) {
		return
	}
	pt := w.tradePartner(o)
	if pt == nil || !inTrade(pt) {
		return
	}
	t, u := trade(o), trade(pt)
	t.OK = p[3] == 1
	w.send(pt, packetTradeOK(b2b(t.OK)))
	if !t.OK || !u.OK {
		return
	}
	if !moneyFits(o.P.Char.Money-t.Money, u.Money) || !moneyFits(pt.P.Char.Money-u.Money, t.Money) {
		w.tradeCancelBoth(o)
		return
	}
	// FUN_00507af0 in both directions: the boxes go straight into the
	// other inventory; any failure cancels, restoring the backups.
	if !w.tradeGive(o, pt) || !w.tradeGive(pt, o) {
		w.tradeCancelBoth(o)
		return
	}
	for _, x := range []*Object{o, pt} {
		other := pt
		if x == pt {
			other = o
		}
		c := x.P.Char
		c.Money = c.Money - trade(x).Money + trade(other).Money
	}
	for _, x := range []*Object{o, pt} {
		w.send(x, packetInventory(x.P.Inventory[:], nil))
		w.rebuildCharSet(x)
		w.sendMoney(x)
		w.saveCharacter(x, nil)
	}
	w.tradeReset(o)
	w.tradeReset(pt)
	w.send(o, packetTradeResult(1))
	w.send(pt, packetTradeResult(1))
}

func b2b(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// tradeGive is FUN_00507af0: insert every box item of from into to's bag.
func (w *World) tradeGive(from, to *Object) bool {
	for _, it := range trade(from).Box {
		if it.IsEmpty() {
			continue
		}
		if w.insertInventory(to, it) < 0 {
			return false
		}
	}
	return true
}

// tradeCancel is FUN_00507a20 for one side: restore the inventory,
// resend it with the money, CharSet and skill list.
func (w *World) tradeCancel(o *Object) {
	if !inTrade(o) {
		return
	}
	t := trade(o)
	if t.backupValid {
		o.P.Inventory = t.backup
	}
	w.send(o, packetInventory(o.P.Inventory[:], nil))
	w.sendMoney(o)
	w.rebuildCharSet(o)
	w.broadcast(o, packetCharSetUpdate(o))
	clearTradeBox(t)
	w.send(o, packetSkills(o.P.Skills[:]))
	w.tradeReset(o)
}

// tradeCancelBoth is FUN_005087f0 (3D): cancel both sides and send
// C1 04 3D 00. A pending request (state 0) is just cleared.
func (w *World) tradeCancelBoth(o *Object) {
	pt := w.tradePartner(o)
	if !inTrade(o) {
		if items(o).If.Use && items(o).If.Type == ifTrade {
			w.tradeReset(o)
			if pt != nil && items(pt).If.Type == ifTrade && !inTrade(pt) {
				w.tradeReset(pt)
			}
		}
		return
	}
	w.tradeCancel(o)
	w.send(o, packetTradeResult(0))
	if pt != nil && inTrade(pt) {
		w.tradeCancel(pt)
		w.send(pt, packetTradeResult(0))
	}
}

// tradeBoxFits places an item in the 8x4 trade grid (FUN_00496000).
func (w *World) tradeBoxFits(t *tradeState, index, slot, skip int) bool {
	wd, ht := w.itemSize(index)
	return gridFits(w, t.Box[:], 4, index, slot, skip, wd, ht)
}

// inventoryToTrade is FUN_00497b80.
func (w *World) inventoryToTrade(o *Object, src, dst int) int {
	if src < 0 || src >= BagEnd || dst < 0 || dst >= tradeSlots {
		return -1
	}
	it := o.P.Inventory[src]
	// FUN_00494ee0: ItemMove AllowTrade, the lock flag and the account
	// lock (message 0x6F2).
	if it.IsEmpty() || it.Flag1 || !w.itemMoveRule(it.Index).AllowTrade {
		return -1
	}
	if o.P.Account.Lock > 0 {
		w.systemMessage(o, w.messageFor(o, 0x6f2))
		return -1
	}
	t := trade(o)
	if !w.tradeBoxFits(t, it.Index, dst, -1) {
		return -1
	}
	t.Box[dst] = it
	w.deleteInventory(o, src)
	if pt := w.tradePartner(o); pt != nil {
		w.send(pt, packetTradeItemAdd(dst, it))
	}
	w.equipmentChanged(o, src)
	return 1
}

// tradeToInventory is FUN_004976c0: resets both OK buttons.
func (w *World) tradeToInventory(o *Object, src, dst int) int {
	if src < 0 || src >= tradeSlots || dst < 0 || dst >= BagEnd {
		return -1
	}
	t := trade(o)
	it := t.Box[src]
	if it.IsEmpty() || !w.canEquip(o, it, dst) || w.placeInventory(o, it, dst) < 0 {
		return -1
	}
	t.Box[src] = item.Empty()
	if pt := w.tradePartner(o); pt != nil {
		t.OK = false
		w.send(o, packetTradeOK(0))
		trade(pt).OK = false
		w.send(pt, packetTradeOK(2))
		w.send(pt, packetTradeItemDelete(src))
	}
	w.equipmentChanged(o, dst)
	return 0
}

// tradeToTrade is FUN_00497ca0.
func (w *World) tradeToTrade(o *Object, src, dst int) int {
	if src < 0 || src >= tradeSlots || dst < 0 || dst >= tradeSlots || src == dst {
		return -1
	}
	t := trade(o)
	it := t.Box[src]
	if it.IsEmpty() || !w.tradeBoxFits(t, it.Index, dst, src) {
		return -1
	}
	t.Box[dst] = it
	t.Box[src] = item.Empty()
	if pt := w.tradePartner(o); pt != nil {
		w.send(pt, packetTradeItemDelete(src))
		w.send(pt, packetTradeItemAdd(dst, it))
	}
	return 1
}
