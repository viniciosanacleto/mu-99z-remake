package game

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/auth"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/castlesiege"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/guild"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// csConfig copies the CastleSiege instance configuration with the event
// on (the shipped file has CastleSiegeEvent = 0) and the guild member
// minimum lowered so one-member test guilds can register.
func csConfig(t *testing.T) *gsconfig.Config {
	t.Helper()
	cfg := instanceConfig(t, "gameserver-cs", map[string]map[string]string{
		"Event": {"CastleSiegeEvent": "1", "CastleSiegeRegisterMinGuildMember": "1"},
	})
	if cfg.ServerType != 1 || cfg.Int("Event", "CastleSiegeEvent", 0) != 1 {
		t.Fatal("config not patched")
	}
	return cfg
}

// csWorld is a bare CastleSiege instance whose castle answered the
// DataServer init (MuCastle_DATA created today, NPC list from the file).
func csWorld(t *testing.T) *bareWorld {
	t.Helper()
	b := newBareWorld(t)
	b.cfg = csConfig(t)
	b.clock = time.Now()
	if err := b.csLoad(); err != nil {
		t.Fatal(err)
	}
	b.csRequestInit()
	b.pump()
	c := b.cs()
	if c == nil || !c.running || c.dbState != 0 {
		t.Fatalf("castle not running: %+v", c)
	}
	return b
}

// csReset moves the cycle so that it starts at start and re-syncs.
func (b *bareWorld) csResync(start time.Time) {
	c := b.cs()
	c.start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	c.end = c.start.AddDate(0, 0, c.cycleDays)
	c.siegeEnded = false
	b.csCheckSync()
	b.pump()
}

func TestCastleSiegeGamePacketLayouts(t *testing.T) {
	if p := packetCastleStateNotice(1); !bytes.Equal(p, []byte{0xC1, 0x05, 0xB2, 0x17, 1}) {
		t.Fatalf("B2 17 = % X", p)
	}
	if p := packetTaxInfo(2, 3); !bytes.Equal(p, []byte{0xC1, 0x06, 0xB2, 0x1A, 2, 3}) {
		t.Fatalf("B2 1A = % X", p)
	}
}

func TestCastleSiegeLoadAndInit(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	if !c.enabled || c.cycleDays != 7 || len(c.npcs) != 24 || c.occupied || c.owner != "" {
		t.Fatalf("castle %+v", c)
	}
	// FIRST_CREATE: the gates and statues were saved and created, each gate
	// with its lever 4 cells south.
	st := b.csStore()
	rows, _ := st.CastleNPCs(context.Background(), c.group)
	if len(rows) != 10 {
		t.Fatalf("MuCastle_NPC rows %d", len(rows))
	}
	gates, statues, levers := 0, 0, 0
	for i := 0; i < SummonStart; i++ {
		o := b.objects[i]
		if o.State != StatePlaying || o.Map != castlesiege.Map {
			continue
		}
		switch o.Class {
		case castlesiege.ClassGate:
			gates++
			if o.CS.NPCKind != 1 || o.CS.GateOpen != 1 || o.Mon.Life != 500000 || o.Mon.Defense != 100 ||
				!hasEffectIndex(o, csEffGateOpen) {
				t.Fatalf("gate %+v %+v", o.CS, o.Mon)
			}
			lv := b.objects[o.CS.Link]
			if lv.Class != castlesiege.ClassLever || lv.X != o.X || lv.Y != o.Y+4 || lv.CS.Link != o.Index {
				t.Fatalf("lever of gate %d at %d,%d", o.Index, lv.X, lv.Y)
			}
		case castlesiege.ClassStatue:
			statues++
			if o.Mon.Defense != 80 || o.Mon.Life != 400000 {
				t.Fatalf("statue %+v", o.Mon)
			}
		case castlesiege.ClassLever:
			levers++
		case castlesiege.ClassCanon, castlesiege.ClassShield:
			t.Fatal("battle NPC outside the battle")
		}
	}
	if gates != 6 || statues != 4 || levers != 6 {
		t.Fatalf("gates %d statues %d levers %d", gates, statues, levers)
	}
	// The cycle starts today: day 0 is the guild registration.
	if c.state != castlesiege.StateRegSiege || c.remain <= 0 {
		t.Fatalf("state %d remain %d", c.state, c.remain)
	}
}

func TestCastleSiegeCheckSyncStates(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	day := func(d int) time.Time {
		n := time.Now()
		return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).AddDate(0, 0, d)
	}
	cases := []struct {
		now    time.Time
		start  time.Time
		state  int
		remain time.Duration
	}{
		{day(0).Add(time.Hour), day(0), castlesiege.StateRegSiege, 46*time.Hour + 59*time.Minute},
		{day(2).Add(time.Hour), day(0), castlesiege.StateRegMark, 23 * time.Hour},
		{day(4).Add(13 * time.Hour), day(0), castlesiege.StateNotify, 6 * time.Hour},
		{day(6).Add(19*time.Hour + 50*time.Minute), day(0), castlesiege.StateReady, 15 * time.Minute}, // last 15 minutes: 900000
		{day(6).Add(21 * time.Hour), day(0), castlesiege.StateReady, 15 * time.Minute},                // during START
		{day(0).Add(time.Hour), day(2), castlesiege.StateIdle1, 47 * time.Hour},
	}
	for i, k := range cases {
		b.clock = k.now
		b.csResync(k.start)
		if c.state != k.state || time.Duration(c.remain)*time.Millisecond != k.remain {
			t.Errorf("case %d: state %d remain %v, want %d %v", i, c.state, time.Duration(c.remain)*time.Millisecond, k.state, k.remain)
		}
	}
	// SIEGE_ENDED: END until the end date.
	b.clock = day(6).Add(23 * time.Hour)
	c.start = day(0)
	c.end = day(7)
	c.siegeEnded = true
	b.csCheckSync()
	if c.state != castlesiege.StateEnd || time.Duration(c.remain)*time.Millisecond != time.Hour {
		t.Fatalf("ended state %d remain %d", c.state, c.remain)
	}
}

