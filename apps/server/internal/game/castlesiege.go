package game

// Castle Siege (CCastleSiege, 0x41a640 .. 0x429cc0 in GameServer.exe; the
// same code runs in CastleSiege.exe, the 5.GameServerCS instance with
// ServerType = 1): the schedule and state machine, the DataServer
// protocol 0x80..0x89 (here direct store calls through w.async), the
// registration, the castle NPCs, taxes and the battle on map 30.
// docs/server-plan/10d-castle-siege.md.
//
// Files: castlesiege.go (state machine, schedule, data load),
// castlesiege_npc.go (gates, statues, levers, upgrades),
// castlesiege_reg.go (registration, siege guild list, sides),
// castlesiege_battle.go (START/END, crown, switches, PvP rules, weapons,
// life stone, mercenaries), castlesiege_tax.go (Senior NPC, taxes,
// Land of Trial, CCastleSiegeSync), castlesiege_packets.go (builders).

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// States of CCastleSiege+0x1c (SetState FUN_0041c690, process
// FUN_0041b780).
const (
	csIdle1    = 0 // before the cycle start date
	csRegSiege = 1 // "SIGN1": guild masters register (B2 01)
	csIdle2    = 2
	csRegMark  = 3 // "SIGN2": Sign of Lord marks (B2 04)
	csIdle3    = 4
	csNotify   = 5 // siege guilds chosen and announced
	csReady    = 6
	csStart    = 7 // the battle
	csEnd      = 8
	csEndCycle = 9
)

// Classes of the castle objects (MonsterList.txt).
const (
	csMap          = 30
	csTrialMap     = 31
	csClassShield  = 0xD7  // 215, removed when the crown room opens (FUN_0041f970)
	csClassCrown   = 0xD8  // 216
	csClassSwitch2 = 0xD9  // 217 (switch 1 of CCastleSiege+0x88)
	csClassSwitch1 = 0xDA  // 218 (switch 2 of CCastleSiege+0x8c)
	csClassLever   = 0xDB  // 219, the gate lever
	csClassKeeper  = 0xDC  // 220, Land of Trial gatekeeper
	csClassSlingA  = 0xDD  // 221, attack slingshot
	csClassSlingD  = 0xDE  // 222, defense slingshot
	csClassSenior  = 0xDF  // 223
	csClassGuard   = 0xE0  // 224, Guardsman
	csClassGate    = 0x115 // 277
	csClassStone   = 0x116 // 278, life stone
	csClassStatue  = 0x11B // 283, guardian statue
	csClassArcher  = 0x11E // 286, mercenary
	csClassSpear   = 0x11F // 287, mercenary
	csClassCanon   = 0x120 // 288, canon tower
)

// csObjState is the castle part of the object (obj+0x974 .. obj+0x99c).
type csObjState struct {
	NPCKind  byte // obj+0x974: 1 DB NPC, 2 other castle NPC/lever, 3 attack slingshot/life stone
	GateOpen byte // obj+0x975
	Link     int  // obj+0x978: gate <-> lever
	DFLevel  byte // obj+0x97c
	RGLevel  byte // obj+0x97d
	Side     byte // obj+0x97e: 1 defenders, 2.. attackers
	Involved bool // obj+0x980: the guild itself registered (not an alliance member)
	Upgraded bool // obj+0x985
	Stones   byte // obj+0x994: life stones summoned by this master
	StoneLvl byte // obj+0x995: life stone growth 0..5
	StoneSec int  // obj+0x998: seconds since the life stone was summoned
	Crown    int  // obj+0x99c: accumulated crown time (ms)
	MarkReq  bool // obj+0x6b8: a B2 04 waits for the DataServer
}

func csOf(o *Object) *csObjState {
	if o.CS == nil {
		o.CS = &csObjState{Link: -1}
	}
	return o.CS
}

func csSide(o *Object) byte {
	if o == nil || o.CS == nil {
		return 0
	}
	return o.CS.Side
}

