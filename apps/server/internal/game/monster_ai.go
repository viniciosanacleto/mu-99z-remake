package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Monster AI (timer id 0, every 100 ms, FUN_004c48b0): the delayed message
// queue (FUN_004c3700 -> FUN_004b48f0) and the action step FUN_004b4090
// with the base AI FUN_004b44a0.

// Emotion values of obj+0x25a.
const (
	emotionNone   = 0
	emotionAttack = 1
	emotionFlee   = 2
	emotionRest   = 3
)

// Message codes (FUN_00510550 senders, FUN_004b48f0 jump table
// DAT_004b4ab8: 0->0, 1->1, 2->2, 3->3, 4->4, 6->5, 7->6, 0x37->7,
// 0x38->8, 0x39->9).
const (
	msgAttacked  = 0 // sent to the target 100 ms after every hit
	msgDie       = 1 // sent to the dead monster 500 ms after death (drop)
	msgResetAI   = 3
	msgRestShort = 4
)

// pathDirs is DAT_005683d4 / the PATH direction table (FUN_004af420).
var pathDirs = [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}

// dirTo is FUN_005122d0 (GetPathPacketDirPos): direction of (dx, dy).
func dirTo(dx, dy int) int {
	switch {
	case dx < 0 && dy < 0:
		return 0
	case dx < 0 && dy == 0:
		return 7
	case dx < 0:
		return 6
	case dx == 0 && dy < 0:
		return 1
	case dx == 0 && dy > 0:
		return 5
	case dx > 0 && dy < 0:
		return 2
	case dx > 0 && dy == 0:
		return 3
	case dx > 0:
		return 4
	}
	return 0
}

// distance is FUN_0050c750: (int)sqrt(dx*dx + dy*dy) in float.
func distance(a, b *Object) int {
	dx := float32(a.X - b.X)
	dy := float32(a.Y - b.Y)
	return int(float32(math.Sqrt(float64(dx*dx + dy*dy))))
}

// sendMsg is FUN_00510550: queue a message on o (20 slots, dropped when
// full).
func (w *World) sendMsg(o *Object, code, sender, delayMs, param int) {
	if o.Mon == nil {
		return
	}
	for i := range o.Mon.Msgs {
		if !o.Mon.Msgs[i].Used {
			o.Mon.Msgs[i] = monsterMsg{Code: code, Sender: sender, Due: w.now().Add(time.Duration(delayMs) * time.Millisecond),
				Param: param, Used: true}
			return
		}
	}
}

// monsterTick is the monster part of FUN_004c48b0.
func (w *World) monsterTick() {
	for i := 0; i < PlayerStart; i++ { // monsters, NPCs and summons
		o := w.objects[i]
		if o.State != StatePlaying || o.Mon == nil || (o.Type != TypeMonster && o.Type != TypeNPC) {
			continue
		}
		w.monsterMessages(o)
		if o.State == StatePlaying && o.Mon != nil {
			w.monsterProcess(o)
		}
	}
}

// monsterMessages is FUN_004c3700 for monsters.
func (w *World) monsterMessages(o *Object) {
	now := w.now()
	for i := range o.Mon.Msgs {
		msg := o.Mon.Msgs[i]
		if !msg.Used || !now.After(msg.Due) {
			continue
		}
		o.Mon.Msgs[i] = monsterMsg{}
		w.monsterMessage(o, msg)
		if o.Mon == nil {
			return
		}
	}
}

// monsterMessage is FUN_004b48f0 (the cases this module implements).
func (w *World) monsterMessage(o *Object, msg monsterMsg) {
	m := o.Mon
	switch msg.Code {
	case msgAttacked:
		if m.Attribute != 0 {
			w.monsterAttacked(o, msg.Sender)
		}
	case msgDie:
		w.monsterDie(o, msg.Sender)
	case msgResetAI:
		m.Target, m.LastAttacker = -1, -1
		m.Delay = 1000
		m.ActAttack, m.ActMove = false, false
		m.Emotion = 0
	case msgRestShort:
		m.Emotion, m.EmotionCount = emotionRest, 1
	case msgPushBack, msgKnockBack2, msgKnockBack3:
		w.monsterPushMessage(o, msg.Code, msg.Sender) // pushback.go
	}
}

