package game

import (
	"strings"
	"testing"
)

// 91 05: F3 E4 with Message.txt 45 in the player's language, no 91 answer
// (FUN_004d8430 at 0x4d868d).
func TestIllusionTempleEventTime(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Seeker", 0, 130, 130)
	b.handleEventTime(o, []byte{0xC1, 0x05, 0x91, 0x05, 0x00})
	pk := b.drain(o)
	if find(pk, 0x91, -1) != nil {
		t.Fatal("91 answered")
	}
	n := find(pk, 0xF3, 0xE4)
	if n == nil || len(n) != 0x86 || n[4] != 0 || n[5] != 0 {
		t.Fatalf("notice % X", n)
	}
	want := b.messageFor(o, itMsgNotStrong)
	if got := strings.TrimRight(string(n[6:]), "\x00"); got != want || !strings.Contains(want, "Illusion Temple") {
		t.Fatalf("text %q want %q", got, want)
	}
	// Type 3 has no branch: the zeroed 91 answer, as before.
	b.handleEventTime(o, []byte{0xC1, 0x05, 0x91, 0x03, 0x00})
	if p := find(b.drain(o), 0x91, -1); p == nil || p[3] != 3 || p[4] != 0 || p[5] != 0 || p[6] != 0 {
		t.Fatalf("type 3 % X", p)
	}
}
