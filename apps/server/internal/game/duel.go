package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Duel (CDuel of the original): AA request FUN_00477890, AC answer
// FUN_00477d60, AB cancel/end FUN_00478170, the 1-second timer
// FUN_00477680, the kill FUN_00477830 / FUN_00477430 and the packets
// AA FUN_00478280, AB FUN_004783e0, AC FUN_00478300, AD FUN_00478460.
//
// Object fields: +0x43c player I asked, +0x440 player who asked me,
// +0x444 opponent, +0x448 score, +0x44c last duel tick (request time,
// then the last hit), +0x450 flag; the interface lock is obj+0x3d4 = use
// 1, type 0x16 (0x581).

// duelState is obj+0x43c..0x450.
type duelState struct {
	Asked    int       // obj+0x43c
	AskedBy  int       // obj+0x440
	Opponent int       // obj+0x444
	Score    int       // obj+0x448
	Tick     time.Time // obj+0x44c
	Busy     bool      // obj+0x3d4 interface "duel" (0x580 | use)
}

func dl(o *Object) *duelState {
	if o.Duel == nil {
		o.Duel = &duelState{Asked: -1, AskedBy: -1, Opponent: -1}
	}
	return o.Duel
}

// duelReset is FUN_004777e0.
func duelReset(o *Object) {
	if o.Duel == nil {
		return
	}
	*o.Duel = duelState{Asked: -1, AskedBy: -1, Opponent: -1}
}

func validIdx(i int) bool { return i >= 0 && i < MaxObjects }

// inDuelWith is FUN_004773e0: a and b are players dueling each other.
func (w *World) inDuelWith(a, b *Object) bool {
	if a == nil || b == nil || a.Type != TypePlayer || b.Type != TypePlayer || a.Duel == nil || b.Duel == nil {
		return false
	}
	return validIdx(a.Duel.Opponent) && validIdx(b.Duel.Opponent) && a.Duel.Opponent == b.Index && b.Duel.Opponent == a.Index
}

func objName(o *Object) string {
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Name
	}
	return ""
}

// packetDuelStart is C1 10 AA result idx(BE) name[10] (FUN_00478280,
// result in CL).
func packetDuelStart(result byte, about *Object) []byte {
	p := newPacket(frame.C1, 0xAA, -1, 0x10)
	p[3] = result
	p[4], p[5] = byte(about.Index>>8), byte(about.Index)
	putName(p[6:], objName(about))
	return p
}

// packetDuelEnd is C1 0F AB idx(BE) name[10] of the receiver itself
// (FUN_004783e0).
func packetDuelEnd(o *Object) []byte {
	p := newPacket(frame.C1, 0xAB, -1, 0x0F)
	p[3], p[4] = byte(o.Index>>8), byte(o.Index)
	putName(p[5:], objName(o))
	return p
}

// packetDuelAsk is C1 0F AC idx(BE) name[10] of the requester
// (FUN_00478300).
func packetDuelAsk(from *Object) []byte {
	p := newPacket(frame.C1, 0xAC, -1, 0x0F)
	p[3], p[4] = byte(from.Index>>8), byte(from.Index)
	putName(p[5:], objName(from))
	return p
}

// packetDuelScore is C1 09 AD a(BE) b(BE) scoreA scoreB (FUN_00478460).
func packetDuelScore(a, b *Object) []byte {
	p := newPacket(frame.C1, 0xAD, -1, 9)
	p[3], p[4] = byte(a.Index>>8), byte(a.Index)
	p[5], p[6] = byte(b.Index>>8), byte(b.Index)
	p[7], p[8] = byte(dl(a).Score), byte(dl(b).Score)
	return p
}

func (w *World) sendDuelScore(a, b *Object) {
	p := packetDuelScore(a, b)
	w.send(a, p)
	w.send(b, p)
}

func (w *World) duelMessage(o *Object, n int) {
	w.systemMessage(o, w.messageFor(o, n))
}

