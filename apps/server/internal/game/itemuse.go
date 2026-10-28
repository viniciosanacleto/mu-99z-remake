package game

import (
	"encoding/binary"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Use item 26 (FUN_00499a20): [3] source slot, [4] target slot, [5] use
// type.

func registerItemuse(h *hooks) {
	h.worldHandlers[0x26] = func(w *World, o *Object, p []byte) { w.handleUseItem(o, p) }
	h.sendLife = func(w *World, o *Object) {
		w.send(o, packetLife(0xFF, int(o.P.Char.Life)))
	}
	h.sendMana = func(w *World, o *Object) {
		w.send(o, packetMana(int(o.P.Char.Mana), int(o.P.Char.BP)))
	}
}

// packetLife is C1 0C 26 type life(BE u16, 65000 max) 00 00 life(u32)
// (FUN_004da3e0; type in CL: 0xFF current, 0xFE maximum, 0xFD unlock).
func packetLife(typ byte, life int) []byte {
	p := packet.New(frame.C1, 0x26, int(typ), 0x0C)
	v := packet.Clamp65000(life)
	p[4], p[5] = byte(v>>8), byte(v)
	binary.LittleEndian.PutUint32(p[8:], uint32(life))
	return p
}

// packetMana is C1 10 27 FF mana(BE u16) bp(BE u16) mana(u32) bp(u32)
// (FUN_004da4c0).
func packetMana(mana, bp int) []byte {
	p := packet.New(frame.C1, 0x27, 0xFF, 0x10)
	m, b := packet.Clamp65000(mana), packet.Clamp65000(bp)
	p[4], p[5] = byte(m>>8), byte(m)
	p[6], p[7] = byte(b>>8), byte(b)
	binary.LittleEndian.PutUint32(p[8:], uint32(mana))
	binary.LittleEndian.PutUint32(p[12:], uint32(bp))
	return p
}

func (w *World) handleUseItem(o *Object, p []byte) {
	if !playerReady(o) || len(p) < 6 {
		return
	}
	c := o.P.Char
	// The handler always starts by sending C1 0C 26 FD with the low byte
	// of the truncated life (the client unlocks item use on it).
	w.send(o, packetLife(0xFD, int(c.Life)&0xFF))
	if ifBusyExceptShop(o) {
		return
	}
	src, dst := int(p[3]), int(p[4])
	if src >= BagEnd || src == dst {
		return
	}
	it := o.P.Inventory[src]
	if it.IsEmpty() {
		return
	}
	idx := it.Index
	switch {
	case (idx > 0x1806 && idx < 0x1819) || idx == 0x1823 || (idx > 0x182b && idx < 0x1831) || idx > 0x1dff:
		// Orbs and scrolls (FUN_004c7200).
		str, _, _, _, _ := statTotals(o)
		if str < w.requirements(it).Str {
			return
		}
		if w.learnFromItem(o, it) {
			w.deleteInventory(o, src)
			w.send(o, packetItemDelete(src, 1))
		}
	case idx == 0x181e || idx == 0x1c0d:
		w.useJewel(o, src, dst, w.useBless)
	case idx == 0x1c0e:
		w.useJewel(o, src, dst, w.useSoul)
	case idx == 0x1c10:
		w.useJewel(o, src, dst, w.useLife)
	case (idx >= 0x1c00 && idx <= 0x1c06) || (idx >= 0x1c23 && idx <= 0x1c28) || idx == 0x1c46 ||
		idx == 0x1c47 || idx == 0x1c85:
		if !w.usePotion(o, it) {
			return
		}
		if w.cfg.ALInt("Custom", "InfinityPotion", min(max(o.P.Account.Level, 0), 3), 0) != 0 {
			return
		}
		w.consumeOne(o, src)
	case idx == evolutionItem:
		w.useEvolution(o, src, dst) // CustomEvolution.txt (customevo.go)
	default:
		// CustomJewel.txt jewels (FUN_004650b0 / FUN_004c8000,
		// customjewel.go). Other consumables (fruits, scrolls of town
		// portal, event items, boxes...) belong to their own systems.
		w.useCustomJewel(o, src, dst)
	}
}

// useJewel runs a jewel effect (FUN_004c7940 / 4c7b70 / 4c7d50) and, on
// success, deletes the jewel (C1 05 28 src 01) and sends the modified
// target (C1 0C F3 14).
func (w *World) useJewel(o *Object, src, dst int, f func(o *Object, src, dst int) bool) {
	if dst < 0 || dst >= BagEnd || !f(o, src, dst) {
		return
	}
	w.deleteInventory(o, src)
	w.send(o, packetItemDelete(src, 1))
	w.send(o, packetItemModify(dst, o.P.Inventory[dst]))
}

// upgradable is the FUN_004944b0 test: slots 0..7 (weapons, shields,
// armour, wings) or 0xec.
func (w *World) upgradable(it item.Item) bool {
	info, ok := w.itemInfo(it.Index)
	return ok && ((info.Slot >= 0 && info.Slot <= 7) || info.Slot == 0xec)
}

// rescaleDurability keeps the durability ratio after a level/option
// change (dur * newMax / oldMax).
func (w *World) rescaleDurability(it *item.Item, oldMax int) {
	newMax := w.itemMaxDurability(*it)
	if oldMax > 0 {
		it.Durability = it.Durability * float32(newMax) / float32(oldMax)
	}
}

// useBless is FUN_004c7940.
func (w *World) useBless(o *Object, src, dst int) bool {
	inv := &o.P.Inventory
	j, t := inv[src], &inv[dst]
	if t.IsEmpty() || t.Flag1 {
		return false
	}
	if j.Index == 0x181e && t.Index != 0x29 {
		return false
	}
	switch {
	case t.Index == 0x29:
		add := float32(4)
		if j.Index == 0x181e {
			add = float32((j.Level*5 + 5) * 8)
		}
		t.Durability = min(t.Durability+add, 255)
	case t.Index == 0x1a25:
		t.Durability = 255
	case w.upgradable(*t) && t.Level < 6:
		old := w.itemMaxDurability(*t)
		t.Level++
		w.rescaleDurability(t, old)
		w.rebuildCharSet(o)
	default:
		return false
	}
	return true
}

// useSoul is FUN_004c7b70: SoulSuccessRate_AL (+AddLuckSuccessRate1_AL
// with luck); failure drops the level by one below +7 and to 0 from +7.
func (w *World) useSoul(o *Object, src, dst int) bool {
	t := &o.P.Inventory[dst]
	if t.IsEmpty() || t.Flag1 || !w.upgradable(*t) || t.Level >= 9 {
		return false
	}
	al := min(max(o.P.Account.Level, 0), 3)
	rate := w.cfg.ALInt("Common", "SoulSuccessRate", al, 0)
	if t.Luck {
		rate += w.cfg.ALInt("Common", "AddLuckSuccessRate1", al, 0)
	}
	old := w.itemMaxDurability(*t)
	if w.rand(100) < rate {
		t.Level++
	} else if t.Level < 7 {
		if t.Level > 0 {
			t.Level--
		}
	} else {
		t.Level = 0
	}
	w.rescaleDurability(t, old)
	w.rebuildCharSet(o)
	return true
}

// useLife is FUN_004c7d50: +1 additional option up to MaxItemOption with
// LifeSuccessRate_AL, reset to 0 on failure.
func (w *World) useLife(o *Object, src, dst int) bool {
	t := &o.P.Inventory[dst]
	if t.IsEmpty() || t.Flag1 || !w.upgradable(*t) {
		return false
	}
	info, _ := w.itemInfo(t.Index)
	if info.Slot == 0xec || t.Option >= w.cfg.Int("Common", "MaxItemOption", 0) {
		return false
	}
	al := min(max(o.P.Account.Level, 0), 3)
	old := w.itemMaxDurability(*t)
	if w.rand(100) < w.cfg.ALInt("Common", "LifeSuccessRate", al, 0) {
		idx := t.Index
		if t.Option == 0 {
			if (idx > 0x1802 && idx < 0x1807) || idx == 0x182a || idx == 0x1831 {
				t.Excellent = t.Excellent&^0x20 | byte(w.rand(2))<<5
			}
			_, custom := w.data.Custom.WingSlot(idx)
			if (idx > 0x1823 && idx < 0x1829) || idx == 0x182b || idx == 0x1832 || custom {
				t.Excellent = t.Excellent&^0x30 | byte(w.rand(3))<<4
			}
		}
		t.Option++
	} else {
		t.Option = 0
	}
	w.rescaleDurability(t, old)
	w.rebuildCharSet(o)
	return true
}

// potionRateKeys are the per-class rate tables of FUN_004c7400 in
// CServerInfo order (DW DK FE MG DL each); the class indexes the flat
// array, so classes >= 5 read the next table like the original.
var potionRateKeys = []string{"ApplePotionRate", "SmallLifePotionRate", "MidleLifePotionRate",
	"LargeLifePotionRate", "SmallManaPotionRate", "MidleManaPotionRate", "LargeManaPotionRate"}

var potionClassPrefix = [5]string{"DW", "DK", "FE", "MG", "DL"}

func (w *World) potionRate(table, class int) int {
	n := table*5 + class
	if class < 0 || n >= len(potionRateKeys)*5 {
		return 0
	}
	return w.cfg.Int("Item", potionClassPrefix[n%5]+potionRateKeys[n/5], 0)
}

// potionHeal is rate * (AddLife + MaxLife) / 100.0, truncated.
func potionHeal(rate int, add int, max float32) int {
	return int(float64(rate) * (float64(add) + float64(max)) / 100.0)
}

// usePotion is FUN_004c7400.
func (w *World) usePotion(o *Object, it item.Item) bool {
	st := items(o)
	now := w.now()
	check := w.cfg.Int("Common", "CheckAutoPotionHack", 0) != 0
	tol := w.cfg.Int("Common", "CheckAutoPotionHackTolerance", 0)
	if check {
		if now.Before(st.LastPotion) {
			return false
		}
	}
	c, p := o.P.Char, o.P
	life, mana := 0, 0
	switch it.Index {
	case 0x1c00, 0x1c01, 0x1c02, 0x1c03:
		life = potionHeal(w.potionRate(it.Index-0x1c00, o.Class), p.AddLife, c.MaxLife)
	case 0x1c04, 0x1c05, 0x1c06:
		mana = potionHeal(w.potionRate(it.Index-0x1c00, o.Class), p.AddMana, c.MaxMana)
	}
	// 14,35..37 and 14,133 (SD) heal nothing here; 14,38..40 and 14,70/71
	// read CServerInfo offsets 0x1a84/0x1aac/0x1ad4/0x1afc/0x1b10, which no
	// known key fills (0).
	nl := float32(float64(life) + float64(c.Life))
	if lim := float32(float64(p.AddLife) + float64(c.MaxLife)); lim < nl {
		nl = lim
	}
	c.Life = nl
	w.send(o, packetLife(0xFF, int(c.Life)))
	nm := float32(float64(mana) + float64(c.Mana))
	if lim := float32(float64(p.AddMana) + float64(c.MaxMana)); lim < nm {
		nm = lim
	}
	c.Mana = nm
	if lim := c.MaxBP + float32(p.AddBP); lim < c.BP {
		c.BP = float32(int(lim))
	}
	if check {
		st.LastPotion = now.Add(msDuration(tol))
	}
	w.send(o, packetMana(int(c.Mana), int(c.BP)))
	return true
}

// consumeOne is FUN_004970f0: one charge less, or delete the item.
func (w *World) consumeOne(o *Object, slot int) {
	if slot < 0 || slot >= BagEnd {
		return
	}
	it := &o.P.Inventory[slot]
	if it.IsEmpty() {
		return
	}
	switch it.Index {
	case 0x1a12, 0x1a1d, 0x1a33, 0x1c13, 0x1c66, 0x1c6d, 0x1c6e:
		w.deleteInventory(o, slot)
		w.send(o, packetItemDelete(slot, 1))
		return
	}
	if it.Durability > 1 {
		it.Durability--
		w.send(o, packetItemDurability(slot, truncDur(it.Durability), 0))
		return
	}
	w.deleteInventory(o, slot)
	w.send(o, packetItemDelete(slot, 1))
}

// skillFromItem is FUN_004f7d70: the skill an orb/scroll teaches.
func skillFromItem(index, level int) int {
	if index >= 0x1e00 && index < 0x1e10 {
		return index&0x1ff + 1
	}
	switch index {
	case 0x1e10:
		return 0x26
	case 0x1807:
		return 0x29
	case 0x1808:
		return 0x1a
	case 0x1809:
		return 0x1b
	case 0x180a:
		return 0x1c
	case 0x180b:
		return level + 0x1e
	case 0x180c:
		return 0x2a
	case 0x180d:
		return 0x2f
	case 0x180e:
		return 0x30
	case 0x1810:
		return 0x37
	case 0x1811:
		return 0x34
	case 0x1812:
		return 0x33
	case 0x1813:
		return 0x2b
	case 0x1815:
		return 0x3d
	case 0x1816:
		return 0x3f
	case 0x1817:
		return 0x40
	case 0x1818:
		return 0x41
	case 0x1823:
		return 0x4e
	case 0x182c:
		return 0xe8
	case 0x182d:
		return 0xeb
	case 0x182e:
		return 0xea
	case 0x182f:
		return 0xec
	case 0x1830:
		return 0xee
	case 0x1e11:
		return 0x27
	case 0x1e12:
		return 0x28
	case 0x1e13:
		return 0xd7
	case 0x1e14:
		return 0xd6
	case 0x1e15:
		return 0xe6
	case 0x1e16:
		return 0xd9
	case 0x1e17:
		return 0xda
	case 0x1e18:
		return 0xdb
	case 0x1e1a:
		return 0xdd
	case 0x1e1b:
		return 0xde
	case 0x1e1c:
		return 0xe9
	case 0x1e1d:
		return 0xed
	case 0x1e1e, 0x1e1f, 0x1e20, 0x1e21, 0x1e22, 0x1e23, 0x1e24:
		return 0x106 + index - 0x1e1e
	}
	return -1
}

// learnFromItem is FUN_004c7200 (requirements, class, skill add).
func (w *World) learnFromItem(o *Object, it item.Item) bool {
	r := w.requirements(it)
	_, dex, _, _, _ := statTotals(o)
	idx := it.Index
	orb := (idx >= 0x1808 && idx <= 0x1818) || (idx >= 0x182c && idx <= 0x1830)
	if r.Dex > dex || (orb && r.Level > o.P.Char.Level) {
		return false
	}
	if !w.classAllowed(o, idx) {
		w.send(o, packetItemDelete(0xFF, 1))
		return false
	}
	sk := skillFromItem(idx, it.Level)
	if sk < 0 {
		w.send(o, packetItemDelete(0xFF, 1))
		return false
	}
	slot := w.addSkill(o, sk, byte(it.Level))
	if slot < 0 {
		w.send(o, packetItemDelete(0xFF, 1))
		return false
	}
	w.send(o, packetSkillChange(0xFE, slot, o.P.Skills[slot]))
	return true
}

func msDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
