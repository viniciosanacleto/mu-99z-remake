package xshop

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestPackets(t *testing.T) {
	if p := PacketCredits(0x01020304); !bytes.Equal(p, []byte{0xC1, 8, 0xFA, 0x03, 4, 3, 2, 1}) {
		t.Fatalf("FA 03 % X", p)
	}
	want := []byte{0xC1, 0x10, 0xFA, 0x04, 7, 0, 0, 0, 0x64, 0, 0, 0, 0x1d, 0x00, 0, 0}
	if p := PacketResult(7, 100, 29); !bytes.Equal(p, want) {
		t.Fatalf("FA 04 % X", p)
	}
	if p := PacketGMResult(3); len(p) != 12 || p[1] != 12 || p[3] != 6 || p[4] != 3 {
		t.Fatalf("FA 06 % X", p)
	}
	p := PacketPage(2, 1, 300, []Entry{{0x401, 0x11223344, 5}, {0x402, 9, 1}})
	if len(p) != 0x68 || p[1] != 0x68 || p[3] != 0x09 || p[4] != 2 || p[5] != 1 || p[6] != 0xFF || p[7] != 2 {
		t.Fatalf("FA 09 header % X", p[:8])
	}
	if !bytes.Equal(p[8:20], []byte{1, 4, 0, 0, 0x44, 0x33, 0x22, 0x11, 5, 0, 0, 0}) || p[20] != 2 || p[24] != 9 || p[28] != 1 {
		t.Fatalf("FA 09 entries % X", p[8:32])
	}
	if p := PacketPage(0, 0, 3, nil); p[6] != 3 || p[7] != 0 {
		t.Fatalf("FA 09 pages % X", p[:8])
	}
}

func TestEMVAmount(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"000201" + "540525.00" + "6304ABCD", "25.00", true},
		{"00020126", "", false},               // length past the end
		{"0002015404" + "1.00", "1.00", true}, // tag 54 last
		{"5404" + "1.00" + "5404" + "2.00", "", false},
		{"0A0201", "", false},
		{"000201", "", false}, // no tag 54
	} {
		v, ok := emvAmount(c.in)
		if v != c.want || ok != c.ok {
			t.Fatalf("%q: %q %v", c.in, v, ok)
		}
	}
	if !amountText("25.00") || amountText("2500") || amountText("2a.00") || amountText(".00") {
		t.Fatal("amount format")
	}
}

func TestRechargeImageRoundTrip(t *testing.T) {
	d := new([RechargeSize]byte)
	binary.LittleEndian.PutUint16(d[RechargeQRSize:], 5)
	d[RechargeQRBits] = 0b10100101
	d[RechargeQRBits+3] = 0x01 // bit 24, the last of 5x5
	path := filepath.Join(t.TempDir(), "q.bmp")
	if err := os.WriteFile(path, EncodeRechargeBMP(d), 0o644); err != nil {
		t.Fatal(err)
	}
	var back [RechargeSize]byte
	if !LoadRechargeImage(path, &back) || back != *d {
		t.Fatalf("round trip % X", back[RechargeQRSize:RechargeQRSize+8])
	}
	// A 24-bit bottom-up 4x2 image scales into a 4x4 matrix, centred.
	img := []byte{'B', 'M', 0, 0, 0, 0, 0, 0, 0, 0, 0x36, 0, 0, 0, 0x28, 0, 0, 0, 4, 0, 0, 0, 2, 0, 0, 0, 1, 0, 24, 0}
	img = append(img, make([]byte, 0x36-len(img))...)
	// bottom row (y=1): white, black, white, black; top row (y=0): black x4
	img = append(img, 0xff, 0xff, 0xff, 0, 0, 0, 0xff, 0xff, 0xff, 0, 0, 0)
	img = append(img, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatal(err)
	}
	clear(back[:])
	if !LoadRechargeImage(path, &back) || binary.LittleEndian.Uint16(back[RechargeQRSize:]) != 4 {
		t.Fatal("24-bit image")
	}
	// rows 1 and 2 of the 4x4 matrix: 1111 then 0101 (bits 4..11).
	if back[RechargeQRBits] != 0xF0 || back[RechargeQRBits+1] != 0x0A {
		t.Fatalf("bits % X", back[RechargeQRBits:RechargeQRBits+2])
	}
	if LoadRechargeImage(filepath.Join(t.TempDir(), "none.bmp"), &back) {
		t.Fatal("missing file loaded")
	}
}

func TestOptionsValid(t *testing.T) {
	tests := []struct {
		name  string
		opts  [5]byte
		flags byte
		want  bool
	}{
		{name: "no options, no flags", opts: [5]byte{}, flags: 0, want: true},
		{name: "options without flag bit 0", opts: [5]byte{1, 0, 0, 0, 0}, flags: 0, want: false},
		{name: "level, luck and option", opts: [5]byte{15, 0, 1, 7, 0}, flags: 1, want: true},
		{name: "level too high", opts: [5]byte{16, 0, 0, 0, 0}, flags: 7, want: false},
		{name: "skill without bit 1", opts: [5]byte{0, 1, 0, 0, 0}, flags: 1, want: false},
		{name: "skill with bit 1", opts: [5]byte{0, 1, 0, 0, 0}, flags: 3, want: true},
		{name: "excellent without bit 2", opts: [5]byte{0, 0, 0, 0, 1}, flags: 3, want: false},
		{name: "excellent with bit 2", opts: [5]byte{0, 0, 0, 0, 0x3f}, flags: 5, want: true},
		{name: "excellent out of range", opts: [5]byte{0, 0, 0, 0, 0x40}, flags: 7, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OptionsValid(tt.opts, tt.flags); got != tt.want {
				t.Fatalf("OptionsValid(%v, %d) = %v, want %v", tt.opts, tt.flags, got, tt.want)
			}
		})
	}
}

func TestFieldText(t *testing.T) {
	if got := FieldText([]byte{'a', 'b', 0, 'c'}); got != "ab" {
		t.Fatalf("FieldText = %q, want %q", got, "ab")
	}
	if got := FieldText([]byte("abc")); got != "abc" {
		t.Fatalf("FieldText without NUL = %q, want %q", got, "abc")
	}
}

func TestCutField(t *testing.T) {
	if got := CutField("abcdef", 4); got != "abc" {
		t.Fatalf("CutField = %q, want %q (size-1 bytes)", got, "abc")
	}
	if got := CutField("ab", 4); got != "ab" {
		t.Fatalf("CutField short = %q", got)
	}
}

func TestIsHTTPURL(t *testing.T) {
	for s, want := range map[string]bool{"http://a": true, "HTTPS://a": true, "ftp://a": false, "a": false} {
		if got := IsHTTPURL(s); got != want {
			t.Errorf("IsHTTPURL(%q) = %v, want %v", s, got, want)
		}
	}
}
