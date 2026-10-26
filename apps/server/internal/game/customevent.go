package game

import (
	"context"
	"log/slog"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Custom events (Data/Custom/Event and the hide and seek of
// GameServerInfo - Event.dat): the pieces they share and their hooks into
// the rest of the server.
//
//   - join commands: FUN_00435400 compares the first word of a "/" line
//     with the Syntax of SobreEvent (FUN_00503490), DuelEvent
//     (FUN_0045dcc0), PairEvent (FUN_004cf510), RoletaEvent (FUN_004e4800),
//     TheftEvent (FUN_00506670) and DeathEvent (FUN_0045bf60) in this order,
//     before the hard-coded commands and the CommandManager table; each
//     join function decides whether the line goes on to the table;
//   - attacks: FUN_0040b110 refuses the Death and Theft waiting players,
//     FUN_00503dc0 (Sobre), FUN_0045f5c0 (Duel) and FUN_004d1240 (Pair);
//   - deaths: FUN_0050f330 calls the Theft and Death kill counters
//     (FUN_00506fe0, FUN_0045c900) and FUN_0050e8a0 the Sobre, Duel and
//     Pair deaths (FUN_00503e10, FUN_0045f620, FUN_004d1350);
//   - leaving (DelCharacterInfo FUN_004c4b40, DelClient FUN_0048a010 and
//     gObjMoveGate FUN_0050f6d0): FUN_00503ee0, FUN_005076f0, FUN_0045cfb0,
//     FUN_0045f680, FUN_004d1580 and FUN_004e5260;
//   - trade, party and duel requests (36, 40, AA in FUN_004d6690) are
//     refused for players of an event (obj+0x24f0 or the Theft, Death and
//     Roleta tables): "Proibido dar trade no evento." etc.;
//   - the 1-second player proc FUN_0050e280 sends home (obj+0x24f9) and
//     restores the PK level (obj+0x24f8) of players that left an event.

func init() {
	startHooks = append(startHooks, loadCustomEvents)
	tick1000Hooks = append(tick1000Hooks, (*World).customEventSecond)
	commandEventHook = (*World).customEventCommand
	customEventAttackHook = (*World).customEventAttack
	packetBlockHook = (*World).customEventPacketBlock
	tradeRequestHook = (*World).customEventTrade
	commandEventBlockHook = func(w *World, o *Object) bool { return w.inCustomEvent(o) }
	deathHooks = append(deathHooks, (*World).customEventDeath)
	beforeGateMoveHooks = append(beforeGateMoveHooks, (*World).customEventLeave)
	leaveWorldHooks = append(leaveWorldHooks, (*World).customEventLeave)
}

// Hooks owned by this file and called from shared code.
var (
	// commandEventHook runs before the commands of FUN_00435400 (name is
	// the first word); true stops the line.
	commandEventHook func(w *World, o *Object, name string) bool
	// customEventAttackHook is false when an event forbids the attack
	// (pvp.go attackFull).
	customEventAttackHook func(w *World, a, t *Object) bool
	// tradeRequestHook is true when the trade request was consumed or
	// refused (trade.go handleTradeRequest).
	tradeRequestHook func(w *World, o, t *Object) bool
)

// customEvents holds the custom events of an instance.
type customEvents struct {
	sobre  sobreEvent
	death  killEvent
	theft  killEvent
	duel   mmEvent
	roleta roletaEvent
	raffle *gamedata.RaffleEvent
	hide   hideSeek
	pair   pairEvent
	world  *worldCustoms // custom_world.go
}

func (w *World) cev() *customEvents {
	if w.cevt == nil {
		w.cevt = newCustomEvents()
	}
	return w.cevt
}

func newCustomEvents() *customEvents {
	c := &customEvents{
		sobre:  sobreEvent{data: &gamedata.SobreEvent{}},
		death:  killEvent{data: &gamedata.KillEvent{}, users: map[int]*killUser{}},
		theft:  killEvent{theft: true, data: &gamedata.KillEvent{}, users: map[int]*killUser{}},
		duel:   mmEvent{data: &gamedata.FightEvent{}},
		roleta: roletaEvent{data: &gamedata.RoletaEvent{}, flags: map[int]bool{}},
		raffle: &gamedata.RaffleEvent{},
		pair:   pairEvent{data: &gamedata.FightEvent{}},
	}
	c.hide.timeMin = -1
	return c
}

// loadCustomEvents is the custom event part of FUN_004e8390.
func loadCustomEvents(w *World) error {
	c := newCustomEvents()
	if w.cevt != nil {
		c.world = w.cevt.world // loaded by custom_world.go
	}
	w.cevt = c
	if w.configDir == "" {
		return nil
	}
	var err error
	if c.sobre.data, err = gamedata.LoadSobreEvent(w.configDir); err != nil {
		return err
	}
	if c.death.data, err = gamedata.LoadDeathEvent(w.configDir); err != nil {
		return err
	}
	if c.theft.data, err = gamedata.LoadTheftEvent(w.configDir); err != nil {
		return err
	}
	if c.duel.data, err = gamedata.LoadDuelEvent(w.configDir); err != nil {
		return err
	}
	if c.roleta.data, err = gamedata.LoadRoletaEvent(w.configDir); err != nil {
		return err
	}
	if c.pair.data, err = gamedata.LoadPairEvent(w.configDir); err != nil {
		return err
	}
	c.raffle, err = gamedata.LoadRaffleEvent(w.configDir)
	return err
}

// customEventSecond runs the custom event timers: the FUN_005107e0 thread
// (Death, Theft and Raffle, Sleep(1000)), the event part of FUN_00510810
// (Sobre, Duel, Roleta), the hide and seek clock (FUN_0045fbf0) and the
// per-player FUN_0050e280.
func (w *World) customEventSecond() {
	c := w.cev()
	w.killSecond(&c.death)
	w.killSecond(&c.theft)
	w.raffleSecond()
	w.sobreSecond()
	w.mmSecond()
	w.pairSecond()
	w.roletaSecond()
	w.hideSecond()
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil {
			w.customEventReturn(o)
		}
	}
}

// customEventReturn is the event part of FUN_0050e280: an alive player
// with obj+0x24f9 goes home (Elves to Noria 171+rand%6, 108+rand&7; the
// others to Lorencia 133+rand%15, 118+rand%15); obj+0x24f8 brings the PK
// level back to 3 (C1 07 F3 08).
func (w *World) customEventReturn(o *Object) {
	g := gmOf(o)
	if g.SendHome && o.Alive {
		if o.Class == 2 {
			x := monsterRand()%6 + 0xAB
			y := monsterRand()&7 + 0x6C
			w.gmWarp(o, 3, x, y)
		} else {
			x := monsterRand()%0xF + 0x85
			y := monsterRand()%0xF + 0x76
			w.gmWarp(o, 0, x, y)
		}
		g.SendHome = false
	}
	if g.RestorePK && o.Alive {
		o.PKLevel, o.P.Char.PKLevel = PKCommoner, PKCommoner
		w.sendPKLevel(o)
		g.RestorePK = false
	}
}

// --- shared helpers -----------------------------------------------------------------------

// eventPlayers calls f for every playing player (obj+4 == 3).
func (w *World) eventPlayers(f func(o *Object)) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil {
			f(o)
		}
	}
}

