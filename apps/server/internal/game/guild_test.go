package game

import (
	"bytes"
	"context"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// pump waits for the async work of the world and runs its continuations.
func (b *bareWorld) pump() {
	for {
		b.wg.Wait()
		select {
		case f := <-b.events:
			f()
		default:
			return
		}
	}
}

func name10(s string) []byte { b := make([]byte, 10); copy(b, s); return b }

// guildSetup creates guild name with master m through the 55 handler.
func guildSetup(t *testing.T, b *bareWorld, m *Object, name string) int {
	t.Helper()
	body := append([]byte{0}, []byte(name)...)
	body = append(body, make([]byte, 8-len(name))...)
	body = append(body, bytes.Repeat([]byte{0x44}, 32)...)
	b.hooks.worldHandlers[0x55](b.World, m, frame.Build(frame.C1, 0x55, body...))
	b.pump()
	if r := find(b.drain(m), 0x56, -1); r == nil || r[3] != 1 {
		t.Fatalf("create %s = % X", name, r)
	}
	return guildNumberOf(m)
}

func TestGuildCreateJoinListLeave(t *testing.T) {
	b := newBareWorld(t)
	m := b.player(PlayerStart, "Master", 0, 130, 130)
	a := b.player(PlayerStart+1, "Ann", 0, 131, 130)
	low := b.player(PlayerStart+2, "Low", 0, 132, 130)
	low.P.Char.Level = 5
	b.see(m, a)

	// NPC 241: level 400 >= 150 -> 54; 54 [3]=1 -> 55 window.
	w := b.World
	w.guildMasterTalk(m)
	if find(b.drain(m), 0x54, -1) == nil || items(m).If.Type != ifGuildCreate {
		t.Fatal("no 54")
	}
	b.hooks.worldHandlers[0x54](w, m, []byte{0xC1, 4, 0x54, 1})
	if find(b.drain(m), 0x55, -1) == nil {
		t.Fatal("no 55 window")
	}
	// Bad names: syntax 5, length 2.
	b.hooks.worldHandlers[0x55](w, m, frame.Build(frame.C1, 0x55, append([]byte{0, 'a', ' ', 'b', 0, 0, 0, 0, 0}, make([]byte, 32)...)...))
	b.hooks.worldHandlers[0x55](w, m, frame.Build(frame.C1, 0x55, append([]byte{0, 'a', 'b', 0, 0, 0, 0, 0, 0}, make([]byte, 32)...)...))
	if out := b.drain(m); len(out) != 2 || out[0][3] != 5 || out[1][3] != 2 {
		t.Fatalf("bad names = % X", out)
	}
	n := guildSetup(t, b, m, "Lions")
	if n == 0 || guildOf(m).Status != store.GuildMasterStatus || guildOf(m).Name != "Lions" {
		t.Fatalf("master state = %+v", guildOf(m))
	}
	if find(b.drain(a), 0x65, -1) == nil {
		t.Fatal("observer did not get C2 65")
	}
	// Same name again -> 3.
	b.hooks.worldHandlers[0x55](w, a, frame.Build(frame.C1, 0x55, append([]byte{0, 'L', 'i', 'o', 'n', 's', 0, 0, 0}, make([]byte, 32)...)...))
	b.pump()
	if r := find(b.drain(a), 0x56, -1); r[3] != 3 {
		t.Fatalf("duplicate = % X", r)
	}

	// Level 5 player asks to join -> 7.
	b.hooks.worldHandlers[0x50](w, low, []byte{0xC1, 5, 0x50, byte(m.Index >> 8), byte(m.Index)})
	if r := find(b.drain(low), 0x51, -1); r[3] != 7 {
		t.Fatalf("low level = % X", r)
	}
	// Ann asks, the master gets 50 and accepts with 51.
	b.hooks.worldHandlers[0x50](w, a, []byte{0xC1, 5, 0x50, byte(m.Index >> 8), byte(m.Index)})
	if r := find(b.drain(m), 0x50, -1); r == nil || int(r[3])<<8|int(r[4]) != a.Index {
		t.Fatalf("50 = % X", r)
	}
	b.hooks.worldHandlers[0x51](w, m, []byte{0xC1, 6, 0x51, 1, byte(a.Index >> 8), byte(a.Index)})
	b.pump()
	out := b.drain(a)
	if r := find(out, 0x51, -1); r == nil || r[3] != 1 || guildNumberOf(a) != n || guildOf(a).Status != 0 {
		t.Fatalf("join = % X state %+v", r, guildOf(a))
	}
	if items(a).If.Use || items(m).If.Use {
		t.Fatal("interface not cleared")
	}
	// Member list.
	b.hooks.worldHandlers[0x52](w, a, []byte{0xC1, 3, 0x52})
	l := find(b.drain(a), 0x52, -1)
	if l == nil || l[5] != 2 || string(l[24:30]) != "Master" || l[24+11] != 0x80 || l[24+12] != 0x80 ||
		string(l[37:40]) != "Ann" || l[37+12] != 0 {
		t.Fatalf("list = % X", l)
	}

	// Ranks: assistant, a second assistant is refused, self is refused.
	assign := func(st byte, who string) {
		b.hooks.worldHandlers[0xE1](w, m, frame.Build(frame.C1, 0xE1, append([]byte{1, st}, name10(who)...)...))
		b.pump()
	}
	assign(0x40, "Ann")
	if r := find(b.drain(m), 0xE1, -1); r[4] != 1 || guildOf(a).Status != 0x40 {
		t.Fatalf("assign = % X %d", r, guildOf(a).Status)
	}
	if find(b.drain(a), 0x0D, -1) == nil {
		t.Fatal("no rank message")
	}
	assign(0x40, "Ann")
	if r := find(b.drain(m), 0xE1, -1); r[4] != 0x12 {
		t.Fatalf("second assistant = % X", r)
	}
	assign(0x20, "Master")
	if r := find(b.drain(m), 0xE1, -1); r[4] != 0x11 {
		t.Fatalf("self = % X", r)
	}

	// Guild chat "@" reaches both members; the notice "@>" too.
	w.guildChat(a, 0, []byte("hi all"))
	if c := find(b.drain(m), 0x00, -1); c == nil || string(c[3:6]) != "Ann" || string(c[13:20]) != "@hi all" {
		t.Fatalf("guild chat = % X", c)
	}
	w.guildChat(m, 2, []byte("it's war"))
	b.pump()
	if c := find(b.drain(a), 0x0D, -1); c == nil || c[3] != NoticeGuild || string(c[13:21]) != "it\"s war" {
		t.Fatalf("notice = % X", c)
	}
	if g, _ := w.guildMgr().ByNumber(n); g.Notice != "it\"s war" {
		t.Fatalf("stored notice %q", g.Notice)
	}

	// Ann leaves (53 with her own name).
	b.advance(1e9)
	leave := func(o *Object, who string) {
		b.hooks.worldHandlers[0x53](w, o, frame.Build(frame.C1, 0x53, append(name10(who), make([]byte, 10)...)...))
		b.pump()
	}
	leave(a, "Master")
	if r := find(b.drain(a), 0x53, -1); r[3] != 0 {
		t.Fatalf("member kicking master = % X", r)
	}
	// A locked account cannot leave (message 0x6F2, FUN_00483ed0).
	b.advance(1e9)
	a.P.Account.Lock = 1234
	leave(a, "Ann")
	if r := find(b.drain(a), 0x53, -1); r == nil || r[3] != 0 || guildNumberOf(a) == 0 {
		t.Fatalf("locked leave = % X", r)
	}
	a.P.Account.Lock = 0
	b.advance(1e9)
	leave(a, "Ann")
	if r := find(b.drain(a), 0x53, -1); r == nil || r[3] != 1 || guildNumberOf(a) != 0 {
		t.Fatalf("leave = % X", r)
	}
	// The master disbands.
	b.advance(1e9)
	leave(m, "Master")
	out = b.drain(m)
	if r := find(out, 0x53, -1); r == nil || r[3] != 1 || find(out, 0x5D, -1) == nil || guildNumberOf(m) != 0 {
		t.Fatalf("disband = % X", out)
	}
	if _, ok := w.guildMgr().ByNumber(n); ok {
		t.Fatal("guild still exists")
	}
}

func TestGuildUnionAndRival(t *testing.T) {
	b := newBareWorld(t)
	w := b.World
	m1 := b.player(PlayerStart, "MasterA", 0, 130, 130)
	m2 := b.player(PlayerStart+1, "MasterB", 0, 131, 130)
	m3 := b.player(PlayerStart+2, "MasterC", 0, 132, 130)
	g1 := guildSetup(t, b, m1, "Alpha")
	g2 := guildSetup(t, b, m2, "Beta")
	g3 := guildSetup(t, b, m3, "Gamma")
	b.see(m1, m2)
	b.see(m2, m3)

	ask := func(from, to *Object, rel, req byte) {
		b.hooks.worldHandlers[0xE5](w, from, []byte{0xC1, 7, 0xE5, rel, req, byte(to.Index >> 8), byte(to.Index)})
	}
	answer := func(from, to *Object, rel, req, yes byte) {
		b.hooks.worldHandlers[0xE6](w, from, []byte{0xC1, 8, 0xE6, rel, req, yes, byte(to.Index >> 8), byte(to.Index)})
		b.pump()
	}
	// Beta asks to join Alpha's union; Alpha accepts.
	ask(m2, m1, 1, 1)
	if r := find(b.drain(m1), 0xE5, -1); r == nil || r[3] != 1 || r[4] != 1 {
		t.Fatalf("E5 = % X", r)
	}
	answer(m1, m2, 1, 1, 1)
	if r := find(b.drain(m2), 0xE6, -1); r == nil || r[5] != 1 {
		t.Fatalf("E6 = % X", r)
	}
	ga, _ := w.guildMgr().ByNumber(g1)
	gb, _ := w.guildMgr().ByNumber(g2)
	if ga.Union != g1 || gb.Union != g1 || w.guildMgr().Relation(g2, g1) != 1 || w.guildMgr().Relation(g1, g2) != 1 {
		t.Fatalf("union %d %d", ga.Union, gb.Union)
	}
	// C2 67 of Beta's master names the union "Alpha".
	if u := find(b.drain(m1), 0x67, -1); u == nil {
		t.Fatal("no 67")
	}
	e, _ := w.unionEntry(m2, m1)
	if e.Union != "Alpha" || e.Relation != 1 {
		t.Fatalf("union entry = %+v", e)
	}
	// E9 lists both guilds.
	b.hooks.worldHandlers[0xE9](w, m2, []byte{0xC1, 3, 0xE9})
	if l := find(b.drain(m2), 0xE9, -1); l == nil || l[4] != 2 || l[5] != 1 || string(l[8+33:8+38]) != "Alpha" {
		t.Fatalf("E9 = % X", l)
	}
	// Gamma declares hostility to Beta: refused (Beta is not a master of
	// a union... Beta is a member; relation 2 needs no union membership).
	ask(m3, m2, 2, 1)
	answer(m2, m3, 2, 1, 1)
	if r := find(b.drain(m3), 0xE6, -1); r == nil || r[5] != 0 {
		t.Fatalf("rival with a union member = % X", r)
	}
	// Gamma and Alpha (union master) become rivals.
	b.see(m1, m3)
	ask(m3, m1, 2, 1)
	answer(m1, m3, 2, 1, 1)
	if r := find(b.drain(m3), 0xE6, -1); r == nil || r[5] != 1 {
		t.Fatalf("rival = % X", r)
	}
	if w.guildMgr().Relation(g2, g3) != 2 || w.guildMgr().Relation(g3, g2) != 2 {
		t.Fatalf("rival relation %d %d", w.guildMgr().Relation(g2, g3), w.guildMgr().Relation(g3, g2))
	}
	// Rival kills give no PK.
	killer := m3
	before := killer.PKLevel
	b.hooks.playerKill(w, killer, m2)
	if killer.PKLevel != before {
		t.Fatal("PK counted against a rival")
	}
	// Alpha kicks Beta from the union: the union is dissolved (2 guilds).
	b.hooks.worldHandlers[0xEB](w, m1, frame.Build(frame.C1, 0xEB, append([]byte{1}, []byte("Beta\x00\x00\x00\x00")...)...))
	b.pump()
	if r := find(b.drain(m1), 0xEB, -1); r == nil || r[4] != 1 {
		t.Fatalf("EB = % X", r)
	}
	ga, _ = w.guildMgr().ByNumber(g1)
	gb, _ = w.guildMgr().ByNumber(g2)
	if ga.Union != 0 || gb.Union != 0 {
		t.Fatalf("union after kick %d %d", ga.Union, gb.Union)
	}
	// Rivalry broken off by Alpha (E5 2/2, answered by Gamma).
	ask(m1, m3, 2, 2)
	answer(m3, m1, 2, 2, 1)
	ga, _ = w.guildMgr().ByNumber(g1)
	if ga.Rival != 0 {
		t.Fatal("rival not cleared")
	}
}

func TestGuildWar(t *testing.T) {
	b := newBareWorld(t)
	w := b.World
	m1 := b.player(PlayerStart, "MasterA", 0, 130, 130)
	m2 := b.player(PlayerStart+1, "MasterB", 0, 131, 130)
	a2 := b.player(PlayerStart+2, "Bee", 0, 132, 130)
	g1 := guildSetup(t, b, m1, "Alpha")
	g2 := guildSetup(t, b, m2, "Beta")
	b.hooks.worldHandlers[0x50](w, a2, []byte{0xC1, 5, 0x50, byte(m2.Index >> 8), byte(m2.Index)})
	b.hooks.worldHandlers[0x51](w, m2, []byte{0xC1, 6, 0x51, 1, byte(a2.Index >> 8), byte(a2.Index)})
	b.pump()
	b.drain(m1)
	b.drain(m2)

	if w.guildWarRequest(m1, "Nobody", 0) {
		t.Fatal("war against a missing guild")
	}
	if r := find(b.drain(m1), 0x60, -1); r[3] != 0 {
		t.Fatalf("missing guild = % X", r)
	}
	if !w.guildWarRequest(m1, "Beta", 0) {
		t.Fatal("request refused")
	}
	if r := find(b.drain(m1), 0x60, -1); r[3] != 1 {
		t.Fatalf("60 = % X", r)
	}
	if r := find(b.drain(m2), 0x61, -1); r == nil || string(r[3:8]) != "Alpha" {
		t.Fatalf("61 = % X", r)
	}
	b.hooks.worldHandlers[0x61](w, m2, []byte{0xC1, 4, 0x61, 1})
	if r := find(b.drain(a2), 0x62, -1); r == nil || string(r[3:8]) != "Alpha" || r[12] != 0 {
		t.Fatalf("62 to Beta = % X", r)
	}
	if r := find(b.drain(m1), 0x62, -1); r == nil || string(r[3:7]) != "Beta" || r[12] != 1 {
		t.Fatalf("62 to Alpha = % X", r)
	}
	if !w.atWar(m1, a2) || !w.atWar(a2, m1) {
		t.Fatal("not at war")
	}
	// A war kill gives no PK; killing the master scores KillScore2 (2).
	before := m1.PKLevel
	b.hooks.playerKill(w, m1, a2)
	if m1.PKLevel != before {
		t.Fatal("PK in war")
	}
	if !w.guildWarKill(m1, a2) || w.warScore(g1) != 1 {
		t.Fatalf("score = %d", w.warScore(g1))
	}
	if s := find(b.drain(m1), 0x64, -1); s == nil || s[3] != 1 || s[4] != 0 {
		t.Fatalf("64 = % X", s)
	}
	w.guildWarKill(m1, m2)
	if w.warScore(g1) != 3 {
		t.Fatalf("master kill score = %d", w.warScore(g1))
	}
	// Up to GuildWarScoreMax1 (20): the winner (enemy 0 points) gets
	// GuildWarWinnerScore1 (3) and both get 63.
	for w.warScore(g1) < 19 {
		w.guildWarKill(m1, a2)
	}
	w.guildWarKill(m1, a2)
	b.pump()
	if r := find(b.drain(m1), 0x63, -1); r == nil || r[3] != 1 || string(r[4:8]) != "Beta" {
		t.Fatalf("63 winner = % X", r)
	}
	if r := find(b.drain(a2), 0x63, -1); r == nil || r[3] != 0 {
		t.Fatalf("63 loser = % X", r)
	}
	if g, _ := w.guildMgr().ByNumber(g1); g.Score != 3 {
		t.Fatalf("G_Score = %d", g.Score)
	}
	b.advance(3e9)
	w.warTick()
	if w.guildWarOf(g1) != nil || w.guildWarOf(g2) != nil {
		t.Fatal("war not cleared")
	}
}

func TestGuildEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	acc, _ := env.store.AccountByLogin(ctx, "tester")
	for _, n := range []string{"Boss", "Mate"} {
		env.store.CreateCharacter(ctx, acc.ID, n, 0x10)
		ch, _ := env.store.LoadCharacter(ctx, acc.ID, n)
		ch.Level = 200
		env.store.SaveCharacter(ctx, ch)
	}
	// The world loaded its guilds at start-up: create this one through the
	// manager, which writes the rows and keeps it in memory.
	m := env.world.guildMgr()
	if r, _ := m.Create(ctx, "Crew", "Boss", [32]byte{}, 0, -1); r != 1 {
		t.Fatalf("create = %d", r)
	}
	if r := m.AddMember(ctx, "Crew", "Mate", -1); r != 1 {
		t.Fatalf("add member = %d", r)
	}

	c := env.dial(t)
	c.next(0xF1, 0x00)
	c.verify()
	if r := c.login("tester", "pass"); r != 1 {
		t.Fatalf("login %d", r)
	}
	c.send(frame.Build(frame.C1, 0xF3, 0x00))
	list := c.next(0xF3, 0x00)
	if list[6] != 2 {
		t.Fatalf("char list = % X", list)
	}
	for i := 0; i < int(list[6]); i++ {
		e := list[7+34*i:]
		if string(e[1:5]) == "Boss" && e[33] != 0x80 {
			t.Fatalf("G_Status in list = % X", e)
		}
	}
	c.send(frame.Build(frame.C1, 0xF3, append([]byte{0x03}, name10("Boss")...)...))
	c.next(0xF3, 0x03)
	v := c.next(0x65, -1)
	if v[4] != 1 || v[9] != 0x80 || v[12]&0x80 == 0 {
		t.Fatalf("own 65 = % X", v)
	}
	c.next(0x67, -1)
	c.send(frame.Build(frame.C1, 0x52))
	l := c.next(0x52, -1)
	if l[5] != 2 || string(l[24:28]) != "Boss" || l[24+11] != 0x80 || string(l[37:41]) != "Mate" || l[37+10] != 0xFF {
		t.Fatalf("52 = % X", l)
	}
	c.send(frame.Build(frame.C1, 0x00, append(name10("Boss"), []byte("@hello")...)...))
	if ch := c.next(0x00, -1); string(ch[13:19]) != "@hello" {
		t.Fatalf("guild chat = % X", ch)
	}
}

