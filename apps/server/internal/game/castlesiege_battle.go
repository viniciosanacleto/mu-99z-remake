package game

import (
	"context"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// The battle (state 7): start and end, the crown and its switches, the
// minimap, the PvP/PK rules of map 30, slingshots, life stones,
// mercenaries and the statue regeneration.

const (
	csCrownTime   = 30000 // FUN_0041edc0: 29999 < accumulated + elapsed
	csCrownMax    = 31000 // FUN_00422bf0 clamp
	csMaxMercs    = 100   // FUN_004b2bd0: count < 0x65
	csStoneGrowth = 60    // FUN_004a38d0: 12 s per level, 5 levels
	csWeaponDelay = 10 * time.Second
	csWeaponHit   = 100 * time.Millisecond
	csMinimapMax  = 1000
)

// Crown access states of B2 1A (FUN_00422bf0).
const (
	csCrownBegin   = 0 // registration started
	csCrownSuccess = 1
	csCrownFail    = 2 // interrupted (time accumulated)
	csCrownOther   = 3 // another player holds the crown
	csCrownNoSwich = 4 // the switches are not held by the player's side
)

// csEnterStart is FUN_0041cc10.
func (w *World) csEnterStart(setRemain bool) {
	c := w.cs()
	w.log.Info("[Castle Siege] SetState START")
	if setRemain && !w.csSetRemainFromSchedule(csStart) {
		return
	}
	c.crownRoom, c.crownOpen = false, false
	c.crownUser, c.switches = -1, [2]int{-1, -1}
	c.crownStart = time.Time{}
	now := w.now()
	c.checkTick, c.readyTick = time.Time{}, time.Time{}
	c.leftTick, c.sideTick, c.mapTick = now, now, now
	c.occupier = ""
	if len(c.all) != 0 {
		w.csSaveNPCs()          // FUN_00426810
		w.csClearBattleNPCs()   // FUN_0041d970
		w.csSpawnBattleNPCs()   // FUN_0041f4a0
		if !w.csStatueAlive() { // FUN_0041eb40
			w.csRemoveShield()
			c.crownRoom = true
		}
		w.csRefreshNPCs()   // FUN_0041e080
		w.csCloseGates()    // FUN_0041f9c0
		w.csRefreshSides()  // FUN_0041e860
		w.csMoveOutsiders() // FUN_0041f620
		w.csSendMap(packetCastleStateNotice(1))
		return
	}
	w.csDecideOwner()
	w.csSetState(csEnd, true)
	w.log.Info("[Castle Siege] SetState START - No Attack Guild")
}

// csStartProc is FUN_0041c2e0.
func (w *World) csStartProc() {
	c := w.cs()
	w.csCrownCheck() // FUN_0041edc0
	now := w.now()
	if now.Sub(c.remainTick) >= time.Second {
		w.csUpdateRemain()
		if now.Sub(c.checkTick) > 5*time.Second {
			w.csCheckDestroyed() // FUN_0041df10
			c.checkTick = now
		}
		// B2 1E every 3 minutes, every minute in the last 3.
		if now.Sub(c.leftTick) > 180*time.Second && c.remain > 180000 {
			w.csSendMap(packetCSRemainTime(c.remain))
			c.leftTick = now
		}
		if c.remain <= 180000 && now.Sub(c.leftTick) > 60*time.Second {
			w.csSendMap(packetCSRemainTime(c.remain))
			c.leftTick = now
		}
		if now.Sub(c.sideTick) > 5*time.Second {
			w.csRefreshSides()
			c.sideTick = now
		}
		if now.Sub(c.mapTick) > 3*time.Second {
			w.csSendMinimap() // FUN_0041fa40
			c.mapTick = now
		}
		if c.noticeTick.IsZero() || now.Sub(c.noticeTick) > 30*time.Minute {
			w.csGlobalNotice(117)
			c.noticeTick = now
		}
	}
	if c.remain >= 1 {
		return
	}
	if w.csDecideOwner() {
		w.csResetTaxInfo() // FUN_00425ec0 (0x80 0x19)
	}
	w.csMoveOutsiders()
	st := w.csStore()
	if st != nil {
		group, occupied, owner := c.group, c.occupied, c.owner
		w.async(func(ctx context.Context) func() {
			if occupied {
				st.SetCastleOwner(ctx, group, true, owner) // FUN_00425b40 (0x0c)
			}
			st.SetCastleSiegeEnded(ctx, group, true) // FUN_00425ab0 (0x0b)
			return nil
		})
	}
	w.csSaveNPCs()
	w.csSendMap(packetCastleStateNotice(0))
	if st != nil {
		group := c.group
		w.async(func(ctx context.Context) func() {
			st.ResetCastleSiegeGuilds(ctx, group) // FUN_00425f50 (0x1a)
			return nil
		})
	}
	w.csSetState(csEnd, true)
}

// csResetTaxInfo is FUN_00425ec0 / answer FUN_004247c0: a new owner
// starts with no money and no taxes.
func (w *World) csResetTaxInfo() {
	c, st := w.cs(), w.csStore()
	if st == nil {
		return
	}
	group := c.group
	w.async(func(ctx context.Context) func() {
		r, err := st.ResetCastleTaxInfo(ctx, group)
		if err != nil || r != 1 {
			return nil
		}
		return func() {
			c.money, c.taxChaos, c.taxStore, c.taxHunt = 0, 0, 0, 0
		}
	})
}

// csDecideOwner is FUN_0041efc0: the guild holding the crown at the end
// owns the castle (notice 119); without one the owner keeps it (118) or
// nobody owns it (120). True when the owner changed.
func (w *World) csDecideOwner() bool {
	c := w.cs()
	c.siegeEnded = true
	if c.occupied && c.owner != "" {
		if c.occupier == "" || c.occupier == c.owner {
			c.occupied = true
			w.csGlobalNotice(118, c.owner)
			return false
		}
		c.occupied = true
		c.owner = c.occupier
		w.csGlobalNotice(119, c.owner)
		w.log.Info("[Castle Siege] Guild is the new owner of the castle.", "guild", c.owner)
		return true
	}
	if c.occupier == "" {
		c.occupied = false
		w.csGlobalNotice(120)
		return false
	}
	c.occupied = true
	c.owner = c.occupier
	w.csGlobalNotice(119, c.owner)
	w.log.Info("[Castle Siege] Guild is the new owner of the castle.", "guild", c.owner)
	return true
}

// csMoveOutsiders is FUN_0041f620: the players of map 30 who are not
// defenders go to a random cell of their side's area (C3 08 1C FF).
func (w *World) csMoveOutsiders() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.P.LogoutCountdown > 0 || o.P.Moving ||
			o.Map != csMap || csSide(o) == 1 {
			continue
		}
		if x, y, ok := w.csRandomSidePos(int(csSide(o))); ok {
			w.placeAt(o, -1, csMap, x, y, o.Dir)
		}
		csOf(o).Crown = 0
	}
}

