package friend

import (
	"bytes"
	"testing"
)

func TestPackets(t *testing.T) {
	name := []byte{'A', 'n', 'n', 0, 0, 0, 0, 0, 0, 0}
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			name: "C0 list",
			got:  PacketList(3, []string{"Ann"}, func(string) byte { return 2 }),
			want: cat([]byte{0xC2, 0, 18, 0xC0, 3, 0x32, 1}, name, []byte{2}),
		},
		{name: "C2 request", got: PacketRequest("Ann"), want: cat([]byte{0xC1, 0x0D, 0xC2}, name)},
		{name: "C4 state", got: PacketState("Ann", 0xFF), want: cat([]byte{0xC1, 0x0E, 0xC4}, name, []byte{0xFF})},
		{name: "C1 add result", got: PacketAddResult(1, "Ann", 4), want: cat([]byte{0xC1, 0x0F, 0xC1, 1}, name, []byte{4})},
		{name: "C3 delete result", got: PacketDelResult(1, "Ann"), want: cat([]byte{0xC1, 0x0E, 0xC3, 1}, name)},
		{name: "C5 mail result", got: PacketMailResult(2), want: []byte{0xC1, 0x08, 0xC5, 2, 0, 0, 0, 0}},
		{name: "C8 mail delete result", got: PacketMailDelResult(1, 0x0102), want: []byte{0xC1, 0x06, 0xC8, 1, 2, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}
