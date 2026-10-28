package game

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func chatPacket(head byte, name, msg string) []byte {
	body := make([]byte, 10)
	copy(body, name)
	body = append(body, msg...)
	body = append(body, 0)
	return frame.Build(frame.C1, head, body...)
}

// noticeText extracts the text of a C1 0D notice.
func noticeText(p []byte) string { return string(cstr(p[13:])) }

func TestFilterText(t *testing.T) {
	msg := []byte("you fuck sex fuckfuck Sex\x00fuck")
	filterText(msg, []string{"fuck", "sex"})
	if got := string(msg); got != "you **** *** ******** Sex\x00fuck" {
		t.Fatalf("filtered %q", got)
	}
}

func TestCommandToken(t *testing.T) {
	if got := commandToken("/move Lorencia", 0, 0x20); got != "/move" {
		t.Fatalf("token 0 = %q", got)
	}
	if got := commandToken("/move Lorencia", 1, 0x20); got != "Lorencia" {
		t.Fatalf("token 1 = %q", got)
	}
	if got := commandToken("/a  b", 2, 0x20); got != "b" {
		t.Fatalf("double space token = %q", got)
	}
	if got := commandToken("/"+strings.Repeat("x", 40), 0, 0x20); len(got) != 31 {
		t.Fatalf("token not cut at 31: %d", len(got))
	}
}

func TestChatPacketLayouts(t *testing.T) {
	p := packetChat([]byte("Alpha\x00\x00\x00\x00\x00"), []byte("hi"))
	if len(p) != 0x49 || p[0] != 0xC1 || p[1] != 0x49 || p[2] != 0 || string(p[3:8]) != "Alpha" || string(p[13:15]) != "hi" {
		t.Fatalf("C1 49 00 = % X", p)
	}
	wp := packetWhisper("Alpha", []byte("hello\x00junk"))
	if !bytes.Equal(wp, append([]byte{0xC1, 0x13, 0x02, 'A', 'l', 'p', 'h', 'a', 0, 0, 0, 0, 0}, "hello\x00"...)) {
		t.Fatalf("C1 02 = % X", wp)
	}
	if long := packetWhisper("A", bytes.Repeat([]byte{'x'}, 80)); len(long) != 0x3B+0x0E {
		t.Fatalf("whisper not cut at 59: %d", len(long))
	}
	if !bytes.Equal(packetWhisperNotFound(), []byte{0xC1, 0x04, 0x0C, 0x00}) {
		t.Fatal("C1 04 0C 00")
	}
	cn := packetCustomNotice(2, 1, "a: b")
	if len(cn) != 0x86 || cn[2] != 0xF3 || cn[3] != 0xE4 || cn[4] != 2 || cn[5] != 1 || string(cn[6:10]) != "a: b" {
		t.Fatalf("F3 E4 = % X", cn[:12])
	}
}

func TestChatBroadcastAndFilter(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 2)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x, y+3)
	b.see(a, c)
	b.handleChat(a, chatPacket(0x00, "Alpha", "hello fuck"))
	want := packetChat([]byte("Alpha\x00\x00\x00\x00\x00"), []byte("hello ****"))
	if p := find(b.drain(a), 0x00, -1); !bytes.Equal(p, want) {
		t.Fatalf("self chat = % X", p)
	}
	if p := find(b.drain(c), 0x00, -1); !bytes.Equal(p, want) {
		t.Fatalf("viewer chat = % X", p)
	}
	// The name must be the sender's own.
	b.handleChat(a, chatPacket(0x00, "Beta", "spoof"))
	if len(b.drain(c)) != 0 {
		t.Fatal("spoofed name accepted")
	}
	// Maps with ChatEnable 0 drop everything.
	info := b.data.Maps[0]
	info.ChatEnable = 0
	b.data.Maps[0] = info
	b.handleChat(a, chatPacket(0x00, "Alpha", "x"))
	if len(b.drain(c)) != 0 {
		t.Fatal("chat on a ChatEnable 0 map")
	}
}

func TestChatLimitTime(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	a.P.Char.ChatLimitTime = 1
	b.handleChat(a, chatPacket(0x00, "Alpha", "hi"))
	pk := b.drain(a)
	if find(pk, 0x00, -1) != nil {
		t.Fatal("chat while banned")
	}
	if p := find(pk, 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 269, 1) {
		t.Fatalf("269 notice = % X", p)
	}
	for i := 0; i < 61; i++ {
		b.chatLimitSecond()
	}
	if a.P.Char.ChatLimitTime != 0 {
		t.Fatalf("ChatLimitTime %d after 61 s", a.P.Char.ChatLimitTime)
	}
	if p := find(b.drain(a), 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 283) {
		t.Fatal("no 283 notice")
	}
}

