package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Kalima (CKalima, 0x4a2c20..0x4a3510; Event/Kalima.dat): dropping a Lost
// Map opens a gate ("Magic stone", classes 152..158) to Kalima 1..7; the
// Illusion of Kundun of Kalima n opens one to Kalima n+1 when Kalima.dat
// enables it. The gate lives 3 minutes and takes its owner and the owner's
// party standing next to it.

const (
	itemLostMap     = 0x1C1C // 14,28
	kalimaGateClass = 0x97   // + map level 1..7
	kalimaGateAttr  = 6      // obj+0x37e of the gate
	kalimaGateLife  = 180 * time.Second
	kundunClass     = 0x113
	kundunRegenTime = 300 * time.Second
)

// kalimaGateMoves is the switch of FUN_004a2d10: gate number by level.
var kalimaGateMoves = [7]int{0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x74}

// kalimaGate is the gate part of a monster object: owner (obj+0x310),
// players taken (obj+0x2b4), level 0..6 (obj+0x2b8) and creation time
// (obj+0x28c).
type kalimaGate struct {
	Owner, Count, Level int
	Created             time.Time
}

type kalimaState struct {
	next  [gamedata.KalimaLevels]int // Kalima.dat CreateNextPortal (this+4)
	gates map[int]*kalimaGate
	regen map[int]time.Time // Kundun obj+0x9f8
}

func (w *World) kalima() *kalimaState {
	e := w.ev()
	if e.kalima == nil {
		e.kalima = &kalimaState{gates: map[int]*kalimaGate{}, regen: map[int]time.Time{}}
	}
	return e.kalima
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		w.ev().kalima = nil
		if w.configDir == "" {
			return nil
		}
		next, err := gamedata.LoadKalima(w.configDir)
		w.kalima().next = next
		return err
	})
	tick1000Hooks = append(tick1000Hooks, (*World).kalimaTick)
	// gObjMoveGate (FUN_0050f6d0) removes the gate when its owner moves to
	// another map (the two FUN_004a3480 calls).
	mapChangeHooks = append(mapChangeHooks, func(w *World, o *Object, from, to int) {
		w.removeKalimaGate(o)
	})
}

func isKalimaMap(mp int) bool { return (mp >= 24 && mp <= 29) || mp == 36 }

// noLostMapMap lists the maps FUN_004a2f20 refuses: Icarus, Kalima, Devil
// Square, Blood Castle and Chaos Castle.
func noLostMapMap(mp int) bool {
	return mp == 10 || isKalimaMap(mp) || mp == 9 || mp == 32 || (mp >= 11 && mp <= 17) || (mp >= 18 && mp <= 23)
}

// lostMapDrop is FUN_004a2f20 from the 23 handler: level 1..7 (the item
// level), the gate at the dropped cell or a free cell within +-3 (50
// tries); not from a safe zone and not with a gate already open.
func (w *World) lostMapDrop(o *Object, level, x, y int) bool {
	if level < 1 || level > 7 || noLostMapMap(o.Map) {
		return false
	}
	if w.cellHas(o, gamedata.AttrSafeZone) || o.P.hasGate {
		return false
	}
	if w.attrAt(o.Map, x, y) != 0 {
		nx, ny, ok := w.randomNearCell(o.Map, x, y, 3, 3, 50)
		if !ok {
			return false
		}
		x, y = nx, ny
	}
	return w.createKalimaGate(o, o.Map, x, y, level-1, kalimaGateClass+level)
}

// kundunLevel is the class switch of FUN_004a3150: the Illusion of Kundun
// of Kalima n (1..6), 0 for other classes.
func kundunLevel(class int) int {
	switch class {
	case 0xA1:
		return 1
	case 0xB5:
		return 2
	case 0xBD:
		return 3
	case 0xC5:
		return 4
	case 0x10B:
		return 5
	case 0x152:
		return 6
	}
	return 0
}

// kundunDie is FUN_004a3150, called from the msgDie handler (FUN_004b70b0)
// before the drop: on a Kalima map the killer gets a gate to the next
// Kalima on the Kundun's cell (or within +-3) when Kalima.dat enables it.
func (w *World) kundunDie(mon *Object, killer int) {
	n := kundunLevel(mon.Class)
	if n == 0 || !isKalimaMap(mon.Map) || killer < 0 || killer >= MaxObjects {
		return
	}
	k := w.objects[killer]
	if !noticePlayer(k) || w.kalima().next[n-1] == 0 || k.P.hasGate {
		return
	}
	x, y := mon.X, mon.Y
	if w.attrAt(k.Map, x, y) != 0 {
		nx, ny, ok := w.randomNearCell(k.Map, x, y, 3, 3, 50)
		if !ok {
			return
		}
		x, y = nx, ny
	}
	w.createKalimaGate(k, k.Map, x, y, n, kalimaGateClass+1+n)
}

