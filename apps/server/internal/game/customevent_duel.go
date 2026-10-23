package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// Mata-Mata (DuelEvent.txt): a knock-out tournament of one-on-one duels,
// optionally for one class. Registered players wait at WAIT; each round
// pairs the first and the last of the list, who fight at FIGHT (best of
// five: three wins); winners go on to the next round, the last one is the
// champion. Globals at DAT_007a01d8: +0 Active, +1 Patente, +2 Syntax,
// +0x1b state, +0x1c seconds, +0x20 "go" announced, +0x24 Level, +0x28
// WAIT and FIGHT (interleaved map/x/y bytes), +0x2e class, +0x30 start
// rows, +0x34 registered vector, +0x44 winners vector, +0x54/+0x5c the
// two fighters with their dead/left/in-fight/score bytes at +0x54..0x57 and
// +0x5c..0x5f. FUN_0045d9f0 GM start (/matamata), FUN_0045dcc0 join,
// FUN_0045e0c0 start, FUN_0045e160 timer, FUN_0045f5c0 attack check,
// FUN_0045f620 death, FUN_0045f680 leave.

const (
	mmClosed   = 0
	mmOpen     = 1
	mmPair     = 2
	mmFight    = 3
	mmDecide   = 4
	mmRound    = 5
	mmLeft     = 6
	mmChampion = 7
)

// mmFighter is one of the two fighters (+0x54 / +0x5c).
type mmFighter struct {
	Index   int
	Dead    bool // +0
	Left    bool // +1
	InFight bool // +2
	Score   int  // +3
}

type mmEvent struct {
	data    *gamedata.FightEvent
	state   int
	counter int
	class   int
	goSent  bool // +0x20
	list    []int
	winners []int
	a, b    mmFighter
}

func init() {
	commandHandlers[132] = (*World).commandMataMataStart
	delete(pendingCommands, 132)
}

func indexIn(list []int, o *Object) int {
	for k, i := range list {
		if i == o.Index {
			return k
		}
	}
	return -1
}

func removeAt(list []int, k int) []int { return append(list[:k], list[k+1:]...) }

// start is FUN_0045e0c0: minutes 0 counts as 1.
func (e *mmEvent) start(minutes, class int) {
	if minutes == 0 {
		minutes = 1
	}
	e.list, e.winners = e.list[:0], e.winners[:0]
	e.state = mmOpen
	e.counter = minutes*60 + 1
	e.class = class
}

// commandMataMataStart is /matamata minutes class (index 132, FUN_0045d9f0).
// Not charged; class 0 cannot be given (0 reads as "missing").
func (w *World) commandMataMataStart(o *Object, c command, arg string) {
	e := &w.cev().duel
	n, class := cmdInt(arg, 0), cmdInt(arg, 1)
	switch {
	case n == 0:
		w.eventText(o, NoticeBlue, "Erro de sintaxe!")
		w.eventText(o, NoticeBlue, "Digite: <tempo> <class>")
	case class == 0:
		w.eventText(o, NoticeBlue, "Exemplo -> Evento para BKs: 3 1")
		w.eventText(o, NoticeBlue, "ID de classes: ")
		w.eventText(o, NoticeBlue, "0: Wizard    | 1: Knight    | 2: Elf")
		w.eventText(o, NoticeBlue, "3: Gladiator | 4: Dark Lord | 5: Todos")
	case e.state == mmClosed:
		w.gmWarp(o, e.data.Fight.Map, e.data.Fight.X, e.data.Fight.Y)
		e.start(n, class)
	}
}

// mmJoin is FUN_0045dcc0: it always stops the line.
func (w *World) mmJoin(o *Object) bool {
	e := &w.cev().duel
	if e.state != mmOpen {
		return true
	}
	if e.class != 5 && o.Class != e.class {
		if n := eventClassMessage(e.class); n != 0 {
			w.eventNotice(o, NoticeBlue, n)
		}
		return true
	}
	switch {
	case indexIn(e.list, o) >= 0:
		w.eventNotice(o, NoticeBlue, 0x459)
	case o.P.Char.Level < e.data.Level:
		w.eventNotice(o, NoticeBlue, 0x457, e.data.Level)
	case inPlayerDuel(o):
		w.eventNotice(o, NoticeGold, 0x45A)
	default:
		w.eventLeaveParty(o)
		gmOf(o).InEvent = true
		e.list = append(e.list, o.Index)
		w.clearEffects(o)
		w.gmWarp(o, e.data.Wait.Map, e.data.Wait.X, e.data.Wait.Y)
		w.eventNotice(o, NoticeBlue, 0x45B)
	}
	return true
}

