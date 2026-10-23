package game

import (
	"encoding/binary"
	"sync"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Shops (CShopManager FUN_004f5300 / CShop, 120 CItems in an 8x15 grid).

const shopSlots = 120

type shop struct {
	info  gamedata.ShopInfo
	items [shopSlots]item.Item
	count int
}

// loadShops reads ShopManager.txt + Shop/*.txt and registers the NPC talk
// handlers of the shop NPC classes.
func (w *World) loadShops() error {
	list, err := gamedata.LoadShops(w.dataDir)
	if err != nil {
		return err
	}
	for _, info := range list {
		s := &shop{info: info}
		for i := range s.items {
			s.items[i] = item.Empty()
		}
		for _, si := range info.Items {
			w.shopInsert(s, si)
		}
		w.itemData.shops = append(w.itemData.shops, s)
	}
	// npcTalkHooks is package state shared by every instance: register the
	// shop classes once, before any world loop runs.
	shopHooksOnce.Do(func() {
		for _, s := range w.itemData.shops {
			if _, ok := npcTalkHooks[s.info.NPCClass]; !ok {
				npcTalkHooks[s.info.NPCClass] = shopTalk
			}
		}
	})
	return nil
}

var shopHooksOnce sync.Once

// shopInsert is FUN_004f4e10: first free cell (rows 0..14, columns 0..7)
// where the item fits; durability 0 means the maximum.
func (w *World) shopInsert(s *shop, si gamedata.ShopItem) {
	info, ok := w.itemInfo(si.Index)
	if !ok || si.Index < 0 {
		return
	}
	var grid [15][8]bool
	for p, it := range s.items {
		if it.IsEmpty() {
			continue
		}
		iw, ih := w.itemSize(it.Index)
		for dy := 0; dy < ih; dy++ {
			for dx := 0; dx < iw; dx++ {
				if y, x := p/8+dy, p%8+dx; y < 15 && x < 8 {
					grid[y][x] = true
				}
			}
		}
	}
	fits := func(x, y int) bool {
		if x+info.Width > 8 || y+info.Height > 15 {
			return false
		}
		for dy := 0; dy < info.Height; dy++ {
			for dx := 0; dx < info.Width; dx++ {
				if grid[y+dy][x+dx] {
					return false
				}
			}
		}
		return true
	}
	for y := 0; y < 15; y++ {
		for x := 0; x < 8; x++ {
			if grid[y][x] || !fits(x, y) {
				continue
			}
			it := item.Item{Index: si.Index, Level: si.Level, Skill: si.Skill, Luck: si.Luck,
				Option: si.Option, Excellent: byte(si.Excellent), SetOption: byte(si.SetOption)}
			d := si.Durability
			if d == 0 {
				d = maxDurability(info, si.Index, si.Level, si.Excellent != 0, false)
			}
			it.Durability = float32(d)
			s.items[y*8+x] = it
			s.count++
			return
		}
	}
}

// shopFor returns the shop of an NPC: the original stores the
// ShopManager row in obj+0x31c when CShopManager spawns the NPC
// (FUN_004f55e0: class, map, x and y must match).
func (w *World) shopFor(npc *Object) *shop {
	if w.itemData == nil {
		return nil
	}
	for _, s := range w.itemData.shops {
		if s.info.NPCClass == npc.Class && s.info.Map == npc.Map && s.info.X == npc.X && s.info.Y == npc.Y {
			return s
		}
	}
	for _, s := range w.itemData.shops {
		if s.info.NPCClass == npc.Class && s.info.Map == npc.Map {
			return s
		}
	}
	return nil
}

func (w *World) shopByNumber(n int) *shop {
	if w.itemData == nil || n < 0 || n >= len(w.itemData.shops) {
		return nil
	}
	return w.itemData.shops[n]
}

// shopTalk is the shop part of FUN_004c2eb0 (after the NPC script found
// nothing to do).
func shopTalk(w *World, o, npc *Object) bool {
	s := w.shopFor(npc)
	if s == nil {
		return false
	}
	st := items(o)
	if st.If.Use {
		return true
	}
	msg := 0
	switch {
	case w.cfg.Int("Common", "PKLimitFree", 0) == 0 && o.PKLevel > 4 && w.cfg.Int("Common", "PKLimitShop", 0) != 0:
		msg = 0xb8
	case s.count == 0:
		msg = 0xb4
	case s.info.GMLevel != -1 && o.P.Account.Level < s.info.GMLevel:
		// FUN_004f5a70 compares CGameMaster's level; the account level
		// stands in for it until the GM module exists.
		msg = 0xb5
	case !s.info.AL[min(max(o.P.Account.Level, 0), 3)]:
		msg = 0xb6
	}
	if msg != 0 {
		w.systemMessage(o, w.messageFor(o, msg))
		return true
	}
	st.If = ifState{Use: true, State: 0, Type: ifShop}
	st.ShopNumber = s.info.Number
	w.send(o, packetNpcWindow(0))
	w.send(o, packetItemList(s.items[:]))
	w.send(o, packetTaxInfo(2, byte(w.castleStoreRate(o)))) // FUN_00429ad0 + FUN_004dae50(.., 2)
	return true
}

// packetNpcWindow is C3 0B 30 type + 7 zero bytes (FUN_004c2eb0: 0 = shop,
// FUN_00517ba0: 2 = warehouse).
func packetNpcWindow(typ byte) []byte {
	p := newPacket(frame.C3, 0x30, int(typ), 0x0B)
	return p
}

// packetItemList is C2 size 31 00 count + {slot, item[7]} (FUN_004f50b0
// for shops, FUN_005177c0 for the warehouse).
func packetItemList(list []item.Item) []byte {
	var body []byte
	n := 0
	for slot, it := range list {
		if it.IsEmpty() {
			continue
		}
		var e [8]byte
		e[0] = byte(slot)
		it.EncodeWire(e[1:], false)
		body = append(body, e[:]...)
		n++
	}
	p := newPacket(frame.C2, 0x31, -1, 6+len(body))
	p[4] = 0
	p[5] = byte(n)
	copy(p[6:], body)
	return p
}

// packetBuyResult is C1 0B 32 result item[7] (FUN_0049a910).
func packetBuyResult(result byte, it *item.Item) []byte {
	p := newPacket(frame.C1, 0x32, int(result), 0x0B)
	if it != nil {
		it.EncodeWire(p[4:11], false)
	}
	return p
}

// packetSellResult is C1 08 33 result money(u32) (FUN_0049adb0).
func packetSellResult(result byte, money uint32) []byte {
	p := newPacket(frame.C1, 0x33, int(result), 8)
	binary.LittleEndian.PutUint32(p[4:], money)
	return p
}

// packetRepairResult is C1 08 34 money(u32) (FUN_0049b080).
func packetRepairResult(money uint32) []byte {
	p := newPacket(frame.C1, 0x34, -1, 8)
	binary.LittleEndian.PutUint32(p[4:], money)
	return p
}

func init() {
	worldHandlers[0x32] = func(w *World, o *Object, p []byte) { w.handleBuy(o, p) }
	worldHandlers[0x33] = func(w *World, o *Object, p []byte) { w.handleSell(o, p) }
	worldHandlers[0x34] = func(w *World, o *Object, p []byte) { w.handleRepair(o, p) }
}

// playerReady is the common entry test of the item handlers: playing, no
// logout countdown (obj+0xa).
func playerReady(o *Object) bool {
	return o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && o.P.LogoutCountdown <= 0
}

// handleBuy is FUN_0049a910: 32 [3] shop position.
func (w *World) handleBuy(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 4 {
		return
	}
	st := items(o)
	fail := func() { w.send(o, packetBuyResult(0xFF, nil)) }
	s := w.shopByNumber(st.ShopNumber)
	if !st.If.Use || st.If.Type != ifShop || s == nil {
		fail()
		return
	}
	pos := int(p[3])
	if pos >= shopSlots || s.items[pos].IsEmpty() {
		fail()
		return
	}
	it := s.items[pos]
	price := w.itemPrice(it).Buy
	tax := w.castleStoreTax(o, price) // FUN_00429ad0 rate * price / 100 (castlesiege_tax.go)
	c := o.P.Char
	if c.Money < price+tax {
		fail()
		return
	}
	if it.Index >= 0x1a47 && it.Index <= 0x1a4b {
		// FUN_0047dc30 (special purchase, not reconstructed).
		fail()
		return
	}
	merged := it
	if w.stackMergeInto(o, &merged) {
		c.Money -= price + tax
		w.sendMoney(o)
		fail()
		return
	}
	slot := w.insertInventory(o, merged)
	if slot < 0 {
		fail()
		return
	}
	c.Money -= price + tax
	w.sendMoney(o)
	w.castleAddTax(tax)
	w.send(o, packetBuyResult(byte(slot), &merged))
}

// handleSell is FUN_0049adb0: 33 [3] inventory slot.
func (w *World) handleSell(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 4 {
		return
	}
	st := items(o)
	fail := func() { w.send(o, packetSellResult(0, 0)) }
	if !st.If.Use || st.If.Type != ifShop || st.ShopNumber < 0 || st.ShopNumber > 99 {
		fail()
		return
	}
	slot := int(p[3])
	if slot >= BagEnd {
		fail()
		return
	}
	it := o.P.Inventory[slot]
	if it.IsEmpty() || it.Flag1 || !w.itemMoveRule(it.Index).AllowSell {
		fail()
		return
	}
	if o.P.Account.Lock > 0 {
		w.systemMessage(o, w.messageFor(o, 0x6f2))
		fail()
		return
	}
	if w.cfg.Int("Common", "BlockSellItemExc", 0) != 0 && it.Excellent >= 0x10 && it.Level >= 10 {
		w.systemMessage(o, "Dont Sell Item Reforced")
		fail()
		return
	}
	addMoney(o, w.itemPrice(it).Sell)
	w.deleteInventory(o, slot)
	w.send(o, packetSellResult(1, uint32(o.P.Char.Money)))
	w.equipmentChanged(o, slot)
}

// repairable is the item filter of FUN_00497260.
func (w *World) repairable(o *Object, it item.Item) bool {
	if it.IsEmpty() || it.Flag1 {
		return false
	}
	info, ok := w.itemInfo(it.Index)
	if !ok || info.Slot < 0 || info.Slot >= 12 || it.Index == 0x29 {
		return false
	}
	idx := it.Index
	if info.Slot == 8 && idx != 0x1a04 {
		return false
	}
	t := items(o).If
	switch {
	case idx == 0x807 || idx == 0x80f || (idx >= 0x1882 && idx <= 0x1887):
		return false
	case (idx == 0x1a04 || idx == 0x1a05) && t.Type != ifRepair:
		return false
	case idx == 0x1a14 && it.Level >= 0 && it.Level <= 2:
		return false
	case idx == 0x1a0a || (idx >= 0x1a26 && idx <= 0x1a2a) || idx == 0x1a44 || idx == 0x1a4c || idx == 0x1a7a:
		return false
	case t.Type == ifTrade:
		return false
	}
	return true
}

// repairItem is FUN_00497260: pay and restore the durability.
func (w *World) repairItem(o *Object, slot int) bool {
	it := &o.P.Inventory[slot]
	if !w.repairable(o, *it) {
		return false
	}
	cost := w.repairCost(*it)
	if cost < 0 {
		cost = 0
	}
	c := o.P.Char
	if cost > c.Money {
		return false
	}
	c.Money -= cost
	it.Durability = float32(w.itemMaxDurability(*it))
	w.send(o, packetItemDurability(slot, truncDur(it.Durability), 0))
	if c.Money == 0 {
		w.sendMoney(o)
	}
	return true
}

// handleRepair is FUN_0049b080: 34 [3] slot (0xFF = all) [4] 1 = repair
// by the player (level 50+).
func (w *World) handleRepair(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 5 {
		return
	}
	if p[4] == 1 && o.P.Char.Level < 50 {
		w.send(o, packetRepairResult(0))
		return
	}
	if p[3] == 0xFF {
		for s := 0; s < BagEnd; s++ {
			if w.repairItem(o, s) {
				w.send(o, packetRepairResult(uint32(o.P.Char.Money)))
			}
		}
		if recalcHook != nil {
			recalcHook(w, o)
		}
		return
	}
	slot := int(p[3])
	if slot >= BagEnd {
		w.send(o, packetRepairResult(0))
		return
	}
	if w.repairItem(o, slot) {
		w.send(o, packetRepairResult(uint32(o.P.Char.Money)))
		if recalcHook != nil {
			recalcHook(w, o)
		}
	}
}

// closeNpcWindow is FUN_004c3170 (31): close the open interface.
func (w *World) closeNpcWindow(o *Object) {
	st := items(o)
	if !st.If.Use || st.If.Type == 0 {
		return
	}
	switch st.If.Type {
	case ifTrade:
		if tradeCancelHook != nil {
			tradeCancelHook(w, o)
		}
		st.If = ifState{}
	case ifShop:
		st.If = ifState{}
		st.ShopNumber = -1
	case ifWarehouse:
		w.closeWarehouse(o)
		st.If = ifState{}
	case ifChaosBox:
		// FUN_004c3170 only closes type 7 when obj+0x984 is set (never by
		// the chaos module); the client closes the box with 87.
	case ifChaosBox2:
		// Type 13: clear the box and commit (FUN_0042a910 + FUN_0050d620).
		w.chaosClear(o)
		w.chaosCommit(o)
		st.If = ifState{}
	default:
		st.If = ifState{}
	}
}

// tradeCancelHook is set by trade.go.
var tradeCancelHook func(w *World, o *Object)

func init() {
	// 31 is dispatched by npc.go (FUN_004c3170) through npcCloseHook.
	npcCloseHook = func(w *World, o *Object) { w.closeNpcWindow(o) }
}

// spawnNPCHook creates an NPC of class at (mp, x, y, dir) and returns it
// (monsters module). CShopManager spawns the shop NPCs itself
// (FUN_004f55e0): they are not in Data/Monster/Spawn.
var spawnNPCHook func(w *World, class, mp, x, y, dir int) *Object

func init() {
	tick1000Hooks = append(tick1000Hooks, func(w *World) {
		d := w.itemData
		if d == nil || d.shopsSpawned || spawnNPCHook == nil {
			return
		}
		d.shopsSpawned = true
		for _, s := range d.shops {
			if s.info.NPCClass == 0x1ec || w.data.MapServer.ServerForMap(w.cfg.ServerCode, s.info.Map) != w.cfg.ServerCode {
				continue
			}
			spawnNPCHook(w, s.info.NPCClass, s.info.Map, s.info.X, s.info.Y, s.info.Dir)
		}
	})
}