// csNPC is one 0x54-byte entry of the NPC list (CCastleSiege+0xdc):
// [0] class [1] index [2] object [3] lever [4] create state (0 none,
// 1 create, 2 created) [5] loaded [6] DBSave [7..13] current DF, RG,
// max life, life, X, Y, dir [14..20] the CastleSiege.dat defaults.
type csNPC struct {
	Class, Index int
	Obj, Lever   int
	Live         int
	InUse        bool
	DBSave       bool
	Side         int

	DF, RG, MaxLife, Life, X, Y, Dir int
	BaseDF, BaseRG, BaseMaxLife      int
	BaseLife, BaseX, BaseY, BaseDir  int
}

// csGuild is a value of the siege guild maps (+0x9c / +0xac).
type csGuild struct {
	Side     int
	Involved bool
	Score    int
}

// castleSiege is CCastleSiege (0x174+ bytes at DAT_010d8d78).
type castleSiege struct {
	data  *gamedata.CastleSiege
	group int // MAP_SVR_GROUP

	enabled   bool // +0x08 CastleSiegeEvent
	fileOK    bool // +0x0c
	dbOK      bool // +0x0d
	running   bool // +0x0e
	dbState   int  // +0x10: 1 idle, 2 waiting for DataServer 0x81, 3 answered, 0 running
	cycleDays int  // +0x18

	state      int       // +0x1c
	remain     int64     // +0x20 (ms)
	target     time.Time // +0x24
	remainTick time.Time // +0x28

	occupied bool   // +0x2c
	owner    string // +0x2d
	occupier string // +0x36: guild holding the crown during the battle

	money                       int64 // +0x40
	taxChaos, taxStore, taxHunt int   // +0x48 / +0x4c / +0x50
	huntOpen                    bool  // +0x54

	saveTick, syncTick, noticeTick, checkTick time.Time // +0x58 .. +0x64
	sideTick, mapTick, leftTick, readyTick    time.Time // +0x68 .. +0x74
	hourTick                                  time.Time // DAT_012395e8

	crownRoom   bool      // +0x78 statues destroyed, shield removed
	crownOpen   bool      // +0x79 both switches held by one side
	crownUser   int       // +0x80
	crownX      int       // +0x84
	crownY      int       // +0x85
	switches    [2]int    // +0x88 (class 0xD9) / +0x8c (class 0xDA)
	crownStart  time.Time // +0x90
	npcsCreated bool      // +0x94

	listLoaded  bool // +0x95 siege guilds chosen
	unionLoaded bool // +0x96 alliances added
	listSet     bool // +0x97 SIEGE_GUILDLIST_SETTED
	siegeEnded  bool // +0x98 SIEGE_ENDED

	basic map[string]*csGuild // +0x9c: the registered guilds (and owner)
	all   map[string]*csGuild // +0xac: with their alliances
	order []string            // insertion order of all (std::map order is by name; see allNames)

	minimap []int    // +0xcc guild masters receiving the minimap
	npcs    []*csNPC // +0xdc

	cycleOverride bool      // +0xfc
	specState     int       // +0x100
	specValid     bool      // +0x104
	cycleEnd      time.Time // +0x108
	cycleStart    time.Time // +0x118 (override)
	specStart     time.Time // +0x128
	stateStart    time.Time // +0x138
	leftSiege     time.Time // +0x148
	start, end    time.Time // +0x158 / +0x168

	mercenaries int       // CMercenary+4
	stones      []csStone // CGuild+0x80c of each guild with a life stone
	shots       []csShot  // CCastleSiegeWeapon+0x644: shots waiting for B7 04 (10 s)
	hits        []csHit   // CCastleSiegeWeapon+0xc: hits landing 100 ms later
}

func (w *World) cs() *castleSiege { return w.castle }

// csEnabled is the "DAT_010bc714 == 1" test of every castle handler
// (ServerType 1 = the CastleSiege server).
func (w *World) csEnabled() bool { return w.castle != nil && w.cfg.ServerType == 1 }

