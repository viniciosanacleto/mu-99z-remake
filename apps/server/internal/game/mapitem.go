package game

import (
	"context"
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Map items (CMapItem): 300 slots per map (stride 0x104 inside the CMap at
// 0x87d1c8 + map*0x130d8; the next-slot cursor is CMap+0x130d4).

const (
	MaxMapItems = 300
	MoneyIndex  = 0x1c0f // GET_ITEM(14,15) Zen
)

// Map item states (CMapItem+0xe8).
const (
	mapItemNew     = 1
	mapItemNormal  = 2
	mapItemRemoved = 8
)

// mapItem is one CMapItem.
type mapItem struct {
	Item       item.Item // CItem at +4 (Index < 0 = free)
	Money      int64     // CItem+0x7c (+0x80 of the slot) for zen
	X, Y       int       // +0xe4 / +0xe5
	Live       bool      // +0xe6
	State      int       // +0xe8
	Expire     time.Time // +0xec
	Owner      int       // +0xf0
	Dropper    int       // +0xf4 (ItemLoot owner)
	OwnerUntil time.Time // +0xf8
	Created    time.Time // +0xfc
	Looting    bool      // +0x100 (serial request pending)
}

func (m *mapItem) valid() bool { return !m.Item.IsEmpty() }

// mapItemTable is the item part of one CMap.
type mapItemTable struct {
	items [MaxMapItems]mapItem
	next  int
}

func newMapItemTable() *mapItemTable {
	t := &mapItemTable{}
	for i := range t.items {
		t.items[i].Item = item.Empty()
		t.items[i].Owner, t.items[i].Dropper = -1, -1
	}
	return t
}

// mapItems returns the table of a map (allocated on first use).
func (w *World) mapItems(mp int) *mapItemTable {
	if w.itemData == nil || mp < 0 || mp >= gamedata.MaxMaps {
		return nil
	}
	if w.itemData.maps[mp] == nil {
		w.itemData.maps[mp] = newMapItemTable()
	}
	return w.itemData.maps[mp]
}

// Viewport phases called from viewport.go.
const (
	vpItemRemove  = iota // FUN_00514470
	vpItemAdd            // FUN_00514a00
	vpItemDestroy        // FUN_00514e80 (C2 21)
	vpItemCreate         // FUN_00515b00 (C2 20)
)

func registerMapitem(h *hooks) {
	h.itemViewport = func(w *World, o *Object, phase int) {
		switch phase {
		case vpItemRemove:
			w.vpItemRemove(o)
		case vpItemAdd:
			w.vpItemAdd(o)
		case vpItemDestroy:
			w.vpItemDestroy(o)
		case vpItemCreate:
			w.vpItemCreate(o)
			if w.itemData != nil {
				w.itemData.lastVP = w.now()
			}
		}
	}
	h.tick1000 = append(h.tick1000, func(w *World) { w.mapItemProc() })
	h.dropMapItem = func(w *World, mp, x, y int, it item.Item, owner int) bool {
		return w.addMapItem(mp, x, y, it, owner) >= 0
	}
	h.dropMapMoney = func(w *World, mp, x, y int, amount int, owner int) bool {
		return w.addMapMoney(mp, x, y, int64(amount)) >= 0
	}
}

// cellDroppable is the attribute test of FUN_004adab0: neither block (4)
// nor no-ground (8); outside the map counts as blocked.
func (w *World) cellDroppable(mp, x, y int) bool {
	t := w.terrain(mp)
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return false
	}
	a := t.Get(x, y)
	return a&gamedata.AttrBlock == 0 && a&gamedata.AttrNoGround == 0
}

// freeMapSlot scans 300 slots from the cursor (FUN_004adab0 loop).
func (t *mapItemTable) freeSlot() int {
	s := t.next
	for n := 0; n < MaxMapItems; n++ {
		if !t.items[s].valid() {
			return s
		}
		s++
		if s >= MaxMapItems {
			s = 0
		}
	}
	return -1
}

func (t *mapItemTable) advance(s int) {
	t.next = s + 1
	if t.next >= MaxMapItems {
		t.next = 0
	}
}

// addMapItem is FUN_004adab0 + FUN_004ae1c0: owner and the loot owner are
// the dropping object; the owner keeps exclusive rights for
// ItemDropTime*500 ms and the item lives ItemDropTime seconds.
func (w *World) addMapItem(mp, x, y int, it item.Item, owner int) int {
	t := w.mapItems(mp)
	if t == nil || it.IsEmpty() || !w.cellDroppable(mp, x, y) {
		return -1
	}
	s := t.freeSlot()
	if s < 0 {
		return -1
	}
	now := w.now()
	drop := time.Duration(w.cfg.Int("Common", "ItemDropTime", 0))
	t.items[s] = mapItem{Item: it, X: x, Y: y, Live: true, State: mapItemNew,
		Expire: now.Add(drop * time.Second), OwnerUntil: now.Add(drop * 500 * time.Millisecond),
		Created: now, Owner: owner, Dropper: owner}
	t.advance(s)
	return s
}

// addMapMoney is FUN_004ad7d0: zen lives MoneyDropTime seconds and has no
// owner protection (+0xf8 = 0).
func (w *World) addMapMoney(mp, x, y int, amount int64) int {
	t := w.mapItems(mp)
	if t == nil || !w.cellDroppable(mp, x, y) {
		return -1
	}
	s := t.freeSlot()
	if s < 0 {
		return -1
	}
	now := w.now()
	drop := time.Duration(w.cfg.Int("Common", "MoneyDropTime", 0))
	it := item.Empty()
	it.Index = MoneyIndex
	t.items[s] = mapItem{Item: it, Money: amount, X: x, Y: y, Live: true, State: mapItemNew,
		Expire: now.Add(drop * time.Second), Created: now, Owner: -1, Dropper: -1}
	t.advance(s)
	return s
}

// removeMapItem is FUN_004ade60: live = 0, state = 8 (cleared by the next
// map item pass, removed from viewports by FUN_00514470).
func (w *World) removeMapItem(mp, num int) {
	t := w.mapItems(mp)
	if t == nil || num < 0 || num >= MaxMapItems {
		return
	}
	t.items[num].Live = false
	t.items[num].State = mapItemRemoved
}

// mapItemProc is FUN_004adea0 for every map: new -> normal, removed ->
// cleared, expired -> removed.
func (w *World) mapItemProc() {
	if w.itemData == nil {
		return
	}
	now := w.now()
	for _, t := range w.itemData.maps {
		if t == nil {
			continue
		}
		for i := range t.items {
			m := &t.items[i]
			if !m.valid() {
				continue
			}
			switch {
			case m.State == mapItemNew:
				// The original runs this pass and the viewport cycle in
				// separate timers; keep the "new" state until one viewport
				// cycle has sent it (see the final report).
				if !m.Created.After(w.itemData.lastVP) {
					m.State = mapItemNormal
				}
			case m.State == mapItemRemoved:
				*m = mapItem{Item: item.Empty(), Owner: -1, Dropper: -1}
			case now.After(m.Expire):
				m.State = mapItemRemoved
			}
		}
	}
}

// vpItemRemove is FUN_00514470.
func (w *World) vpItemRemove(o *Object) {
	t := w.mapItems(o.Map)
	r := w.viewRange(o.Map)
	for i := range o.VPItem {
		e := &o.VPItem[i]
		if (e.State != VPNew && e.State != VPVisible) || e.Type != 5 || e.Index < 0 || e.Index >= MaxMapItems {
			continue
		}
		if t == nil {
			e.State = VPRemove
			continue
		}
		m := &t.items[e.Index]
		if !m.valid() || m.State == mapItemRemoved || !(o.X >= m.X-r && o.X <= m.X+r && o.Y >= m.Y-r && o.Y <= m.Y+r) {
			e.State = VPRemove
		}
	}
}

// vpItemAdd is FUN_00514a00 (players only).
func (w *World) vpItemAdd(o *Object) {
	if !o.IsPlayer() {
		return
	}
	t := w.mapItems(o.Map)
	if t == nil {
		return
	}
	r := w.viewRange(o.Map)
	for i := range t.items {
		m := &t.items[i]
		if !m.Live || (m.State != mapItemNew && m.State != mapItemNormal) {
			continue
		}
		if m.X-r <= o.X && o.X <= m.X+r && m.Y-r <= o.Y && o.Y <= m.Y+r {
			vpAdd(&o.VPItem, i, 5)
		}
	}
}

// vpItemDestroy is FUN_00514e80: C2 size 21 count + index(BE) per removed
// entry.
func (w *World) vpItemDestroy(o *Object) {
	var nums []int
	for _, e := range o.VPItem {
		if e.State == VPRemove && e.Type == 5 && e.Index >= 0 && e.Index < MaxMapItems {
			nums = append(nums, e.Index)
		}
	}
	if len(nums) > 0 {
		w.send(o, packetMapItemDestroy(nums))
	}
}

// vpItemCreate is FUN_00515b00: C2 size 20 count + 11-byte entries for new
// entries.
func (w *World) vpItemCreate(o *Object) {
	t := w.mapItems(o.Map)
	if t == nil {
		return
	}
	var entries [][]byte
	for _, e := range o.VPItem {
		if e.State != VPNew || e.Type != 5 || e.Index < 0 || e.Index >= MaxMapItems {
			continue
		}
		entries = append(entries, mapItemEntry(e.Index, &t.items[e.Index]))
	}
	if len(entries) > 0 {
		w.send(o, packetMapItems(entries))
	}
}

// mapItemEntry is one C2 20 entry: index BE (bit 7 when the item is new),
// X, Y, item[7]; zen is encoded as 0F m2 m1 00 m0 E0 m3.
func mapItemEntry(num int, m *mapItem) []byte {
	b := make([]byte, 11)
	b[0], b[1] = byte(num>>8), byte(num)
	if m.State == mapItemNew {
		b[0] |= 0x80
	}
	b[2], b[3] = byte(m.X), byte(m.Y)
	if m.Item.Index == MoneyIndex {
		v := uint32(m.Money)
		b[4], b[5], b[6], b[7] = byte(MoneyIndex&0xFF), byte(v>>16), byte(v>>8), 0
		b[8], b[9], b[10] = byte(v), 0xE0, byte(v>>24)
		return b
	}
	m.Item.EncodeWire(b[4:11], false)
	return b
}

// packetMapItems is C2 size 20 count + entries (FUN_00515b00).
func packetMapItems(entries [][]byte) []byte {
	p := newPacket(frame.C2, 0x20, -1, 5+11*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+11*i:], e)
	}
	return p
}

