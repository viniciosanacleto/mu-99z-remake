package game

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestDuelPacketLayouts(t *testing.T) {
	o := &Object{Index: 0x5401, P: &Player{}}
	o.P.Char = nil
	if p := packetDuelStart(1, o); !bytes.Equal(p[:6], []byte{0xC1, 0x10, 0xAA, 1, 0x54, 0x01}) || len(p) != 16 {
		t.Fatalf("AA % X", p)
	}
	if p := packetDuelEnd(o); !bytes.Equal(p[:5], []byte{0xC1, 0x0F, 0xAB, 0x54, 0x01}) || len(p) != 15 {
		t.Fatalf("AB % X", p)
	}
	if p := packetDuelAsk(o); !bytes.Equal(p[:5], []byte{0xC1, 0x0F, 0xAC, 0x54, 0x01}) || len(p) != 15 {
		t.Fatalf("AC % X", p)
	}
	a := &Object{Index: 0x5400, Duel: &duelState{Score: 3}}
	if p := packetDuelScore(a, o); !bytes.Equal(p, []byte{0xC1, 9, 0xAD, 0x54, 0, 0x54, 1, 3, 0}) {
		t.Fatalf("AD % X", p)
	}
}

func duelRequest(a, t *Object) []byte {
	n := make([]byte, 10)
	copy(n, objName(t))
	return frame.Build(frame.C1, 0xAA, append([]byte{byte(t.Index >> 8), byte(t.Index)}, n...)...)
}

func duelAnswer(ok byte, r *Object) []byte {
	n := make([]byte, 10)
	copy(n, objName(r))
	return frame.Build(frame.C1, 0xAC, append([]byte{ok, byte(r.Index >> 8), byte(r.Index)}, n...)...)
}

func TestDuelFlowAndMaxScore(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	a.P.Char.Life, a.P.Char.MaxLife = 1000, 1000
	b.handleDuelRequest(a, duelRequest(a, v))
	if ask := find(b.drain(v), 0xAC, -1); ask == nil || int(ask[3])<<8|int(ask[4]) != a.Index || cString(ask[5:15]) != "Alpha" {
		t.Fatalf("AC % X", ask)
	}
	// A second request while busy is refused.
	b.handleDuelRequest(a, duelRequest(a, v))
	if find(b.drain(v), 0xAC, -1) != nil {
		t.Fatal("second request")
	}
	b.handleDuelAnswer(v, duelAnswer(1, a))
	pa, pv := b.drain(a), b.drain(v)
	if s := find(pa, 0xAA, -1); s == nil || s[3] != 1 || int(s[4])<<8|int(s[5]) != v.Index {
		t.Fatalf("AA to requester % X", s)
	}
	if s := find(pv, 0xAA, -1); s == nil || s[3] != 1 || int(s[4])<<8|int(s[5]) != a.Index {
		t.Fatalf("AA to answerer % X", s)
	}
	if find(pv, 0xAD, -1) == nil || !b.inDuelWith(a, v) {
		t.Fatal("not dueling")
	}
	// DuelMaxScore 10: ten kills end the duel with AB and the firework.
	fixRand(t, 0)
	for i := 1; i <= 10; i++ {
		v.Alive, v.Live, v.P.Char.Life = true, LiveNormal, 1
		b.attack(a, v, 0, false)
		if i < 10 && dl(a).Score != i {
			t.Fatalf("kill %d score %d", i, dl(a).Score)
		}
		delete(b.mw().deaths, v.Index)
	}
	pa = b.drain(a)
	if b.inDuelWith(a, v) || find(pa, 0xAB, -1) == nil || find(pa, 0xF3, 0x40) == nil {
		t.Fatalf("duel not ended: %v", b.inDuelWith(a, v))
	}
	// FUN_00476bc0: a win for the winner, a loss for the loser.
	b.runAsync()
	rs := b.store.(store.RankingStore)
	if w, l, _ := rs.DuelRanking(context.Background(), "Alpha"); w != 1 || l != 0 {
		t.Fatalf("winner ranking %d %d", w, l)
	}
	if w, l, _ := rs.DuelRanking(context.Background(), objName(v)); w != 0 || l != 1 {
		t.Fatalf("loser ranking %d %d", w, l)
	}
	// No death penalty in a duel.
	if v.P.Char.Money != 1000000 {
		t.Fatalf("money %d", v.P.Char.Money)
	}
}