// csLoad is CCastleSiege::LoadData (FUN_0041ab60).
func (w *World) csLoad() error {
	d, err := gamedata.LoadCastleSiege(w.configDir)
	if err != nil {
		return err
	}
	c := &castleSiege{data: d, group: w.cfg.ServerCode / 20, cycleDays: d.CycleDays, dbState: 1,
		crownUser: -1, switches: [2]int{-1, -1}, basic: map[string]*csGuild{}, all: map[string]*csGuild{}}
	for _, n := range d.NPCs {
		c.npcs = append(c.npcs, &csNPC{Class: n.Class, Index: n.Index, Obj: -1, Lever: -1, DBSave: n.DBSave, Side: n.Side,
			DF: n.DFLevel, RG: n.RGLevel, MaxLife: n.MaxLife, Life: n.MaxLife, X: n.X, Y: n.Y, Dir: n.Dir,
			BaseDF: n.DFLevel, BaseRG: n.RGLevel, BaseMaxLife: n.MaxLife, BaseLife: n.MaxLife,
			BaseX: n.X, BaseY: n.Y, BaseDir: n.Dir})
	}
	c.fileOK = true
	now := w.now()
	c.enabled = w.cfg.Int("Event", "CastleSiegeEvent", 0) != 0
	ev := func(k string) int { return w.cfg.Int("Event", k, 0) }
	cy, cm, cd := ev("CastleSiegeCycleStartYear"), ev("CastleSiegeCycleStartMonth"), ev("CastleSiegeCycleStartDay")
	c.specState = ev("CastleSiegeSpecificState")
	sy, sm, sd := ev("CastleSiegeStateStartYear"), ev("CastleSiegeStateStartMonth"), ev("CastleSiegeStateStartDay")
	sh, smi := ev("CastleSiegeStateStartHour"), ev("CastleSiegeStateStartMinute")
	if cy != 0 && cm != 0 && cd != 0 {
		c.cycleStart = time.Date(cy, time.Month(cm), cd, 0, 0, 0, 0, now.Location())
		// The override is used while today is before start + CycleDays.
		c.cycleOverride = dateKey(now) < dateKey(c.cycleStart.AddDate(0, 0, c.cycleDays)) && cm < 13 && cd >= 1 && cd <= 31
	}
	if sy != 0 && sm != 0 && sd != 0 && c.specState >= 1 && c.specState <= 9 {
		c.specStart = time.Date(sy, time.Month(sm), sd, sh, smi, 0, 0, now.Location())
		c.specValid = now.Before(c.specStart) && c.specState != 8 && sm <= 12 && sd >= 1 && sd <= 31 && sh <= 23 && smi <= 59
	}
	// The keys are written back as "0" (one-shot overrides); only when set,
	// so an untouched file stays byte-identical.
	if cy|cm|cd|c.specState|sy|sm|sd|sh|smi != 0 {
		w.csResetEventKeys()
	}
	c.dbState = 2
	w.castle = c
	return nil
}

func dateKey(t time.Time) int { y, m, d := t.Date(); return y*10000 + int(m)*100 + d }

// csResetEventKeys is the WritePrivateProfileStringA("...", "0") block
// of FUN_0041ab60, applied to the [Event] table of the instance file.
func (w *World) csResetEventKeys() {
	for _, key := range []string{"CastleSiegeCycleStartYear", "CastleSiegeCycleStartMonth", "CastleSiegeCycleStartDay",
		"CastleSiegeSpecificState", "CastleSiegeStateStartYear", "CastleSiegeStateStartMonth",
		"CastleSiegeStateStartDay", "CastleSiegeStateStartHour", "CastleSiegeStateStartMinute"} {
		if err := w.cfg.SetInt("Event", key, 0); err != nil {
			w.log.Warn("castle siege: reset event keys", "err", err)
			return
		}
	}
}

func (w *World) csStore() store.CastleStore {
	s, _ := w.store.(store.CastleStore)
	return s
}

// csRequestInit is DataServer 0x81 (sent by FUN_00425c90 while +0x10 is
// 2): WZ_CS_GetCastleTotalInfo + WZ_CS_GetCastleNpcInfo; the answer is
// FUN_00424880.
func (w *World) csRequestInit() {
	c, st := w.cs(), w.csStore()
	if c == nil || st == nil {
		return
	}
	group, days := c.group, c.cycleDays
	w.async(func(ctx context.Context) func() {
		d, first, err := st.CastleTotalInfo(ctx, group, days)
		if err != nil {
			w.log.Error("castle siege: total info", "err", err)
			return nil
		}
		npcs, err := st.CastleNPCs(ctx, group)
		if err != nil {
			w.log.Error("castle siege: npc info", "err", err)
			return nil
		}
		return func() { w.csInitData(d, first, npcs) }
	})
}

