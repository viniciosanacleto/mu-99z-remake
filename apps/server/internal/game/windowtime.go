package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// CCustomWindowTime (FUN_00458a70, WindowEventSwitch of GameServerInfo -
// Event.dat): the event timetable window of Main.dll. C1 xx 06 opens it
// (FUN_004d7de0: obj+0x24a8 = 1), a DF position fix closes it
// (FUN_004d80d0); while open the 1-second player proc (FUN_00510810) sends
// C2 size F3 EF [5] count, then 40-byte rows {name[32], open byte, 3
// pad, remaining seconds i32}, when there is at least one row:
//
//   - Devil Square, Blood Castle, Chaos Castle (their switches; room 0):
//     state 0 closed, 1 open with the remaining time, other open;
//   - every Invasion, Bonus and Drop event not in state 0 (name of the
//     file), remaining time in state 1;
//   - every CustomArena not in state 0, remaining time in state 1;
//   - Luke Helper and Castle Deep like Devil Square;
//   - Death, Duel, Pair, Sobre and Theft (Active): idle = seconds to the
//     next start (open when not 0, -1 without starts), registration =
//     open, running = open with the time.

type windowRow struct {
	Name   string
	Open   bool
	Remain int
}

func registerWindowtime(h *hooks) {
	h.worldHandlers[0x06] = func(w *World, o *Object, p []byte) { w.windowOpen(o) }
	h.windowClose = func(w *World, o *Object) { delete(w.wcx().window, o.Index) }
	h.tick1000 = append(h.tick1000, (*World).windowSecond)
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { delete(w.wcx().window, o.Index) })
}

// windowOpen is FUN_004d7de0.
func (w *World) windowOpen(o *Object) {
	if o.State == StatePlaying && o.IsPlayer() && o.P.LogoutCountdown < 1 && !o.P.Moving {
		w.wcx().window[o.Index] = true
	}
}

// packetWindowTime is C2 F3 EF.
func packetWindowTime(rows []windowRow) []byte {
	p := newPacket(frame.C2, 0xF3, 0xEF, 6+40*len(rows))
	p[5] = byte(len(rows))
	for i, r := range rows {
		b := p[6+40*i:]
		name := r.Name
		if len(name) > 31 {
			name = name[:31]
		}
		copy(b[:32], name)
		if r.Open {
			b[32] = 1
		}
		u32le(b[36:], uint32(int32(r.Remain)))
	}
	return p
}

// stateRow is the Devil Square / Blood Castle / Chaos Castle / Luke /
// Castle Deep rule.
func stateRow(name string, state, remain int) windowRow {
	r := windowRow{Name: name, Open: true}
	switch state {
	case 0:
		r.Open = false
	case 1:
		r.Remain = remain
	}
	return r
}

// timedRows lists the Invasion / Bonus / Drop events not in state 0.
func timedRows(states []*timedEventState) []windowRow {
	var out []windowRow
	for i, s := range states {
		if i >= 20 || s == nil || s.State == 0 {
			continue
		}
		r := windowRow{Name: s.data.Name, Open: true}
		if s.State == 1 {
			r.Remain = s.Remain
		}
		out = append(out, r)
	}
	return out
}

// nextEventSeconds is FUN_0045d0f0 (and the Duel, Pair, Sobre and Theft
// copies): the seconds to the nearest start of the next 7 days (today only
// when still ahead), -1 without any.
func nextEventSeconds(starts []gamedata.EventStart, now time.Time) int {
	best := -1
	cur := (now.Hour()*60+now.Minute())*60 + now.Second()
	for k := 0; k < 7; k++ {
		day := (int(now.Weekday()) + k) % 7
		for _, s := range starts {
			if s.Day != day {
				continue
			}
			t := (s.Hour*60 + s.Minute) * 60
			var d int
			if k == 0 {
				d = t - cur
				if d < 1 {
					continue
				}
			} else {
				d = k*86400 - cur + t
			}
			if best == -1 || d < best {
				best = d
			}
		}
	}
	return best
}

// idleRow is the "state 0" branch of the custom events.
func idleRow(name string, starts []gamedata.EventStart, now time.Time) windowRow {
	n := nextEventSeconds(starts, now)
	return windowRow{Name: name, Open: n != 0, Remain: n}
}