// monsterAttacked is FUN_004b85a0: react to a hit by sender.
func (w *World) monsterAttacked(o *Object, sender int) {
	m := o.Mon
	if sender < 0 || sender >= MaxObjects || m.Target == sender {
		return
	}
	a := w.objects[sender]
	if !a.Alive || a.Live != LiveNormal || !w.checkWall(o, a) || w.cellHas(a, gamedata.AttrSafeZone) {
		return
	}
	switch m.Emotion {
	case emotionNone:
		m.Emotion, m.EmotionCount = emotionAttack, 10
	case emotionAttack:
		m.EmotionCount = 10
	}
	if !m.ActAttack {
		r := m.attackReach()
		if distance(o, a) <= r {
			// 10% of the hits are ignored unless the attacker is a
			// summon (attacker obj+0x37e == 4).
			if (a.Mon == nil || a.Mon.Attribute != 4) && rnd(100) > 0x59 {
				return
			}
			m.Target = sender
			return
		}
		if m.Target < 0 || m.Target >= MaxObjects {
			m.Target = sender
		}
		return
	}
	if monsterRand()&1 == 1 {
		m.ActAttack = true
		m.Target = sender
	}
}

// attackReach is AttackRange (+2 when AttackType >= 100).
func (m *monsterState) attackReach() int {
	r := m.AttackRange
	if m.AttackType > 99 {
		r += 2
	}
	return r
}

// cellHas is FUN_004ad4d0 on the object's cell.
func (w *World) cellHas(o *Object, attr byte) bool {
	t := w.terrain(o.Map)
	if t == nil || o.X < 0 || o.Y < 0 || o.X >= t.Width || o.Y >= t.Height {
		return attr&gamedata.AttrBlock != 0
	}
	return t.Get(o.X, o.Y)&attr != 0
}

// attrAt returns the attribute of a cell (out of the map reads 0x04 like
// the getters of the original).
func (w *World) attrAt(mp, x, y int) byte {
	t := w.terrain(mp)
	if t == nil || x < 0 || y < 0 || x >= t.Width || y >= t.Height {
		return gamedata.AttrBlock
	}
	return t.Get(x, y)
}

// checkWall is FUN_004adf20: Bresenham line from a to b failing on any
// cell with 0x04 or 0x10.
func (w *World) checkWall(a, b *Object) bool {
	t := w.terrain(a.Map)
	if t == nil {
		return false
	}
	x, y := a.X, a.Y
	dx, dy := b.X-a.X, b.Y-a.Y
	sx, sy := 1, 1
	if dx < 0 {
		dx, sx = -dx, -1
	}
	if dy < 0 {
		dy, sy = -dy, -1
	}
	// The major axis advances every step; the minor one when the error
	// passes half of the major length.
	major, minor := dy, dx
	stepMajX, stepMajY, stepMinX, stepMinY := 0, sy, sx, 0
	if dy < dx {
		major, minor = dx, dy
		stepMajX, stepMajY, stepMinX, stepMinY = sx, 0, 0, sy
	}
	e := 0
	for i := 0; i <= major; i++ {
		a := w.attrAt(a.Map, x&0xFF, y&0xFF)
		if a&(gamedata.AttrBlock|gamedata.AttrWall) != 0 {
			return false
		}
		e += minor
		if major/2 < e {
			x, y = x+stepMinX, y+stepMinY
			e -= major
		}
		x, y = x+stepMajX, y+stepMajY
	}
	return true
}

