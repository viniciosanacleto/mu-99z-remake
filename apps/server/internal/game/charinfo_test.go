package game

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// TestEnterWorldSendsCharRefresh: the Main.dll window shows level and
// experience only from F3 05 / F3 E0, so entering sends F3 E0.
func TestEnterWorldSendsCharRefresh(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	env.store.CreateCharacter(ctx, acc.ID, "Leveled", 0x00)
	ch, _ := env.store.LoadCharacter(ctx, acc.ID, "Leveled")
	ch.Level, ch.Strength, ch.Resets = 100, 1000, 4
	if err := env.store.SaveCharacter(ctx, ch); err != nil {
		t.Fatal(err)
	}

	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login = %d", r)
	}
	name := make([]byte, 10)
	copy(name, "Leveled")
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name...)...))
	info := c.next(0xF3, 0x03)
	if len(info) != 0xB0 || binary.LittleEndian.Uint32(info[0x3c:]) != 4 || binary.LittleEndian.Uint32(info[0x9c:]) != 1000 {
		t.Fatalf("F3 03 % X", info)
	}
	r := c.next(0xF3, 0xE0)
	if len(r) != 0xA4 || binary.LittleEndian.Uint16(r[4:]) != 100 || binary.LittleEndian.Uint32(r[0x90:]) != 1000 {
		t.Fatalf("F3 E0 % X", r)
	}
	exp := env.world.data.Experience
	if binary.LittleEndian.Uint32(r[8:]) != exp.Floor(100) || binary.LittleEndian.Uint32(r[12:]) != exp.Next(100) {
		t.Fatalf("F3 E0 exp %d/%d", binary.LittleEndian.Uint32(r[8:]), binary.LittleEndian.Uint32(r[12:]))
	}
}
