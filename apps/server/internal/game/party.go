package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Party: CParty (FUN_004d2310..FUN_004d34b0) and the handlers 40..43
// (FUN_004d2970, FUN_004d2c80, FUN_004d2e80, FUN_004d3100).

const (
	maxParties       = MaxObjects // CParty has 0x57e4 entries
	partyMaxMembers  = 5
	partyRequestTime = 5 * time.Second // FUN_0050e280: 4999 < elapsed
)

// Party results of C1 04 41 (FUN_004d3230).
const (
	partyResultFail     = 0 // map without parties, busy, timeout, not leader
	partyResultRefused  = 1 // target refuses requests (obj+0x434 bit 0) / answered "no"
	partyResultFull     = 2 // could not create or join
	partyResultInParty  = 4 // target already in a party
	partyResultLevelGap = 5 // PartyMaxGapLevel exceeded
)

// party is one CParty entry (0x18 bytes: count +8, members +0xc).
type party struct {
	number  int
	count   int
	members [partyMaxMembers]int // object indexes, -1 = empty
}

// partyManager is CParty.
type partyManager struct {
	parties map[int]*party
	next    int // +4: round-robin start of the free-number search
}

// partyState is the per-player party data.
type partyState struct {
	Number int // obj+0x2f0
	// Pending request (obj+0x3d4 interface 0x80, +0x3d8 time, +0x2f4 the
	// player the request was sent to, +0x318 the other side).
	Requesting  bool
	RequestTime time.Time
	RequestTo   int // obj+0x2f4
	Other       int // obj+0x318
	// Flags is obj+0x434 (default 3): bit 0 accepts requests, bit 2 enables
	// the custom auto-party (password at obj+0x630).
	Flags int
	// Password is obj+0x630 (10 chars), set by /request auto.
	Password string
}

func registerParty(h *hooks) {
	h.worldHandlers[0x40] = func(w *World, o *Object, p []byte) { w.handlePartyRequest(o, p) }
	h.worldHandlers[0x41] = func(w *World, o *Object, p []byte) { w.handlePartyAnswer(o, p) }
	h.worldHandlers[0x42] = func(w *World, o *Object, p []byte) { w.handlePartyList(o) }
	h.worldHandlers[0x43] = func(w *World, o *Object, p []byte) { w.handlePartyLeave(o, p) }
	h.start = append(h.start, func(w *World) error {
		w.parties = &partyManager{parties: map[int]*party{}}
		return nil
	})
	h.enterWorld = append(h.enterWorld, func(w *World, o *Object) { w.partyStateOf(o) })
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { w.partyLeaveWorld(o) })
	h.tick1000 = append(h.tick1000, func(w *World) { w.partySecond() })
}

func (w *World) partyStateOf(o *Object) *partyState {
	if o.P.Party == nil {
		o.P.Party = &partyState{Number: -1, RequestTo: -1, Other: -1, Flags: 3}
	}
	return o.P.Party
}

// partyOf returns the party of a player (nil when none).
func (w *World) partyOf(o *Object) *party {
	if o.P == nil || o.P.Party == nil || w.parties == nil {
		return nil
	}
	return w.parties.parties[o.P.Party.Number]
}

// objDistance is FUN_0050c750 (truncated float distance).
func objDistance(a, b *Object) int {
	dx, dy := float32(b.Y-a.Y), float32(b.X-a.X)
	return int(float32(math.Sqrt(float64(dx*dx + dy*dy))))
}

// partyUsable is FUN_0050cc00: a playing player not logging out.
func partyUsable(o *Object) bool {
	return o.State == StatePlaying && o.Type == TypePlayer && o.P != nil && o.P.Char != nil && o.P.LogoutCountdown < 1
}

func (w *World) mapPartyEnabled(mp int) bool {
	info, ok := w.data.Maps[mp]
	return ok && info.PartyEnable != 0
}

// --- packets ----------------------------------------------------------------

func packetPartyResult(result byte) []byte {
	b := newPacket(frame.C1, 0x41, -1, 4)
	b[3] = result
	return b
}

