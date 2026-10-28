package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Roleta Russa (RoletaEvent.txt): registered players go to the event place
// and, once the event starts, take turns asking the GM for a trade: each
// request pulls the trigger, one chance in six eliminates the player. The
// last one left wins. Globals at DAT_00ff43e0: Syntax, +0x19 Active, +0x1c
// Level, +0x20 map/x/y, +0x23 state, +0x28 seconds, +0x2c shot steps,
// +0x30 the player vector; per-index table DAT_00ff4420 (+0 registered, +4
// index). FUN_004e46a0 GM start (/rodar), FUN_004e4800 join, FUN_004e4a20
// start, FUN_004e4a80 timer, FUN_004e4e70 trade (shot), FUN_004e4ed0 the
// shot thread, FUN_004e51d0 elimination, FUN_004e5260 leave.

const (
	roletaClosed  = 0
	roletaOpen    = 1
	roletaReady   = 2
	roletaRunning = 3
	roletaEnd     = 4
)

type roletaEvent struct {
	data    *gamedata.RoletaEvent
	state   int
	counter int
	list    []int
	flags   map[int]bool
	// shot is DAT_00ff440c (steps left) and shooter the index of the
	// player who asked (the +4 field of its table entry).
	shot    int
	shooter int
}

func registerCustomeventRoleta(h *hooks) {
	h.commandHandlers[135] = (*World).commandRoletaStart
}

func (e *roletaEvent) has(o *Object) bool {
	for _, i := range e.list {
		if i == o.Index {
			return true
		}
	}
	return false
}

func (e *roletaEvent) remove(o *Object) {
	for k, i := range e.list {
		if i == o.Index {
			e.list = append(e.list[:k], e.list[k+1:]...)
			return
		}
	}
}

// commandRoletaStart is /rodar minutes (index 135, FUN_004e46a0). Not
// charged.
func (w *World) commandRoletaStart(o *Object, c command, arg string) {
	e := &w.cev().roleta
	n := cmdInt(arg, 0)
	if n == 0 {
		w.eventText(o, NoticeBlue, "Erro de sintaxe!")
		w.eventText(o, NoticeBlue, "Digite: <tempo>")
		return
	}
	if e.state != roletaClosed {
		return
	}
	w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
	// FUN_004e4a20.
	e.list = e.list[:0]
	e.state = roletaOpen
	e.counter = n*60 + 1
}

// roletaJoin is FUN_004e4800; true stops the line.
func (w *World) roletaJoin(o *Object) bool {
	e := &w.cev().roleta
	if e.state != roletaOpen {
		return false
	}
	switch {
	case e.has(o):
		w.eventNotice(o, NoticeGold, 0x459)
		return true
	case o.P.Char.Level < e.data.Level:
		w.eventNotice(o, NoticeGold, 0x457, e.data.Level)
		return true
	case inPlayerDuel(o):
		w.eventNotice(o, NoticeGold, 0x45A)
		return true
	}
	e.list = append(e.list, o.Index)
	e.flags[o.Index] = true
	w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
	w.eventNotice(o, NoticeGold, 0x45B)
	return false
}

