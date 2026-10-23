package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"

// CCustomArena (Custom/CustomArena.txt, GameServerInfo - Custom.dat
// CustomArenaSwitch / MapNumber / VictimScoreDecrease /
// KillerScoreIncrease read by FUN_00442fd0): 30 scheduled PvP arenas on
// one map. A player joins by using a Move.txt move whose gate is the
// arena's StartGate while registration is open (FUN_00443c50 from
// CMove::Move FUN_004bec30); kills give score (FUN_00444570), dead players
// respawn on the StartGate until MaxDeath (FUN_004443f0), and at the end
// every player gets the bag and a message with the rank (FUN_00443920).
//
// Entry layout (0xb9c bytes from DAT_010be500): +0 index, +4 state, +8
// remain (s), +0xc target time, +0x10 tick, +0x14 enter flag, +0x18 alarm
// minute, +0x1c last minute shown, +0x20 name, +0x40.. the CustomArena.txt
// rule, +0x9c 100 user rows of 0x1c bytes {index, score, rank, kills,
// kill streak, deaths, death streak}, +0xb8c the start times.

const (
	arenaStateNone    = 0 // disabled / no start time
	arenaStateWait    = 1 // waiting for the start; registration in the alarm window
	arenaStateStand   = 2 // registration closed, StandTime
	arenaStatePlay    = 3 // fighting, EventTime
	arenaStateClose   = 4 // results, CloseTime
	arenaMaxUsers     = 100
	arenaDefaultBag   = 0x14 // RewardBagIndex "*"
	msgArenaAlarm     = 0x1F5
	msgArenaClosed    = 0x1F6
	msgArenaStandby   = 0x1F7
	msgArenaStart     = 0x1F8
	msgArenaEnd       = 0x1F9
	msgArenaWinner    = 0x1FA
	msgArenaRank      = 0x1FB
	msgArenaCannot    = 0x1FC
	msgArenaNotOpen   = 0x1FD
	msgArenaFull      = 0x1FE
	msgArenaMinLevel  = 0x1FF
	msgArenaWrongClas = 0x207
)

type arenaUser struct {
	Index               int // -1 = empty
	Score, Rank         int
	Kills, KillStreak   int
	Deaths, DeathStreak int
}

type arenaEntry struct {
	Index, State    int
	Remain          int
	Target          int64 // time_t
	Enter           bool
	Minute, LastMin int
	Users           [arenaMaxUsers]arenaUser
}

type customArena struct {
	on                   bool // CustomArenaSwitch (+4)
	mapNumber            int  // +0x15c50
	victimDec, killerInc int  // +0x15c54 / +0x15c58
	data                 *gamedata.CustomArena
	e                    [gamedata.CustomArenaCount]arenaEntry
}

func newCustomArena() *customArena {
	a := &customArena{data: &gamedata.CustomArena{}, mapNumber: -1}
	for i := range a.e {
		a.e[i] = arenaEntry{Index: i, Minute: -1, LastMin: -1}
		a.e[i].clearRows()
	}
	return a
}

func (e *arenaEntry) clearRows() {
	for i := range e.Users {
		e.Users[i] = arenaUser{Index: -1}
	}
}

func (w *World) arena() *customArena {
	x := w.wcx()
	if x.arena == nil {
		x.arena = newCustomArena()
	}
	return x.arena
}

func (a *customArena) rule(e *arenaEntry) *gamedata.CustomArenaRule { return &a.data.Rules[e.Index] }

func init() {
	startHooks = append(startHooks, loadCustomArena)
	tick1000Hooks = append(tick1000Hooks, (*World).arenaSecond)
	deathHooks = append(deathHooks, (*World).arenaDeath)
	arenaMoveHook = (*World).arenaJoin
	arenaFightHook = (*World).arenaFighting
}

// Hooks owned by this file and called from shared code.
var (
	// arenaMoveHook is the arena part of CMove::Move (command.go moveTo):
	// false stops the move.
	arenaMoveHook func(w *World, o *Object, gate int) bool
	// arenaFightHook is FUN_00443f50 for the attacker (pvp.go, skill.go).
	arenaFightHook func(w *World, a *Object) bool
)