// csInitData is FUN_00424880 (DataServer 0x81 answer).
func (w *World) csInitData(d store.CastleData, first bool, npcs []store.CastleNPCData) {
	c := w.cs()
	if c == nil || c.dbState != 2 {
		return
	}
	c.dbState = 3
	c.start, c.end = d.StartDate, d.EndDate
	c.listSet, c.siegeEnded = d.GuildListSet, d.SiegeEnded
	c.occupied, c.owner = d.Occupied, d.Owner
	c.money = d.Money
	c.taxChaos, c.taxStore, c.taxHunt = d.TaxChaos, d.TaxStore, d.TaxHunt
	// +0x108 = start + the day/hour/minute of the END_CYCLE row.
	c.cycleEnd = c.start
	if s, ok := c.data.ScheduleOf(csEndCycle); ok {
		c.cycleEnd = c.start.Add(time.Duration(s.Day)*24*time.Hour + time.Duration(s.Hour)*time.Hour +
			time.Duration(s.Minute)*time.Minute)
	}
	if dateKey(c.end) < dateKey(c.start) {
		w.log.Warn("[Castle Siege] DGCastleSiegeInitDataRecv - StartDateTime > EndDateTime")
		return
	}
	for _, r := range npcs {
		found := false
		for _, n := range c.npcs {
			if n.Class == r.Number && n.Index == r.Index {
				n.Live, n.InUse = 1, true
				n.DF, n.RG, n.MaxLife, n.Life = r.DFLevel, r.RGLevel, r.MaxHP, r.HP
				n.X, n.Y, n.Dir = r.X&0xFF, r.Y&0xFF, r.Dir&0xFF
				found = true
				break
			}
		}
		if !found {
			c.npcs = append(c.npcs, &csNPC{Class: r.Number, Index: r.Index, Obj: -1, Lever: -1, Live: 1, InUse: true,
				DF: r.DFLevel, RG: r.RGLevel, MaxLife: r.MaxHP, Life: r.HP, X: r.X & 0xFF, Y: r.Y & 0xFF, Dir: r.Dir & 0xFF,
				BaseDF: r.DFLevel, BaseRG: r.RGLevel, BaseMaxLife: r.MaxHP, BaseLife: r.HP,
				BaseX: r.X & 0xFF, BaseY: r.Y & 0xFF, BaseDir: r.Dir & 0xFF})
		}
	}
	c.dbOK = true
	c.dbState = 4
	if first {
		w.csSaveDefaultNPCs() // FUN_00426070 (DataServer 0x84)
	}
	if c.fileOK && c.dbOK {
		c.dbState = 0
		w.csCheckLevers() // FUN_0041dd70
		if w.csCheckSync() {
			c.running = true
			now := w.now()
			c.saveTick, c.syncTick, c.noticeTick, c.checkTick = now, time.Time{}, time.Time{}, time.Time{}
			c.readyTick, c.leftTick, c.sideTick, c.mapTick = time.Time{}, time.Time{}, time.Time{}, time.Time{}
		}
	}
}

// --- schedule ---------------------------------------------------------------

// csDuration returns the schedule duration of a state in ms, or -1
// (FUN_0041c8b0 & co: "NextRemainTime < 0" stops the castle).
func (c *castleSiege) csDuration(state int) int64 {
	if s, ok := c.data.ScheduleOf(state); ok && s.Duration >= 0 {
		return int64(s.Duration) * 1000
	}
	return -1
}

func (w *World) csSetRemain(ms int64) {
	c := w.cs()
	c.remain = ms
	c.target = w.now().Add(time.Duration(ms/1000) * time.Second)
}

// csSetRemainFromSchedule is the common body of the SetState_* helpers:
// remain = schedule duration of the state, or the castle stops.
func (w *World) csSetRemainFromSchedule(state int) bool {
	c := w.cs()
	d := c.csDuration(state)
	if d < 0 {
		c.running = false
		w.log.Error("[Castle Siege] NextRemainTime < 0")
		return false
	}
	w.csSetRemain(d)
	return true
}

