package game

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Guild war and battle soccer request flow (/guildwar, /guildsoccer, 60..64).
// The war state lives in the GameServer guild object (+0x7f8 enemy guild,
// +0x7fc requesting, +0x7fd active, +0x7fe type 0 war / 1 soccer, +0x7ff
// team, +0x800 score), so it is per instance as in the original.

// guildWar is that state for one guild of this instance.
type guildWar struct {
	enemy      int
	requesting bool // +0x7fc == 1
	active     bool // +0x7fd == 1
	typ        byte
	team       byte
	score      byte
}

// guildWorld is the per-instance guild data (wars and delayed war ends).
type guildWorld struct {
	wars   map[int]*guildWar
	clears []warClear
}

// warClear is the FUN_00510550(4, killer, 2000) message: the war flags of
// both guilds are cleared two seconds after the end (FUN_004c4120 case 4
// -> FUN_00486780).
type warClear struct {
	at   time.Time
	a, b int
}

func (w *World) guildWarOf(n int) *guildWar {
	if w.guilds == nil || n == 0 {
		return nil
	}
	return w.guilds.wars[n]
}

func (w *World) guildWarActive(n int) bool {
	war := w.guildWarOf(n)
	return war != nil && war.active
}

func (w *World) warScore(n int) byte {
	if war := w.guildWarOf(n); war != nil {
		return war.score
	}
	return 0
}

// clearWar is FUN_00486780 for one guild.
func (w *World) clearWar(n int) {
	if w.guilds != nil {
		delete(w.guilds.wars, n)
	}
}

// --- packets ---------------------------------------------------------------------

// packetGuildWarRequestResult is C1 04 60 result (FUN_00485820).
func packetGuildWarRequestResult(result byte) []byte {
	p := newPacket(frame.C1, 0x60, -1, 4)
	p[3] = result
	return p
}

// packetGuildWarRequest is C1 0C 61 guild[8] type (FUN_004858c0), sent to
// the master of the challenged guild.
func packetGuildWarRequest(guild string, typ byte) []byte {
	p := newPacket(frame.C1, 0x61, -1, 0x0C)
	putName8(p[3:], guild)
	p[11] = typ
	return p
}

// packetGuildWarStart is C1 0D 62 enemy[8] type team (FUN_00485980).
func packetGuildWarStart(enemy string, typ, team byte) []byte {
	p := newPacket(frame.C1, 0x62, -1, 0x0D)
	putName8(p[3:], enemy)
	p[11], p[12] = typ, team
	return p
}

// packetGuildWarEnd is C1 0C 63 result enemy[8] (FUN_00485a50): 1 won, 0
// lost, 3/2 the war was abandoned (own / enemy side).
func packetGuildWarEnd(result byte, enemy string) []byte {
	p := newPacket(frame.C1, 0x63, -1, 0x0C)
	p[3] = result
	putName8(p[4:], enemy)
	return p
}

// packetGuildWarScore is C1 06 64 own enemy 0 (FUN_00485ab0).
func packetGuildWarScore(own, enemy byte) []byte {
	p := newPacket(frame.C1, 0x64, -1, 6)
	p[3], p[4] = own, enemy
	return p
}

func (w *World) packetWarScoreFor(n int) []byte {
	var enemy byte
	if war := w.guildWarOf(n); war != nil {
		enemy = w.warScore(war.enemy)
	}
	return packetGuildWarScore(w.warScore(n), enemy)
}

// --- request ------------------------------------------------------------------------

