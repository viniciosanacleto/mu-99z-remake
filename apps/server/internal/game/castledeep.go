package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Castle Deep event (CCastleDeep, 0x419060..0x41a2c0; singleton at
// 0x01241a68; Event/CastleDeepEvent.dat; switch CastleDeepEvent and
// duration CastleDeepEventTime in "GameServerInfo - Event.dat",
// CServerInfo+0x6e4 / +0x6e8). An invasion of Valley of Loren (map 30):
// EMPTY waits for the next schedule (a 5-minute warning and the start
// notice go to every server of the group), START draws one group by its
// rate and spawns each stage of the group at its minute; at the end every
// attribute-7 monster is removed.
//
// Layout of the singleton: +4 state, +8 remain (s), +0xc target time,
// +0x10 tick, +0x14 warned, +0x18 stage, +0x1c group, +0x20/+0x30/+0x40/
// +0x50 the four vectors of the data file.

const (
	cdDisabled = iota
	cdEmpty
	cdStart
)

const (
	cdMap       = 30   // Valley of Loren (obj+0x1ad = 0x1e)
	cdAttribute = 7    // obj+0x37e of the event monsters
	cdRegen     = 1000 // obj+0x290 (ms)
	cdWarnSec   = 300  // 0 < remain < 0x12d
	cdTries     = 10   // FUN_0050c860 attempts

	cdMsgWarn  = 52 // 0x34 "Castle Deep will start in 5 minutes"
	cdMsgStart = 53 // 0x35
	cdMsgEnd   = 54 // 0x36
	cdMsgKill  = 55 // 0x37 "%s was killed by %s"
)

type castleDeep struct {
	data   *gamedata.CastleDeep
	State  int       // +4
	Remain int       // +8, seconds
	Target time.Time // +0xc
	Tick   time.Time // +0x10
	Warned bool      // +0x14
	Stage  int       // +0x18
	Group  int       // +0x1c (0 = none)
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		// FUN_00419060: state EMPTY with nothing scheduled until Init.
		cd := &castleDeep{data: &gamedata.CastleDeep{}, State: cdEmpty, Tick: w.now()}
		w.ev().deep = cd
		if w.dataDir != "" {
			d, err := gamedata.LoadCastleDeep(w.dataDir) // FUN_00419320
			if err != nil {
				return err
			}
			cd.data = d
		}
		w.cdInit()
		return nil
	})
	tick1000Hooks = append(tick1000Hooks, (*World).cdTick)
	deathHooks = append(deathHooks, func(w *World, killer, victim *Object) {
		if victim.Map == cdMap && victim.Type != TypePlayer {
			w.cdMonsterDie(killer, victim)
		}
	})
}

func (w *World) deep() *castleDeep { return w.ev().deep }

func (w *World) cdOn() bool { return w.cfg.Int("Event", "CastleDeepEvent", 0) != 0 }

// cdInit is FUN_00419260 (called by the start-up spawn FUN_004baa20):
// with the switch off nothing happens (the tick is gated by the switch).
func (w *World) cdInit() {
	cd := w.deep()
	if cd == nil || !w.cdOn() {
		return
	}
	cd.State = cdEmpty
	w.cdEnterEmpty()
}

// cdSetState is FUN_00419ad0.
func (w *World) cdSetState(s int) {
	if !w.cdOn() {
		return
	}
	w.deep().State = s
	switch s {
	case cdDisabled:
		w.log.Info("[Castle Deep] SetState DISABLED")
	case cdEmpty:
		w.cdEnterEmpty()
	case cdStart:
		w.cdEnterStart()
	}
}

// cdTick is FUN_00419720: once a second the remaining time is
// difftime(target, now) truncated, then the state procedure runs.
func (w *World) cdTick() {
	cd := w.deep()
	if cd == nil || !w.cdOn() {
		return
	}
	now := w.now()
	if now.Sub(cd.Tick) < time.Second {
		return
	}
	cd.Tick = now
	cd.Remain = int(cd.Target.Sub(now).Seconds())
	switch cd.State {
	case cdEmpty:
		w.cdEmptyProc()
	case cdStart:
		w.cdStartProc()
	}
}

// cdEmptyProc is FUN_004197c0.
func (w *World) cdEmptyProc() {
	cd := w.deep()
	if cd.Remain > 0 && cd.Remain <= cdWarnSec && !cd.Warned {
		cd.Warned = true
		w.groupNotice(NoticeGold, w.messageFor(nil, cdMsgWarn))
	}
	if cd.Remain < 1 {
		w.groupNotice(NoticeGold, w.messageFor(nil, cdMsgStart))
		w.cdSetState(cdStart)
	}
}

// cdStartProc is FUN_00419960: the stage row of the drawn group whose
// minute equals the elapsed minutes spawns and advances the stage; at the
// end the local notice 54 and back to EMPTY.
func (w *World) cdStartProc() {
	cd := w.deep()
	elapsed := (w.cdEventTime()*60 - cd.Remain) / 60
	for _, r := range cd.data.Stages {
		if r.Group == cd.Group && r.Stage == cd.Stage && r.Time == elapsed {
			w.cdSpawn(cd.Group, cd.Stage)
			cd.Stage++
			break
		}
	}
	if cd.Remain < 1 {
		w.noticeAllMsg(NoticeGold, cdMsgEnd)
		if w.cdOn() {
			cd.State = cdEmpty
			w.cdEnterEmpty()
		}
	}
}

func (w *World) cdEventTime() int { return w.cfg.Int("Event", "CastleDeepEventTime", 0) }

