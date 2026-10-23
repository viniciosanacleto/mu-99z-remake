package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Item move 24 (FUN_00499410): [3] source flag, [4] source slot,
// [5..11] item (client copy, unused), [12] target flag, [13] target slot.
// Flags: 0 inventory, 1 trade box, 2 warehouse, 3/5..20 chaos boxes,
// 4 personal shop.

const (
	moveInventory = 0
	moveTrade     = 1
	moveWarehouse = 2
	movePShop     = 4
)

func init() {
	worldHandlers[0x24] = func(w *World, o *Object, p []byte) { w.handleItemMove(o, p) }
}

// packetMoveResult is C3 0C 24 result targetSlot item[7] (result = the
// target flag on success, 0xFF on failure with FF bytes).
func packetMoveResult(result byte, slot byte, it *item.Item) []byte {
	p := newPacket(frame.C3, 0x24, int(result), 0x0C)
	p[4] = slot
	if it == nil {
		for i := 5; i < 12; i++ {
			p[i] = 0xFF
		}
		return p
	}
	it.EncodeWire(p[5:12], false)
	return p
}

func (w *World) handleItemMove(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 14 {
		return
	}
	sf, ss, tf, ts := int(p[3]), int(p[4]), int(p[12]), int(p[13])
	st := items(o)
	fail := func() { w.send(o, packetMoveResult(0xFF, byte(ts), nil)) }
	if !o.Alive || o.Teleporting || o.Live != LiveNormal {
		fail()
		return
	}
	if (sf == moveTrade || tf == moveTrade) && !(inTrade(o) && w.tradePartner(o) != nil) {
		fail()
		return
	}
	if (sf == moveWarehouse || tf == moveWarehouse) &&
		!(warehouseOpen(st) && st.WarehouseLoaded && st.WarehouseLock != 1) {
		fail()
		return
	}
	if (sf == movePShop || tf == movePShop) && (pshopOpen(o) || pshop(o).Busy) {
		fail() // obj+0x2468 / obj+0x2469 (pshop.go)
		return
	}
	if (isChaosFlag(sf) || isChaosFlag(tf)) && !chaosMoveAllowed(o, sf, tf) {
		fail() // interface 7 / 13 and no mix running (chaos.go)
		return
	}
	// Permission obj+0x20a5 and BridgeFunction_OnUserItemMove (FUN_004e7720, lua.go).
	if !w.scriptUserItemMove(o, sf, ss, tf, ts) {
		fail()
		return
	}
	res := -1
	switch {
	case sf == moveInventory && isChaosFlag(tf):
		res = w.inventoryToChaos(o, ss, ts, tf)
	case isChaosFlag(sf) && tf == moveInventory:
		res = w.chaosToInventory(o, ss, ts)
	case isChaosFlag(sf) && isChaosFlag(tf):
		res = w.chaosToChaos(o, ss, ts, tf)
	case sf == moveInventory && tf == movePShop:
		res = w.inventoryToPShop(o, ss, ts)
	case sf == movePShop && tf == moveInventory:
		res = w.pshopToInventory(o, ss, ts)
	case sf == movePShop && tf == movePShop:
		res = w.pshopToPShop(o, ss, ts)
	case sf == moveInventory && tf == moveInventory:
		res = w.inventoryToInventory(o, ss, ts)
	case sf == moveInventory && tf == moveTrade:
		res = w.inventoryToTrade(o, ss, ts)
	case sf == moveInventory && tf == moveWarehouse:
		res = w.inventoryToWarehouse(o, ss, ts)
	case sf == moveTrade && tf == moveInventory:
		res = w.tradeToInventory(o, ss, ts)
	case sf == moveTrade && tf == moveTrade:
		res = w.tradeToTrade(o, ss, ts)
	case sf == moveWarehouse && tf == moveInventory:
		res = w.warehouseToInventory(o, ss, ts)
	case sf == moveWarehouse && tf == moveWarehouse:
		res = w.warehouseToWarehouse(o, ss, ts)
	}
	if res < 0 {
		fail()
		if sf == moveInventory && tf == moveInventory && ss < InventorySlots && !o.P.Inventory[ss].IsEmpty() {
			it := o.P.Inventory[ss]
			w.send(o, packetItemDurability(ss, truncDur(it.Durability), 0))
		}
		return
	}
	var it item.Item
	switch tf {
	case moveInventory:
		it = o.P.Inventory[ts]
	case moveTrade:
		it = trade(o).Box[ts]
	case moveWarehouse:
		it = st.Warehouse[ts]
	case movePShop:
		it = o.P.Inventory[ts]
	default:
		if isChaosFlag(tf) {
			it = chaos(o).Box[ts]
		}
	}
	w.send(o, packetMoveResult(byte(res), byte(ts), &it))
	if tf == moveInventory {
		w.broadcast(o, packetCharSetUpdate(o))
		if it.Flag1 {
			w.send(o, packetPeriodicItem(ss, ts, it.Expire)) // FUN_004d3f00
		}
	}
}

// inventoryToInventory is FUN_00497500 (with the stack merge of
// FUN_00495c00).
func (w *World) inventoryToInventory(o *Object, src, dst int) int {
	if src >= BagEnd || dst >= BagEnd || src == dst {
		return -1
	}
	inv := &o.P.Inventory
	it := inv[src]
	if it.IsEmpty() || !w.canEquip(o, it, dst) {
		return -1
	}
	if w.stackMergeMove(o, src, dst) {
		return -1
	}
	inv[src] = item.Empty()
	if w.placeInventory(o, it, dst) < 0 {
		inv[src] = it
		return -1
	}
	if src < EquipSlots && it.Skill {
		w.removeItemSkill(o, it)
	}
	w.equipmentChanged(o, src)
	w.equipmentChanged(o, dst)
	return moveInventory
}

// stackMergeMove is FUN_00495c00: dropping a stack onto a stack of the
// same item merges them (the move itself then fails with 0xFF).
func (w *World) stackMergeMove(o *Object, src, dst int) bool {
	inv := &o.P.Inventory
	a, b := &inv[src], &inv[dst]
	if a.IsEmpty() || b.IsEmpty() || a.Index != b.Index || a.Level != b.Level || a.JoH != b.JoH ||
		(a.OptionEx^b.OptionEx)&0x40 != 0 || w.itemData == nil {
		return false
	}
	max := w.itemData.stack.Max(a.Index, a.Level)
	if max < 1 || b.Durability >= float32(max) {
		return false
	}
	move := float32(int(float32(max) - b.Durability))
	if a.Durability < move {
		move = float32(int(a.Durability))
	}
	a.Durability -= move
	b.Durability += move
	w.send(o, packetItemDurability(dst, truncDur(b.Durability), 0))
	if a.Durability < 1 {
		w.deleteInventory(o, src)
		w.send(o, packetItemDelete(src, 1))
	}
	return true
}
