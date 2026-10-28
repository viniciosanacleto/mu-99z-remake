package game

import (
	"context"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// tickEvents runs n seconds of the custom event timers.
func (b *bareWorld) tickEvents(n int) {
	for i := 0; i < n; i++ {
		b.advance(time.Second)
		b.customEventSecond()
	}
}

// eventPlayer is a non-GM player stored in the memory store (for the
// patente and ranking writes).
func eventPlayer(b *bareWorld, index int, name string, mp, x, y int) *Object {
	o := b.player(index, name, mp, x, y)
	ctx := context.Background()
	acc, err := b.store.CreateAccount(ctx, o.P.Account.Login, "plain:x", "")
	if err != nil {
		b.t.Fatal(err)
	}
	if r, _, err := b.store.CreateCharacter(ctx, acc.ID, name, 0x10); err != nil || r != store.CreateOK {
		b.t.Fatal(err)
	}
	o.P.Account.ID = acc.ID
	o.Class, o.P.Char.Class = 1, 0x10
	return o
}

func (b *bareWorld) storedPatente(o *Object) int {
	b.runAsync()
	c, err := b.store.LoadCharacter(context.Background(), o.P.Account.ID, o.P.Char.Name)
	if err != nil {
		b.t.Fatal(err)
	}
	return c.Patente
}

func (b *bareWorld) customRank(name, rank string) int {
	b.runAsync()
	n, err := b.store.(store.CustomEventStore).CustomRank(context.Background(), name, rank)
	if err != nil {
		b.t.Fatal(err)
	}
	return n
}

func TestCustomEventsLoaded(t *testing.T) {
	b := newBareWorld(t)
	c := b.cev()
	if c.death.data.Syntax != "/irdeath" || c.theft.data.Syntax != "/irevento" || c.sobre.data.Syntax != "/prainhapvp" ||
		c.duel.data.Syntax != "/irmatamata" || c.roleta.data.Syntax != "/irroleta" || c.pair.data.Syntax != "/dupla" ||
		!c.raffle.Active {
		t.Fatalf("not loaded: %+v", c)
	}
}

func TestRaffle(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 25, 7, 59, 59, 0, time.UTC) // Sunday
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	p := b.player(PlayerStart+1, "Lucky", 0, 131, 130)
	fixRand(t, 0)
	b.tickEvents(1) // 08:00:00
	hasText(t, b.drain(gm), b.messageFor(gm, 0x462, "Lucky"))
	hasText(t, b.drain(p), b.messageFor(p, 0x463))
	b.tickEvents(1)
	if len(b.drain(p)) != 0 {
		t.Fatal("raffle ran twice")
	}
}

func TestHideAndSeek(t *testing.T) {
	b := newBareWorld(t)
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	p := b.player(PlayerStart+1, "Seeker", 0, 131, 130)
	b.runCommand(gm, "/escesc")
	h := &b.cev().hide
	if !h.active || h.gm != gm.Index || h.remain != 5*60 {
		t.Fatalf("not started %+v", h)
	}
	pk := b.drain(p)
	hasText(t, pk, b.messageFor(p, 0x68F, "Gm"))
	hasText(t, pk, b.messageFor(p, 0x690))
	b.tickEvents(1)
	hasText(t, b.drain(p), b.messageFor(p, 0x692, 5))
	// A trade request to the GM wins.
	b.handleTradeRequest(p, []byte{0xC1, 5, 0x36, byte(gm.Index >> 8), byte(gm.Index)})
	if h.active || h.remain != 0 {
		t.Fatal("not won")
	}
	if items(p).If.Use {
		t.Fatal("trade opened")
	}
	hasText(t, b.drain(p), b.messageFor(p, 0x691, "Seeker"))
	// Start again and cancel.
	b.runCommand(gm, "/escesc")
	b.runCommand(gm, "/escesc")
	if h.active || h.remain != 0 {
		t.Fatal("not cancelled")
	}
	hasText(t, b.drain(p), b.messageFor(p, 0x68E))
	// Running out of time.
	b.runCommand(gm, "/escesc")
	b.tickEvents(300)
	if h.active {
		t.Fatal("still running")
	}
	hasText(t, b.drain(p), b.messageFor(p, 0x68D))
}