// eventNotice is FUN_004c07f0(index, type, ...) with a Message.txt text.
func (w *World) eventNotice(o *Object, typ byte, n int, args ...any) {
	w.send(o, packetNotice(typ, 0, 0, 0, 0, 0, w.messageFor(o, n, args...)))
}

// eventText is FUN_004c07f0 with a literal text.
func (w *World) eventText(o *Object, typ byte, text string) {
	w.send(o, packetNotice(typ, 0, 0, 0, 0, 0, text))
}

// eventGlobal is FUN_004c0ab0(0, ...): a gold notice to every player.
func (w *World) eventGlobal(n int, args ...any) { w.noticeAllMsg(NoticeGold, n, args...) }

// eventAll sends the gold notices ns to every playing player.
func (w *World) eventAll(ns ...int) {
	w.eventPlayers(func(o *Object) {
		for _, n := range ns {
			w.eventNotice(o, NoticeGold, n)
		}
	})
}

// eventPK sets the PK level, count and time of an event player and sends
// C1 07 F3 08 (FUN_004db9d0).
func (w *World) eventPK(o *Object, level, count, time int) {
	o.PKLevel, o.P.Char.PKLevel = level, level
	o.P.Char.PKCount, o.P.Char.PKTime = count, time
	w.sendPKLevel(o)
}

