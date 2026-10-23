package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// noText fails when a text is among the notices.
func noText(t *testing.T, pkts [][]byte, text string) {
	t.Helper()
	if slices.Contains(texts(pkts), text) {
		t.Fatalf("unexpected %q", text)
	}
}

// dailyAt is a schedule row for every day at h:m:00.
func dailyAt(h, m int) gamedata.Schedule {
	return gamedata.Schedule{Year: -1, Month: -1, Day: -1, DayOfWeek: -1, Hour: h, Minute: m, Second: 0}
}

// arenaFixture enables the arena on map 6 with arena 0 entering by a gate
// of map 6 and leaving by gate 17 (Lorencia).
func arenaFixture(t *testing.T) (*bareWorld, *arenaEntry, int) {
	b := newBareWorld(t)
	a := b.arena()
	a.on, a.mapNumber, a.victimDec, a.killerInc = true, 6, 2, 3
	gate := -1
	for i, g := range b.data.Gates {
		if g.Map == 6 && (gate == -1 || i < gate) {
			gate = i
		}
	}
	if gate < 0 {
		t.Fatal("no gate of map 6")
	}
	a.data = &gamedata.CustomArena{}
	a.data.Rules[0] = gamedata.CustomArenaRule{Name: "[Test]", AlarmTime: 5, StandTime: 1, EventTime: 10, CloseTime: 4,
		StartGate: gate, FinalGate: 17, MinUser: 2, MaxUser: 100, MaxDeath: 2, MinLevel: -1, MaxLevel: -1,
		MinReset: -1, MaxReset: -1, MinMasterReset: -1, MaxMasterReset: -1, RewardBag: -1, Class: [5]int{1, 1, 1, 1, 1},
		Loaded: true}
	a.data.Schedule[0] = []gamedata.Schedule{dailyAt(0, 3)} // every day 00:03:00
	for i := range a.e {
		a.e[i] = arenaEntry{Index: i, Minute: -1, LastMin: -1}
		a.e[i].clearRows()
	}
	b.arenaInit()
	return b, &a.e[0], gate
}