func TestChatPrefixes(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 2)
	a := b.player(PlayerStart, "Alpha", 0, x, y)
	c := b.player(PlayerStart+1, "Beta", 0, x+50, y)
	// "!" without authority is plain chat; with authority > 3 a gold
	// notice to every player.
	b.handleChat(a, chatPacket(0x00, "Alpha", "!hey"))
	if find(b.drain(c), 0x0D, -1) != nil {
		t.Fatal("notice without authority")
	}
	a.Move.Authority = 8
	b.handleChat(a, chatPacket(0x00, "Alpha", "!hey"))
	if p := find(b.drain(c), 0x0D, -1); p == nil || p[3] != NoticeGold || noticeText(p) != "hey" {
		t.Fatalf("global notice = % X", p)
	}
	// "~" goes to the party.
	b.hooks.partyMembers = func(*World, *Object) []int { return []int{PlayerStart, PlayerStart + 1} }
	b.drain(a)
	b.handleChat(a, chatPacket(0x00, "Alpha", "~party"))
	if p := find(b.drain(c), 0x00, -1); p == nil || string(cstr(p[13:])) != "~party" {
		t.Fatalf("party chat = % X", p)
	}
	// "@" routes to the guild module.
	var kinds []int
	b.hooks.guildChat = func(_ *World, _ *Object, kind int, msg []byte) { kinds = append(kinds, kind) }
	b.handleChat(a, chatPacket(0x00, "Alpha", "@g"))
	b.handleChat(a, chatPacket(0x00, "Alpha", "@@u"))
	b.handleChat(a, chatPacket(0x00, "Alpha", "@>n'o"))
	if len(kinds) != 3 || kinds[0] != 0 || kinds[1] != 1 || kinds[2] != 2 {
		t.Fatalf("guild kinds %v", kinds)
	}
}

func TestWhisper(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	c := b.player(PlayerStart+1, "Beta", 3, 170, 120)
	b.handleWhisper(a, chatPacket(0x02, "Beta", "psst"))
	if p := find(b.drain(c), 0x02, -1); !bytes.Equal(p, packetWhisper("Alpha", []byte("psst"))) {
		t.Fatalf("whisper = % X", p)
	}
	b.handleWhisper(a, chatPacket(0x02, "Nobody", "x"))
	if p := find(b.drain(a), 0x0C, -1); !bytes.Equal(p, []byte{0xC1, 0x04, 0x0C, 0x00}) {
		t.Fatalf("not found = % X", p)
	}
	b.handleWhisper(a, chatPacket(0x02, "Alpha", "me"))
	if p := find(b.drain(a), 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 270) {
		t.Fatal("self whisper notice")
	}
	var relayed string
	b.hooks.globalWhisper = func(_ *World, _ *Object, target string, msg []byte) bool {
		relayed = target + ":" + string(msg)
		return true
	}
	b.handleWhisper(a, chatPacket(0x02, "Remote", "yo"))
	if relayed != "Remote:yo" || find(b.drain(a), 0x0C, -1) != nil {
		t.Fatalf("relay %q", relayed)
	}
}

func TestCommandMove(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 3, 170, 120)
	a.P.Char.Level = 5
	a.P.Char.Money = 5000
	b.handleChat(a, chatPacket(0x00, "Alpha", "/move Lorencia"))
	if p := find(b.drain(a), 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 224, 10) {
		t.Fatalf("level notice = %q", noticeText(p))
	}
	a.P.Char.Level = 10
	b.handleChat(a, chatPacket(0x00, "Alpha", "/move lorencia")) // names are case-insensitive
	pk := b.drain(a)
	if a.Map != 0 || a.X < 133 || a.X > 150 || a.Y < 118 || a.Y > 134 {
		t.Fatalf("moved to map %d %d,%d", a.Map, a.X, a.Y)
	}
	if a.P.Char.Money != 4000 {
		t.Fatalf("money %d", a.P.Char.Money)
	}
	if p := find(pk, 0x22, 0xFE); p == nil || p[4] != 0 || p[7] != 0xA0 || p[6] != 0x0F {
		t.Fatalf("money packet = % X", p)
	}
}

func TestCommandRequirements(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	c := b.moveData.commands["/move"]
	c.Delay, c.MinLevel = 10, -1
	b.moveData.commands["/move"] = c
	b.handleChat(a, chatPacket(0x00, "Alpha", "/move Noria"))
	b.drain(a)
	a.Move.Regen = 0
	b.advance(3 * time.Second)
	b.handleChat(a, chatPacket(0x00, "Alpha", "/move Noria"))
	if p := find(b.drain(a), 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 300, 7, "/move") {
		t.Fatalf("delay notice %q", noticeText(p))
	}
	// GM-only commands.
	b.handleChat(a, chatPacket(0x00, "Alpha", "/gmove x"))
	if p := find(b.drain(a), 0x0D, -1); p == nil || noticeText(p) != b.messageFor(a, 301, "/gmove") {
		t.Fatalf("gm notice %q", noticeText(p))
	}
}

func TestCommandPost(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Alpha", 0, 130, 130)
	c := b.player(PlayerStart+1, "Beta", 2, 30, 30)
	b.handleChat(a, chatPacket(0x00, "Alpha", "/post hello all"))
	want := packetWhisper("Alpha", []byte(strings.Replace(b.messageFor(c, 323), "%s", "hello all", 1)))
	if p := find(b.drain(c), 0x02, -1); !bytes.Equal(p, want) {
		t.Fatalf("post = % X\nwant % X", p, want)
	}
}

func TestEndToEndChat(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x00, 0, 135, 125, 20)
	c := enterAt(t, env, "second", "Beta", 0x00, 0, 137, 126, 20)
	waitSees(t, c, PlayerStart)
	a.send(chatPacket(0x00, "Alpha", "hello Beta"))
	p := c.next(0x00, -1)
	if p[1] != 0x49 || string(cstr(p[3:13])) != "Alpha" || string(cstr(p[13:])) != "hello Beta" {
		t.Fatalf("Beta got % X", p)
	}
	a.send(chatPacket(0x02, "Beta", "secret"))
	if p := c.next(0x02, -1); string(cstr(p[3:13])) != "Alpha" || string(cstr(p[13:])) != "secret" {
		t.Fatalf("whisper % X", p)
	}
}
