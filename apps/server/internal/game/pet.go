package game

import (
	"context"
	"sync"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Dark Horse (13,4) and Dark Raven (13,5): level/experience (CItem+0xc4 /
// +0xc8, FUN_0048d010 / FUN_0048db00 / FUN_0048dc80), the experience they
// gain from kills (FUN_004c69d0), the pet info packet A9 (FUN_004d8730 /
// FUN_004dacf0), the Dark Raven commands A7 (FUN_004d86c0 / FUN_0046b450)
// and the DataServer pet info protocol (request 0x09 FUN_004758f0, answer
// FUN_004747a0, save 0x34 FUN_00476430). See
// docs/server-plan/09c-chaos-loja-pets.md.

// petMaxTableLevel is the size of the pet experience table (FUN_0050ad00
// fills levels below 1000).
const petMaxTableLevel = 1000

// petExpTable is DAT_00770de8: entry L is the experience where level L
// starts; L < 3 is 0, otherwise (L+10) * L^3 * PetExperienceMultiplierConstA
// (int32 arithmetic).
func petExpTable(constA int) [petMaxTableLevel + 1]uint32 {
	var t [petMaxTableLevel + 1]uint32
	for l := 3; l < petMaxTableLevel; l++ {
		t[l] = uint32(int32(l+10) * int32(constA) * int32(l) * int32(l) * int32(l))
	}
	return t
}

// petTables caches the table of each World.
var petTables sync.Map

func (w *World) petTable() *[petMaxTableLevel + 1]uint32 {
	if t, ok := petTables.Load(w); ok {
		return t.(*[petMaxTableLevel + 1]uint32)
	}
	t := petExpTable(w.cfg.Int("Common", "PetExperienceMultiplierConstA", 0))
	petTables.Store(w, &t)
	return &t
}

func (w *World) maxPetLevel() int { return w.cfg.Int("Common", "MaxPetLevel", 0) }

// isPet reports the Dark Horse / Dark Raven indexes.
func isPet(idx int) bool { return idx == idxDarkHorse || idx == idxDarkRaven }

// petInitTable is FUN_0048d010 with an explicit table: level capped to
// MaxPetLevel, experience clamped into [table[level], table[level+1]).
func petInitTable(it *item.Item, level int, exp uint32, maxLevel int, t *[petMaxTableLevel + 1]uint32) {
	if !isPet(it.Index) {
		return
	}
	if level > maxLevel {
		level = maxLevel
	}
	level = min(max(level, 0), petMaxTableLevel-1)
	it.PetLevel = level
	if exp < t[level] {
		exp = t[level]
	}
	if t[level+1] <= exp {
		exp = t[level]
	}
	it.PetExp = exp
}

// petInit is FUN_0048d010 using the default table (MultiplierConstA 1);
// worlds use w.petInit.
func petInit(it *item.Item, level int, exp uint32, maxLevel int) {
	t := petExpTable(1)
	petInitTable(it, level, exp, maxLevel, &t)
}

func (w *World) petInit(it *item.Item, level int, exp uint32) {
	petInitTable(it, level, exp, w.maxPetLevel(), w.petTable())
}

// petAddExp is FUN_0048db00: true on level up. An item without pet data
// (level < 1) becomes level 1 / 0 and reports true.
func (w *World) petAddExp(it *item.Item, exp int64) bool {
	if it.Durability == 0 || !isPet(it.Index) {
		return false
	}
	if it.PetLevel < 1 {
		it.PetLevel, it.PetExp = 1, 0
		return true
	}
	div := int64(w.cfg.Int("Common", "PetExperienceRateDivisor", 1))
	if div == 0 {
		div = 1
	}
	gain := uint32(int32(exp / div))
	t := w.petTable()
	if it.PetLevel >= w.maxPetLevel() || it.PetLevel >= petMaxTableLevel-1 {
		return false
	}
	if t[it.PetLevel+1] <= it.PetExp+gain {
		it.PetLevel++
		it.PetExp = t[it.PetLevel]
		return true
	}
	it.PetExp += gain
	return false
}

// petSubExp is FUN_0048dc80: true on level down.
func (w *World) petSubExp(it *item.Item, exp int64) bool {
	if it.Durability == 0 || !isPet(it.Index) {
		return false
	}
	if it.PetLevel < 1 {
		it.PetLevel, it.PetExp = 1, 0
		return true
	}
	div := int64(w.cfg.Int("Common", "PetExperienceRateDivisor", 1))
	if div == 0 {
		div = 1
	}
	loss := uint32(int32(exp / div))
	t := w.petTable()
	if it.PetExp-loss < t[it.PetLevel] {
		it.PetLevel--
		it.PetExp = t[it.PetLevel+1] - 1
		return true
	}
	it.PetExp -= loss
	return false
}

// petPrice is FUN_0048cf60: the buy value of a pet with data is level *
// 2,000,000 (horse) or 1,000,000 (raven), the sell value a third of it,
// both with the >99 / >999 rounding.
func petPrice(it item.Item) (itemPrices, bool) {
	if !isPet(it.Index) || it.PetLevel < 1 {
		return itemPrices{}, false
	}
	v := uint32(it.PetLevel) * 2000000
	if it.Index == idxDarkRaven {
		v = uint32(it.PetLevel) * 1000000
	}
	buy := v
	if buy > 99 {
		buy = buy / 10 * 10
	}
	if buy > 999 {
		buy = buy / 100 * 100
	}
	sell := uint32(int32(v) / 3)
	if sell > 99 {
		sell = sell / 10 * 10
	}
	if sell > 999 {
		sell = sell / 100 * 100
	}
	return itemPrices{Buy: int64(buy), Sell: int64(sell)}, true
}

// packetPetInfo is C1 0C A9 petType invenType pos level 00 exp(u32)
// (FUN_004dacf0; petType 0 raven, 1 horse; invenType 0xFE marks a level
// up after a kill).
func packetPetInfo(petType, invenType, pos byte, level int, exp uint32) []byte {
	p := newPacket(frame.C1, 0xA9, -1, 0x0C)
	p[3], p[4], p[5], p[6] = petType, invenType, pos, byte(level)
	u32le(p[8:], exp)
	return p
}

// petEquippable is FUN_00496d00 on an inventory item.
func (w *World) petEquippable(o *Object, it item.Item) bool {
	if w.statsData == nil {
		return true
	}
	info, ok := w.data.Items[it.Index]
	if !ok {
		return false
	}
	return w.meetsRequirements(o, w.statsOf(o), w.statsData.convertItem(info, it, it.PetLevel))
}

// petKillExp is FUN_004c69d0, called with the experience a kill gave the
// player (before the level up): the Dark Horse in slot 8 and the Dark
// Raven in slot 1 gain it. A level up the owner can no longer equip is
// undone (FUN_0048dc80).
func (w *World) petKillExp(o *Object, exp int64) {
	if o.P == nil {
		return
	}
	pets := [2]struct {
		slot  int
		index int
		typ   byte
	}{{8, idxDarkHorse, 1}, {1, idxDarkRaven, 0}}
	for _, pt := range pets {
		it := &o.P.Inventory[pt.slot]
		if it.Index != pt.index {
			continue
		}
		inv := byte(0)
		if w.petAddExp(it, exp) {
			if w.petEquippable(o, *it) {
				inv = 0xFE
			} else if !w.petSubExp(it, exp) {
				continue
			}
			if recalcHook != nil {
				recalcHook(w, o)
			}
		}
		w.send(o, packetPetInfo(pt.typ, inv, byte(pt.slot), it.PetLevel, it.PetExp))
	}
}

func init() {
	petExperienceHook = func(w *World, o *Object, exp int64) { w.petKillExp(o, exp) }
	worldHandlers[0xA9] = func(w *World, o *Object, p []byte) { w.handlePetInfo(o, p) }
	worldHandlers[0xA7] = func(w *World, o *Object, p []byte) { w.handlePetCommand(o, p) }
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) { w.requestPetInfo(o, 0) })
	warehouseLoadedHook = func(w *World, o *Object) { w.requestPetInfo(o, 1) }
	warehouseSaveHook = func(w *World, o *Object) { w.savePetInfo(o, 1) }
	characterSaveHooks = append(characterSaveHooks, func(w *World, o *Object, _ *store.Character) {
		w.savePetInfo(o, 0)
	})
}