// csSideAreas are the rectangles of FUN_0041f810 per side (0, 1, 2+).
var csSideAreas = [3][4]int{{35, 17, 144, 48}, {74, 144, 115, 154}, {35, 17, 144, 48}}

// csRandomSidePos is FUN_0041f810: up to 100 random cells without the
// 0x04 / 0x08 attributes.
func (w *World) csRandomSidePos(side int) (int, int, bool) {
	a := csSideAreas[min(side, 2)]
	t := w.terrain(csMap)
	if t == nil {
		return 0, 0, false
	}
	for i := 0; i < 100; i++ {
		x := w.rand(a[2]-a[0]) + a[0]
		y := w.rand(a[3]-a[1]) + a[1]
		if t.Get(x, y)&(gamedata.AttrBlock|gamedata.AttrNoGround) == 0 {
			return x, y, true
		}
	}
	return 0, 0, false
}

// --- crown and switches ------------------------------------------------------------

// csSwitchSlot maps the switch classes to +0x88 / +0x8c (FUN_0041f3c0).
func csSwitchSlot(class int) int {
	switch class {
	case csClassSwitch2:
		return 0
	case csClassSwitch1:
		return 1
	}
	return -1
}

func (w *World) csObj(idx int) *Object {
	if idx < 0 || idx >= MaxObjects || w.objects[idx].State != StatePlaying {
		return nil
	}
	return w.objects[idx]
}

// csSwitchesHeldBy reports both switches held by players of side.
func (w *World) csSwitchesHeldBy(side byte) bool {
	c := w.cs()
	a, b := w.csObj(c.switches[0]), w.csObj(c.switches[1])
	return a != nil && b != nil && csSide(a) == side && csSide(b) == side
}

// csCrownTalk is FUN_00426a50 (NPC 0xD8): an attacking guild master next
// to the open crown starts the registration.
func (w *World) csCrownTalk(o, crown *Object) {
	c := w.cs()
	cs := csOf(o)
	if c.state != csStart || cs.Side < 2 || !cs.Involved || !w.csGuildMaster(o) {
		return
	}
	if abs(o.Y-crown.Y) >= 4 || abs(o.X-crown.X) >= 4 || !c.crownOpen {
		return
	}
	if cur := w.csObj(c.crownUser); cur == nil {
		if w.csSwitchesHeldBy(cs.Side) {
			w.csCrownAccess(o, csCrownBegin)
			c.crownUser = o.Index
			c.crownX, c.crownY = o.X, o.Y
			c.crownStart = w.now()
			w.csSendMap(packetCSCrownGuild(0, csGuildName(o)))
			return
		}
		w.csCrownAccess(o, csCrownNoSwich)
		return
	} else if cur.Index != o.Index {
		w.csCrownAccess(o, csCrownOther)
	}
}

