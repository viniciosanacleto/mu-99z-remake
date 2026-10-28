package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Gates and teleport (CMove / CGate): the 1C request (FUN_004bf010), the
// gate move gObjMoveGate (FUN_0050f6d0), the Teleport magic
// (FUN_0050e180) and the per-second teleport state machine (FUN_004c37f0
// before the viewport cycle, FUN_004c3b10 after it).

// teleportSkill is the Teleport magic index (SkillList 6).
const teleportSkill = 6

// costRate stands for obj+0x8b0 / obj+0x8b4 (mana / BP cost percent).
const costRate = 100

// packetTeleport is C3 08 1C flag map X Y dir (FUN_004bf2f0: flag is 1
// when the requested gate is > 0, else the gate byte, i.e. 0; FUN_004bf340
// and the magic teleport end send flag 0).
func packetTeleport(gate int, mp, x, y, dir int) []byte {
	p := newPacket(frame.C3, 0x1C, -1, 8)
	p[3] = 1
	if gate <= 0 {
		p[3] = byte(gate)
	}
	p[4], p[5], p[6], p[7] = byte(mp), byte(x), byte(y), byte(dir)
	return p
}

// packetCastleStateNotice is C1 05 B2 17 v (FUN_004c5220), sent after
// every teleport; v is 1 only on map 30 during the Castle Siege battle
// (state 7), which the castle module reports through hooks.castleBattle.
func packetCastleStateNotice(v byte) []byte {
	p := newPacket(frame.C1, 0xB2, 0x17, 5)
	p[4] = v
	return p
}

// packetMapEventState is C1 05 0B 00 00: FUN_0048a690 sends it when no
// map event (20 slots of 0xcec bytes at obj+0x810..) is active for the
// map; the event modules are not implemented yet.
func packetMapEventState() []byte { return []byte{0xC1, 0x05, 0x0B, 0x00, 0x00} }

// packetMagicUse is C3 08 19 skill idxHi idxLo targetHi|ok<<7 targetLo
// (FUN_00501570).
func packetMagicUse(skill, index, target int, ok bool) []byte {
	p := newPacket(frame.C3, 0x19, -1, 8)
	p[3] = byte(skill)
	p[4], p[5] = byte(index>>8), byte(index)
	p[6] = byte(target >> 8)
	if ok {
		p[6] |= 0x80
	}
	p[7] = byte(target)
	return p
}

func (w *World) sendCastleState(o *Object) {
	var v byte
	if o.Map == 30 && w.hooks.castleBattle != nil && w.hooks.castleBattle(w) {
		v = 1
	}
	w.send(o, packetCastleStateNotice(v))
}

// clearViewportSilently is FUN_0050d930: the object's VP, VP2 and VPItem
// lists are emptied without telling anyone.
func clearViewportSilently(o *Object) {
	for i := range o.VP {
		o.VP[i] = VPEntry{Index: -1}
		o.VP2[i] = VPEntry{Index: -1}
		o.VPItem[i] = VPEntry{Index: -1}
	}
}

// reshow is FUN_0050da50 for players: map event state, the own C2 12
// (FUN_005167b0). The skin (FUN_00516d80) and guild (FUN_00516f60)
// viewport packets belong to modules not implemented yet.
func (w *World) reshow(o *Object) {
	if !o.IsPlayer() {
		// Monsters/NPCs (FUN_00516a90 / FUN_00516bf0) are re-announced by
		// the next viewport cycle of their observers.
		return
	}
	if w.hooks.mapEventState != nil {
		w.send(o, w.hooks.mapEventState(w, o)) // FUN_0048a690 (invasion.go)
	} else {
		w.send(o, packetMapEventState())
	}
	w.showSelf(o)
	if w.hooks.guildReshow != nil {
		w.hooks.guildReshow(w, o)
	}
}

// gatePosition is FUN_004810b0: follow TargetGate, then up to 30 random
// cells X1 + rand%(X2-X1), Y1 + rand%(Y2-Y1) (X2/Y2 themselves are never
// chosen) without the 0x04 / 0x08 attributes.
func (w *World) gatePosition(gate int) (g gamedata.Gate, x, y int, ok bool) {
	g, ok = w.data.Gates[gate]
	if !ok {
		return g, 0, 0, false
	}
	if g.TargetGate != 0 {
		if g, ok = w.data.Gates[g.TargetGate]; !ok {
			return g, 0, 0, false
		}
	}
	t := w.terrain(g.Map)
	for i := 0; i < 30; i++ {
		x, y = g.X1, g.Y1
		if d := g.X2 - g.X1; d > 0 {
			x = g.X1 + w.rand(d)
		}
		if d := g.Y2 - g.Y1; d > 0 {
			y = g.Y1 + w.rand(d)
		}
		if t.Get(x, y)&(gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
			return g, x, y, true
		}
	}
	return g, 0, 0, false
}