// mmHeader is the class header of the notices: 0x46F and the class
// line 0x470..0x474 (class 5: only 0x46F; another value: nothing).
func (w *World) mmHeader(o *Object, class int) {
	switch {
	case class >= 0 && class <= 4:
		w.eventNotice(o, NoticeGold, 0x46F)
		w.eventNotice(o, NoticeGold, 0x470+class)
	case class == 5:
		w.eventNotice(o, NoticeGold, 0x46F)
	}
}

func (w *World) mmObj(i int) *Object {
	if !validIdx(i) {
		return nil
	}
	o := w.objects[i]
	if !o.IsPlayer() || o.P.Char == nil {
		return nil
	}
	return o
}

// mmValid is the test of the pairing loop: obj+0x14 != -1 and obj+4 ==
// 3 (a playing object).
func (w *World) mmValid(i int) bool {
	o := w.mmObj(i)
	return o != nil && o.State == StatePlaying
}

// mmSecond is FUN_0045e160.
func (w *World) mmSecond() {
	e := &w.cev().duel
	if !e.data.Active {
		return
	}
	for _, s := range e.data.Starts {
		if w.eventScheduled(s) && e.state == mmClosed {
			e.start(s.Time, s.Extra)
		}
	}
	if e.state == mmClosed {
		return
	}
	if e.counter > 0 {
		e.counter--
	}
	switch e.state {
	case mmOpen:
		w.mmRegistration(e)
	case mmPair:
		if e.counter == 0 {
			w.mmPairing(e)
		}
	case mmFight:
		a, b := w.mmObj(e.a.Index), w.mmObj(e.b.Index)
		if e.counter == 0 {
			if !e.goSent {
				for _, o := range []*Object{a, b} {
					if o != nil {
						w.eventNotice(o, NoticeGold, 0x47E)
					}
				}
				e.goSent = true
			}
		} else if e.counter >= 1 && e.counter <= 3 {
			for _, o := range []*Object{a, b} {
				if o != nil {
					w.eventNotice(o, NoticeGold, 0x47D, e.counter)
				}
			}
		}
	case mmDecide:
		if e.counter == 0 {
			w.mmDecideRound(e)
		}
	case mmRound:
		e.list = append(e.list[:0], e.winners...)
		e.winners = e.winners[:0]
		e.a, e.b = mmFighter{Index: -1}, mmFighter{Index: -1}
		switch n := len(e.list); {
		case n == 0:
			e.state, e.counter = mmClosed, 0
		case n == 1:
			e.state, e.counter = mmChampion, 0
		default:
			if n%2 == 0 {
				w.eventGlobal(0x484)
			} else {
				w.eventGlobal(0x485)
			}
			e.state, e.counter = mmPair, 0
		}
	case mmLeft:
		winner, loser := &e.a, &e.b
		if e.a.Left {
			winner, loser = &e.b, &e.a
		}
		w.mmRoundWinner(e, winner, loser)
	case mmChampion:
		w.mmFinish(e)
	}
}

// mmRegistration is state 1: the minute announcements; at the end two
// or more players close the registration (0x476, 0x477; pairing in 3
// seconds), otherwise the event is cancelled (0x475) and the registered
// players go home.
func (w *World) mmRegistration(e *mmEvent) {
	if e.counter != 0 {
		if e.counter%60 == 0 {
			w.eventPlayers(func(o *Object) {
				w.mmHeader(o, e.class)
				w.eventNotice(o, NoticeGold, 0x478, e.counter/60)
				w.eventNotice(o, NoticeGold, 0x479, e.data.Syntax)
			})
		}
		return
	}
	if len(e.list) > 1 {
		w.eventPlayers(func(o *Object) {
			w.eventNotice(o, NoticeGold, 0x476)
			w.eventNotice(o, NoticeGold, 0x477)
		})
		e.state, e.counter = mmPair, 3
		return
	}
	w.eventPlayers(func(o *Object) {
		w.mmHeader(o, e.class)
		w.eventNotice(o, NoticeGold, 0x475)
		if g := gmOf(o); g.InEvent {
			g.InEvent, g.SendHome = false, true
		}
	})
	e.state, e.counter = mmClosed, 0
}