// eventPKLevel changes only the PK level (+0x19c) and sends F3 08.
func (w *World) eventPKLevel(o *Object, level int) {
	o.PKLevel, o.P.Char.PKLevel = level, level
	w.sendPKLevel(o)
}

// eventMurderer is the PK 6 / count 3 / PKDownRequirePoint3 given to the
// players of the fighting events.
func (w *World) eventMurderer(o *Object) {
	w.eventPK(o, PKOutlaw2, 3, w.cfg.Int("Common", "PKDownRequirePoint3", 0))
}

// eventBag is CItemBagManager::DropItem (FUN_00491430) at the player.
func (w *World) eventBag(o *Object, bag, index int) {
	if itemBagHook != nil {
		itemBagHook(w, o, bag, index, o.Map, o.X, o.Y)
	}
}

// eventLeaveParty is the party part of the Duel and Pair joins: a party of
// fewer than 3 members is dissolved (FUN_004d2500), otherwise the player
// leaves it (FUN_004d2650).
func (w *World) eventLeaveParty(o *Object) {
	pt := w.partyOf(o)
	if pt == nil {
		return
	}
	if pt.count < 3 {
		w.partyDestroy(pt)
	} else {
		w.partyRemove(pt, o)
	}
}

// inParty is obj+0x2f0 != -1.
func (w *World) inParty(o *Object) bool { return w.partyOf(o) != nil }

// inPlayerDuel stands for the duel flag obj+0x450 tested by the joins
// (message 0x45A).
func inPlayerDuel(o *Object) bool { return o.Duel != nil && validIdx(o.Duel.Opponent) }

// setPatente is FUN_00477060: obj+0x2500 = value, the value is taken from
// any other player holding it, and DS F3 E7 (C1 14 F3 E7 name[11] value)
// stores it (UPDATE Character SET Patente = 0 WHERE Patente = N, then N on
// the character; the DataServer ignores values <= 0).
func (w *World) setPatente(o *Object, value int) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if t := w.objects[i]; t != o && t.State != StateEmpty && t.IsPlayer() && t.P.Char != nil &&
			t.P.Char.Patente == value {
			t.P.Char.Patente = 0
		}
	}
	o.P.Char.Patente = value
	cs, _ := w.store.(store.CustomEventStore)
	if cs == nil {
		return
	}
	name := o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		if err := cs.SetPatente(ctx, name, value); err != nil {
			slog.Warn("patente", "name", name, "err", err)
		}
		return nil
	})
}

// eventRanking is FUN_00476fb0 with ESI = 1.
func (w *World) eventRanking(o *Object, rank string) { w.eventRankingN(o, rank, 1) }

// eventRankingN is FUN_00476fb0: DS C1 2C 41 index account name rank[11]
// score (the score is passed in ESI); the DataServer adds it to the column
// (FUN_004108f0).
func (w *World) eventRankingN(o *Object, rank string, n int) {
	w.log.Info("[ RANKING ]", "name", o.P.Char.Name, "rank", rank, "score", n)
	cs, _ := w.store.(store.CustomEventStore)
	if cs == nil {
		return
	}
	name := o.P.Char.Name
	w.async(func(ctx context.Context) func() {
		if err := cs.AddCustomRank(ctx, name, rank, n); err != nil {
			slog.Warn("custom ranking", "name", name, "rank", rank, "err", err)
		}
		return nil
	})
}

// eventSchedule reports a start row matching now: weekday, hour, minute
// and second 0 (GetLocalTime).
func (w *World) eventScheduled(s gamedata.EventStart) bool {
	now := w.now()
	return int(now.Weekday()) == s.Day && now.Hour() == s.Hour && now.Minute() == s.Minute && now.Second() == 0
}

// --- hooks --------------------------------------------------------------------------------

// customEventCommand is the start of FUN_00435400: the join command of
// each event, in the original order, with each join's own rule for
// letting the line reach the command table.
func (w *World) customEventCommand(o *Object, name string) bool {
	c := w.cev()
	if name == c.sobre.data.Syntax && w.sobreJoin(o) {
		return true
	}
	if name == c.duel.data.Syntax && w.mmJoin(o) {
		return true
	}
	if name == pairSyntax(w) && w.pairJoin(o) {
		return true
	}
	if name == c.roleta.data.Syntax && w.roletaJoin(o) {
		return true
	}
	if name == c.theft.data.Syntax && !w.killJoin(&c.theft, o) {
		return true
	}
	if name == c.death.data.Syntax && !w.killJoin(&c.death, o) {
		return true
	}
	return false
}

