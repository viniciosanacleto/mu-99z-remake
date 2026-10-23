package game

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

func partyReq(target int) []byte {
	return frame.Build(frame.C3, 0x40, byte(target>>8), byte(target))
}

func partyAns(ok byte, requester int) []byte {
	return frame.Build(frame.C3, 0x41, ok, byte(requester>>8), byte(requester))
}

func TestPartyRequestAnswerList(t *testing.T) {
	b := newBareWorld(t)
	a := statsPlayer(b, PlayerStart, 0, 50)
	c := statsPlayer(b, PlayerStart+1, 1, 60)
	b.calcCharacter(a)
	b.calcCharacter(c)
	b.drain(a)
	b.drain(c)
	b.handlePartyRequest(a, partyReq(c.Index))
	if p := find(b.drain(c), 0x40, -1); p == nil || len(p) != 5 || int(p[3])<<8|int(p[4]) != a.Index {
		t.Fatalf("40 to target % X", p)
	}
	b.handlePartyAnswer(c, partyAns(1, a.Index))
	// The leader first gets the one-member list of the new party
	// (FUN_004d2450), then both get the two-member list (FUN_004d25b0).
	la := b.drain(a)
	if p := find(la, 0x42, -1); p == nil || p[4] != 1 {
		t.Fatalf("first list % X", p)
	}
	pa := findLast(la, 0x42)
	pc := find(b.drain(c), 0x42, -1)
	if pa == nil || pc == nil || len(pa) != 5+2*24 || pa[3] != 1 || pa[4] != 2 {
		t.Fatalf("42 % X / % X", pa, pc)
	}
	e0, e1 := pa[5:], pa[29:]
	if string(e0[:5]) != "StatA" || e0[10] != 0 || e0[11] != 0 || int(e0[12]) != a.X || int(e0[13]) != a.Y ||
		binary.LittleEndian.Uint32(e0[20:]) != 109 || string(e1[:5]) != "StatB" || e1[10] != 1 {
		t.Fatalf("entries % X", pa)
	}
	pt := b.partyOf(a)
	if pt == nil || pt.leader() != a.Index || b.partyOf(c) != pt || pt.count != 2 {
		t.Fatal("party state")
	}
	// List request.
	b.handlePartyList(c)
	if p := find(b.drain(c), 0x42, -1); p == nil || p[4] != 2 {
		t.Fatalf("list % X", p)
	}
	// Member HP bar every second: C1 06 44 02 then (life*10/max)&0xF ^ slot<<4.
	c.P.Char.Life = c.P.Char.MaxLife / 2
	b.partySecond()
	if p := find(b.drain(a), 0x44, -1); p == nil || len(p) != 6 || p[3] != 2 || p[4] != 10 || p[5] != 0x10|5 {
		t.Fatalf("44 % X", p)
	}
	// Target already in a party -> 4; requests to self ignored.
	d := statsPlayer(b, PlayerStart+2, 2, 55)
	b.handlePartyRequest(d, partyReq(c.Index))
	if p := find(b.drain(d), 0x41, -1); p == nil || p[3] != partyResultInParty {
		t.Fatalf("in party % X", p)
	}
	// Non-leader can't invite.
	b.handlePartyRequest(c, partyReq(d.Index))
	if p := find(b.drain(c), 0x41, -1); p == nil || p[3] != partyResultFail {
		t.Fatalf("non leader % X", p)
	}
	// Leader invites a third member; kick it (3 members -> removal).
	b.handlePartyRequest(a, partyReq(d.Index))
	b.handlePartyAnswer(d, partyAns(1, a.Index))
	if pt.count != 3 {
		t.Fatalf("count %d", pt.count)
	}
	b.drain(a)
	b.drain(c)
	b.drain(d)
	b.handlePartyLeave(a, frame.Build(frame.C1, 0x43, 2))
	if find(b.drain(d), 0x43, -1) == nil || b.partyOf(d) != nil || pt.count != 2 {
		t.Fatal("kick")
	}
	if p := find(b.drain(c), 0x42, -1); p == nil || p[4] != 2 {
		t.Fatalf("list after kick % X", p)
	}
	// Two members left: leaving dissolves the party.
	b.handlePartyLeave(c, frame.Build(frame.C1, 0x43, 1))
	if find(b.drain(a), 0x43, -1) == nil || b.partyOf(a) != nil || b.partyOf(c) != nil {
		t.Fatal("dissolve")
	}
}