// packetMapItemDestroy is C2 size 21 count + index(BE) (FUN_00514e80).
func packetMapItemDestroy(nums []int) []byte {
	p := newPacket(frame.C2, 0x21, -1, 5+2*len(nums))
	p[4] = byte(len(nums))
	for i, n := range nums {
		p[5+2*i], p[6+2*i] = byte(n>>8), byte(n)
	}
	return p
}

// itemLootAllowed is CItemLoot (FUN_00493390): items listed with
// AllowLoot 0 may only be taken by the object that dropped them.
func (w *World) itemLootAllowed(it item.Item) bool {
	if w.itemData == nil {
		return true
	}
	for _, r := range w.itemData.loot {
		if r.Index == it.Index && (r.Level == -1 || r.Level == it.Level) {
			return r.AllowLoot
		}
	}
	return true
}

// canLoot is FUN_004adcc0. ok=false with msg=true sends message 260.
func (w *World) canLoot(o *Object, num int) (ok bool, msg bool) {
	t := w.mapItems(o.Map)
	if t == nil || num < 0 || num >= MaxMapItems {
		return false, false
	}
	m := &t.items[num]
	if !m.valid() || !m.Live || m.State == mapItemRemoved {
		return false, false
	}
	if dx := m.X - o.X + 2; dx < 0 || dx >= 5 {
		return false, false
	}
	if dy := m.Y - o.Y + 2; dy < 0 || dy >= 5 {
		return false, false
	}
	if w.superDropOn() { // /superdrop (command_hardcoded.go)
		return true, false
	}
	if m.Dropper != o.Index && !w.itemLootAllowed(m.Item) {
		return false, false
	}
	if m.Owner >= 0 && m.Owner < MaxObjects && m.Owner != o.Index && w.now().Before(m.OwnerUntil) {
		// Party members share the rights (obj+0x2f0) unless the item has
		// the CItem+0xb1 flag (event tickets, set by Convert).
		if itemFlagB1(m.Item.Index) || !samePartyHookCall(w, o, w.objects[m.Owner]) {
			return false, true
		}
	}
	return true, false
}