// gateAllowed is FUN_00480fb0: the player must stand within 5 cells of
// the gate's (X2, Y2) corner on its map and meet the level (MG needs 2/3)
// and reset limits.
func (w *World) gateAllowed(o *Object, gate int) bool {
	g, ok := w.data.Gates[gate]
	if !ok || o.P == nil || o.P.Char == nil || o.Map != g.Map {
		return false
	}
	if o.X < g.X2-5 || o.X > g.X2+5 || o.Y < g.Y2-5 || o.Y > g.Y2+5 {
		return false
	}
	c := o.P.Char
	if g.MinLevel != -1 {
		req := g.MinLevel
		if o.Class == 3 {
			req = req * 2 / 3
		}
		if c.Level < req {
			return false
		}
	}
	if g.MaxLevel != -1 && c.Level > g.MaxLevel {
		return false
	}
	if g.MinReset != -1 && c.Resets < g.MinReset {
		return false
	}
	if g.MaxReset != -1 && c.Resets > g.MaxReset {
		return false
	}
	return true
}

func (w *World) slotItem(o *Object, slot int) item.Item {
	if o.P == nil {
		return item.Empty()
	}
	return o.P.Inventory[slot]
}

// moveFailed is the common tail LAB_0050fcc6 of gObjMoveGate: the client
// gets the teleport packet for its current place and is shown again.
func (w *World) moveFailed(o *Object, gate int) {
	m := mv(o)
	clearViewportSilently(o)
	w.send(o, packetTeleport(gate, o.Map, o.X, o.Y, o.Dir))
	w.reshow(o)
	w.sendCastleState(o)
	m.RegenMap, m.RegenX, m.RegenY = o.Map, o.X, o.Y
	m.Regen = 1
}

// gateMove is gObjMoveGate (FUN_0050f6d0). It returns true when the
// object was moved (or handed to another server).
func (w *World) gateMove(o *Object, gate int) bool {
	m := mv(o)
	if o.IsPlayer() {
		for _, h := range w.hooks.beforeGateMove {
			h(w, o)
		}
	}
	if m.Regen != 0 {
		w.moveFailed(o, gate)
		return false
	}
	g, x, y, ok := w.gatePosition(gate)
	if !ok {
		w.moveFailed(o, gate)
		return false
	}
	if _, ok := w.data.Maps[g.Map]; !ok {
		w.moveFailed(o, gate)
		return false
	}
	// Gate 0x61 (97, the crown room) asks the castle module (FUN_0041e9b0 /
	// FUN_0041ea30, castlesiege_hooks.go); obj+0x2468 blocking 30<->0 is
	// still an open unknown.
	if gate == 0x61 && o.IsPlayer() && (w.hooks.gate97 == nil || !w.hooks.gate97(w, o)) {
		w.moveFailed(o, gate)
		return false
	}
	if o.IsPlayer() {
		pet := w.slotItem(o, 8)
		switch g.Map {
		case 7:
			if !pet.IsEmpty() && (pet.Index == item.Make(13, 2) || pet.Index == item.Make(13, 3)) {
				w.systemMessage(o, w.messageFor(o, 0x112))
				w.moveFailed(o, gate)
				return false
			}
		case 10:
			if w.slotItem(o, 7).IsEmpty() && (pet.IsEmpty() ||
				(pet.Index != item.Make(13, 3) && pet.Index != item.Make(13, 37))) {
				w.systemMessage(o, w.messageFor(o, 0x113))
				w.moveFailed(o, gate)
				return false
			}
		}
	}
	if o.Map != g.Map {
		for _, h := range w.hooks.mapChange {
			h(w, o, o.Map, g.Map)
		}
	}
	if !o.Alive {
		// obj+0x3d4 & 3 or obj+0x286 (dying): only the state changes.
		o.Live = LiveTeleport
		m.Regen = 1
		w.send(o, packetTeleport(gate, o.Map, o.X, o.Y, o.Dir))
		return false
	}
	o.Live = LiveTeleport
	if o.Map != g.Map && o.IsPlayer() {
		srv := w.data.MapServer.ServerForMap(w.cfg.ServerCode, g.Map)
		if srv != w.cfg.ServerCode {
			if srv < 0 {
				return false
			}
			w.startServerMove(o, srv, g.Map, x, y)
			return true
		}
	}
	w.placeAt(o, gate, g.Map, x, y, g.TargetDir)
	return true
}

