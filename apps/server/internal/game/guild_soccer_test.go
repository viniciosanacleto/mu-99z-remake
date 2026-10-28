package game

import (
	"testing"
	"time"
)

// soccerMatch creates the guilds Alpha (m1) and Beta (m2), asks a battle
// soccer from Alpha and lets Beta accept it with both masters on the
// ground: Beta (the answering guild) is team 0.
func soccerMatch(t *testing.T) (*bareWorld, *Object, *Object, int, int) {
	t.Helper()
	b := newBareWorld(t)
	w := b.World
	m1 := b.player(PlayerStart, "MasterA", 0, 130, 130)
	m2 := b.player(PlayerStart+1, "MasterB", 0, 131, 130)
	g1 := guildSetup(t, b, m1, "Alpha")
	g2 := guildSetup(t, b, m2, "Beta")
	if !w.guildWarRequest(m1, "Beta", 1) {
		t.Fatal("soccer request refused")
	}
	for _, p := range []*Object{m1, m2} {
		b.occupy(p, false)
		p.Map, p.Y = soccerMap, 160
		b.occupy(p, true)
	}
	m1.X, m2.X = 58, 66
	b.drain(m1)
	b.drain(m2)
	b.hooks.worldHandlers[0x61](w, m2, []byte{0xC1, 4, 0x61, 1})
	return b, m1, m2, g1, g2
}

func TestSoccerStateMachine(t *testing.T) {
	b, m1, m2, _, g2 := soccerMatch(t)
	s := b.soccer()
	if s.state != soccerStand || s.teams[0].guild != g2 || s.teams[0].members[0] != m2.Index ||
		s.teams[1].members[0] != m1.Index || !s.teams[0].master[0] {
		t.Fatalf("stand = %+v", *s)
	}
	if r := find(b.drain(m1), 0x62, -1); r == nil || r[11] != 1 || r[12] != 1 {
		t.Fatalf("62 = % X", r)
	}
	// Outside START every attack of a member is swallowed (FUN_0040b110).
	if r, stop := b.guildWarAttackRule(m1, m2); !r || !stop {
		t.Fatal("STAND attack not swallowed")
	}
	// A second soccer is refused while the ground is busy.
	if b.soccerBegin(m1, 99, 98) {
		t.Fatal("busy ground accepted")
	}
	if r := find(b.drain(m1), 0x60, -1); r == nil || r[3] != 4 {
		t.Fatalf("busy = % X", r)
	}
	// STAND lasts 10 seconds, START 900.
	b.advance(11 * time.Second)
	b.soccerSecond()
	if s.state != soccerStart || s.remain != 900 {
		t.Fatalf("start = %d/%d", s.state, s.remain)
	}
	if _, stop := b.guildWarAttackRule(m1, m2); stop {
		t.Fatal("START attack blocked")
	}
	// A team member leaving map 6 empties the team: CLEAN.
	m1.Map = 0
	b.advance(time.Second)
	b.soccerSecond()
	if s.state != soccerClean || b.guildWarOf(g2) != nil {
		t.Fatalf("clean = %d", s.state)
	}
	if r := find(b.drain(m2), 0x63, -1); r == nil || r[3] != 1 {
		t.Fatalf("63 to team 0 = % X", r)
	}
	// CLEAN lasts 10 s; EMPTY sends the members to gate 17 (Lorencia).
	b.advance(11 * time.Second)
	b.soccerSecond()
	if s.state != soccerEmpty || m2.Map != 0 || s.teams[0].members[0] != -1 {
		t.Fatalf("empty: state %d m2 map %d", s.state, m2.Map)
	}
}

func TestSoccerGoalsEndTheMatch(t *testing.T) {
	b, m1, m2, g1, g2 := soccerMatch(t)
	b.advance(11 * time.Second)
	b.soccerSecond()
	ball := b.soccerBall()
	if ball == nil {
		t.Fatal("no ball")
	}
	b.drain(m1)
	b.drain(m2)
	// A ball inside goal area 0 scores GuildWarKillScore3 (20) for team 0
	// and goes back to its spawn.
	ball.X, ball.Y = 62, 181
	b.soccerGoalCheck(ball)
	if b.warScore(g2) != 20 || b.warScore(g1) != 0 {
		t.Fatalf("scores %d / %d", b.warScore(g2), b.warScore(g1))
	}
	if ball.Y >= 181 {
		t.Fatalf("ball not reset: %d,%d", ball.X, ball.Y)
	}
	if r := find(b.drain(m1), 0x64, -1); r == nil || r[3] != 0 || r[4] != 20 {
		t.Fatalf("64 = % X", r)
	}
	// Goal area 1 scores for team 1; GuildWarScoreMax2 (100) ends it.
	for i := 0; i < 5; i++ {
		ball.X, ball.Y = 61, 139
		b.soccerGoalCheck(ball)
	}
	s := b.soccer()
	if s.state != soccerClean || b.guildWarOf(g1) != nil || b.guildWarOf(g2) != nil {
		t.Fatalf("after the limit: state %d", s.state)
	}
	// FUN_00410fd0 runs first and reports team 0 (Beta) as the winner.
	if r := find(b.drain(m2), 0x63, -1); r == nil || r[3] != 1 {
		t.Fatalf("63 to Beta = % X", r)
	}
	if r := find(b.drain(m1), 0x63, -1); r == nil || r[3] != 0 {
		t.Fatalf("63 to Alpha = % X", r)
	}
}

func TestSoccerBallAndIntruders(t *testing.T) {
	b, m1, m2, _, _ := soccerMatch(t)
	b.advance(11 * time.Second)
	b.soccerSecond()
	ball := b.soccerBall()
	// Hitting the ball rolls it 2 cells away and misses (the viewport
	// cycle has shown the reset ball: live state 2).
	ball.Live = LiveNormal
	m1.X, m1.Y = ball.X, ball.Y-1
	x, y := ball.X, ball.Y
	life := ball.Mon.Life
	b.attackFull(m1, ball, attackArgs{})
	if ball.Mon.Life != life || ball.Y != y+2 || ball.X != x {
		t.Fatalf("ball at %d,%d (from %d,%d) life %v", ball.X, ball.Y, x, y, ball.Mon.Life)
	}
	// A player who does not play is sent to gate 17 from the ground.
	in := b.player(PlayerStart+2, "Intruder", soccerMap, 60, 150)
	b.soccerIntruders()
	if in.Map != 0 || m2.Map != soccerMap {
		t.Fatalf("intruder map %d, player map %d", in.Map, m2.Map)
	}
	// A war (type 0) cannot hit monsters.
	b.guildWarOf(guildNumberOf(m1)).typ = 0
	if r, stop := b.guildWarAttackRule(m1, ball); r || !stop {
		t.Fatal("war member hit a monster")
	}
}

func TestSoccerKillRespawn(t *testing.T) {
	b, m1, m2, _, _ := soccerMatch(t)
	b.advance(11 * time.Second)
	b.soccerSecond()
	if !b.guildWarKill(m1, m2) {
		t.Fatal("kill not counted")
	}
	fixRand(t, 5)
	x, y, ok := b.soccerRespawn(m2)
	if !ok || x != 0x36+5 || y != 0x8D+5 {
		t.Fatalf("respawn = %d,%d %v", x, y, ok)
	}
	if _, _, ok := b.soccerRespawn(m2); ok {
		t.Fatal("respawn mark kept")
	}
}
