package mapitem

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
)

func TestTableFreeSlot(t *testing.T) {
	tb := NewTable()
	if s := tb.FreeSlot(); s != 0 {
		t.Fatalf("empty table FreeSlot = %d, want 0", s)
	}
	tb.Items[0].Item = item.Item{Index: 1}
	tb.Advance(0)
	if s := tb.FreeSlot(); s != 1 {
		t.Fatalf("after slot 0 FreeSlot = %d, want 1", s)
	}
	// The cursor wraps: from the last slot the scan continues at 0.
	tb.Advance(MaxItems - 1)
	tb.Items[0].Item = item.Empty()
	if s := tb.FreeSlot(); s != 0 {
		t.Fatalf("wrapped FreeSlot = %d, want 0", s)
	}
	for i := range tb.Items {
		tb.Items[i].Item = item.Item{Index: 1}
	}
	if s := tb.FreeSlot(); s != -1 {
		t.Fatalf("full table FreeSlot = %d, want -1", s)
	}
}

func TestEntryZen(t *testing.T) {
	m := &Item{Item: item.Item{Index: MoneyIndex}, Money: 0x01020304, X: 10, Y: 20, State: StateNew}
	want := []byte{0x80 | 0x01, 0x02, 10, 20, 0x0F, 0x02, 0x03, 0, 0x04, 0xE0, 0x01}
	if got := Entry(0x0102, m); !bytes.Equal(got, want) {
		t.Fatalf("Entry = % X, want % X", got, want)
	}
}

func TestPackets(t *testing.T) {
	tests := []struct {
		name string
		got  []byte
		want []byte
	}{
		{name: "20 items", got: PacketItems([][]byte{bytes.Repeat([]byte{7}, 11)}), want: append([]byte{0xC2, 0, 16, 0x20, 1}, bytes.Repeat([]byte{7}, 11)...)},
		{name: "21 destroy", got: PacketDestroy([]int{0x0102, 3}), want: []byte{0xC2, 0, 9, 0x21, 2, 1, 2, 0, 3}},
		{name: "22 pickup failed", got: PacketPickupResult(0xFF, nil), want: []byte{0xC3, 0x10, 0x22, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{name: "23 drop result", got: PacketDropResult(1, 12), want: []byte{0xC1, 0x05, 0x23, 1, 12}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Equal(tt.got, tt.want) {
				t.Fatalf("got % X, want % X", tt.got, tt.want)
			}
		})
	}
}