func packetPartyRequest(from int) []byte {
	b := newPacket(frame.C1, 0x40, -1, 5)
	b[3], b[4] = byte(from>>8), byte(from)
	return b
}

func packetPartyLeft() []byte { return newPacket(frame.C1, 0x43, -1, 3) }

// packetPartyList is C1 size 42 result count + 24-byte entries: name[10],
// slot, map, x, y, 2 bytes, life u32, max life u32 (FUN_004d32d0 /
// FUN_004d2e80). Without a party: C1 05 42 00 00.
func (w *World) packetPartyList(pt *party) []byte {
	if pt == nil {
		return newPacket(frame.C1, 0x42, 0, 5)
	}
	var body []byte
	n := 0
	for slot, idx := range pt.members {
		if idx < 0 || idx >= MaxObjects {
			continue
		}
		m := w.objects[idx]
		e := make([]byte, 24)
		if m.P != nil && m.P.Char != nil {
			putName(e, m.P.Char.Name)
			e[10], e[11] = byte(slot), byte(m.Map)
			e[12], e[13] = byte(m.X), byte(m.Y)
			u32le(e[16:], uint32(int32(math.RoundToEven(float64(m.P.Char.Life)))))
			u32le(e[20:], uint32(int32(math.RoundToEven(float64(float32(m.P.AddLife)+m.P.Char.MaxLife)))))
		}
		body = append(body, e...)
		n++
	}
	b := newPacket(frame.C1, 0x42, 1, 5+len(body))
	b[4] = byte(n)
	copy(b[5:], body)
	return b
}

// packetPartyLife is C1 size 44 count + one byte per member:
// (life*10/maxLife rounded) & 0xF ^ slot<<4 (FUN_004d34b0).
func (w *World) packetPartyLife(pt *party) []byte {
	var body []byte
	for slot, idx := range pt.members {
		if idx < 0 || idx >= MaxObjects {
			continue
		}
		m := w.objects[idx]
		v := 0
		if m.P != nil && m.P.Char != nil {
			c := m.P.Char
			den := float32(m.P.AddLife) + c.MaxLife
			v = int(math.RoundToEven(float64(c.Life / (den / 10))))
		}
		body = append(body, byte(v)&0xF^byte(slot)<<4)
	}
	b := newPacket(frame.C1, 0x44, -1, 4+len(body))
	b[3] = byte(len(body))
	copy(b[4:], body)
	return b
}

// sendPartyList is FUN_004d32d0: the list to every member.
func (w *World) sendPartyList(pt *party) {
	pkt := w.packetPartyList(pt)
	for _, idx := range pt.members {
		if idx >= 0 && idx < MaxObjects {
			w.sendIndex(idx, pkt)
		}
	}
}

// --- CParty -------------------------------------------------------------------

// create is FUN_004d2450: a new party led by leader unless the leader is
// already in one.
func (w *World) partyCreate(leader *Object) bool {
	if w.partyOf(leader) != nil {
		return false
	}
	pm := w.parties
	n := pm.next
	for i := 0; i < maxParties; i++ {
		if _, used := pm.parties[n]; !used {
			pt := &party{number: n, count: 1, members: [partyMaxMembers]int{leader.Index, -1, -1, -1, -1}}
			pm.parties[n] = pt
			w.partyStateOf(leader).Number = n
			w.sendPartyList(pt)
			pm.next = n + 1
			if pm.next >= maxParties {
				pm.next = 0
			}
			return true
		}
		n++
		if n >= maxParties {
			n = 0
		}
	}
	return false
}

// add is FUN_004d25b0.
func (w *World) partyAdd(pt *party, o *Object) bool {
	if pt == nil {
		return false
	}
	for _, idx := range pt.members {
		if idx == o.Index {
			return false
		}
	}
	slot := 0
	for slot < partyMaxMembers && pt.members[slot] >= 0 && pt.members[slot] < MaxObjects {
		slot++
	}
	if slot >= partyMaxMembers {
		return false
	}
	w.partyStateOf(o).Number = pt.number
	pt.count++
	pt.members[slot] = o.Index
	w.sendPartyList(pt)
	return true
}

