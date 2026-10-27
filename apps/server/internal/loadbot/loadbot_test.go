package loadbot

import (
	"bytes"
	"io"
	"math/rand/v2"
	"net"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func TestAccount(t *testing.T) {
	if l, c := Account(7); l != "load0007" || c != "ld0007" {
		t.Fatalf("Account(7) = %s %s", l, c)
	}
	if AccountClass(1) != 0x00 || AccountClass(2) != 0x10 || AccountClass(3) != 0x20 || AccountClass(4) != 0x00 {
		t.Fatal("class rotation")
	}
}

// The layouts below are the ones internal/game reads (login.go handleLogin,
// move.go handleWalk, combat.go handleAttack, chat.go, login.go
// handleLiveClient) and its tests send.
func TestPacketLayouts(t *testing.T) {
	if got := Walk(135, 125, 3, 3); !bytes.Equal(got, []byte{0xC1, 0x07, 0xD3, 135, 125, 0x31, 0x30}) {
		t.Errorf("walk = % X", got)
	}
	if got := Walk(10, 20, 5, 1, 2, 3); !bytes.Equal(got, []byte{0xC1, 0x08, 0xD3, 10, 20, 0x53, 0x12, 0x30}) {
		t.Errorf("walk 3 steps = % X", got)
	}
	if got := Walk(10, 20, 5); !bytes.Equal(got, []byte{0xC1, 0x06, 0xD3, 10, 20, 0x50}) {
		t.Errorf("turn = % X", got)
	}
	if got := Attack(0x1234, ActionAttack, 3); !bytes.Equal(got, []byte{0xC1, 0x07, 0xD7, 0x12, 0x34, 0x78, 3}) {
		t.Errorf("attack = % X", got)
	}
	if got := Chat("Hero", "hi"); !bytes.Equal(got, []byte{0xC1, 0x10, 0x00, 'H', 'e', 'r', 'o', 0, 0, 0, 0, 0, 0, 'h', 'i', 0}) {
		t.Errorf("chat = % X", got)
	}
	if got := SelectChar("Hero"); !bytes.Equal(got, []byte{0xC1, 0x0E, 0xF3, 0x03, 'H', 'e', 'r', 'o', 0, 0, 0, 0, 0, 0}) {
		t.Errorf("select = % X", got)
	}
	if got := CharList(); !bytes.Equal(got, []byte{0xC1, 0x04, 0xF3, 0x00}) {
		t.Errorf("list = % X", got)
	}
	if got := ServerList(); !bytes.Equal(got, []byte{0xC1, 0x04, 0xF4, 0x06}) {
		t.Errorf("server list = % X", got)
	}
	if got := ServerInfo(2); !bytes.Equal(got, []byte{0xC1, 0x05, 0xF4, 0x03, 2}) {
		t.Errorf("server info = % X", got)
	}
	if got := LiveClient(0x01020304, 5, 6); !bytes.Equal(got, []byte{0xC3, 0x0C, 0x0E, 0, 4, 3, 2, 1, 5, 0, 6, 0}) {
		t.Errorf("live = % X", got)
	}

	v := Verify(HWID, 1)
	if len(v) != 50 || v[2] != 0xF3 || v[3] != 0x09 || v[49] != 1 {
		t.Fatalf("verify = % X", v)
	}
	hw := append([]byte(nil), v[4:49]...)
	crypto.Bux(hw)
	if string(hw[:44]) != HWID || hw[44] != 0 {
		t.Errorf("verify hwid %q", hw)
	}

	l := Login("acc", "pw", 0x0A0B0C0D, [5]byte{'1', '0', '0', '0', '0'}, "A9kT3wX5rBz7fQ1u")
	if len(l) != 0x31 || l[0] != 0xC3 || l[2] != 0xF1 || l[3] != 0x01 {
		t.Fatalf("login = % X", l)
	}
	acc, pw := append([]byte(nil), l[4:14]...), append([]byte(nil), l[14:24]...)
	crypto.Bux(acc)
	crypto.Bux(pw)
	if string(bytes.TrimRight(acc, "\x00")) != "acc" || string(bytes.TrimRight(pw, "\x00")) != "pw" {
		t.Errorf("login account %q password %q", acc, pw)
	}
	if !bytes.Equal(l[24:28], []byte{0x0D, 0x0C, 0x0B, 0x0A}) || string(l[0x1c:0x21]) != "10000" || string(l[0x21:0x31]) != "A9kT3wX5rBz7fQ1u" {
		t.Errorf("login tail = % X", l[24:])
	}
}

func TestDirections(t *testing.T) {
	for d := byte(0); d < 8; d++ {
		x, y := Step(100, 100, d)
		if got := DirTo(100, 100, x, y); got != d {
			t.Errorf("DirTo step %d = %d", d, got)
		}
	}
	if got := DirTo(10, 10, 30, 12); got != 4 { // dx>0, dy>0
		t.Errorf("diagonal = %d", got)
	}
	if x, y := Step(10, 10, 7); x != 9 || y != 10 {
		t.Errorf("step 7 = %d,%d", x, y)
	}
}

func TestMatch(t *testing.T) {
	if !Match([]byte{0xC1, 0x05, 0xF3, 0x03, 0}, 0xF3, 0x03) || Match([]byte{0xC1, 0x05, 0xF3, 0x04, 0}, 0xF3, 0x03) {
		t.Error("C1 sub")
	}
	if !Match([]byte{0xC2, 0x00, 0x06, 0xF4, 0x06, 0}, 0xF4, 0x06) || !Match([]byte{0xC1, 0x04, 0x00, 0x01}, 0x00, -1) {
		t.Error("C2 / any sub")
	}
}

func TestParseServerInfo(t *testing.T) {
	p := append([]byte{0xC1, 0x16, 0xF4, 0x03}, []byte("192.168.1.1\x00\x00\x00\x00\x00")...)
	p = append(p, 0xAC, 0xD8) // 55468 LE
	if got, err := parseServerInfo(p, ""); err != nil || got != "192.168.1.1:55468" {
		t.Errorf("= %s %v", got, err)
	}
	if got, _ := parseServerInfo(p, "127.0.0.1"); got != "127.0.0.1:55468" {
		t.Errorf("host override = %s", got)
	}
	if _, err := parseServerInfo(p[:10], ""); err == nil {
		t.Error("short packet accepted")
	}
}

func newTestBot() *Bot {
	return &Bot{Index: 0x53FC, X: 100, Y: 100, homeX: 100, homeY: 100, Stats: &Stats{}, Profile: DefaultProfile,
		monsters: map[int]*monster{}, Rand: rand.New(rand.NewPCG(1, 2))}
}

func TestHandle(t *testing.T) {
	b := newTestBot()
	vp := make([]byte, 5+32)
	vp[0], vp[1], vp[2], vp[3], vp[4] = 0xC2, 0, byte(len(vp)), 0x13, 2
	e := vp[5:]
	e[0], e[1], e[10], e[11] = 0x80|0x01, 0x10, 101, 102 // new monster 0x0110
	e = vp[21:]
	e[0], e[1], e[10], e[11] = 0x02, 0x20, 120, 120
	b.handle(vp)
	if m := b.monsters[0x0110]; m == nil || m.x != 101 || m.y != 102 || len(b.monsters) != 2 {
		t.Fatalf("monsters = %v", b.monsters)
	}
	b.handle([]byte{0xC1, 0x08, 0xD3, 0x01, 0x10, 103, 104, 0x30})
	if m := b.monsters[0x0110]; m.x != 103 || m.y != 104 {
		t.Errorf("monster walk %d,%d", m.x, m.y)
	}
	b.handle([]byte{0xC1, 0x08, 0xD3, 0x53, 0xFC, 99, 98, 0x30})
	b.handle([]byte{0xC1, 0x07, 0xDF, 0x53, 0xFC, 97, 96})
	if b.X != 97 || b.Y != 96 {
		t.Errorf("self at %d,%d", b.X, b.Y)
	}
	b.handle([]byte{0xC1, 0x10, 0xD7, 0x01, 0x10, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	b.handle([]byte{0xC1, 0x08, 0x17, 0x01, 0x10, 0, 0x53, 0xFC})
	if _, ok := b.monsters[0x0110]; ok || b.Stats.Hits.Load() != 1 || b.Stats.Kills.Load() != 1 {
		t.Errorf("kill: monsters %v hits %d kills %d", b.monsters, b.Stats.Hits.Load(), b.Stats.Kills.Load())
	}
	b.handle([]byte{0xC1, 0x06, 0x14, 1, 0x02, 0x20})
	if len(b.monsters) != 0 {
		t.Errorf("destroy left %v", b.monsters)
	}
	b.handle([]byte{0xC1, 0x08, 0x17, 0x53, 0xFC, 0, 0x01, 0x10})
	if !b.dead || b.Stats.Deaths.Load() != 1 {
		t.Error("death not seen")
	}
	rs := make([]byte, 0x24)
	rs[0], rs[1], rs[2], rs[3], rs[4], rs[5], rs[6] = 0xC3, 0x24, 0xF3, 0x04, 140, 130, 0
	b.handle(rs)
	if b.dead || b.X != 140 || b.Y != 130 || b.homeX != 140 {
		t.Errorf("respawn: dead %v at %d,%d", b.dead, b.X, b.Y)
	}
}

func TestPathStopsAtBlockedCells(t *testing.T) {
	tr := &gamedata.Terrain{Width: 256, Height: 256, Attr: make([]byte, 256*256)}
	tr.Set(103, 100, gamedata.AttrBlock)
	b := newTestBot()
	b.Terrain = tr
	if got := b.path(110, 100, 5); !bytes.Equal(got, []byte{3, 3}) {
		t.Errorf("path = %v", got)
	}
	tr.Set(101, 101, gamedata.AttrSafeZone)
	if got := b.path(105, 105, 3); len(got) != 0 {
		t.Errorf("path into safe zone = %v", got)
	}
	if got := b.path(100, 100, 3); len(got) != 0 {
		t.Errorf("path to self = %v", got)
	}
}

// fakeServer answers one connection with the given plain C1 packets.
func fakeServer(t *testing.T, handle func(nc net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		defer nc.Close()
		handle(nc)
	}()
	return ln.Addr().String()
}

func TestGameServerAddress(t *testing.T) {
	addr := fakeServer(t, func(nc net.Conn) {
		nc.Write([]byte{0xC1, 0x04, 0x00, 0x01})
		buf := make([]byte, 4)
		io.ReadFull(nc, buf) // F4 06
		// Answer split across writes to exercise the reassembly.
		list := frame.Build(frame.C2, 0xF4, 0x06, 0, 1, 0, 0, 0, 0xCC)
		nc.Write(list[:3])
		nc.Write(list[3:])
		io.ReadFull(nc, buf[:3]) // F4 03 code (5 bytes, header read below)
		io.ReadFull(nc, buf[:2])
		info := append([]byte{0xC1, 0x16, 0xF4, 0x03}, []byte("10.0.0.1\x00\x00\x00\x00\x00\x00\x00\x00")...)
		nc.Write(append(info, 0x0C, 0xDA))
	})
	got, err := GameServerAddress(addr, 0, "", 2*time.Second)
	if err != nil || got != "10.0.0.1:55820" {
		t.Fatalf("= %s %v", got, err)
	}
}

func TestBotLoginRefused(t *testing.T) {
	addr := fakeServer(t, func(nc net.Conn) {
		nc.Write(frame.Build(frame.C1, 0xF1, 0x00, 0)) // F1 00 result 0
		time.Sleep(200 * time.Millisecond)
	})
	b := &Bot{Addr: addr, Account: "x", Char: "y", Timeout: time.Second}
	if err := b.Run(t.Context()); err == nil || b.Stats.LoginFail.Load() != 1 {
		t.Fatalf("Run = %v, fails %d", err, b.Stats.LoginFail.Load())
	}
}
