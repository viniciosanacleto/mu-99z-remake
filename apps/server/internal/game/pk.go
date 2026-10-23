package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Player killing: PK level / count / time (obj+0x19c PKLevel, +0x1a0
// PKCount, +0x1a4 PKTime), FUN_0050f330 (a player killed another player),
// FUN_0050e750 (PK time decrease) and the C1 07 F3 08 broadcast
// (FUN_004db9d0).
//
// PK levels: 0..2 hero, 3 commoner, 4..6 murderer (4 warning, 5 and 6
// outlaw). PKTime is a counter in "points" decreased by
// PKDownPlusTimePoint every second.

// PK level constants.
const (
	PKHero2      = 0
	PKHero1      = 1
	PKHero0      = 2
	PKCommoner   = 3
	PKWarning    = 4
	PKOutlaw1    = 5
	PKOutlaw2    = 6
	selfDefSlots = 5
)

// pkSelfDefense is obj+0x2c4: the players this player attacked first
// (5 slots). Killing someone who is in the victim's list is not a PK.
type pkSelfDefense struct {
	Index [selfDefSlots]int
	Time  [selfDefSlots]time.Time
	init  bool
}

func init() {
	playerKillHook = func(w *World, killer, victim *Object) { w.playerKill(killer, victim) }
	tick1000Hooks = append(tick1000Hooks, func(w *World) { w.pkSecond() })
}

// packetPKLevel is C1 07 F3 08 index(BE) level.
func packetPKLevel(index, level int) []byte {
	b := newPacket(frame.C1, 0xF3, 0x08, 7)
	b[4], b[5], b[6] = byte(index>>8), byte(index), byte(level)
	return b
}

// sendPKLevel is FUN_004db9d0: to the player and its viewport.
func (w *World) sendPKLevel(o *Object) {
	pkt := packetPKLevel(o.Index, o.PKLevel)
	w.send(o, pkt)
	w.broadcast(o, pkt)
}

// pkLevelFromTime maps PKTime to a level (FUN_0050e750 / FUN_0050f330).
func (w *World) pkLevelFromTime(murderer bool, t int) int {
	req1 := w.cfg.Int("Common", "PKDownRequirePoint1", 0)
	req2 := w.cfg.Int("Common", "PKDownRequirePoint2", 0)
	switch {
	case t > req2:
		if murderer {
			return PKOutlaw2
		}
		return PKHero2
	case t > req1:
		if murderer {
			return PKOutlaw1
		}
		return PKHero1
	case t > 0:
		if murderer {
			return PKWarning
		}
		return PKHero0
	}
	return PKCommoner
}

// pkDown is FUN_0050e750(points, flag): heroes only lose time when flag
// is 0, murderers always.
func (w *World) pkDown(o *Object, points int, keep bool) {
	if o.PKLevel == PKCommoner || o.P == nil || o.P.Char == nil {
		return
	}
	c := o.P.Char
	murderer := o.PKLevel > PKCommoner
	if murderer || !keep {
		c.PKTime -= points
	}
	if lvl := w.pkLevelFromTime(murderer, c.PKTime); lvl != o.PKLevel {
		o.PKLevel = lvl
		c.PKLevel = lvl
		w.sendPKLevel(o)
	}
}

// pkSecond is the FUN_00510810 call FUN_0050e750(PKDownPlusTimePoint, 0).
func (w *World) pkSecond() {
	pts := w.cfg.Int("Common", "PKDownPlusTimePoint", 0)
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State == StatePlaying && o.Type == TypePlayer && o.P != nil && o.P.Char != nil {
			w.pkDown(o, pts, false)
		}
	}
}

// addSelfDefense records that attacker hit target first (for the combat
// module; SelfDefenseTime expiry is applied in isSelfDefense).
func (w *World) addSelfDefense(attacker, target *Object) {
	if attacker.P == nil || target.Type != TypePlayer {
		return
	}
	sd := &w.statsOf(attacker).selfDefense
	if !sd.init {
		sd.Index, sd.init = [selfDefSlots]int{-1, -1, -1, -1, -1}, true
	}
	free := -1
	for i, idx := range sd.Index {
		if idx == target.Index {
			sd.Time[i] = w.now()
			return
		}
		if idx < 0 && free < 0 {
			free = i
		}
	}
	if free >= 0 {
		sd.Index[free], sd.Time[free] = target.Index, w.now()
	}
}

