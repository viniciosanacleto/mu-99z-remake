package game

import (
	"context"
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Warehouse (CWarehouse, FUN_00517390..FUN_00518000). The vault lives in
// the store (DataServer 0x05 load / 0x06 save in the original).

// warehouseNPC is the Safety Guardian class (CNpcTalk case 0xf0,
// FUN_004c24b0).
const warehouseNPC = 0xf0

func registerWarehouse(h *hooks) {
	h.npcTalk[warehouseNPC] = func(w *World, o, npc *Object) bool {
		w.openWarehouse(o)
		return true
	}
	h.worldHandlers[0x81] = func(w *World, o *Object, p []byte) { w.handleWarehouseMoney(o, p) }
	h.worldHandlers[0x82] = func(w *World, o *Object, p []byte) {
		if playerReady(o) {
			w.closeWarehouse(o)
		}
	}
	h.worldHandlers[0x83] = func(w *World, o *Object, p []byte) { w.handleWarehousePassword(o, p) }
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		if o.P != nil && o.P.Items != nil && o.P.Items.WarehouseLoaded {
			w.closeWarehouse(o)
		}
	})
}

// warehouseInUse is the "use && type 6 && state != 0" test.
func warehouseOpen(st *itemsState) bool {
	return st.If.Use && st.If.Type == ifWarehouse && st.If.State != 0
}

// openWarehouse is FUN_004c24b0 + FUN_00517f70 (request) + FUN_00517ba0
// (reply).
func (w *World) openWarehouse(o *Object) {
	st := items(o)
	if st.If.Type == ifChaosBox || st.If.Type == 8 || (st.If.Use && st.If.Type == ifTrade) {
		return
	}
	if st.WarehouseLoaded || st.warehouseLoading {
		return
	}
	st.If = ifState{Use: true, State: 0, Type: ifWarehouse}
	st.warehouseLoading = true
	acc := o.P.Account.ID
	num := st.WarehouseNumber
	c := o.conn
	w.async(func(ctx context.Context) func() {
		wh, err := w.store.LoadWarehouse(ctx, acc, num)
		return func() {
			o := w.objectOf(c)
			if o == nil || o.State != StatePlaying {
				return
			}
			st := items(o)
			st.warehouseLoading = false
			if err != nil {
				w.log.Error("load warehouse", "err", err)
				st.If = ifState{}
				return
			}
			w.warehouseLoaded(o, wh)
		}
	})
}

// warehouseLoaded is FUN_00517ba0.
func (w *World) warehouseLoaded(o *Object, wh *store.Warehouse) {
	st := items(o)
	if !(st.If.Use && st.If.Type == ifWarehouse && st.If.State == 0) || st.WarehouseLoaded {
		return
	}
	for i := range st.Warehouse {
		st.Warehouse[i] = item.Empty()
		if (i+1)*store.ItemDBSize <= len(wh.Items) {
			if it, ok := item.DecodeDB(wh.Items[i*store.ItemDBSize:]); ok {
				st.Warehouse[i] = it
			}
		}
	}
	st.WarehouseMoney = wh.Money
	st.WarehousePass = wh.Password
	st.WarehouseLock = 0
	if wh.Password != 0 {
		st.WarehouseLock = 1
	}
	w.send(o, packetNpcWindow(2))
	w.send(o, packetItemList(st.Warehouse[:]))
	w.send(o, packetWarehouseMoney(1, st.WarehouseMoney, o.P.Char.Money))
	w.send(o, packetWarehouseState(st.WarehouseLock, wareNumberLimit))
	st.If = ifState{Use: true, State: 1, Type: ifWarehouse}
	st.WarehouseLoaded = true
	if w.hooks.warehouseLoaded != nil {
		w.hooks.warehouseLoaded(w, o) // FUN_004758f0(1): pet info of the vault
	}
}

// closeWarehouse is FUN_00517540 (82): save the inventory (DS 0x31) and
// the vault, then clear it and send C1 03 82.
func (w *World) closeWarehouse(o *Object) {
	st := items(o)
	if !warehouseOpen(st) || !st.WarehouseLoaded {
		return
	}
	w.saveInventory(o)
	if w.hooks.warehouseSave != nil {
		w.hooks.warehouseSave(w, o) // FUN_00518000 -> FUN_00476430(1)
	}
	wh := &store.Warehouse{AccountID: o.P.Account.ID, Number: st.WarehouseNumber,
		Items: make([]byte, store.WarehouseSize), Money: st.WarehouseMoney, Password: st.WarehousePass}
	for i, it := range st.Warehouse {
		it.EncodeDB(wh.Items[i*store.ItemDBSize:])
	}
	w.async(func(ctx context.Context) func() {
		if err := w.store.SaveWarehouse(ctx, wh); err != nil {
			w.log.Error("save warehouse", "err", err)
		}
		return nil
	})
	for i := range st.Warehouse {
		st.Warehouse[i] = item.Empty()
	}
	st.If = ifState{}
	st.WarehouseMoney = 0
	st.WarehouseLoaded = false
	w.send(o, packet.New(frame.C1, 0x82, -1, 3))
}