func TestCustomArenaFlow(t *testing.T) {
	b, e, gate := arenaFixture(t)
	if e.State != arenaStateWait || e.Remain != 180 || b.arena().e[1].State != arenaStateNone {
		t.Fatalf("init %+v", e)
	}
	p1 := b.player(PlayerStart, "Alpha", 0, 130, 130)
	p2 := b.player(PlayerStart+1, "Beta", 0, 131, 130)
	// Before the alarm window the registration is closed.
	if b.arenaJoin(p1, gate) {
		t.Fatal("joined a closed arena")
	}
	hasText(t, b.drain(p1), b.messageFor(p1, msgArenaNotOpen))
	// 00:00:01: 179 s left, inside the 5 minutes: minute 2 announced.
	b.advance(time.Second)
	b.arenaSecond()
	if !e.Enter || e.LastMin != 2 {
		t.Fatalf("alarm %+v", e)
	}
	hasText(t, b.drain(p1), b.messageFor(p1, msgArenaAlarm, "[Test]", 3))
	b.arenaSecond()
	noText(t, b.drain(p1), b.messageFor(p1, msgArenaAlarm, "[Test]", 3))
	// Class column 0 refuses; a gate of another map is not an arena move.
	b.arena().data.Rules[0].Class[0] = 0
	p1.Class = 0
	if b.arenaJoin(p1, gate) {
		t.Fatal("class allowed")
	}
	hasText(t, b.drain(p1), b.messageFor(p1, msgArenaWrongClas))
	b.arena().data.Rules[0].Class[0] = 1
	if !b.arenaJoin(p1, 17) {
		t.Fatal("Lorencia move refused")
	}
	if !b.arenaJoin(p1, gate) || !b.arenaJoin(p2, gate) || b.arenaJoin(p1, gate) || b.arenaCount(e) != 2 {
		t.Fatalf("join count %d", b.arenaCount(e))
	}
	b.gateMove(p1, gate)
	b.gateMove(p2, gate)
	p1.Live, p2.Live = LiveNormal, LiveNormal
	if p1.Map != 6 || !b.arenaListed(p1) || b.arenaFighting(p1) {
		t.Fatal("listed / fighting")
	}
	// Already on the arena map: 0x1FC.
	p3 := b.player(PlayerStart+2, "Gamma", 6, 60, 60)
	if b.arenaJoin(p3, gate) {
		t.Fatal("joined from the arena map")
	}
	hasText(t, b.drain(p3), b.messageFor(p3, msgArenaCannot))
	// The viewport shows the players with PK level 6.
	entry := make([]byte, 44)
	viewportPlayerEntry(entry, p1, false)
	b.customViewEntry(p2, p1, entry)
	if entry[40]&0x0F != 6 {
		t.Fatalf("pk nibble %x", entry[40])
	}
	// Start: registration closes, standby.
	b.advance(179 * time.Second)
	b.drain(p1)
	b.arenaSecond()
	if e.State != arenaStateStand || e.Enter || e.Remain != 60 {
		t.Fatalf("standby %+v", e)
	}
	pk := b.drain(p1)
	hasText(t, pk, b.messageFor(p1, msgArenaClosed, "[Test]"))
	hasText(t, pk, b.messageFor(p1, msgArenaStandby, "[Test]", 1))
	b.advance(60 * time.Second)
	b.arenaSecond()
	if e.State != arenaStatePlay || e.Remain != 600 || !b.arenaFighting(p1) {
		t.Fatalf("fight %+v", e)
	}
	hasText(t, b.drain(p2), b.messageFor(p2, msgArenaStart, "[Test]"))
	if !b.pvpAllowed(p1, p2) {
		t.Fatal("arena players cannot fight")
	}
	// p1 kills p2 twice: +3 each, p2 loses min(score, 2).
	b.arenaDeath(p1, p2)
	b.arenaDeath(p1, p2)
	u1, u2 := e.user(p1.Index), e.user(p2.Index)
	if u1.Score != 6 || u1.Kills != 2 || u1.KillStreak != 2 || u2.Deaths != 2 || u2.DeathStreak != 2 || u2.Score != 0 {
		t.Fatalf("scores %+v %+v", *u1, *u2)
	}
	b.arenaDeath(p2, p1)
	if u1.Score != 4 || u1.KillStreak != 0 || u2.Score != 3 || u2.DeathStreak != 0 {
		t.Fatalf("revenge %+v %+v", *u1, *u2)
	}
	// Respawn: p1 (1 death) on the StartGate, p2 (2 deaths) out.
	if mp, _, _, ok := b.arenaRespawn(p1); !ok || mp != 6 || e.user(p1.Index) == nil {
		t.Fatal("respawn on the start gate")
	}
	if mp, _, _, ok := b.arenaRespawn(p2); !ok || mp != 0 || e.user(p2.Index) != nil {
		t.Fatalf("respawn out %d", mp)
	}
	// One player left: results at once, without 0x1F9.
	b.drain(p1)
	b.arenaSecond()
	if e.State != arenaStateClose || u1.Rank != 1 || e.Remain != 240 {
		t.Fatalf("close %+v %+v", e.State, *u1)
	}
	pk = b.drain(p1)
	noText(t, pk, b.messageFor(p1, msgArenaEnd, "[Test]"))
	hasText(t, pk, b.messageFor(p1, msgArenaWinner, "[Test]", "Alpha"))
	hasText(t, pk, b.messageFor(p1, msgArenaRank, "[Test]", 1, 4))
	// CloseTime over: back to the waiting state, players to the FinalGate.
	mv(p1).Regen = 0 // the client answered the earlier move (F3 12)
	b.advance(240 * time.Second)
	b.arenaSecond()
	if e.State != arenaStateWait || b.arenaCount(e) != 0 || p1.Map != 0 {
		t.Fatalf("restart %+v map %d", e.State, p1.Map)
	}
}

func TestCustomArenaRankAndStandby(t *testing.T) {
	b, e, gate := arenaFixture(t)
	for i := range 3 {
		b.arenaAdd(e, PlayerStart+i)
	}
	e.Users[0].Score, e.Users[1].Score, e.Users[2].Score = 5, 7, 5
	b.arenaRank(e)
	if e.Users[0].Rank != 2 || e.Users[1].Rank != 1 || e.Users[2].Rank != 3 {
		t.Fatalf("ranks %+v", e.Users[:3])
	}
	// Fewer than MinUser in the standby: everybody leaves (FinalGate).
	b2, e2, _ := arenaFixture(t)
	p := b2.player(PlayerStart, "Solo", 6, 60, 60)
	b2.arenaAdd(e2, p.Index)
	e2.State = arenaStateStand
	b2.arenaSecond()
	if e2.State != arenaStateWait || b2.arenaCount(e2) != 0 || p.Map != 0 {
		t.Fatalf("standby restart %+v %d", e2.State, p.Map)
	}
	_ = gate
	_ = b
}