// csSetState is FUN_0041c690.
func (w *World) csSetState(state int, setRemain bool) {
	c := w.cs()
	if state < -1 || state > 9 {
		return
	}
	c.state = state
	switch state {
	case csIdle1:
		w.log.Info("[Castle Siege] SetState IDLE1")
		if setRemain {
			w.csRemainToStart()
		}
	case csRegSiege:
		w.csEnterRegSiege(setRemain)
	case csIdle2, csRegMark, csIdle3:
		w.log.Info("[Castle Siege] SetState", "state", state)
		if setRemain {
			w.csSetRemainFromSchedule(state)
		}
	case csNotify, csReady:
		if setRemain && !w.csSetRemainFromSchedule(state) {
			return
		}
		w.csLoadSiegeGuildList() // FUN_0041e670
	case csStart:
		w.csEnterStart(setRemain)
	case csEnd:
		w.csEnterEnd(setRemain)
	case csEndCycle:
		w.csEnterEndCycle()
		return // SetState(1) already ran the common tail.
	}
	w.csStateTail()
}

// csStateTail is the end of every SetState: create the DB NPCs once
// (FUN_0041dab0), remember when the state began, reset the notice timers.
func (w *World) csStateTail() {
	c := w.cs()
	if !c.npcsCreated {
		c.npcsCreated = true
		w.csCreateDBNPCs()
	}
	c.stateStart = w.now()
	c.noticeTick, c.checkTick = time.Time{}, time.Time{}
}

// csRemainToStart is FUN_0041c7e0: until 00:00 of the cycle start date.
func (w *World) csRemainToStart() {
	c := w.cs()
	ms := c.start.Sub(w.now()).Milliseconds()
	c.remain = ms
	c.target = w.now().Add(time.Duration(ms/1000) * time.Second)
}

// csEnterRegSiege is FUN_0041c8b0: the registration starts and the
// DataServer resets the siege (WZ_CS_ResetCastleSiege, 0x80 0x0E).
func (w *World) csEnterRegSiege(setRemain bool) {
	w.log.Info("[Castle Siege] SetState SIGN1")
	if setRemain && !w.csSetRemainFromSchedule(csRegSiege) {
		return
	}
	c, st := w.cs(), w.csStore()
	if st == nil {
		return
	}
	group := c.group
	w.async(func(ctx context.Context) func() {
		r, err := st.ResetCastleSiege(ctx, group)
		if err != nil || r != 1 {
			return nil
		}
		return w.csResetSiegeDone // FUN_00424260
	})
}

// csResetSiegeDone is FUN_00424260.
func (w *World) csResetSiegeDone() {
	c := w.cs()
	c.listLoaded, c.unionLoaded = false, false
	c.basic, c.all, c.order = map[string]*csGuild{}, map[string]*csGuild{}, nil
	c.crownRoom, c.crownOpen = false, false
	c.listSet, c.siegeEnded = false, false
}

// csEnterEnd is FUN_0041cd70.
func (w *World) csEnterEnd(setRemain bool) {
	c := w.cs()
	w.log.Info("[Castle Siege] SetState END")
	if setRemain {
		ms := c.end.Sub(w.now()).Milliseconds()
		c.remain = ms
		c.target = w.now().Add(time.Duration(ms/1000) * time.Second)
	}
	w.csClearBattleNPCs()  // FUN_0041d970
	w.csResetPlayersSide() // FUN_0041e7f0
	w.csGlobalNotice(110)
}

// csEnterEndCycle is FUN_0041cf30: the next cycle starts where this one's
// END_CYCLE row was and lasts CycleDays; the registrations and the siege
// guild list are cleared and SetState(1) starts the registration.
func (w *World) csEnterEndCycle() {
	c := w.cs()
	w.log.Info("[Castle Siege] SetState END_CYCLE")
	c.start = c.cycleEnd
	c.end = c.cycleEnd.AddDate(0, 0, c.cycleDays)
	c.cycleEnd = c.cycleEnd.AddDate(0, 0, c.cycleDays)
	w.csSaveSchedule() // FUN_004259d0 (0x80 0x09)
	if st := w.csStore(); st != nil {
		group := c.group
		w.async(func(ctx context.Context) func() {
			st.ResetCastleRegSiege(ctx, group)    // FUN_00425fe0 (0x80 0x1b)
			st.ResetCastleSiegeGuilds(ctx, group) // FUN_00425f50 (0x80 0x1a)
			return nil
		})
	}
	w.csSetState(csRegSiege, true)
}

