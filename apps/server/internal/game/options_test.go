package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestOptionsPacket(t *testing.T) {
	p := packetOptions(defaultOptions())
	want := append([]byte{0xC1, 0x13, 0xF3, 0x30}, bytes.Repeat([]byte{0xFF}, 14)...)
	want = append(want, 6)
	if !bytes.Equal(p, want) {
		t.Fatalf("default = % X", p)
	}
	o, ok := parseOptions(p)
	if !ok || o.ChatWindow != 6 || o.QKey != 0xFF {
		t.Fatalf("parse = %+v", o)
	}
	if _, ok := parseOptions(p[:10]); ok {
		t.Fatal("short packet accepted")
	}
}

func TestOptionsRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc.ID, "Opt", 0x10)

	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	c.login("tester", "pass")
	name := make([]byte, 10)
	copy(name, "Opt")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	if got := c.next(0xF3, 0x30); got[18] != 6 {
		t.Fatalf("default options = % X", got)
	}
	body := append([]byte{0x30}, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 0x01, 0x02, 0x03, 0x04, 0x05)
	c.send(frame.Build(frame.C1, 0xF3, body...))

	ch, _ := env.store.LoadCharacter(ctx, acc.ID, "Opt")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o, err := env.store.LoadOptions(ctx, ch.ID); err == nil {
			if o.SkillKeys[9] != 10 || o.ChatWindow != 5 || o.QKey != 2 {
				t.Fatalf("saved = %+v", o)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("options not saved")
}
