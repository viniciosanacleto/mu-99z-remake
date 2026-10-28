package monsterskill

import (
	"slices"
	"testing"
)

func TestList(t *testing.T) {
	cases := []struct {
		class, at int
		want      []int
	}{
		{0, 0, nil},
		{1, 150, []int{50}},
		{1, 17, []int{17}},
		{0x42, 3, []int{3, 0x11}},
		{0x4d, 1, []int{1, 3}},
		{0xa1, 9, []int{9, 1, 0x11, 0x37, 200, 0xc9, 0xca}},
		{0x231, 1, []int{1, 9, 0x26, 0xee}},
		{0x10c, 1, []int{1}},
	}
	for _, c := range cases {
		if got := List(c.class, c.at); !slices.Equal(got, c.want) {
			t.Errorf("monsterSkillList(%#x, %d) = %v want %v", c.class, c.at, got, c.want)
		}
	}
}

func TestPacketAreaAndUse(t *testing.T) {
	p := PacketArea(50, 0x1234, 10, 20, 0)
	want := []byte{0xC3, 0x09, 0x1E, 50, 0x12, 0x34, 10, 20, 0}
	if !slices.Equal(p, want) {
		t.Fatalf("1E = % X", p)
	}
	q := PacketUse(4, 0x1234, 0x5400)
	if !slices.Equal(q, []byte{0xC1, 0x08, 0x69, 4, 0x34, 0x12, 0x00, 0x54}) {
		t.Fatalf("69 = % X", q)
	}
}

func TestDataForClass(t *testing.T) {
	info := &Info{Monster: 7}
	info.Units[0], info.Units[3] = &Unit{Index: 1}, &Unit{Index: 2}
	d := &Data{byClass: map[int]*Info{7: info}}
	if got := d.ForClass(7); got != info || got.UnitCount() != 2 {
		t.Fatalf("ForClass(7) = %+v, want the entry with 2 units", got)
	}
	if got := d.ForClass(8); got != nil {
		t.Fatalf("ForClass(8) = %+v, want nil", got)
	}
}

func TestElementDelta(t *testing.T) {
	tests := []struct {
		name string
		typ  int
		val  int
		cur  float32
		want int
	}{
		{name: "add percent", typ: 1, val: 10, cur: 205, want: 20},
		{name: "subtract percent", typ: 0x66, val: 10, cur: 205, want: -20},
		{name: "add value", typ: 0xb, val: 30, cur: 205, want: 30},
		{name: "subtract value", typ: 0x70, val: 30, cur: 205, want: -30},
		{name: "other type", typ: 3, val: 30, cur: 205, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Element{IncDecType: tt.typ, IncDecValue: tt.val}
			if got := ElementDelta(e, tt.cur); got != tt.want {
				t.Fatalf("ElementDelta = %d, want %d", got, tt.want)
			}
		})
	}
}