func TestCustomEventDrop(t *testing.T) {
	b := newBareWorld(t)
	d := b.edrop()
	d.on = true
	x, y := b.openRow(0, 8)
	d.data = &gamedata.CustomEventDrop{}
	d.data.Schedule[0] = []gamedata.Schedule{dailyAt(0, 2)}
	d.data.Rules[0] = gamedata.CustomEventDropRule{Name: "Rain", Map: 0, X: x, Y: y, Radius: 2, AlarmTime: 5, EventTime: 1}
	d.data.Items[0] = []gamedata.CustomEventDropItem{{Index: 14*512 + 13, Count: 2, Delay: 0},
		{Index: 14*512 + 14, Count: 1, Delay: 30}}
	b.eventDropInit()
	o := b.player(PlayerStart, "Watcher", 0, x, y)
	e := &d.e[0]
	b.eventDropSecond()
	hasText(t, b.drain(o), b.messageFor(o, msgEventDropMin, "Rain", 2))
	b.advance(120 * time.Second)
	b.eventDropSecond()
	if e.State != eventDropRun || e.Remain != 60 {
		t.Fatalf("start %+v", e)
	}
	hasText(t, b.drain(o), b.messageFor(o, msgEventDropGo, "Rain"))
	b.eventDropSecond()
	pk := b.drain(o)
	if n := countPackets(pk, 0xF3, 0x40); n != 2 || !e.Dropped[0] || e.Dropped[1] {
		t.Fatalf("first drops %d %v", n, e.Dropped)
	}
	b.advance(30 * time.Second)
	b.eventDropSecond()
	if !e.Dropped[1] {
		t.Fatal("delayed row not dropped")
	}
	b.advance(30 * time.Second)
	b.drain(o)
	b.eventDropSecond()
	hasText(t, b.drain(o), b.messageFor(o, msgEventDropEnd, "Rain"))
	if e.State != eventDropWait || e.Dropped[0] {
		t.Fatalf("end %+v", e)
	}
}

func countPackets(pkts [][]byte, head byte, sub int) int {
	n := 0
	for _, p := range pkts {
		if find([][]byte{p}, head, sub) != nil {
			n++
		}
	}
	return n
}

func TestLukeHelper(t *testing.T) {
	b := newBareWorld(t)
	l := b.luke()
	l.on = true
	x, y := b.openRow(0, 8)
	l.data = &gamedata.CustomLukeHelper{EventTime: 2, Schedule: []gamedata.Schedule{dailyAt(0, 4)},
		Spots: []gamedata.LukeSpot{{Key: 0, Map: 0, Class: 258, StartX: x, StartY: y, EndX: x, EndY: y, Bag: -1}}}
	l.State = lukeWait
	b.lukeSchedule()
	o := b.player(PlayerStart, "Seeker", 0, x+1, y)
	b.lukeSecond() // 240 s left (below 300): "4 minutes"
	hasText(t, b.drain(o), b.messageFor(o, msgLukeMinutes, 4))
	b.advance(time.Second)
	b.lukeSecond() // 239 s: still minute 3, not announced again
	noText(t, b.drain(o), b.messageFor(o, msgLukeMinutes, 4))
	b.advance(239 * time.Second)
	b.lukeSecond()
	if l.State != lukeOut || !validIdx(l.Monster) || l.Remain != 120 {
		t.Fatalf("out %+v", l)
	}
	npc := b.objects[l.Monster]
	pk := b.drain(o)
	hasText(t, pk, b.messageFor(o, msgLukeAppeared, b.data.Maps[0].Name))
	hasText(t, pk, b.messageFor(o, msgLukeFindIt))
	if npc.Class != 258 || npc.Map != 0 {
		t.Fatalf("npc %d map %d", npc.Class, npc.Map)
	}
	if !b.npcTalk(o, npc) {
		t.Fatal("talk not handled")
	}
	pk = b.drain(o)
	if countPackets(pk, 0xF3, 0x40) != 10 {
		t.Fatal("fireworks")
	}
	hasText(t, pk, b.messageFor(o, msgLukeFound, "Seeker"))
	if l.State != lukeWait || validIdx(l.Monster) || npc.State != StateEmpty {
		t.Fatalf("after talk %+v", l)
	}
	// Window row: waiting, with the time to the next day's start.
	if r := b.lukeWindow(o); len(r) != 1 || !r[0].Open || r[0].Remain != l.Remain {
		t.Fatalf("window %+v", r)
	}
}