// mapDuelEnabled is FUN_004ae990: MapManager DuelEnable, or DuelSwitch
// when the column is "*".
func (w *World) mapDuelEnabled(mp int) bool {
	if info, ok := w.data.Maps[mp]; ok && info.DuelEnable != -1 {
		return info.DuelEnable == 1
	}
	return w.cfg.Int("Common", "DuelSwitch", 0) != 0
}

func duelIdle(o *Object) bool {
	d := o.Duel
	return d == nil || (!validIdx(d.Asked) && !validIdx(d.AskedBy) && !validIdx(d.Opponent))
}

// handleDuelRequest is AA (FUN_00477890): [3..4] target BE, [5..14] name.
func (w *World) handleDuelRequest(o *Object, p []byte) {
	if len(p) < 15 || !skillPlayerOK(o) {
		return
	}
	if !w.mapDuelEnabled(o.Map) {
		w.duelMessage(o, 0xa7)
		return
	}
	idx := int(p[3])<<8 | int(p[4])
	if !validIdx(idx) || idx == o.Index || !partyUsable(w.objects[idx]) {
		w.duelMessage(o, 0xa0)
		return
	}
	t := w.objects[idx]
	if dl(o).Busy || dl(t).Busy {
		w.duelMessage(o, 0xa1)
		return
	}
	if t.P.Party != nil && t.P.Party.Flags&2 == 0 {
		w.duelMessage(o, 0xa2)
		return
	}
	if !duelIdle(o) {
		w.duelMessage(o, 0xa3)
		return
	}
	if !duelIdle(t) {
		w.duelMessage(o, 0xa4)
		return
	}
	if objName(t) != cString(p[5:15]) {
		w.duelMessage(o, 0xa9)
		return
	}
	if t.Trade() || o.Trade() {
		w.duelMessage(o, 0xab)
		return
	}
	now := w.now()
	*dl(o) = duelState{Asked: idx, AskedBy: -1, Opponent: -1, Tick: now, Busy: true}
	*dl(t) = duelState{Asked: -1, AskedBy: o.Index, Opponent: -1, Tick: now, Busy: true}
	w.send(t, packetDuelAsk(o))
}

// Trade reports whether the player has a trade window open (the
// original tests the trade slots obj+0x2c4.. for 0xaa).
func (o *Object) Trade() bool {
	return o.P != nil && o.P.Trade != nil && tradeOpenHook != nil && tradeOpenHook(o)
}

// tradeOpenHook lets the trade module report an open trade.
var tradeOpenHook func(o *Object) bool

// handleDuelAnswer is AC (FUN_00477d60): [3] accept, [4..5] requester BE,
// [6..15] requester name.
func (w *World) handleDuelAnswer(o *Object, p []byte) {
	if len(p) < 16 || !skillPlayerOK(o) {
		return
	}
	idx := int(p[4])<<8 | int(p[5])
	if !validIdx(idx) {
		return
	}
	r := w.objects[idx]
	if !partyUsable(r) || !dl(o).Busy || !dl(r).Busy {
		return
	}
	fail := func(msg int) {
		w.duelMessage(o, msg)
		duelReset(o)
		duelReset(r)
		w.send(r, packetDuelStart(0, o))
	}
	if p[3] == 0 {
		fail(0xac)
		return
	}
	if validIdx(dl(o).Asked) || validIdx(dl(o).Opponent) {
		fail(0xa3)
		return
	}
	if validIdx(dl(r).AskedBy) || validIdx(dl(r).Opponent) {
		fail(0xa4)
		return
	}
	if dl(r).Asked != o.Index {
		fail(0xa4)
		return
	}
	if !w.mapDuelEnabled(r.Map) {
		fail(0xa7)
		return
	}
	if objName(r) != cString(p[6:16]) {
		fail(0xa9)
		return
	}
	now := w.now()
	*dl(o) = duelState{Asked: -1, AskedBy: -1, Opponent: r.Index, Tick: now}
	*dl(r) = duelState{Asked: -1, AskedBy: -1, Opponent: o.Index, Tick: now}
	w.send(o, packetDuelStart(1, r))
	w.send(r, packetDuelStart(1, o))
	w.sendDuelScore(o, r)
	if w.cfg.Int("Common", "DuelAnnounceSwitch", 0) != 0 {
		w.noticeAll(0, w.messageFor(o, 0x2d0, objName(o), objName(r)))
	}
}

