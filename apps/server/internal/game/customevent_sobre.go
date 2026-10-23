package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Prainha da Morte (SobreEvent.txt, the "survivor" event): registered
// players wait on the event map, then fight; the last one standing wins.
// Globals at DAT_0104c260: +0 Active, +1 Patente, +2 state, +3 Syntax,
// +0x1c Level, +0x20 map/x/y, +0x23 minimum players, +0x24 class (always
// 5), +0x2c seconds, +0x30 the player vector, +0x40 the start rows.

const (
	sobreClosed = 0
	sobreOpen   = 1 // registration (minutes announced)
	sobreReady  = 2 // 6 second countdown
	sobreFight  = 3 // 300 seconds
	sobreEnd    = 4
)

type sobreEvent struct {
	data    *gamedata.SobreEvent
	state   int
	counter int
	class   int
	list    []int // object indexes in join order
}

func init() {
	commandHandlers[131] = (*World).commandSobreStart
	delete(pendingCommands, 131)
}

func (e *sobreEvent) has(o *Object) bool {
	for _, i := range e.list {
		if i == o.Index {
			return true
		}
	}
	return false
}

func (e *sobreEvent) remove(o *Object) {
	for k, i := range e.list {
		if i == o.Index {
			e.list = append(e.list[:k], e.list[k+1:]...)
			return
		}
	}
}

// sobreStart is FUN_005037a0: minutes 0 counts as 1.
func (e *sobreEvent) start(minutes int) {
	if minutes == 0 {
		minutes = 1
	}
	e.list = e.list[:0]
	e.state = sobreOpen
	e.counter = minutes*60 + 1
	e.class = 5
}

// commandSobreStart is /prainhapvp minutes (index 131, FUN_00503330): 0 or
// no number gives "Erro de sintaxe!" and "Digite: <tempo>"; with the event
// closed the GM goes to the event place and the registration opens. The
// command is not charged.
func (w *World) commandSobreStart(o *Object, c command, arg string) {
	e := &w.cev().sobre
	n := cmdInt(arg, 0)
	if n == 0 {
		w.eventText(o, NoticeBlue, "Erro de sintaxe!")
		w.eventText(o, NoticeBlue, "Digite: <tempo>")
		return
	}
	if e.state != sobreClosed {
		return
	}
	w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
	e.start(n)
}

// sobreJoin is FUN_00503490 (the player command); true stops the line,
// false lets it reach the CommandManager table (/prainhapvp is also the
// GM command 131, so a successful join goes on to the table too, as in
// the original).
func (w *World) sobreJoin(o *Object) bool {
	e := &w.cev().sobre
	if e.state != sobreOpen {
		return false
	}
	if e.class != 5 && o.Class != e.class {
		if n := eventClassMessage(e.class); n != 0 {
			w.eventNotice(o, NoticeBlue, n)
		}
		return true
	}
	switch {
	case e.has(o):
		w.eventNotice(o, NoticeBlue, 0x459)
		return true
	case o.P.Char.Level < e.data.Level:
		w.eventNotice(o, NoticeBlue, 0x457, e.data.Level)
		return true
	case w.inParty(o):
		w.eventNotice(o, NoticeBlue, 0x458)
		return true
	case inPlayerDuel(o):
		w.eventNotice(o, NoticeGold, 0x45A)
		return true
	}
	e.list = append(e.list, o.Index)
	gmOf(o).InEvent = true
	w.gmWarp(o, e.data.Place.Map, e.data.Place.X, e.data.Place.Y)
	w.clearEffects(o)
	w.eventMurderer(o)
	w.eventNotice(o, NoticeBlue, 0x45B)
	return false
}

// eventClassMessage is the "event for class X only" message 0x45C..0x460
// of the class-restricted joins (0 for class 5 = everybody).
func eventClassMessage(class int) int {
	if class >= 0 && class <= 4 {
		return 0x45C + class
	}
	return 0
}