// saveInventory is DataServer 0x31 (FUN_00476230): inventory only.
func (w *World) saveInventory(o *Object) {
	if o.P == nil || o.P.Char == nil {
		return
	}
	inv := make([]byte, store.InventorySize)
	for i, it := range o.P.Inventory {
		it.EncodeDB(inv[i*store.ItemDBSize:])
	}
	id := o.P.Char.ID
	w.async(func(ctx context.Context) func() {
		if err := w.store.SaveInventory(ctx, id, inv); err != nil {
			w.log.Error("save inventory", "err", err)
		}
		return nil
	})
}

// packetWarehouseMoney is C1 0C 81 result whMoney(u32) money(u32)
// (FUN_00517a00 / FUN_005177c0).
func packetWarehouseMoney(result byte, wh, money int64) []byte {
	p := packet.New(frame.C1, 0x81, int(result), 0x0C)
	binary.LittleEndian.PutUint32(p[4:], uint32(wh))
	binary.LittleEndian.PutUint32(p[8:], uint32(money))
	return p
}

// packetWarehouseState is C1 08 83 state CommandWareNumber_AL(u32)
// (FUN_00517ac0).
func packetWarehouseState(state byte, vaults int) []byte {
	p := packet.New(frame.C1, 0x83, int(state), 8)
	binary.LittleEndian.PutUint32(p[4:], uint32(vaults))
	return p
}

// warehouseFee is FUN_00517300: level^2 * 0.1 * 0.4 (+ level*2 when a
// password is set), minimum 1, rounded to 10/100.
func warehouseFee(level int, password bool) int64 {
	v := int64(float64(level*level) * 0.1 * 0.4)
	if password {
		v += int64(level * 2)
	}
	if v < 1 {
		return 1
	}
	if v > 999 {
		return v / 100 * 100
	}
	if v > 99 {
		return v / 10 * 10
	}
	return v
}

// handleWarehouseMoney is FUN_00517390: 81 [3] 0 deposit / 1 withdraw,
// [4..7] amount.
func (w *World) handleWarehouseMoney(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 8 {
		return
	}
	st := items(o)
	if !warehouseOpen(st) {
		return
	}
	c := o.P.Char
	amount := int64(binary.LittleEndian.Uint32(p[4:]))
	fail := func() { w.send(o, packetWarehouseMoney(0, st.WarehouseMoney, c.Money)) }
	switch p[3] {
	case 0:
		if amount == 0 || amount > 2000000000 || st.WarehouseMoney+amount > 2000000000 || c.Money < amount {
			fail()
			return
		}
		c.Money -= amount
		st.WarehouseMoney += amount
	case 1:
		if st.WarehouseLock == 1 || amount == 0 || amount > 2000000000 || st.WarehouseMoney < amount {
			fail()
			return
		}
		st.WarehouseMoney -= amount
		c.Money += amount
		fee := warehouseFee(c.Level, st.WarehousePass != 0)
		if c.Money < fee {
			if st.WarehouseMoney < fee {
				st.WarehouseMoney += amount
				c.Money -= amount
				fail()
				return
			}
			st.WarehouseMoney -= fee
		} else {
			c.Money -= fee
		}
	default:
		return
	}
	st.WarehouseChanges++
	w.send(o, packetWarehouseMoney(1, st.WarehouseMoney, c.Money))
}

// personalCodeOK is FUN_0050cf60 (PersonalCodeCheck, 7 characters of the
// personal id from offset 6).
func (w *World) personalCodeOK(o *Object, code []byte) bool {
	if !w.cfg.PersonalCodeCheck {
		return true
	}
	s := cString(code)
	pid := o.P.Account.PersonalID
	if len(pid) > 6 {
		pid = pid[6:]
	} else {
		pid = ""
	}
	return len(s) >= 7 && len(pid) >= 7 && s[:7] == pid[:7]
}

