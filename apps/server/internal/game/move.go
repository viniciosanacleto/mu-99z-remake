package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Movement: walking (D3, FUN_004d8b80), position fix (DF, FUN_004d80d0),
// actions (18, FUN_004d8300) and the 100 ms path walker (timer id 1,
// FUN_004c4480). The path arrays mirror obj+0x200 (X), obj+0x21e (Y) and
// obj+0x23c (direction bytes), 15 shorts each in the original.

// pathSlots is the size of the original path arrays (15) plus one spare
// slot: a client path of 15 steps writes index 15, which in the original
// aliases the next array (Y[0] / dir[0..1]); here it lands in the spare
// slot (see the report's open unknowns). The walker never uses it
// (PathCur < 15).
const pathSlots = 16

// PlayerMoveSpeed is the obj+0x374 value players get when loaded.
const PlayerMoveSpeed = 400

// moveState is the per-object movement, teleport and chat state owned by
// move.go, teleport.go, chat.go and command.go.
type moveState struct {
	PathCount    int             // obj+0x1d8
	PathCur      int             // obj+0x1dc
	PathStartEnd byte            // obj+0x1e0
	PathX, PathY [pathSlots]int  // obj+0x200 / obj+0x21e
	PathDir      [pathSlots]byte // obj+0x23c

	// OldX/OldY are obj+0x1ca/0x1cc: the cell that holds the 0x02
	// occupancy bit (the walk target, not the current step).
	OldX, OldY int

	LastWalk time.Time // obj+0x278: last accepted D3 (100 ms throttle)
	LastStep time.Time // obj+0x24c: last path step taken by the walker

	// MoveSpeed is obj+0x374 (players 400; monsters set it from
	// MonsterList through setMoveSpeed). Slow is obj+0x296 (+300 ms per
	// step, set by an effect not identified yet).
	MoveSpeed int
	Slow      bool

	// Flag270 is the byte obj+0x270: cleared by D3/FUN_00511cb0, set to 1
	// when a castle gate blocks a walk, and to obj+0x25c by actions
	// 0x80..0x82.
	Flag270 byte

	// CCPushTick is obj+0x438, written by the Chaos Castle knock-back
	// (FUN_00432a10 & co.); DF is refused for 1 s after it on maps 18..23.
	CCPushTick time.Time

	// Teleport state (teleport.go).
	TeleportState byte      // obj+0x284 (0 none, 1 magic teleport, 2, 3)
	TeleportTick  time.Time // obj+0x280
	Regen         byte      // obj+0x287 (gate move: 1 sent, 2 client ready, 3 shown)
	RegenMap      int       // obj+0x288
	RegenX        int       // obj+0x289
	RegenY        int       // obj+0x28a
	LastTeleport  time.Time // magic teleport skill delay (obj+0x600[6])

	// Chat state (chat.go / command.go).
	ChatSeconds  int               // obj+0x170: seconds into the current ChatLimitTime minute
	CommandTicks map[int]time.Time // obj+0xa00+index*4: last use of each command
	Authority    int               // obj+0x250 (see gmLevel)
}

// mv returns the movement state of o, creating it on first use.
func mv(o *Object) *moveState {
	if o.Move == nil {
		o.Move = &moveState{OldX: o.X, OldY: o.Y}
		if o.Type == TypePlayer {
			o.Move.MoveSpeed = PlayerMoveSpeed
		}
	}
	return o.Move
}

// setMoveSpeed sets obj+0x374 (monsters: MonsterList MoveSpeed).
func (w *World) setMoveSpeed(o *Object, ms int) { mv(o).MoveSpeed = ms }

// dirTable is DAT_005683d4: {dx, dy} for the 8 client directions.
var dirTable = [8][2]int{{-1, -1}, {0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}}

// dirStep returns the offsets of a direction nibble. Nibbles 8..15 read
// past the original table (DAT_005683f4..); they are treated as no
// movement.
func dirStep(d byte) (int, int) {
	if d < 8 {
		return dirTable[d][0], dirTable[d][1]
	}
	return 0, 0
}

func (w *World) hasEffect(o *Object, e int) bool {
	return w.hooks.moveEffect != nil && w.hooks.moveEffect(w, o, e)
}

// immobilized is FUN_0047a470 || FUN_0047a4e0: effects 61 (Stern), 72,
// 147, 178 and 57 (Ice Arrow), 65, 146 forbid walking.
func (w *World) immobilized(o *Object) bool {
	for _, e := range [...]int{0x3D, 0x48, 0x93, 0xB2, 0x39, 0x41, 0x92} {
		if w.hasEffect(o, e) {
			return true
		}
	}
	return false
}

