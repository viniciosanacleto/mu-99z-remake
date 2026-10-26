package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Inventory model of CInventory (obj+0x3e0, 108 CItems of 0xe0 bytes) and
// its occupancy map (obj+0x3ec, 8 bytes per row):
//
//	0..11    equipment
//	12..75   8x8 bag (rows 0..7)
//	76..107  extended area (rows 8..11), only reachable as a move target
//	         when obj+0x60c (expanded inventory count) >= 4 — never in this
//	         build's move paths, which require slot < 76 (FUN_00497500)
//
// The occupancy is derived from the items instead of a separate byte map.

// Interface types of obj+0x3d4 (bits 6..15) used by the item module.
const (
	ifNone      = 0
	ifTrade     = 1
	ifShop      = 3
	ifWarehouse = 6
	ifChaosBox  = 7
	ifRepair    = 13
)

// ifState is obj+0x3d4: use (bits 0..1), state (bits 2..5), type (6..15).
type ifState struct {
	Use   bool
	State int
	Type  int
}

// itemsState is the per-player state of the item module.
type itemsState struct {
	If ifState

	ShopNumber int // obj+0x31a (-1 = none)

	// Warehouse (CWarehouse): obj+0x408 items, 0x418 money, 0x412
	// password, 0x414 lock, 0x588 loaded, 0x410 change counter.
	Warehouse        [WarehouseSlots]item.Item
	WarehouseMoney   int64
	WarehousePass    int
	WarehouseLock    byte
	WarehouseLoaded  bool
	WarehouseNumber  int
	WarehouseChanges int
	warehouseLoading bool

	LastPotion time.Time // obj+0x6c0
}

// WarehouseSlots is the CWarehouse capacity (0x6900 / 0xe0).
const WarehouseSlots = 120

// itemData holds the tables of the item module (loaded by startHooks).
type itemData struct {
	stack  *gamedata.ItemStack
	values []gamedata.ItemValueRule
	move   map[int]gamedata.ItemMoveRule
	loot   []gamedata.ItemLootRule
	shops  []*shop
	maps   [gamedata.MaxMaps]*mapItemTable
	lastVP time.Time

	shopsSpawned bool
}

func init() {
	startHooks = append(startHooks, loadItemData)
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) { items(o) })
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) {
		// Runs after the trade/warehouse hooks of the same list only when
		// registered later; reset the window state of the session.
		if o.P != nil && o.P.Items != nil && !o.P.Items.WarehouseLoaded {
			o.P.Items.If = ifState{}
			o.P.Items.ShopNumber = -1
		}
	})
}

func loadItemData(w *World) error {
	d := &itemData{}
	var err error
	if d.stack, err = gamedata.LoadItemStack(w.configDir); err != nil {
		return err
	}
	if d.values, err = gamedata.LoadItemValue(w.configDir); err != nil {
		return err
	}
	if d.move, err = gamedata.LoadItemMove(w.configDir); err != nil {
		return err
	}
	if d.loot, err = gamedata.LoadItemLoot(w.configDir); err != nil {
		return err
	}
	w.itemData = d
	if err := w.loadShops(); err != nil {
		return err
	}
	return nil
}

// items returns (creating) the item state of a player.
func items(o *Object) *itemsState {
	if o.P == nil {
		return &itemsState{ShopNumber: -1}
	}
	if o.P.Items == nil {
		s := &itemsState{ShopNumber: -1}
		for i := range s.Warehouse {
			s.Warehouse[i] = item.Empty()
		}
		o.P.Items = s
	}
	return o.P.Items
}

// ifBusy reports obj+0x3d4 & 3 != 0 with a type other than the shop (the
// "(use && type != 3)" test shared by pick up, drop and use).
func ifBusyExceptShop(o *Object) bool {
	s := items(o).If
	return s.Use && s.Type != ifShop
}

// itemMoveRule returns the ItemMove.txt row of an index (allowed when absent).
func (w *World) itemMoveRule(index int) gamedata.ItemMoveRule {
	if w.itemData != nil {
		if r, ok := w.itemData.move[index]; ok {
			return r
		}
	}
	return gamedata.ItemMoveRule{AllowDrop: true, AllowSell: true, AllowTrade: true, AllowVault: true}
}

// itemSize returns Width/Height from Item.txt (0,0 when unknown).
func (w *World) itemSize(index int) (int, int) {
	info, ok := w.itemInfo(index)
	if !ok {
		return 0, 0
	}
	return info.Width, info.Height
}

