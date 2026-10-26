// Package item models CItem and its two wire formats
// (docs/server-plan/09-itens-e-inventario.md §2):
// the 16-byte database encoding (GameServer 0x496830 write / 0x4969c0 read)
// and the 7-byte client encoding (0x496700).
package item

import "encoding/binary"

// Index helpers: ItemIndex = category*512 + index.
const (
	MaxIndex = 0x2000
	DBSize   = 16
	WireSize = 7
)

// Make returns GET_ITEM(cat, idx).
func Make(cat, idx int) int { return cat*512 + idx }

// Item is the subset of CItem that is persisted or sent to clients.
type Item struct {
	Index      int     // -1 = empty
	Level      int     // 0..15
	Durability float32 // CItem+0x20
	Serial     uint32
	Skill      bool
	Luck       bool
	Option     int  // additional option 0..7
	Excellent  byte // 6 flags
	SetOption  byte // ancient, 4 bits
	OptionEx   byte // CItem+0xd2 (bits 6 and 7 persisted)
	Flag1      bool // CItem+0xb7 (DB byte 9 bit 1): periodic item
	Flag2      bool // CItem+0xb6 (DB byte 9 bit 2)
	JoH        byte // Jewel of Harmony option (CItem+0xcc)

	// Dark Horse / Dark Raven level and experience (CItem+0xc4 / +0xc8).
	// They are not part of either encoding: the DataServer keeps them in
	// T_PetItem_Info by serial (game/pet.go).
	PetLevel int
	PetExp   uint32

	// Personal shop price (CItem+0x84), set by 3F 01; never persisted.
	PShopPrice int32

	// Periodic item state (CPeriodicItem): CItem+0xb8 is set once the
	// expiry time (+0xbc, Unix seconds) is known; the DataServer keeps the
	// time in PeriodicItem by serial (game/periodic.go).
	PeriodicActive bool
	Expire         int64
}

// Empty returns an empty slot.
func Empty() Item { return Item{Index: -1} }

// IsEmpty reports whether the slot holds no item.
func (it Item) IsEmpty() bool { return it.Index < 0 || it.Index >= MaxIndex }

// Category returns index / 512.
func (it Item) Category() int { return it.Index / 512 }

// skipDB lists items the original never serializes (0x496830).
func skipDB(index int) bool { return index == 0x1A13 || index == 0x1C40 || index == 0x1CDF }

// EncodeDB writes the 16-byte database form into dst.
func (it Item) EncodeDB(dst []byte) {
	if it.IsEmpty() || skipDB(it.Index) {
		for i := 0; i < DBSize; i++ {
			dst[i] = 0xFF
		}
		return
	}
	for i := 0; i < DBSize; i++ {
		dst[i] = 0
	}
	dst[0] = byte(it.Index)
	dst[1] = byte(it.Level)<<3 | b2u(it.Skill)<<7 | b2u(it.Luck)<<2 | byte(it.Option)&3
	dst[2] = byte(roundHalfEven(it.Durability))
	binary.BigEndian.PutUint32(dst[3:7], it.Serial)
	dst[7] = byte(it.Index>>1)&0x80 | it.Excellent
	if it.Option > 3 {
		dst[7] |= 0x40
	}
	dst[8] = it.SetOption&0xF | it.OptionEx&0x40
	dst[9] = byte(it.Index>>5)&0xF0 | it.OptionEx>>4&8 | b2u(it.Flag1)<<1 | b2u(it.Flag2)<<2
	dst[10] = it.JoH
	// bytes 11..15 stay 0 (the original leaves whatever the buffer had; the
	// DataServer initializes blobs with 0xFF only for empty slots).
}

// DecodeDB reads the 16-byte database form (0x4969c0). ok is false for an
// empty slot.
func DecodeDB(src []byte) (it Item, ok bool) {
	if src[0] == 0xFF && src[7]&0x80 != 0 && src[9]&0xF0 == 0xF0 {
		return Empty(), false
	}
	it.Index = int(src[7]&0x80)<<1 | int(src[9]&0xF0)<<5 | int(src[0])
	it.Skill = src[1]>>7 != 0
	it.Luck = src[1]>>2&1 != 0
	it.Option = int(src[7]>>4&4) + int(src[1]&3)
	it.Level = int(src[1] >> 3 & 0xF)
	it.Durability = float32(src[2])
	if it.Index == 0x1A46 {
		it.Durability = 2
	}
	it.Serial = binary.BigEndian.Uint32(src[3:7])
	it.Excellent = src[7] & 0x3F
	it.SetOption = src[8] & 0xF
	it.OptionEx = (src[9]&8)<<4 | src[8]&0x40
	it.Flag1 = src[9]>>1&1 != 0
	it.Flag2 = src[9]>>2&1 != 0
	it.JoH = src[10]
	return it, true
}

// EncodeWire writes the 7-byte client form (0x496700). xshopFlag is the
// Custom CCustomXShop marker (byte 4 bit 7) for items with OptionEx
// bit 0x40 that appear in the XShop table.
func (it Item) EncodeWire(dst []byte, xshopFlag bool) {
	dst[0] = byte(it.Index)
	dst[1] = byte(it.Level)<<3 | b2u(it.Skill)<<7 | byte(it.Option)&3 | b2u(it.Luck)<<2
	dst[2] = byte(roundHalfEven(it.Durability))
	dst[3] = byte(it.Index>>1)&0x80 | it.Excellent
	if it.Option > 3 {
		dst[3] |= 0x40
	}
	dst[4] = it.SetOption
	if it.OptionEx&0x40 != 0 && xshopFlag {
		dst[4] |= 0x80
	}
	dst[5] = byte(it.Index>>5)&0xF0 | it.OptionEx>>4&8
	dst[6] = it.JoH
}

// DecodeWire parses the 7-byte client form (used for items the client sends
// back, e.g. personal shop and trade previews).
func DecodeWire(src []byte) Item {
	return Item{
		Index:      int(src[3]&0x80)<<1 | int(src[5]&0xF0)<<5 | int(src[0]),
		Skill:      src[1]&0x80 != 0,
		Luck:       src[1]&4 != 0,
		Option:     int(src[1]&3) + int(src[3]&0x40)>>4,
		Level:      int(src[1] >> 3 & 0xF),
		Durability: float32(src[2]),
		Excellent:  src[3] & 0x3F,
		SetOption:  src[4] & 0x7F,
		OptionEx:   (src[5] & 8) << 4,
		JoH:        src[6],
	}
}

func b2u(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// roundHalfEven mirrors the x87 ROUND() Ghidra shows for float->byte casts
// (default rounding mode is round-to-nearest-even).
func roundHalfEven(f float32) int {
	i := int(f)
	frac := f - float32(i)
	switch {
	case frac > 0.5:
		return i + 1
	case frac < 0.5:
		return i
	case i%2 == 0:
		return i
	}
	return i + 1
}