func TestSobreEvent(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().sobre
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	a := eventPlayer(b, PlayerStart+1, "Alpha", 0, 131, 130)
	c := eventPlayer(b, PlayerStart+2, "Charlie", 0, 132, 130)
	// Closed: the player line goes on to the table (GM only: 0x12D).
	b.runCommand(a, "/prainhapvp")
	hasText(t, b.drain(a), b.messageFor(a, 0x12D, "/prainhapvp"))
	b.runCommand(gm, "/prainhapvp")
	hasText(t, b.drain(gm), "Erro de sintaxe!")
	b.runCommand(gm, "/prainhapvp 1")
	if e.state != sobreOpen || e.counter != 61 || gm.X != 240 || gm.Y != 130 {
		t.Fatalf("not started %d %d %d,%d", e.state, e.counter, gm.X, gm.Y)
	}
	a.P.Char.Level = 100
	b.runCommand(a, "/prainhapvp")
	hasText(t, b.drain(a), b.messageFor(a, 0x457, 210))
	a.P.Char.Level = 400
	b.runCommand(a, "/prainhapvp")
	if !gmOf(a).InEvent || a.PKLevel != PKOutlaw2 || a.X != 240 || !e.has(a) {
		t.Fatal("not joined")
	}
	pk := b.drain(a)
	hasText(t, pk, b.messageFor(a, 0x45B))
	// The successful join also reaches the table (0x12D for a player).
	hasText(t, pk, b.messageFor(a, 0x12D, "/prainhapvp"))
	b.runCommand(a, "/prainhapvp")
	hasText(t, b.drain(a), b.messageFor(a, 0x459))
	b.runCommand(c, "/prainhapvp")
	if b.inCustomEvent(c) != true || !b.hooks.commandEventBlock(b.World, c) {
		t.Fatal("event flag")
	}
	// Registered players cannot be attacked while waiting.
	if b.customEventAttack(c, a) {
		t.Fatal("attack while waiting")
	}
	b.tickEvents(1)
	hasText(t, b.drain(a), b.messageFor(a, 0x4B0, 1))
	b.tickEvents(60)
	if e.state != sobreReady {
		t.Fatalf("state %d", e.state)
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x4AF))
	b.tickEvents(6)
	if e.state != sobreFight || !b.customEventAttack(c, a) {
		t.Fatalf("fight %d", e.state)
	}
	// Charlie kills Alpha: Alpha is out.
	b.customEventDeath(c, a)
	if e.has(a) || gmOf(a).InEvent || !gmOf(a).SendHome || a.Map != 0 || a.X != 0x7D {
		t.Fatal("victim still in")
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x4B7, "Alpha"))
	b.tickEvents(2)
	if e.state != sobreClosed || gmOf(c).InEvent {
		t.Fatalf("not finished %d", e.state)
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x4B6, "Charlie"))
	if c.P.Char.Patente != 11 || b.storedPatente(c) != 11 || b.customRank("Charlie", "topsurvivor") != 1 {
		t.Fatal("reward")
	}
	if a.PKLevel != PKCommoner {
		t.Fatal("victim PK not restored")
	}
}

func TestSobreNotEnoughPlayers(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().sobre
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	a := eventPlayer(b, PlayerStart+1, "Alone", 0, 131, 130)
	b.runCommand(gm, "/prainhapvp 1")
	b.runCommand(a, "/prainhapvp")
	b.tickEvents(61)
	if e.state != sobreClosed || gmOf(a).InEvent || a.PKLevel != PKCommoner {
		t.Fatal("not cancelled")
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x4AE))
	b.tickEvents(1)
	if gmOf(a).SendHome || a.Map != 0 || a.X < 0x85 || a.X >= 0x85+15 {
		t.Fatalf("not sent home %d,%d", a.X, a.Y)
	}
}

func TestDeathMatch(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().death
	b.clock = time.Date(2026, 10, 21, 19, 29, 59, 0, time.UTC) // Wednesday
	a := eventPlayer(b, PlayerStart, "Killer", 0, 131, 130)
	v := eventPlayer(b, PlayerStart+1, "Victim", 0, 132, 130)
	b.runCommand(a, "/irdeath")
	hasText(t, b.drain(a), b.messageFor(a, 0x456))
	b.tickEvents(1)
	if !e.open || e.phase != killOpen {
		t.Fatal("not open")
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x465, 5))
	b.runCommand(a, "/irdeath")
	b.runCommand(v, "/irdeath")
	if e.count != 2 || a.Map != 6 || a.X != 116 || v.PKLevel != PKOutlaw2 {
		t.Fatalf("join %d %d", e.count, a.Map)
	}
	b.runCommand(a, "/irdeath")
	hasText(t, b.drain(a), b.messageFor(a, 0x459))
	// Waiting players cannot attack.
	if b.customEventAttack(a, v) {
		t.Fatal("attack while waiting")
	}
	if !b.inCustomEvent(a) {
		t.Fatal("not in event")
	}
	b.tickEvents(5 * 60)
	if !e.running || e.open || e.users[a.Index].Wait {
		t.Fatal("not running")
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x468))
	b.customEventDeath(a, v)
	b.customEventDeath(a, v)
	if e.users[a.Index].Score != 2 {
		t.Fatalf("score %d", e.users[a.Index].Score)
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x46A, 2))
	b.tickEvents(5 * 60)
	// Reset moves them to Lorencia 125,125; obj+0x24f9 then picks a town
	// cell in the same second.
	if e.running || len(e.users) != 0 || a.Map != 0 || gmOf(a).SendHome {
		t.Fatal("not finished")
	}
	hasText(t, b.drain(v), b.messageFor(v, 0x46D, "Killer", 2))
	if b.storedPatente(a) != 9 || b.customRank("Killer", "TopDeath") != 1 {
		t.Fatal("reward")
	}
}