// mmPairing is state 2: the first and the last valid players of the
// list fight (0x47B with both names to everybody, 0x47C to both, PK 6,
// FIGHT place one cell apart, 6 second countdown); a single player goes
// on by walkover (0x47A).
func (w *World) mmPairing(e *mmEvent) {
	if len(e.list) == 0 {
		e.state, e.counter = mmRound, 0
		return
	}
	if len(e.list) > 1 {
		for len(e.list) > 0 && !w.mmValid(e.list[0]) {
			e.list = e.list[1:]
		}
		for len(e.list) > 0 && !w.mmValid(e.list[len(e.list)-1]) {
			e.list = e.list[:len(e.list)-1]
		}
		if len(e.list) > 1 {
			a, b := w.objects[e.list[0]], w.objects[e.list[len(e.list)-1]]
			e.a = mmFighter{Index: a.Index, InFight: true}
			e.b = mmFighter{Index: b.Index, InFight: true}
			w.eventPlayers(func(o *Object) {
				w.mmHeader(o, e.class)
				w.eventNotice(o, NoticeGold, 0x47B, a.P.Char.Name, b.P.Char.Name)
			})
			w.eventNotice(a, NoticeGold, 0x47C)
			w.eventNotice(b, NoticeGold, 0x47C)
			e.goSent = false
			e.state, e.counter = mmFight, 6
			f := e.data.Fight
			w.gmWarp(a, f.Map, f.X, f.Y-1)
			w.gmWarp(b, f.Map, f.X, f.Y+1)
			w.eventMurderer(a)
			w.eventMurderer(b)
			e.list = e.list[1 : len(e.list)-1]
			return
		}
		if len(e.list) == 0 {
			e.state, e.counter = mmRound, 0
			return
		}
	}
	a := w.objects[e.list[0]]
	e.a = mmFighter{Index: a.Index}
	w.eventPlayers(func(o *Object) {
		w.mmHeader(o, e.class)
		w.eventNotice(o, NoticeGold, 0x47A, a.P.Char.Name)
	})
	e.winners = append(e.winners, a.Index)
	e.state, e.counter = mmRound, 0
}

// mmDecideRound is state 4 (4 seconds after a death): with one fighter
// dead the other scores (0x480, 0x481 names, 0x482 scores to everybody);
// three points win the duel, otherwise (or with both dead, 0x47F "Draw!")
// the duel goes on after a new 6 second countdown.
func (w *World) mmDecideRound(e *mmEvent) {
	a, b := w.mmObj(e.a.Index), w.mmObj(e.b.Index)
	if !(e.a.Dead && e.b.Dead) {
		winner, loser := &e.a, &e.b
		if e.a.Dead {
			winner, loser = &e.b, &e.a
		}
		winner.Score++
		w.eventPlayers(func(o *Object) {
			w.eventNotice(o, NoticeGold, 0x480)
			if a != nil && b != nil {
				w.eventNotice(o, NoticeGold, 0x481, a.P.Char.Name, b.P.Char.Name)
			}
			w.eventNotice(o, NoticeGold, 0x482, e.a.Score, e.b.Score)
		})
		if winner.Score > 2 {
			w.mmRoundWinner(e, winner, loser)
			return
		}
		for _, o := range []*Object{a, b} {
			if o != nil {
				w.eventNotice(o, NoticeGold, 0x47C)
			}
		}
	} else {
		w.eventGlobal(0x47F)
	}
	e.counter, e.state, e.goSent = 6, mmFight, false
	e.a.Dead, e.b.Dead = false, false
}