func TestHealthBarPacket(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Viewer", 0, x, y)
	m := b.monsterNear(3, p, 1)
	b.see(p, m)
	m.Mon.Life, m.Mon.MaxLife = 50, 200
	b.hpBar().rows = []gamedata.HealthBarRow{{Class: -1, Map: -1, Switch: 1}}
	pk := b.packetHealthBar(p)
	want := []byte{0xC2, 0x00, 0x0A, 0xF3, 0xE2, 1, byte(m.Index), byte(m.Index >> 8), 2, 25}
	if !bytes.Equal(pk, want) {
		t.Fatalf("% X", pk)
	}
	b.hpBar().rows = []gamedata.HealthBarRow{{Class: 3, Map: -1, Switch: 0}, {Class: -1, Map: -1, Switch: 1}}
	if pk := b.packetHealthBar(p); pk[5] != 0 || len(pk) != 6 {
		t.Fatalf("switched off % X", pk)
	}
	if !b.healthBarMap(0) {
		t.Fatal("MonsterHealthBarSwitch")
	}
}

func TestMonsterKillCount(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(0, 8)
	p := b.player(PlayerStart, "Hunter", 0, x, y)
	p.P.Char.Money = 0
	b.kills().rows = []gamedata.KillCountRow{{DBIndex: 0, Monster: 3, AL: [4]int{1, 0, 0, 0}, KillCount: 2, Money: 500,
		Message: "Spider reward", Items: []gamedata.KillCountItem{{Index: 14*512 + 13}}}}
	m := b.monsterNear(3, p, 1)
	b.killCountKill(m, p)
	b.runAsync()
	hasText(t, b.drain(p), m.Mon.Info.Name+" 1/2")
	b.killCountKill(m, p)
	b.runAsync()
	pk := b.drain(p)
	hasText(t, pk, "Spider reward")
	if p.P.Char.Money != 500 || find(pk, 0xF3, 0x40) == nil {
		t.Fatalf("reward money %d", p.P.Char.Money)
	}
	b.killCountKill(m, p) // 3 > 2: nothing
	b.runAsync()
	if len(texts(b.drain(p))) != 0 {
		t.Fatal("message after the count")
	}
	p.P.Account.Level = 1 // column AL1 off: no request
	b.killCountKill(m, p)
	b.runAsync()
	if n, _ := b.store.(store.CustomWorldStore).AddMonsterKill(context.Background(), "Hunter", 3); n != 4 {
		t.Fatalf("count %d", n)
	}
}

func TestSummonScroll(t *testing.T) {
	b := newBareWorld(t)
	x, y := b.openRow(3, 8)
	o := b.player(PlayerStart, "Caller", 3, x, y)
	scroll := item.Empty()
	scroll.Index = 14*512 + 213
	before := b.countMonsters(3)
	handled, ok := b.summonScrollDrop(o, scroll, x+1, y)
	if !handled || !ok {
		t.Fatal("scroll not used")
	}
	if n := b.countMonsters(3) - before; n != 6 {
		t.Fatalf("summoned %d", n)
	}
	sx, sy := b.safeCell(0)
	o2 := b.player(PlayerStart+1, "Safe", 0, sx, sy)
	if handled, ok := b.summonScrollDrop(o2, scroll, sx, sy); !handled || ok {
		t.Fatal("scroll used in the safe zone")
	}
	other := item.Empty()
	other.Index = 14*512 + 13
	if handled, _ := b.summonScrollDrop(o, other, x, y); handled {
		t.Fatal("not a scroll")
	}
	if !summonEventMap(9) || !summonEventMap(20) || summonEventMap(3) {
		t.Fatal("event maps")
	}
}