// cdEnterEmpty is FUN_00419b50.
func (w *World) cdEnterEmpty() {
	cd := w.deep()
	cd.Warned, cd.Stage, cd.Group = false, 1, 0
	w.cdClearMonsters()
	w.cdSync()
	w.log.Info("[Castle Deep] SetState EMPTY")
}

// cdSync is FUN_00419c60: the next schedule start; none (or an empty
// schedule) disables the event.
func (w *World) cdSync() {
	cd := w.deep()
	now := w.now()
	next, ok := nextSchedule(cd.data.Schedule, now)
	if !ok {
		w.cdSetState(cdDisabled)
		return
	}
	cd.Target = next
	cd.Remain = int(next.Sub(now).Seconds())
}

// cdEnterStart is FUN_00419b90: EventTime minutes and a group drawn in
// file order (the first row with rand%100 < rate wins; the original
// redraws until one wins and hangs when none can, which is cut short
// here).
func (w *World) cdEnterStart() {
	cd := w.deep()
	cd.Remain = w.cdEventTime() * 60
	cd.Target = w.now().Add(time.Duration(cd.Remain) * time.Second)
	if !cd.drawable() {
		w.log.Warn("[Castle Deep] no group can be drawn")
	}
	for cd.Group == 0 && cd.drawable() {
		for _, g := range cd.data.Groups {
			if monsterRand()%100 < g.Rate {
				cd.Group = g.Group
				break
			}
		}
	}
	w.log.Info("[Castle Deep] SetState START", "group", cd.Group)
}

// drawable: some row can end the draw loop of FUN_00419b90.
func (cd *castleDeep) drawable() bool {
	for _, g := range cd.data.Groups {
		if g.Rate > 0 && g.Group != 0 {
			return true
		}
	}
	return false
}

// cdClearMonsters is FUN_00419e10: every playing monster index (0..0x5013)
// with attribute 7, on any map.
func (w *World) cdClearMonsters() {
	for i := 0; i < SummonStart; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.Mon != nil && o.Mon.Attribute == cdAttribute {
			w.deleteMonster(o)
		}
	}
}

// cdSpawn is FUN_00419e60: MaxSpawnCount monsters per section-3 row of the
// group and stage, each on a random free cell around (RangeStartX,
// RangeStartY) within +-(RangeEnd - RangeStart) (FUN_0050c860, 10 tries,
// else the start corner), regenerating near their death cell after 1 s.
func (w *World) cdSpawn(group, stage int) {
	for _, r := range w.deep().data.Monsters {
		if r.Group != group || r.Stage != stage {
			continue
		}
		for n := 0; n < r.Count; n++ {
			idx := w.allocMonsterIndex()
			if idx < 0 {
				continue
			}
			o := w.objects[idx]
			o.reset()
			o.State = StateConnected
			x, y, ok := w.randomNearCell(cdMap, r.X1, r.Y1, r.X2-r.X1, r.Y2-r.Y1, cdTries)
			if !ok {
				x, y = r.X1, r.Y1
			}
			o.Map, o.X, o.Y, o.TX, o.TY = cdMap, x, y, x, y
			o.Dir = monsterRand() % 8
			if !w.setMonster(o, r.Class) {
				w.deleteMonster(o)
				continue
			}
			m := o.Mon
			m.SpawnIndex = -1 // obj+0x294 = 0xFFFF
			m.Die = 0
			m.Attribute = cdAttribute
			m.RegenTime = cdRegen
		}
	}
}

// cdMonsterDie is FUN_0041a050 (from FUN_004cda80 for monsters on map 30):
// the top hitter (else the killer) is named when an attribute-7 monster of
// class 295 or 300..303 dies.
func (w *World) cdMonsterDie(killer, victim *Object) {
	if victim.Mon == nil || victim.Mon.Attribute != cdAttribute {
		return
	}
	if c := victim.Class; c != 295 && (c < 300 || c > 303) {
		return
	}
	if top := w.topHitter(victim); top >= 0 && top < MaxObjects {
		killer = w.objects[top]
	}
	w.noticeAllMsg(NoticeGold, cdMsgKill, monsterName(victim), cdName(killer))
}

// cdName is obj+0x7a: the character name of a player, the MonsterList
// name otherwise.
func cdName(o *Object) string {
	if o == nil {
		return ""
	}
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Name
	}
	return monsterName(o)
}

// --- notices to the server group -----------------------------------------

// mapServerGroup is the group of the instance in MapServerInfo.txt (the
// +4 field of CMapServerManager's own entry, DAT_01239534); -1 when the
// server is not listed.
func (w *World) mapServerGroup() int {
	if w.data == nil || w.data.MapServer == nil {
		return -1
	}
	for _, s := range w.data.MapServer.Servers {
		if s.Code == w.cfg.ServerCode {
			return s.Group
		}
	}
	return -1
}

// groupNotice is FUN_00475b50 (C1 94 21 to the DataServer: group u16,
// type, count, opacity, delay u16, color u32, speed, text[128]), relayed by
// the DataServer to every game server (FUN_0040e4a0) and shown by those of
// the same group (FUN_00474c90 -> FUN_004c0ab0). The other instances of
// the process stand for the DataServer's connections.
func (w *World) groupNotice(typ byte, text string) {
	group := w.mapServerGroup()
	if group < 0 {
		return // FUN_00474c90 compares 0xFFFF with -1: never shown
	}
	w.hub.mu.Lock()
	worlds := make([]*World, 0, len(w.hub.worlds))
	for _, t := range w.hub.worlds {
		worlds = append(worlds, t)
	}
	w.hub.mu.Unlock()
	for _, t := range worlds {
		if t.mapServerGroup() != group {
			continue
		}
		if t == w {
			w.noticeAll(typ, text)
			continue
		}
		t.post(func() { t.noticeAll(typ, text) })
	}
}
