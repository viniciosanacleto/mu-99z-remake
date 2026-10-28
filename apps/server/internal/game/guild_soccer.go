package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Battle soccer (CBattleSoccer at DAT_0115f298, docs/server-plan/09b-guild.md
// "Battle soccer"): the ground on Arena (map 6), the ball (monster class
// 200, whose index FUN_004b4fe0 keeps at +4), the two teams and the state
// machine run every second by FUN_00410ce0:
//
//   - STAND (1, FUN_004110a0, 10 s): /guildsoccer accepted; each team gets
//     the first 5 online members of its guild (member list order);
//   - START (2, FUN_00410f80, 900 s): goals (FUN_00411640, the ball's AI
//     step) score GuildWarKillScore3 for the team owning the goal area;
//   - CLEAN (3, FUN_00410fd0, 10 s): the war ends (63 result 1 to team 0
//     and 0 to team 1 while both wars are still active) and is cleared;
//   - EMPTY (0, FUN_00410f50): team members are sent to gate 17.
//
// A team losing all its members (logout, other map) ends STAND/START.
// Players on the ground who are not playing the match are sent to gate 17
// every second (FUN_00411590 from FUN_00510810).

const (
	soccerBallClass = 200
	soccerMap       = 6
	soccerGate      = 0x11

	soccerEmpty = 0
	soccerStand = 1
	soccerStart = 2
	soccerClean = 3
)

// soccerGoals is DAT_005479c4: the goal area {x1, y1, x2, y2} where team
// i scores.
var soccerGoals = [2][4]int{{60, 181, 64, 182}, {60, 139, 64, 140}}

// soccerTeam is one CBattleSoccer team (0x34 bytes at +0x18): +8 guild,
// +0xc five {member index, master flag}. The party number at +4 is never
// set by any code in this build (U-GUILD-09).
type soccerTeam struct {
	guild   int
	members [5]int
	master  [5]bool
}

// soccerGround is the CBattleSoccer state.
type soccerGround struct {
	ball   int       // +4
	state  int       // +8
	remain int       // +0xc seconds left (difftime)
	end    time.Time // +0x10 (time64)
	tick   time.Time // +0x14
	teams  [2]soccerTeam
	// respawn marks the players killed in a counted soccer kill (respawn
	// type obj+0x285 = 2): they come back inside the ground.
	respawn map[int]bool
}

// reset is FUN_00411350 (+ FUN_00410c50): no members.
func (s *soccerGround) reset() {
	for i := range s.teams {
		for k := range s.teams[i].members {
			s.teams[i].members[k], s.teams[i].master[k] = -1, false
		}
	}
	if s.respawn == nil {
		s.respawn = map[int]bool{}
	}
}

func (w *World) soccer() *soccerGround { return &w.q().soccer }

func (w *World) soccerState() int { return w.soccer().state }

// slotOf is FUN_00411300: the team and slot of a member.
func (s *soccerGround) slotOf(idx int) (int, int, bool) {
	if idx < 0 || idx >= MaxObjects {
		return 0, 0, false
	}
	for t := range s.teams {
		for k, m := range s.teams[t].members {
			if m == idx {
				return t, k, true
			}
		}
	}
	return 0, 0, false
}

// add is FUN_004111c0: the first free slot of team t.
func (w *World) soccerAdd(t int, o *Object) bool {
	s := w.soccer()
	if _, _, ok := s.slotOf(o.Index); ok {
		return false
	}
	for k, m := range s.teams[t].members {
		if m < 0 || m >= MaxObjects {
			s.teams[t].members[k] = o.Index
			s.teams[t].master[k] = guildOf(o).Status == store.GuildMasterStatus
			return true
		}
	}
	return false
}

// count is FUN_00411500.
func (s *soccerGround) count(t int) int {
	n := 0
	for _, m := range s.teams[t].members {
		if m >= 0 && m < MaxObjects {
			n++
		}
	}
	return n
}

// soccerBegin is the battle soccer branch of the 61 answer (FUN_00484520):
// with the ground busy both requests are dropped and the answering master
// gets 60 result 4; otherwise the scores are zeroed and STAND starts with
// the answering guild as team 0 (FUN_004110a0).
func (w *World) soccerBegin(o *Object, own, enemy int) bool {
	s := w.soccer()
	if s.state != soccerEmpty {
		w.clearWar(own)
		w.clearWar(enemy)
		w.send(o, guild.PacketWarRequestResult(4))
		return false
	}
	s.teams[0].guild, s.teams[1].guild = own, enemy
	mgr := w.guildMgr()
	g0, _ := mgr.ByNumber(own)
	g1, _ := mgr.ByNumber(enemy)
	for i := 0; i < guild.MaxMembers; i++ {
		if m := w.playerHere(g0.Members[i].Name); m != nil {
			w.soccerAdd(0, m)
		}
		if m := w.playerHere(g1.Members[i].Name); m != nil {
			w.soccerAdd(1, m)
		}
	}
	w.soccerBallReset()
	w.soccerSet(soccerStand, 10)
	w.log.Info("[Battle Soccer] SetState STAND")
	return true
}

// soccerSet stores the state and its duration (+0xc / +0x10).
func (w *World) soccerSet(state, seconds int) {
	s := w.soccer()
	s.state, s.remain = state, seconds
	s.end = w.now().Add(time.Duration(seconds) * time.Second)
}

// soccerBall returns the ball (CBattleSoccer+4): the class 200 monster.
func (w *World) soccerBall() *Object {
	s := w.soccer()
	if s.ball >= 0 && s.ball < MaxObjects {
		if o := w.objects[s.ball]; o.State == StatePlaying && o.Mon != nil && o.Class == soccerBallClass {
			return o
		}
	}
	s.ball = -1
	for i := 0; i < PlayerStart; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.Mon != nil && o.Class == soccerBallClass {
			s.ball = i
			return o
		}
	}
	return nil
}

