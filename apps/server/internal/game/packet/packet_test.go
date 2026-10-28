package packet

import (
	"bytes"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestPutUint16(t *testing.T) {
	b := make([]byte, 2)
	PutUint16(b, 0x11234)
	if !bytes.Equal(b, []byte{0x34, 0x12}) {
		t.Fatalf("got % X, want 34 12", b)
	}
}

func TestPutUint32(t *testing.T) {
	b := make([]byte, 4)
	PutUint32(b, 0x01020304)
	if !bytes.Equal(b, []byte{4, 3, 2, 1}) {
		t.Fatalf("got % X, want 04 03 02 01", b)
	}
}

func TestClamp65000(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "below", in: 100, want: 100},
		{name: "limit", in: 65000, want: 65000},
		{name: "first clamped (0xFDE9)", in: 65001, want: 65000},
		{name: "above", in: 70000, want: 65000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clamp65000(tt.in); got != tt.want {
				t.Fatalf("Clamp65000(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		kind byte
		head byte
		sub  int
		n    int
		want []byte
	}{
		{name: "C1 with sub", kind: frame.C1, head: 0xF1, sub: 0x01, n: 5, want: []byte{0xC1, 5, 0xF1, 0x01, 0}},
		{name: "C1 without sub", kind: frame.C1, head: 0x50, sub: -1, n: 4, want: []byte{0xC1, 4, 0x50, 0}},
		{name: "C2 with sub", kind: frame.C2, head: 0xF3, sub: 0x10, n: 6, want: []byte{0xC2, 0, 6, 0xF3, 0x10, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.kind, tt.head, tt.sub, tt.n); !bytes.Equal(got, tt.want) {
				t.Fatalf("New = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestPutName(t *testing.T) {
	b := bytes.Repeat([]byte{0xFF}, 11)
	PutName(b, "ABCDEFGHIJKL")
	if string(b[:10]) != "ABCDEFGHIJ" || b[10] != 0xFF {
		t.Fatalf("got % X, want the name cut at 10 bytes", b)
	}
}

func TestPutGuildName(t *testing.T) {
	b := bytes.Repeat([]byte{0xFF}, 9)
	PutGuildName(b, "Guardians")
	if string(b[:8]) != "Guardian" || b[8] != 0xFF {
		t.Fatalf("got % X, want the name cut at 8 bytes", b)
	}
}