// guildWarRequest is FUN_00485f40 (/guildwar typ 0, /guildsoccer typ 1):
// true when the request was sent to the other master.
func (w *World) guildWarRequest(o *Object, target string, typ byte) bool {
	if w.cfg.Int("Common", "GuildWarSwitch", 0) == 0 || !guildUsable(o) || !w.mapGuildWar(o.Map) {
		return false
	}
	pkFree := w.cfg.Int("Common", "PKLimitFree", 0) != 0
	gs := guildOf(o)
	var result byte
	switch {
	case gs.Number == 0:
		result = 3
	case w.guildWarOf(gs.Number) != nil && (w.guildWarOf(gs.Number).active || w.guildWarOf(gs.Number).requesting),
		!pkFree && o.PKLevel > 4:
		result = 4
	case gs.Status != store.GuildMasterStatus:
		result = 5
	default:
		mine, _ := w.guildMgr().byNumber(gs.Number)
		if cut8(mine.Name) == cut8(target) {
			return false // strncmp(own name, target, 8) == 0
		}
		enemy, ok := w.guildMgr().find(target)
		if !ok {
			result = 0
			break
		}
		if ew := w.guildWarOf(enemy.Number); ew != nil && (ew.active || ew.requesting) {
			result = 4
			break
		}
		if mine.Union != 0 && mine.Union == enemy.Union {
			return false
		}
		m := w.playerHere(enemy.Master)
		if m == nil {
			result = 2
			break
		}
		if !pkFree && m.PKLevel > 4 {
			result = 4
			break
		}
		if !w.mapGuildWar(m.Map) {
			return false
		}
		if !w.acceptsRequests(m) {
			result = 4
			break
		}
		w.send(o, packetGuildWarRequestResult(1))
		w.send(m, packetGuildWarRequest(mine.Name, typ))
		w.guilds.wars[mine.Number] = &guildWar{enemy: enemy.Number, requesting: true, typ: typ}
		w.guilds.wars[enemy.Number] = &guildWar{enemy: mine.Number, requesting: true, typ: typ}
		return true
	}
	w.send(o, packetGuildWarRequestResult(result))
	return false
}

func cut8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// mapGuildWar is the MapManager GuildWarEnable flag (FUN_004aeab0 +0x60).
func (w *World) mapGuildWar(mp int) bool {
	info, ok := w.data.Maps[mp]
	return ok && info.GuildWarEnable != 0
}

// commandGuildWar is /guildwar (case 1, FUN_00436eb0) and /guildsoccer
// (case 2, FUN_00436f90): a guild name of 4 or more characters (strlen < 4
// is refused at 0x436ef6), otherwise 60 result 0.
func (w *World) commandGuildWar(typ byte) func(*World, *Object, command, string) {
	return func(w *World, o *Object, c command, arg string) {
		name := commandToken(arg, 0, 9)
		if len(name) < 4 {
			w.send(o, packetGuildWarRequestResult(0))
			return
		}
		if w.guildWarRequest(o, name, typ) {
			w.commandDone(o, c)
		}
	}
}

// --- answer -------------------------------------------------------------------------

// handleGuildWarAnswer is 61 (FUN_00484520): [3] accept.
func (w *World) handleGuildWarAnswer(o *Object, p []byte) {
	if len(p) < 4 {
		return
	}
	n := guildNumberOf(o)
	if n == 0 {
		w.send(o, packetGuildWarRequestResult(0))
		return
	}
	accept := p[3] != 0
	mine := w.guildWarOf(n)
	var enemyNum int
	if mine != nil {
		enemyNum = mine.enemy
	}
	enemy, _ := w.guildMgr().byNumber(enemyNum)
	enemyMaster := w.playerHere(enemy.Master)
	if enemyNum != 0 && accept && enemyMaster != nil {
		switch mp := enemyMaster.Map; {
		case mp > 10 && mp < 18, mp > 17 && mp < 24, mp == 9, mp == 32:
			accept = false // Blood Castle, Chaos Castle, Devil Square
		}
	}
	ew := w.guildWarOf(enemyNum)
	if !accept {
		if ew != nil && ew.requesting {
			w.clearWar(n)
			w.clearWar(enemyNum)
			if enemyMaster != nil {
				w.send(enemyMaster, packetGuildWarRequestResult(6))
			}
		}
		return
	}
	if mine == nil || !mine.requesting || ew == nil || !ew.requesting {
		return
	}
	// Battle soccer: the ground must be free (FUN_0040aa60 / FUN_004110a0,
	// guild_soccer.go).
	if mine.typ == 1 && !w.soccerBegin(o, n, enemyNum) {
		return
	}
	mine.requesting, mine.active, mine.team, mine.score = false, true, 0, 0
	ew.requesting, ew.active, ew.team, ew.score = false, true, 1, 0
	mg, _ := w.guildMgr().byNumber(n)
	w.warStartSend(&mg, mine, enemy.Name)
	w.warStartSend(&enemy, ew, mg.Name)
	if mine.typ == 1 {
		w.guildMessage(n, NoticeBlue, 0x11F)
		w.guildMessage(enemyNum, NoticeBlue, 0x11F)
	}
}