// createKalimaGate is the common tail of FUN_004a2f20 / FUN_004a3150.
func (w *World) createKalimaGate(o *Object, mp, x, y, level, class int) bool {
	idx := w.allocMonsterIndex()
	if idx < 0 {
		return false
	}
	g := w.objects[idx]
	g.reset()
	g.State = StateConnected
	g.Map, g.X, g.Y, g.TX, g.TY = mp, x, y, x, y
	g.Mon = &monsterState{SpawnIndex: -1, StartX: x & 0xFF, StartY: y & 0xFF}
	if !w.setMonster(g, class) {
		w.deleteMonster(g)
		return false
	}
	m := g.Mon
	m.MTX, m.MTY = x, y
	m.MoveRange = 0
	m.DieTime = w.now()
	m.RegenTime = 0
	m.Attribute = kalimaGateAttr
	w.kalima().gates[idx] = &kalimaGate{Owner: o.Index, Level: level, Created: w.now()}
	o.P.hasGate, o.P.gate = true, idx
	return true
}

// removeKalimaGate is FUN_004a3480: the owner's gate dies (C1 08 17 with
// no killer) and is deleted. The owner's flags are only cleared when the
// gate still exists.
func (w *World) removeKalimaGate(o *Object) bool {
	if o.P == nil || !o.P.hasGate {
		return false
	}
	idx := o.P.gate
	if idx < 0 || idx >= MaxObjects {
		return false
	}
	g := w.objects[idx]
	if g.State == StateEmpty || g.Mon == nil || g.Mon.Attribute != kalimaGateAttr {
		return false
	}
	w.deleteKalimaGate(g)
	o.P.hasGate, o.P.gate = false, -1
	return true
}

func (w *World) deleteKalimaGate(g *Object) {
	w.broadcast(g, packetDie(g.Index, 0, 0))
	delete(w.kalima().gates, g.Index)
	w.deleteMonster(g)
}

// kalimaTick is the Kalima part of the per-object second of FUN_00510810:
// gate objects (obj+0x37e == 6, FUN_004a2d10) and the Kundun life regen
// (class 0x113, FUN_004a3510).
func (w *World) kalimaTick() {
	ks := w.kalima()
	for i := 0; i < PlayerStart; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.Mon == nil {
			continue
		}
		if o.Class == kundunClass {
			w.kundunRegen(o)
		} else if o.Mon.Attribute == kalimaGateAttr {
			if g := ks.gates[i]; g != nil {
				w.kalimaGateSecond(o, g)
			}
		}
	}
}

// kundunRegen is FUN_004a3510: every 300 s a quarter of the maximum life
// comes back (the timer starts at 0, so the first second already counts).
func (w *World) kundunRegen(o *Object) {
	if !o.Alive {
		return
	}
	ks := w.kalima()
	if w.now().Sub(ks.regen[o.Index]) < kundunRegenTime {
		return
	}
	ks.regen[o.Index] = w.now()
	m := o.Mon
	m.Life = min(m.Life+m.MaxLife*0.25, m.MaxLife)
}

// kalimaGateSecond is FUN_004a2d10.
func (w *World) kalimaGateSecond(g *Object, kg *kalimaGate) {
	if !g.Alive || g.Live != LiveNormal {
		return
	}
	if kg.Owner < 0 || kg.Owner >= MaxObjects || !partyUsable(w.objects[kg.Owner]) {
		w.deleteKalimaGate(g)
		return
	}
	owner := w.objects[kg.Owner]
	age := w.now().Sub(kg.Created)
	if age < 2*time.Second {
		return
	}
	if _, dead := w.mw().deaths[owner.Index]; dead || kg.Count > 4 {
		w.removeKalimaGate(owner)
		return
	}
	if age >= kalimaGateLife {
		w.removeKalimaGate(owner)
		return
	}
	for _, e := range g.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index >= MaxObjects || e.Type != byte(TypePlayer) {
			continue
		}
		p := w.objects[e.Index]
		if !p.Alive || p.P == nil || !inRange(g, p.Map, p.X, p.Y, 2) {
			continue
		}
		if p.Index != owner.Index && !sameParty(p, owner) {
			continue
		}
		w.gateMove(p, kalimaGateMoves[kg.Level])
		c := kg.Count
		kg.Count++
		if c > 4 {
			return
		}
	}
}

// sameParty is the obj+0x2f0 test of FUN_004a2d10.
func sameParty(a, b *Object) bool {
	if a.P == nil || b.P == nil || a.P.Party == nil || b.P.Party == nil {
		return false
	}
	n := a.P.Party.Number
	return n >= 0 && n < MaxObjects && n == b.P.Party.Number
}