// gridRows is the number of occupancy rows (bag 8 + extended 4).
const gridRows = 12

// slotXY converts a slot >= 12 to (x, y) in the occupancy grid.
func slotXY(slot int) (int, int) { return (slot - BagStart) % 8, (slot - BagStart) / 8 }

// gridLimit is the last row (exclusive) an item at slot may reach:
// 8 in the bag, 12 in the extended area (FUN_004953b0 bounds).
func gridLimit(slot int) int {
	if slot < BagEnd {
		return 8
	}
	return gridRows
}

// occupancy is the obj+0x3ec map rebuilt from the items, skipping one slot.
func (w *World) occupancy(inv *[InventorySlots]item.Item, skip int) [gridRows][8]bool {
	var g [gridRows][8]bool
	for s := BagStart; s < InventorySlots; s++ {
		if s == skip || inv[s].IsEmpty() {
			continue
		}
		wd, ht := w.itemSize(inv[s].Index)
		x, y := slotXY(s)
		if x+wd > 8 || y+ht > gridLimit(s) {
			continue // FUN_004953b0 marks nothing when it does not fit
		}
		for dy := 0; dy < ht; dy++ {
			for dx := 0; dx < wd; dx++ {
				g[y+dy][x+dx] = true
			}
		}
	}
	return g
}

// fitsAt is FUN_00495550: the w*h rectangle at slot is inside its area and
// free.
func fitsAt(g *[gridRows][8]bool, slot, wd, ht int) bool {
	if slot < BagStart || slot >= InventorySlots || wd <= 0 || ht <= 0 {
		return false
	}
	x, y := slotXY(slot)
	if x+wd > 8 || y+ht > gridLimit(slot) {
		return false
	}
	for dy := 0; dy < ht; dy++ {
		for dx := 0; dx < wd; dx++ {
			if g[y+dy][x+dx] {
				return false
			}
		}
	}
	return true
}

// findBagSlot is the search of FUN_00495610 / FUN_004952a0: rows 0..7,
// columns 0..7, first free cell where the item fits. Returns -1.
func (w *World) findBagSlot(inv *[InventorySlots]item.Item, index int) int {
	wd, ht := w.itemSize(index)
	g := w.occupancy(inv, -1)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			s := BagStart + y*8 + x
			if !g[y][x] && fitsAt(&g, s, wd, ht) {
				return s
			}
		}
	}
	return -1
}

// insertInventory is FUN_00495610: put the item in the first free bag
// position. Returns the slot or -1.
func (w *World) insertInventory(o *Object, it item.Item) int {
	s := w.findBagSlot(&o.P.Inventory, it.Index)
	if s >= 0 {
		o.P.Inventory[s] = it
	}
	return s
}

// classAllowed is FUN_00494b50: the Item.txt class column for the
// character class (classes >= 5 are never allowed).
func (w *World) classAllowed(o *Object, index int) bool {
	info, ok := w.itemInfo(index)
	if !ok || o.Class < 0 || o.Class >= 5 {
		return false
	}
	return info.Class[o.Class] != 0
}

// statTotals returns the base + bonus stats (obj+0xfc.. + obj+0x174..).
// The bonus part belongs to the stats module and is zero until it exists.
func statTotals(o *Object) (str, dex, vit, ene, lead int) {
	c := o.P.Char
	return c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership
}

