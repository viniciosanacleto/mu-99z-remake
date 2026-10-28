package guild

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPacketLayouts(t *testing.T) {
	l := PacketList(300, 4, "Rivals", []ListMember{{"Ann", 0, 0x80}, {"Bob", -1, 0x20}})
	if len(l) != 24+26 || l[0] != 0xC2 || l[3] != 0x52 || l[4] != 1 || l[5] != 2 ||
		binary.LittleEndian.Uint32(l[8:]) != 300 || l[12] != 4 || string(l[13:19]) != "Rivals" {
		t.Fatalf("52 = % X", l)
	}
	if e := l[24:37]; string(e[:3]) != "Ann" || e[10] != 0 || e[11] != 0x80 || e[12] != 0x80 {
		t.Fatalf("52 online entry = % X", e)
	}
	if e := l[37:50]; e[10] != 0xFF || e[11] != 0x7F || e[12] != 0x20 {
		t.Fatalf("52 offline entry = % X", e)
	}
	v := PacketViewport([]ViewEntry{{Number: 7, Status: 0x80, Type: 1, Relation: 2, Index: 0x53FC}})
	if !bytes.Equal(v[:5], []byte{0xC2, 0x00, 0x11, 0x65, 1}) ||
		!bytes.Equal(v[5:14], []byte{7, 0, 0, 0, 0x80, 1, 2, 0x53 | 0x80, 0xFC}) {
		t.Fatalf("65 = % X", v)
	}
	if v := PacketViewport([]ViewEntry{{Number: 7, Index: 0x53FC}}); v[12] != 0x53 {
		t.Fatalf("65 member = % X", v)
	}
	u := PacketUnionViewport([]UnionViewEntry{{Index: 0x53FD, Number: 9, Relation: 1, Union: "Alpha"}})
	if !bytes.Equal(u[:5], []byte{0xC2, 0x00, 0x19, 0x67, 1}) || u[5] != 0x53 || u[6] != 0xFD ||
		binary.LittleEndian.Uint32(u[9:]) != 9 || u[13] != 1 || string(u[14:19]) != "Alpha" {
		t.Fatalf("67 = % X", u)
	}
	g := Record{Number: 5, Name: "Lions", Type: 2}
	g.Mark[0], g.Mark[31] = 0x11, 0x22
	i := PacketInfo(&g, "Alpha")
	if len(i) != 0x3C || i[0] != 0xC1 || i[2] != 0x66 || i[4] != 5 || i[8] != 2 ||
		string(i[9:14]) != "Alpha" || string(i[17:22]) != "Lions" || i[25] != 0x11 || i[56] != 0x22 {
		t.Fatalf("66 = % X", i)
	}
	g.Members[0].Name, g.Members[3].Name = "A", "B"
	e9 := PacketUnionList(1, 2, 0, []Record{g})
	if len(e9) != 8+41 || e9[0] != 0xC2 || e9[3] != 0xE9 || e9[4] != 1 || e9[5] != 1 || e9[6] != 2 ||
		e9[8] != 2 || e9[9] != 0x11 || e9[40] != 0x22 || string(e9[41:46]) != "Lions" {
		t.Fatalf("E9 = % X", e9)
	}
	for _, c := range []struct {
		got, want []byte
	}{
		{PacketJoinRequest(0x53FC), []byte{0xC1, 5, 0x50, 0x53, 0xFC}},
		{PacketResult(5), []byte{0xC1, 4, 0x51, 5}},
		{PacketLeaveResult(1), []byte{0xC1, 4, 0x53, 1}},
		{PacketMasterQuestion(), []byte{0xC1, 3, 0x54}},
		{PacketCreateWindow(), []byte{0xC1, 3, 0x55}},
		{PacketCreateResult(1, 2), []byte{0xC1, 5, 0x56, 1, 2}},
		{PacketViewportDel(0x53FC), []byte{0xC1, 5, 0x5D, 0x53, 0xFC}},
		{PacketTypeResult(1, 0x11), []byte{0xC1, 5, 0xE2, 1, 0x11}},
		{PacketRelationRequest(1, 2, 0x5400), []byte{0xC1, 7, 0xE5, 1, 2, 0x54, 0}},
		{PacketRelationResult(2, 1, 0x18, 0x5401), []byte{0xC1, 8, 0xE6, 2, 1, 0x18, 0x54, 1}},
		{PacketUnionKickResult(1), []byte{0xC1, 7, 0xEB, 1, 1, 2, 1}},
		{PacketWarRequestResult(6), []byte{0xC1, 4, 0x60, 6}},
		{PacketWarScore(3, 4), []byte{0xC1, 6, 0x64, 3, 4, 0}},
	} {
		if !bytes.Equal(c.got, c.want) {
			t.Errorf("got % X, want % X", c.got, c.want)
		}
	}
	if p := PacketAssignResult(1, 1, "Bob"); len(p) != 0x0F || p[2] != 0xE1 || p[3] != 1 || p[4] != 1 || string(p[5:8]) != "Bob" {
		t.Fatalf("E1 = % X", p)
	}
	if p := PacketWarRequest("Lions", 1); len(p) != 0x0C || p[2] != 0x61 || string(p[3:8]) != "Lions" || p[11] != 1 {
		t.Fatalf("61 = % X", p)
	}
	if p := PacketWarStart("Lions", 0, 1); len(p) != 0x0D || p[2] != 0x62 || p[11] != 0 || p[12] != 1 {
		t.Fatalf("62 = % X", p)
	}
	if p := PacketWarEnd(1, "Lions"); len(p) != 0x0C || p[2] != 0x63 || p[3] != 1 || string(p[4:9]) != "Lions" {
		t.Fatalf("63 = % X", p)
	}
}

func TestSameName(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{name: "equal", a: "Lions", b: "Lions", want: true},
		{name: "different", a: "Lions", b: "Tigers", want: false},
		{name: "equal first 8 bytes", a: "Guardians", b: "GuardianX", want: true},
		{name: "prefix shorter than 8", a: "Lion", b: "Lions", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameName(tt.a, tt.b); got != tt.want {
				t.Fatalf("SameName(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