func (b *bareWorld) countMonsters(mp int) int {
	n := 0
	for i := 0; i < SummonStart; i++ {
		if o := b.objects[i]; o.State == StatePlaying && o.Type == TypeMonster && o.Map == mp {
			n++
		}
	}
	return n
}

func TestCreationGold(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Jeweler", 0, 130, 130)
	jewel := item.Empty()
	jewel.Index, jewel.Durability = creationJewel, 1
	o.P.Inventory[12] = jewel
	if b.store != nil {
		b.store.(*store.Memory).AddGold(context.Background(), o.P.Account.Login, 0)
	}
	req := []byte{0xC1, 0x05, 0xFA, 0x12, 12}
	faHandlers[0x12](b.World, o, req)
	hasText(t, b.drain(o), "Clique novamente com o botao direito para usar a Creation e receber +20 Gold.")
	b.advance(50 * time.Millisecond)
	faHandlers[0x12](b.World, o, req) // too fast: ignored
	if len(b.drain(o)) != 0 || o.P.Inventory[12].IsEmpty() {
		t.Fatal("fast click used")
	}
	b.advance(100 * time.Millisecond)
	faHandlers[0x12](b.World, o, req)
	b.runAsync()
	pk := b.drain(o)
	hasText(t, pk, "Voce recebeu +20 Gold de bonus usando a Creation!")
	if !o.P.Inventory[12].IsEmpty() || find(pk, 0x28, -1) == nil {
		t.Fatal("jewel not consumed")
	}
	if g, _ := b.store.(store.CustomWorldStore).Gold(context.Background(), o.P.Account.Login); g != 20 {
		t.Fatalf("gold %d", g)
	}
	// A late second click arms again.
	o.P.Inventory[12] = jewel
	faHandlers[0x12](b.World, o, req)
	b.advance(2 * time.Second)
	faHandlers[0x12](b.World, o, req)
	if o.P.Inventory[12].IsEmpty() {
		t.Fatal("late click used")
	}
}

func TestDailyRewardPackets(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var flags [dailyDays]bool
	flags[0], flags[22] = true, true
	p := packetDailyMonth(now, flags)
	if len(p) != 0x2C || p[2] != 0xFB || p[3] != 0x17 || p[4] != 31 || p[5] != 23 || p[6] != byte(time.Thursday) ||
		p[7] != 1 || p[29] != 1 || p[8] != 0 || binary.LittleEndian.Uint32(p[40:]) != uint32(now.Unix()) {
		t.Fatalf("FB 17 % X", p)
	}
	for m, want := range map[time.Month]int{2: 28, 4: 30, 7: 31, 8: 31, 9: 30, 12: 31} {
		if got := daysInMonth(time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC)); got != want {
			t.Fatalf("month %d: %d", m, got)
		}
	}
	if daysInMonth(time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC)) != 29 {
		t.Fatal("leap")
	}
	r := gamedata.DailyRewardRow{Day: 1, Index: 512 + 3, Level: 13, Skill: 1, Luck: 1, Option: 7, Durability: 50,
		Excellent: 63, Ancient: 5, Info1: "Axe", Info2: "+13"}
	q := packetDailyItem(r)
	if len(q) != 0xE0 || q[3] != 0x18 || binary.LittleEndian.Uint16(q[4:]) != 515 || q[6] != 50 ||
		!bytes.Equal(q[7:12], []byte{3, 13<<3 | 0x80 | 4 | 3, 50, 0x40 | 63, 5}) ||
		string(q[24:27]) != "Axe" || string(q[124:127]) != "+13" {
		t.Fatalf("FB 18 % X", q)
	}
	r.Index = 0x103 // bit 8 of the index goes to bit 7 of [3]
	if b := dailyItemBytes(r); b[0] != 3 || b[3] != 0x80|0x40|63 {
		t.Fatalf("item bytes % X", b)
	}
}

