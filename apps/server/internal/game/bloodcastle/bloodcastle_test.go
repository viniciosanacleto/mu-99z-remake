package bloodcastle

import (
	"bytes"
	"testing"
)

func TestMagicSkeleton(t *testing.T) {
	for _, class := range []int{0x59, 0x5F, 0x70, 0x76, 0x7C, 0x82, 0x8F, 0x1B1} {
		if !MagicSkeleton(class) {
			t.Errorf("MagicSkeleton(%#x) = false, want true", class)
		}
	}
	if MagicSkeleton(0x58) {
		t.Error("MagicSkeleton(0x58) = true, want false")
	}
}

func TestPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			name: "93 result",
			got:  PacketResult(1, "Hero", User{Score: 0x0102, Exp: 3, Money: 4}),
			want: []byte{0xC1, 0x1D, 0x93, 1, 0xFF, 'H', 'e', 'r', 'o', 0, 0, 0, 0, 0, 0, 0, 0, 2, 1, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0},
		},
		{
			name: "9B status with weapon level",
			got:  PacketStatus(1, 0x0102, 40, 7, 0x1234, 2),
			want: []byte{0xC1, 0x0E, 0x9B, 1, 2, 1, 40, 0, 7, 0, 0x34, 0x12, 3, 0},
		},
		{
			name: "9B status without weapon level",
			got:  PacketStatus(0, 0, 0, 0, 0, -1),
			want: []byte{0xC1, 0x0E, 0x9B, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0},
		},
		{name: "9B fixed state", got: PacketState(2), want: []byte{0xC1, 0x0E, 0x9B, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0}},
		{name: "9A enter result", got: PacketEnterResult(5), want: []byte{0xC1, 0x04, 0x9A, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}