// warStartSend is the member loop of FUN_00484520 with FUN_00485eb0: 62 and
// 64 to every online member (for a soccer only the master and the members
// of the master's party).
func (w *World) warStartSend(g *guildRecord, war *guildWar, enemy string) {
	master := w.playerHere(g.Master)
	party := -1
	if master != nil && master.P.Party != nil {
		party = master.P.Party.Number
	}
	for _, m := range w.guildMembersHere(g.Number) {
		if war.typ == 1 && m != master && (party < 0 || m.P.Party == nil || m.P.Party.Number != party) {
			continue
		}
		w.send(m, packetGuildWarStart(enemy, war.typ, war.team))
		w.send(m, w.packetWarScoreFor(g.Number))
	}
}

// --- rules --------------------------------------------------------------------------

// atWar is FUN_004864b0: both players are in guilds at war with each other.
func (w *World) atWar(a, b *Object) bool {
	if !a.IsPlayer() || !b.IsPlayer() {
		return false
	}
	na, nb := guildNumberOf(a), guildNumberOf(b)
	wa, wb := w.guildWarOf(na), w.guildWarOf(nb)
	return wa != nil && wb != nil && wa.active && wb.active && wb.enemy == na
}

// guildWarKill is FUN_004868f0 (called by the player death FUN_004cda80):
// the killer's guild scores GuildWarKillScore2 for the enemy master,
// GuildWarKillScore1 otherwise. Returns true when the kill counted.
func (w *World) guildWarKill(killer, victim *Object) bool {
	if !w.atWar(killer, victim) {
		return false
	}
	kn, vn := guildNumberOf(killer), guildNumberOf(victim)
	vg, _ := w.guildMgr().byNumber(vn)
	score := w.cfg.Int("Common", "GuildWarKillScore1", 0)
	if victim.P.Char.Name == vg.Master {
		score = w.cfg.Int("Common", "GuildWarKillScore2", 0)
	}
	if kw := w.guildWarOf(kn); kw != nil && kw.typ == 1 {
		// Respawn type 2 (obj+0x285) inside the soccer ground.
		w.soccer().respawn[victim.Index] = true
	}
	if !w.warAddScore(kn, vn, score) {
		return true
	}
	if kw := w.guildWarOf(kn); kw != nil && kw.typ == 1 {
		w.soccerKillEnd(kn, vn) // FUN_00486660 / FUN_00486780 / FUN_00410fd0
		return true
	}
	w.warEnd(kn, vn, 1, 0)
	w.guilds.clears = append(w.guilds.clears, warClear{at: w.now().Add(2 * time.Second), a: kn, b: vn})
	return true
}