func TestCastleSiegeStateMachineAndCycle(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	o := b.player(PlayerStart, "Watcher", 0, 130, 130)
	n := time.Now()
	start := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
	b.clock = start.Add(time.Hour)
	b.csResync(start)
	b.csProcess()
	if p := find(b.drain(o), 0x0D, -1); p == nil || p[3] != NoticeGold || !bytes.Contains(p, []byte(b.messageFor(nil, 111))) {
		t.Fatalf("registration notice % X", p)
	}
	// 30 minutes later the notice repeats; the state ends at day 1 23:59.
	b.clock = b.clock.Add(31 * time.Minute)
	b.csProcess()
	if find(b.drain(o), 0x0D, -1) == nil {
		t.Fatal("no periodic notice")
	}
	b.clock = start.Add(47*time.Hour + 59*time.Minute + time.Second)
	b.csProcess()
	if c.state != castlesiege.StateIdle2 {
		t.Fatalf("state %d after registration", c.state)
	}
	b.clock = start.Add(48*time.Hour + time.Second)
	b.csProcess()
	if c.state != castlesiege.StateRegMark {
		t.Fatalf("state %d, want mark registration", c.state)
	}
	// END -> END_CYCLE -> the next cycle starts at this cycle's day 7.
	c.state = castlesiege.StateEnd
	b.csSetRemain(1000)
	b.clock = b.clock.Add(2 * time.Second)
	b.csProcess()
	b.pump()
	if c.state != castlesiege.StateRegSiege || !c.start.Equal(start.AddDate(0, 0, 7)) || !c.end.Equal(start.AddDate(0, 0, 14)) {
		t.Fatalf("new cycle state %d %v..%v", c.state, c.start, c.end)
	}
	d, _, _ := b.csStore().CastleTaxInfo(context.Background(), c.group)
	if !d.StartDate.Equal(c.start) || !d.EndDate.Equal(c.end) {
		t.Fatalf("schedule not saved: %v %v", d.StartDate, d.EndDate)
	}
	// The sync (published every 100 ms) is shared with the other instances.
	b.csProcess()
	if b.csSyncState() != castlesiege.StateRegSiege {
		t.Fatalf("sync state %d", b.csSyncState())
	}
}

// csGuild creates a guild whose master is o (level 400 bare player).
func csGuildOf(t *testing.T, b *bareWorld, o *Object, name string) int {
	t.Helper()
	return guildSetup(t, b, o, name)
}

func csSend(b *bareWorld, o *Object, head byte, body ...byte) {
	b.hooks.worldHandlers[head](b.World, o, frame.Build(frame.C1, head, body...))
	b.pump()
}

func TestCastleSiegeRegistration(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	m := b.player(PlayerStart, "Master", 0, 130, 130)
	csGuildOf(t, b, m, "Alpha")
	c.state = castlesiege.StateIdle2
	csSend(b, m, 0xB2, 0x01)
	if r := find(b.drain(m), 0xB2, 0x01); r == nil || r[4] != csRegNotTime || string(r[5:10]) != "Alpha" {
		t.Fatalf("B2 01 out of time = % X", r)
	}
	c.state = castlesiege.StateRegSiege
	m.P.Char.Level = 100
	csSend(b, m, 0xB2, 0x01)
	if r := find(b.drain(m), 0xB2, 0x01); r == nil || r[4] != csRegLevel {
		t.Fatalf("B2 01 low level = % X", r)
	}
	m.P.Char.Level = 400
	csSend(b, m, 0xB2, 0x01)
	if r := find(b.drain(m), 0xB2, 0x01); r == nil || r[4] != store.CastleRegOK {
		t.Fatalf("B2 01 = % X", r)
	}
	csSend(b, m, 0xB2, 0x01)
	if r := find(b.drain(m), 0xB2, 0x01); r == nil || r[4] != store.CastleRegAlready {
		t.Fatalf("B2 01 again = % X", r)
	}
	// Without a guild.
	x := b.player(PlayerStart+1, "Lone", 0, 131, 130)
	csSend(b, x, 0xB2, 0x01)
	if r := find(b.drain(x), 0xB2, 0x01); r == nil || r[4] != csRegNoGuild {
		t.Fatalf("B2 01 no guild = % X", r)
	}
	// Marks: only in state 3, one Sign of Lord +3 each.
	c.state = castlesiege.StateRegMark
	csSend(b, m, 0xB2, 0x04)
	if r := find(b.drain(m), 0xB2, 0x04); r == nil || r[4] != 3 {
		t.Fatalf("B2 04 without sign = % X", r)
	}
	b.insertInventory(m, item.Item{Index: csSignOfLord, Level: 3, Durability: 1})
	b.insertInventory(m, item.Item{Index: csSignOfLord, Level: 3, Durability: 1})
	csSend(b, m, 0xB2, 0x04)
	r := find(b.drain(m), 0xB2, 0x04)
	if r == nil || r[4] != 1 || string(r[5:10]) != "Alpha" || binary.BigEndian.Uint32(r[13:]) != 1 ||
		b.countItems(m, csSignOfLord, 3) != 1 {
		t.Fatalf("B2 04 = % X (signs %d)", r, b.countItems(m, csSignOfLord, 3))
	}
	csSend(b, m, 0xB2, 0x04)
	b.drain(m)
	csSend(b, m, 0xB2, 0x03)
	if r := find(b.drain(m), 0xB2, 0x03); r == nil || r[4] != 1 || binary.BigEndian.Uint32(r[13:]) != 2 || r[17] != 0 || r[18] != 1 {
		t.Fatalf("B2 03 = % X", r)
	}
	csSend(b, m, 0xB4)
	if r := find(b.drain(m), 0xB4, -1); r == nil || r[4] != 1 || r[8] != 1 || string(r[12:17]) != "Alpha" ||
		binary.BigEndian.Uint32(r[20:]) != 2 {
		t.Fatalf("B4 = % X", r)
	}
	// Giving up returns 3000 zen per mark.
	money := m.P.Char.Money
	csSend(b, m, 0xB2, 0x02, 1)
	if r := find(b.drain(m), 0xB2, 0x02); r == nil || r[4] != 1 || r[5] != 1 || m.P.Char.Money != money+6000 {
		t.Fatalf("B2 02 = % X money %d", r, m.P.Char.Money-money)
	}
	c.state = castlesiege.StateNotify
	csSend(b, m, 0xB2, 0x02, 1)
	if r := find(b.drain(m), 0xB2, 0x02); r == nil || r[4] != 3 {
		t.Fatalf("B2 02 out of time = % X", r)
	}
}