// mmRoundWinner ends a duel (states 4 and 6): 0x483 (name) to
// everybody, the winner joins the winners and goes back to WAIT, the
// loser leaves the event at Lorencia 125,125; both get PK 3 / count 3 /
// time 0.
func (w *World) mmRoundWinner(e *mmEvent, winner, loser *mmFighter) {
	wo, lo := w.mmObj(winner.Index), w.mmObj(loser.Index)
	if wo != nil {
		w.eventGlobal(0x483, wo.P.Char.Name)
		e.winners = append(e.winners, wo.Index)
	}
	for _, o := range []*Object{w.mmObj(e.a.Index), w.mmObj(e.b.Index)} {
		if o != nil {
			w.eventPK(o, PKCommoner, 3, 0)
		}
	}
	e.a.Score, e.b.Score = 0, 0
	if wo != nil {
		w.gmWarp(wo, e.data.Wait.Map, e.data.Wait.X, e.data.Wait.Y)
	}
	if lo != nil {
		w.gmWarp(lo, 0, 0x7D, 0x7D)
		gmOf(lo).InEvent = false
	}
	e.state, e.counter = mmPair, 0
	e.a.Dead, e.b.Dead = false, false
}

// mmFinish is state 7: the champion (item bag 0x1A, ranking Duelsm,
// Duelbk, Duelelf, Duelmg, Dueldl or Duelall by the event class, patente
// 3..8 when enabled, 0x487 to everybody, Lorencia 125,125), or 0x486 when
// nobody is left.
func (w *World) mmFinish(e *mmEvent) {
	if len(e.list) == 0 {
		w.eventGlobal(0x486)
	} else if o := w.mmObj(e.list[0]); o != nil {
		w.eventBag(o, 0x1A, 0)
		ranks := [...]string{"Duelsm", "Duelbk", "Duelelf", "Duelmg", "Dueldl", "Duelall"}
		if e.class >= 0 && e.class <= 5 {
			w.eventRanking(o, ranks[e.class])
			if e.data.Patente {
				w.setPatente(o, 3+e.class)
			}
		}
		w.eventPlayers(func(t *Object) { w.eventNotice(t, NoticeGold, 0x487, o.P.Char.Name) })
		w.gmWarp(o, 0, 0x7D, 0x7D)
		gmOf(o).InEvent = false
	}
	e.list, e.winners = e.list[:0], e.winners[:0]
	e.state, e.counter = mmClosed, 0
}

// mmAttack is FUN_0045f5c0 (a attacks t): during a duel the two
// fighters may only hit each other, and only after the countdown; nobody
// else may hit them.
func (w *World) mmAttack(a, t *Object) bool {
	e := &w.cev().duel
	if e.state != mmFight {
		return true
	}
	A, B := e.a.Index, e.b.Index
	ai, ti := a.Index, t.Index
	switch {
	case A == ai:
		if B == ti {
			return e.counter == 0
		}
	case B != ai:
		if A != ti && B != ti {
			return true
		}
	default: // B == ai
		if A == ti {
			return e.counter == 0
		}
	}
	return false
}

// mmDeath is FUN_0045f620: a fighter died during the duel; in state 3
// the round is decided 4 seconds later.
func (w *World) mmDeath(victim *Object) {
	e := &w.cev().duel
	if e.state != mmFight && e.state != mmDecide {
		return
	}
	if e.a.Index == victim.Index {
		e.a.Dead = true
	}
	if e.b.Index == victim.Index {
		e.b.Dead = true
	}
	if e.state == mmFight && (e.a.Dead || e.b.Dead) {
		e.state, e.counter = mmDecide, 4
	}
}

// mmLeave is FUN_0045f680: a fighter leaving during the duel loses it
// (state 6); a waiting player leaves the list or the winners.
func (w *World) mmLeave(o *Object) {
	e := &w.cev().duel
	if e.state == mmClosed {
		return
	}
	for _, f := range []*mmFighter{&e.a, &e.b} {
		if f.Index == o.Index {
			if f.InFight {
				f.Left = true
				gmOf(o).SendHome = true
				w.eventPK(o, PKCommoner, 3, 0)
				e.counter, e.state = 0, mmLeft
			}
			return
		}
	}
	out := func() {
		g := gmOf(o)
		g.SendHome, g.InEvent = true, false
		w.eventPK(o, PKCommoner, 3, 0)
	}
	if k := indexIn(e.list, o); k >= 0 {
		out()
		e.list = removeAt(e.list, k)
	}
	if k := indexIn(e.winners, o); k >= 0 {
		out()
		e.winners = removeAt(e.winners, k)
	}
}