// sobreSecond is FUN_00503810.
func (w *World) sobreSecond() {
	e := &w.cev().sobre
	if !e.data.Active {
		return
	}
	for _, s := range e.data.Starts {
		if w.eventScheduled(s) && e.state == sobreClosed {
			e.start(s.Time)
		}
	}
	if e.state == sobreClosed {
		return
	}
	if e.counter > 0 {
		e.counter--
	}
	switch e.state {
	case sobreOpen:
		if e.counter == 0 {
			if len(e.list) >= e.data.MinPlayers {
				w.eventGlobal(0x4AF)
				e.state, e.counter = sobreReady, 6
				return
			}
			// Not enough players: the registered ones go home with PK 3.
			w.eventPlayers(func(o *Object) {
				w.eventNotice(o, NoticeGold, 0x4AD)
				w.eventNotice(o, NoticeGold, 0x4AE)
				if g := gmOf(o); g.InEvent {
					g.InEvent, g.SendHome = false, true
					w.eventPKLevel(o, PKCommoner)
				}
			})
			e.state, e.counter = sobreClosed, 0
			return
		}
		if e.counter%60 == 0 {
			w.eventPlayers(func(o *Object) {
				w.eventNotice(o, NoticeGold, 0x4B0, e.counter/60)
				w.eventNotice(o, NoticeGold, 0x4B1, e.data.Syntax)
				w.eventNotice(o, NoticeGold, 0x4B2, e.data.MinPlayers)
			})
		}
	case sobreReady:
		if e.counter == 0 {
			w.eventGlobal(0x4B4)
			e.state, e.counter = sobreFight, 300
			return
		}
		if e.counter >= 1 && e.counter <= 5 {
			for _, i := range e.list {
				if o := w.objects[i]; o.State == StatePlaying {
					w.eventNotice(o, NoticeGold, 0x4B3, e.counter)
				}
			}
		}
	case sobreFight:
		if len(e.list) <= 1 || e.counter == 0 {
			e.state = sobreEnd
			if len(e.list) <= 1 {
				e.counter = 0
			}
		}
	case sobreEnd:
		w.sobreFinish()
	}
}

// sobreFinish is state 4 of FUN_00503810: with more than one survivor
// there is no winner (0x4B5; the vector is cleared, so 0x4B5 is sent a
// second time); with one, 0x4B6 (name), item bag 0x1F, ranking
// "topsurvivor", patente 11 when enabled, and the winner goes to
// Lorencia 125,125.
func (w *World) sobreFinish() {
	e := &w.cev().sobre
	if len(e.list) > 1 {
		w.eventGlobal(0x4B5)
		e.list = e.list[:0]
		e.state, e.counter = sobreClosed, 0
	}
	if len(e.list) == 0 {
		w.eventGlobal(0x4B5)
	} else {
		o := w.objects[e.list[0]]
		if o.IsPlayer() && o.P.Char != nil {
			g := gmOf(o)
			g.InEvent, g.SendHome = false, true
			w.eventPKLevel(o, PKCommoner)
			w.eventGlobal(0x4B6, o.P.Char.Name)
			w.eventBag(o, 0x1F, 0)
			w.eventRanking(o, "topsurvivor")
			if e.data.Patente {
				w.setPatente(o, 11)
			}
			w.gmWarp(o, 0, 0x7D, 0x7D)
		}
	}
	e.list = e.list[:0]
	e.state, e.counter = sobreClosed, 0
}

// sobreAttack is FUN_00503dc0: during the registration and the countdown
// a registered player cannot be attacked.
func (w *World) sobreAttack(t *Object) bool {
	e := &w.cev().sobre
	if (e.state == sobreOpen || e.state == sobreReady) && t.Type == TypePlayer && e.has(t) {
		return false
	}
	return true
}

// sobreDeath is FUN_00503e10: during the fight a registered player killed
// by a player is out: 0x4B7 (name) to everybody, PK and home restored on
// the next second, Lorencia 125,125.
func (w *World) sobreDeath(killer, victim *Object) {
	e := &w.cev().sobre
	if e.state != sobreFight || len(e.list) <= 1 || !gmOf(victim).InEvent || !e.has(victim) ||
		killer.Type != TypePlayer {
		return
	}
	w.eventGlobal(0x4B7, victim.P.Char.Name)
	g := gmOf(victim)
	g.RestorePK, g.SendHome, g.InEvent = true, true, false
	w.gmWarp(victim, 0, 0x7D, 0x7D)
	e.remove(victim)
}

// sobreLeave is FUN_00503ee0.
func (w *World) sobreLeave(o *Object) {
	e := &w.cev().sobre
	if e.state == sobreClosed || !e.has(o) {
		return
	}
	gmOf(o).InEvent = false
	w.eventPKLevel(o, PKCommoner)
	e.remove(o)
}
