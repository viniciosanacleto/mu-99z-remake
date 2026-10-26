// Package charset builds the 18-byte CharSet (appearance) block sent in the
// character list (F3 00, built by 0x472d00) and in the player viewport
// (C2 12, built by 0x4c6ba0). Both builders share the same bit layout;
// it is reproduced here byte for byte, including the custom wing
// (CustomWing.txt) and custom pet (CustomPet.txt) markers.
package charset

// Size of a CharSet.
const Size = 18

// Equipment slots used by the CharSet, in inventory order.
const (
	SlotRightHand = iota
	SlotLeftHand
	SlotHelm
	SlotArmor
	SlotPants
	SlotGloves
	SlotBoots
	SlotWings
	SlotHelper
	Slots
)

// Piece is one equipped item as the builders see it.
type Piece struct {
	Index     int // full item index; < 0 when the slot is empty
	Level     int
	Excellent byte // 6 bits
	SetOption byte // ancient, only "!= 0" matters
}

// Custom resolves the custom tables.
type Custom interface {
	// WingSlot returns the CustomWing.txt Index column for an item index.
	WingSlot(itemIndex int) (int, bool)
	// IsPet reports whether an item index is listed in CustomPet.txt.
	IsPet(itemIndex int) bool
}

// NoCustom is a Custom with empty tables.
type NoCustom struct{}

func (NoCustom) WingSlot(int) (int, bool) { return 0, false }
func (NoCustom) IsPet(int) bool           { return false }

// Input describes the character.
type Input struct {
	DBClass     byte // Character.Class: class<<4 | change-up
	ActionState byte // obj+0x25c: 0x80 / 0x81 set bits of byte 0 (viewport only)
	FullSet     bool // obj+0x5d1: complete ancient set (viewport only)
	Items       [Slots]Piece
}

// levelShift is the per-slot shift table {1,0,6,5,4,3,2} (0x5060001,
// 0x304, 2 on the stack).
var levelShift = [7]uint{1, 0, 6, 5, 4, 3, 2}

// Build returns the CharSet.
func Build(in Input, custom Custom) [Size]byte {
	var c [Size]byte
	cls := in.DBClass
	c[0] = (cls>>4)*32 - (cls&0xF)>>1 + cls*16
	switch in.ActionState {
	case 0x80:
		c[0] |= 2
	case 0x81:
		c[0] |= 3
	}

	// i[0], i[1]: full index or 0xFFFF; i[2..8]: index & 0x1FF or 0x1FF.
	var i [Slots]uint16
	for k, p := range in.Items {
		switch {
		case p.Index < 0 || p.Index >= 0x2000:
			if k < 2 {
				i[k] = 0xFFFF
			} else {
				i[k] = 0x1FF
			}
		case k < 2:
			i[k] = uint16(p.Index)
		default:
			i[k] = uint16(p.Index & 0x1FF)
		}
	}
	lo := func(k int) byte { return byte(i[k]) }

	c[12] = byte(i[0]>>4) & 0xF0
	c[1] = lo(0)
	c[2] = lo(1)
	c[13] = byte(i[1]>>4)&0xF0 | byte(i[2]>>5)&0x0F
	c[3] = lo(2)<<4 | lo(3)&0xF
	c[14] = byte(i[3]>>1)&0xF0 | byte(i[4]>>5)&0x0F
	c[4] = lo(5)&0xF | lo(4)<<4
	c[5] = lo(6) << 4
	t := (lo(2)&0xF0)*2 | lo(3)&0x10
	t = t*2 | lo(4)&0x10
	c[9] = t*2 | lo(6)>>1&8 | lo(5)&0x10
	c[15] = byte(i[5]>>1)&0xF0 | byte(i[6]>>5)&0x0F

	var levels uint32
	for k := 0; k < 7; k++ {
		if i[k] == 0x1FF || i[k] == 0xFFFF {
			continue
		}
		p := in.Items[k]
		levels |= uint32(int32(p.Level-1)/2) << (3 * k)
		if p.Excellent&0x3F != 0 {
			c[10] |= 2 << levelShift[k]
		}
		if p.SetOption&3 != 0 {
			c[11] |= 2 << levelShift[k]
		}
	}
	if in.FullSet {
		c[11] |= 1
	}
	c[8] = byte(levels)
	c[6] = byte(levels >> 16)
	c[7] = byte(levels >> 8)

	wings(&c, i[SlotWings], custom)
	helper(&c, i[SlotHelper], custom)
	return c
}

func wings(c *[Size]byte, w uint16, custom Custom) {
	switch {
	case w == 0x1FF:
		c[5] |= 0x0C
	case w < 3:
		c[5] |= byte(w) * 4
	case w-3 < 4:
		c[5] |= 0x0C
		c[9] |= byte(w) - 2
	case w == 0x1E:
		c[5] |= 0x0C
		c[9] |= 5
	default:
		slot, isCustom := custom.WingSlot(int(w) + 0x1800)
		if (w < 0x24 || w > 0x28) && !isCustom {
			return // unknown wing: no bits, as in the original
		}
		c[5] |= 0x0C
		if isCustom {
			if slot >= 0 && slot < 100 {
				c[17] |= byte(slot)*2 + 2
			}
			return
		}
		c[17] |= (byte(w) + 0x1E) * 4
	}
}

func helper(c *[Size]byte, h uint16, custom Custom) {
	switch {
	case h == 0x1FF:
		c[5] |= 3
	case h < 3:
		c[5] |= byte(h)
	case h == 3:
		c[5] |= 3
		c[10] |= 1
	case h == 4:
		c[5] |= 3
		c[12] |= 1
	}
	if h > 0x45 && h != 0x1FF && custom.IsPet(int(h)+0x1A00) {
		c[5] |= 3
		c[16] |= (byte(h) + 0x3C) * 2
	}
}