// canEquip is FUN_00494c00: slot < 0 or > 11 is always accepted.
func (w *World) canEquip(o *Object, it item.Item, slot int) bool {
	if it.IsEmpty() {
		return false
	}
	if slot < 0 || slot > 11 {
		return true
	}
	r := w.requirements(it)
	str, dex, vit, ene, lead := statTotals(o)
	if r.Level > o.P.Char.Level || r.Str-int(joHStrReduce(it)) > str || r.Dex-int(joHDexReduce(it)) > dex ||
		r.Vit > vit || r.Ene > ene || r.Lead > lead || !w.classAllowed(o, it.Index) {
		return false
	}
	info, _ := w.itemInfo(it.Index)
	is := info.Slot
	weapon := func(s int) bool { return s == 0 || s == 1 }
	ring := func(s int) bool { return s == 10 || s == 11 }
	if is != slot && !(weapon(slot) && weapon(is)) && !(ring(slot) && ring(is)) {
		return false
	}
	inv := &o.P.Inventory
	twoHand := infoTwoHand(info)
	switch slot {
	case 0:
		if other := inv[1]; !other.IsEmpty() {
			oi, _ := w.itemInfo(other.Index)
			if twoHand && other.Index != 0x807 {
				return false
			}
			if infoTwoHand(oi) && it.Index != 0x80f {
				return false
			}
		}
	case 1:
		if other := inv[0]; !other.IsEmpty() {
			oi, _ := w.itemInfo(other.Index)
			if (twoHand && other.Index != 0x80f) || (infoTwoHand(oi) && it.Index != 0x807) {
				return false
			}
		}
	}
	idx := it.Index
	mp := o.Map
	if mp == 7 && (idx == 0x1a02 || idx == 0x1a03) {
		return false
	}
	if mp >= 0x12 && mp <= 0x17 && (idx == 0x1a0a || (idx >= 0x1a27 && idx <= 0x1a2a) || idx == 0x1a44 ||
		idx == 0x1a4c || idx == 0x1a7a) {
		return false
	}
	if idx == 0x1a0a || (idx == 0x1a14 && it.Level == 0) || (idx > 0x1a25 && idx < 0x1a2b) || idx == 0x1a44 ||
		idx == 0x1a4c || idx == 0x1a6b || idx == 0x1a7a {
		if other := inv[slot^1]; !other.IsEmpty() && other.Index == idx {
			return false
		}
	}
	return true
}

// joHStrReduce / joHDexReduce are CItem+0xce/+0xd0 (Jewel of Harmony
// requirement reductions). CJewelOfHarmonyOption is not reconstructed yet.
func joHStrReduce(item.Item) int { return 0 }
func joHDexReduce(item.Item) int { return 0 }

// placeInventory is FUN_004956f0: put it at a specific slot (equipment
// needs the class check, the bag needs free cells). Returns the slot or -1.
func (w *World) placeInventory(o *Object, it item.Item, slot int) int {
	if slot < 0 || slot >= InventorySlots || !o.P.Inventory[slot].IsEmpty() || it.IsEmpty() {
		return -1
	}
	if slot < EquipSlots {
		if !w.classAllowed(o, it.Index) {
			return -1
		}
		o.P.Inventory[slot] = it
		if it.Skill {
			w.addItemSkill(o, it)
		}
		return slot
	}
	wd, ht := w.itemSize(it.Index)
	g := w.occupancy(&o.P.Inventory, -1)
	if !fitsAt(&g, slot, wd, ht) {
		return -1
	}
	o.P.Inventory[slot] = it
	return slot
}

// deleteInventory is FUN_00495880: clears a slot, removing the weapon
// skill of an equipped item with the skill option.
func (w *World) deleteInventory(o *Object, slot int) {
	if slot < 0 || slot >= InventorySlots {
		return
	}
	it := o.P.Inventory[slot]
	o.P.Inventory[slot] = item.Empty()
	if slot < EquipSlots && !it.IsEmpty() && it.Skill {
		w.removeItemSkill(o, it)
	}
}

// itemSkill is CItem+0x2a (the Item.txt Skill column).
func (w *World) itemSkill(index int) int {
	info, ok := w.itemInfo(index)
	if !ok {
		return 0
	}
	return info.Skill
}

// addItemSkill is FUN_004f8900: learn the weapon skill and send F3 11 FE.
func (w *World) addItemSkill(o *Object, it item.Item) {
	sk := w.itemSkill(it.Index)
	if sk <= 0 {
		return
	}
	if slot := w.addSkill(o, sk, 0); slot >= 0 {
		w.send(o, packetSkillChange(0xFE, slot, o.P.Skills[slot]))
	}
}

// removeItemSkill is FUN_004f89a0: forget the skill when no other equipped
// item with the skill option grants it (the item being removed is already
// out of the inventory here, so the count must be 0).
func (w *World) removeItemSkill(o *Object, it item.Item) {
	sk := w.itemSkill(it.Index)
	if sk <= 0 {
		return
	}
	for s := 0; s < EquipSlots; s++ {
		e := o.P.Inventory[s]
		if !e.IsEmpty() && e.Skill && w.itemSkill(e.Index) == sk {
			return
		}
	}
	for i, s := range o.P.Skills {
		if s.Index == sk {
			o.P.Skills[i] = Skill{Index: -1}
			w.send(o, packetSkillChange(0xFF, i, s))
			return
		}
	}
}

