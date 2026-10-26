package game

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// CustomPick (CCustomPick, getter FUN_00434800; docs/server-plan/
// 10e-customs.md): an auto loot. /pick (index 63,
// FUN_004560d0) turns it on or off, /pickset (64, FUN_00456300) adds a
// text filter and /pickclear (65, FUN_00456510) resets them. Every 100 ms
// (FUN_004c48b0 -> FUN_00455c80) the items around the player are taken
// through the 22 handler. The filters and switches live in the CustomPick
// table (DS 0x13 00 on enter, DS 0x13 0x30 with every character save).

// pickState is obj+0x22bc (on), 0x22bd (filter count), 0x22be (20 filters
// of 20 bytes), 0x2450 (jewels), 0x2454 (set), 0x2458 (excellent),
// 0x245c (zen), 0x2464 (start time) and 0x24a0 (loaded from the DS).
type pickState struct {
	On                     bool
	Texts                  [store.PickSlots]string
	Count                  int
	Money, Jewel, Exc, Set int
	Start                  time.Time
	Loaded                 bool
}

func init() {
	commandHandlers[63] = (*World).commandPick
	commandHandlers[64] = (*World).commandPickSet
	commandHandlers[65] = (*World).commandPickClear
	tick100Hooks = append(tick100Hooks, (*World).pickTick)
	enterWorldHooks = append(enterWorldHooks, func(w *World, o *Object) { w.loadPick(o) })
	characterSaveHooks = append(characterSaveHooks, func(w *World, o *Object, _ *store.Character) { w.savePick(o) })
}

func pickOf(o *Object) *pickState { return &custom(o).Pick }

// pickOff is FUN_004566a0: off and message 0x1C5.
func (w *World) pickOff(o *Object) {
	pickOf(o).On = false
	w.customMsg(o, 0x1C5)
}

// commandPick is /pick [zen jewel exc set] (FUN_004560d0): CustomPickSwitch;
// on turns it off; a safe zone gives 0x1C2, a map outside
// CustomPickMapZone 0x23E. Arguments 0..3 set the zen, jewel, excellent
// and set switches; with at least one filter or switch it starts (0x1C4
// with the count and the switches, charged as index 63), otherwise 0x1C3.
func (w *World) commandPick(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomPickSwitch", 0) == 0 {
		return
	}
	pk := pickOf(o)
	if pk.On {
		w.pickOff(o)
		return
	}
	if w.inSafeZone(o) {
		w.customMsg(o, 0x1C2)
		return
	}
	if !w.mapZone("CustomPickMapZone", o.Map) {
		w.customMsg(o, 0x23E)
		return
	}
	if arg != "" {
		pk.Money, pk.Jewel, pk.Exc, pk.Set = cmdInt(arg, 0), cmdInt(arg, 1), cmdInt(arg, 2), cmdInt(arg, 3)
	}
	if pk.Count == 0 && pk.Money == 0 && pk.Jewel == 0 && pk.Exc == 0 && pk.Set == 0 {
		w.customMsg(o, 0x1C3)
		return
	}
	pk.On, pk.Start = true, w.now()
	w.customMsg(o, 0x1C4, pk.Count, pk.Money, pk.Jewel, pk.Exc, pk.Set)
	w.log.Info("[CommandCustomPick] Active", "account", o.P.Account.Login, "name", o.P.Char.Name,
		"count", pk.Count, "zen", pk.Money, "jewel", pk.Jewel, "exc", pk.Exc, "set", pk.Set)
	w.commandDone(o, c)
}

