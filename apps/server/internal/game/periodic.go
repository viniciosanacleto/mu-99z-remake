package game

import (
	"context"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Periodic items (CPeriodicItem, 0x4d36b0..0x4d3fd0): an item with
// CItem+0xb7 (item.Flag1, DB byte 9 bit 1) disappears at the Unix time in
// +0xbc. The time comes from the item's creation (time + duration) or,
// after a login, from the DataServer table PeriodicItem by serial; +0xb8
// (PeriodicActive) is set once it is known.

func init() {
	enterWorldHooks = append(enterWorldHooks, (*World).periodicLoad)
	characterSaveHooks = append(characterSaveHooks, func(w *World, o *Object, _ *store.Character) {
		w.periodicSave(o)
	})
	tick1000Hooks = append(tick1000Hooks, (*World).periodicSecond)
}

// packetPeriodicItem is C1 0C D5 01 from(u16) slot(u16) time(u32)
// (FUN_004d3f00): from is 0xFFFF for a new item, else the source slot.
func packetPeriodicItem(from, slot int, expire int64) []byte {
	p := newPacket(frame.C1, 0xD5, 0x01, 0x0C)
	u16le(p[4:], from&0xFFFF)
	u16le(p[6:], slot)
	u32le(p[8:], uint32(expire))
	return p
}

// packetPeriodicDelete is C1 05 D5 02 slot (FUN_004d3fd0).
func packetPeriodicDelete(slot int) []byte {
	p := newPacket(frame.C1, 0xD5, 0x02, 5)
	p[4] = byte(slot)
	return p
}

func (w *World) periodicStore() store.PeriodicStore {
	s, _ := w.store.(store.PeriodicStore)
	return s
}

// periodicLoad is FUN_004d3a70 (C2 2A 00, sent by the character load
// FUN_00473a10) and the reply FUN_004d38e0: every periodic item asks for
// its time; a matching item still inactive gets it and D5 01.
func (w *World) periodicLoad(o *Object) {
	ps := w.periodicStore()
	if ps == nil || o.P == nil {
		return
	}
	type entry struct {
		slot   int
		serial uint32
	}
	var list []entry
	for slot, it := range o.P.Inventory {
		if !it.IsEmpty() && it.Flag1 {
			list = append(list, entry{slot, it.Serial})
		}
	}
	if len(list) == 0 {
		return
	}
	serials := make([]uint32, len(list))
	for i, e := range list {
		serials[i] = e.serial
	}
	w.async(func(ctx context.Context) func() {
		times, err := ps.PeriodicTimes(ctx, serials)
		if err != nil {
			w.log.Error("periodic item load", "err", err)
			return nil
		}
		return func() {
			if o.State != StatePlaying || o.P == nil {
				return
			}
			for _, e := range list {
				it := &o.P.Inventory[e.slot]
				if it.IsEmpty() || it.Serial != e.serial || !it.Flag1 || it.PeriodicActive {
					continue
				}
				it.PeriodicActive, it.Expire = true, times[e.serial]
				w.send(o, packetPeriodicItem(0xFFFF, e.slot, it.Expire))
			}
		}
	})
}

// periodicSave is FUN_004d3c00 (C2 2A 30, sent by the character save
// FUN_00475c30): the time of every active periodic item.
func (w *World) periodicSave(o *Object) {
	ps := w.periodicStore()
	if ps == nil || o.P == nil {
		return
	}
	times := map[uint32]int64{}
	for _, it := range o.P.Inventory {
		if !it.IsEmpty() && it.Flag1 && it.PeriodicActive {
			times[it.Serial] = it.Expire
		}
	}
	if len(times) == 0 {
		return
	}
	w.async(func(ctx context.Context) func() {
		if err := ps.SavePeriodic(ctx, times); err != nil {
			w.log.Error("periodic item save", "err", err)
		}
		return nil
	})
}

// periodicSecond is FUN_004d36e0 (timer 6, every second): an expired item
// of a playing character without a pending logout is deleted (28 slot 1,
// equipment refresh, D5 02).
func (w *World) periodicSecond() {
	now := w.now().Unix()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o == nil || o.State != StatePlaying || !o.IsPlayer() || o.P == nil || o.P.LogoutCountdown > 0 {
			continue
		}
		for slot := range o.P.Inventory {
			it := o.P.Inventory[slot]
			if it.IsEmpty() || !it.Flag1 || !it.PeriodicActive || it.Expire > now {
				continue
			}
			w.deleteInventory(o, slot)
			w.send(o, packetItemDelete(slot, 1))
			w.equipmentChanged(o, slot)
			w.send(o, packetPeriodicDelete(slot))
		}
	}
}