// roletaSecond is FUN_004e4a80 (and the 1-second steps of the shot
// thread FUN_004e4ed0).
func (w *World) roletaSecond() {
	e := &w.cev().roleta
	w.roletaShotStep()
	if !e.data.Active || e.state == roletaClosed {
		return
	}
	if e.counter > 0 {
		e.counter--
	}
	switch e.state {
	case roletaOpen:
		if e.counter == 0 {
			if len(e.list) > 1 {
				w.eventGlobal(0x4A1)
				e.state, e.counter = roletaReady, 6
				return
			}
			w.eventPlayers(func(o *Object) {
				w.eventNotice(o, NoticeGold, 0x49F)
				w.eventNotice(o, NoticeGold, 0x4A0)
				if e.flags[o.Index] {
					delete(e.flags, o.Index)
					gmOf(o).SendHome = true
				}
			})
			e.state, e.counter = roletaClosed, 0
			return
		}
		if e.counter%60 == 0 {
			w.eventPlayers(func(o *Object) {
				w.eventNotice(o, NoticeGold, 0x49F)
				w.eventNotice(o, NoticeGold, 0x4A2, e.counter/60)
				w.eventNotice(o, NoticeGold, 0x4A3, e.data.Syntax)
			})
		}
	case roletaReady:
		if e.counter == 0 {
			w.eventGlobal(0x4A5)
			e.state = roletaRunning
			return
		}
		if e.counter >= 1 && e.counter <= 5 {
			w.eventPlayers(func(o *Object) {
				w.eventNotice(o, NoticeGold, 0x49F)
				w.eventNotice(o, NoticeGold, 0x4A4, e.counter)
			})
		}
	case roletaRunning:
		if len(e.list) <= 1 {
			e.state, e.counter = roletaEnd, 0
		}
	case roletaEnd:
		// The original reads the first entry without checking the vector
		// (an empty one would crash); here nobody wins then.
		if len(e.list) > 0 {
			o := w.objects[e.list[0]]
			if o.IsPlayer() && o.P.Char != nil {
				w.eventBag(o, 0x1E, -1)
				w.eventRanking(o, "TopRoleta")
				delete(e.flags, o.Index)
				w.eventPlayers(func(t *Object) {
					w.eventNotice(t, NoticeGold, 0x49F)
					w.eventNotice(t, NoticeGold, 0x4A6, o.P.Char.Name)
				})
				fw := packetFirework(0, byte(o.X), byte(o.Y))
				w.send(o, fw)
				w.broadcast(o, fw)
			}
		}
		e.list = e.list[:0]
		e.state, e.counter = roletaClosed, 0
	}
}

// roletaTrade is FUN_004e4e70: while the event runs and no shot is in
// progress, a registered player asking a GM (Authority > 1) for a trade
// pulls the trigger (five steps, one per second); the trade itself is
// dropped.
func (w *World) roletaTrade(o, t *Object) bool {
	e := &w.cev().roleta
	if e.state != roletaRunning || e.shot != 0 || gmAuthority(t) <= 1 || !e.flags[o.Index] {
		return false
	}
	e.shot = 5
	e.shooter = o.Index
	return true
}

// roletaShotStep is one Sleep(1000) step of FUN_004e4ed0: 0x4A7..0x4AA to
// everybody, then rand()%6 == 3 eliminates the player (0x4AB, name) and
// anything else spares him (0x4AC).
func (w *World) roletaShotStep() {
	e := &w.cev().roleta
	if e.shot < 1 {
		return
	}
	e.shot--
	switch e.shot {
	case 4:
		w.eventGlobal(0x4A7)
	case 3:
		w.eventGlobal(0x4A8)
	case 2:
		w.eventGlobal(0x4A9)
	case 1:
		w.eventGlobal(0x4AA)
	case 0:
		o := w.objects[e.shooter]
		if !o.IsPlayer() || o.P.Char == nil {
			return
		}
		if monsterRand()%6 == 3 {
			w.eventGlobal(0x4AB, o.P.Char.Name)
			w.roletaEliminate(o)
		} else {
			w.eventGlobal(0x4AC, o.P.Char.Name)
		}
	}
}

// roletaEliminate is FUN_004e51d0: the player dies (obj+0x6e = 0, die
// tick, C1 08 17 die packet), goes home and leaves the event.
func (w *World) roletaEliminate(o *Object) {
	e := &w.cev().roleta
	if !e.has(o) {
		return
	}
	o.Alive = false
	o.Live = LiveDying
	w.mw().deaths[o.Index] = &deadState{Die: 1, Time: w.now(), KillerID: o.Index}
	p := packetDie(o.Index, 0, 0)
	w.send(o, p)
	w.broadcast(o, p)
	gmOf(o).SendHome = true
	w.gmWarp(o, 0, 0x7D, 0x7D)
	delete(e.flags, o.Index)
	e.remove(o)
}

// roletaLeave is FUN_004e5260.
func (w *World) roletaLeave(o *Object) {
	e := &w.cev().roleta
	if e.state == roletaClosed || !e.has(o) {
		return
	}
	gmOf(o).SendHome = true
	delete(e.flags, o.Index)
	e.remove(o)
}
