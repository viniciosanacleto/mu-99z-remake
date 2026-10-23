package charset

import (
	"bytes"
	"testing"
)

func empty() Input {
	var in Input
	for k := range in.Items {
		in.Items[k].Index = -1
	}
	return in
}

type tables struct {
	wings map[int]int
	pets  map[int]bool
}

func (t tables) WingSlot(i int) (int, bool) { s, ok := t.wings[i]; return s, ok }
func (t tables) IsPet(i int) bool           { return t.pets[i] }

func TestNaked(t *testing.T) {
	in := empty()
	in.DBClass = 0x10 // Dark Knight
	got := Build(in, NoCustom{})
	want := [Size]byte{0x20, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 0xF8, 0, 0, 0xF0, 0xFF, 0xFF, 0xFF, 0, 0}
	if got != want {
		t.Fatalf("got  % X\nwant % X", got, want)
	}
}

func TestClassByte(t *testing.T) {
	for cls, want := range map[byte]byte{0x00: 0x00, 0x10: 0x20, 0x20: 0x40, 0x30: 0x60, 0x40: 0x80, 0x11: 0x30, 0x21: 0x50} {
		in := empty()
		in.DBClass = cls
		if got := Build(in, NoCustom{})[0]; got != want {
			t.Errorf("class %#x -> %#x, want %#x", cls, got, want)
		}
	}
	in := empty()
	in.ActionState = 0x81
	if Build(in, NoCustom{})[0]&3 != 3 {
		t.Error("action state 0x81")
	}
}

func TestEquipment(t *testing.T) {
	in := empty()
	in.DBClass = 0x10
	in.Items[SlotRightHand] = Piece{Index: 0*512 + 1, Level: 7, Excellent: 1} // Short Sword +7 exc
	in.Items[SlotArmor] = Piece{Index: 8*512 + 21, Level: 9, SetOption: 1}
	in.Items[SlotWings] = Piece{Index: 12*512 + 4}  // wing 4: 2nd level
	in.Items[SlotHelper] = Piece{Index: 13*512 + 3} // Dinorant
	got := Build(in, NoCustom{})

	if got[1] != 1 || got[12] != 0 {
		t.Fatalf("right hand bytes: %X %X", got[1], got[12])
	}
	// armor 21 = 0x15: low nibble 5 in byte 3, bit4 in byte 9 bit6, bits 5..8 in byte 14 high nibble.
	if got[3] != 0xF5 || got[9]&0x40 == 0 || got[14]&0xF0 != 0 {
		t.Fatalf("armor bytes: % X", got)
	}
	// levels: right hand (7-1)/2=3 at bits 0..2, armor (9-1)/2=4 at bits 9..11.
	levels := uint32(got[8]) | uint32(got[7])<<8 | uint32(got[6])<<16
	if levels != 3|4<<9 {
		t.Fatalf("levels = %#x", levels)
	}
	if got[10] != 2<<1|1 /* exc right hand + dinorant */ || got[11] != 2<<5 /* ancient armor */ {
		t.Fatalf("exc/anc = %X %X", got[10], got[11])
	}
	if got[5]&0x0F != 0x0C|3 || got[9]&0x07 != 2 {
		t.Fatalf("wings/helper = %X %X", got[5], got[9])
	}
}

func TestCustomWingAndPet(t *testing.T) {
	tb := tables{wings: map[int]int{12*512 + 180: 15}, pets: map[int]bool{13*512 + 70: true}}
	in := empty()
	in.Items[SlotWings] = Piece{Index: 12*512 + 180}
	in.Items[SlotHelper] = Piece{Index: 13*512 + 70}
	got := Build(in, tb)
	if got[17] != 15*2+2 || got[5]&0x0C != 0x0C {
		t.Fatalf("custom wing: % X", got)
	}
	if got[16] != byte((70+0x3C)*2%256) || got[5]&3 != 3 {
		t.Fatalf("custom pet: % X", got)
	}
	// Third wings without a custom entry use (w+30)*4.
	in = empty()
	in.Items[SlotWings] = Piece{Index: 12*512 + 36}
	if got := Build(in, tb); got[17] != byte((36+0x1E)*4%256) {
		t.Fatalf("3rd wing: %X", got[17])
	}
	// Unknown wing: no wing bits at all.
	in.Items[SlotWings] = Piece{Index: 12*512 + 20}
	if got := Build(in, tb); got[5]&0x0C != 0 || got[17] != 0 {
		t.Fatalf("unknown wing: % X", got)
	}
}

func TestDeterministic(t *testing.T) {
	in := empty()
	a, b := Build(in, NoCustom{}), Build(in, NoCustom{})
	if !bytes.Equal(a[:], b[:]) {
		t.Fatal("non deterministic")
	}
}