// customEventAttack is the event part of FUN_0040b110 (a attacks t).
func (w *World) customEventAttack(a, t *Object) bool {
	c := w.cev()
	for _, e := range []*killEvent{&c.death, &c.theft} {
		if e.open && a.Type == TypePlayer && t.Type == TypePlayer {
			if u := e.users[a.Index]; u != nil && u.Wait {
				return false
			}
		}
	}
	return w.sobreAttack(t) && w.mmAttack(a, t) && w.pairAttack(a, t)
}

// inCustomEvent is the test of the trade, party and duel requests and of
// the reset replies (U-CUST-07): obj+0x24f0, or the Theft, Death or Roleta
// table entry of the player.
func (w *World) inCustomEvent(o *Object) bool {
	if o.P == nil {
		return false
	}
	c := w.cev()
	if gmOf(o).InEvent || c.roleta.flags[o.Index] {
		return true
	}
	if u := c.theft.users[o.Index]; u != nil {
		return true
	}
	u := c.death.users[o.Index]
	return u != nil
}

// customEventPacketBlock refuses 36 (trade), 40 (party) and AA (duel)
// requests of an event player (FUN_004d6690).
func (w *World) customEventPacketBlock(o *Object, head byte) bool {
	var text string
	switch head {
	case 0x36:
		text = "Proibido dar trade no evento."
	case 0x40:
		text = "Proibido formar party no evento."
	case 0xAA:
		text = "Proibido iniciar o duelo no evento."
	default:
		return false
	}
	if !o.IsPlayer() || !w.inCustomEvent(o) {
		return false
	}
	w.eventText(o, NoticeBlue, text)
	return true
}

// customEventTrade is the event part of the trade request FUN_00507cf0:
// the Russian roulette shot (FUN_004e4e70) before the trade checks, then
// after them the hide and seek win (FUN_0045fed0) and the obj+0x24f0
// refusal ("Tentei dar Trade durante o Evento", no answer).
func (w *World) customEventTrade(o, t *Object) bool {
	if w.roletaTrade(o, t) {
		return true
	}
	if items(o).If.Use || items(t).If.Use || !w.tradeAllowed(o.Map) || o.Map != t.Map || !near2(o, t) ||
		pshopOpen(o) || pshopOpen(t) {
		return false
	}
	if w.hideTrade(o, t) {
		return true
	}
	if gmOf(o).InEvent || gmOf(t).InEvent {
		w.log.Debug("Tentei dar Trade durante o Evento", "name", o.P.Char.Name)
		return true
	}
	return false
}

// customEventDeath runs the kill counters and the event deaths.
func (w *World) customEventDeath(killer, victim *Object) {
	if victim.Type != TypePlayer || victim.P == nil || victim.P.Char == nil {
		return
	}
	c := w.cev()
	if killer.Type == TypePlayer && killer.P != nil && killer != victim {
		w.killScore(&c.theft, killer, victim)
		w.killScore(&c.death, killer, victim)
	}
	w.sobreDeath(killer, victim)
	w.mmDeath(victim)
	w.pairDeath(victim)
}

// customEventLeave is the event part of DelCharacterInfo, DelClient and
// gObjMoveGate.
func (w *World) customEventLeave(o *Object) {
	if !o.IsPlayer() || o.P.Char == nil {
		return
	}
	c := w.cev()
	w.sobreLeave(o)
	w.killLeave(&c.theft, o)
	w.killLeave(&c.death, o)
	w.mmLeave(o)
	w.pairLeave(o)
	w.roletaLeave(o)
}

// --- raffle (RaffleEvent.txt, FUN_004e0770) -------------------------------------------------