// canPlayerAct is the common player precondition of D3/DF/18/1C: logout
// countdown <= 0 and not moving to another server (obj+0x964).
func canPlayerAct(o *Object) bool {
	return o.P != nil && o.P.LogoutCountdown <= 0 && !o.P.Moving
}

// clearPath empties the path (the memset sequences of FUN_004d8b80).
func (m *moveState) clearPath() {
	m.PathCount, m.PathCur, m.PathStartEnd = 0, 0, 0
	m.PathX, m.PathY = [pathSlots]int{}, [pathSlots]int{}
}

// stuckInWall is FUN_0050d120: when every cell of the 3x3 square starting
// at (X, Y) is blocked (0x04) or has no ground (0x08) the player is sent
// to the town gate of its class and the packet is ignored. Chaos Castle
// maps 18..23 in state 4 skip the check.
func (w *World) stuckInWall(o *Object) bool {
	if o.Map >= 18 && o.Map <= 23 && w.hooks.chaosCastleState != nil && w.hooks.chaosCastleState(w, o.Map-18) == 4 {
		return false
	}
	t := w.terrain(o.Map)
	for dx := 0; dx < 3; dx++ {
		for dy := 0; dy < 3; dy++ {
			a := t.Get(o.X+dx, o.Y+dy)
			if a&gamedata.AttrBlock == 0 && a&gamedata.AttrNoGround == 0 {
				return false
			}
		}
	}
	gate := -1
	switch o.Class {
	case 0, 1, 3, 4, 6:
		gate = 0x11
	case 2:
		gate = 0x1B
	case 5:
		gate = 0x10B
	}
	if gate >= 0 {
		w.gateMove(o, gate)
	}
	return true
}

// packetWalk is C1 08 D3 idxHi idxLo TX TY dir<<4 (FUN_004d8b80).
func packetWalk(index, tx, ty, dir int) []byte {
	p := newPacket(frame.C1, 0xD3, -1, 8)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6] = byte(tx), byte(ty)
	p[7] = byte(dir) << 4
	return p
}

// packetPosition is C1 07 DF idxHi idxLo X Y (FUN_004d80d0).
func packetPosition(index, x, y int) []byte {
	p := newPacket(frame.C1, 0xDF, -1, 7)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6] = byte(x), byte(y)
	return p
}

// packetAction is C1 09 18 idxHi idxLo dir action targetHi targetLo
// (FUN_004d8300; the two target bytes are copied from the request).
func packetAction(index int, dir, action, t0, t1 byte) []byte {
	p := newPacket(frame.C1, 0x18, -1, 9)
	p[3], p[4] = byte(index>>8), byte(index)
	p[5], p[6], p[7], p[8] = dir, action, t0, t1
	return p
}

// sendToViewers is the loop shared by D3/DF/18: VP2 entries of type 1
// whose state is not 0, 8 or 0x10 (only 0 can occur with this viewport).
func (w *World) sendToViewers(o *Object, p []byte) { w.broadcast(o, p) }

// handleWalk is D3 (FUN_004d8b80): [3] X [4] Y [5] dir<<4|count
// [6..] direction nibbles, high nibble first.
func (w *World) handleWalk(o *Object, pkt []byte) {
	if len(pkt) < 6 || o.State != StatePlaying {
		return
	}
	if o.Type == TypePlayer && !canPlayerAct(o) {
		return
	}
	m := mv(o)
	if m.Regen > 0 || m.TeleportState != 0 {
		return
	}
	now := w.now()
	if now.Sub(m.LastWalk) < 100*time.Millisecond {
		return
	}
	if o.Type == TypePlayer && w.stuckInWall(o) {
		return
	}
	if w.immobilized(o) {
		return
	}
	w.summonCancel(o) // obj+0x5d2: walking cancels a Summon Party (skill_guild.go)
	m.LastWalk = now
	sx, sy := int(pkt[3]), int(pkt[4])
	o.Dir = int(pkt[5] >> 4)
	count := int(pkt[5] & 0x0F)
	var dirs []byte
	for k := 1; k <= count; k++ {
		i := 5 + (k+1)/2
		var b byte
		if i < len(pkt) {
			b = pkt[i]
		}
		if k%2 == 1 {
			dirs = append(dirs, b>>4)
		} else {
			dirs = append(dirs, b&0x0F)
		}
	}
	w.walk(o, sx, sy, dirs)
}