func TestCastleSiegeGuildListAndSides(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	st := b.csStore()
	ctx := context.Background()
	names := []string{"Alpha", "Beta", "Gamma", "Delta"}
	var masters []*Object
	for i, n := range names {
		o := b.player(PlayerStart+i, "M"+n, 0, 130+i, 130)
		csGuildOf(t, b, o, n)
		masters = append(masters, o)
		if r, _ := st.RegisterSiegeGuild(ctx, c.group, n, store.CastleGuildCheck{Exists: true, MasterLevel: 400, Members: 1}); r != 1 {
			t.Fatalf("register %s = %d", n, r)
		}
	}
	// Alpha gives up; the castle belongs to "Owners".
	st.GiveUpSiegeGuild(ctx, c.group, "Alpha", true)
	own := b.player(PlayerStart+10, "OwnerM", 0, 140, 130)
	csGuildOf(t, b, own, "Owners")
	c.occupied, c.owner = true, "Owners"
	// Gamma's alliance includes Friends.
	f := b.player(PlayerStart+11, "FriendM", 0, 141, 130)
	csGuildOf(t, b, f, "Friends")
	mgr := b.guildMgr()
	gam, _ := mgr.Find("Gamma")
	noWrite := func(store.GuildStore) error { return nil }
	for _, name := range []string{"Gamma", "Friends"} {
		if mgr.Update(ctx, name, noWrite, func(r *guild.Record) { r.Union = gam.Number }) != 1 {
			t.Fatalf("union of %s not set", name)
		}
	}

	b.csSetState(castlesiege.StateNotify, true)
	b.pump()
	if !c.listLoaded || !c.unionLoaded {
		t.Fatal("list not loaded")
	}
	// The three highest SEQ_NUM not given up: Delta (side 2), Gamma (3), Beta (4).
	want := map[string]csGuild{"Delta": {Side: 2, Involved: true}, "Gamma": {Side: 3, Involved: true},
		"Beta": {Side: 4, Involved: true}, "Owners": {Side: 1, Involved: true}, "Friends": {Side: 3}}
	if len(c.all) != len(want) {
		t.Fatalf("all %v", c.all)
	}
	for n, g := range want {
		if e := c.all[n]; e == nil || *e != g {
			t.Fatalf("%s = %+v, want %+v", n, e, g)
		}
	}
	if csSide(masters[3]) != 2 || csSide(masters[2]) != 3 || csSide(f) != 3 || csSide(own) != 1 || csSide(masters[0]) != 0 {
		t.Fatalf("sides %d %d %d %d %d", csSide(masters[3]), csSide(masters[2]), csSide(f), csSide(own), csSide(masters[0]))
	}
	saved, _ := st.SiegeGuildList(ctx, c.group)
	if len(saved) != 5 {
		t.Fatalf("saved list %+v", saved)
	}
	csSend(b, own, 0xB5)
	if r := find(b.drain(own), 0xB5, -1); r == nil || r[4] != 1 || r[8] != 5 {
		t.Fatalf("B5 = % X", r)
	}
	c.state = castlesiege.StateStart
	csSend(b, own, 0xB5)
	if r := find(b.drain(own), 0xB5, -1); r == nil || r[4] != 2 {
		t.Fatalf("B5 in battle = % X", r)
	}
}