// soccerBallReset is FUN_004b4e40 on the ball: back to a cell of its
// spawn record, removed from the viewports to be shown again.
func (w *World) soccerBallReset() {
	o := w.soccerBall()
	if o == nil {
		return
	}
	w.monsterOccupy(o, false)
	clearViewportSilently(o)
	if w.regenPosition(o) {
		o.Live = LiveNew
		w.monsterOccupy(o, true)
	}
}

// soccerClean is FUN_00410fd0: the war of the two teams ends (63 result 1
// to team 0, 0 to team 1, only while both wars are active), both war
// states are cleared and CLEAN lasts 10 seconds.
func (w *World) soccerClean() {
	s := w.soccer()
	g0, g1 := s.teams[0].guild, s.teams[1].guild
	w0, w1 := w.guildWarOf(g0), w.guildWarOf(g1)
	if w0 != nil && w1 != nil && w0.active && w1.active {
		w.warEnd(g0, g1, 1, 0)
	}
	w.clearWar(g0)
	w.clearWar(g1)
	w.soccerSet(soccerClean, 10)
	w.log.Info("[Battle Soccer] SetState CLEAN")
}

// soccerEnd moves to CLEAN with the ball back in place (the common tail
// of FUN_00410d60 / FUN_00410e50 / FUN_00411640 / FUN_004868f0).
func (w *World) soccerEnd() {
	w.soccerBallReset()
	w.soccer().state = soccerClean
	w.soccerClean()
}

// soccerEmptyState is FUN_00410f50 with FUN_00411390: every member goes to
// gate 17 and the teams are emptied.
func (w *World) soccerEmptyState() {
	s := w.soccer()
	s.remain, s.end, s.tick = 0, time.Time{}, w.now()
	for t := range s.teams {
		for k, m := range s.teams[t].members {
			if m >= 0 && m < MaxObjects {
				if o := w.objects[m]; o.State == StatePlaying && o.IsPlayer() {
					w.gateMove(o, soccerGate)
				}
			}
			s.teams[t].members[k], s.teams[t].master[k] = -1, false
		}
	}
	w.log.Info("[Battle Soccer] SetState EMPTY")
}

// soccerPurge is FUN_00411400: members that left the game or map 6 leave
// their team (the in-map test of the original compares X with both ends of
// the goal area and can never hold).
func (w *World) soccerPurge() {
	s := w.soccer()
	for t := range s.teams {
		for k, m := range s.teams[t].members {
			if m < 0 || m >= MaxObjects {
				continue
			}
			if o := w.objects[m]; o.State != StatePlaying || o.Map != soccerMap {
				s.teams[t].members[k], s.teams[t].master[k] = -1, false
			}
		}
	}
}