// csCrownAccess is FUN_00422bf0: B2 1A with the accumulated crown time;
// a failure adds the elapsed time minus CastleSiegeLowerAccumulatedTimeValue.
func (w *World) csCrownAccess(o *Object, state byte) {
	c := w.cs()
	cs := csOf(o)
	switch state {
	case csCrownBegin:
		if cs.Crown < 0 || cs.Crown > csCrownMax {
			cs.Crown = 0
		}
	case csCrownSuccess:
		cs.Crown = 0
	case csCrownFail:
		cs.Crown += int(w.now().Sub(c.crownStart).Milliseconds())
		cs.Crown -= w.cfg.Int("Event", "CastleSiegeLowerAccumulatedTimeValue", 0)
		if cs.Crown < 0 || cs.Crown > csCrownMax {
			cs.Crown = 0
		}
	}
	w.send(o, packetCSCrownAccess(state, cs.Crown))
}

// csCrownCheck is FUN_0041edc0: the registering master keeps the crown
// for 30 s (minus the accumulated time) and his guild takes the castle.
func (w *World) csCrownCheck() {
	c := w.cs()
	if c.crownStart.IsZero() {
		return
	}
	u, s1, s2 := w.csObj(c.crownUser), w.csObj(c.switches[0]), w.csObj(c.switches[1])
	if u == nil || s1 == nil || s2 == nil || csGuildName(u) == "" || csGuildName(s1) == "" || csGuildName(s2) == "" {
		return
	}
	side := csSide(u)
	if side < 2 || csSide(s1) < 2 || csSide(s2) < 2 || !csOf(u).Involved || side != csSide(s1) || side != csSide(s2) {
		return
	}
	if int(w.now().Sub(c.crownStart).Milliseconds())+csOf(u).Crown < csCrownTime {
		return
	}
	guild := csGuildName(u)
	w.log.Info("[Castle Siege] Guild has taken possession of the castle.", "guild", guild)
	w.csCrownAccess(u, csCrownSuccess)
	w.csCrownState(2)
	c.occupier = guild
	w.csKillLifeStones() // FUN_0041da20
	w.csSwapSides(side)  // FUN_0041eb90
	w.csSendMap(packetCSCrownGuild(1, guild))
	c.crownStart, c.crownUser, c.crownX, c.crownY = time.Time{}, -1, 0, 0
	c.switches = [2]int{-1, -1}
	c.crownOpen = false
	w.csMoveOutsiders()
	w.csSendMap(packetCSRemainTime(c.remain))
	c.leftTick = w.now()
}

// csCrownState is FUN_00422d10: effect 20 on the crown (open) and B2 16 to
// its viewers (0 open, 1 closed, 2 taken).
func (w *World) csCrownState(v byte) {
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.Class != csClassCrown || o.Mon == nil {
			continue
		}
		if eventEffectHook != nil {
			eventEffectHook(w, o, csEffCrown, v == 0)
		}
		w.broadcast(o, packetCSCrownState(v))
		return
	}
}

// csSwapSides is FUN_0041eb90: the guilds of the winning side become the
// defenders and the old defenders take the winners' number.
func (w *World) csSwapSides(side byte) {
	c := w.cs()
	for _, g := range c.all {
		switch g.Side {
		case int(side):
			g.Side = 1
		case 1:
			g.Side = int(side)
		}
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || csGuildName(o) == "" {
			continue
		}
		if e, ok := c.all[csGuildName(o)]; ok {
			cs := csOf(o)
			cs.Side, cs.Involved = byte(e.Side), e.Involved
		}
		csOf(o).Crown = 0
		w.csSideMark(o)
	}
}

// csCrownProc is the crown object proc FUN_00429570 (1 s): the
// registration fails when the master moved, left or lost a switch.
func (w *World) csCrownProc() {
	c := w.cs()
	if c.crownUser < 0 {
		return
	}
	u := w.csObj(c.crownUser)
	if u == nil {
		c.crownX, c.crownY, c.crownUser, c.crownStart = 0, 0, -1, time.Time{}
		return
	}
	cs := csOf(u)
	if u.Map == csMap && cs.Side > 1 && cs.Involved && u.X == c.crownX && u.Y == c.crownY &&
		w.csSwitchesHeldBy(cs.Side) {
		return
	}
	w.csCrownAccess(u, csCrownFail)
	c.crownX, c.crownY, c.crownUser, c.crownStart = 0, 0, -1, time.Time{}
}

// csSwitchTalk is FUN_00426bb0 (NPC 0xD9/0xDA): an attacker within 4
// cells holds the switch while the guardian statues are down.
func (w *World) csSwitchTalk(o, sw *Object) {
	c := w.cs()
	if c.state != csStart || csSide(o) < 2 || abs(o.Y-sw.Y) >= 4 || abs(o.X-sw.X) >= 4 {
		return
	}
	if w.csStatueAlive() {
		w.systemMessage(o, w.messageFor(o, 121))
		return
	}
	slot := csSwitchSlot(sw.Class)
	if slot < 0 {
		return
	}
	cur := w.csObj(c.switches[slot])
	if cur == nil {
		w.send(o, packetCSSwitchUser(sw.Index, -1, 1))
		c.switches[slot] = o.Index
		return
	}
	if cur.Index != o.Index {
		w.send(o, packetCSSwitchUser(sw.Index, cur.Index, 2))
	}
}

