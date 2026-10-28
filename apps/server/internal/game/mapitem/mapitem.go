// Package mapitem holds the items lying on a map (CMapItem): the 300-slot
// table of each map, the slot record and the client packets of the item
// viewport, pickup and drop (C2 20, C2 21, C3 22, C1 23). The game package
// drops, expires and hands out the items.
package mapitem

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Table size and the zen item.
const (
	MaxItems   = 300
	MoneyIndex = 0x1c0f // GET_ITEM(14,15) Zen
)

// Map item states (CMapItem+0xe8).
const (
	StateNew     = 1
	StateNormal  = 2
	StateRemoved = 8
)

// Item is one CMapItem.
type Item struct {
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

// Valid reports whether the slot holds an item.
func (m *Item) Valid() bool { return !m.Item.IsEmpty() }

// Table is the item part of one CMap.
type Table struct {
	Items [MaxItems]Item
	next  int
}

// NewTable returns a table with every slot free.
func NewTable() *Table {
	t := &Table{}
	for i := range t.Items {
		t.Items[i].Item = item.Empty()
		t.Items[i].Owner, t.Items[i].Dropper = -1, -1
	}
	return t
}

// FreeSlot scans the 300 slots from the cursor (FUN_004adab0 loop); -1 when
// the map is full.
func (t *Table) FreeSlot() int {
	s := t.next
	for n := 0; n < MaxItems; n++ {
		if !t.Items[s].Valid() {
			return s
		}
		s++
		if s >= MaxItems {
			s = 0
		}
	}
	return -1
}

// Advance moves the cursor past slot s (wrapping at MaxItems).
func (t *Table) Advance(s int) {
	t.next = s + 1
	if t.next >= MaxItems {
		t.next = 0
	}
}

// Entry is one C2 20 entry: index BE (bit 7 when the item is new),
// X, Y, item[7]; zen is encoded as 0F m2 m1 00 m0 E0 m3.
func Entry(num int, m *Item) []byte {
	b := make([]byte, 11)
	b[0], b[1] = byte(num>>8), byte(num)
	if m.State == StateNew {
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

// PacketItems is C2 size 20 count + entries (FUN_00515b00).
func PacketItems(entries [][]byte) []byte {
	p := packet.New(frame.C2, 0x20, -1, 5+11*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+11*i:], e)
	}
	return p
}

// PacketDestroy is C2 size 21 count + index(BE) (FUN_00514e80).
func PacketDestroy(nums []int) []byte {
	p := packet.New(frame.C2, 0x21, -1, 5+2*len(nums))
	p[4] = byte(len(nums))
	for i, n := range nums {
		p[5+2*i], p[6+2*i] = byte(n>>8), byte(n)
	}
	return p
}

// PacketPickupResult is C3 10 22 result item[7] + 5 zero bytes
// (FUN_004982e0: 0xFF fail, 0xFD merged into a stack, else the slot).
func PacketPickupResult(result byte, it *item.Item) []byte {
	p := packet.New(frame.C3, 0x22, int(result), 0x10)
	if it != nil {
		it.EncodeWire(p[4:11], false)
	}
	return p
}

// PacketDropResult is C1 05 23 result slot (FUN_004988f0).
func PacketDropResult(result byte, slot byte) []byte {
	p := packet.New(frame.C1, 0x23, int(result), 5)
	p[4] = slot
	return p
}