// soccerSecond is FUN_00410ce0.
func (w *World) soccerSecond() {
	s := w.soccer()
	now := w.now()
	if now.Sub(s.tick) < 999*time.Millisecond {
		return
	}
	s.tick = now
	if !s.end.IsZero() {
		s.remain = int(s.end.Sub(now) / time.Second)
	}
	switch s.state {
	case soccerStand:
		w.soccerPurge()
		if s.count(0) != 0 && s.count(1) != 0 {
			if s.remain > 0 {
				return
			}
			w.soccerBallReset()
			w.soccerSet(soccerStart, 900)
			w.log.Info("[Battle Soccer] SetState START")
			return
		}
		w.soccerEnd()
	case soccerStart:
		w.soccerPurge()
		if s.count(0) != 0 && s.count(1) != 0 && s.remain > 0 {
			return
		}
		w.soccerEnd()
	case soccerClean:
		w.soccerPurge()
		if s.remain < 1 {
			w.soccerBallReset()
			s.state = soccerEmpty
			w.soccerEmptyState()
		}
	}
}

// soccerGoalCheck is FUN_00411640 (the AI step of the ball): a ball inside
// goal area i during START scores GuildWarKillScore3 for team i
// (FUN_004867c0, 64 to both guilds); reaching GuildWarScoreMax2 ends the
// soccer.
func (w *World) soccerGoalCheck(ball *Object) {
	s := w.soccer()
	s.ball = ball.Index
	if s.state != soccerStart {
		return
	}
	for i, g := range soccerGoals {
		if ball.X < g[0] || ball.X > g[2] || ball.Y < g[1] || ball.Y > g[3] {
			continue
		}
		w.soccerBallReset()
		scorer, enemy := s.teams[i].guild, s.teams[i^1].guild
		sw := w.guildWarOf(scorer)
		if sw == nil || w.guildWarOf(enemy) == nil {
			continue
		}
		if !w.warAddScore(scorer, enemy, w.cfg.Int("Common", "GuildWarKillScore3", 0)) || sw.typ != 1 {
			continue
		}
		// The score reached the limit: CLEAN first (it reports team 0 as
		// the winner), then FUN_00486660 / FUN_00486780 find the wars
		// already cleared.
		w.soccerEnd()
		return
	}
}

// soccerKillEnd is the soccer branch of FUN_004868f0 when a kill reaches
// the limit: 63 to both guilds, the wars cleared at once, ball reset and
// CLEAN.
func (w *World) soccerKillEnd(kn, vn int) {
	w.warEnd(kn, vn, 1, 0)
	w.clearWar(kn)
	w.clearWar(vn)
	w.soccerEnd()
}

// guildWarAttackRule is the guild war test of FUN_0040b110 / FUN_0046b5a0:
// during a battle soccer that is not in START every attack of a member is
// swallowed (returns 1), and a guild at war (type 0) cannot hit monsters.
func (w *World) guildWarAttackRule(a, t *Object) (result, stop bool) {
	if a.Type != TypePlayer {
		return false, false
	}
	war := w.guildWarOf(guildNumberOf(a))
	if war == nil || !war.active {
		return false, false
	}
	if war.typ == 1 && w.soccerState() != soccerStart {
		return true, true
	}
	if war.typ == 0 && t.Type == TypeMonster {
		return false, true
	}
	return false, false
}

// inSoccerGround is FUN_00411590: inside the ground (x 54..70, y
// 141..180) and not a member of a running match.
func (w *World) inSoccerGround(o *Object) bool {
	if o.X <= 0x35 || o.X >= 0x47 || o.Y <= 0x8C || o.Y >= 0xB5 {
		return false
	}
	s := w.soccer()
	if s.state == soccerEmpty {
		return true
	}
	_, _, member := s.slotOf(o.Index)
	return !member
}

// soccerIntruders is the map 6 part of FUN_00510810: players on the ground
// who do not play go to gate 17.
func (w *World) soccerIntruders() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.Map != soccerMap || !o.Alive {
			continue
		}
		if w.inSoccerGround(o) {
			w.gateMove(o, soccerGate)
		}
	}
}

// soccerRespawn is case 2 of the respawn switch of FUN_004c3b10: a player
// whose death counted in a battle soccer comes back at a random cell of the
// ground (FUN_004115e0: x 54 + rand%16, y 141 + rand%39).
func (w *World) soccerRespawn(o *Object) (int, int, bool) {
	s := w.soccer()
	if !s.respawn[o.Index] {
		return 0, 0, false
	}
	delete(s.respawn, o.Index)
	return 0x36 + rnd(16), 0x8D + rnd(39), true
}

func registerGuildSoccer(h *hooks) {
	h.tick1000 = append(h.tick1000, func(w *World) {
		w.soccerSecond()
		w.soccerIntruders()
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		if w.rq != nil {
			delete(w.soccer().respawn, o.Index)
		}
	})
}