func TestCastleSiegeNPCManagement(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	m := b.player(PlayerStart, "Lord", castlesiege.Map, 100, 120)
	csGuildOf(t, b, m, "Owners")
	c.occupied, c.owner = true, "Owners"
	m.P.Char.Money = 2000000000
	gate := c.npcEntry(castlesiege.ClassGate, 1)
	g := b.objects[gate.Obj]
	// B3: the gate list.
	csSend(b, m, 0xB3, 1)
	if r := find(b.drain(m), 0xB3, -1); r == nil || r[4] != 1 || r[8] != 6 || r[38] != 1 {
		t.Fatalf("B3 = % X", r)
	}
	// Repair: (lost life + DF*200000)*5.
	g.Mon.Life = 400000
	body := make([]byte, 9)
	packet.PutUint32(body[1:], uint32(castlesiege.ClassGate))
	packet.PutUint32(body[5:], 1)
	body[0] = 0x06
	money := m.P.Char.Money
	csSend(b, m, 0xB2, body...)
	if r := find(b.drain(m), 0xB2, 0x06); r == nil || r[4] != 1 || binary.LittleEndian.Uint32(r[16:]) != 500000 ||
		g.Mon.Life != 500000 || money-m.P.Char.Money != 100000*5 {
		t.Fatalf("B2 06 = % X life %v cost %d", r, g.Mon.Life, money-m.P.Char.Money)
	}
	// Upgrade the defense to 1: 2 Jewels of Guardian and 3,000,000 zen.
	up := make([]byte, 17)
	up[0] = 0x07
	packet.PutUint32(up[1:], uint32(castlesiege.ClassGate))
	packet.PutUint32(up[5:], 1)
	packet.PutUint32(up[9:], csUpDefense)
	packet.PutUint32(up[13:], 1)
	csSend(b, m, 0xB2, up...)
	if r := find(b.drain(m), 0xB2, 0x07); r == nil || r[4] != 4 {
		t.Fatalf("B2 07 without jewels = % X", r)
	}
	b.insertInventory(m, item.Item{Index: csJewelGuardian, Durability: 1})
	b.insertInventory(m, item.Item{Index: csJewelGuardian, Durability: 1})
	money = m.P.Char.Money
	csSend(b, m, 0xB2, up...)
	if r := find(b.drain(m), 0xB2, 0x07); r == nil || r[4] != 1 || gate.DF != 1 || g.Mon.Defense != 180 ||
		money-m.P.Char.Money != 3000000 || b.countItems(m, csJewelGuardian, 0) != 0 {
		t.Fatalf("B2 07 = % X DF %d def %d", r, gate.DF, g.Mon.Defense)
	}
	packet.PutUint32(up[13:], 3) // skipping a level
	csSend(b, m, 0xB2, up...)
	if r := find(b.drain(m), 0xB2, 0x07); r == nil || r[4] != 6 {
		t.Fatalf("B2 07 skip = % X", r)
	}
	// Destroy the gate outside the battle: the row is deleted, the block lifted.
	b.csNPCKilled(g)
	b.pump()
	if gate.Live != 0 || gate.Obj != -1 || g.CS.GateOpen != 1 {
		t.Fatalf("gate entry after death %+v", gate)
	}
	rows, _ := b.csStore().CastleNPCs(context.Background(), c.group)
	for _, r := range rows {
		if r.Number == castlesiege.ClassGate && r.Index == 1 {
			t.Fatal("destroyed gate still in MuCastle_NPC")
		}
	}
	b.deleteMonster(g)
	// Buy it again: 500000*5.
	buy := make([]byte, 9)
	buy[0] = 0x05
	packet.PutUint32(buy[1:], uint32(castlesiege.ClassGate))
	packet.PutUint32(buy[5:], 1)
	money = m.P.Char.Money
	csSend(b, m, 0xB2, buy...)
	if r := find(b.drain(m), 0xB2, 0x05); r == nil || r[4] != 1 || money-m.P.Char.Money != 2500000 || gate.Live != 2 {
		t.Fatalf("B2 05 = % X cost %d", r, money-m.P.Char.Money)
	}
	csSend(b, m, 0xB2, buy...)
	if r := find(b.drain(m), 0xB2, 0x05); r == nil || r[4] != 0 {
		t.Fatalf("B2 05 existing = % X", r)
	}
	// Somebody else is refused silently.
	x := b.player(PlayerStart+1, "Other", castlesiege.Map, 101, 120)
	csGuildOf(t, b, x, "Others")
	csSend(b, x, 0xB3, 1)
	if r := find(b.drain(x), 0xB3, -1); r == nil || r[4] != 2 {
		t.Fatalf("B3 not owner = % X", r)
	}
}

