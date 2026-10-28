package game

import (
	"fmt"
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Invasion manager (CInvasionManager, 0x488be0..0x48a730;
// Event/InvasionManager.dat). Up to 20 invasions alternate EMPTY (waiting
// for the next schedule) and START (monsters spawned for InvasionTime
// seconds). Each START draws one map row per group and spawns the matching
// section-3 rows.

const (
	invBlank = iota
	invEmpty
	invStart
	invMaxMobs    = 500
	invAttribute  = 5 // obj+0x37e of invasion monsters
	invCounters   = 20
	invMsgKilled  = 1100 // 0x44c: last monster of a kill-message row killed
	invMsgRemains = 1101 // 0x44d: "%s killed %s, %d/%d"
)

type invCounter struct {
	Class, Alive, Max int // r[0x2c3 + i*6]
}

type invasionState struct {
	data      *gamedata.Invasion
	State     int
	Remain    int
	Target    time.Time
	Tick      time.Time
	Mobs      []int
	BossOnMap [100]int // r[0x205..]
	Counters  [invCounters]invCounter
}

func registerInvasion(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		e := w.ev()
		if w.configDir == "" {
			return nil
		}
		list, err := gamedata.LoadInvasions(w.configDir)
		if err != nil {
			return err
		}
		for _, d := range list {
			if d == nil {
				e.inv = append(e.inv, nil)
				continue
			}
			s := &invasionState{data: d, Tick: w.now()}
			s.resetCounters()
			e.inv = append(e.inv, s)
		}
		// FUN_00488de0.
		if w.invOn() {
			for _, s := range e.inv {
				if s != nil {
					s.State = invEmpty
					w.invEnterEmpty(s)
				}
			}
		}
		return nil
	})
	h.tick1000 = append(h.tick1000, (*World).invTick)
	h.death = append(h.death, func(w *World, killer, victim *Object) {
		if victim.Mon != nil && victim.Mon.Invasion >= 0 {
			w.invMonsterKilled(victim)
		}
	})
	h.mapEventState = (*World).invMapState
}

func (w *World) invOn() bool { return w.cfg.Int("Event", "InvasionManagerSwitch", 0) != 0 }

func (s *invasionState) resetCounters() {
	for i := range s.Counters {
		s.Counters[i] = invCounter{Class: -1}
	}
}

// packetMapEvent is C1 05 0B flag effect (FUN_004d9ea0 / FUN_0048a690).
func packetMapEvent(on bool, effect int) []byte {
	p := []byte{0xC1, 0x05, 0x0B, 0, byte(effect)}
	if on {
		p[3] = 1
	}
	return p
}

// invSendMapEvent is FUN_004d9ea0: every playing player of the map
// (mp = -1: all maps).
func (w *World) invSendMapEvent(mp int, on bool, effect int) {
	p := packetMapEvent(on, effect)
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if noticePlayer(o) && (mp == -1 || o.Map == mp) {
			w.send(o, p)
		}
	}
}

// invMapState is FUN_0048a690: the first running invasion with a living
// boss on the player's map and an effect type gives C1 05 0B 01 effect.
func (w *World) invMapState(o *Object) []byte {
	for _, s := range w.ev().inv {
		if s == nil || s.State != invStart || s.data.EffectType == 0 || o.Map < 0 || o.Map >= len(s.BossOnMap) {
			continue
		}
		if s.BossOnMap[o.Map] > 0 {
			return packetMapEvent(true, s.data.EffectType)
		}
	}
	return packetMapEvent(false, 0)
}

// invNotice is FUN_004c0ab0 with the text of FUN_004b3470 (the default
// language table).
func (w *World) invNotice(n int, args ...any) {
	text := w.messageFor(nil, n)
	if len(args) > 0 {
		text = fmt.Sprintf(cFormat(text), args...)
	}
	w.noticeAll(NoticeGold, text)
}

// invTick is FUN_00489610.
func (w *World) invTick() {
	if !w.invOn() {
		return
	}
	now := w.now()
	for _, s := range w.ev().inv {
		if s == nil || now.Sub(s.Tick) < time.Second {
			continue
		}
		s.Tick = now
		s.Remain = int(math.Floor(s.Target.Sub(now).Seconds()))
		switch s.State {
		case invEmpty: // FUN_004896c0
			if s.Remain < 1 {
				if s.data.RespawnMsg != -1 {
					w.invNotice(s.data.RespawnMsg)
				}
				if w.invOn() {
					s.State = invStart
					w.invStart(s)
				}
			}
		case invStart: // FUN_004897f0
			if s.Remain < 1 {
				if s.data.DespawnMsg != -1 {
					w.invNotice(s.data.DespawnMsg)
				}
				if w.invOn() {
					s.State = invEmpty
					w.invEnterEmpty(s)
				}
			}
		}
	}
}

// invEnterEmpty is FUN_00489910: monsters removed, next schedule, boss
// counts cleared (and the map effect turned off everywhere).
func (w *World) invEnterEmpty(s *invasionState) {
	for _, idx := range s.Mobs {
		if o := w.objects[idx]; o.State != StateEmpty && o.Mon != nil {
			w.deleteMonster(o)
		}
	}
	s.Mobs = s.Mobs[:0]
	s.resetCounters()
	s.BossOnMap = [100]int{}
	if s.data.EffectType != 0 {
		w.invSendMapEvent(-1, false, s.data.EffectType)
	}
	now := w.now()
	next, ok := nextSchedule(s.data.Schedule, now)
	if !ok {
		s.State = invBlank
		return
	}
	s.Target = next
	s.Remain = int(math.Floor(next.Sub(now).Seconds()))
}

