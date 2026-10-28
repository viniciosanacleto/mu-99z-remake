package move

import (
	"bytes"
	"testing"
	"time"
)

func TestPackets(t *testing.T) {
	if got := PacketWalk(0x53FC, 130, 131, 3); !bytes.Equal(got, []byte{0xC1, 0x08, 0xD3, 0x53, 0xFC, 130, 131, 0x30}) {
		t.Fatalf("D3 = % X", got)
	}
	if got := PacketPosition(0x53FD, 10, 20); !bytes.Equal(got, []byte{0xC1, 0x07, 0xDF, 0x53, 0xFD, 10, 20}) {
		t.Fatalf("DF = % X", got)
	}
	if got := PacketAction(0x53FC, 2, 0x7A, 0, 0); !bytes.Equal(got, []byte{0xC1, 0x09, 0x18, 0x53, 0xFC, 2, 0x7A, 0, 0}) {
		t.Fatalf("18 = % X", got)
	}
}

func TestDirTable(t *testing.T) {
	want := [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}
	for d := byte(0); d < 8; d++ {
		if x, y := DirStep(d); x != want[d][0] || y != want[d][1] {
			t.Fatalf("dir %d = %d,%d", d, x, y)
		}
	}
}

func TestStepDelay(t *testing.T) {
	cases := []struct {
		dir  byte
		ms   int
		slow bool
		want time.Duration
	}{
		{3, 400, false, 400 * time.Millisecond},
		{2, 400, false, 520 * time.Millisecond},
		{4, 401, false, 521 * time.Millisecond}, // 521.3 truncated (FISTP, RC=chop)
		{0, 400, true, 910 * time.Millisecond},
		{7, 400, true, 700 * time.Millisecond},
	}
	for _, c := range cases {
		if got := StepDelay(c.dir, c.ms, c.slow); got != c.want {
			t.Fatalf("stepDelay(%d,%d,%v) = %v, want %v", c.dir, c.ms, c.slow, got, c.want)
		}
	}
}

func TestStepToward(t *testing.T) {
	tests := []struct {
		name      string
		v, target int
		want      int
	}{
		{name: "below", v: 3, target: 7, want: 4},
		{name: "above", v: 7, target: 3, want: 6},
		{name: "there", v: 5, target: 5, want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StepToward(tt.v, tt.target); got != tt.want {
				t.Fatalf("StepToward(%d, %d) = %d, want %d", tt.v, tt.target, got, tt.want)
			}
		})
	}
}