func TestDeathCancelKeepsEntry(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().death
	b.clock = time.Date(2026, 10, 21, 19, 29, 59, 0, time.UTC)
	a := eventPlayer(b, PlayerStart, "Lonely", 0, 131, 130)
	b.tickEvents(1)
	b.runCommand(a, "/irdeath")
	b.tickEvents(5 * 60)
	if e.open || e.running || e.phase != killIdle {
		t.Fatal("not cancelled")
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x469))
	// The original clears obj+0x24f0 instead of the table entry.
	if e.users[a.Index] == nil || a.PKLevel != PKCommoner {
		t.Fatal("entry")
	}
	// Leaving (gate move) clears it.
	b.customEventLeave(a)
	if e.users[a.Index] != nil {
		t.Fatal("leave")
	}
}

func TestTheftOfPoints(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().theft
	b.clock = time.Date(2026, 10, 21, 22, 29, 59, 0, time.UTC)
	a := eventPlayer(b, PlayerStart, "Thief", 0, 131, 130)
	v := eventPlayer(b, PlayerStart+1, "Mark", 0, 132, 130)
	b.tickEvents(1)
	b.runCommand(a, "/irevento")
	b.runCommand(v, "/irevento")
	b.tickEvents(5 * 60)
	if !e.running || e.users[a.Index].Score != 1 {
		t.Fatal("not running")
	}
	b.customEventDeath(v, a) // Mark takes Thief's point
	b.customEventDeath(a, v) // Thief takes Mark's 2 points
	if e.users[a.Index].Score != 3 || e.users[v.Index].Score != 1 {
		t.Fatalf("scores %d %d", e.users[a.Index].Score, e.users[v.Index].Score)
	}
	hasText(t, b.drain(a), b.messageFor(a, 0x4BE, 2, 3))
	b.tickEvents(5 * 60)
	if b.storedPatente(a) != 10 || b.customRank("Thief", "TopTheft") != 1 || a.P.Char.PKTime != 0 {
		t.Fatal("reward")
	}
}

func TestRoletaEvent(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().roleta
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	a := eventPlayer(b, PlayerStart+1, "Brave", 0, 131, 130)
	c := eventPlayer(b, PlayerStart+2, "Lucky", 0, 132, 130)
	b.runCommand(gm, "/rodar 1")
	if e.state != roletaOpen || gm.Map != 6 || gm.X != 94 {
		t.Fatal("not started")
	}
	b.runCommand(a, "/irroleta")
	b.runCommand(c, "/irroleta")
	if len(e.list) != 2 || a.Map != 6 || !b.inCustomEvent(a) {
		t.Fatal("not joined")
	}
	b.tickEvents(61)
	if e.state != roletaReady {
		t.Fatalf("state %d", e.state)
	}
	b.tickEvents(6)
	if e.state != roletaRunning {
		t.Fatalf("state %d", e.state)
	}
	// Brave asks the GM for a trade: the trigger is pulled.
	b.gmWarp(a, 6, 95, 194)
	b.handleTradeRequest(a, []byte{0xC1, 5, 0x36, byte(gm.Index >> 8), byte(gm.Index)})
	if e.shot != 5 || items(a).If.Use {
		t.Fatal("no shot")
	}
	fixRand(t, 3)
	b.tickEvents(5)
	if e.has(a) || a.Alive {
		t.Fatal("not eliminated")
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x4AB, "Brave"))
	b.tickEvents(2)
	if e.state != roletaClosed || b.customRank("Lucky", "TopRoleta") != 1 {
		t.Fatalf("not finished %d", e.state)
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x4A6, "Lucky"))
}