// handlePetInfo is A9 [3] petType [4] invenType [5] pos (FUN_004d8730).
func (w *World) handlePetInfo(o *Object, p []byte) {
	if len(p) < 6 || !playerReady(o) || o.P.Moving || p[3] > 1 {
		return
	}
	inv, pos := int(p[4]), int(p[5])
	var it item.Item
	switch inv {
	case 0:
		if pos >= InventorySlots {
			return
		}
		it = o.P.Inventory[pos]
	case 1:
		st := items(o)
		if pos >= WarehouseSlots || !st.WarehouseLoaded {
			return
		}
		it = st.Warehouse[pos]
	case 2, 3:
		if pos >= tradeSlots || !inTrade(o) {
			return
		}
		src := o
		if inv == 3 {
			if src = w.tradePartner(o); src == nil || !inTrade(src) {
				return
			}
		}
		it = trade(src).Box[pos]
	case 4:
		if pos >= chaosBoxSlots {
			return
		}
		it = chaos(o).Box[pos]
	case 5:
		if pos < ShopStart || pos >= InventorySlots {
			return
		}
		t := pshop(o).Viewing
		if t < 0 || t >= MaxObjects || w.objects[t].P == nil {
			return
		}
		it = w.objects[t].P.Inventory[pos]
	default:
		return
	}
	w.send(o, packetPetInfo(p[3], byte(inv), byte(pos), it.PetLevel, it.PetExp))
}

