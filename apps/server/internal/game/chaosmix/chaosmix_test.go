package chaosmix

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestMakeSet(t *testing.T) {
	sets := &gamedata.SetItemTypes{ByIndex: map[int]gamedata.SetItemType{
		1: {Index: 1, Option: [2]int{2, 0}},
		2: {Index: 2, Option: [2]int{0, 3}},
	}}
	zero := func() int { return 0 }
	cases := []struct {
		idx, v int
		want   byte
	}{{1, 1, 5}, {1, 2, 9}, {1, 3, 5}, {1, 4, 9}, {1, 5, 0}, {2, 1, 10}, {2, 2, 6}, {2, 5, 6}, {2, 6, 10}, {3, 1, 0}, {1, 0, 0}}
	for _, c := range cases {
		if got := MakeSet(sets, c.idx, c.v, zero); got != c.want {
			t.Errorf("set(%d,%d) = %d, want %d", c.idx, c.v, got, c.want)
		}
	}
}

func TestPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{name: "86 result without item", got: PacketResult(ResultFail, nil), want: []byte{0xC1, 0x0B, 0x86, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		{name: "88 rate", got: PacketRate(1, 2, 3), want: []byte{0xC1, 0x10, 0x88, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}

func TestItemClasses(t *testing.T) {
	tests := []struct {
		name string
		fn   func(int) bool
		idx  int
		want bool
	}{
		{name: "first wing", fn: IsFirstWing, idx: 0x1801, want: true},
		{name: "fairy wing", fn: IsFirstWing, idx: 0x1829, want: true},
		{name: "second wing is not first", fn: IsFirstWing, idx: 0x1803, want: false},
		{name: "chaos dragon axe", fn: IsChaosWeapon, idx: 0x406, want: true},
		{name: "other weapon", fn: IsChaosWeapon, idx: 0x407, want: false},
		{name: "cloak keeps durability", fn: FixedDurability, idx: ItemCloak, want: true},
		{name: "jewel does not", fn: FixedDurability, idx: ItemChaos, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.idx); got != tt.want {
				t.Fatalf("%s(%#x) = %v, want %v", tt.name, tt.idx, got, tt.want)
			}
		})
	}
}