// csSwitchProc is FUN_00429750 (1 s per switch): B2 20 to the holder and
// the viewers; within 3 cells the switch stays held and opens the crown
// when both are held by one side; otherwise it is released.
func (w *World) csSwitchProc(sw *Object) {
	c := w.cs()
	slot := csSwitchSlot(sw.Class)
	if slot < 0 {
		return
	}
	u := w.csObj(c.switches[slot])
	release := func() {
		if c.crownOpen {
			c.crownOpen = false
			w.csCrownState(1)
		}
		c.switches[slot] = -1
	}
	if u == nil {
		release()
		return
	}
	info := w.csSwitchInfo(sw, u)
	w.send(u, info)
	w.broadcast(sw, info)
	if u.Map == csMap && csSide(u) > 1 {
		if abs(u.X-sw.X) <= 3 && abs(u.Y-sw.Y) <= 3 {
			a, b := w.csObj(c.switches[0]), w.csObj(c.switches[1])
			if a == nil || b == nil || csSide(a) != csSide(b) || c.crownOpen {
				return
			}
			c.crownOpen = true
			w.csCrownState(0)
			return
		}
		w.send(u, packetCSSwitchUser(sw.Index, -1, 0))
		release()
		return
	}
	w.send(u, packetCSSwitchUser(sw.Index, -1, 0))
	release()
}

// csSwitchInfo is FUN_004230e0: C1 1A B2 20 with the holder's side, guild
// (the alliance master guild when in an alliance) and name.
func (w *World) csSwitchInfo(sw, u *Object) []byte {
	guild := csGuildName(u)
	if m := csUnionMaster(w, u); m != "" {
		guild = m
	}
	return packetCSSwitchInfo(sw.Index, true, csSide(u), guild, u.P.Char.Name)
}

// --- minimap ---------------------------------------------------------------

// handleCSMinimap is B2 1B (FUN_00421610): attacking/defending guild
// masters receive the minimap during the battle.
func (w *World) handleCSMinimap(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	if csSide(o) == 0 || !w.csGuildMaster(o) {
		w.send(o, packetCSMinimapResult(3))
		return
	}
	if c.state != csStart {
		w.send(o, packetCSMinimapResult(2))
		return
	}
	w.send(o, packetCSMinimapResult(1))
	for _, i := range c.minimap {
		if i == o.Index {
			return
		}
	}
	c.minimap = append(c.minimap, o.Index)
}

// handleCSMinimapClose is B2 1C (FUN_004216f0).
func (w *World) handleCSMinimapClose(o *Object, _ []byte) {
	if !w.csEnabled() || !o.IsPlayer() {
		return
	}
	c := w.cs()
	for i, idx := range c.minimap {
		if idx == o.Index {
			c.minimap = append(c.minimap[:i], c.minimap[i+1:]...)
			return
		}
	}
}

// handleCSCommand is B2 1D (FUN_00421790): a guild master command (4
// bytes) to map 30 and to the players of the same side.
func (w *World) handleCSCommand(o *Object, p []byte) {
	if len(p) < 8 || !w.csEnabled() || !o.IsPlayer() || csSide(o) == 0 || !w.csGuildMaster(o) ||
		w.cs().state != csStart {
		return
	}
	pk := packetCSCommand(p[4], p[5], p[6], p[7])
	for i := PlayerStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State == StatePlaying && t.IsPlayer() && (t.Map == csMap || csSide(t) == csSide(o)) {
			w.send(t, pk)
		}
	}
}

// csSendMinimap is FUN_0041fa40 (every 3 s of the battle): each master of
// the minimap list gets the cells of his side's players (C2 B6) and the
// living gates and statues (C2 BB).
func (w *World) csSendMinimap() {
	c := w.cs()
	if !c.listLoaded || len(c.minimap) == 0 {
		return
	}
	var npcs []csMinimapNPC
	for _, n := range c.npcs {
		if n.Class != csClassGate && n.Class != csClassStatue {
			continue
		}
		t := w.csNPCObj(n)
		if t == nil || !t.Alive {
			continue
		}
		typ := byte(0)
		if n.Class == csClassStatue {
			typ = 1
		}
		npcs = append(npcs, csMinimapNPC{Type: typ, X: byte(t.X), Y: byte(t.Y)})
	}
	keep := c.minimap[:0]
	for _, idx := range c.minimap {
		m := w.csObj(idx)
		if m == nil || !m.IsPlayer() {
			continue
		}
		keep = append(keep, idx)
		side := csSide(m)
		var cells [][2]byte
		for i := PlayerStart; i < MaxObjects && len(cells) < csMinimapMax; i++ {
			t := w.objects[i]
			if t.State == StatePlaying && t.IsPlayer() && t.Map == csMap && csSide(t) == side && side != 0 {
				cells = append(cells, [2]byte{byte(t.X), byte(t.Y)})
			}
		}
		w.send(m, packetCSMinimapPlayers(cells))
		w.send(m, packetCSMinimapNPCs(npcs))
	}
	c.minimap = keep
}