// loadCustomArena is FUN_00442fd0 + FUN_00443040 and, at the end of the
// start-up, FUN_00442f70.
func loadCustomArena(w *World) error {
	a := newCustomArena()
	w.wcx().arena = a
	a.on = w.cfg.Int("Custom", "CustomArenaSwitch", 0) != 0
	a.mapNumber = w.cfg.Int("Custom", "CustomArenaMapNumber", 0)
	a.victimDec = w.cfg.Int("Custom", "CustomArenaVictimScoreDecrease", 0)
	a.killerInc = w.cfg.Int("Custom", "CustomArenaKillerScoreIncrease", 0)
	if w.dataDir != "" {
		d, err := gamedata.LoadCustomArena(w.dataDir)
		if err != nil {
			return err
		}
		a.data = d
	}
	w.arenaInit()
	return nil
}

// arenaInit is FUN_00442f70: with the switch on every arena goes to state
// 1 and takes its next start time.
func (w *World) arenaInit() {
	a := w.arena()
	if !a.on {
		return
	}
	for i := range a.e {
		w.arenaRestart(&a.e[i])
	}
}

// arenaRestart is the "back to state 1" sequence: enter closed, minutes
// reset, every player sent to the FinalGate (FUN_004440f0) and the next
// start time (FUN_00443b20).
func (w *World) arenaRestart(e *arenaEntry) {
	e.State, e.Enter, e.Minute, e.LastMin = arenaStateWait, false, -1, -1
	w.arenaClearUsers(e)
	w.arenaSchedule(e)
}

// arenaSchedule is FUN_00443b20: the earliest start time of the arena
// (CScheduleManager); none sets the state to 0.
func (w *World) arenaSchedule(e *arenaEntry) {
	a := w.arena()
	now := w.now()
	next, ok := nextSchedule(a.data.Schedule[e.Index], now)
	if !ok {
		if a.on {
			e.State = arenaStateNone
		}
		return
	}
	e.Target = next.Unix()
	e.Remain = int(e.Target - now.Unix())
}

func (w *World) arenaSetTimer(e *arenaEntry, minutes int) {
	e.Minute, e.LastMin, e.Enter = -1, -1, false
	e.Remain = minutes * 60
	e.Target = w.now().Unix() + int64(e.Remain)
}

// arenaSecond is FUN_00443490 (from the 1-second timer FUN_00509cc0).
func (w *World) arenaSecond() {
	a := w.arena()
	if !a.on {
		return
	}
	for i := range a.e {
		e := &a.e[i]
		e.Remain = int(e.Target - w.now().Unix())
		switch e.State {
		case arenaStateWait:
			w.arenaWait(e)
		case arenaStateStand:
			w.arenaStand(e)
		case arenaStatePlay:
			w.arenaPlay(e)
		case arenaStateClose:
			w.arenaClosing(e)
		}
	}
}

// arenaWait is FUN_00443550: inside the alarm window registration is open
// and the minutes are announced to everybody (0x1F5); at the start the
// registration closes (0x1F6 to all, 0x1F7 to the players) and the
// standby begins.
func (w *World) arenaWait(e *arenaEntry) {
	a := w.arena()
	r := a.rule(e)
	if e.Remain > 0 && e.Remain <= r.AlarmTime*60 {
		w.arenaPurge(e)
		e.Enter = true
		m := e.Remain / 60
		if e.Remain%60 == 0 {
			m--
		}
		e.Minute = m
		if m != e.LastMin {
			e.LastMin = m
			w.noticeAllMsg(NoticeGold, msgArenaAlarm, r.Name, e.LastMin+1)
		}
	}
	if e.Remain < 1 {
		w.noticeAllMsg(NoticeGold, msgArenaClosed, r.Name)
		w.arenaUsersMsg(e, NoticeBlue, msgArenaStandby, r.Name, r.StandTime)
		if a.on {
			e.State = arenaStateStand
			w.arenaSetTimer(e, r.StandTime) // FUN_00443880
		}
	}
}