func TestMataMata(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().duel
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	a := eventPlayer(b, PlayerStart+1, "Ace", 0, 131, 130)
	c := eventPlayer(b, PlayerStart+2, "Cid", 0, 132, 130)
	b.runCommand(gm, "/matamata 1")
	hasText(t, b.drain(gm), "Exemplo -> Evento para BKs: 3 1")
	b.runCommand(gm, "/matamata 1 5")
	if e.state != mmOpen || e.class != 5 || gm.Map != 6 || gm.X != 71 {
		t.Fatal("not started")
	}
	b.runCommand(a, "/irmatamata")
	b.runCommand(c, "/irmatamata")
	if len(e.list) != 2 || a.Map != 40 || a.X != 22 || !gmOf(a).InEvent {
		t.Fatal("not joined")
	}
	b.tickEvents(61 + 3)
	if e.state != mmFight || e.a.Index != a.Index || e.b.Index != c.Index || a.Map != 6 || a.Y != 10 || c.Y != 12 {
		t.Fatalf("not paired %d", e.state)
	}
	if b.customEventAttack(a, c) || b.customEventAttack(gm, a) {
		t.Fatal("attack before the countdown or by an outsider")
	}
	b.tickEvents(6)
	if !b.customEventAttack(a, c) || !b.customEventAttack(c, a) || b.customEventAttack(a, gm) {
		t.Fatal("fight rules")
	}
	for round := 0; round < 3; round++ {
		b.customEventDeath(a, c)
		if e.state != mmDecide {
			t.Fatalf("round %d state %d", round, e.state)
		}
		b.tickEvents(4)
		if round < 2 {
			b.tickEvents(6)
		}
	}
	if e.a.Score != 0 || len(e.winners) != 1 && e.state != mmRound {
		t.Fatalf("winner %+v %v", e.a, e.winners)
	}
	if gmOf(c).InEvent || c.X != 0x7D {
		t.Fatal("loser not out")
	}
	b.tickEvents(3) // pairing (nobody left), round (one winner), champion
	if e.state != mmClosed || gmOf(a).InEvent {
		t.Fatalf("not finished %d", e.state)
	}
	hasText(t, b.drain(c), b.messageFor(c, 0x487, "Ace"))
	if b.storedPatente(a) != 8 || b.customRank("Ace", "Duelall") != 1 {
		t.Fatal("reward")
	}
}

func TestCustomEventBlocks(t *testing.T) {
	b := newBareWorld(t)
	a := b.player(PlayerStart, "Busy", 0, 130, 130)
	p := b.player(PlayerStart+1, "Other", 0, 131, 130)
	gmOf(a).InEvent = true
	if !b.customEventPacketBlock(a, 0x36) || b.customEventPacketBlock(a, 0x24) || b.customEventPacketBlock(p, 0x36) {
		t.Fatal("block rule")
	}
	hasText(t, b.drain(a), "Proibido dar trade no evento.")
	b.customEventPacketBlock(a, 0x40)
	hasText(t, b.drain(a), "Proibido formar party no evento.")
	b.customEventPacketBlock(a, 0xAA)
	hasText(t, b.drain(a), "Proibido iniciar o duelo no evento.")
	// A trade request with an event player opens nothing.
	b.handleTradeRequest(p, []byte{0xC1, 5, 0x36, byte(a.Index >> 8), byte(a.Index)})
	if items(p).If.Use {
		t.Fatal("trade opened")
	}
	// obj+0x24f8 / 0x24f9 on the next second.
	g := gmOf(a)
	g.RestorePK, g.SendHome = true, true
	a.PKLevel = PKOutlaw2
	a.Class = 2
	b.customEventSecond()
	if a.PKLevel != PKCommoner || a.Map != 3 || g.SendHome || g.RestorePK {
		t.Fatalf("return %d %d", a.PKLevel, a.Map)
	}
}

func TestPatenteCommand(t *testing.T) {
	b := newBareWorld(t)
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	a := eventPlayer(b, PlayerStart+1, "Holder", 0, 131, 130)
	c := eventPlayer(b, PlayerStart+2, "Taker", 0, 132, 130)
	b.runCommand(gm, "/patente Holder 7")
	if a.P.Char.Patente != 7 || b.storedPatente(a) != 7 {
		t.Fatal("not set")
	}
	hasText(t, b.drain(a), b.messageFor(a, msgGMEditedBy, "Gm"))
	b.runCommand(gm, "/patente Taker 7")
	if a.P.Char.Patente != 0 || c.P.Char.Patente != 7 || b.storedPatente(a) != 0 {
		t.Fatal("not moved")
	}
	if _, ok := mv(gm).CommandTicks[134]; ok {
		t.Fatal("/patente charged")
	}
}

