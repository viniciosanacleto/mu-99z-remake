package character

import "testing"

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "plain", in: "Hero", want: true},
		{name: "empty", in: "", want: false},
		{name: "space", in: "He ro", want: false},
		{name: "quote", in: `He"ro`, want: false},
		{name: "percent", in: "He%ro", want: false},
		{name: "apostrophe", in: "He'ro", want: false},
		{name: "slash", in: "He/ro", want: false},
		{name: "backslash", in: `He\ro`, want: false},
		{name: "underscore", in: "He_ro", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidName(tt.in); got != tt.want {
				t.Fatalf("ValidName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidDBClass(t *testing.T) {
	for _, c := range []byte{0x00, 0x01, 0x02, 0x10, 0x11, 0x12, 0x20, 0x21, 0x22, 0x30, 0x32, 0x40, 0x42, 0x50, 0x51, 0x52, 0x60, 0x62} {
		if !ValidDBClass(c) {
			t.Errorf("ValidDBClass(%#x) = false, want true", c)
		}
	}
	for _, c := range []byte{0x03, 0x31, 0x41, 0x61, 0x70} {
		if ValidDBClass(c) {
			t.Errorf("ValidDBClass(%#x) = true, want false", c)
		}
	}
}