// monsterProcess is FUN_004b4090.
func (w *World) monsterProcess(o *Object) {
	m := o.Mon
	if !o.Alive || o.Live != LiveNormal || m.Die != 0 {
		return
	}
	now := w.now()
	if now.Sub(m.LastAct) < time.Duration(m.Delay)*time.Millisecond {
		return
	}
	m.LastAct = now
	if m.Attribute == 6 {
		return
	}
	switch o.Class {
	case 0x83, 0x84, 0x85, 0x86, 0x116, 0x11B, 0x120:
		return
	case soccerBallClass:
		w.soccerGoalCheck(o) // FUN_00411640 (guild_soccer.go)
		return
	}
	if isTrap(o.Class) {
		w.trapAct(o) // FUN_004b5be0
	} else {
		w.monsterBaseAct(o)
	}
	if w.summonKeepUp(o) {
		return
	}
	if m.ActMove {
		w.monsterMoveAction(o)
		m.ActMove = false
		return
	}
	if !m.ActAttack || o.Class == 0x1CB {
		return
	}
	if w.hasMonsterSkill(o) {
		// CMonsterSkillManager (FUN_004bdcd0): rand % 4 == 0 -> D7,
		// otherwise a skill unit (FUN_004bde50).
		if t := w.validTarget(m.Target); t != nil {
			o.Dir = dirTo(t.X-o.X, t.Y-o.Y)
			if monsterRand()%4 == 0 {
				w.monsterD7(o, t)
			} else {
				w.useMonsterSkill(o, t)
			}
			m.ActAttack = false
			return
		}
	} else if t := w.validTarget(m.Target); t != nil {
		o.Dir = dirTo(t.X-o.X, t.Y-o.Y)
		if m.AttackType == 50 || (m.AttackType > 99 && rnd(5) == 0) {
			w.monsterAreaAttack(o) // FUN_004b7a20
		} else {
			w.monsterAttackAction(o, t) // FUN_004b7d00
		}
		m.ActAttack = false
		return
	}
	m.Emotion = 0
	m.ActAttack, m.ActMove = false, false
	m.Delay = 1000
	m.LastAttacker, m.Target = -1, -1
}

// validTarget is the target check of FUN_004b4090: playing, no logout
// countdown, alive and not teleporting.
func (w *World) validTarget(idx int) *Object {
	if idx < 0 || idx >= MaxObjects {
		return nil
	}
	t := w.objects[idx]
	if t.State != StatePlaying || !t.Alive || t.Teleporting {
		return nil
	}
	if t.P != nil && t.P.LogoutCountdown > 0 {
		return nil
	}
	return t
}

// moving reports a walk in progress (obj+0x1e0).
func moving(o *Object) bool { return o.Move != nil && o.Move.PathStartEnd != 0 }

// monsterBaseAct is FUN_004b44a0.
func (w *World) monsterBaseAct(o *Object) {
	m := o.Mon
	if m.Target < 0 || m.Target >= MaxObjects {
		m.Emotion = emotionNone
	}
	if w.hasEffect(o, 0x48) { // effect 72 freezes the AI
		return
	}
	switch m.Emotion {
	case emotionNone:
		if m.Attribute == 0 {
			return
		}
		if m.ActAttack {
			m.ActAttack = false
			m.Target = -1
			m.Delay = 500
		}
		if m.Attribute == 4 || monsterRand()&1 != 0 {
			if m.MoveRange > 0 && o.Alive && w.canMonsterMove(o) {
				if m.Attribute == 4 {
					w.summonFollow(o) // FUN_004b6370
				} else {
					w.monsterRandomMove(o)
				}
			}
		} else {
			m.Delay = 500
		}
		w.monsterSearchEnemy(o)
		if m.Target >= 0 && m.Target < MaxObjects {
			m.Emotion, m.EmotionCount = emotionAttack, 30
		}
	case emotionAttack:
		w.countEmotion(m)
		if m.Target < 0 || m.Target >= MaxObjects || moving(o) {
			return
		}
		t := w.objects[m.Target]
		if distance(o, t) <= m.attackReach() {
			if !w.checkWall(o, t) {
				m.Target = -1
				m.ActAttack = false
				m.Delay = 500
				m.ActMove, m.Emotion = true, 0
				return
			}
			if !w.cellHas(t, gamedata.AttrSafeZone) {
				m.ActAttack = true
			} else {
				m.Target = -1
				m.Emotion, m.EmotionCount = emotionAttack, 30
			}
			o.Dir = dirTo(t.X-o.X, t.Y-o.Y)
			m.Delay = m.AttackSpeed
			return
		}
		if !w.monsterTargetPos(o) {
			w.monsterRandomMove(o)
			return
		}
		if !w.checkWall(o, t) {
			w.monsterRandomMove(o)
			m.Emotion, m.EmotionCount = emotionRest, 10
			return
		}
		m.ActMove = true
		m.Delay = 400
		o.Dir = dirTo(t.X-o.X, t.Y-o.Y)
	case emotionFlee:
		w.countEmotion(m)
		m.ActMove = true
		m.Delay = 800
		if m.Target >= 0 && m.Target < MaxObjects {
			t := w.objects[m.Target]
			d := dirTo(t.X-o.X, t.Y-o.Y)
			m.MTX -= pathDirs[d][0] * 3
			m.MTY -= pathDirs[d][1] * 3
		}
	case emotionRest:
		w.countEmotion(m)
		m.ActAttack, m.ActMove = false, false
		m.Delay = 400
	}
}