func TestCastleSiegeGatesAndLever(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	m := b.player(PlayerStart, "Lord", castlesiege.Map, 100, 120)
	csGuildOf(t, b, m, "Owners")
	c.occupied, c.owner = true, "Owners"
	gate := b.objects[c.npcEntry(castlesiege.ClassGate, 1).Obj]
	lever := b.objects[gate.CS.Link]
	b.hooks.npcTalk[castlesiege.ClassLever](b.World, m, lever)
	if r := find(b.drain(m), 0xB2, 0x11); r == nil || r[4] != 1 || int(r[5])<<8|int(r[6]) != gate.Index {
		t.Fatalf("B2 11 = % X", r)
	}
	t30 := b.terrain(castlesiege.Map)
	csSend(b, m, 0xB2, 0x12, 0, byte(gate.Index>>8), byte(gate.Index))
	if r := find(b.drain(m), 0xB2, 0x12); r == nil || r[4] != 1 || r[5] != 0 ||
		t30.Get(gate.X, gate.Y)&gamedata.AttrWall == 0 || gate.CS.GateOpen != 0 {
		t.Fatalf("B2 12 close = % X", r)
	}
	// The next C2 13 entry drops the open-gate effect (FUN_00515510).
	e := make([]byte, 16)
	monsterViewportEntry(e, gate)
	b.csMonsterEntry(m, gate, e)
	if hasEffectIndex(gate, csEffGateOpen) {
		t.Fatal("effect 13 kept on a closed gate")
	}
	csSend(b, m, 0xB2, 0x12, 1, byte(gate.Index>>8), byte(gate.Index))
	if t30.Get(gate.X, gate.Y)&gamedata.AttrWall != 0 || gate.CS.GateOpen != 1 {
		t.Fatal("gate not opened")
	}
	// A stranger may not use the lever.
	x := b.player(PlayerStart+1, "Other", castlesiege.Map, 101, 120)
	b.hooks.npcTalk[castlesiege.ClassLever](b.World, x, lever)
	if r := find(b.drain(x), 0xB2, 0x11); r == nil || r[4] != 4 {
		t.Fatalf("B2 11 stranger = % X", r)
	}
	// Gate 97 (the crown room) outside the battle: owner only.
	if !b.csGate97(m) || b.csGate97(x) {
		t.Fatal("gate 97 rule")
	}
}

// csBattle sets up a battle: owner "Owners" defends, "Attack" attacks.
func csBattle(t *testing.T, b *bareWorld) (def, att *Object) {
	t.Helper()
	c := b.cs()
	def = b.player(PlayerStart, "Defender", castlesiege.Map, 94, 200)
	att = b.player(PlayerStart+1, "Attacker", castlesiege.Map, 90, 30)
	csGuildOf(t, b, def, "Owners")
	csGuildOf(t, b, att, "Attack")
	c.occupied, c.owner = true, "Owners"
	c.basic = map[string]*csGuild{"Owners": {Side: 1, Involved: true}, "Attack": {Side: 2, Involved: true}}
	c.all = map[string]*csGuild{"Owners": {Side: 1, Involved: true}, "Attack": {Side: 2, Involved: true}}
	c.listLoaded, c.unionLoaded = true, true
	b.csSetState(castlesiege.StateStart, true)
	b.pump()
	b.csPublishSync()
	return def, att
}