// warAddScore is FUN_004867c0: false while the score is below
// GuildWarScoreMax1 (war) / GuildWarScoreMax2 (soccer), sending 64 to both
// guilds; at the limit the winner's G_Score grows by GuildWarWinnerScore1
// (enemy 0 points), 2 (enemy at most half the limit) or 3, and both totals
// go to the DataServer (A0 08, FUN_00483490).
func (w *World) warAddScore(kn, en int, add int) bool {
	kw, ew := w.guildWarOf(kn), w.guildWarOf(en)
	limit := w.cfg.Int("Common", "GuildWarScoreMax1", 0)
	if kw.typ == 1 {
		limit = w.cfg.Int("Common", "GuildWarScoreMax2", 0)
	}
	kw.score += byte(add)
	if int(kw.score) < limit {
		for _, n := range []int{kn, en} {
			for _, m := range w.guildMembersHere(n) {
				w.send(m, w.packetWarScoreFor(n))
			}
		}
		return false
	}
	var bonus int
	switch {
	case ew.score == 0:
		bonus = w.cfg.Int("Common", "GuildWarWinnerScore1", 0)
	case int(ew.score) <= limit/2:
		bonus = w.cfg.Int("Common", "GuildWarWinnerScore2", 0)
	default:
		bonus = w.cfg.Int("Common", "GuildWarWinnerScore3", 0)
	}
	mgr := w.guildMgr()
	kg, _ := mgr.byNumber(kn)
	eg, _ := mgr.byNumber(en)
	total := kg.Score + bonus
	w.async(func(ctx context.Context) func() {
		mgr.update(ctx, kg.Name, func(s store.GuildStore) error { return s.SetGuildScore(ctx, kg.Name, total) },
			func(r *guildRecord) { r.Score = total })
		mgr.update(ctx, eg.Name, func(s store.GuildStore) error { return s.SetGuildScore(ctx, eg.Name, eg.Score) },
			func(r *guildRecord) {})
		return nil
	})
	return true
}

// warEnd is FUN_00486530: 63 to both guilds (own result to guild a, enemy
// result to guild b).
func (w *World) warEnd(a, b int, own, enemy byte) {
	ga, _ := w.guildMgr().byNumber(a)
	gb, _ := w.guildMgr().byNumber(b)
	for _, m := range w.guildMembersHere(a) {
		w.send(m, packetGuildWarEnd(own, gb.Name))
	}
	for _, m := range w.guildMembersHere(b) {
		w.send(m, packetGuildWarEnd(enemy, ga.Name))
	}
}

// guildWarAbandon is FUN_00486a60 (the master disbands the guild): 63
// result 3 / 2 and both war states cleared at once.
func (w *World) guildWarAbandon(o *Object) {
	n := guildNumberOf(o)
	war := w.guildWarOf(n)
	if war == nil {
		return
	}
	if war.active {
		w.warEnd(n, war.enemy, 3, 2)
	}
	if war.typ == 1 {
		w.soccerEnd() // ball reset, CLEAN (FUN_00410fd0)
	}
	w.clearWar(war.enemy)
	w.clearWar(n)
}

// warTick runs the delayed war clears.
func (w *World) warTick() {
	if w.guilds == nil {
		return
	}
	now := w.now()
	keep := w.guilds.clears[:0]
	for _, c := range w.guilds.clears {
		if now.Before(c.at) {
			keep = append(keep, c)
			continue
		}
		w.clearWar(c.a)
		w.clearWar(c.b)
	}
	w.guilds.clears = keep
}

func registerGuildWar(h *hooks) {
	h.commandHandlers[1] = func(w *World, o *Object, c command, arg string) { w.commandGuildWar(0)(w, o, c, arg) }
	h.commandHandlers[2] = func(w *World, o *Object, c command, arg string) { w.commandGuildWar(1)(w, o, c, arg) }
	h.tick1000 = append(h.tick1000, (*World).warTick)
	h.guildWarKill = func(w *World, killer, victim *Object) bool { return w.guildWarKill(killer, victim) }
	h.guildWar = func(w *World, a, b *Object) bool { return w.atWar(a, b) }
	h.pkExempt = append(h.pkExempt, func(w *World, killer, victim *Object) bool {
		// FUN_0050f330: no PK for rival guilds (FUN_00486c20 == 2) or a war.
		kn, vn := guildNumberOf(killer), guildNumberOf(victim)
		if kn != 0 && vn != 0 && w.guildMgr().relation(kn, vn) == 2 {
			return true
		}
		return w.atWar(killer, victim)
	})
}