func TestDailyRewardClaim(t *testing.T) {
	b := newBareWorld(t)
	b.clock = time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	o := b.player(PlayerStart, "Daily", 0, 130, 130)
	cs := b.store.(store.CustomWorldStore)
	cs.AddDailyReward(context.Background(), o.P.Account.Login, "Daily", 1, 10)
	b.dailyLoad(o)
	b.runAsync()
	pk := b.drain(o)
	if m := find(pk, 0xFB, 0x17); m == nil || m[7] != 1 || m[9] != 0 {
		t.Fatalf("FB 17 %v", m)
	}
	if find(pk, 0xFB, 0x18) == nil {
		t.Fatal("FB 18")
	}
	claim := func(day byte) [][]byte {
		b.handleDailyReward(o, []byte{0xC1, 0x0C, 0xFB, 0x02, 2, 0, 0, 0, day, 0, 0, 0})
		b.runAsync()
		return b.drain(o)
	}
	hasText(t, claim(1), "You Complete!")
	hasText(t, claim(2), "You Can't Receive Gifts!")
	hasText(t, claim(40), "No Reward!")
	pk = claim(3)
	hasText(t, pk, "You Received Success Gifts!")
	if !b.daily().players[o.Index].flags[2] || find(pk, 0xFB, 0x17) == nil {
		t.Fatal("day 3 not marked")
	}
	if days, _ := cs.DailyRewardDays(context.Background(), o.P.Account.Login, 10); !slices.Equal(days, []int{1, 3}) {
		t.Fatalf("saved %v", days)
	}
	hasText(t, claim(3), "You Complete!")
	// A new day sends the window again.
	b.advance(24 * time.Hour)
	b.dailySecond()
	if find(b.drain(o), 0xFB, 0x17) == nil || b.daily().players[o.Index].mday != 4 {
		t.Fatal("new day")
	}
}

func TestPatentePacket(t *testing.T) {
	b := newBareWorld(t)
	p := packetPatente(0x5400, -1)
	if !bytes.Equal(p, []byte{0xC1, 0x0C, 0xF3, 0xE5, 0x00, 0x54, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatalf("% X", p)
	}
	o := b.player(PlayerStart, "Ranked", 0, 130, 130)
	o.P.Char.Patente = 9
	if b.patenteRank(o) != -1 {
		t.Fatal("rank without list")
	}
	b.wcx().patente = []int{11, 9}
	if b.patenteRank(o) != 2 {
		t.Fatal("rank 2")
	}
	b.player(PlayerStart+1, "Other", 0, 131, 130)
	b.drain(o)
	b.patenteAll(o)
	if n := countPackets(b.drain(o), 0xF3, 0xE5); n != 2 {
		t.Fatalf("packets %d", n)
	}
}

func TestEventWindow(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) // Friday
	starts := []gamedata.EventStart{{Day: 5, Hour: 12, Minute: 30}, {Day: 6, Hour: 1}}
	if n := nextEventSeconds(starts, now); n != 1800 {
		t.Fatalf("next %d", n)
	}
	if n := nextEventSeconds(starts[1:], now); n != 13*3600 {
		t.Fatalf("tomorrow %d", n)
	}
	if n := nextEventSeconds([]gamedata.EventStart{{Day: 5, Hour: 12}}, now); n != -1 {
		t.Fatalf("passed %d", n)
	}
	p := packetWindowTime([]windowRow{{Name: "[Blood Castle]", Open: true, Remain: 300}, {Name: "x"}})
	if len(p) != 86 || p[0] != 0xC2 || p[2] != 86 || p[3] != 0xF3 || p[4] != 0xEF || p[5] != 2 ||
		!strings.HasPrefix(string(p[6:38]), "[Blood Castle]") || p[38] != 1 ||
		binary.LittleEndian.Uint32(p[42:]) != 300 || p[78] != 0 {
		t.Fatalf("% X", p)
	}
	b := newBareWorld(t)
	o := b.player(PlayerStart, "Planner", 0, 130, 130)
	worldHandlers[0x06](b.World, o, []byte{0xC1, 0x03, 0x06})
	b.windowSecond()
	w := find(b.drain(o), 0xF3, 0xEF)
	if w == nil || w[5] == 0 {
		t.Fatal("no window")
	}
	rows := b.windowRows(o)
	if rows[0].Name != b.messageFor(o, 0x2EE) || rows[1].Name != b.messageFor(o, 0x2EF) {
		t.Fatalf("rows %+v", rows[:2])
	}
	b.handlePosition(o, []byte{0xC1, 0x05, 0xDF, byte(o.X), byte(o.Y)})
	b.windowSecond()
	if find(b.drain(o), 0xF3, 0xEF) != nil {
		t.Fatal("window still open")
	}
}