// --- PvP, PK and attacks on map 30 ------------------------------------------------

// csBattleOn is the "CCastleSiegeSync+4 == 7" test used by the other
// modules (shared by every instance).
func (w *World) csBattleOn() bool { return w.csSyncState() == csStart }

// csAttackAllowed is the start of FUN_0040b110: the gates, statues and
// canon towers of map 30 only take hits during the battle from attackers.
func (w *World) csAttackAllowed(a, t *Object) bool {
	if t.Map != csMap || t.Type != TypeMonster ||
		(t.Class != csClassGate && t.Class != csClassStatue && t.Class != csClassCanon) {
		return true
	}
	c := w.cs()
	if c == nil || c.state != csStart {
		return false
	}
	return csSide(w.attackOwner(a)) >= 2
}

// csPvP is the map 30 branch of FUN_0040dca0: during the battle players
// with sides may fight unless both are on the same side and
// CastleSiegeDamageRate2 is 0.
func (w *World) csPvP(a, t *Object) (allowed, decided bool) {
	if a.Map != csMap || t.Map != csMap || !w.csBattleOn() || csSide(a) == 0 || csSide(t) == 0 {
		return false, false
	}
	if w.cfg.Int("Character", "CastleSiegeDamageRate2", 0) == 0 && csSide(a) == csSide(t) {
		return false, true
	}
	return true, true
}

// csDamageRate is the map 30 branch of the PvP damage of FUN_0040b110:
// CastleSiegeDamageRate1 between sides (or without one), Rate2 within a
// side.
func (w *World) csDamageRate(a, t *Object) (int, bool) {
	c := w.cs()
	if c == nil || c.state != csStart || a.Map != csMap || t.Map != csMap {
		return 0, false
	}
	if csSide(a) == 0 || csSide(t) == 0 || csSide(a) != csSide(t) {
		return w.cfg.Int("Character", "CastleSiegeDamageRate1", 0), true
	}
	return w.cfg.Int("Character", "CastleSiegeDamageRate2", 0), true
}

// csPKExempt is the map 30 part of FUN_0050f330: no PK for kills with a
// side during the battle, for kills by the owner's alliance inside the
// castle (Y > 0x71) and for victims with PK level above 5.
func (w *World) csPKExempt(killer, victim *Object) bool {
	if killer.Map != csMap {
		return false
	}
	if w.csBattleOn() && csSide(killer) != 0 {
		return true
	}
	if killer.Y > 0x71 && w.cs() != nil && w.csOwnerAlliance(killer) {
		return true
	}
	return victim.PKLevel > 5
}

// csRespawn is the map 30 branch of FUN_004c53c0: next to the guild's
// grown life stone, otherwise gate 101 (inside the castle) for defenders
// and gate 100 (the attackers' field) for the others.
func (w *World) csRespawn(o *Object) (gate, mp, x, y int, ok bool) {
	if o.Map != csMap || w.cs() == nil {
		return 0, 0, 0, 0, false
	}
	if s := w.csGuildStone(o); s != nil && s.CS.StoneSec > 59 {
		return -1, csMap, s.X, s.Y, true
	}
	if csSide(o) == 1 {
		return 101, 0, 0, 0, true
	}
	return 100, 0, 0, 0, true
}

// csDecayCrown is the map 30 part of FUN_00510810: outside the crown area
// (150..200 x 175..225) the accumulated crown time drops by
// CastleSiegeDecayAccumulatedTimeValue per second.
func (w *World) csDecayCrown(o *Object) {
	if o.Map != csMap || o.CS == nil || o.CS.Crown <= 0 || !w.csBattleOn() {
		return
	}
	if o.X >= 150 && o.X <= 200 && o.Y >= 175 && o.Y <= 225 {
		return
	}
	o.CS.Crown = max(o.CS.Crown-w.cfg.Int("Event", "CastleSiegeDecayAccumulatedTimeValue", 0), 0)
}

// --- regeneration around statues and life stones -------------------------------------