func TestPairEvent(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().pair
	gm := gmPlayer(b, PlayerStart, "Gm", 8, 0, 130, 130)
	// "/dupla" is the player syntax too: the join stops the line, so the
	// GM command never runs with the original files.
	b.runCommand(gm, "/dupla 1")
	if e.state != pairClosed {
		t.Fatal("GM /dupla reached the table")
	}
	e.start(1)
	var ps []*Object
	for i, n := range []string{"P1", "P2", "P3", "P4", "P5"} {
		o := eventPlayer(b, PlayerStart+1+i, n, 0, 131+i, 130)
		b.runCommand(o, "/dupla")
		ps = append(ps, o)
	}
	if len(e.list) != 5 || !gmOf(ps[0]).InEvent || ps[0].X != 240 {
		t.Fatal("not joined")
	}
	b.tickEvents(61)
	// Five players: two teams, the last one is out.
	if e.state != pairPair || len(e.teams) != 2 || gmOf(ps[4]).InEvent {
		t.Fatalf("teams %d %d", e.state, len(e.teams))
	}
	hasText(t, b.drain(ps[4]), b.messageFor(ps[4], 0x48A))
	b.tickEvents(3)
	if e.state != pairFight || e.t[0].M[0].Index != ps[0].Index || e.t[1].M[1].Index != ps[3].Index {
		t.Fatalf("not paired %d", e.state)
	}
	hasText(t, b.drain(gm), b.messageFor(gm, 0x491, "P1", "P2", "P3", "P4"))
	b.tickEvents(3) // counter 5: to the arena
	if ps[0].Map != 0 || ps[0].X != 239 || ps[0].Y != 129 || ps[3].X != 241 || ps[3].Y != 131 || ps[0].PKLevel != PKOutlaw2 {
		t.Fatalf("arena %d,%d %d,%d", ps[0].X, ps[0].Y, ps[3].X, ps[3].Y)
	}
	// Teammates never hit each other; before the start nobody fights.
	if b.customEventAttack(ps[0], ps[1]) || b.customEventAttack(ps[0], ps[2]) {
		t.Fatal("attack rules before the start")
	}
	b.tickEvents(5)
	if !b.customEventAttack(ps[0], ps[2]) || b.customEventAttack(ps[2], ps[3]) {
		t.Fatal("attack rules after the start")
	}
	for round := 0; round < 3; round++ {
		b.customEventDeath(ps[0], ps[2])
		b.customEventDeath(ps[0], ps[3])
		if e.state != pairDecide {
			t.Fatalf("round %d state %d", round, e.state)
		}
		b.tickEvents(4)
		if round < 2 {
			if e.state != pairFight || e.t[0].Score != round+1 {
				t.Fatalf("round %d score %d", round, e.t[0].Score)
			}
			b.tickEvents(8)
		}
	}
	if gmOf(ps[2]).InEvent || ps[2].X != 0x7D || len(e.winners) != 1 {
		t.Fatal("losers not out")
	}
	b.tickEvents(3) // pairing (none left), round (one team), champions
	if e.state != pairClosed || gmOf(ps[0]).InEvent {
		t.Fatalf("not finished %d", e.state)
	}
	hasText(t, b.drain(gm), b.messageFor(gm, 0x49D, "P1", "P2"))
	if b.storedPatente(ps[0]) != 12 || b.storedPatente(ps[1]) != 10 ||
		b.customRank("P1", "TopDupla") != 1 || b.customRank("P2", "TopDupla") != 13 {
		t.Fatal("rewards")
	}
}

func TestPairLeaveDuringFight(t *testing.T) {
	b := newBareWorld(t)
	e := &b.cev().pair
	e.start(1)
	var ps []*Object
	for i, n := range []string{"Q1", "Q2", "Q3", "Q4"} {
		o := eventPlayer(b, PlayerStart+i, n, 0, 131+i, 130)
		b.runCommand(o, "/dupla")
		ps = append(ps, o)
	}
	b.tickEvents(61 + 3 + 3)
	if !e.t[1].M[0].InFight {
		t.Fatal("not fighting")
	}
	b.customEventLeave(ps[2])
	if e.state != pairBroken || gmOf(ps[2]).InEvent {
		t.Fatalf("state %d", e.state)
	}
	b.tickEvents(1)
	if e.state != pairPair || len(e.winners) != 1 || e.winners[0].M[0].Index != ps[0].Index ||
		gmOf(ps[3]).InEvent || ps[1].Map != 0 || ps[1].X != 240 {
		t.Fatalf("walkover %d %v", e.state, e.winners)
	}
}