// arenaStand is FUN_00443670: fewer than MinUser players ("*" = 1) send
// everybody out and wait for the next start; at the end of the standby
// 0x1F8 and the fight.
func (w *World) arenaStand(e *arenaEntry) {
	a := w.arena()
	r := a.rule(e)
	w.arenaPurge(e)
	minUser := 1
	if r.MinUser != -1 {
		minUser = r.MinUser
	}
	if w.arenaCount(e) < minUser {
		if a.on {
			w.arenaRestart(e)
		}
		return
	}
	if e.Remain < 1 {
		w.arenaUsersMsg(e, NoticeGold, msgArenaStart, r.Name)
		if a.on {
			e.State = arenaStatePlay
			w.arenaSetTimer(e, r.EventTime) // FUN_004438d0
		}
	}
}

// arenaPlay is FUN_00443710: nobody left restarts the arena; one player
// left ends it at once (without 0x1F9); otherwise the end of EventTime
// sends 0x1F9 and the results.
func (w *World) arenaPlay(e *arenaEntry) {
	a := w.arena()
	w.arenaPurge(e)
	n := w.arenaCount(e)
	if n == 0 {
		if a.on {
			w.arenaRestart(e)
		}
		return
	}
	if n != 1 {
		if e.Remain > 0 {
			return
		}
		w.arenaUsersMsg(e, NoticeGold, msgArenaEnd, a.rule(e).Name)
	}
	if a.on {
		e.State = arenaStateClose
		w.arenaFinish(e)
	}
}

// arenaClosing is FUN_004437a0.
func (w *World) arenaClosing(e *arenaEntry) {
	w.arenaPurge(e)
	if e.Remain < 1 && w.arena().on {
		w.arenaRestart(e)
	}
}

// arenaFinish is FUN_00443920: ranks (FUN_00444290); the first is
// announced to everybody (0x1FA); every player gets the bag RewardBagIndex
// ("*" = 0x14) with the arena index as SpecialLevel at its cell and
// message 0x1FB (rank, score); CloseTime starts.
func (w *World) arenaFinish(e *arenaEntry) {
	a := w.arena()
	r := a.rule(e)
	e.Minute, e.LastMin, e.Enter = -1, -1, false
	w.arenaRank(e)
	for i := range e.Users {
		u := &e.Users[i]
		if !validIdx(u.Index) {
			continue
		}
		o := w.objects[u.Index]
		if u.Rank == 1 {
			w.noticeAllMsg(NoticeGold, msgArenaWinner, r.Name, charName(o))
		}
		bag := arenaDefaultBag
		if r.RewardBag != -1 {
			bag = r.RewardBag
		}
		if itemBagHook != nil {
			itemBagHook(w, o, bag, e.Index, o.Map, o.X, o.Y)
		}
		if o.P != nil {
			w.customMsg(o, msgArenaRank, r.Name, u.Rank, u.Score)
		}
	}
	e.Remain = r.CloseTime * 60
	e.Target = w.now().Unix() + int64(e.Remain)
}

// arenaRank is FUN_00444290: rank = 1 + the players with more score, or
// the same score in an earlier row.
func (w *World) arenaRank(e *arenaEntry) {
	for i := range e.Users {
		p := &e.Users[i]
		if !validIdx(p.Index) {
			continue
		}
		better := 0
		for k := range e.Users {
			q := &e.Users[k]
			if k == i || !validIdx(q.Index) || q.Index == p.Index {
				continue
			}
			if q.Score > p.Score || (q.Score == p.Score && k < i) {
				better++
			}
		}
		p.Rank = better + 1
	}
}

// arenaPurge is FUN_00444150: rows of players that are no longer playing
// or left the arena map are emptied.
func (w *World) arenaPurge(e *arenaEntry) {
	mp := w.arena().mapNumber
	for i := range e.Users {
		u := &e.Users[i]
		if !validIdx(u.Index) {
			continue
		}
		o := w.objects[u.Index]
		if o.State != StatePlaying || o.Map != mp {
			e.Users[i] = arenaUser{Index: -1}
		}
	}
}