// csHeal is the 1% per second life/mana/BP of FUN_00481410 / FUN_004a38d0.
func (w *World) csHeal(t *Object) {
	if t.P == nil || t.P.Char == nil {
		return
	}
	c := t.P.Char
	if lim := c.MaxLife + float32(t.P.AddLife); c.Life < lim {
		c.Life = min(c.Life+lim/100, lim)
		if sendLifeHook != nil {
			sendLifeHook(w, t)
		}
	}
	if lim := c.MaxMana + float32(t.P.AddMana); c.Mana < lim {
		c.Mana = min(c.Mana+lim/100, lim)
		if sendManaHook != nil {
			sendManaHook(w, t)
		}
	}
	if lim := c.MaxBP + float32(t.P.AddBP); c.BP < lim {
		c.BP = min(c.BP+float32(int(lim)/100), lim)
		if sendManaHook != nil {
			sendManaHook(w, t)
		}
	}
}

// csHealAround heals the living players of a side within 4 cells among
// the object's viewport.
func (w *World) csHealAround(o *Object, side byte) {
	for _, e := range o.VP {
		if e.State == VPEmpty || e.Type != byte(TypePlayer) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.Alive && csSide(t) == side && distance(o, t) < 4 {
			w.csHeal(t)
		}
	}
}

// csStatueProc is FUN_00481410: the guardian statue heals the defenders.
func (w *World) csStatueProc(o *Object) {
	if o.Alive && o.Live == LiveNormal {
		w.csHealAround(o, 1)
	}
}

// csStoneProc is FUN_004a38d0: the life stone grows one level every 12 s
// (C1 07 B9 01 index level to the viewers) and from level 5 heals its
// side.
func (w *World) csStoneProc(o *Object) {
	cs := csOf(o)
	cs.StoneSec++
	lvl := byte(5)
	if cs.StoneSec < csStoneGrowth {
		lvl = byte(cs.StoneSec / 12)
	}
	if lvl != cs.StoneLvl {
		cs.StoneLvl = lvl
		w.broadcast(o, packetCSStoneState(o.Index, lvl))
	}
	if cs.StoneLvl > 4 {
		w.csHealAround(o, cs.Side)
	}
}

// csStones maps a guild number to its life stone (CGuild+0x80c).
func (c *castleSiege) stoneOf(n int) int {
	for i := range c.stones {
		if c.stones[i].guild == n {
			return c.stones[i].obj
		}
	}
	return -1
}

type csStone struct{ guild, obj int }

// csGuildStone returns the life stone of the player's guild.
func (w *World) csGuildStone(o *Object) *Object {
	c := w.cs()
	if c == nil || guildNumberOf(o) == 0 {
		return nil
	}
	s := w.csObj(c.stoneOf(guildNumberOf(o)))
	if s == nil || s.Class != csClassStone {
		return nil
	}
	return s
}

// csStoneGone is FUN_004a3810: the guild may summon a new stone.
func (w *World) csStoneGone(o *Object) {
	c := w.cs()
	for i := range c.stones {
		if c.stones[i].obj == o.Index {
			c.stones = append(c.stones[:i], c.stones[i+1:]...)
			return
		}
	}
}

// csKillLifeStones is FUN_0041da20 (the crown is taken).
func (w *World) csKillLifeStones() {
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if csIsCastleObject(o) && o.CS.NPCKind != 1 && o.Class == csClassStone {
			w.csStoneGone(o)
			w.deleteMonster(o)
		}
	}
}

// csSummonStone is FUN_004a35b0 (dropping a Life Stone scroll 13,11 +1):
// attacking guild masters only, one per guild, not inside the dragon
// tower area (151..209 x 176..229). Messages 122..127.
func (w *World) csSummonStone(o *Object) bool {
	c := w.cs()
	if csGuildStatus(o) != 0x80 {
		return false
	}
	msg := func(n int) bool { w.systemMessage(o, w.messageFor(o, n)); return false }
	if o.Map != csMap || c == nil || c.state != csStart {
		return msg(122)
	}
	if csSide(o) < 2 {
		return msg(123)
	}
	if c.stoneOf(guildNumberOf(o)) >= 0 {
		return msg(124)
	}
	if uint(o.X-0x97) < 0x3b && uint(o.Y-0xb0) < 0x36 {
		return msg(125)
	}
	s := w.spawnFixedNPC(csClassStone, o.Map, o.X, o.Y, 1)
	if s == nil {
		return msg(127)
	}
	s.Mon.RegenTime = 1000
	s.Mon.Attribute = 3
	cs := csOf(s)
	cs.NPCKind, cs.Side = 3, csSide(o)
	c.stones = append(c.stones, csStone{guild: guildNumberOf(o), obj: s.Index})
	csOf(o).Stones++
	w.systemMessage(o, w.messageFor(o, 126))
	return true
}

