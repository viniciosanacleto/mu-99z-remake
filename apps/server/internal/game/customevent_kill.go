package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Death-Match (DeathEvent.txt) and Theft of Points (TheftEvent.txt): the
// players registered during TIME minutes fight on the event map for
// DURATION minutes; the one with the highest score wins. Both run in the
// FUN_005107e0 thread, which blocks in Sleep(60000) while the registration
// is open and in Sleep(DURATION) while the event runs (here a non-blocking
// timeline driven by the 1-second tick; U-CEVT-03).
//
// Death: globals at DAT_00772d98 (+0 Active, +1 Patente, +2 Syntax, +0x1b
// registration open, +0x1c running, +0x20 Level, +0x24 map/x/y, +0x28 the
// start rows, +0x2c best index, +0x30 players), per-index table
// DAT_007742b8 (8 bytes: +0 registered, +1 waiting, +4 kills); FUN_0045c2b0
// timer, FUN_0045bf60 join, FUN_0045c900 kill, FUN_0045ca20 end,
// FUN_0045cec0 reset, FUN_0045cfb0 leave.
// Theft: DAT_0104d290 (the same layout) and the table DAT_0104e8b4;
// FUN_00506960 timer, FUN_00506670 join, FUN_00506fe0 kill, FUN_00507150
// end, FUN_005075f0 reset, FUN_005076f0 leave.

const (
	killIdle    = 0
	killOpen    = 1 // registration, announcing every minute
	killRunning = 2
)

// killUser is a table entry: +0 registered (presence in the map), +1
// waiting (cannot attack while the registration is open), +4 score.
type killUser struct {
	Wait  bool
	Score int
}

type killEvent struct {
	theft   bool
	data    *gamedata.KillEvent
	open    bool // +0x1b
	running bool // +0x1c
	count   int  // registered players (+0x30)
	users   map[int]*killUser

	phase   int
	minutes int       // announcements left
	next    time.Time // next announcement / end of the registration
	end     time.Time // end of the event (Sleep(DURATION))
	row     gamedata.EventStart
}

// Messages of the two events (Death, Theft).
func (e *killEvent) msgHeader() int {
	if e.theft {
		return 0x4B8
	}
	return 0x464
}

func (w *World) killNotice(e *killEvent, o *Object, n int, args ...any) {
	w.eventNotice(o, NoticeGold, n, args...)
}

// killSecond is FUN_0045c2b0 / FUN_00506960.
func (w *World) killSecond(e *killEvent) {
	if !e.data.Active {
		return
	}
	now := w.now()
	switch e.phase {
	case killIdle:
		for _, s := range e.data.Starts {
			if w.eventScheduled(s) {
				w.killTrigger(e, s)
				return
			}
		}
	case killOpen:
		if now.Before(e.next) {
			return
		}
		e.minutes--
		if e.minutes > 0 {
			w.killAnnounce(e)
			e.next = e.next.Add(time.Minute)
			return
		}
		w.killCloseRegistration(e)
	case killRunning:
		if !now.Before(e.end) {
			w.killFinish(e)
			e.phase = killIdle
		}
	}
}

// killTrigger is the start row match: best and count reset, registration
// open, then TIME announcements one minute apart.
func (w *World) killTrigger(e *killEvent, s gamedata.EventStart) {
	e.row = s
	e.count = 0
	e.open = true
	e.minutes = s.Time
	if s.Time < 1 {
		w.killCloseRegistration(e)
		return
	}
	w.killAnnounce(e)
	e.phase = killOpen
	e.next = w.now().Add(time.Minute)
}

// killAnnounce is the minute loop: header, "starts in N minute(s)" and the
// syntax, to everybody (FUN_004c0ab0).
func (w *World) killAnnounce(e *killEvent) {
	w.eventGlobal(e.msgHeader())
	if e.theft {
		w.eventGlobal(0x4B9, e.minutes)
		w.eventGlobal(0x4BA, e.data.Syntax)
	} else {
		w.eventGlobal(0x465, e.minutes)
		w.eventGlobal(0x466, e.data.Syntax)
	}
}

// killCloseRegistration is the end of the registration: with fewer than 2
// players (or TIME < 1) the event is cancelled, otherwise it starts and
// runs DURATION minutes.
func (w *World) killCloseRegistration(e *killEvent) {
	if e.row.Time < 1 || e.count < 2 {
		w.killCancel(e)
		e.count = 0
		e.open = false
		if e.running {
			e.phase = killRunning
			e.end = w.now().Add(time.Duration(e.row.Extra) * time.Minute)
		} else {
			e.phase = killIdle
		}
		return
	}
	for idx, u := range e.users {
		if o := w.objects[idx]; o.State == StatePlaying {
			u.Score = 0
			if e.theft {
				u.Score = 1
			}
			u.Wait = false
		}
	}
	w.eventPlayers(func(o *Object) {
		if e.theft {
			w.killNotice(e, o, 0x4B8)
			w.killNotice(e, o, 0x4BB)
			w.killNotice(e, o, 0x4BC)
		} else {
			w.killNotice(e, o, 0x464)
			w.killNotice(e, o, 0x467)
			w.killNotice(e, o, 0x468)
		}
	})
	e.open, e.running = false, true
	e.phase = killRunning
	e.end = w.now().Add(time.Duration(e.row.Extra) * time.Minute)
}