// csSaveSchedule is FUN_004259d0 (WZ_CS_ModifyCastleSchedule).
func (w *World) csSaveSchedule() {
	c, st := w.cs(), w.csStore()
	if st == nil {
		return
	}
	group, start, end := c.group, c.start, c.end
	w.async(func(ctx context.Context) func() {
		st.SetCastleSchedule(ctx, group, start, end)
		return nil
	})
}

// csCheckSync is FUN_0041d020: pick the state from the dates and the
// schedule after start-up.
func (w *World) csCheckSync() bool {
	c := w.cs()
	if !c.fileOK {
		w.log.Error("[Castle Siege] CCastleSiege::CheckSync() - m_CastleSiegeLoadFile == 0")
		return false
	}
	now := w.now()
	c.remainTick = now
	if c.cycleOverride {
		c.start = c.cycleStart
		c.end = c.start.AddDate(0, 0, c.cycleDays)
	}
	if dateKey(c.end) <= dateKey(now) {
		// The stored cycle is over: a new one starts today.
		c.start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		c.end = c.start.AddDate(0, 0, c.cycleDays)
		w.csSaveSchedule()
		c.cycleOverride = false
		return w.csCheckSync()
	}
	if c.cycleOverride {
		w.csSaveSchedule()
	}
	if c.specValid {
		if _, ok := c.data.ScheduleOf(c.specState); !ok {
			w.log.Error("[Castle Siege] CCastleSiege::CheckSync() - FIXED STATE NOT FOUND IN SCHEDULE", "state", c.specState)
			return false
		}
		ms := c.specStart.Sub(now).Milliseconds()
		c.remain = ms
		c.target = now.Add(time.Duration(ms/1000) * time.Second)
		c.state = c.specState - 1
		w.csSetState(c.state, false)
		return true
	}
	if c.siegeEnded {
		c.state = csEnd
		w.csSetState(csEnd, true)
		return true
	}
	if dateKey(c.start) > dateKey(now) {
		c.state = csIdle1
		w.csSetState(csIdle1, true)
		return true
	}
	prev := 0
	for _, s := range c.data.Schedule {
		at := c.start.AddDate(0, 0, s.Day).Add(time.Duration(s.Hour)*time.Hour + time.Duration(s.Minute)*time.Minute)
		if now.Before(at) {
			c.state = prev
			ms := at.Sub(now).Milliseconds()
			c.remain = ms
			c.target = now.Add(time.Duration(ms/1000) * time.Second)
			// A restart during the battle (or in its last 15 minutes of
			// READY) resumes in READY with 15 minutes left.
			if c.state == csStart || (c.state == csReady && c.remain < 900000) {
				c.state = csReady
				c.remain = 900000
				c.target = now.Add(900 * time.Second)
			}
			w.csSetState(c.state, false)
			return true
		}
		prev = s.State
	}
	w.log.Error("[Castle Siege] CCastleSiege::CheckSync() - bStateSetted == 0")
	return false
}

// --- process ---------------------------------------------------------------