// csSummonMercenary is FUN_004b2bd0 (dropping a scroll 13,7 +0/+1):
// defending masters/assistants place an archer (286) or spearman (287)
// at the drop cell during the battle, up to 100. Messages 128..133.
func (w *World) csSummonMercenary(o *Object, class, x, y int) bool {
	c := w.cs()
	msg := func(n int) bool { w.systemMessage(o, w.messageFor(o, n)); return false }
	if o.Map != csMap || c == nil || c.state != csStart {
		return msg(128)
	}
	if csSide(o) != 1 {
		return msg(129)
	}
	if !csMasterOrAssistant(o) {
		return msg(130)
	}
	if c.mercenaries >= csMaxMercs+1 {
		return msg(131)
	}
	m := w.spawnFixedNPC(class, o.Map, x, y, 1)
	if m == nil {
		return msg(133)
	}
	m.Mon.MaxLife = m.Mon.Life
	m.Mon.MoveRange = 0
	// obj+0x37e = 0 in the original; obj+0x294 = 0xFFFF removes it when it
	// dies, which this reconstruction expresses with attribute 3.
	m.Mon.Attribute = 3
	m.Mon.RegenTime = 1000
	m.Type = TypeMonster
	cs := csOf(m)
	cs.NPCKind, cs.Side = 2, 1
	c.mercenaries++
	w.systemMessage(o, w.messageFor(o, 132))
	return true
}

// csMercenarySearch is FUN_004b2ec0: mercenaries pick a random visible
// player of another side (not GM, outside the safe zone).
func (w *World) csMercenarySearch(o *Object) bool {
	if o.Class != csClassArcher && o.Class != csClassSpear {
		return false
	}
	var cand []int
	for _, e := range o.VP {
		if e.State == VPEmpty || e.Type != byte(TypePlayer) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if csSide(t) == csSide(o) || !t.Alive || (t.Move != nil && t.Move.Authority == 2) || gmFull(t) {
			continue
		}
		if hasEffectIndex(t, effInvisibility) || w.cellHas(t, gamedata.AttrSafeZone) {
			continue
		}
		cand = append(cand, e.Index)
	}
	m := o.Mon
	if len(cand) == 0 {
		m.Target = -1
		return true
	}
	m.Target = cand[rnd(len(cand))]
	m.ActAttack = true
	return true
}

// csDropItem intercepts the castle scrolls of the item drop FUN_004988f0.
func (w *World) csDropItem(o *Object, it item.Item, x, y int) (handled, ok bool) {
	if w.cs() == nil {
		return false, false
	}
	switch {
	case it.Index == item.Make(13, 7) && (it.Level == 0 || it.Level == 1):
		return true, w.csSummonMercenary(o, csClassArcher+it.Level, x, y)
	case it.Index == item.Make(13, 11) && it.Level == 1:
		return true, w.csSummonStone(o)
	}
	return false, false
}

// --- slingshots (CCastleSiegeWeapon) ----------------------------------------------

// csWeaponZones are DAT_00547990 / DAT_005479a0: 4 target rectangles per
// slingshot (attack, defense).
var csWeaponZones = [2][4][4]int{
	{{62, 103, 72, 112}, {88, 104, 124, 111}, {116, 105, 124, 112}, {73, 86, 105, 103}},
	{{61, 88, 93, 108}, {92, 89, 127, 111}, {84, 52, 102, 66}, {0, 0, 0, 0}},
}

type csShot struct {
	npc, x, y int
	until     time.Time
}

type csHit struct {
	npc, target int
	at          time.Time
}

// csSlingTalk is FUN_0042a2a0 / FUN_0042a380: the slingshot window
// (interface 15 attack, 16 defense) and C1 08 B7 00 01 type index.
func (w *World) csSlingTalk(o, npc *Object) {
	typ := byte(1)
	ifType := 15
	if npc.Class == csClassSlingD {
		typ, ifType = 2, 16
	}
	items(o).If = ifState{Use: true, Type: ifType}
	w.send(o, packetCSWeaponWindow(typ, npc.Index))
}

// handleCSWeaponUse is B7 01 (FUN_0042a460): [4..5] slingshot index
// [6] target zone 1..4.
func (w *World) handleCSWeaponUse(o *Object, p []byte) {
	if len(p) < 7 || !o.IsPlayer() || w.cs() == nil {
		return
	}
	st := items(o).If
	if !st.Use || (st.Type != 15 && st.Type != 16) {
		return
	}
	idx := int(p[4])<<8 | int(p[5])
	npc := w.csObj(idx)
	if npc == nil || npc.Type != TypeNPC || (npc.Class != csClassSlingA && npc.Class != csClassSlingD) {
		return
	}
	typ := npc.Class - csClassSlingA
	zone := int(p[6]) - 1
	x, y := 0, 0
	if zone >= 0 && zone < 4 {
		z := csWeaponZones[typ][zone]
		if z[2] > z[0] && z[3] > z[1] {
			x = w.rand(z[2]-z[0]) + z[0]
			y = w.rand(z[3]-z[1]) + z[1]
		}
	}
	w.send(o, packetCSWeaponUse(1, idx, byte(typ+1), x, y))
	other := packetCSWeaponUse(2, idx, byte(typ+1), x, y)
	for _, e := range npc.VP2 {
		if e.State != VPEmpty && e.Type == byte(TypePlayer) && e.Index != o.Index && e.State == VPVisible {
			w.sendIndex(e.Index, other)
		}
	}
	// FUN_0042a750: the players within 6 cells of the target see the shot.
	shot := packetCSWeaponShot(byte(typ+1), x, y)
	for i := PlayerStart; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State == StatePlaying && t.Map == npc.Map && abs(t.X-x) <= 6 && abs(t.Y-y) <= 6 {
			w.send(t, shot)
		}
	}
	c := w.cs()
	if len(c.shots) < 50 {
		c.shots = append(c.shots, csShot{npc: idx, x: x, y: y, until: w.now().Add(csWeaponDelay)})
	}
}