// destroy is FUN_004d2500: every member gets C1 03 43.
func (w *World) partyDestroy(pt *party) {
	if pt == nil {
		return
	}
	for i, idx := range pt.members {
		if idx < 0 || idx >= MaxObjects {
			continue
		}
		m := w.objects[idx]
		if m.P != nil && m.P.Party != nil {
			m.P.Party.Number = -1
		}
		w.sendIndex(idx, packetPartyLeft())
		pt.count--
		pt.members[i] = -1
	}
	delete(w.parties.parties, pt.number)
}

// remove is FUN_004d2650 (+FUN_004d2700: a removed leader is replaced by
// the next member).
func (w *World) partyRemove(pt *party, o *Object) bool {
	if pt == nil {
		return false
	}
	for slot, idx := range pt.members {
		if idx != o.Index {
			continue
		}
		if o.P != nil && o.P.Party != nil {
			o.P.Party.Number = -1
		}
		pt.count--
		pt.members[slot] = -1
		if slot == 0 {
			for j := 1; j < partyMaxMembers; j++ {
				if m := pt.members[j]; m >= 0 && m < MaxObjects {
					pt.members[0], pt.members[j] = m, -1
					break
				}
			}
		}
		w.send(o, packetPartyLeft())
		w.sendPartyList(pt)
		if pt.count <= 0 {
			delete(w.parties.parties, pt.number)
		}
		return true
	}
	return false
}

func (pt *party) leader() int { return pt.members[0] }

// clearRequest resets the party interface of a player.
func (w *World) clearPartyRequest(o *Object) {
	s := w.partyStateOf(o)
	s.Requesting = false
	s.RequestTime = time.Time{}
	s.RequestTo = -1
	s.Other = -1
}

// --- handlers -------------------------------------------------------------------