// walk builds the path and validates/applies it (the body of
// FUN_004d8b80 after the preconditions); hooks.setPath uses it for every
// object type.
func (w *World) walk(o *Object, sx, sy int, dirs []byte) bool {
	m := mv(o)
	m.Flag270 = 0
	m.clearPath()
	o.TX, o.TY = sx, sy
	n := len(dirs)
	if n > 0 {
		m.PathCur = 1
		m.PathCount = n + 1
	}
	m.PathStartEnd = 1
	m.PathX[0], m.PathY[0] = sx, sy
	m.PathDir[0] = byte(o.Dir)
	for k := 1; k <= n && k < pathSlots; k++ {
		d := dirs[k-1]
		dx, dy := dirStep(d)
		m.PathX[k] = m.PathX[k-1] + dx
		m.PathY[k] = m.PathY[k-1] + dy
		o.TX, o.TY = m.PathX[k], m.PathY[k]
		m.PathDir[k] = d
	}
	reject := func() bool {
		m.clearPath()
		w.resendPosition(o, o.X, o.Y)
		return false
	}
	if o.TX < o.X-15 || o.TX > o.X+15 || o.TY < o.Y-15 || o.TY > o.Y+15 {
		return reject()
	}
	t := w.terrain(o.Map)
	if m.PathCount > 0 && t.Get(o.TX, o.TY)&(gamedata.AttrBlock|gamedata.AttrNoGround) != 0 {
		return reject()
	}
	if o.Type == TypePlayer && o.Map == 30 {
		// Castle gates: step from (X, Y) towards (TX, TY) one cell per
		// axis, at most 256 cells, looking for the 0x10 attribute.
		x, y := o.X, o.Y
		for i := 0; i < 0x100; i++ {
			x, y = stepToward(x, o.TX), stepToward(y, o.TY)
			if t.Get(x, y)&gamedata.AttrWall != 0 {
				// obj+0x270 = 1 is overwritten with 0 by FUN_00511cb0
				// right away (same order as the original).
				m.PathCount, m.PathCur, m.PathStartEnd = 0, 0, 0
				m.Flag270 = 1
				w.resendPosition(o, o.X, o.Y)
				w.send(o, packetAction(o.Index, byte(o.Dir), 0x7A, 0, 0))
				return false
			}
			if x == o.TX && y == o.TY {
				break
			}
		}
	}
	w.setOccupancy(o, m.OldX, m.OldY, false)
	o.X, o.Y = sx, sy
	m.OldX, m.OldY = o.TX, o.TY
	o.ActionState = 0
	w.setOccupancy(o, m.OldX, m.OldY, true)
	p := packetWalk(o.Index, o.TX, o.TY, o.Dir)
	if o.Type == TypePlayer {
		w.send(o, p)
	}
	w.sendToViewers(o, p)
	return true
}

func stepToward(v, target int) int {
	if v > target {
		return v - 1
	}
	if v < target {
		return v + 1
	}
	return v
}

// setOccupancy sets or clears the 0x02 attribute of one cell.
func (w *World) setOccupancy(o *Object, x, y int, on bool) {
	t := w.terrain(o.Map)
	a := t.Get(x, y)
	if x < 0 || y < 0 || t == nil || x >= t.Width || y >= t.Height {
		return
	}
	if on {
		t.Set(x, y, a|gamedata.AttrCharacter)
	} else {
		t.Set(x, y, a&^gamedata.AttrCharacter)
	}
}

// resendPosition is FUN_00511cb0: move the occupancy bit from the old
// cell to (x, y), make it the old position and run the DF handler with a
// synthesized "C1 05 DF x y" request (which validates and broadcasts).
func (w *World) resendPosition(o *Object, x, y int) {
	m := mv(o)
	m.Flag270 = 0
	w.setOccupancy(o, m.OldX, m.OldY, false)
	w.setOccupancy(o, x, y, true)
	m.OldX, m.OldY = x, y
	w.handlePosition(o, []byte{0xC1, 0x05, 0xDF, byte(x), byte(y)})
}

// handlePosition is DF (FUN_004d80d0): [3] X [4] Y.
func (w *World) handlePosition(o *Object, pkt []byte) {
	if len(pkt) < 5 || o.State != StatePlaying {
		return
	}
	if o.Type == TypePlayer && !canPlayerAct(o) {
		return
	}
	m := mv(o)
	if m.TeleportState != 0 {
		return
	}
	if o.Type == TypePlayer && w.stuckInWall(o) {
		return
	}
	if w.hooks.windowClose != nil { // obj+0x24a8 = 0 (windowtime.go)
		w.hooks.windowClose(w, o)
	}
	if o.Map >= 18 && o.Map <= 23 && w.now().Sub(m.CCPushTick) < time.Second {
		return
	}
	x, y := int(pkt[3]), int(pkt[4])
	if x < o.X-15 || x > o.X+15 || y < o.Y-15 || y > o.Y+15 {
		return
	}
	w.setOccupancy(o, m.OldX, m.OldY, false)
	o.X, o.Y, o.TX, o.TY = x, y, x, y
	m.OldX, m.OldY = x, y
	w.setOccupancy(o, x, y, true)
	p := packetPosition(o.Index, o.TX, o.TY)
	if o.Type == TypePlayer {
		w.send(o, p)
	}
	w.sendToViewers(o, p)
}