// equipmentChanged is FUN_00496de0: for an equipment slot, recompute the
// character, rebuild the CharSet and broadcast C1 0C 25.
func (w *World) equipmentChanged(o *Object, slot int) {
	if slot < 0 || slot >= EquipSlots {
		return
	}
	w.rebuildCharSet(o) // runs CalcCharacter (FUN_004c6ba0)
	w.broadcast(o, packetEquipChange(o.Index, slot, o.P.Inventory[slot]))
}

// packetEquipChange is C1 0C 25 index(BE) item[7] with byte 6 replaced by
// slot<<4 | ((level-1)/2)&0xF (FUN_0049b230), sent to VP2 players.
func packetEquipChange(index, slot int, it item.Item) []byte {
	p := newPacket(frame.C1, 0x25, -1, 0x0C)
	p[3], p[4] = byte(index>>8), byte(index)
	it.EncodeWire(p[5:12], false)
	p[6] = byte(slot)<<4 | byte(((it.Level-1)/2)&0xF)
	return p
}

// packetCharSetUpdate is C1 18 F3 13 index(BE) CharSet[18] (FUN_0049b5a0),
// sent to VP2 players after an item arrives in the inventory.
func packetCharSetUpdate(o *Object) []byte {
	p := newPacket(frame.C1, 0xF3, 0x13, 0x18)
	p[4], p[5] = byte(o.Index>>8), byte(o.Index)
	copy(p[6:], o.CharSet[:])
	return p
}

// packetItemDurability is C1 06 2A slot durability flag (FUN_0049b3b0).
func packetItemDurability(slot int, dur byte, flag byte) []byte {
	p := newPacket(frame.C1, 0x2A, -1, 6)
	p[3], p[4], p[5] = byte(slot), dur, flag
	return p
}

// packetItemDelete is C1 05 28 slot flag (FUN_0049b2e0).
func packetItemDelete(slot int, flag byte) []byte {
	p := newPacket(frame.C1, 0x28, -1, 5)
	p[3], p[4] = byte(slot), flag
	return p
}

// packetItemModify is C1 0C F3 14 slot item[7] (FUN_0049b620): an item
// changed in place (jewel upgrades).
func packetItemModify(slot int, it item.Item) []byte {
	p := newPacket(frame.C1, 0xF3, 0x14, 0x0C)
	p[4] = byte(slot)
	it.EncodeWire(p[5:12], false)
	return p
}

// truncDur is the FISTP with truncation used for durability bytes.
func truncDur(f float32) byte {
	if f <= 0 {
		return 0
	}
	if f >= 255 {
		return 255
	}
	return byte(int(f))
}

// moneyFits is FUN_0050cf10: money + amount stays within 2,000,000,000.
func moneyFits(money, amount int64) bool { return money+amount <= 2000000000 && money+amount >= 0 }

// addMoney adds zen with the 2,000,000,000 clamp of the callers.
func addMoney(o *Object, amount int64) {
	c := o.P.Char
	if moneyFits(c.Money, amount) {
		c.Money += amount
	} else {
		c.Money = 2000000000
	}
}

// stackMergeInto is FUN_00495960: merge a picked/bought stackable item
// into existing bag stacks (slots 12..75). Returns true when nothing is
// left. it.Durability is reduced by what was merged.
func (w *World) stackMergeInto(o *Object, it *item.Item) bool {
	if it.IsEmpty() || w.itemData == nil {
		return false
	}
	max := w.itemData.stack.Max(it.Index, it.Level)
	if max <= 0 {
		return false
	}
	inv := &o.P.Inventory
	for s := BagStart; s < BagEnd; s++ {
		e := &inv[s]
		if e.IsEmpty() || e.Index != it.Index || e.Level != it.Level || e.JoH != it.JoH ||
			(e.OptionEx^it.OptionEx)&0x40 != 0 || float32(max) <= e.Durability {
			continue
		}
		move := float32(int(float32(max) - e.Durability))
		if it.Durability < move {
			move = float32(int(it.Durability))
		}
		it.Durability -= move
		e.Durability += move
		w.send(o, packetItemDurability(s, truncDur(e.Durability), 0))
		if it.Durability < 1 {
			return true
		}
	}
	return false
}