// raffleSecond is FUN_004e0770: at each start row a random playing user
// of Authority 1 (obj+0x250, a player who is not a GM; the rewrite keeps
// the GameMaster.txt level there, 0 for players) wins item bag 0x1D;
// everybody gets 0x461 and 0x462 (name), the winner 0x463. rand() is the
// monster generator here (U-CUST-10).
func (w *World) raffleSecond() {
	r := w.cev().raffle
	if r == nil || !r.Active {
		return
	}
	for _, s := range r.Starts {
		if !w.eventScheduled(s) {
			continue
		}
		var cand []*Object
		w.eventPlayers(func(o *Object) {
			if gmAuthority(o) <= 1 {
				cand = append(cand, o)
			}
		})
		if len(cand) == 0 {
			continue
		}
		win := cand[monsterRand()%len(cand)]
		w.eventPlayers(func(o *Object) {
			w.eventNotice(o, NoticeGold, 0x461)
			w.eventNotice(o, NoticeGold, 0x462, win.P.Char.Name)
		})
		w.eventBag(win, 0x1D, -1)
		w.eventNotice(win, NoticeGold, 0x463)
	}
}

// --- hide and seek (CEventHideAndSeek, FUN_0045fbf0 / FUN_0045fcd0 / FUN_0045fed0) -----------

// hideSeek is CEventHideAndSeek: +4 the GM index, +8 the remaining
// seconds, +0x10 active, +0x14 the last announced minute.
type hideSeek struct {
	gm      int
	remain  int
	active  bool
	timeMin int
}

// commandHideSeek is /escesc (index 130, FUN_0045fcd0): with
// EventHideAndSeekSwitch a stopped event starts for
// EventHideAndSeekMaxTime minutes with the GM as the target (0x68F with
// the GM name, then 0x690); a running one is cancelled (0x68E). The
// command is not charged.
func (w *World) commandHideSeek(o *Object, c command, arg string) {
	if w.cfg.Int("Event", "EventHideAndSeekSwitch", 0) == 0 {
		return
	}
	h := &w.cev().hide
	if h.remain < 1 {
		h.active = true
		h.gm = o.Index
		h.remain = w.cfg.Int("Event", "EventHideAndSeekMaxTime", 0) * 60
		w.eventGlobal(0x68F, o.P.Char.Name)
		w.eventGlobal(0x690)
		return
	}
	*h = hideSeek{timeMin: -1}
	w.eventGlobal(0x68E)
}

// hideSecond is FUN_0045fbf0: the minute announcement 0x692 (minutes
// left, rounded up), the last ten seconds 0x68C and the end without a
// winner 0x68D.
func (w *World) hideSecond() {
	h := &w.cev().hide
	if h.remain <= 0 {
		return
	}
	m := h.remain / 60
	if h.remain%60 == 0 {
		m--
	}
	if h.timeMin != m {
		h.timeMin = m
		w.eventGlobal(0x692, m+1)
	}
	if h.remain < 11 {
		w.eventGlobal(0x68C, h.remain)
	}
	h.remain--
	if h.remain < 1 {
		h.active = false
		w.eventGlobal(0x68D)
	}
}

// hideTrade is FUN_0045fed0: a trade request to the GM of a running hide
// and seek wins it: 0x691 (name), a firework (FUN_004dcb60) and item bag
// 0x1B at the winner.
func (w *World) hideTrade(o, t *Object) bool {
	h := &w.cev().hide
	if w.cfg.Int("Event", "EventHideAndSeekSwitch", 0) == 0 || !h.active || h.gm != t.Index {
		return false
	}
	*h = hideSeek{timeMin: -1}
	w.eventGlobal(0x691, o.P.Char.Name)
	fw := packetFirework(0, byte(o.X), byte(o.Y))
	w.send(o, fw)
	w.broadcast(o, fw)
	w.eventBag(o, 0x1B, -1)
	return true
}

// --- /patente -----------------------------------------------------------------------------

// commandPatente is /patente name value (index 134, FUN_0043f540):
// message 0x175 (GM name) to the character when it is not the GM, then
// FUN_00477060. Not charged.
func (w *World) commandPatente(o *Object, c command, arg string) {
	t := w.gmTarget(o, arg)
	if t == nil {
		return
	}
	v := cmdInt(arg, 1)
	if t != o {
		w.msg(t, msgGMEditedBy, o.P.Char.Name)
	}
	w.setPatente(t, v)
}

func init() {
	commandHandlers[130] = (*World).commandHideSeek
	commandHandlers[134] = (*World).commandPatente
	delete(pendingCommands, 130)
	delete(pendingCommands, 134)
}