// handlePartyRequest is 40 (FUN_004d2970): C1 05 40 target(BE).
func (w *World) handlePartyRequest(o *Object, p []byte) {
	if len(p) < 5 || !partyUsable(o) {
		return
	}
	if !w.mapPartyEnabled(o.Map) {
		return
	}
	ti := int(p[3])<<8 | int(p[4])
	if ti < 0 || ti >= MaxObjects {
		return
	}
	t := w.objects[ti]
	if !partyUsable(t) || t == o {
		return
	}
	os, ts := w.partyStateOf(o), w.partyStateOf(t)
	if os.Requesting || ts.Requesting || !o.Alive || !t.Alive || o.Teleporting || t.Teleporting {
		return
	}
	result := byte(partyResultFail)
	switch {
	case !w.mapPartyEnabled(t.Map) || os.RequestTo >= 0 || ts.RequestTo >= 0:
		result = partyResultFail
	case abs(o.P.Char.Level-t.P.Char.Level) > w.cfg.Int("Common", "PartyMaxGapLevel", 0):
		result = partyResultLevelGap
	default:
		// FUN_004d2780 (auto-party by password) needs obj+0x434 bit 2,
		// only set by custom commands (U-PARTY-01).
		pt := w.partyOf(o)
		if pt != nil && pt.leader() != o.Index {
			result = partyResultFail
			break
		}
		if ts.Flags&1 == 0 {
			result = partyResultRefused
			break
		}
		if w.partyOf(t) != nil {
			result = partyResultInParty
			break
		}
		now := w.now()
		os.Requesting, os.RequestTime, os.Other, os.RequestTo = true, now, t.Index, t.Index
		ts.Requesting, ts.RequestTime, ts.Other = true, now, o.Index
		w.send(t, packetPartyRequest(o.Index))
		return
	}
	w.send(o, packetPartyResult(result))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// handlePartyAnswer is 41 (FUN_004d2c80): C1 06 41 answer requester(BE),
// sent by the player who received the request.
func (w *World) handlePartyAnswer(o *Object, p []byte) {
	if len(p) < 6 || !partyUsable(o) {
		return
	}
	ri := int(p[4])<<8 | int(p[5])
	if ri >= MaxObjects {
		return
	}
	r := w.objects[ri]
	if !partyUsable(r) {
		return
	}
	os, rs := w.partyStateOf(o), w.partyStateOf(r)
	if !os.Requesting || !rs.Requesting {
		return
	}
	switch {
	case !w.mapPartyEnabled(o.Map):
		w.send(o, packetPartyResult(partyResultFail))
	case p[3] == 0:
		w.send(o, packetPartyResult(partyResultRefused))
	default:
		created := w.partyCreate(r)
		if !w.partyAdd(w.partyOf(r), o) {
			w.send(o, packetPartyResult(partyResultFull))
			w.send(r, packetPartyResult(partyResultFull))
			if created {
				w.partyDestroy(w.partyOf(r))
			}
		}
	}
	w.clearPartyRequest(o)
	w.clearPartyRequest(r)
}

// handlePartyList is 42 (FUN_004d2e80).
func (w *World) handlePartyList(o *Object) {
	if !partyUsable(o) {
		return
	}
	w.send(o, w.packetPartyList(w.partyOf(o)))
}

// handlePartyLeave is 43 (FUN_004d3100): C1 04 43 slot. Members leave
// themselves; the leader may kick anyone. With fewer than 3 members the
// party is dissolved.
func (w *World) handlePartyLeave(o *Object, p []byte) {
	if len(p) < 4 || !partyUsable(o) {
		return
	}
	pt := w.partyOf(o)
	slot := int(p[3])
	if pt == nil || slot >= partyMaxMembers {
		return
	}
	ti := pt.members[slot]
	if ti < 0 || ti >= MaxObjects {
		return
	}
	t := w.objects[ti]
	if !partyUsable(t) || (ti != o.Index && pt.leader() != o.Index) {
		return
	}
	if pt.count < 3 {
		w.partyDestroy(pt)
		return
	}
	w.partyRemove(pt, t)
}

// partyLeaveWorld is the party part of FUN_004c4b40: with ReconnectTime
// set a member leaves while 2+ remain, otherwise the party is dissolved
// (the reconnect restore itself is U-PARTY-02).
func (w *World) partyLeaveWorld(o *Object) {
	if o.P == nil || o.P.Party == nil {
		return
	}
	s := o.P.Party
	if s.Requesting && s.Other >= 0 && s.Other < MaxObjects {
		if t := w.objects[s.Other]; t.P != nil && t.P.Party != nil && t.P.Party.Other == o.Index {
			w.clearPartyRequest(t)
		}
	}
	w.clearPartyRequest(o)
	pt := w.partyOf(o)
	if pt == nil {
		return
	}
	threshold := 1
	if w.cfg.Int("Common", "ReconnectTime", 0) < 1 {
		threshold = 2
	}
	if pt.count > threshold {
		w.partyRemove(pt, o)
	} else {
		w.partyDestroy(pt)
	}
}

// partySecond is the party part of the per-second player processing
// (FUN_00510810 -> FUN_004d34b0 and the request timeout of FUN_0050e280).
func (w *World) partySecond() {
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.P == nil || o.P.Party == nil {
			continue
		}
		s := o.P.Party
		if s.Requesting && now.Sub(s.RequestTime) >= partyRequestTime {
			if s.Other >= 0 && s.Other < MaxObjects {
				t := w.objects[s.Other]
				if t.P != nil && t.P.Party != nil {
					t.P.Party.Requesting = false
					t.P.Party.Other = -1
					t.P.Party.RequestTo = -1
					w.send(t, packetPartyResult(partyResultFail))
				}
			}
			w.clearPartyRequest(o)
			w.send(o, packetPartyResult(partyResultFail))
		}
		if pt := w.partyOf(o); pt != nil {
			pkt := w.packetPartyLife(pt)
			for _, idx := range pt.members {
				if idx >= 0 && idx < MaxObjects {
					w.sendIndex(idx, pkt)
				}
			}
		}
	}
}

func registerPartyLoot(h *hooks) {
	// Map-item loot rights are shared inside a party (FUN_004982e0).
	h.sameParty = func(w *World, a, b *Object) bool {
		pa := w.partyOf(a)
		return pa != nil && pa == w.partyOf(b)
	}
}
