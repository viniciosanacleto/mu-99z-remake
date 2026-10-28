package effect

import (
	"bytes"
	"testing"
)

func TestPacketState(t *testing.T) {
	p := PacketState(true, 0x2000000, 0x1234)
	want := []byte{0xC1, 0x0C, 0x07, 1, 0, 0, 0, 2, 0x12, 0x34, 0, 0}
	if !bytes.Equal(p, want) {
		t.Fatalf("% X", p)
	}
	if p := PacketState(false, 1, 5); p[3] != 0 || p[4] != 1 {
		t.Fatalf("% X", p)
	}
}

// The ViewSkillState bits of FUN_0047a9e0 (table 0x47ab10).
func TestStateBit(t *testing.T) {
	for e, bit := range map[int]uint32{1: 4, 2: 8, 3: 0x2000000, 4: 0x100, 5: 0x800, 6: 0x4000000, 8: 0x10, 9: 0x2000,
		12: 0, 18: 0x4000, 28: 0x1000000, 55: 1, 56: 2, 57: 0x20, 58: 0x40, 61: 0x1000, 70: 0x100, 71: 0, 0: 0} {
		if got := StateBit(e); got != bit {
			t.Errorf("effect %d bit %#x, want %#x", e, got, bit)
		}
	}
}