// handleCSWeaponHit is B7 04 (FUN_0042a620): the client reports the
// landing; everything within 3 cells of the target is hit 100 ms later.
func (w *World) handleCSWeaponHit(o *Object, p []byte) {
	if len(p) < 6 || !o.IsPlayer() || w.cs() == nil {
		return
	}
	st := items(o).If
	if !st.Use || (st.Type != 15 && st.Type != 16) {
		return
	}
	idx := int(p[4])<<8 | int(p[5])
	npc := w.csObj(idx)
	if npc == nil || npc.Type != TypeNPC || (npc.Class != csClassSlingA && npc.Class != csClassSlingD) {
		return
	}
	c := w.cs()
	si := -1
	for i, s := range c.shots {
		if s.npc == idx {
			si = i
			break
		}
	}
	if si < 0 {
		return
	}
	s := c.shots[si]
	c.shots = append(c.shots[:si], c.shots[si+1:]...)
	for i := 0; i < MaxObjects; i++ {
		t := w.objects[i]
		if t.State != StatePlaying || t.Map != npc.Map || abs(t.X-s.x) > 3 || abs(t.Y-s.y) > 3 {
			continue
		}
		if len(c.hits) < 100 {
			c.hits = append(c.hits, csHit{npc: idx, target: i, at: w.now().Add(csWeaponHit)})
		}
	}
}

// csWeaponProc is FUN_00429d70 / FUN_00429e40: due hits land, old shots
// expire.
func (w *World) csWeaponProc() {
	c := w.cs()
	now := w.now()
	keep := c.hits[:0]
	for _, h := range c.hits {
		if now.Before(h.at) {
			keep = append(keep, h)
			continue
		}
		npc, t := w.csObj(h.npc), w.csObj(h.target)
		if npc != nil && t != nil && npc != t {
			w.csWeaponDamage(npc, t)
		}
	}
	c.hits = keep
	shots := c.shots[:0]
	for _, s := range c.shots {
		if now.Before(s.until) {
			shots = append(shots, s)
		}
	}
	c.shots = shots
}

// csWeaponDamage is FUN_0042a0c0: CastleSiegeOffensiveWeaponDamage (0xDD)
// or DefensiveWeaponDamage (0xDE) off the life of a living target on the
// same map (traps, slingshots and attribute-6 monsters are spared).
func (w *World) csWeaponDamage(npc, t *Object) {
	if npc.Map != t.Map || !t.Alive || t.Live != LiveNormal || t.Teleporting {
		return
	}
	if t.IsPlayer() && (t.P.LogoutCountdown > 0 || t.P.Moving || gmFull(t)) {
		return
	}
	if t.Type == TypeMonster && t.Mon != nil && (t.Mon.Attribute == 6 || (t.Class >= 100 && t.Class <= 0x6E) ||
		t.Class == 0x20B || t.Class == csClassSlingA || t.Class == csClassSlingD) {
		return
	}
	dmg := 0
	switch npc.Class {
	case csClassSlingA:
		dmg = w.cfg.Int("Event", "CastleSiegeOffensiveWeaponDamage", 0)
	case csClassSlingD:
		dmg = w.cfg.Int("Event", "CastleSiegeDefensiveWeaponDamage", 0)
	}
	setObjLife(t, max(objLife(t)-float32(dmg), 0))
	w.lifeCheck(npc, t, dmg, 0, 0)
}

// --- per-object proc ---------------------------------------------------------

// csObjectProc is the castle part of FUN_00510810 (1 s).
func (w *World) csObjectProc() {
	c := w.cs()
	if c == nil {
		return
	}
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || o.Mon == nil {
			continue
		}
		switch o.Class {
		case csClassCrown:
			w.csCrownProc()
		case csClassSwitch1, csClassSwitch2:
			w.csSwitchProc(o)
		case csClassStone:
			if o.CS != nil {
				w.csStoneProc(o)
			}
		case csClassStatue:
			w.csStatueProc(o)
		}
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.IsPlayer() {
			w.csDecayCrown(o)
		}
	}
}

func init() {
	tick1000Hooks = append(tick1000Hooks, (*World).csObjectProc)
	tick100Hooks = append(tick100Hooks, func(w *World) {
		if w.cs() != nil {
			w.csWeaponProc()
		}
	})
}
