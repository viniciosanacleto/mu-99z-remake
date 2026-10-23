package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestFriendPacketLayouts(t *testing.T) {
	l := packetFriendList(3, []string{"Ann", "Bob"}, func(n string) byte {
		if n == "Ann" {
			return 0
		}
		return 0xFF
	})
	if len(l) != 7+22 || l[0] != 0xC2 || l[3] != 0xC0 || l[4] != 3 || l[5] != 0x32 || l[6] != 2 ||
		string(l[7:10]) != "Ann" || l[17] != 0 || l[28] != 0xFF {
		t.Fatalf("list = % X", l)
	}
	if p := packetFriendAddResult(1, "Bob", 0); !bytes.Equal(p[:4], []byte{0xC1, 0x0F, 0xC1, 1}) || p[14] != 0 {
		t.Fatalf("add result = % X", p)
	}
	h := packetMailHeader(store.Mail{Index: 11, From: "Ann", Subject: "Hi", Read: true,
		Date: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	if len(h) != 0x50 || h[0] != 0xC3 || h[2] != 0xC6 || h[3] != 11 || string(h[5:8]) != "Ann" ||
		string(h[15:34]) != "2026-01-02 03:04:05" || string(h[45:47]) != "Hi" || h[77] != 1 {
		t.Fatalf("mail header = % X", h)
	}
	r := packetMailRead(&store.Mail{Index: 11, Memo: []byte("hey"), Photo: bytes.Repeat([]byte{7}, 18), Dir: 1, Act: 2})
	if len(r) != 0x1C+3 || r[0] != 0xC4 || r[3] != 0xC7 || r[4] != 11 || r[6] != 3 || r[8] != 7 ||
		r[26] != 2 || r[27] != 1 || string(r[28:]) != "hey" {
		t.Fatalf("mail read = % X", r)
	}
	if p := packetMailDelResult(1, 11); !bytes.Equal(p, []byte{0xC1, 0x06, 0xC8, 1, 11, 0}) {
		t.Fatalf("mail del = % X", p)
	}
	if p := packetMoney(0x01020304); !bytes.Equal(p[:8], []byte{0xC3, 0x10, 0x22, 0xFE, 1, 2, 3, 4}) || len(p) != 16 {
		t.Fatalf("money = % X", p)
	}
}

func TestFriendsEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	h, _ := auth.HashPassword("pass")
	acc2, _ := env.store.CreateAccount(ctx, "second", h, "")
	acc1, _ := env.store.AccountByLogin(ctx, "tester")
	for _, c := range []struct {
		acc  int64
		name string
	}{{acc1.ID, "Ann"}, {acc2.ID, "Bob"}} {
		env.store.CreateCharacter(ctx, c.acc, c.name, 0x10)
		ch, _ := env.store.LoadCharacter(ctx, c.acc, c.name)
		ch.Level, ch.Money = 10, 5000
		env.store.SaveCharacter(ctx, ch)
	}
	enter := func(acc, char string) *testClient {
		c := env.dial(t)
		c.next(0xF1, 0x00)
		c.verify()
		if r := c.login(acc, "pass"); r != 1 {
			t.Fatalf("login %s = %d", acc, r)
		}
		n := make([]byte, 10)
		copy(n, char)
		c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, n...)...))
		c.next(0xF3, 0x03)
		if l := c.next(0xC0, -1); l[6] != 0 {
			t.Fatalf("%s initial friends = % X", char, l)
		}
		return c
	}
	ann := enter("tester", "Ann")
	bob := enter("second", "Bob")

	name := func(s string) []byte { b := make([]byte, 10); copy(b, s); return b }
	ann.send(frame.Build(frame.C1, 0xC1, name("Bob")...))
	if r := ann.next(0xC1, -1); r[3] != 1 || r[14] != 0xFF {
		t.Fatalf("add request result = % X", r)
	}
	if req := bob.next(0xC2, -1); string(req[3:6]) != "Ann" {
		t.Fatalf("request = % X", req)
	}
	bob.send(frame.Build(frame.C1, 0xC2, append([]byte{1}, name("Ann")...)...))
	if r := bob.next(0xC1, -1); r[3] != 1 || r[14] != 0 {
		t.Fatalf("accept result = % X", r)
	}
	if st := ann.next(0xC4, -1); string(st[3:6]) != "Bob" || st[13] != 0 {
		t.Fatalf("state = % X", st)
	}
	// Ann asks again: Bob is now in her list (pending row became mutual).
	ann.send(frame.Build(frame.C1, 0xC0))
	if l := ann.next(0xC0, -1); l[6] != 1 || string(l[7:10]) != "Bob" || l[17] != 0 {
		t.Fatalf("ann list = % X", l)
	}

	// Mail from Ann to Bob (C5 is C3 per HackPacketCheck.txt).
	memo := []byte("hello bob")
	body := make([]byte, 0x36-3+len(memo))
	copy(body[8-3:], "Bob")
	copy(body[18-3:], "Subject")
	body[0x34-3] = byte(len(memo))
	copy(body[0x36-3:], memo)
	ann.send(frame.Build(frame.C3, 0xC5, body...))
	if r := ann.next(0xC5, -1); r[3] != 1 {
		t.Fatalf("mail result = % X", r)
	}
	if m := ann.next(0x22, 0xFE); m[7] != byte((5000-1000)&0xFF) || m[6] != byte((5000-1000)>>8) {
		t.Fatalf("money = % X", m)
	}
	hdr := bob.next(0xC6, -1)
	if string(hdr[5:8]) != "Ann" || string(hdr[45:52]) != "Subject" {
		t.Fatalf("mail header = % X", hdr)
	}
	idx := []byte{hdr[3], hdr[4]}
	bob.send(frame.Build(frame.C1, 0xC7, append([]byte{0}, idx...)...))
	if r := bob.next(0xC7, -1); string(r[28:]) != "hello bob" {
		t.Fatalf("mail read = % X", r)
	}
	bob.send(frame.Build(frame.C1, 0xC8, append([]byte{0}, idx...)...))
	if r := bob.next(0xC8, -1); r[3] != 1 {
		t.Fatalf("mail del = % X", r)
	}
	// Bob leaves: Ann sees him offline.
	bob.nc.Close()
	if st := ann.next(0xC4, -1); string(st[3:6]) != "Bob" || st[13] != 0xFF {
		t.Fatalf("offline state = % X", st)
	}
}