// handleAction is 18 (FUN_004d8300): [3] dir [4] action [5..6] target.
// Actions 0x80/0x81/0x82 (sit/pose/heal-like) set obj+0x271 to 2/3/4.
// The original reads [5]/[6] without a size check; missing bytes read 0.
func (w *World) handleAction(o *Object, pkt []byte) {
	if len(pkt) < 5 || !o.IsPlayer() || !canPlayerAct(o) {
		return
	}
	var t0, t1 byte
	if len(pkt) > 5 {
		t0 = pkt[5]
	}
	if len(pkt) > 6 {
		t1 = pkt[6]
	}
	m := mv(o)
	o.Dir = int(pkt[3])
	action := pkt[4]
	o.PoseState = action
	switch action {
	case 0x80, 0x81, 0x82:
		o.ActionState = action - 0x80 + 2
		m.Flag270 = o.PoseState
	}
	w.sendToViewers(o, packetAction(o.Index, pkt[3], action, t0, t1))
}

// noWalkMonster lists the monster classes FUN_004c4480 never moves
// (castle/event statues and gates).
func noWalkMonster(class int) bool {
	switch class {
	case 0x83, 0x84, 0x85, 0x86, 0xCC, 0xCD, 0xCE, 0xCF, 0xD0, 0xD1,
		0xD8, 0xD9, 0xDA, 0xDB, 0x115, 0x116, 0x11B, 0x120, 0x20C, 0x20D, 0x20F, 0x210:
		return true
	}
	return false
}

// stepDelay is the per-step delay of FUN_004c4480: (Slow ? 300 : 0) +
// MoveSpeed, times 1.3 truncated (FISTP with RC=chop) for even (diagonal)
// directions.
func stepDelay(dir byte, moveSpeed int, slow bool) time.Duration {
	base := moveSpeed
	if slow {
		base += 300
	}
	if dir%2 == 0 {
		base = int(float64(base) * 1.3)
	}
	return time.Duration(base) * time.Millisecond
}

// moveTick is FUN_004c4480 (timer id 1, every 100 ms).
func (w *World) moveTick() {
	now := w.now()
	for _, o := range w.objects {
		if o.State != StatePlaying || o.Live != LiveNormal || o.Move == nil || o.Move.PathCount == 0 {
			continue
		}
		if w.immobilized(o) {
			continue
		}
		if o.Type == TypeMonster && noWalkMonster(o.Class) {
			// TODO(move): obj+0x37e == 6 also excludes the monster; the
			// field is owned by the monster module and not identified.
			continue
		}
		m := o.Move
		if m.PathCur >= pathSlots {
			continue
		}
		if now.Sub(m.LastStep) <= stepDelay(m.PathDir[m.PathCur], m.MoveSpeed, m.Slow) || m.PathCur >= 15 {
			continue
		}
		k := m.PathCur
		if w.terrain(o.Map).Get(m.PathX[k], m.PathY[k])&(gamedata.AttrBlock|gamedata.AttrNoGround) != 0 {
			m.PathCur, m.PathCount = 0, 0
			m.LastStep = now
			if o.Type != TypePlayer {
				m.PathStartEnd = 0
			}
			m.PathX, m.PathY = [pathSlots]int{}, [pathSlots]int{}
			w.resendPosition(o, o.X, o.Y)
			continue
		}
		o.X, o.Y, o.Dir = m.PathX[k], m.PathY[k], int(m.PathDir[k])
		m.PathCur++
		m.LastStep = now
		if m.PathCur >= m.PathCount {
			m.PathCur, m.PathCount = 0, 0
			if o.Type != TypePlayer {
				m.PathStartEnd = 0
			}
		}
	}
}

func registerMove(h *hooks) {
	h.worldHandlers[0xD3] = (*World).handleWalk
	h.worldHandlers[0xDF] = (*World).handlePosition
	h.worldHandlers[0x18] = (*World).handleAction
	h.tick100 = append(h.tick100, (*World).moveTick)
	h.setPath = func(w *World, o *Object, sx, sy int, dirs []byte) { w.walk(o, sx, sy, dirs) }
	h.enterWorld = append(h.enterWorld, func(w *World, o *Object) {
		o.Move = nil
		m := mv(o)
		m.OldX, m.OldY = o.X, o.Y
		m.Authority = w.gmLevel(o)
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		if m := o.Move; m != nil {
			w.setOccupancy(o, m.OldX, m.OldY, false)
			m.clearPath()
		}
	})
}