// arenaClearUsers is FUN_004440f0: every listed player goes to the
// FinalGate (gObjMoveGate) and its row is emptied.
func (w *World) arenaClearUsers(e *arenaEntry) {
	gate := w.arena().rule(e).FinalGate
	for i := range e.Users {
		u := &e.Users[i]
		if !validIdx(u.Index) {
			continue
		}
		if o := w.objects[u.Index]; o.State == StatePlaying && o.IsPlayer() {
			w.gateMove(o, gate)
		}
		e.Users[i] = arenaUser{Index: -1}
	}
}

// arenaCount is FUN_004441f0.
func (w *World) arenaCount(e *arenaEntry) int {
	n := 0
	for i := range e.Users {
		if validIdx(e.Users[i].Index) {
			n++
		}
	}
	return n
}

// arenaUser is FUN_004440a0: the row of an object, nil when not listed.
func (e *arenaEntry) user(idx int) *arenaUser {
	if !validIdx(idx) {
		return nil
	}
	for i := range e.Users {
		if e.Users[i].Index == idx {
			return &e.Users[i]
		}
	}
	return nil
}

// arenaUsersMsg is FUN_00444670: a C1 0D notice of type typ with a
// Message.txt text to every listed player.
func (w *World) arenaUsersMsg(e *arenaEntry, typ byte, n int, args ...any) {
	for i := range e.Users {
		if u := e.Users[i]; validIdx(u.Index) {
			if o := w.objects[u.Index]; o.P != nil {
				w.eventNotice(o, typ, n, args...)
			}
		}
	}
}

// arenaJoin is FUN_00443c50 for a move to the arena map: the gate must be
// the StartGate of an arena (otherwise the move is refused silently); the
// player must be out of party, guild war, duel and personal shop, not on
// the arena map already and the map served here (0x1FC); then open
// registration (0x1FD), room (MaxUser, at most 100: 0x1FE), level, master
// level, resets and master resets (0x1FF..0x206) and the class column
// (allowed from change-up value-1, else 0x207). A successful join clears
// the temporary effects and lists the player.
func (w *World) arenaJoin(o *Object, gate int) bool {
	a := w.arena()
	g, ok := w.data.Gates[gate]
	if !ok || g.Map != a.mapNumber {
		return true // not a move to the arena map
	}
	refuse := func(n int, args ...any) bool {
		w.customNoticeMsg(o, 1, 2, n, args...)
		return false
	}
	served := false
	if _, ok := w.data.Maps[a.mapNumber]; ok {
		served = w.data.MapServer == nil || w.data.MapServer.ServerForMap(w.cfg.ServerCode, a.mapNumber) == w.cfg.ServerCode
	}
	if validIdx(w.partyNumber(o)) || w.guildWarActive(guildNumberOf(o)) || inPlayerDuel(o) || pshopOpen(o) ||
		o.Map == a.mapNumber || !served {
		return refuse(msgArenaCannot)
	}
	var e *arenaEntry
	for i := range a.e {
		if a.data.Rules[i].StartGate == gate {
			e = &a.e[i]
			break
		}
	}
	if e == nil {
		return false
	}
	if !e.Enter {
		return refuse(msgArenaNotOpen)
	}
	r := a.rule(e)
	if w.arenaCount(e) >= min(r.MaxUser, arenaMaxUsers) {
		return refuse(msgArenaFull)
	}
	ch := o.P.Char
	masterLevel := 0 // obj+0x5e4, only set by Lua in this build (U-COMBAT-05)
	checks := []struct {
		limit int
		fail  bool
	}{
		{r.MinLevel, ch.Level < r.MinLevel},
		{r.MaxLevel, r.MaxLevel < ch.Level},
		{r.MinMasterLevel, masterLevel < r.MinMasterLevel},
		{r.MaxMasterLevel, r.MaxMasterLevel < masterLevel},
		{r.MinReset, ch.Resets < r.MinReset},
		{r.MaxReset, r.MaxReset < ch.Resets},
		{r.MinMasterReset, ch.MasterResets < r.MinMasterReset},
		{r.MaxMasterReset, r.MaxMasterReset < ch.MasterResets},
	}
	for i, c := range checks {
		if c.limit != -1 && c.fail {
			return refuse(msgArenaMinLevel+i, c.limit)
		}
	}
	if o.Class < 0 || o.Class >= len(r.Class) || r.Class[o.Class] == 0 || r.Class[o.Class] > changeUp(o)+1 {
		return refuse(msgArenaWrongClas)
	}
	w.clearEffects(o)
	return w.arenaAdd(e, o.Index)
}