func (w *World) countEmotion(m *monsterState) {
	if m.EmotionCount == 0 {
		m.Emotion = emotionNone
	} else {
		m.EmotionCount--
	}
}

// canMonsterMove is FUN_004b6a80: not regenerating, not teleporting, live
// state 2 and no immobilizing effect.
func (w *World) canMonsterMove(o *Object) bool {
	return !o.Teleporting && o.Live == LiveNormal && !w.immobilized(o)
}

// monsterSearchEnemy is FUN_004b59e0: random choice among the objects
// that see the monster (VP2) and are inside its ViewRange. Guards
// (classes 0xF7/0xF9) only pick PK level > 4 players.
func (w *World) monsterSearchEnemy(o *Object) {
	if w.hooks.monsterSearch != nil && w.hooks.monsterSearch(w, o) { // mercenaries (FUN_004b2ec0)
		return
	}
	m := o.Mon
	var cand []int
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.State != StatePlaying || !t.Alive {
			continue
		}
		if !inRange(t, o.Map, o.X, o.Y, m.ViewRange) {
			continue
		}
		if m.Attribute == 4 && e.Type == byte(TypePlayer) {
			continue
		}
		if o.Class == 0xF7 || o.Class == 0xF9 {
			if t.PKLevel > 4 {
				cand = append(cand, e.Index)
			}
			continue
		}
		cand = append(cand, e.Index)
	}
	if len(cand) == 0 {
		m.Target = -1
		return
	}
	m.Target = cand[rnd(len(cand))]
}

// moveCheck is FUN_004b68e0: the cell must stay within the spawn radius
// (spawn record Dis) or MoveRange from the start position.
func (w *World) moveCheck(o *Object, tx, ty int) bool {
	m := o.Mon
	radius := m.MoveRange
	if m.SpawnIndex >= 0 && m.SpawnIndex < len(w.mw().spawns) {
		radius = w.mw().spawns[m.SpawnIndex].Dis
	}
	if radius == 0 {
		return false
	}
	dx, dy := float64(m.StartX-tx), float64(m.StartY-ty)
	return float32(math.Sqrt(dx*dx+dy*dy)) <= float32(radius)
}