// csProcess is FUN_0041b780 (main timer, every 100 ms).
func (w *World) csProcess() {
	c := w.cs()
	if c == nil {
		return
	}
	if c.dbState == 2 {
		// FUN_00425c90: the 0x81 request is repeated every minute.
		if c.syncTick.IsZero() || w.now().Sub(c.syncTick) > 60*time.Second {
			c.syncTick = w.now()
			w.csRequestInit()
		}
		return
	}
	w.csPublishSync() // FUN_00425c90 (0x80 0x17 broadcast)
	if !c.running || !c.enabled {
		return
	}
	switch c.state {
	case csIdle1:
		w.csTimedState(csRegSiege, 0, 0)
	case csRegSiege:
		w.csTimedState(csIdle2, 30*time.Minute, 111)
	case csIdle2:
		w.csTimedState(csRegMark, 0, 0)
	case csRegMark:
		w.csTimedState(csIdle3, 30*time.Minute, 112)
	case csIdle3:
		w.csTimedState(csNotify, 2*time.Hour, 113)
	case csNotify:
		w.csTimedState(csReady, 30*time.Minute, 114)
	case csReady:
		w.csReadyProc()
	case csStart:
		w.csStartProc()
	case csEnd:
		w.csTimedState(csEndCycle, 0, 0)
	case csEndCycle:
		w.csTimedState(csIdle1, 0, 0) // FUN_0041c5f0
	}
	now := w.now()
	if now.Sub(c.saveTick) > 300*time.Second {
		if c.state != csStart {
			w.csSaveNPCs() // FUN_00426810
		}
		c.saveTick = now
	}
	if c.leftTick.IsZero() || now.Sub(c.leftTick) > 10*time.Second {
		c.leftTick = now
		w.csCalcLeftSiege() // FUN_0041d760
	}
	if c.hourTick.IsZero() || now.Sub(c.hourTick) > time.Hour {
		c.hourTick = now
		if s := w.csCalcLeftSiege(); s >= 0 {
			w.log.Info("[Castle Siege] Sync Start Time.", "days", s/86400)
		}
	}
}

// csUpdateRemain refreshes +0x20 once per second (difftime(+0x24, now)).
func (w *World) csUpdateRemain() {
	c := w.cs()
	now := w.now()
	if now.Sub(c.remainTick) < time.Second {
		return
	}
	c.remain = c.target.Sub(now).Milliseconds()
	c.remainTick = now
}

// csTimedState is the shape of FUN_0041b8d0 .. FUN_0041c5f0: the remain
// time, a global notice every period, and the next state at 0.
func (w *World) csTimedState(next int, period time.Duration, msg int) {
	c := w.cs()
	w.csUpdateRemain()
	if period > 0 {
		now := w.now()
		if c.noticeTick.IsZero() || now.Sub(c.noticeTick) > period {
			w.csGlobalNotice(msg)
			c.noticeTick = now
		}
	}
	if c.remain < 1 {
		w.csSetState(next, true)
	}
}

// csReadyProc is FUN_0041c0b0: notice 115 every 30 minutes and, in the
// last 5 minutes, notice 116 with the minutes left every minute.
func (w *World) csReadyProc() {
	c := w.cs()
	w.csUpdateRemain()
	now := w.now()
	if c.noticeTick.IsZero() || now.Sub(c.noticeTick) > 30*time.Minute {
		w.csGlobalNotice(115)
		c.noticeTick = now
	}
	if c.remain <= 300000 && (c.readyTick.IsZero() || now.Sub(c.readyTick) > time.Minute) {
		w.csGlobalNotice(116, int(c.remain/60000)+1)
		c.readyTick = now
	}
	if c.remain < 1 {
		w.csSetState(csStart, true)
	}
}

// csCalcLeftSiege is FUN_0041d760: the date of the next battle (+0x148)
// and the seconds left, -1 when unknown (during or after the battle).
func (w *World) csCalcLeftSiege() int {
	c := w.cs()
	c.leftSiege = time.Time{}
	if c.state < 0 || c.state > 9 || !c.running || c.state == csStart || c.siegeEnded || c.state > csStart {
		return -1
	}
	left, found := -1, false
	for _, s := range c.data.Schedule {
		if s.State == csStart {
			break
		}
		if found {
			left += s.Duration
		}
		if s.State == c.state {
			left = int(c.remain / 1000)
			found = true
		}
	}
	if left < 0 {
		return -1
	}
	c.leftSiege = w.now().Add(time.Duration(left) * time.Second)
	return left
}

// csGlobalNotice is FUN_00475b50 with the Message.txt text (default
// language): a type 0 notice to every game server of the group
// (groupNotice, castledeep.go).
func (w *World) csGlobalNotice(msg int, args ...any) {
	w.groupNotice(NoticeGold, w.messageFor(nil, msg, args...))
}

// csSendMap is FUN_004202f0: every player on map 30.
func (w *World) csSendMap(p []byte) {
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.Map == csMap {
			w.send(o, p)
		}
	}
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		if w.cfg.ServerType != 1 {
			return nil
		}
		return w.csLoad()
	})
	tick100Hooks = append(tick100Hooks, (*World).csProcess)
}