func TestDuelRefusedAndTimeouts(t *testing.T) {
	b := newBareWorld(t)
	a, v := pvpPair(t, b, 0, 1)
	b.handleDuelRequest(a, duelRequest(a, v))
	b.handleDuelAnswer(v, duelAnswer(0, a))
	if s := find(b.drain(a), 0xAA, -1); s == nil || s[3] != 0 || !duelIdle(a) || !duelIdle(v) {
		t.Fatalf("refusal % X", s)
	}
	b.handleDuelRequest(a, duelRequest(a, v))
	b.advance(11 * time.Second)
	b.duelSecond()
	if s := find(b.drain(a), 0xAA, -1); s == nil || s[3] != 0 || !duelIdle(a) {
		t.Fatalf("request timeout % X", s)
	}
	b.handleDuelRequest(a, duelRequest(a, v))
	b.handleDuelAnswer(v, duelAnswer(1, a))
	b.drain(a)
	b.advance(61 * time.Second)
	b.duelSecond()
	if b.inDuelWith(a, v) || find(b.drain(a), 0xAB, -1) == nil {
		t.Fatal("duel did not time out")
	}
	// AB cancels a running duel.
	b.handleDuelRequest(a, duelRequest(a, v))
	b.handleDuelAnswer(v, duelAnswer(1, a))
	b.handleDuelCancel(a, frame.Build(frame.C1, 0xAB))
	if b.inDuelWith(a, v) || find(b.drain(v), 0xAB, -1) == nil {
		t.Fatal("cancel")
	}
}

// HackPacketCheck.txt requires C3 for AA..AC.
func TestEndToEndDuel(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x00, 0, 135, 125, 20)
	c := enterAt(t, env, "second", "Beta", 0x10, 0, 137, 126, 20)
	waitSees(t, a, PlayerStart+1)
	n := make([]byte, 10)
	copy(n, "Beta")
	a.send(frame.Build(frame.C3, 0xAA, append([]byte{byte((PlayerStart + 1) >> 8), byte((PlayerStart + 1) & 0xFF)}, n...)...))
	ask := c.next(0xAC, -1)
	if int(ask[3])<<8|int(ask[4]) != PlayerStart || cString(ask[5:15]) != "Alpha" {
		t.Fatalf("AC % X", ask)
	}
	m := make([]byte, 10)
	copy(m, "Alpha")
	c.send(frame.Build(frame.C3, 0xAC, append([]byte{1, byte(PlayerStart >> 8), byte(PlayerStart & 0xFF)}, m...)...))
	if s := a.next(0xAA, -1); s[3] != 1 || int(s[4])<<8|int(s[5]) != PlayerStart+1 {
		t.Fatalf("AA % X", s)
	}
	if sc := a.next(0xAD, -1); sc[7] != 0 || sc[8] != 0 {
		t.Fatalf("AD % X", sc)
	}
	a.send(frame.Build(frame.C3, 0xAB))
	if e := c.next(0xAB, -1); int(e[3])<<8|int(e[4]) != PlayerStart+1 {
		t.Fatalf("AB % X", e)
	}
}

func TestEndToEndSkillOnMonster(t *testing.T) {
	env := newTestEnv(t)
	var x, y int
	var m *Object
	env.onWorld(func() {
		w := env.world
		x, y = (&bareWorld{World: w, t: t}).openRow(0, 3)
		m = w.monsterByClass(0, 0)
		w.monsterOccupy(m, false)
		m.X, m.Y, m.TX, m.TY = x+2, y, x+2, y
		m.Move = nil
		w.monsterOccupy(m, true)
		m.Mon.Attribute, m.Mon.DefenseRate, m.Mon.Defense, m.Mon.Life = 0, 0, 0, 100000
	})
	c := enterAt(t, env, "tester", "Wiz", 0x00, 0, x, y, 50)
	waitMonster(t, c, m.Index)
	env.onWorld(func() {})
	// Energy Ball (17) is a default DW skill.
	c.send(frame.Build(frame.C3, 0x19, 17, byte(m.Index>>8), byte(m.Index)))
	u := c.next(0x19, -1)
	if u[3] != 17 || int(u[4])<<8|int(u[5]) != PlayerStart || int(u[6]&0x7F)<<8|int(u[7]) != m.Index {
		t.Fatalf("19 % X", u)
	}
	d := c.next(0xD7, -1)
	if int(d[3]&0x7F)<<8|int(d[4]) != m.Index {
		t.Fatalf("D7 % X", d)
	}
	if mana := c.next(0x27, 0xFF); mana == nil {
		t.Fatal("no mana update")
	}
}