// arenaAdd is FUN_00443fa0: the first empty row; false when the player is
// already listed or the arena is full.
func (w *World) arenaAdd(e *arenaEntry, idx int) bool {
	if !validIdx(idx) || e.user(idx) != nil {
		return false
	}
	for i := range e.Users {
		if !validIdx(e.Users[i].Index) {
			e.Users[i] = arenaUser{Index: idx}
			return true
		}
	}
	return false
}

// arenaListed is FUN_00443f00: the player is on the arena map and listed
// in an arena that is not in state 0 (the viewport shows it with PK
// level 6).
func (w *World) arenaListed(o *Object) bool {
	a := w.arena()
	if o.Map != a.mapNumber {
		return false
	}
	for i := range a.e {
		if a.e[i].State != arenaStateNone && a.e[i].user(o.Index) != nil {
			return true
		}
	}
	return false
}

// arenaFighting is FUN_00443f50: the attacker is on the arena map and
// listed in an arena in the fight state.
func (w *World) arenaFighting(o *Object) bool {
	a := w.arena()
	if o.Map != a.mapNumber {
		return false
	}
	for i := range a.e {
		if a.e[i].State == arenaStatePlay && a.e[i].user(o.Index) != nil {
			return true
		}
	}
	return false
}

// arenaDeath is FUN_00444570 (from FUN_0050e8a0 when the victim is on the
// arena map): in every fighting arena listing both, the victim loses
// min(score, VictimScoreDecrease) and counts a death (streak, kill streak
// reset), the killer gains KillerScoreIncrease and counts a kill (death
// streak reset).
func (w *World) arenaDeath(killer, victim *Object) {
	a := w.arena()
	if !a.on || !victim.IsPlayer() || victim.Map != a.mapNumber {
		return
	}
	for i := range a.e {
		e := &a.e[i]
		if e.State != arenaStatePlay {
			continue
		}
		v := e.user(victim.Index)
		if v == nil {
			return
		}
		k := e.user(killer.Index)
		if k == nil {
			return
		}
		dec := min(v.Score, a.victimDec)
		v.Deaths++
		v.DeathStreak++
		v.Score -= dec
		v.KillStreak = 0
		k.Score += a.killerInc
		k.Kills++
		k.KillStreak++
		k.DeathStreak = 0
	}
}

// arenaRespawn is FUN_004443f0 (respawn FUN_004c53c0 on the arena map
// when its MapManager RespawnGate is "*"): a player of a fighting arena
// comes back on the StartGate, or on the FinalGate (and leaves the list)
// once its deaths reached MaxDeath. A gate that cannot give a cell falls
// through to the next arena.
func (w *World) arenaRespawn(o *Object) (int, int, int, bool) {
	a := w.arena()
	if !a.on {
		return 0, 0, 0, false
	}
	for i := range a.e {
		e := &a.e[i]
		if e.State != arenaStatePlay {
			continue
		}
		u := e.user(o.Index)
		if u == nil {
			continue
		}
		r := a.rule(e)
		gate, out := r.StartGate, false
		if r.MaxDeath != -1 && u.Deaths >= r.MaxDeath {
			gate, out = r.FinalGate, true
		}
		g, ok := w.data.Gates[gate]
		if !ok {
			continue
		}
		x, y := w.randomGatePosition(gate)
		if out {
			*u = arenaUser{Index: -1}
		}
		return g.Map, x, y, true
	}
	return 0, 0, 0, false
}

// arenaWindow lists the arenas for the event time window (FUN_00458a70):
// every arena not in state 0, with the remaining time in state 1.
func (w *World) arenaWindow() []windowRow {
	a := w.arena()
	if !a.on {
		return nil
	}
	var out []windowRow
	for i := range a.e {
		e := &a.e[i]
		if e.State == arenaStateNone {
			continue
		}
		row := windowRow{Name: a.data.Rules[i].Name, Open: true}
		if e.State == arenaStateWait {
			row.Remain = e.Remain
		}
		out = append(out, row)
	}
	return out
}
