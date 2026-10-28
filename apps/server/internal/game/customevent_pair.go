package game

import (
	"sort"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Mata-Mata de Duplas (PairEvent.txt): the registered players are sorted
// (std::sort of the object pointers, which follow the object index) and
// paired two by two; each round the first and the last team fight at
// FIGHT, three points win; the winners go on until one team is left.
// Globals at DAT_00fee628: +0 Active, +1 Patente, +2 Syntax, +0x1b state,
// +0x1c "go" sent, +0x1d.. three 0xFF text lines (FUN_004d1d10),
// +0x31c Level, +0x320 WAIT/FIGHT (interleaved), +0x326 class (always 5),
// +0x328 start rows count, +0x32c seconds, +0x330 registered vector,
// +0x340 teams vector and +0x350 winners vector (0x1C-byte team records),
// +0x360 and +0x37c the two fighting teams. A team record holds, for each
// member, dead/left/in-fight bytes, a revive countdown and the object,
// then the score byte. FUN_004cf3a0 GM start (/dupla), FUN_004cf510 join,
// FUN_004cf7b0 start, FUN_004cf8f0 timer, FUN_004d1240 attack check,
// FUN_004d1350 death, FUN_004d1410 revive, FUN_004d1580 leave.

const (
	pairClosed   = 0
	pairOpen     = 1
	pairPair     = 2
	pairFight    = 3
	pairDecide   = 4
	pairRound    = 5
	pairBroken   = 6
	pairChampion = 7
)

// pairMember is one half of a team record.
type pairMember struct {
	Index   int // -1 = nobody (a null object pointer)
	Dead    bool
	Left    bool
	InFight bool
	Revive  int
}

type pairTeam struct {
	M     [2]pairMember
	Score int
}

func newPairTeam(a, b int) pairTeam {
	return pairTeam{M: [2]pairMember{{Index: a}, {Index: b}}}
}

func (t *pairTeam) full() bool { return t.M[0].Index >= 0 && t.M[1].Index >= 0 }

type pairEvent struct {
	data    *gamedata.FightEvent
	state   int
	counter int
	goSent  bool
	list    []int
	teams   []pairTeam
	winners []pairTeam
	t       [2]pairTeam // the fighting teams (+0x360, +0x37c)
}

func registerCustomeventPair(h *hooks) {
	h.commandHandlers[133] = (*World).commandPairStart
}

func pairSyntax(w *World) string { return w.cev().pair.data.Syntax }

func (e *pairEvent) start(minutes int) {
	if minutes == 0 {
		minutes = 1
	}
	e.teams, e.winners, e.list = e.teams[:0], e.winners[:0], e.list[:0]
	e.state = pairOpen
	e.counter = minutes*60 + 1
}

func (w *World) pairObj(i int) *Object {
	if !validIdx(i) {
		return nil
	}
	if o := w.objects[i]; o.IsPlayer() && o.P.Char != nil {
		return o
	}
	return nil
}

// fighters returns the four fighters (nil when missing).
func (w *World) pairFighters(e *pairEvent) []*Object {
	var out []*Object
	for _, t := range e.t {
		for _, m := range t.M {
			if o := w.pairObj(m.Index); o != nil {
				out = append(out, o)
			}
		}
	}
	return out
}

func (w *World) pairName(i int) string {
	if o := w.pairObj(i); o != nil {
		return o.P.Char.Name
	}
	return ""
}

// pairLines sends the text lines built by FUN_004d1d10 to every playing
// player (gold notices).
func (w *World) pairLines(lines ...[]any) {
	w.eventPlayers(func(o *Object) {
		for _, l := range lines {
			w.eventNotice(o, NoticeGold, l[0].(int), l[1:]...)
		}
	})
}

func line(n int, args ...any) []any { return append([]any{n}, args...) }

// commandPairStart is /dupla minutes (index 133, FUN_004cf3a0). With the
// original PairEvent.txt the player syntax is also "/dupla" and the join
// always stops the line, so this GM command is never reached.
func (w *World) commandPairStart(o *Object, c command, arg string) {
	e := &w.cev().pair
	n := cmdInt(arg, 0)
	if n == 0 {
		w.eventText(o, NoticeBlue, "Erro de sintaxe!")
		w.eventText(o, NoticeBlue, "Digite: <tempo>")
		return
	}
	if e.state != pairClosed {
		return
	}
	w.gmWarp(o, e.data.Fight.Map, e.data.Fight.X, e.data.Fight.Y)
	e.start(n)
}

// pairJoin is FUN_004cf510: registration while the event is open and the
// player alive; the line always stops.
func (w *World) pairJoin(o *Object) bool {
	e := &w.cev().pair
	if e.state != pairOpen || !o.Alive {
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

// pairSecond is FUN_004cf8f0 followed by FUN_004d1410.
func (w *World) pairSecond() {
	e := &w.cev().pair
	if e.data.Active {
		w.pairTimer(e)
	}
	w.pairRevive(e)
}

func (w *World) pairTimer(e *pairEvent) {
	for _, s := range e.data.Starts {
		if w.eventScheduled(s) && e.state == pairClosed {
			e.start(s.Time)
		}
	}
	if e.state == pairClosed {
		return
	}
	if e.counter > 0 {
		e.counter--
	}
	switch e.state {
	case pairOpen:
		w.pairRegistration(e)
	case pairPair:
		if e.counter == 0 {
			w.pairPairing(e)
		}
	case pairFight:
		w.pairCountdown(e)
	case pairDecide:
		if e.counter == 0 {
			w.pairDecideRound(e)
		}
	case pairRound:
		e.teams = append(e.teams[:0], e.winners...)
		e.winners = e.winners[:0]
		e.t = [2]pairTeam{newPairTeam(-1, -1), newPairTeam(-1, -1)}
		switch n := len(e.teams); {
		case n == 0:
			e.state, e.counter = pairClosed, 0
		case n == 1:
			e.state, e.counter = pairChampion, 0
		default:
			if n%2 == 0 {
				w.eventGlobal(0x49B)
			} else {
				w.eventGlobal(0x49C)
			}
			e.state, e.counter = pairPair, 0
		}
	case pairBroken:
		w.pairBrokenTeam(e)
	case pairChampion:
		w.pairFinish(e)
	}
}

// pairRegistration is state 1: minute lines 0x488, 0x48D, 0x48E; at the
// end at least 4 players form teams (an odd one is out, 0x48A) and 0x48B
// / 0x48C close the registration, otherwise 0x489 cancels it.
func (w *World) pairRegistration(e *pairEvent) {
	if e.counter != 0 {
		if e.counter%60 == 0 {
			w.pairLines(line(0x488), line(0x48D, e.counter/60), line(0x48E, e.data.Syntax))
		}
		return
	}
	if len(e.list) > 3 {
		sort.Ints(e.list)
		for k := 0; k+1 < len(e.list); k += 2 {
			e.teams = append(e.teams, newPairTeam(e.list[k], e.list[k+1]))
		}
		if len(e.list)%2 == 1 {
			last := e.list[len(e.list)-1]
			if o := w.pairObj(last); o != nil {
				w.eventNotice(o, NoticeBlue, 0x48A)
				g := gmOf(o)
				g.SendHome, g.InEvent = true, false
			}
			e.list = e.list[:len(e.list)-1]
		}
		w.pairLines(line(0x48B), line(0x48C))
		e.state, e.counter = pairPair, 3
		return
	}
	w.eventPlayers(func(o *Object) {
		w.eventNotice(o, NoticeGold, 0x488)
		w.eventNotice(o, NoticeGold, 0x489)
		if g := gmOf(o); g.InEvent {
			g.InEvent, g.SendHome = false, true
		}
	})
	e.state, e.counter = pairClosed, 0
}

// pairPairing is state 2: incomplete teams at the front and the back are
// dropped; the first and the last team fight (0x488 + 0x491 names, 8
// second countdown), a single team goes on by walkover (0x48F / 0x490).
func (w *World) pairPairing(e *pairEvent) {
	if len(e.teams) == 0 {
		e.state, e.counter = pairRound, 0
		return
	}
	if len(e.teams) != 1 {
		for len(e.teams) > 0 && !e.teams[0].full() {
			e.teams = e.teams[1:]
		}
		for len(e.teams) > 0 && !e.teams[len(e.teams)-1].full() {
			e.teams = e.teams[:len(e.teams)-1]
		}
		if len(e.teams) > 1 {
			e.t[0], e.t[1] = e.teams[0], e.teams[len(e.teams)-1]
			for k := range e.t {
				e.t[k].Score = 0
				for m := range e.t[k].M {
					mm := &e.t[k].M[m]
					mm.Dead, mm.Left, mm.InFight = false, false, false
				}
			}
			names := []any{w.pairName(e.t[0].M[0].Index), w.pairName(e.t[0].M[1].Index),
				w.pairName(e.t[1].M[0].Index), w.pairName(e.t[1].M[1].Index)}
			w.pairLines(line(0x488), line(0x491, names...))
			e.state, e.counter, e.goSent = pairFight, 8, false
			e.teams = e.teams[1 : len(e.teams)-1]
			return
		}
		if len(e.teams) == 0 {
			e.state, e.counter = pairRound, 0
			return
		}
	}
	t := e.teams[0]
	e.t[0] = t
	a, b := w.pairName(t.M[0].Index), w.pairName(t.M[1].Index)
	switch {
	case a == "" && b != "":
		w.pairLines(line(0x488), line(0x490, b))
	case a != "" && b == "":
		w.pairLines(line(0x488), line(0x490, a))
	case a != "" && b != "":
		w.pairLines(line(0x488), line(0x48F, a, b))
	default:
		w.pairLines(line(0x488))
	}
	e.winners = append(e.winners, t)
	e.state, e.counter = pairRound, 0
}

// pairCountdown is state 3: at 5 the four fighters get 0x492, PK 6 and go
// to FIGHT (team 1 one row above, team 2 below, partners side by side);
// 3..1 is announced with 0x493 and 0 with 0x494.
func (w *World) pairCountdown(e *pairEvent) {
	switch e.counter {
	case 0:
		if !e.goSent {
			for _, o := range w.pairFighters(e) {
				w.eventNotice(o, NoticeGold, 0x494)
			}
			e.goSent = true
		}
	case 1, 2, 3:
		for _, o := range w.pairFighters(e) {
			w.eventNotice(o, NoticeGold, 0x493, e.counter)
		}
	case 5:
		f := e.data.Fight
		for k := range e.t {
			for m := range e.t[k].M {
				mm := &e.t[k].M[m]
				o := w.pairObj(mm.Index)
				if o == nil {
					continue
				}
				w.eventNotice(o, NoticeGold, 0x492)
				mm.InFight = true
				dx, dy := m*2-1, k*2-1
				w.gmWarp(o, f.Map, f.X+dx, f.Y+dy)
				w.eventMurderer(o)
			}
		}
	}
}

// pairWinnerIndex is the team decision of state 4: 0 for the first team
// (it still has a living first member unless that member left with the
// partner dead), 1 for the second; -1 when everybody is dead ("Draw!").
func (e *pairEvent) pairWinnerIndex() int {
	t1, t2 := &e.t[0], &e.t[1]
	decide := func() int {
		if !t1.M[0].Left || !t1.M[1].Dead {
			return 0
		}
		return 1
	}
	if !t1.M[0].Dead && t1.M[0].Index >= 0 {
		return decide()
	}
	if (!t1.M[1].Dead && t1.M[1].Index >= 0) || (!t2.M[0].Dead && t2.M[0].Index >= 0) ||
		(!t2.M[1].Dead && t2.M[1].Index >= 0) {
		if !t1.M[0].Dead || (!t1.M[1].Dead && !t1.M[1].Left) {
			return decide()
		}
		return 1
	}
	return -1
}

// pairDecideRound is state 4 (4 seconds after a team was wiped out): the
// winning team scores (0x496, 0x497 names, 0x498 scores); three points
// win the duel (0x499 / 0x49A to everybody, winners back to WAIT, losers
// out at Lorencia), otherwise a new 8 second countdown.
func (w *World) pairDecideRound(e *pairEvent) {
	defer func() {
		for k := range e.t {
			for m := range e.t[k].M {
				e.t[k].M[m].Dead = false
			}
		}
	}()
	wi := e.pairWinnerIndex()
	if wi < 0 {
		w.eventGlobal(0x495)
		e.state, e.counter = pairFight, 8
		return
	}
	win, lose := &e.t[wi], &e.t[1-wi]
	win.Score++
	lines := [][]any{line(0x496)}
	if e.t[0].full() && e.t[1].full() {
		lines = append(lines, line(0x497, w.pairName(e.t[0].M[0].Index), w.pairName(e.t[0].M[1].Index),
			w.pairName(e.t[1].M[0].Index), w.pairName(e.t[1].M[1].Index)))
	}
	lines = append(lines, line(0x498, e.t[0].Score, e.t[1].Score))
	w.pairLines(lines...)
	if win.Score < 3 {
		e.state, e.counter = pairFight, 8
		return
	}
	a, b := w.pairName(win.M[0].Index), w.pairName(win.M[1].Index)
	switch {
	case a != "" && b != "":
		w.eventGlobal(0x499, a, b)
	case a != "":
		w.eventGlobal(0x49A, a)
	case b != "":
		w.eventGlobal(0x49A, b)
	}
	e.winners = append(e.winners, *win)
	wd := e.data.Wait
	for m := range win.M {
		win.M[m].InFight = false
		if o := w.pairObj(win.M[m].Index); o != nil {
			w.eventPK(o, PKCommoner, 3, 0)
			w.gmWarp(o, wd.Map, wd.X, wd.Y)
		}
	}
	for m := range lose.M {
		lose.M[m].InFight = false
		if o := w.pairObj(lose.M[m].Index); o != nil {
			w.eventPK(o, PKCommoner, 3, 0)
			w.gmWarp(o, 0, 0x7D, 0x7D)
			gmOf(o).InEvent = false
		}
	}
	e.state, e.counter = pairPair, 0
}

// pairBrokenTeam is state 6: a fighter left; the complete team wins by
// walkover (0x48F / 0x490). Its first member goes back to WAIT and its
// second to FIGHT (as in the original); the other team is out.
func (w *World) pairBrokenTeam(e *pairEvent) {
	wi := 1
	if (!e.t[0].M[0].Left || !e.t[0].M[1].Left) && e.t[0].full() {
		wi = 0
	}
	win, lose := &e.t[wi], &e.t[1-wi]
	a, b := w.pairName(win.M[0].Index), w.pairName(win.M[1].Index)
	switch {
	case a != "" && b != "":
		w.eventGlobal(0x48F, a, b)
	case a != "":
		w.eventGlobal(0x490, a)
	case b != "":
		w.eventGlobal(0x490, b)
	}
	e.winners = append(e.winners, *win)
	for k := range e.t {
		for m := range e.t[k].M {
			e.t[k].M[m].InFight = false
		}
	}
	if o := w.pairObj(win.M[0].Index); o != nil {
		w.eventPK(o, PKCommoner, 3, 0)
		w.gmWarp(o, e.data.Wait.Map, e.data.Wait.X, e.data.Wait.Y)
	}
	if o := w.pairObj(win.M[1].Index); o != nil {
		w.eventPK(o, PKCommoner, 3, 0)
		w.gmWarp(o, e.data.Fight.Map, e.data.Fight.X, e.data.Fight.Y)
	}
	for m := range lose.M {
		if o := w.pairObj(lose.M[m].Index); o != nil {
			w.eventPK(o, PKCommoner, 3, 0)
			w.gmWarp(o, 0, 0x7D, 0x7D)
			gmOf(o).InEvent = false
		}
	}
	e.state, e.counter = pairPair, 0
}

// pairFinish is state 7: the champion team gets item bag 0x1C (both at the
// first member's cell), ranking "TopDupla" (+1 for the first member, +13
// for the second) and the patente (12 for the first, 10 for the second);
// 0x49D / 0x49E to everybody; both go to Lorencia 125,125.
func (w *World) pairFinish(e *pairEvent) {
	var a, b *Object
	if len(e.teams) > 0 {
		a, b = w.pairObj(e.teams[0].M[0].Index), w.pairObj(e.teams[0].M[1].Index)
	}
	if a != nil {
		w.eventBag(a, 0x1C, 0)
		w.eventRankingN(a, "TopDupla", 1)
		if e.data.Patente {
			w.setPatente(a, 12)
		}
	}
	if b != nil {
		if a != nil && w.hooks.itemBag != nil {
			w.hooks.itemBag(w, b, 0x1C, 0, a.Map, a.X, a.Y)
		}
		w.eventRankingN(b, "TopDupla", 13)
		if e.data.Patente {
			w.setPatente(b, 10)
		}
	}
	switch {
	case a != nil && b != nil:
		w.pairLines(line(0x488), line(0x49D, a.P.Char.Name, b.P.Char.Name))
	case a != nil:
		w.pairLines(line(0x488), line(0x49E, a.P.Char.Name))
	case b != nil:
		w.pairLines(line(0x488), line(0x49E, b.P.Char.Name))
	default:
		w.pairLines(line(0x488))
	}
	for _, o := range []*Object{a, b} {
		if o != nil {
			w.gmWarp(o, 0, 0x7D, 0x7D)
			gmOf(o).InEvent = false
		}
	}
	e.teams, e.winners, e.list = e.teams[:0], e.winners[:0], e.list[:0]
	e.state, e.counter = pairClosed, 0
}

// pairRevive is FUN_004d1410: 6 seconds after dying a fighter goes back to
// WAIT (when no teleport is pending, obj+0x287).
func (w *World) pairRevive(e *pairEvent) {
	if e.state != pairFight && e.state != pairDecide {
		return
	}
	for k := range e.t {
		for m := range e.t[k].M {
			mm := &e.t[k].M[m]
			if mm.Revive <= 0 {
				continue
			}
			mm.Revive--
			if mm.Revive == 1 {
				if o := w.pairObj(mm.Index); o != nil && !o.Teleporting {
					w.gmWarp(o, e.data.Wait.Map, e.data.Wait.X, e.data.Wait.Y)
					mm.Revive = 0
				}
			}
		}
	}
}

// pairAttack is FUN_004d1240 (a attacks t).
func (w *World) pairAttack(a, t *Object) bool {
	e := &w.cev().pair
	A1, A2 := e.t[0].M[0].Index, e.t[0].M[1].Index
	B1, B2 := e.t[1].M[0].Index, e.t[1].M[1].Index
	atk, tgt := a.Index, t.Index
	switch e.state {
	case pairPair, pairDecide, pairRound, pairBroken:
		if (A1 == atk && A2 == tgt) || (A2 == atk && A1 == tgt) || (B1 == atk && B2 == tgt) {
			return false
		}
		return B2 != atk || B1 != tgt
	case pairFight:
	default:
		return true
	}
	gotoB8 := false
	switch {
	case A1 == atk:
		if A2 == tgt {
			return false
		}
		gotoB8 = true
	case A1 == tgt:
		gotoB8 = true
	case A2 != atk:
		if A2 != tgt && B1 != atk && B1 != tgt && B2 != atk && B2 != tgt {
			return true
		}
		gotoB8 = true
	}
	if !gotoB8 || A2 == atk {
		if A1 == tgt {
			return false
		}
	}
	if (B1 == atk && B2 == tgt) || (B2 == atk && B1 == tgt) {
		return false
	}
	if ((A1 != atk && A2 != atk) || (B1 != tgt && B2 != tgt)) &&
		((B1 != atk && B2 != atk) || (A1 != tgt && A2 != tgt)) {
		return true
	}
	return e.counter == 0
}

// pairDeath is FUN_004d1350: a fighter died (revive in 7 seconds); a team
// with both members dead ends the round 4 seconds later.
func (w *World) pairDeath(victim *Object) {
	e := &w.cev().pair
	if e.state != pairFight && e.state != pairDecide {
		return
	}
	for k := range e.t {
		for m := range e.t[k].M {
			if mm := &e.t[k].M[m]; mm.Index == victim.Index {
				mm.Dead, mm.Revive = true, 7
			}
		}
	}
	dead := func(t *pairTeam) bool { return t.M[0].Dead && t.M[1].Dead }
	if e.state == pairFight && (dead(&e.t[0]) || dead(&e.t[1])) {
		e.state, e.counter = pairDecide, 4
	}
}

// pairLeave is FUN_004d1580.
func (w *World) pairLeave(o *Object) {
	e := &w.cev().pair
	if e.state == pairClosed {
		return
	}
	if k := indexIn(e.list, o); k >= 0 {
		w.eventPKLevel(o, PKCommoner)
		w.gmWarp(o, 0, 0x7D, 0x7D)
		gmOf(o).InEvent = false
		e.list = removeAt(e.list, k)
	}
	for k := range e.t {
		t := &e.t[k]
		for m := range t.M {
			if t.M[m].Index != o.Index {
				continue
			}
			if t.M[m].InFight {
				t.M[m].Left = true
				// Team 1: the position is only overwritten (no packet);
				// team 2: a real teleport.
				if k == 0 {
					o.Map, o.X, o.Y = 0, 0x7D, 0x7D
				} else {
					w.gmWarp(o, 0, 0x7D, 0x7D)
				}
				w.eventPK(o, PKCommoner, 3, 0)
				gmOf(o).InEvent = false
				t.M[m].Index = -1
			}
			if !t.full() {
				e.counter, e.state = 0, pairBroken
			}
			return
		}
	}
	leaveTeam := func(list []pairTeam) ([]pairTeam, bool) {
		for i := range list {
			t := &list[i]
			for m := range t.M {
				if t.M[m].Index != o.Index {
					continue
				}
				t.M[m].Index = -1
				w.gmWarp(o, 0, 0x7D, 0x7D)
				gmOf(o).InEvent = false
				for p := range t.M {
					if q := w.pairObj(t.M[p].Index); q != nil {
						gmOf(q).InEvent = false
						w.gmWarp(q, 0, 0x7D, 0x7D)
					}
				}
				return append(list[:i], list[i+1:]...), true
			}
		}
		return list, false
	}
	var done bool
	if e.teams, done = leaveTeam(e.teams); done {
		return
	}
	e.winners, _ = leaveTeam(e.winners)
}