func TestCastleSiegeBattle(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	def, att := csBattle(t, b)
	if c.state != castlesiege.StateStart || csSide(def) != 1 || csSide(att) != 2 {
		t.Fatalf("state %d sides %d %d", c.state, csSide(def), csSide(att))
	}
	if !hasEffectIndex(def, 14) || !hasEffectIndex(att, 15) {
		t.Fatal("side marks")
	}
	if b.csSyncState() != castlesiege.StateStart || b.hooks.castleBattle == nil || !b.hooks.castleBattle(b.World) {
		t.Fatal("battle not visible to the other modules")
	}
	// Battle NPCs and closed gates.
	var shield, canon int
	for i := 0; i < SummonStart; i++ {
		o := b.objects[i]
		if o.State == StatePlaying && o.Map == castlesiege.Map {
			switch o.Class {
			case castlesiege.ClassShield:
				shield++
			case castlesiege.ClassCanon:
				canon++
			}
		}
	}
	gate := b.objects[c.npcEntry(castlesiege.ClassGate, 1).Obj]
	if shield != 1 || canon != 9 || gate.CS.GateOpen != 0 || b.terrain(castlesiege.Map).Get(gate.X, gate.Y)&gamedata.AttrWall == 0 {
		t.Fatalf("shield %d canon %d gate open %d", shield, canon, gate.CS.GateOpen)
	}
	// Attack rules: only attackers hit gates; PvP between sides at 40%.
	if b.csAttackAllowed(def, gate) || !b.csAttackAllowed(att, gate) {
		t.Fatal("gate attack rule")
	}
	att.X, att.Y = 95, 200
	if ok, decided := b.csPvP(att, def); !ok || !decided {
		t.Fatal("PvP between sides")
	}
	if r, ok := b.csDamageRate(att, def); !ok || r != 40 {
		t.Fatalf("damage rate %d", r)
	}
	if !b.csPKExempt(att, def) {
		t.Fatal("PK during the battle")
	}
	// Gate 97 during the battle: defenders yes, attackers only after the statues.
	if !b.csGate97(def) || b.csGate97(att) {
		t.Fatal("gate 97 during the battle")
	}
	// The switches need the statues down (message 121).
	sw1, sw2, crown := b.monsterByClass(castlesiege.Map, castlesiege.ClassSwitch1), b.monsterByClass(castlesiege.Map, castlesiege.ClassSwitch2), b.monsterByClass(castlesiege.Map, castlesiege.ClassCrown)
	if sw1 == nil || sw2 == nil || crown == nil {
		t.Fatal("crown NPCs not spawned")
	}
	place := func(o *Object, x, y int) { o.X, o.Y, o.TX, o.TY = x, y, x, y }
	place(att, sw1.X, sw1.Y+1)
	b.hooks.npcTalk[castlesiege.ClassSwitch1](b.World, att, sw1)
	if p := find(b.drain(att), 0x0D, -1); p == nil || c.switches[1] != -1 {
		t.Fatalf("switch with statues up % X", p)
	}
	for _, n := range c.npcs {
		if n.Class == castlesiege.ClassStatue {
			if o := b.csNPCObj(n); o != nil {
				o.Alive = false // the death path runs FUN_0041e180 on the dead object
				b.csNPCKilled(o)
				b.deleteMonster(o)
			}
		}
	}
	if !c.crownRoom || b.monsterByClass(castlesiege.Map, castlesiege.ClassShield) != nil || !b.csGate97(att) {
		t.Fatal("crown room not open")
	}
	// Two attackers hold the switches; the crown opens.
	member := func(o *Object) {
		g := *guildOf(att)
		g.Status = 0
		o.P.Guild = &g
		csOf(o).Side = 2
	}
	a2 := b.player(PlayerStart+2, "Helper", castlesiege.Map, sw2.X, sw2.Y+1)
	member(a2)
	b.hooks.npcTalk[castlesiege.ClassSwitch1](b.World, att, sw1)
	b.hooks.npcTalk[castlesiege.ClassSwitch2](b.World, a2, sw2)
	if r := find(b.drain(att), 0xB2, 0x14); r == nil || r[8] != 1 {
		t.Fatalf("B2 14 = % X", r)
	}
	b.csObjectProc()
	if !c.crownOpen || !hasEffectIndex(crown, csEffCrown) {
		t.Fatal("crown not open")
	}
	// The attacking master registers at the crown and holds it 30 s.
	place(att, crown.X+1, crown.Y)
	csOf(att).Crown = 0
	// The switch holder moved: put him back so the switch stays held.
	place(att, sw1.X, sw1.Y+1)
	b.csObjectProc()
	place(att, crown.X+1, crown.Y)
	// Use a third attacker on switch 1 so the master can stand at the crown.
	a3 := b.player(PlayerStart+3, "Holder", castlesiege.Map, sw1.X, sw1.Y+1)
	member(a3)
	c.switches[1] = a3.Index
	b.hooks.npcTalk[castlesiege.ClassCrown](b.World, att, crown)
	if r := find(b.drain(att), 0xB2, 0x15); r == nil || r[4] != castlesiege.CrownBegin || c.crownUser != att.Index {
		t.Fatalf("B2 15 = % X", r)
	}
	b.clock = b.clock.Add(31 * time.Second)
	b.csCrownCheck()
	if c.occupier != "Attack" || csSide(att) != 1 || csSide(def) != 2 || c.crownOpen {
		t.Fatalf("occupation: occupier %q sides %d %d", c.occupier, csSide(att), csSide(def))
	}
	// The old defenders were moved out of the castle area.
	if def.Y > 48 {
		t.Fatalf("defender not moved out: %d,%d", def.X, def.Y)
	}
	// End of the battle: Attack owns the castle, the taxes are reset.
	c.taxChaos, c.money = 3, 1000
	b.csSetRemain(0)
	c.remain = 0
	b.clock = b.clock.Add(2 * time.Second)
	b.csProcess()
	b.pump()
	if c.state != castlesiege.StateEnd || c.owner != "Attack" || !c.occupied || c.taxChaos != 0 {
		t.Fatalf("after battle state %d owner %q tax %d", c.state, c.owner, c.taxChaos)
	}
	d, _, _ := b.csStore().CastleTaxInfo(context.Background(), c.group)
	if !d.Occupied || d.Owner != "Attack" || !d.SiegeEnded {
		t.Fatalf("stored castle %+v", d)
	}
	if csSide(att) != 0 || hasEffectIndex(att, 14) {
		t.Fatal("sides not cleared at END")
	}
}

func TestCastleSiegeNoAttackers(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	o := b.player(PlayerStart, "Watcher", 0, 130, 130)
	c.occupied, c.owner = true, "Owners"
	b.csSetState(castlesiege.StateStart, true)
	if c.state != castlesiege.StateEnd || c.owner != "Owners" || !c.siegeEnded {
		t.Fatalf("state %d owner %q", c.state, c.owner)
	}
	if p := find(b.drain(o), 0x0D, -1); p == nil || !bytes.Contains(p, []byte("Owners")) {
		t.Fatalf("defense notice % X", p)
	}
}