// handleWarehousePassword is FUN_00517670: 83 [3] type (0 unlock, 1 set,
// 2 remove) [4..5] password [6..] personal code.
func (w *World) handleWarehousePassword(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 6 {
		return
	}
	st := items(o)
	if !warehouseOpen(st) {
		return
	}
	vaults := wareNumberLimit
	reply := func(r byte) { w.send(o, packetWarehouseState(r, vaults)) }
	pass := int(binary.LittleEndian.Uint16(p[4:]))
	var code []byte
	if len(p) > 6 {
		code = p[6:min(len(p), 6+20)]
	}
	switch p[3] {
	case 0:
		if st.WarehouseLock != 0 {
			if st.WarehousePass == pass {
				st.WarehouseLock = 0
				reply(0xc)
			} else {
				reply(0xa)
			}
		}
	case 1:
		if st.WarehouseLock != 0 && st.WarehouseLock != 0xFF {
			reply(0xb)
			return
		}
		if !w.personalCodeOK(o, code) {
			reply(0xd)
			return
		}
		st.WarehousePass = pass
		st.WarehouseLock = 1
		reply(1)
	case 2:
		if !w.personalCodeOK(o, code) {
			reply(0xd)
			return
		}
		st.WarehousePass = 0
		st.WarehouseLock = 0
		reply(0)
	}
}

// Moves between the inventory and the vault (called by the 24 handler).

// warehouseToInventory is FUN_00497830: pays the withdrawal fee.
func (w *World) warehouseToInventory(o *Object, src, dst int) int {
	st := items(o)
	if src < 0 || src >= WarehouseSlots || dst < 0 || dst >= BagEnd {
		return -1
	}
	fee := warehouseFee(o.P.Char.Level, st.WarehousePass != 0)
	c := o.P.Char
	if c.Money < fee && st.WarehouseMoney < fee {
		return -1
	}
	it := st.Warehouse[src]
	if it.IsEmpty() || !w.canEquip(o, it, dst) {
		return -1
	}
	if w.placeInventory(o, it, dst) < 0 {
		return -1
	}
	st.Warehouse[src] = item.Empty()
	if c.Money < fee {
		st.WarehouseMoney -= fee
	} else {
		c.Money -= fee
	}
	w.send(o, packetWarehouseMoney(1, st.WarehouseMoney, c.Money))
	w.equipmentChanged(o, dst)
	return 0
}

// inventoryToWarehouse is FUN_00497dd0 (ItemMove AllowVault, FUN_00495000).
func (w *World) inventoryToWarehouse(o *Object, src, dst int) int {
	st := items(o)
	if src < 0 || src >= BagEnd || dst < 0 || dst >= WarehouseSlots {
		return -1
	}
	it := o.P.Inventory[src]
	if it.IsEmpty() || it.Flag1 || !w.itemMoveRule(it.Index).AllowVault {
		return -1
	}
	if !w.warehouseFits(st, it.Index, dst, -1) {
		return -1
	}
	st.Warehouse[dst] = it
	w.deleteInventory(o, src)
	w.equipmentChanged(o, src)
	return 2
}

// warehouseToWarehouse is FUN_00497ec0.
func (w *World) warehouseToWarehouse(o *Object, src, dst int) int {
	st := items(o)
	if src < 0 || src >= WarehouseSlots || dst < 0 || dst >= WarehouseSlots || src == dst {
		return -1
	}
	it := st.Warehouse[src]
	if it.IsEmpty() || !w.warehouseFits(st, it.Index, dst, src) {
		return -1
	}
	st.Warehouse[dst] = it
	st.Warehouse[src] = item.Empty()
	return 2
}

// warehouseFits is FUN_00496310's placement test on the 8x15 vault grid.
func (w *World) warehouseFits(st *itemsState, index, slot, skip int) bool {
	wd, ht := w.itemSize(index)
	return gridFits(w, st.Warehouse[:], 15, index, slot, skip, wd, ht)
}

// gridFits checks an 8-wide grid of rows rows where list[i] sits at
// (i%8, i/8).
func gridFits(w *World, list []item.Item, rows, index, slot, skip, wd, ht int) bool {
	if slot < 0 || slot >= len(list) || wd <= 0 || ht <= 0 || !list[slot].IsEmpty() && slot != skip {
		return false
	}
	x, y := slot%8, slot/8
	if x+wd > 8 || y+ht > rows {
		return false
	}
	occ := make([]bool, 8*rows)
	for i, it := range list {
		if i == skip || it.IsEmpty() {
			continue
		}
		iw, ih := w.itemSize(it.Index)
		for dy := 0; dy < ih; dy++ {
			for dx := 0; dx < iw; dx++ {
				if xx, yy := i%8+dx, i/8+dy; xx < 8 && yy < rows {
					occ[yy*8+xx] = true
				}
			}
		}
	}
	for dy := 0; dy < ht; dy++ {
		for dx := 0; dx < wd; dx++ {
			if occ[(y+dy)*8+x+dx] {
				return false
			}
		}
	}
	return true
}
