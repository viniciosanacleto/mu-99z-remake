package item

import (
	"bytes"
	"testing"
)

func TestDBRoundTrip(t *testing.T) {
	it := Item{Index: Make(7, 300), Level: 13, Durability: 45, Serial: 0x01020304,
		Skill: true, Luck: true, Option: 6, Excellent: 0x2B, SetOption: 5, OptionEx: 0xC0,
		Flag1: true, Flag2: true, JoH: 0x33}
	var b [DBSize]byte
	it.EncodeDB(b[:])
	// index 7*512+300 = 3884 = 0xF2C: low 0x2C, bit8 set, category 7.
	want := []byte{0x2C, 13<<3 | 0x80 | 0x04 | 2, 45, 1, 2, 3, 4, 0x80 | 0x40 | 0x2B, 5 | 0x40, 0x70 | 8 | 2 | 4, 0x33}
	if !bytes.Equal(b[:11], want) {
		t.Fatalf("db = % X, want % X", b[:11], want)
	}
	got, ok := DecodeDB(b[:])
	if !ok || got != it {
		t.Fatalf("decode = %+v", got)
	}
}

func TestDBEmptyAndSkipped(t *testing.T) {
	var b [DBSize]byte
	Empty().EncodeDB(b[:])
	if !bytes.Equal(b[:], bytes.Repeat([]byte{0xFF}, DBSize)) {
		t.Fatalf("empty = % X", b)
	}
	if _, ok := DecodeDB(b[:]); ok {
		t.Fatal("empty decoded as item")
	}
	Item{Index: 0x1C40}.EncodeDB(b[:])
	if b[0] != 0xFF {
		t.Fatal("0x1C40 must not be serialized")
	}
	// Item 0x1FFF (15,511) encodes to the same 0xFF/0x80/0xF0 pattern as an
	// empty slot, so the original reads it back as empty.
	Item{Index: 0x1FFF, Durability: 1}.EncodeDB(b[:])
	if _, ok := DecodeDB(b[:]); ok {
		t.Fatal("0x1FFF must read back as empty")
	}
	Item{Index: 0x1FFE, Durability: 1}.EncodeDB(b[:])
	if it, ok := DecodeDB(b[:]); !ok || it.Index != 0x1FFE {
		t.Fatalf("0x1FFE decoded as %+v %v", it, ok)
	}
}

func TestSpecialDurability(t *testing.T) {
	var b [DBSize]byte
	Item{Index: 0x1A46, Durability: 9}.EncodeDB(b[:])
	if it, _ := DecodeDB(b[:]); it.Durability != 2 {
		t.Fatalf("0x1A46 durability = %v", it.Durability)
	}
}

func TestWire(t *testing.T) {
	it := Item{Index: Make(12, 15), Level: 0, Durability: 1, Option: 4, Excellent: 1, SetOption: 9, OptionEx: 0xC0, JoH: 7}
	var w [WireSize]byte
	it.EncodeWire(w[:], false)
	want := []byte{15, 0, 1, 0x40 | 1, 9, 0xC0 | 8, 7}
	if !bytes.Equal(w[:], want) {
		t.Fatalf("wire = % X, want % X", w, want)
	}
	it.EncodeWire(w[:], true)
	if w[4] != 0x89 {
		t.Fatalf("xshop flag: % X", w[4])
	}
	d := DecodeWire(w[:])
	if d.Index != it.Index || d.Option != 4 || d.Excellent != 1 || d.SetOption != 9 || d.OptionEx != 0x80 {
		t.Fatalf("decode wire = %+v", d)
	}
}

func TestRoundHalfEven(t *testing.T) {
	for in, want := range map[float32]int{0.5: 0, 1.5: 2, 2.5: 2, 2.6: 3, 2.4: 2, 255: 255} {
		if got := roundHalfEven(in); got != want {
			t.Errorf("round(%v) = %d, want %d", in, got, want)
		}
	}
}