// isSelfDefense reports whether victim attacked killer first.
func (w *World) isSelfDefense(killer, victim *Object) bool {
	if victim.P == nil || victim.P.Stats == nil || !victim.P.Stats.selfDefense.init {
		return false
	}
	sd := &victim.P.Stats.selfDefense
	limit := time.Duration(w.cfg.Int("Common", "SelfDefenseTime", 0)) * time.Second
	for i, idx := range sd.Index {
		if idx == killer.Index && w.now().Sub(sd.Time[i]) <= limit {
			return true
		}
	}
	return false
}

// playerKill is FUN_0050f330: killer killed victim. Duels, guild wars and
// Castle Siege rules are not reproduced (U-PK-01).
func (w *World) playerKill(killer, victim *Object) {
	if killer.Type != TypePlayer || victim.Type != TypePlayer || killer.P == nil || victim.P == nil ||
		killer.P.Char == nil || victim.P.Char == nil {
		return
	}
	if !w.scriptCheckUserKiller(killer, victim) { // BridgeFunction_OnCheckUserKiller (FUN_004e73b0, lua.go)
		return
	}
	arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
	if killer.Map == arena && victim.Map == arena {
		return
	}
	if killer.Map > 0x11 && killer.Map < 0x18 && victim.Map > 0x11 && victim.Map < 0x18 {
		return // Chaos Castle
	}
	for _, h := range pkExemptHooks {
		if h(w, killer, victim) {
			return
		}
	}
	if killer.Map == 30 && victim.PKLevel > 5 {
		return
	}
	if info, ok := w.data.Maps[killer.Map]; ok && info.NonPK == 2 {
		return // FUN_004ae7f0
	}
	if w.isSelfDefense(killer, victim) {
		return
	}
	c := killer.P.Char
	req := func(n int) int { return w.cfg.Int("Common", "PKDownRequirePoint"+string(rune('0'+n)), 0) }
	lvl := killer.PKLevel
	if lvl < PKWarning {
		if victim.PKLevel > PKWarning {
			// Killing an outlaw makes (or keeps) a hero.
			c.PKCount++
			switch lvl {
			case PKCommoner:
				c.PKTime = req(1)
			case PKHero0:
				c.PKTime = req(2)
			case PKHero1:
				c.PKTime = req(3)
			default:
				c.PKTime += req(4)
			}
			killer.PKLevel = w.pkLevelFromTime(false, c.PKTime)
			if c.PKTime <= 0 {
				killer.PKLevel = lvl // the original leaves the level unchanged
			}
		} else {
			killer.PKLevel = PKCommoner
			c.PKCount = 1
			c.PKTime = req(1)
			killer.PKLevel = w.pkMurdererLevel(c.PKTime, PKCommoner)
		}
	} else {
		if victim.PKLevel > PKWarning {
			return
		}
		c.PKCount++
		switch lvl {
		case PKWarning:
			c.PKTime = req(2)
		case PKOutlaw1:
			c.PKTime = req(3)
		default:
			c.PKTime += req(4)
		}
		killer.PKLevel = w.pkMurdererLevel(c.PKTime, lvl)
	}
	c.PKLevel = killer.PKLevel
	w.sendPKLevel(killer)
}

// pkMurdererLevel is the tail of FUN_0050f330: 6/5/4 by PKTime, otherwise
// the current level is kept.
func (w *World) pkMurdererLevel(t, cur int) int {
	switch {
	case t > w.cfg.Int("Common", "PKDownRequirePoint2", 0):
		return PKOutlaw2
	case t > w.cfg.Int("Common", "PKDownRequirePoint1", 0):
		return PKOutlaw1
	case t > 0:
		return PKWarning
	}
	return cur
}