// windowRows builds the rows for one player (names in its language).
func (w *World) windowRows(o *Object) []windowRow {
	var rows []windowRow
	ev := w.ev()
	msg := func(n int) string { return w.messageFor(o, n) }
	on := func(key string) bool { return w.cfg.Int("Event", key, 0) != 0 }
	if on("DevilSquareEvent") && ev.ds != nil {
		rows = append(rows, stateRow(msg(0x2EE), ev.ds.sq[0].State, ev.ds.sq[0].Remain))
	}
	if on("BloodCastleEvent") && ev.bc != nil {
		rows = append(rows, stateRow(msg(0x2EF), ev.bc.c[0].State, ev.bc.c[0].Remain))
	}
	if on("ChaosCastleEvent") && ev.cc != nil {
		rows = append(rows, stateRow(msg(0x2F0), ev.cc.c[0].State, ev.cc.c[0].Remain))
	}
	if on("InvasionManagerSwitch") {
		for i, s := range ev.inv {
			if i >= 20 || s == nil || s.State == 0 {
				continue
			}
			r := windowRow{Name: s.data.Name, Open: true}
			if s.State == 1 {
				r.Remain = s.Remain
			}
			rows = append(rows, r)
		}
	}
	if on("BonusManagerSwitch") && ev.bonus != nil {
		rows = append(rows, timedRows(ev.bonus.events)...)
	}
	if on("DropEventSwitch") && ev.drop != nil {
		rows = append(rows, timedRows(ev.drop.events)...)
	}
	rows = append(rows, w.arenaWindow()...)
	rows = append(rows, w.lukeWindow(o)...)
	if on("CastleDeepEvent") && ev.deep != nil {
		rows = append(rows, stateRow(msg(0x2F4), ev.deep.State, ev.deep.Remain))
	}
	now := w.now()
	c := w.cev()
	killRow := func(e *killEvent, name int) {
		if !e.data.Active {
			return
		}
		r := windowRow{Name: msg(name), Open: true}
		switch {
		case !e.running && !e.open:
			r = idleRow(r.Name, e.data.Starts, now)
		case e.running && !e.open:
			r.Remain = runningSeconds(e.data.Starts, now)
		}
		rows = append(rows, r)
	}
	killRow(&c.death, 0x2F9)
	if c.duel.data.Active {
		r := windowRow{Name: msg(0x2FA), Open: true}
		switch c.duel.state {
		case 0:
			r = idleRow(r.Name, c.duel.data.Starts, now)
		case 3, 4, 5, 6, 7:
			r.Remain = c.duel.counter
		}
		rows = append(rows, r)
	}
	if c.pair.data.Active {
		r := windowRow{Name: msg(0x2FB), Open: true}
		if c.pair.state == 0 {
			r = idleRow(r.Name, c.pair.data.Starts, now)
		}
		// States 3 and 7 show DAT_00fee954 * 60, a field the reconstruction
		// does not identify (U-CW-07): 0 here.
		rows = append(rows, r)
	}
	if c.sobre.data.Active {
		r := windowRow{Name: msg(0x2FC), Open: true}
		if c.sobre.state == 0 {
			r = idleRow(r.Name, c.sobre.data.Starts, now)
		}
		// States 2..4 show DAT_0104c28c * 60 (U-CW-07): 0 here.
		rows = append(rows, r)
	}
	killRow(&c.theft, 0x2FD)
	return rows
}

// runningSeconds is FUN_0045d060: while running, the duration of the start
// row of this very minute (Duration minutes), otherwise 0.
func runningSeconds(starts []gamedata.EventStart, now time.Time) int {
	for _, s := range starts {
		if s.Day == int(now.Weekday()) && s.Hour == now.Hour() && s.Minute == now.Minute() {
			return s.Extra * 60
		}
	}
	return 0
}

// windowSecond is the FUN_00458a70 call of FUN_00510810.
func (w *World) windowSecond() {
	if w.cfg.Int("Event", "WindowEventSwitch", 0) == 0 {
		return
	}
	for idx := range w.wcx().window {
		o := w.objects[idx]
		if o.State != StatePlaying || !o.IsPlayer() {
			continue
		}
		if rows := w.windowRows(o); len(rows) > 0 {
			w.send(o, packetWindowTime(rows))
		}
	}
}