// monsterRandomMove is FUN_004b5db0: 10 tries of a random cell within
// +-MoveRange.
func (w *World) monsterRandomMove(o *Object) {
	m := o.Mon
	if !o.Alive || o.Teleporting || o.Live != LiveNormal || w.immobilized(o) {
		return
	}
	m.Delay = 1000
	for try := 0; try < 10; try++ {
		tx := monsterRand()%(m.MoveRange*2+1) + o.X - m.MoveRange
		ty := monsterRand()%(m.MoveRange*2+1) - m.MoveRange + o.Y
		if m.Emotion != emotionAttack && !w.moveCheck(o, tx, ty) {
			continue
		}
		a := w.attrAt(o.Map, tx, ty)
		if o.Class == 0xF7 || o.Class == 0xF9 {
			if a&(gamedata.AttrCharacter|gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
				m.MTX, m.MTY = tx, ty
				m.ActMove = true
				m.Delay = 3000
				return
			}
			continue
		}
		if a&(gamedata.AttrSafeZone|gamedata.AttrCharacter|gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
			m.MTX, m.MTY = tx, ty
			m.ActMove, m.Emotion = true, 0
			m.Delay = 500
			m.Target = -1
			m.ActAttack = false
			return
		}
	}
}

// monsterTargetPos is FUN_004b60a0: a cell at attack distance from the
// target, or one of the 8 cells around it.
func (w *World) monsterTargetPos(o *Object) bool {
	m := o.Mon
	if !o.Alive || o.Teleporting || o.Live != LiveNormal || w.immobilized(o) {
		return false
	}
	if m.Target < 0 || m.Target >= MaxObjects {
		return false
	}
	t := w.objects[m.Target]
	if t.Teleporting || vpFind(&o.VP, t.Index, byte(t.Type)) < 0 && vpFind(&o.VP2, t.Index, byte(t.Type)) < 0 {
		return false
	}
	reach := m.attackReach()
	tpx, tpy := t.X, t.Y
	if o.X < t.X {
		tpx -= reach
	}
	if o.X > t.X {
		tpx += reach
	}
	if o.Y < t.Y {
		tpy -= reach
	}
	if o.Y > t.Y {
		tpy += reach
	}
	free := func(x, y int) bool {
		a := w.attrAt(o.Map, x, y)
		return (m.Emotion == emotionAttack || w.moveCheck(o, x, y)) &&
			a&(gamedata.AttrSafeZone|gamedata.AttrCharacter|gamedata.AttrBlock|gamedata.AttrNoGround) == 0
	}
	if w.attrAt(o.Map, tpx, tpy)&(gamedata.AttrCharacter|gamedata.AttrBlock|gamedata.AttrNoGround) != 0 {
		d := dirTo(t.X-tpx, t.Y-tpy)
		for i := 0; i < 8; i++ {
			x, y := t.X+pathDirs[d][0], t.Y+pathDirs[d][1]
			if free(x, y) {
				m.MTX, m.MTY = x, y
				return true
			}
			d = (d + 1) % 8
		}
		return false
	}
	if free(tpx, tpy) {
		m.MTX, m.MTY = tpx, tpy
		return true
	}
	return false
}

// monsterMoveAction is FUN_004b65b0: find a path to (MTX, MTY) and walk it
// through the D3 code (hooks.setPath).
func (w *World) monsterMoveAction(o *Object) {
	if w.immobilized(o) {
		return
	}
	m := o.Mon
	path := w.findPath(o.Map, o.X, o.Y, m.MTX, m.MTY, true)
	if path == nil {
		path = w.findPath(o.Map, o.X, o.Y, m.MTX, m.MTY, false)
	}
	if len(path) < 2 {
		return
	}
	dirs := make([]byte, 0, len(path)-1)
	for i := 1; i < len(path); i++ {
		dirs = append(dirs, byte(dirTo(path[i][0]-path[i-1][0], path[i][1]-path[i-1][1])))
	}
	if w.hooks.setPath != nil {
		w.hooks.setPath(w, o, o.X, o.Y, dirs)
		return
	}
	// Without the movement module: jump to the end of the path and
	// broadcast the D3 the original sends (C1 08 D3).
	w.monsterOccupy(o, false)
	last := path[len(path)-1]
	o.X, o.Y, o.TX, o.TY = last[0], last[1], last[0], last[1]
	w.monsterOccupy(o, true)
	p := packet.New(0xC1, 0xD3, -1, 8)
	p[3], p[4] = byte(o.Index>>8), byte(o.Index)
	p[5], p[6], p[7] = byte(o.TX), byte(o.TY), byte(o.Dir)<<4
	w.broadcast(o, p)
}