// placeAt is the same-server part of gObjMoveGate (LAB_0050fb72).
func (w *World) placeAt(o *Object, gate, mp, x, y, dir int) {
	m := mv(o)
	// The original never moves the 0x02 occupancy bit here (it stays on
	// the old map until the next D3/DF of that cell); it is moved here to
	// keep the attribute consistent.
	w.setOccupancy(o, m.OldX, m.OldY, false)
	o.Live = LiveTeleport
	o.Map, o.Dir = mp, dir
	o.X, o.Y, o.TX, o.TY = x, y, x, y
	m.OldX, m.OldY = x, y
	w.setOccupancy(o, x, y, true)
	m.PathCount = 0
	m.TeleportState = 0
	o.Teleporting = false
	o.ActionState = 0
	clearViewportSilently(o)
	if o.IsPlayer() {
		w.send(o, packetTeleport(gate, o.Map, o.X, o.Y, o.Dir))
	}
	w.reshow(o)
	if o.IsPlayer() {
		w.sendCastleState(o)
	}
	m.RegenMap, m.RegenX, m.RegenY = o.Map, o.X, o.Y
	m.Regen = 1
}

// teleportTo implements hooks.teleport: a warp to (mp, x, y) handled like a
// gate move to that place (server change included).
func (w *World) teleportTo(o *Object, mp, x, y, dir int) {
	if o.Map != mp && o.IsPlayer() {
		if srv := w.data.MapServer.ServerForMap(w.cfg.ServerCode, mp); srv != w.cfg.ServerCode {
			if srv >= 0 {
				w.startServerMove(o, srv, mp, x, y)
			}
			return
		}
	}
	w.placeAt(o, 1, mp, x, y, dir)
}

// handleTeleport is 1C (FUN_004bf010): [3] gate; gate 0 is the Teleport
// magic with [4] X [5] Y.
func (w *World) handleTeleport(o *Object, pkt []byte) {
	if len(pkt) < 4 || !o.IsPlayer() || !canPlayerAct(o) || w.immobilized(o) {
		return
	}
	gate := int(pkt[3])
	if gate != 0 {
		if !w.gateAllowed(o, gate) {
			clearViewportSilently(o)
			w.send(o, packetTeleport(gate, o.Map, o.X, o.Y, o.Dir))
			w.sendCastleState(o)
			return
		}
		w.gateMove(o, gate)
		return
	}
	if len(pkt) < 6 {
		return
	}
	w.magicTeleport(o, int(pkt[4]), int(pkt[5]))
}

// hasSkill reports whether the player learned a skill (FUN_004f8cb0).
func hasSkill(o *Object, index int) bool {
	if o.P == nil {
		return false
	}
	for _, s := range o.P.Skills {
		if s.Index == index {
			return true
		}
	}
	return false
}

// magicTeleport is the gate-0 branch of FUN_004bf010.
func (w *World) magicTeleport(o *Object, x, y int) {
	m := mv(o)
	info, ok := w.data.Skills[teleportSkill]
	now := w.now()
	// FUN_004f83b0: skill delay from SkillList (obj+0x600[skill]).
	if !ok || now.Sub(m.LastTeleport) < time.Duration(info.Delay)*time.Millisecond {
		w.send(o, packetTeleport(0, o.Map, o.X, o.Y, o.Dir))
		return
	}
	m.LastTeleport = now
	// FUN_0050d0a0: alive, not teleporting, within 8 cells, neither the
	// current nor the target cell in a safe zone.
	t := w.terrain(o.Map)
	if !o.Alive || m.TeleportState != 0 || x < o.X-8 || x > o.X+8 || y < o.Y-8 || y > o.Y+8 ||
		t.Get(o.X, o.Y)&gamedata.AttrSafeZone != 0 || t.Get(x, y)&gamedata.AttrSafeZone != 0 {
		w.send(o, packetTeleport(0, o.Map, o.X, o.Y, o.Dir))
		return
	}
	// TODO(teleport): map 30 with X < 0x90 calls the castle module
	// (FUN_0041f400) first.
	if !hasSkill(o, teleportSkill) {
		return
	}
	c := o.P.Char
	// FUN_004f8030 / FUN_004f80b0 and the cost: Mana -= cost*obj+0x8b0/100,
	// BP -= cost*obj+0x8b4/100. The two rates belong to the stats module
	// (100 = no reduction is assumed).
	mana, bp := float32(info.Mana*costRate/100), float32(info.BP*costRate/100)
	if c.Mana < mana || c.BP < bp {
		return
	}
	p := packetMagicUse(teleportSkill, o.Index, o.Index, true)
	w.send(o, p)
	w.broadcast(o, p)
	w.startMagicTeleport(o, x, y)
	c.Mana -= mana
	c.BP -= bp
	if w.hooks.sendMana != nil {
		w.hooks.sendMana(w, o)
	}
}