// TestGuildAcrossInstances: a member on another instance of the process
// gets the guild chat, the notice and the member list shows its server.
func TestGuildAcrossInstances(t *testing.T) {
	b1 := newBareWorld(t)
	b2 := newBareWorld(t)
	b2.cfg.ServerCode = 19
	b2.hub = b1.hub
	b1.hub.register(b2.World)
	m := b1.player(PlayerStart, "Master", 0, 130, 130)
	n := guildSetup(t, b1, m, "Lions")
	b1.guildMgr().AddMember(context.Background(), "Lions", "Far", -1)
	far := b2.player(PlayerStart, "Far", 0, 130, 130)
	b2.guildEnter(far)
	b1.pump()
	b2.pump()
	if guildNumberOf(far) != n || find(b2.drain(far), 0x65, -1) == nil {
		t.Fatal("remote member not set")
	}
	b1.hooks.worldHandlers[0x52](b1.World, m, []byte{0xC1, 3, 0x52})
	l := find(b1.drain(m), 0x52, -1)
	if l == nil || l[5] != 2 || string(l[37:40]) != "Far" || l[37+10] != 19 || l[37+11] != 0x80|19 {
		t.Fatalf("52 = % X", l)
	}
	b1.guildChat(m, 0, []byte("yo"))
	b2.pump()
	if c := find(b2.drain(far), 0x00, -1); c == nil || string(c[13:16]) != "@yo" {
		t.Fatalf("remote chat = % X", c)
	}
	b2.guildLeave(far)
	if g, _ := b1.guildMgr().ByNumber(n); g.Members[1].Server != -1 {
		t.Fatal("still online")
	}
}