func samePartyHookCall(w *World, a, b *Object) bool {
	return w.hooks.sameParty != nil && w.hooks.sameParty(w, a, b)
}

// itemFlagB1 is CItem+0xb1 (FUN_0048aed0): 14,23..14,26 and 14,65..14,68.
func itemFlagB1(index int) bool {
	switch index {
	case 0x1c17, 0x1c18, 0x1c19, 0x1c1a, 0x1c41, 0x1c42, 0x1c43, 0x1c44:
		return true
	}
	return false
}

func registerMapItemHandlers(h *hooks) {
	h.worldHandlers[0x22] = func(w *World, o *Object, p []byte) { w.handlePickup(o, p) }
	h.worldHandlers[0x23] = func(w *World, o *Object, p []byte) { w.handleDrop(o, p) }
}

// packetPickupResult is C3 10 22 result item[7] + 5 zero bytes
// (FUN_004982e0: 0xFF fail, 0xFD merged into a stack, else the slot).
func packetPickupResult(result byte, it *item.Item) []byte {
	p := newPacket(frame.C3, 0x22, int(result), 0x10)
	if it != nil {
		it.EncodeWire(p[4:11], false)
	}
	return p
}

// handlePickup is FUN_004982e0: 22 [3..4] map item number (BE).
func (w *World) handlePickup(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 5 {
		return
	}
	fail := func() { w.send(o, packetPickupResult(0xFF, nil)) }
	if ifBusyExceptShop(o) {
		fail()
		return
	}
	num := int(p[3])<<8 | int(p[4])
	if num >= MaxMapItems {
		fail()
		return
	}
	if _, ok := w.data.Maps[o.Map]; !ok {
		fail()
		return
	}
	ok, msg := w.canLoot(o, num)
	if !ok {
		if msg {
			w.systemMessage(o, w.messageFor(o, 0x104))
		}
		fail()
		return
	}
	// Permission obj+0x20aa and BridgeFunction_OnUserItemPick (FUN_004e74d0, lua.go).
	if !w.scriptUserItemPick(o, num) {
		fail()
		return
	}
	t := w.mapItems(o.Map)
	m := &t.items[num]
	idx := m.Item.Index
	if (idx == 0x1a14 && m.Item.Level == 0) || idx == 0x1a26 {
		// FUN_004948f0: only one of these per character.
		for _, e := range o.P.Inventory {
			if !e.IsEmpty() && e.Index == idx && e.Level == m.Item.Level {
				fail()
				return
			}
		}
	}
	if idx == MoneyIndex {
		// Solo or split among the party (PartyMoneyDistribute, drop.go).
		amount := m.Money
		w.removeMapItem(o.Map, num)
		w.pickupMoney(o, amount)
		return
	}
	if m.Looting {
		fail()
		return
	}
	it := m.Item
	if w.stackMergeInto(o, &it) {
		w.removeMapItem(o.Map, num)
		w.send(o, packetPickupResult(0xFD, nil))
		return
	}
	m.Item.Durability = it.Durability
	if it.Serial == math.MaxUint32 {
		// The item has no serial yet: FUN_0049ba50 asks the DataServer
		// for one and finishes the pick up in the reply.
		if w.findBagSlot(&o.P.Inventory, it.Index) < 0 {
			fail()
			return
		}
		m.Looting = true
		mp, c := o.Map, o.conn
		w.async(func(ctx context.Context) func() {
			serial, err := w.store.NextItemSerial(ctx)
			return func() {
				t := w.mapItems(mp)
				mi := &t.items[num]
				mi.Looting = false
				o := w.objectOf(c)
				if err != nil || o == nil || o.State != StatePlaying || !mi.valid() || mi.State == mapItemRemoved {
					return
				}
				it := mi.Item
				it.Serial = serial
				w.finishPickup(o, mp, num, it)
			}
		})
		return
	}
	w.finishPickup(o, o.Map, num, it)
}