// commandPickSet is /pickset text (FUN_00456300): no text gives 0x1C6,
// fewer than 3 characters 0x1CB, 20 filters 0x1CC; the text (19
// characters at most, lower case, FUN_005137d0) goes to the next slot,
// message 0x1C7 (count, text) and the charge of index 64.
func (w *World) commandPickSet(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomPickSwitch", 0) == 0 {
		return
	}
	if arg == "" {
		w.customMsg(o, 0x1C6)
		return
	}
	text := commandToken(arg, 0, 0x14)
	if len(text) < 3 {
		w.customMsg(o, 0x1CB)
		return
	}
	pk := pickOf(o)
	if pk.Count >= store.PickSlots {
		w.customMsg(o, 0x1CC)
		return
	}
	pk.Texts[pk.Count] = strings.ToLower(text)
	pk.Count++
	w.customMsg(o, 0x1C7, pk.Count, text)
	w.log.Info("[CommandCustomPickAdd]", "account", o.P.Account.Login, "name", o.P.Char.Name, "slot", pk.Count, "text", text)
	w.commandDone(o, c)
}

// commandPickClear is /pickclear (FUN_00456510): off (0x1C5 when it was
// on), every filter and switch cleared, 0x1C8 and the charge of index 65.
func (w *World) commandPickClear(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomPickSwitch", 0) == 0 {
		return
	}
	pk := pickOf(o)
	if pk.On {
		w.pickOff(o)
	}
	pk.Count, pk.Texts = 0, [store.PickSlots]string{}
	pk.Money, pk.Jewel, pk.Exc, pk.Set = 0, 0, 0, 0
	w.customMsg(o, 0x1C8)
	w.log.Info("[CommandCustomPickClear] Reset config", "account", o.P.Account.Login, "name", o.P.Char.Name)
	w.commandDone(o, c)
}

// pickData builds the CustomPickList record.
func (pk *pickState) data() store.PickData {
	var d store.PickData
	for i, t := range pk.Texts {
		copy(d.List[i*store.PickTextSize:(i+1)*store.PickTextSize-1], t)
	}
	d.Money, d.Jewel, d.Exc, d.Set = pk.Money, pk.Jewel, pk.Exc, pk.Set
	return d
}

// setData is the DS 0x13 reply (FUN_00456730): the filters, the count of
// non-empty ones, the switches and obj+0x24a0.
func (pk *pickState) setData(d store.PickData) {
	pk.Count = 0
	for i := range pk.Texts {
		b := d.List[i*store.PickTextSize : (i+1)*store.PickTextSize]
		if n := strings.IndexByte(string(b), 0); n >= 0 {
			b = b[:n]
		}
		pk.Texts[i] = string(b)
		if pk.Texts[i] != "" {
			pk.Count++
		}
	}
	pk.Money, pk.Jewel, pk.Exc, pk.Set = d.Money, d.Jewel, d.Exc, d.Set
	pk.Loaded = true
}

// loadPick is FUN_00456890 (C1 1C 13 00 index account name), sent when
// the character enters the world.
func (w *World) loadPick(o *Object) {
	cs := w.customStore()
	if cs == nil || o.P.Char == nil {
		return
	}
	name, conn := o.P.Char.Name, o.conn
	w.async(func(ctx context.Context) func() {
		d, err := cs.LoadPick(ctx, name)
		if err != nil {
			w.log.Error("load pick", "name", name, "err", err)
			return nil
		}
		return func() {
			if o := w.objectOf(conn); o != nil && o.P.Char != nil && o.P.Char.Name == name {
				pickOf(o).setData(d)
			}
		}
	})
}

// savePick is FUN_00456930 (C2 01C0 13 30), sent by the character save
// once the DS reply arrived.
func (w *World) savePick(o *Object) {
	cs := w.customStore()
	if cs == nil || o.P.Char == nil || o.P.cust == nil || !o.P.cust.Pick.Loaded {
		return
	}
	name, d := o.P.Char.Name, o.P.cust.Pick.data()
	w.async(func(ctx context.Context) func() {
		if err := cs.SavePick(ctx, name, d); err != nil {
			w.log.Error("save pick", "name", name, "err", err)
		}
		return nil
	})
}

// pickTick runs FUN_00455c80 for every playing player.
func (w *World) pickTick() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.P.cust == nil || !o.P.cust.Pick.On {
			continue
		}
		w.pickProc(o)
	}
}

