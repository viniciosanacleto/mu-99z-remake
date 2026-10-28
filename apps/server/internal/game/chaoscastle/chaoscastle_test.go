package chaoscastle

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{
			name: "93 result",
			got:  PacketResult(1, "Hero", User{MonsterKills: 2, Exp: 3, UserKills: 4}),
			want: []byte{0xC1, 0x1D, 0x93, 1, 0xFE, 'H', 'e', 'r', 'o', 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0, 4, 0, 0, 0},
		},
		{name: "AF 01 enter result", got: PacketEnterResult(7), want: []byte{0xC1, 0x05, 0xAF, 0x01, 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}

func TestSetAttr(t *testing.T) {
	tr := &gamedata.Terrain{Width: 4, Height: 4, Attr: make([]byte, 16)}
	SetAttr(tr, 1, 2, 0x08, true)
	SetAttr(tr, 1, 2, 0x04, true)
	if got := tr.Get(1, 2); got != 0x0C {
		t.Fatalf("after set = %#x, want 0x0C", got)
	}
	SetAttr(tr, 1, 2, 0x08, false)
	if got := tr.Get(1, 2); got != 0x04 {
		t.Fatalf("after clear = %#x, want 0x04", got)
	}
	SetAttr(tr, 4, 0, 0x08, true) // outside: ignored, no panic
	SetAttr(tr, -1, 0, 0x08, true)
}