// finishPickup is the insert part of FUN_004982e0.
func (w *World) finishPickup(o *Object, mp, num int, it item.Item) {
	slot := w.insertInventory(o, it)
	if slot < 0 {
		w.send(o, packetPickupResult(0xFF, nil))
		return
	}
	w.removeMapItem(mp, num)
	w.send(o, packetPickupResult(byte(slot), &it))
	if it.Flag1 {
		w.send(o, packetPeriodicItem(0xFFFF, slot, it.Expire)) // FUN_004d3f00
	}
	for _, h := range w.hooks.itemPicked {
		h(w, o, it)
	}
}

// packetDropResult is C1 05 23 result slot (FUN_004988f0).
func packetDropResult(result byte, slot byte) []byte {
	p := newPacket(frame.C1, 0x23, int(result), 5)
	p[4] = slot
	return p
}

// handleDrop is FUN_004988f0: 23 [3] X [4] Y [5] inventory slot.
func (w *World) handleDrop(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 6 {
		return
	}
	x, y, slot := int(p[3]), int(p[4]), int(p[5])
	fail := func() { w.send(o, packetDropResult(0, byte(slot))) }
	if ifBusyExceptShop(o) || slot >= BagEnd {
		fail()
		return
	}
	it := o.P.Inventory[slot]
	if it.IsEmpty() {
		fail()
		return
	}
	// /superdrop (command_hardcoded.go) skips every restriction below.
	if !w.superDropOn() && it.Flag1 {
		fail()
		return
	}
	if !w.superDropOn() && o.P.Account.Lock > 0 {
		w.systemMessage(o, w.messageFor(o, 0x6f2))
		fail()
		return
	}
	// Permission obj+0x20a9 and BridgeFunction_OnUserItemDrop (FUN_004e75f0,
	// lua.go), both skipped by /superdrop.
	if !w.superDropOn() && !w.scriptUserItemDrop(o, slot, x, y) {
		fail()
		return
	}
	if !w.superDropOn() && !w.itemMoveRule(it.Index).AllowDrop {
		fail()
		return
	}
	// Items +5 or above (categories 0..11), excellent and ancient items
	// cannot be dropped (FUN_0048ad90 / FUN_0048adc0).
	if !w.superDropOn() && ((it.Index < 0x1800 && it.Level > 4) || (it.Excellent != 0 && it.Index != 0x1a03 && it.Index != 0x1a25) ||
		it.SetOption&3 != 0) {
		fail()
		return
	}
	if it.Index == 0x1a14 && it.Level != 0 {
		if it.Level == 1 && o.P.Char.Level < 0x28 {
			w.systemMessage(o, w.messageFor(o, 0x101))
			fail()
			return
		}
		if it.Level == 2 && o.P.Char.Level < 0x50 {
			w.systemMessage(o, w.messageFor(o, 0x102))
			fail()
			return
		}
	}
	// Bags (boxes, rings, medals) and firecrackers open instead of
	// falling (FUN_004988f0 -> FUN_00491340, drop.go).
	consumed, refused := w.dropSpecialItem(o, it, x, y)
	if refused {
		fail()
		return
	}
	if consumed {
		w.deleteInventory(o, slot)
		w.send(o, packetDropResult(1, byte(slot)))
		w.equipmentChanged(o, slot)
		return
	}
	num := w.addMapItem(o.Map, x, y, it, o.Index)
	if num < 0 {
		fail()
		return
	}
	w.deleteInventory(o, slot)
	w.send(o, packetDropResult(1, byte(slot)))
	w.equipmentChanged(o, slot)
	for _, h := range w.hooks.itemDropped {
		h(w, o, it, num) // Blood Castle weapon (FUN_004166b0)
	}
}