// handleDuelCancel is AB (FUN_00478170).
func (w *World) handleDuelCancel(o *Object, p []byte) {
	if !skillPlayerOK(o) || o.Duel == nil {
		return
	}
	w.duelCancel(o)
}

func (w *World) duelCancel(o *Object) {
	d := dl(o)
	switch {
	case validIdx(d.Asked):
		t := w.objects[d.Asked]
		duelReset(o)
		w.send(o, packetDuelStart(0, t))
		duelReset(t)
	case validIdx(d.AskedBy):
		t := w.objects[d.AskedBy]
		duelReset(o)
		w.send(t, packetDuelStart(0, o))
		duelReset(t)
	case validIdx(d.Opponent):
		t := w.objects[d.Opponent]
		duelReset(o)
		w.send(t, packetDuelEnd(t))
		duelReset(t)
		w.send(o, packetDuelEnd(o))
	}
}

// duelSecond is FUN_00477680: requests expire after 10 s, a duel after
// 60 s without hits.
func (w *World) duelSecond() {
	now := w.now()
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.Duel == nil {
			continue
		}
		d := o.Duel
		if validIdx(d.Asked) && now.Sub(d.Tick) > 10*time.Second {
			t := w.objects[d.Asked]
			duelReset(o)
			w.send(o, packetDuelStart(0, t))
			duelReset(t)
		}
		if validIdx(d.AskedBy) && now.Sub(d.Tick) > 10*time.Second {
			t := w.objects[d.AskedBy]
			duelReset(o)
			w.send(t, packetDuelStart(0, o))
			duelReset(t)
		}
		if validIdx(d.Opponent) && now.Sub(d.Tick) > 60*time.Second {
			t := w.objects[d.Opponent]
			duelReset(t)
			w.send(t, packetDuelEnd(t))
			duelReset(o)
			w.send(o, packetDuelEnd(o))
		}
	}
}

// duelScore is FUN_00477830: the killer of a duel kill scores and the
// score is sent to both.
func (w *World) duelScore(killer, victim *Object) {
	if !w.inDuelWith(killer, victim) {
		return
	}
	dl(killer).Score++
	w.sendDuelScore(killer, victim)
}

// duelKill is FUN_00477430: after a duel kill, DuelMaxScore ends the duel
// with AB to both and the C1 07 F3 40 firework at the winner.
func (w *World) duelKill(killer, victim *Object) {
	if !w.inDuelWith(killer, victim) {
		return
	}
	if w.cfg.Int("Common", "DuelAnnounceSwitch", 0) == 2 {
		w.noticeAll(0, w.messageFor(killer, 0x2d1, objName(killer), dl(killer).Score, dl(victim).Score, objName(victim)))
	}
	if dl(killer).Score < w.cfg.Int("Common", "DuelMaxScore", 0) {
		return
	}
	w.reportDuelRanking(killer, 1, 0)
	w.reportDuelRanking(victim, 0, 1)
	duelReset(killer)
	w.send(killer, packetDuelEnd(killer))
	duelReset(victim)
	w.send(victim, packetDuelEnd(victim))
	fw := newPacket(frame.C1, 0xF3, 0x40, 7)
	fw[5], fw[6] = byte(killer.X), byte(killer.Y)
	w.send(killer, fw)
	w.broadcast(killer, fw)
	if w.cfg.Int("Common", "DuelAnnounceSwitch", 0) != 0 {
		w.noticeAll(0, w.messageFor(killer, 0x2d2, objName(killer), objName(victim)))
	}
}

func init() {
	worldHandlers[0xAA] = (*World).handleDuelRequest
	worldHandlers[0xAB] = (*World).handleDuelCancel
	worldHandlers[0xAC] = (*World).handleDuelAnswer
	tick1000Hooks = append(tick1000Hooks, func(w *World) { w.duelSecond() })
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) {
		if o.Duel != nil {
			w.duelCancel(o)
			o.Duel = nil
		}
	})
}