// startMagicTeleport is FUN_0050e180.
func (w *World) startMagicTeleport(o *Object, x, y int) {
	m := mv(o)
	if m.TeleportState != 0 {
		return
	}
	m.TeleportTick = w.now()
	m.TeleportState = 1
	o.Teleporting = true
	o.ActionState = 1
	m.PathCount = 0
	o.X, o.Y, o.TX, o.TY = x, y, x, y
	w.setOccupancy(o, m.OldX, m.OldY, false)
	w.setOccupancy(o, x, y, true)
	m.OldX, m.OldY = x, y
	// FUN_0050d9b0: C1 06 14 01 idx to everyone who sees the object.
	w.broadcast(o, packetDestroy([]int{o.Index}))
}

// handleRegenReady is F3 12 (FUN_004d9bb0): the client finished loading
// after a teleport packet.
func (w *World) handleRegenReady(o *Object, _ []byte) {
	if !o.IsPlayer() || !canPlayerAct(o) {
		return
	}
	if m := mv(o); m.Regen == 1 {
		m.Regen = 2
	}
}

// teleportSecond is the teleport part of FUN_004c37f0, run for every
// object before the viewport cycle.
func (w *World) teleportSecond() {
	now := w.now()
	for _, o := range w.objects {
		if o.State != StatePlaying || o.Move == nil {
			continue
		}
		m := o.Move
		if m.TeleportState == 2 {
			m.TeleportState = 3
			o.TX, o.TY = o.X, o.Y
			if o.IsPlayer() {
				clearViewportSilently(o)
				w.send(o, packetTeleport(0, o.Map, o.X, o.Y, o.Dir))
				w.reshow(o)
				w.sendCastleState(o)
			} else {
				// TODO(teleport): NPCs (type 3) with obj+0x310 out of range
				// jump by rand%6-3 on each axis (summons); monster module.
				w.reshow(o)
			}
			m.TeleportState = 0
			o.Teleporting = false
			m.PathCur, m.PathCount, m.PathStartEnd = 0, 0, 0
		}
		if m.TeleportState == 1 {
			if !o.Alive {
				m.TeleportState = 0
				o.Teleporting = false
			} else if now.Sub(m.TeleportTick) > 500*time.Millisecond {
				m.TeleportState = 2
			}
		}
		if o.IsPlayer() && m.Regen == 2 {
			o.Live = LiveNew
			o.X, o.TX = m.RegenX, m.RegenX
			o.Y, o.TY = m.RegenY, m.RegenY
			o.Map = m.RegenMap
			m.Regen = 3
			w.sendCastleState(o)
		}
	}
}

// teleportViewportEnd is the FUN_004c3b10 part run after the viewport
// cycle: obj+0x284 3 -> 0 and, once the object is visible again
// (Live 1 -> 2, done by viewportProc), obj+0x287 3 -> 0.
func (w *World) teleportViewportEnd() {
	for _, o := range w.objects {
		if o.State != StatePlaying || o.Move == nil {
			continue
		}
		if o.Move.TeleportState == 3 {
			o.Move.TeleportState = 0
			o.Teleporting = false
		}
		if o.Move.Regen == 3 && o.Live == LiveNormal {
			o.Move.Regen = 0
		}
	}
}

func registerTeleport(h *hooks) {
	h.worldHandlers[0x1C] = (*World).handleTeleport
	h.f3Handlers[0x12] = (*World).handleRegenReady
	h.tick1000 = append(h.tick1000, (*World).teleportSecond)
	h.viewportEnd = append(h.viewportEnd, (*World).teleportViewportEnd)
	h.teleport = func(w *World, o *Object, mp, x, y, dir int) { w.teleportTo(o, mp, x, y, dir) }
}