// findPath is the PATH class of the original (FUN_004afa70 with the step
// chooser FUN_004af4a0): a greedy walk of at most 15 nodes towards the
// target with back-tracking. Cells are passable when their attribute is
// below 2. errorCheck enables the side steps and back-tracking of the
// first attempt (FUN_004adfd0 calls it with 1, then 0).
func (w *World) findPath(mp, sx, sy, tx, ty int, errorCheck bool) [][2]int {
	t := w.terrain(mp)
	if t == nil {
		return nil
	}
	const maxPath = 15
	hit := map[int]bool{}
	var path [][2]int
	x, y := sx, sy
	lastDir, fails := 0, 0
	first := true
	passable := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < t.Width && y < t.Height
	}
	tryDir := func(d, x, y int) (bool, bool) { // ok, inBounds
		nx, ny := x+pathDirs[d][0], y+pathDirs[d][1]
		if !passable(nx, ny) {
			return false, false
		}
		if t.Get(nx, ny) < 2 && !hit[ny*t.Width+nx] {
			return true, true
		}
		hit[ny*t.Width+nx] = true
		return false, true
	}
	loopCheck := func(x, y int) {
		for i, p := range path {
			if p[0] == x && p[1] == y {
				hit[y*t.Width+x] = true
				path = path[:i]
				return
			}
		}
	}
	next := func() int {
		best, bestDir := 10000000, 0
		eval := func(d int) {
			d &= 7
			nx, ny := x+pathDirs[d][0], y+pathDirs[d][1]
			dist := (ty-ny)*(ty-ny) + (tx-nx)*(tx-nx)
			if dist < best && passable(nx, ny) && !hit[ny*t.Width+nx] {
				best, bestDir = dist, d
			}
		}
		if first {
			for d := 0; d < 8; d++ {
				eval(d)
			}
		} else {
			for d := lastDir + 7; d <= lastDir+9; d++ {
				eval(d)
			}
			if best == 10000000 {
				for d := lastDir + 2; d <= lastDir+6; d++ {
					eval(d)
				}
			}
		}
		if lastDir >= 0 {
			l := pathDirs[lastDir&7][0]*pathDirs[lastDir&7][0] + pathDirs[lastDir&7][1]*pathDirs[lastDir&7][1]
			if best-6 < l && l < best+6 && best > 0x18 {
				bestDir = lastDir & 7
			}
		}
		if best == 10000000 {
			return -1
		}
		if ok, _ := tryDir(bestDir, x, y); ok {
			loopCheck(x, y)
			return bestDir
		}
		if errorCheck {
			for _, d := range []int{(bestDir + 3) & 7, (bestDir + 5) & 7} {
				if ok, _ := tryDir(d, x, y); ok {
					loopCheck(x, y)
					return d
				}
			}
		}
		hit[y*t.Width+x] = true
		return -1
	}
	for {
		if x == tx && y == ty {
			path = append(path, [2]int{x, y})
			if len(path) < maxPath {
				return path
			}
			return nil
		}
		d := next()
		first = false
		lastDir = d
		if d < 0 {
			if !errorCheck {
				path = append(path, [2]int{x, y})
				if len(path) < maxPath {
					return path
				}
				return nil
			}
			if len(path) == 0 {
				return nil
			}
			p := path[len(path)-1]
			path = path[:len(path)-1]
			x, y = p[0], p[1]
			fails++
			if fails >= 10 {
				return nil
			}
			lastDir = 0
			continue
		}
		path = append(path, [2]int{x, y})
		x, y = x+pathDirs[d][0], y+pathDirs[d][1]
		if len(path) >= maxPath {
			return nil
		}
	}
}