func TestPartyRequestRules(t *testing.T) {
	b := newBareWorld(t)
	a := statsPlayer(b, PlayerStart, 0, 10)
	c := statsPlayer(b, PlayerStart+1, 1, 200)
	// Level gap > PartyMaxGapLevel (130) -> 5.
	b.handlePartyRequest(a, partyReq(c.Index))
	if p := find(b.drain(a), 0x41, -1); p == nil || p[3] != partyResultLevelGap {
		t.Fatalf("gap % X", p)
	}
	c.P.Char.Level = 20
	// Refusal: the answering player gets result 1 (FUN_004d2c80).
	b.handlePartyRequest(a, partyReq(c.Index))
	b.handlePartyAnswer(c, partyAns(0, a.Index))
	if p := find(b.drain(c), 0x41, -1); p == nil || p[3] != partyResultRefused || b.partyOf(a) != nil {
		t.Fatalf("refused % X", p)
	}
	// Unanswered request: both get result 0 after 5 s.
	b.handlePartyRequest(a, partyReq(c.Index))
	b.drain(a)
	b.drain(c)
	b.advance(6 * time.Second)
	b.partySecond()
	if p := find(b.drain(a), 0x41, -1); p == nil || p[3] != 0 {
		t.Fatalf("timeout requester % X", p)
	}
	if p := find(b.drain(c), 0x41, -1); p == nil || p[3] != 0 {
		t.Fatalf("timeout target % X", p)
	}
	if a.P.Party.Requesting || c.P.Party.Requesting {
		t.Fatal("requests not cleared")
	}
	// Leaving the world (FUN_004c4b40, ReconnectTime 300): the member is
	// removed while more than one remains, the last one dissolves it.
	b.handlePartyRequest(a, partyReq(c.Index))
	b.handlePartyAnswer(c, partyAns(1, a.Index))
	b.drain(a)
	b.partyLeaveWorld(c)
	if pt := b.partyOf(a); pt == nil || pt.count != 1 || b.partyOf(c) != nil {
		t.Fatal("leave world")
	}
	if p := findLast(b.drain(a), 0x42); p == nil || p[4] != 1 {
		t.Fatalf("list % X", p)
	}
	b.partyLeaveWorld(a)
	if b.partyOf(a) != nil || find(b.drain(a), 0x43, -1) == nil || len(b.parties.parties) != 0 {
		t.Fatal("last member")
	}
}

func findLast(pkts [][]byte, head byte) []byte {
	var last []byte
	for _, p := range pkts {
		if p != nil && frame.Head(p) == head {
			last = p
		}
	}
	return last
}

// End to end: two clients form a party over the original protocol.
func TestEndToEndParty(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x00, 0, 135, 125, 20)
	c := enterAt(t, env, "second", "Beta", 0x10, 0, 137, 126, 25)
	waitSees(t, a, PlayerStart+1)
	a.send(partyReq(PlayerStart + 1))
	req := c.next(0x40, -1)
	if int(req[3])<<8|int(req[4]) != PlayerStart {
		t.Fatalf("40 % X", req)
	}
	c.send(partyAns(1, PlayerStart))
	if first := a.next(0x42, -1); first[4] != 1 {
		t.Fatalf("first list % X", first)
	}
	la := a.next(0x42, -1)
	lc := c.next(0x42, -1)
	if la[3] != 1 || la[4] != 2 || lc[4] != 2 || string(la[5:10]) != "Alpha" || string(la[29:33]) != "Beta" {
		t.Fatalf("lists % X / % X", la, lc)
	}
	c.send(frame.Build(frame.C1, 0x43, 1))
	a.next(0x43, -1)
	c.next(0x43, -1)
}

// End to end: F3 06 adds a stat point (DK created with the default stats).
func TestEndToEndStatAdd(t *testing.T) {
	env := newTestEnv(t)
	a := enterAt(t, env, "tester", "Alpha", 0x10, 0, 135, 125, 20)
	env.world.post(func() { env.world.objects[PlayerStart].P.Char.LevelUpPoints = 5 })
	a.send(frame.Build(frame.C1, 0xF3, 0x06, 0))
	p := a.next(0xF3, 0x06)
	if p[4] != 0x10 || binary.LittleEndian.Uint32(p[12:]) != 4 || binary.LittleEndian.Uint32(p[28:]) != 29 {
		t.Fatalf("F3 06 % X", p)
	}
}
