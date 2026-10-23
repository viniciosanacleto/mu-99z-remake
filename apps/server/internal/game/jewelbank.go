package game

import (
	"context"
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CCustomJewelBank (0x44c2e0..0x44ced0): every account keeps ten jewel
// counters (obj+0x24c4..0x24e8, table CustomJewelBank). The client deposits
// one jewel with F3 F5 and withdraws with F3 F6; each change goes to the
// DataServer as a delta (DS 0xF7 04) and the counters come back to the
// client in C1 2C F3 F5. They are loaded when the character enters the
// game (FUN_004cc730 -> DS 0xF7 05 -> FUN_0044ce20).

func init() {
	f3Handlers[0xF5] = (*World).jewelBankDeposit
	f3Handlers[0xF6] = (*World).jewelBankWithdraw
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) { w.jewelBankLoad(o) })
}

// jewelBankIndex is the jump table at 0x44c5e4 (FUN_0044c5a0): the jewel of
// each counter, in CustomJewelBank column order.
var jewelBankIndex = [store.JewelBankTypes]int{
	12*512 + 15, // Chaos
	14*512 + 16, // Life
	14*512 + 14, // Soul
	14*512 + 13, // Bless
	14*512 + 22, // Creation
	14*512 + 31, // Guardian
	14*512 + 42, // Harmony
	14*512 + 41, // GemStone
	14*512 + 43, // LowStone (refining stone)
	14*512 + 44, // HighStone
}

// jewelBankPack is the pack of the two bank types that withdraw in packs:
// Chaos (12,141) and Life (12,136). The original indexes the
// CCustomJewelPack table (0x4a1144) with the bank type and hands out a Bless
// or Soul pack instead (U-JWL-03, fixed here).
var jewelBankPack = [2]int{12*512 + 141, 12*512 + 136}

// packetJewelBank is C1 2C F3 F5 + ten u32 counters (FUN_0044ced0).
func packetJewelBank(v [store.JewelBankTypes]int) []byte {
	p := newPacket(frame.C1, 0xF3, 0xF5, 0x2C)
	for i, n := range v {
		u32le(p[4+4*i:], uint32(int32(n)))
	}
	return p
}

func (w *World) jewelBankStore() store.JewelBankStore {
	s, _ := w.store.(store.JewelBankStore)
	return s
}

// jewelBankLoad is DS 0xF7 05 (FUN_0044cd70) and its reply FUN_0044ce20,
// which copies the counters and sends F3 F5.
func (w *World) jewelBankLoad(o *Object) {
	bs := w.jewelBankStore()
	if bs == nil || o.P == nil {
		return
	}
	login, c := o.P.Account.Login, o.conn
	w.async(func(ctx context.Context) func() {
		v, err := bs.JewelBank(ctx, login)
		if err != nil {
			w.log.Error("[CustomJewelBank] load", "account", login, "err", err)
			return nil
		}
		return func() {
			if o := w.objectOf(c); o != nil && o.State == StatePlaying && o.P != nil {
				cash(o).Bank = v
				w.send(o, packetJewelBank(v))
			}
		}
	})
}

// jewelBankAdd is DS 0xF7 04 (C1 18 F7 04 index account type(u16)
// delta(i32)) followed by F3 F5 (FUN_0044ced0).
func (w *World) jewelBankAdd(o *Object, typ, delta int) {
	if bs := w.jewelBankStore(); bs != nil {
		login := o.P.Account.Login
		w.async(func(ctx context.Context) func() {
			if err := bs.AddJewelBank(ctx, login, typ, delta); err != nil {
				w.log.Error("[CustomJewelBank] save", "account", login, "type", typ, "delta", delta, "err", err)
			}
			return nil
		})
	}
	w.send(o, packetJewelBank(cash(o).Bank))
}

// jewelBankReady is the common test of both handlers: playing, no logout
// countdown, no open interface (obj+0x3d4 & 3) and no chaos mix running
// (obj+0x430).
func (w *World) jewelBankReady(o *Object) bool {
	return playerReady(o) && !ifBusy(o) && !chaos(o).Lock
}

// jewelBankDeposit is FUN_0044c610: F3 F5 type(u32). The type (< 0x4c, the
// original also reads the bag slot of the same number without using it)
// selects the jewel; with one of level 0 in the inventory the counter goes
// up, one jewel is deleted (FUN_00496ee0) and DS 0xF7 04 gets +1.
func (w *World) jewelBankDeposit(o *Object, p []byte) {
	if len(p) < 8 || !w.jewelBankReady(o) {
		return
	}
	typ := int32(binary.LittleEndian.Uint32(p[4:]))
	if typ < 0 || typ >= 0x4c || typ >= store.JewelBankTypes {
		return
	}
	idx := jewelBankIndex[typ]
	if w.countItems(o, idx, 0) <= 0 {
		return
	}
	cash(o).Bank[typ]++
	w.deleteItems(o, idx, 0, 1)
	w.jewelBankAdd(o, int(typ), 1)
}

// jewelBankWithdraw is FUN_0044c830: F3 F6 type(u32) count(u32). It needs
// a free bag cell; count 99 means "as many as free cells" and takes the
// whole counter when it has fewer. Otherwise the counter must hold count.
// Chaos and Life (types 0 and 1) asked in 10, 20 or 30 come out as one
// pack of level 0/1/2 of the same jewel; every other count creates count
// jewels of level 0. DS 0xF7 04 gets -count.
//
// Fixed against the original (U-JWL-04): it gave a single jewel for any
// other Chaos/Life count (a free jewel for count 0, a loss above 1); here
// count 0 is refused, the jewels are all created, and a count that does not
// fit in the free cells is refused before anything is taken.
func (w *World) jewelBankWithdraw(o *Object, p []byte) {
	if len(p) < 12 || !w.jewelBankReady(o) {
		return
	}
	typ := int32(binary.LittleEndian.Uint32(p[4:]))
	want := int32(binary.LittleEndian.Uint32(p[8:]))
	if typ < 0 || want <= 0 {
		return
	}
	free := w.freeBagCells(o)
	if free <= 0 || typ >= store.JewelBankTypes {
		return
	}
	n := int(want)
	pack := (typ == 0 || typ == 1) && want != 99 && (n == 10 || n == 20 || n == 30)
	if want == 99 {
		n = free
	} else if !pack && n > free {
		return
	}
	bank := &cash(o).Bank[typ]
	if want == 99 && *bank < n {
		n, *bank = *bank, 0
	} else {
		if *bank < n {
			return
		}
		*bank -= n
	}
	idx, level := jewelBankIndex[typ], 0
	if pack {
		w.createItem(o, 0xEB, 0, 0, dropSpec{Index: jewelBankPack[typ], Level: n/10 - 1, ToInventory: true})
	} else {
		for i := 0; i < n; i++ {
			w.createItem(o, 0xEB, 0, 0, dropSpec{Index: idx, Level: level, ToInventory: true})
		}
	}
	w.jewelBankAdd(o, int(typ), -n)
}
