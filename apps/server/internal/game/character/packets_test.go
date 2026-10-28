package character

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// TestPacketInfoMainDLLLayout pins C3 B0 F3 03 to the offsets the original
// client's Main.dll handler (0x1001d640) reads.
func TestPacketInfoMainDLLLayout(t *testing.T) {
	p := PacketInfo(Info{
		X: 130, Y: 131, Map: 3, Dir: 2, Exp: 1000, NextExp: 2000, LevelUpPoints: 3003,
		Str: 1001, Agi: 1002, Vit: 1003, Ene: 1004, Life: 70000, MaxLife: 70001, Mana: 2198, MaxMana: 2199,
		BP: 1100, MaxBP: 1101, Money: 8955000, PKLevel: 3, CtlCode: 0x20, Leadership: 1005,
		Class: 0x10, Resets: 7, MasterResets: 2, AccountLevel: 1, Authority: 32,
	})
	if len(p) != 0xB0 || p[0] != frame.C3 || p[1] != 0xB0 || p[2] != 0xF3 || p[3] != 0x03 {
		t.Fatalf("header % X len %d", p[:4], len(p))
	}
	u16 := func(o int) int { return int(binary.LittleEndian.Uint16(p[o:])) }
	u32 := func(o int) int { return int(binary.LittleEndian.Uint32(p[o:])) }
	// 97D part (main.exe): u16 values clamped to 65000.
	if p[4] != 130 || p[5] != 131 || p[6] != 3 || p[7] != 2 || u32(8) != 1000 || u32(12) != 2000 ||
		u16(16) != 3003 || u16(18) != 1001 || u16(26) != 65000 || u16(28) != 65000 || u32(40) != 8955000 ||
		p[44] != 3 || p[45] != 0x20 || u16(50) != 1005 {
		t.Fatalf("97D part % X", p[:56])
	}
	// Main.dll part.
	for _, f := range []struct{ off, got, want int }{
		{0x38, u16(0x38), 0x10}, {0x3c, u32(0x3c), 7}, {0x40, u32(0x40), 2}, {0x44, u32(0x44), 1},
		{0x7c, u32(0x7c), 32},
		{0x80, u32(0x80), 3003}, {0x84, u32(0x84), 70000}, {0x88, u32(0x88), 70001}, {0x8c, u32(0x8c), 2198},
		{0x90, u32(0x90), 2199}, {0x94, u32(0x94), 1100}, {0x98, u32(0x98), 1101}, {0x9c, u32(0x9c), 1001},
		{0xa0, u32(0xa0), 1002}, {0xa4, u32(0xa4), 1003}, {0xa8, u32(0xa8), 1004}, {0xac, u32(0xac), 1005},
	} {
		if f.got != f.want {
			t.Errorf("[%#x] = %d, want %d", f.off, f.got, f.want)
		}
	}
	for o := 0x48; o < 0x7c; o++ {
		if p[o] != 0 {
			t.Fatalf("custom field [%#x] = %d", o, p[o])
		}
	}
}

func TestResultPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{name: "F1 01 login result", got: PacketLoginResult(3), want: []byte{0xC1, 0x05, 0xF1, 0x01, 3}},
		{name: "F1 02 logout result", got: PacketLogoutResult(1), want: []byte{0xC3, 0x05, 0xF1, 0x02, 1}},
		{name: "F3 02 delete result", got: PacketDeleteResult(1), want: []byte{0xC1, 0x05, 0xF3, 0x02, 1}},
		{
			name: "F1 00 join result",
			got:  PacketJoinResult(0x1234, [5]byte{'1', '0', '4', '0', '4'}, 0x0102),
			want: []byte{0xC1, 0x0E, 0xF1, 0x00, 1, 0x12, 0x34, '1', '0', '4', '0', '4', 0x02, 0x01},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}

func TestPacketCreateResult(t *testing.T) {
	if p := PacketCreateResult(0, "Hero", 2, 1, 0x20); len(p) != 0x2C || p[4] != 0 || string(p[5:9]) != "Hero" || p[15] != 0 {
		t.Fatalf("failure = % X, want only result and name", p)
	}
	p := PacketCreateResult(1, "Hero", 2, 0x0102, 0x20)
	if p[15] != 2 || p[16] != 0x02 || p[17] != 0x01 || p[18] != 0x20 || p[19] != 0xFF || p[42] != 0xFF || p[43] != 0 {
		t.Fatalf("success = % X", p)
	}
}