// pickBlocked is FUN_00456660: a CustomPick.txt row for the item index,
// level (-1 any) and an account level up to its AccountLevel (-1 all).
func (w *World) pickBlocked(o *Object, it item.Item) bool {
	al := accountLevel(o)
	for _, b := range w.cw().pickBlock {
		if b.Index == it.Index && (b.Level == -1 || b.Level == it.Level) && (b.AccountLevel == -1 || al <= b.AccountLevel) {
			return true
		}
	}
	return false
}

// excellentItem is FUN_0048ad90 (Dinorant and Fenrir excluded).
func excellentItem(it item.Item) bool {
	return it.Index != 13*512+3 && it.Index != 13*512+37 && it.Excellent != 0
}

// pickName is the text the filters are searched in (FUN_00455c80):
// the lower case item name (FUN_00494a40: the ItemLevel name of the
// index and level, itemlevel.go), " +level" when the level is above 0 and
// has no ItemLevel row, " +skill", " +luck" and " +option".
func (w *World) pickName(it item.Item) string {
	s := strings.ToLower(w.itemName(it.Index, it.Level))
	if _, named := w.itemLevelName(it.Index, it.Level); it.Level > 0 && !named {
		s += fmt.Sprintf(" +%d", it.Level)
	}
	if it.Skill {
		s += " +skill"
	}
	if it.Luck {
		s += " +luck"
	}
	if it.Option != 0 {
		s += " +option"
	}
	return s
}

// pickProc is FUN_00455c80: CustomPickMaxTime_AL over gives 0x1CA and
// turns it off. Outside a safe zone and on a CustomPickMapZone map, each
// live map item older than one second, not blocked by CustomPick.txt and
// within CustomPickRange is picked (C1 05 22 index through the 22
// handler) when it is a jewel (CustomPickJewelPick and the jewel switch),
// excellent, a set item, zen, or when its name contains one of the
// filters.
func (w *World) pickProc(o *Object) {
	pk := pickOf(o)
	now := w.now()
	if lim := w.cfg.ALInt("Custom", "CustomPickMaxTime", accountLevel(o), 0); lim >= 1 && !now.Before(pk.Start.Add(time.Duration(lim)*time.Second)) {
		w.customMsg(o, 0x1CA)
		if pk.On {
			w.pickOff(o)
		}
		return
	}
	if w.inSafeZone(o) || !w.mapZone("CustomPickMapZone", o.Map) {
		return
	}
	t := w.mapItems(o.Map)
	if t == nil {
		return
	}
	rng := w.cfg.Int("Custom", "CustomPickRange", 0)
	jewelOn := w.cfg.Int("Custom", "CustomPickJewelPick", 0) != 0 && pk.Jewel != 0
	excOn := w.cfg.Int("Custom", "CustomPickExcellentPick", 0) != 0 && pk.Exc != 0
	setOn := w.cfg.Int("Custom", "CustomPickAncientPick", 0) != 0 && pk.Set != 0
	moneyOn := w.cfg.Int("Custom", "CustomPickMoneyPick", 0) != 0 && pk.Money != 0
	for i := range t.items {
		m := &t.items[i]
		if !m.valid() || !m.Live || m.Looting || m.State == mapItemRemoved || now.Sub(m.Created) <= 999*time.Millisecond {
			continue
		}
		if w.pickBlocked(o, m.Item) || !nearPos(o, o.Map, m.X, m.Y, rng) {
			continue
		}
		take := (jewelOn && w.jewelItem(m.Item.Index)) || (excOn && excellentItem(m.Item)) ||
			(setOn && m.Item.SetOption&3 != 0) || (moneyOn && m.Item.Index == MoneyIndex)
		if !take && pk.Count != 0 {
			name := w.pickName(m.Item)
			for _, f := range pk.Texts {
				if len(f) > 1 && strings.Contains(name, f) {
					take = true
					break
				}
			}
		}
		if take {
			w.handlePickup(o, []byte{0xC1, 0x05, 0x22, byte(i >> 8), byte(i)})
		}
	}
}
