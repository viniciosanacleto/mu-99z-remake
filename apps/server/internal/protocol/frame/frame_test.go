package frame

import (
	"bytes"
	"errors"
	"testing"
)

func TestNext(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		max     int
		wantN   int
		wantErr error
	}{
		{"empty", nil, MaxSizeConnect, 0, nil},
		{"partial header", []byte{0xC1, 0x04}, MaxSizeConnect, 0, nil},
		{"bad first byte", []byte{0x00}, MaxSizeConnect, 0, ErrHeader},
		{"bad first byte long", []byte{0xA1, 0x05, 0x00, 0x00, 0x00}, MaxSizeConnect, 0, ErrHeader},
		{"c1 complete", []byte{0xC1, 0x04, 0x00, 0x01, 0xFF}, MaxSizeConnect, 4, nil},
		{"c1 incomplete", []byte{0xC1, 0x05, 0xF4, 0x06}, MaxSizeConnect, 0, nil},
		{"c1 size too small", []byte{0xC1, 0x02, 0x00}, MaxSizeConnect, 0, ErrSize},
		{"c2 big endian size", []byte{0xC2, 0x00, 0x05, 0xF4, 0x06}, MaxSizeConnect, 5, nil},
		{"c2 above connect limit", []byte{0xC2, 0x08, 0x01, 0xF4}, MaxSizeConnect, 0, ErrSize},
		{"c2 at game limit waits", []byte{0xC2, 0x20, 0x00, 0xF4}, MaxSizeGame, 0, nil},
		{"c2 above game limit", []byte{0xC2, 0x20, 0x01, 0xF4}, MaxSizeGame, 0, ErrSize},
		{"c3", []byte{0xC3, 0x03, 0x99}, MaxSizeGame, 3, nil},
		{"c4", []byte{0xC4, 0x00, 0x03}, MaxSizeGame, 3, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := Next(tt.buf, tt.max)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if n != tt.wantN {
				t.Fatalf("n = %d, want %d", n, tt.wantN)
			}
		})
	}
}

func TestBuildAndAccessors(t *testing.T) {
	p := Build(C1, 0xF4, 0x03, 0x00)
	if want := []byte{0xC1, 0x05, 0xF4, 0x03, 0x00}; !bytes.Equal(p, want) {
		t.Fatalf("Build C1 = % X, want % X", p, want)
	}
	if Head(p) != 0xF4 {
		t.Fatalf("Head = %X", Head(p))
	}
	if sub, ok := Sub(p); !ok || sub != 0x03 {
		t.Fatalf("Sub = %X %v", sub, ok)
	}

	q := Build(C2, 0xF4, 0x06, 0x00, 0x01)
	if want := []byte{0xC2, 0x00, 0x07, 0xF4, 0x06, 0x00, 0x01}; !bytes.Equal(q, want) {
		t.Fatalf("Build C2 = % X, want % X", q, want)
	}
	if sub, ok := Sub(q); !ok || sub != 0x06 {
		t.Fatalf("C2 Sub = %X %v", sub, ok)
	}
	if !bytes.Equal(Payload(q), []byte{0x06, 0x00, 0x01}) {
		t.Fatalf("Payload = % X", Payload(q))
	}

	if _, ok := Sub(Build(C1, 0x0E)); ok {
		t.Fatal("Sub on headless packet should report !ok")
	}
}

func TestBuildPanicsOnOversizedC1(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	Build(C1, 0x00, make([]byte, 253)...)
}