// requestPetInfo is FUN_004758f0 (DS 0x09) and its answer FUN_004747a0:
// the pets of the inventory (0) or the warehouse (1) get level and
// experience by serial; equipped ones recompute the character.
func (w *World) requestPetInfo(o *Object, where int) {
	ps, ok := w.store.(store.PetStore)
	if !ok || o.P == nil {
		return
	}
	list := o.P.Inventory[:]
	if where == 1 {
		list = items(o).Warehouse[:]
	}
	var pos []int
	var serials []uint32
	for i, it := range list {
		if isPet(it.Index) {
			pos = append(pos, i)
			serials = append(serials, it.Serial)
		}
	}
	if len(serials) == 0 {
		return
	}
	cn := o.conn
	w.async(func(ctx context.Context) func() {
		info, err := ps.LoadPetInfo(ctx, serials)
		return func() {
			o := w.objectOf(cn)
			if o == nil || o.P == nil {
				return
			}
			if err != nil {
				w.log.Error("load pet info", "err", err)
				return
			}
			list := o.P.Inventory[:]
			if where == 1 {
				list = items(o).Warehouse[:]
			}
			recalc := false
			for i, pi := range info {
				p := pos[i]
				it := &list[p]
				if !isPet(it.Index) || it.Serial != pi.Serial {
					continue
				}
				w.petInit(it, pi.Level, pi.Exp)
				if where == 0 && p < EquipSlots {
					recalc = true
				}
			}
			if recalc && recalcHook != nil {
				recalcHook(w, o)
			}
		}
	})
}

// savePetInfo is FUN_00476430 (DS 0x34): every pet with data of the
// inventory (0) or warehouse (1).
func (w *World) savePetInfo(o *Object, where int) {
	ps, ok := w.store.(store.PetStore)
	if !ok || o.P == nil {
		return
	}
	list := o.P.Inventory[:]
	if where == 1 {
		list = items(o).Warehouse[:]
	}
	var rows []store.PetInfo
	for _, it := range list {
		if isPet(it.Index) && it.PetLevel > 0 {
			rows = append(rows, store.PetInfo{Serial: it.Serial, Level: it.PetLevel, Exp: it.PetExp})
		}
	}
	if len(rows) == 0 {
		return
	}
	w.async(func(ctx context.Context) func() {
		if err := ps.SavePetInfo(ctx, rows); err != nil {
			w.log.Error("save pet info", "err", err)
		}
		return nil
	})
}

// --- Dark Raven (CDarkSpirit) -------------------------------------------------------

// petState is the Dark Raven of a Dark Lord (CDarkSpirit at
// DAT_007a1628 + index*0x28). The attack AI lives in pet_raven.go.
type petState struct {
	Owner  bool      // +0x14 valid: a Dark Lord with a usable raven (FUN_0046b0c0)
	Next   time.Time // +0x1c: next attack (GetTickCount)
	Mode   int       // +0x20: 0 normal, 1 random attack, 2 attack with owner, 3 attack target
	Target int       // +0x18
	// FUN_0046b0c0 stats (CServerInfo DarkSpirit* keys, stats.go
	// calcDarkSpirit): +4 min, +8 max, +0xc speed, +0x10 success rate.
	DamageMin, DamageMax, AttackSpeed, SuccessRate int
	// DurCount is CItem+0x28 of the raven (slot 1): the damage counter of
	// FUN_0046c440 that costs one durability point when full.
	DurCount int
}

func pet(o *Object) *petState {
	if o.P == nil {
		return &petState{Target: -1}
	}
	if o.P.Pet == nil {
		o.P.Pet = &petState{Target: -1}
	}
	return o.P.Pet
}

// packetPetCommand is C1 07 A7 00 mode targetHi targetLo (FUN_0046b450 /
// FUN_0046b2d0 / FUN_0046b3b0).
func packetPetCommand(mode byte, target int) []byte {
	p := newPacket(frame.C1, 0xA7, 0, 7)
	p[4], p[5], p[6] = mode, byte(target>>8), byte(target)
	return p
}

// handlePetCommand is A7 [3] petType (0 = raven) [4] mode [5..6] target
// (FUN_004d86c0 -> FUN_0046b450): only a Dark Lord whose raven is active
// (CDarkSpirit+0x14 set by FUN_0046b0c0) changes the mode, and only to a
// different one; mode 3 (FUN_0046b2d0) also takes the target and is
// answered even when unchanged. Modes 0..2 leave the target bytes of the
// answer uninitialised in the original (sent as 0 here).
func (w *World) handlePetCommand(o *Object, p []byte) {
	if len(p) < 5 || !playerReady(o) || o.P.Moving || p[3] != 0 || o.Class != 4 {
		return
	}
	ps := pet(o)
	if !ps.Owner {
		return
	}
	mode := int(p[4])
	if mode == 3 {
		if len(p) < 7 {
			return
		}
		target := int(p[5])<<8 | int(p[6])
		if target < 0 || target >= MaxObjects {
			return
		}
		ps.Target = target
		ps.Mode = 3
		w.send(o, packetPetCommand(3, target))
		return
	}
	if mode > 2 {
		mode = 0
	}
	if ps.Mode == mode {
		return
	}
	ps.Mode = mode
	w.send(o, packetPetCommand(byte(mode), 0))
}