// killCancel is the "minimum not reached" branch. Death clears the
// player's obj+0x24f0/0x24f1/0x24f4 instead of its table entry, which
// stays registered until the player leaves (as in the original); Theft
// clears the entry but gives PK 6 / count 3 / PKDownRequirePoint3.
func (w *World) killCancel(e *killEvent) {
	w.eventPlayers(func(o *Object) {
		if u := e.users[o.Index]; u != nil {
			g := gmOf(o)
			g.SendHome = true
			if e.theft {
				delete(e.users, o.Index)
				w.eventMurderer(o)
			} else {
				g.InEvent = false
				w.eventPKLevel(o, PKCommoner)
			}
		}
		if e.theft {
			w.killNotice(e, o, 0x4B8)
			w.killNotice(e, o, 0x4BD)
		} else {
			w.killNotice(e, o, 0x464)
			w.killNotice(e, o, 0x469)
		}
	})
}

// killFinish is FUN_0045ca20 / FUN_00507150: the first registered playing
// player with the highest score (> 0) wins.
func (w *World) killFinish(e *killEvent) {
	var best *Object
	bestScore := 0
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		u := e.users[i]
		if o.State != StatePlaying || !e.running || u == nil {
			continue
		}
		if bestScore < u.Score {
			best, bestScore = o, u.Score
		}
	}
	if best == nil || bestScore < 1 {
		w.eventPlayers(func(o *Object) {
			w.killNotice(e, o, e.msgHeader())
			if e.theft {
				w.killNotice(e, o, 0x4C3)
			} else {
				w.killNotice(e, o, 0x46E)
			}
		})
	} else {
		if e.theft {
			w.eventBag(best, 0x20, 0)
			w.eventRanking(best, "TopTheft")
		} else {
			w.eventBag(best, 0x19, -1)
			w.eventRanking(best, "TopDeath")
		}
		w.eventPlayers(func(o *Object) {
			w.killNotice(e, o, e.msgHeader())
			if e.theft {
				w.killNotice(e, o, 0x4C2, best.P.Char.Name, bestScore)
			} else {
				w.killNotice(e, o, 0x46D, best.P.Char.Name, bestScore)
			}
		})
		if e.data.Patente {
			if e.theft {
				w.setPatente(best, 10)
			} else {
				w.setPatente(best, 9)
			}
		}
	}
	e.count = 0
	e.running = false
	w.killReset(e)
}

// killReset is FUN_0045cec0 / FUN_005075f0: every registered player
// leaves the table, goes home (obj+0x24f9) with PK 3 (Theft also PKTime
// 0) and is moved to Lorencia 125,125.
func (w *World) killReset(e *killEvent) {
	w.eventPlayers(func(o *Object) {
		if e.users[o.Index] == nil {
			return
		}
		delete(e.users, o.Index)
		gmOf(o).SendHome = true
		w.eventPKLevel(o, PKCommoner)
		if e.theft {
			o.P.Char.PKTime = 0
		}
		w.gmWarp(o, 0, 0x7D, 0x7D)
	})
}

// killJoin is FUN_0045bf60 / FUN_00506670 (the player command): false
// stops the line (the dispatcher continues only on a non-zero result).
func (w *World) killJoin(e *killEvent, o *Object) bool {
	if !e.open {
		w.eventNotice(o, NoticeBlue, 0x456)
		return false
	}
	switch {
	case o.P.Char.Level < e.data.Level:
		w.eventNotice(o, NoticeBlue, 0x457, e.data.Level)
		return false
	case w.inParty(o):
		w.eventNotice(o, NoticeBlue, 0x458)
		return false
	case gmOf(o).InEvent:
		w.eventNotice(o, NoticeBlue, 0x459)
		return false
	case !e.theft && e.users[o.Index] != nil:
		// Only the Death join tests its own table.
		w.eventNotice(o, NoticeBlue, 0x459)
		return false
	case inPlayerDuel(o):
		w.eventNotice(o, NoticeGold, 0x45A)
		return true
	}
	e.count++
	w.clearEffects(o)
	e.users[o.Index] = &killUser{Wait: true}
	w.eventMurderer(o)
	if e.theft {
		w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
		w.eventNotice(o, NoticeBlue, 0x45B)
	} else {
		w.eventNotice(o, NoticeBlue, 0x45B)
		w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
	}
	return true
}

// killScore is FUN_0045c900 / FUN_00506fe0: while the event runs a
// registered player killing another one scores. Death: +1 (0x46A to the
// killer); Theft: the victim's points (0x4BE to the killer, 0x4BF to the
// victim, whose score goes back to 1). The registered players get the
// score board (0x46B/0x46C, 0x4C0/0x4C1).
func (w *World) killScore(e *killEvent, killer, victim *Object) {
	ku, vu := e.users[killer.Index], e.users[victim.Index]
	if !e.running || ku == nil || vu == nil {
		return
	}
	if e.theft {
		ku.Score += vu.Score
		w.eventNotice(killer, NoticeBlue, 0x4BE, vu.Score, ku.Score)
		w.eventNotice(victim, NoticeBlue, 0x4BF, vu.Score)
		vu.Score = 1
	} else {
		ku.Score++
		w.eventNotice(killer, NoticeBlue, 0x46A, ku.Score)
	}
	w.eventPlayers(func(o *Object) {
		if e.users[o.Index] == nil {
			return
		}
		if e.theft {
			w.eventNotice(o, NoticeGold, 0x4C0)
			w.eventNotice(o, NoticeGold, 0x4C1, killer.P.Char.Name, ku.Score)
		} else {
			w.eventNotice(o, NoticeGold, 0x46B)
			w.eventNotice(o, NoticeGold, 0x46C, killer.P.Char.Name, ku.Score)
		}
	})
}

// killLeave is FUN_0045cfb0 / FUN_005076f0: the entry is cleared, the
// count drops, PK 3 and the player goes home.
func (w *World) killLeave(e *killEvent, o *Object) {
	if e.users[o.Index] == nil {
		return
	}
	delete(e.users, o.Index)
	gmOf(o).SendHome = true
	e.count--
	w.eventPKLevel(o, PKCommoner)
}