func TestCastleSiegeTaxes(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	m := b.player(PlayerStart, "Lord", 0, 130, 130)
	csGuildOf(t, b, m, "Owners")
	x := b.player(PlayerStart+1, "Buyer", 0, 131, 130)
	c.occupied, c.owner = true, "Owners"
	// B2 09: chaos 3, store 2, hunt 1000; above the limits refused.
	rate := func(kind byte, v uint32) []byte {
		body := []byte{0x09, kind, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(body[2:], v)
		csSend(b, m, 0xB2, body...)
		return find(b.drain(m), 0xB2, 0x09)
	}
	if r := rate(1, 4); r == nil || r[4] != 0 {
		t.Fatalf("chaos 4 = % X", r)
	}
	if r := rate(1, 3); r == nil || r[4] != 1 || r[5] != 1 || binary.BigEndian.Uint32(r[6:]) != 3 || c.taxChaos != 3 {
		t.Fatalf("chaos 3 = % X", r)
	}
	rate(2, 2)
	rate(3, 1000)
	b.csPublishSync()
	// Everybody but the owner guild pays.
	if b.castleChaosTax(x) != 3 || b.castleChaosTax(m) != 0 || b.castleStoreTax(x, 1000) != 20 {
		t.Fatalf("taxes %d %d %d", b.castleChaosTax(x), b.castleChaosTax(m), b.castleStoreTax(x, 1000))
	}
	// Collected tax goes to the castle money once a minute.
	b.castleAddTax(500)
	b.csFlushTax()
	b.pump()
	if c.money != 500 {
		t.Fatalf("castle money %d", c.money)
	}
	csSend(b, m, 0xB2, 0x08)
	if r := find(b.drain(m), 0xB2, 0x08); r == nil || r[4] != 1 || r[5] != 3 || r[6] != 2 || binary.BigEndian.Uint64(r[7:]) != 500 {
		t.Fatalf("B2 08 = % X", r)
	}
	// B2 10: take 200 out.
	money := m.P.Char.Money
	body := []byte{0x10, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(body[1:], 200)
	csSend(b, m, 0xB2, body...)
	if r := find(b.drain(m), 0xB2, 0x10); r == nil || r[4] != 1 || binary.BigEndian.Uint64(r[5:]) != 300 ||
		m.P.Char.Money != money+200 || c.money != 300 {
		t.Fatalf("B2 10 = % X", r)
	}
	binary.LittleEndian.PutUint32(body[1:], 301)
	csSend(b, m, 0xB2, body...)
	if r := find(b.drain(m), 0xB2, 0x10); r == nil || r[4] != 0 {
		t.Fatalf("B2 10 too much = % X", r)
	}
	// Land of Trial: closed for strangers, then open for 1000 zen.
	csSend(b, x, 0xB9, 0x05)
	if r := find(b.drain(x), 0xB9, 0x05); r == nil || r[4] != 0 {
		t.Fatalf("B9 05 closed = % X", r)
	}
	csSend(b, m, 0xB2, 0x1F, 1)
	if r := find(b.drain(m), 0xB2, 0x1F); r == nil || r[4] != 1 || !c.huntOpen {
		t.Fatalf("B2 1F = % X", r)
	}
	money = x.P.Char.Money
	csSend(b, x, 0xB9, 0x05)
	if r := find(b.drain(x), 0xB9, 0x05); r == nil || r[4] != 1 || money-x.P.Char.Money != 1000 || x.Map != castlesiege.TrialMap {
		t.Fatalf("B9 05 = % X paid %d map %d", r, money-x.P.Char.Money, x.Map)
	}
	// Gatekeeper information.
	b.csKeeperTalk(m)
	if r := find(b.drain(m), 0xB9, 0x03); r == nil || r[4] != 3 || r[5] != 1 || binary.LittleEndian.Uint32(r[8:]) != 1000 {
		t.Fatalf("B9 03 = % X", r)
	}
	// Senior: owner masters only.
	b.hooks.npcTalk[castlesiege.ClassSenior](b.World, x, x)
	if p := find(b.drain(x), 0x0D, -1); p == nil {
		t.Fatal("senior refused without notice")
	}
	b.hooks.npcTalk[castlesiege.ClassSenior](b.World, m, m)
	pk := b.drain(m)
	if w := find(pk, 0x30, -1); w == nil || w[3] != 0x0C || find(pk, 0xB2, 0x1A) == nil {
		t.Fatalf("senior window % X", w)
	}
	// B9 02: the owner's mark.
	csSend(b, x, 0xB9, 0x02)
	if r := find(b.drain(x), 0xB9, 0x02); r == nil || r[4] != 0x44 {
		t.Fatalf("B9 02 = % X", r)
	}
}

func TestCastleSiegeStateRequest(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	o := b.player(PlayerStart, "Asker", 0, 130, 130)
	csSend(b, o, 0xB2, 0x00)
	r := find(b.drain(o), 0xB2, 0x00)
	if r == nil || len(r) != 0x2E || r[4] != 2 || r[5] != byte(c.state) {
		t.Fatalf("B2 00 = % X", r)
	}
	m := b.player(PlayerStart+1, "Lord", 0, 131, 130)
	csGuildOf(t, b, m, "Owners")
	c.occupied, c.owner = true, "Owners"
	csSend(b, o, 0xB2, 0x00)
	if r := find(b.drain(o), 0xB2, 0x00); r == nil || r[4] != 1 || string(r[24:30]) != "Owners" || string(r[32:36]) != "Lord" {
		t.Fatalf("B2 00 owner = % X", r)
	}
}

func TestCastleSiegeWeaponsAndStones(t *testing.T) {
	b := csWorld(t)
	c := b.cs()
	_, att := csBattle(t, b)
	sling := b.monsterByClass(castlesiege.Map, castlesiege.ClassSlingA)
	if sling == nil {
		t.Fatal("no attack slingshot")
	}
	att.X, att.Y = sling.X, sling.Y+1
	b.hooks.npcTalk[castlesiege.ClassSlingA](b.World, att, sling)
	if r := find(b.drain(att), 0xB7, 0x00); r == nil || r[4] != 1 || r[5] != 1 {
		t.Fatalf("B7 00 = % X", r)
	}
	csSend(b, att, 0xB7, 0x01, byte(sling.Index>>8), byte(sling.Index), 1)
	r := find(b.drain(att), 0xB7, 0x01)
	if r == nil || r[4] != 1 || r[7] != 1 || r[8] < 62 || r[8] >= 72 || r[9] < 103 || r[9] >= 112 {
		t.Fatalf("B7 01 = % X", r)
	}
	// A player at the landing cell loses CastleSiegeOffensiveWeaponDamage.
	v := b.player(PlayerStart+5, "Victim", castlesiege.Map, int(r[8]), int(r[9]))
	v.P.Char.Life = 1000
	csSend(b, att, 0xB7, 0x04, byte(sling.Index>>8), byte(sling.Index))
	b.clock = b.clock.Add(200 * time.Millisecond)
	b.csWeaponProc()
	if v.P.Char.Life != 700 {
		t.Fatalf("victim life %v", v.P.Char.Life)
	}
	// Life stone: attacking guild masters only, one per guild.
	att.X, att.Y = 60, 30
	stone := item.Item{Index: item.Make(13, 11), Level: 1, Durability: 1}
	if handled, ok := b.csDropItem(att, stone, att.X, att.Y); !handled || !ok {
		t.Fatal("life stone not summoned")
	}
	if handled, ok := b.csDropItem(att, stone, att.X, att.Y); !handled || ok {
		t.Fatal("second life stone")
	}
	s := b.csGuildStone(att)
	if s == nil || csSide(s) != 2 {
		t.Fatal("stone missing")
	}
	for i := 0; i < 60; i++ {
		b.csStoneProc(s)
	}
	if s.CS.StoneLvl != 5 {
		t.Fatalf("stone level %d", s.CS.StoneLvl)
	}
	// Respawn next to the grown stone.
	if _, mp, x, y, ok := b.csRespawn(att); !ok || mp != castlesiege.Map || x != s.X || y != s.Y {
		t.Fatal("respawn at the stone")
	}
	// Mercenaries are for defenders.
	scroll := item.Item{Index: item.Make(13, 7), Level: 0, Durability: 1}
	if _, ok := b.csDropItem(att, scroll, att.X, att.Y); ok {
		t.Fatal("attacker summoned a mercenary")
	}
	_ = c
}

// TestCastleSiegeEndToEnd speaks the original client protocol to the
// CastleSiege instance (ServerType 1, code 19): the state request B2 00
// and a registration attempt B2 01 without a guild.
func TestCastleSiegeEndToEnd(t *testing.T) {
	env := newTestEnvWith(t, csConfig(t))
	var running bool
	for i := 0; i < 50 && !running; i++ {
		env.onWorld(func() { running = env.world.cs() != nil && env.world.cs().running })
		if !running {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !running {
		t.Fatal("castle siege did not start")
	}
	c := enterAt(t, env, "tester", "Hero", 0x10, castlesiege.Map, 90, 30, 200)
	c.send(frame.Build(frame.C1, 0xB2, 0x00))
	s := c.next(0xB2, 0x00)
	if len(s) != 0x2E || s[4] != 2 || s[5] != castlesiege.StateRegSiege {
		t.Fatalf("B2 00 = % X", s)
	}
	c.send(frame.Build(frame.C1, 0xB2, 0x01))
	if r := c.next(0xB2, 0x01); r[4] != csRegNoGuild {
		t.Fatalf("B2 01 = % X", r)
	}
	c.send(frame.Build(frame.C1, 0xB5))
	if r := c.next(0xB5, -1); r[4] != 2 {
		t.Fatalf("B5 = % X", r)
	}
}

// newTestEnvWith is newTestEnv (client_test.go) for another instance
// configuration.
func newTestEnvWith(t *testing.T, cfg *gsconfig.Config) *testEnv {
	t.Helper()
	mem, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	h, _ := auth.HashPassword("pass")
	if _, err := mem.CreateAccount(context.Background(), "tester", h, "1234567890123"); err != nil {
		t.Fatal(err)
	}
	a := auth.New(mem, auth.Config{})
	w, err := New(Options{DataDir: repoData("Data"), ConfigDir: repoConfig(), Config: cfg, Store: mem, Auth: a,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Serve(ctx, ln) }()
	env := &testEnv{world: w, store: mem, auth: a, addr: ln.Addr().String()}
	env.stop = func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	}
	t.Cleanup(env.stop)
	return env
}

// TestCastleSiegeOverrideKeysReset checks the WritePrivateProfileStringA
// block of FUN_0041ab60: the one-shot cycle / state overrides are read
// and then written back as 0 in [Event] of the instance file, keeping
// the rest of the file.
func TestCastleSiegeOverrideKeysReset(t *testing.T) {
	b := newBareWorld(t)
	keys := []string{"CastleSiegeCycleStartYear", "CastleSiegeCycleStartMonth", "CastleSiegeCycleStartDay",
		"CastleSiegeSpecificState", "CastleSiegeStateStartYear", "CastleSiegeStateStartMonth",
		"CastleSiegeStateStartDay", "CastleSiegeStateStartHour", "CastleSiegeStateStartMinute"}
	set := map[string]string{"CastleSiegeEvent": "1"}
	for i, k := range keys {
		set[k] = strconv.Itoa(i + 1)
	}
	b.cfg = instanceConfig(t, "gameserver-cs", map[string]map[string]string{"Event": set})
	before, err := os.ReadFile(b.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	b.clock = time.Now()
	if err := b.csLoad(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(b.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	re, err := gsconfig.Load(b.cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if re.Int("Event", k, -1) != 0 || b.cfg.Int("Event", k, -1) != 0 {
			t.Errorf("%s not reset", k)
		}
	}
	if re.Int("Event", "CastleSiegeEvent", 0) != 1 || re.ServerName != "Castle Siege" {
		t.Fatal("other keys changed")
	}
	if strings.Count(string(after), "\n# ") != strings.Count(string(before), "\n# ") {
		t.Fatal("comments lost")
	}
}