// invStart is FUN_00489960.
func (w *World) invStart(s *invasionState) {
	s.BossOnMap = [100]int{}
	d := s.data
	for g := 0; g < invCounters; g++ {
		rows := d.Groups[g]
		if len(rows) == 0 {
			continue
		}
		pick := rows[rnd(len(rows))]
		for _, sp := range d.Spawns {
			if sp.Group != pick.Group || sp.Map != pick.Map || sp.Stage != pick.Stage {
				continue
			}
			for n := 0; n < sp.Count; n++ {
				w.invSpawn(s, sp)
			}
		}
	}
	s.Remain = d.Time
	s.Target = w.now().Add(time.Duration(d.Time) * time.Second)
}

func (w *World) invSpawn(s *invasionState, sp gamedata.InvasionSpawn) {
	idx := w.allocMonsterIndex()
	if idx < 0 {
		return
	}
	o := w.objects[idx]
	o.reset()
	o.State = StateConnected
	x, y := w.invCell(sp)
	o.Map, o.X, o.Y, o.TX, o.TY, o.Dir = sp.Map, x, y, x, y, rnd(8)
	if !w.setMonster(o, sp.Monster) {
		w.deleteMonster(o)
		return
	}
	m := o.Mon
	m.MoveRange = sp.Radius
	m.Attribute = invAttribute
	m.Invasion = s.data.Index
	if sp.RegenType == 0 {
		m.RegenTime = s.data.Time * 1000
	} else {
		m.RegenTime = sp.Regen
	}
	// Per-class counters (r[0x2c3]).
	found := false
	for i := range s.Counters {
		c := &s.Counters[i]
		if c.Class == o.Class && c.Class > 0 {
			c.Alive++
			if c.Max < c.Alive {
				c.Max = c.Alive
			}
			found = true
			break
		}
	}
	if !found {
		for i := range s.Counters {
			if c := &s.Counters[i]; c.Class == -1 {
				*c = invCounter{Class: o.Class, Alive: 1, Max: 1}
				break
			}
		}
	}
	if len(s.Mobs) >= invMaxMobs {
		w.deleteMonster(o)
		return
	}
	s.Mobs = append(s.Mobs, idx)
	if s.data.Boss == sp.Monster && sp.Map >= 0 && sp.Map < len(s.BossOnMap) {
		s.BossOnMap[sp.Map]++
		if s.data.EffectType != 0 {
			w.invSendMapEvent(sp.Map, true, s.data.EffectType)
		}
	}
}

// invCell is FUN_0050c9c0: a random free cell of the range, else its
// first corner.
func (w *World) invCell(sp gamedata.InvasionSpawn) (int, int) {
	t := w.terrain(sp.Map)
	dx, dy := sp.X2-sp.X1, sp.Y2-sp.Y1
	if t != nil && dx > 0 && dy > 0 {
		for i := 0; i < 100; i++ {
			x, y := sp.X1+rnd(dx), sp.Y1+rnd(dy)
			if x < t.Width && y < t.Height && t.Get(x, y)&(1|4|8) == 0 {
				return x, y
			}
		}
	}
	return sp.X1, sp.Y1
}

// invMonsterKilled is FUN_0048a2d0.
func (w *World) invMonsterKilled(mon *Object) {
	list := w.ev().inv
	if mon.Mon.Invasion < 0 || mon.Mon.Invasion >= len(list) || list[mon.Mon.Invasion] == nil {
		return
	}
	s := list[mon.Mon.Invasion]
	if s.State != invStart {
		return
	}
	owned := false
	for _, idx := range s.Mobs {
		owned = owned || idx == mon.Index
	}
	if !owned {
		return
	}
	var killer *Object
	if top := w.topHitter(mon); top >= 0 && top < MaxObjects {
		killer = w.objects[top]
	}
	name := monsterName(mon)
	for i := range s.Counters {
		c := &s.Counters[i]
		if c.Class != mon.Class {
			continue
		}
		if c.Alive > 0 {
			c.Alive--
		}
		// The kill-message flag is read from the spawn row with the same
		// position as the counter (r[0x2bf] + i*0x34 + 0x30).
		if i < len(s.data.Spawns) && s.data.Spawns[i].KillMessage == 1 {
			if c.Alive == 0 {
				w.invNotice(invMsgKilled, name)
			} else {
				w.invNotice(invMsgRemains, bcName(killer), name, c.Max-c.Alive, c.Max)
			}
		}
	}
	if mon.Class == s.data.Boss && killer != nil {
		mp := killer.Map
		if mp >= 0 && mp < len(s.BossOnMap) {
			s.BossOnMap[mp]--
			if s.BossOnMap[mp] < 1 && s.data.EffectType != 0 {
				w.invSendMapEvent(mp, false, s.data.EffectType)
			}
		}
		if s.data.BossKilledMsg != -1 {
			mapName := "Unknown"
			if info, ok := w.data.Maps[killer.Map]; ok {
				mapName = info.Name
			}
			w.invNotice(s.data.BossKilledMsg, bcName(killer), mapName)
		}
	}
}

func monsterName(o *Object) string {
	if o.Mon != nil && o.Mon.Info != nil {
		return o.Mon.Info.Name
	}
	return "No Name"
}
