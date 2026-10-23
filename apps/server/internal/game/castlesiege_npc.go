package game

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Castle NPCs: the gates and guardian statues kept in MuCastle_NPC, the
// gate levers, the battle NPCs of CastleSiege.dat (canons, shield,
// slingshots), their purchase, repair and upgrades.

const (
	csIfLever        = 12     // obj+0x3d4 type 0x0c set by the lever talk
	csEffGateOpen    = 13     // "Castle Gate State" (EffectList.txt)
	csEffCrown       = 20     // "Guild State 6", the open crown
	csLeverLife      = 10000  // FUN_0041dde0: 0x461c4000
	csLeverDY        = 4      // the lever stands 4 cells south of the gate (FUN_0041dd70)
	csJewelGuardian  = 0x1C1F // 14,31
	csGateRepairBase = 200000 // FUN_00420b20
	csStatueRepair   = 1000000
)

func (c *castleSiege) npcEntry(class, index int) *csNPC {
	for _, n := range c.npcs {
		if n.Class == class && n.Index == index {
			return n
		}
	}
	return nil
}

// csNPCObj returns the live object of an entry (FUN_0050cbd0 + the class
// check of the callers), or nil.
func (w *World) csNPCObj(n *csNPC) *Object {
	if n == nil || n.Obj < 0 || n.Obj >= MaxObjects {
		return nil
	}
	o := w.objects[n.Obj]
	if o.State != StatePlaying || o.Class != n.Class {
		return nil
	}
	return o
}

// csSpawnObject places a castle object on map 30 with no regen
// (obj+0x294 = 0xFFFF, obj+0x290 = 0): it is removed when it dies.
func (w *World) csSpawnObject(class, x, y, dir int) *Object {
	o := w.spawnFixedNPC(class, csMap, x, y, dir)
	if o == nil {
		return nil
	}
	o.Mon.RegenTime = 0
	o.Mon.Attribute = 3
	return o
}

// csGateBlock is FUN_0041e5a0: the 6x3 cells (x-2..x+3, y..y+2) of a gate
// get the 0x10 attribute while it is closed.
func (w *World) csGateBlock(x, y int, open bool) {
	t := w.terrain(csMap)
	if t == nil || x < 0 || x > 0xFF || y < 0 || y > 0xFF {
		return
	}
	for xx := x - 2; xx <= x+3; xx++ {
		for yy := y; yy <= y+2; yy++ {
			if xx < 0 || yy < 0 || xx >= t.Width || yy >= t.Height {
				continue
			}
			a := t.Get(xx, yy)
			if open {
				t.Set(xx, yy, a&^gamedata.AttrWall)
			} else {
				t.Set(xx, yy, a|gamedata.AttrWall)
			}
		}
	}
}

// csSetGate opens (effect 13, no block) or closes a gate.
func (w *World) csSetGate(o *Object, open bool) {
	w.csGateBlock(o.X, o.Y, open)
	if eventEffectHook != nil {
		eventEffectHook(w, o, csEffGateOpen, open)
	}
	if open {
		csOf(o).GateOpen = 1
	} else {
		csOf(o).GateOpen = 0
	}
}

// csCreateNPCObject is the body shared by FUN_0041dab0 and FUN_00423500:
// the object of a DB NPC with its life, defense and (gates) lever.
func (w *World) csCreateNPCObject(n *csNPC) *Object {
	c := w.cs()
	o := w.csSpawnObject(n.Class, n.X, n.Y, n.Dir)
	if o == nil {
		return nil
	}
	m := o.Mon
	m.Life, m.MaxLife = float32(n.Life), float32(n.MaxLife)
	cs := csOf(o)
	cs.NPCKind = 1
	switch n.Class {
	case csClassGate:
		def := c.data.GateDefense[clampLevel(n.DF)]
		m.Defense, m.MagicDefense = def, def
		cs.DFLevel = byte(n.DF)
		w.csSetGate(o, true)
		if lv := w.csLeverOf(n); lv != nil {
			csOf(lv).Link = o.Index
			cs.Link = lv.Index
		} else if lv := w.csCreateLever(n.X, n.Y+csLeverDY); lv != nil {
			n.Lever = lv.Index
			csOf(lv).Link = o.Index
			cs.Link = lv.Index
		}
	case csClassStatue:
		def := c.data.StatueDefense[clampLevel(n.DF)]
		m.Defense, m.MagicDefense = def, def
		cs.DFLevel, cs.RGLevel = byte(n.DF), byte(n.RG)
	}
	n.Obj, n.Live = o.Index, 2
	return o
}

func clampLevel(l int) int { return min(max(l, 0), 3) }

// csLeverOf returns the lever object of a gate entry when still valid.
func (w *World) csLeverOf(n *csNPC) *Object {
	if n.Lever < 0 || n.Lever >= MaxObjects {
		return nil
	}
	if o := w.objects[n.Lever]; o.State == StatePlaying && o.Class == csClassLever {
		return o
	}
	return nil
}

// csCreateLever is FUN_0041dde0.
func (w *World) csCreateLever(x, y int) *Object {
	if x < 0 || x > 0xFF || y < 0 || y > 0xFF {
		return nil
	}
	o := w.csSpawnObject(csClassLever, x, y, 1)
	if o == nil {
		return nil
	}
	o.Mon.Life, o.Mon.MaxLife = csLeverLife, csLeverLife
	cs := csOf(o)
	cs.NPCKind = 2
	cs.Link = -1
	return o
}

// csCheckLevers is FUN_0041dd70: every gate entry gets a lever.
func (w *World) csCheckLevers() {
	for _, n := range w.cs().npcs {
		if n.Class == csClassGate && w.csLeverOf(n) == nil {
			if lv := w.csCreateLever(n.X, n.Y+csLeverDY); lv != nil {
				n.Lever = lv.Index
			}
		}
	}
}

// csCreateDBNPCs is FUN_0041dab0: the entries waiting to be created.
func (w *World) csCreateDBNPCs() {
	for _, n := range w.cs().npcs {
		if n.Live == 1 {
			w.csCreateNPCObject(n)
		}
	}
}

// csSaveDefaultNPCs is FUN_00426070 (DataServer 0x84, first start): the
// DBSave rows go back to the CastleSiege.dat defaults and replace
// MuCastle_NPC; the answer (FUN_00424f50) creates them.
func (w *World) csSaveDefaultNPCs() {
	c, st := w.cs(), w.csStore()
	var rows []store.CastleNPCData
	for _, n := range c.npcs {
		if !n.DBSave {
			continue
		}
		n.X, n.Y, n.DF, n.Dir = n.BaseX, n.BaseY, n.BaseDF, n.BaseDir
		n.Live, n.InUse = 1, true
		n.RG, n.MaxLife, n.Life = n.BaseRG, n.BaseMaxLife, n.BaseLife
		rows = append(rows, store.CastleNPCData{Number: n.Class, Index: n.Index, DFLevel: n.DF, RGLevel: n.RG,
			MaxHP: n.MaxLife, HP: n.Life, X: n.X, Y: n.Y, Dir: n.Dir})
	}
	if st == nil {
		return
	}
	group := c.group
	w.async(func(ctx context.Context) func() {
		if err := st.ReplaceCastleNPCs(ctx, group, rows); err != nil {
			return nil
		}
		return func() {
			if c := w.cs(); c != nil && !c.npcsCreated {
				w.csCreateDBNPCs()
				c.npcsCreated = true
			}
		}
	})
}

// csSaveNPCs is FUN_00426810 (DataServer 0x89): the living DBSave NPCs
// replace MuCastle_NPC (destroyed ones disappear from it).
func (w *World) csSaveNPCs() {
	c, st := w.cs(), w.csStore()
	if st == nil {
		return
	}
	var rows []store.CastleNPCData
	for _, n := range c.npcs {
		if !n.DBSave {
			continue
		}
		o := w.csNPCObj(n)
		if o == nil || !o.Alive || o.Mon.Life <= 0 {
			continue
		}
		rows = append(rows, store.CastleNPCData{Number: n.Class, Index: n.Index, DFLevel: n.DF, RGLevel: n.RG,
			MaxHP: int(o.Mon.MaxLife), HP: int(o.Mon.Life), X: n.X, Y: n.Y, Dir: n.Dir})
	}
	group := c.group
	w.async(func(ctx context.Context) func() {
		st.ReplaceCastleNPCs(ctx, group, rows)
		return nil
	})
}

// csIsCastleObject reports a castle object on map 30 (obj+0x974 set).
func csIsCastleObject(o *Object) bool {
	return o.State == StatePlaying && o.Map == csMap && o.CS != nil && o.CS.NPCKind != 0
}

// csClearBattleNPCs is FUN_0041d970: every castle object but the DB NPCs
// and the levers goes away (life stones and mercenaries update their
// counters).
func (w *World) csClearBattleNPCs() {
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if !csIsCastleObject(o) || o.CS.NPCKind == 1 || o.Class == csClassLever {
			continue
		}
		switch o.Class {
		case csClassStone:
			w.csStoneGone(o)
		case csClassArcher, csClassSpear:
			c := w.cs()
			c.mercenaries = max(c.mercenaries-1, 0)
		}
		w.deleteMonster(o)
	}
}

// csSpawnBattleNPCs is FUN_0041f4a0: the non-DBSave rows (canons, shield,
// slingshots) appear for the battle.
func (w *World) csSpawnBattleNPCs() {
	for _, n := range w.cs().npcs {
		if n.DBSave {
			continue
		}
		o := w.csSpawnObject(n.Class, n.X, n.Y, n.Dir)
		if o == nil {
			continue
		}
		o.Mon.Life, o.Mon.MaxLife = float32(n.Life), float32(n.MaxLife)
		csOf(o).NPCKind = 2
		if n.Class == csClassSlingA {
			csOf(o).NPCKind = 3
		}
	}
}

// csCloseGates is FUN_0041f9c0 (siege start).
func (w *World) csCloseGates() {
	for _, n := range w.cs().npcs {
		if n.Class != csClassGate {
			continue
		}
		if o := w.csNPCObj(n); o != nil {
			w.csSetGate(o, false)
		}
	}
}

// csRefreshNPCs is FUN_0041e080: the defense of the DB NPC objects from
// their levels.
func (w *World) csRefreshNPCs() {
	c := w.cs()
	for _, n := range c.npcs {
		if n.Live < 1 {
			continue
		}
		o := w.csNPCObj(n)
		if o == nil || o.CS == nil || o.CS.NPCKind == 0 {
			continue
		}
		switch n.Class {
		case csClassGate:
			o.CS.DFLevel = byte(n.DF)
			o.Mon.Defense = c.data.GateDefense[clampLevel(n.DF)]
		case csClassStatue:
			o.Mon.Defense = c.data.StatueDefense[clampLevel(n.DF)]
			o.CS.DFLevel, o.CS.RGLevel = byte(n.DF), byte(n.RG)
		}
	}
}

// csStatueAlive is FUN_0041eb40: a living guardian statue on map 30.
func (w *World) csStatueAlive() bool {
	for i := 0; i < SummonStart; i++ {
		o := w.objects[i]
		if o.State == StatePlaying && o.Class == csClassStatue && o.Map == csMap && o.Alive && o.Mon != nil {
			return true
		}
	}
	return false
}

// csRemoveShield is FUN_0041f970: the shield (class 0xD7) in front of the
// crown room disappears.
func (w *World) csRemoveShield() {
	for i := 0; i < SummonStart; i++ {
		if o := w.objects[i]; o.State == StatePlaying && o.Class == csClassShield && o.Mon != nil {
			w.deleteMonster(o)
		}
	}
}

// csStatueGone opens the crown room when the last statue fell during the
// battle (FUN_0041e180 / FUN_0041df10).
func (w *World) csStatueGone() {
	c := w.cs()
	if c.state == csStart && !w.csStatueAlive() {
		w.csRemoveShield()
		c.crownRoom = true
		w.log.Info("[Castle Siege] Crown Room entrance is open")
	}
}

// csNPCKilled is FUN_0041e180 (from the monster death FUN_004b70b0): the
// entry returns to its defaults; out of the battle the row is deleted
// (DataServer 0x80 0x16).
func (w *World) csNPCKilled(o *Object) {
	c := w.cs()
	if c == nil || o.Index < 0 {
		return
	}
	var n *csNPC
	for _, e := range c.npcs {
		if e.Class == o.Class && e.Obj == o.Index && e.Live > 0 {
			n = e
			break
		}
	}
	if n == nil {
		return
	}
	n.RG, n.Dir, n.MaxLife, n.DF = n.BaseRG, n.BaseDir, n.BaseMaxLife, n.BaseDF
	n.Life, n.X, n.Y = n.BaseLife, n.BaseX, n.BaseY
	n.Obj, n.Live = -1, 0
	switch n.Class {
	case csClassGate:
		w.csSetGate(o, true)
		w.log.Info("[Castle Siege] Castle Gate has been destroyed.", "index", n.Index, "x", n.X, "y", n.Y)
	case csClassStatue:
		w.log.Info("[Castle Siege] Defend Statue has been destroyed.", "index", n.Index, "x", n.BaseX, "y", n.BaseY)
		w.csStatueGone()
	}
	if c.state != csStart {
		if st := w.csStore(); st != nil {
			group, class, index := c.group, n.Class, n.Index
			w.async(func(ctx context.Context) func() {
				st.RemoveCastleNPC(ctx, group, class, index)
				return nil
			})
		}
	}
}

// csCheckDestroyed is FUN_0041df10 (every 5 s of the battle): entries
// whose object vanished are treated as destroyed.
func (w *World) csCheckDestroyed() {
	for _, n := range w.cs().npcs {
		if (n.Class != csClassGate && n.Class != csClassStatue) || n.Obj == -1 {
			continue
		}
		if o := w.csNPCObj(n); o != nil && o.Alive {
			continue
		}
		if n.Class == csClassGate {
			if n.Obj >= 0 && n.Obj < MaxObjects {
				w.csSetGate(w.objects[n.Obj], true)
			}
			w.log.Info("[Castle Siege] Castle Gate has been destroyed.", "index", n.Index)
		} else {
			w.log.Info("[Castle Siege] Defend Statue has been destroyed.", "index", n.Index)
			w.csStatueGone()
		}
		n.Obj = -1
	}
}

// --- castle management (owner guild master / assistant) ------------------------

// csGuildName is obj+0x2fc; csGuildStatus obj+0x30c.
func csGuildName(o *Object) string {
	if o.P == nil || o.P.Guild == nil || o.P.Guild.Number == 0 {
		return ""
	}
	return o.P.Guild.Name
}

func csGuildStatus(o *Object) int {
	if o.P == nil || o.P.Guild == nil || o.P.Guild.Number == 0 {
		return -1
	}
	return o.P.Guild.Status
}

// csOwnerGuild is FUN_0041e560 / FUN_0041e9b0: the player's guild owns
// the castle.
func (w *World) csOwnerGuild(o *Object) bool {
	c := w.cs()
	g := csGuildName(o)
	return c != nil && g != "" && g == c.owner
}

// csOwnerAlliance is FUN_0041ea30: the master guild of the player's
// alliance owns the castle.
func (w *World) csOwnerAlliance(o *Object) bool {
	c := w.cs()
	if c == nil || csGuildName(o) == "" {
		return false
	}
	return csUnionMaster(w, o) == c.owner && c.owner != ""
}

// csUnionMaster is the alliance master guild name of the player's guild
// (FUN_00502900 + CUnionInfo+0x10), "" without an alliance.
func csUnionMaster(w *World, o *Object) string {
	n := guildNumberOf(o)
	if n == 0 {
		return ""
	}
	mgr := w.guildMgr()
	g, ok := mgr.byNumber(n)
	if !ok || g.Union == 0 {
		return ""
	}
	if u, ok := mgr.byNumber(g.Union); ok {
		return u.Name
	}
	return ""
}

// csMasterOrAssistant is the obj+0x30c == 0x80 || 0x40 test.
func csMasterOrAssistant(o *Object) bool {
	s := csGuildStatus(o)
	return s == 0x80 || s == 0x40
}

// csManager is the common test of B2 05/06/07 and B3: playing player of
// the owner guild, master or assistant.
func (w *World) csManager(o *Object) bool {
	return w.csEnabled() && o.State == StatePlaying && o.IsPlayer() && csGuildName(o) != "" &&
		csMasterOrAssistant(o) && w.csOwnerGuild(o)
}

// csNPCCost is the price of a new gate (5 zen per life point) or statue
// (3 per point) in FUN_00420930.
func csNPCCost(n *csNPC) int64 {
	switch n.Class {
	case csClassGate:
		return int64(n.BaseMaxLife) * 5
	case csClassStatue:
		return int64(n.BaseMaxLife) * 3
	}
	return 0
}

// handleCSNPCBuy is B2 05 (FUN_00420930): [4] class u32 [8] index u32.
func (w *World) handleCSNPCBuy(o *Object, p []byte) {
	if len(p) < 12 || !w.csManager(o) {
		return
	}
	c := w.cs()
	number, index := int(binary.LittleEndian.Uint32(p[4:])), int(binary.LittleEndian.Uint32(p[8:]))
	n := c.npcEntry(number, index)
	if n == nil {
		return
	}
	if n.Live > 0 {
		if t := w.csNPCObj(n); t != nil && t.Alive {
			w.send(o, packetCSNPCBuy(0, number, index))
			return
		}
	}
	cost := csNPCCost(n)
	if o.P.Char.Money < cost {
		w.send(o, packetCSNPCBuy(3, number, index))
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	row := store.CastleNPCData{Number: number, Index: index, DFLevel: n.DF, RGLevel: n.RG, MaxHP: n.MaxLife,
		HP: n.Life, X: n.X, Y: n.Y, Dir: n.Dir}
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, err := st.BuyCastleNPC(ctx, group, row)
		if err != nil {
			r = 0
		}
		return func() { w.csNPCBuyDone(idx, r, number, index, cost) }
	})
}

// csNPCBuyDone is FUN_00423500 (DataServer 0x80 0x03 answer).
func (w *World) csNPCBuyDone(idx, result, number, index int, cost int64) {
	o := w.objects[idx]
	if o.State != StatePlaying || o.P == nil || o.P.Char == nil {
		return
	}
	if result == 1 {
		n := w.cs().npcEntry(number, index)
		if n != nil {
			if t := w.csNPCObj(n); t != nil && t.Alive {
				return
			}
			n.Live = 1
			w.csCreateNPCObject(n)
		}
		o.P.Char.Money -= cost
		w.sendMoney(o)
	}
	w.send(o, packetCSNPCBuy(byte(result), number, index))
}

// csRepairCost is FUN_00420b20: gates (lost life + DF level x 200000) x 5,
// statues (DF + RG) x 1000000 + lost life x 3.
func csRepairCost(n *csNPC, t *Object) int64 {
	lost := int64(math.Round(float64(t.Mon.MaxLife - t.Mon.Life)))
	switch n.Class {
	case csClassGate:
		return (lost + int64(t.CS.DFLevel)*csGateRepairBase) * 5
	case csClassStatue:
		return int64(int(t.CS.RGLevel)+int(t.CS.DFLevel))*csStatueRepair + lost*3
	}
	return 0
}

// handleCSNPCRepair is B2 06 (FUN_00420b20): [4] class [8] index.
func (w *World) handleCSNPCRepair(o *Object, p []byte) {
	if len(p) < 12 || !w.csManager(o) {
		return
	}
	c := w.cs()
	number, index := int(binary.LittleEndian.Uint32(p[4:])), int(binary.LittleEndian.Uint32(p[8:]))
	n := c.npcEntry(number, index)
	t := w.csNPCObj(n)
	if n == nil || t == nil || !t.Alive {
		w.send(o, packetCSNPCRepair(0, number, index, 0, 0))
		return
	}
	csOf(t)
	if t.Mon.Life > t.Mon.MaxLife {
		t.Mon.Life = t.Mon.MaxLife
		w.send(o, packetCSNPCRepair(1, number, index, int(t.Mon.MaxLife), int(t.Mon.Life)))
		return
	}
	cost := csRepairCost(n, t)
	if cost > o.P.Char.Money {
		w.send(o, packetCSNPCRepair(3, number, index, 0, 0))
		return
	}
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, hp, maxHP, err := st.RepairCastleNPC(ctx, group, number, index)
		if err != nil {
			r = 0
		}
		return func() { w.csNPCRepairDone(idx, r, number, index, hp, maxHP, cost) }
	})
}

// csNPCRepairDone is FUN_00423910.
func (w *World) csNPCRepairDone(idx, result, number, index, hp, maxHP int, cost int64) {
	o := w.objects[idx]
	if o.State != StatePlaying || o.P == nil || o.P.Char == nil {
		return
	}
	if result == 1 {
		if n := w.cs().npcEntry(number, index); n != nil && n.Live > 0 {
			n.Life, n.MaxLife = hp, maxHP
			if t := w.csNPCObj(n); t != nil && t.CS != nil && t.CS.NPCKind != 0 {
				t.Mon.Life = t.Mon.MaxLife
			}
			o.P.Char.Money -= cost
			w.sendMoney(o)
		}
	}
	w.send(o, packetCSNPCRepair(byte(result), number, index, hp, maxHP))
}

// Upgrade types of B2 07.
const (
	csUpDefense = 1
	csUpRegen   = 2
	csUpLife    = 3
)

// csLifeLevel maps a max life to its upgrade level 0..3 (FUN_00420d80).
func csLifeLevel(rows [3]gamedata.CastleUpgrade, life int) int {
	l := 0
	for i, r := range rows {
		if life >= r.Value {
			l = i + 1
		}
	}
	return l
}

// handleCSNPCUpgrade is B2 07 (FUN_00420d80): [4] class [8] index
// [12] type [16] value. Results: 7 unknown NPC, 6 wrong level, 5 wrong
// type, 3 no zen, 4 not enough Jewels of Guardian.
func (w *World) handleCSNPCUpgrade(o *Object, p []byte) {
	if len(p) < 20 || !w.csManager(o) {
		return
	}
	c := w.cs()
	number, index := int(binary.LittleEndian.Uint32(p[4:])), int(binary.LittleEndian.Uint32(p[8:]))
	typ, value := int(int32(binary.LittleEndian.Uint32(p[12:]))), int(int32(binary.LittleEndian.Uint32(p[16:])))
	fail := func(r byte) { w.send(o, packetCSNPCUpgrade(r, number, index, typ, value)) }
	n := c.npcEntry(number, index)
	if n == nil {
		fail(7)
		return
	}
	var row gamedata.CastleUpgrade
	newValue := value
	level := func(cur int) bool { return value >= 1 && value <= 3 && value == cur+1 }
	switch number {
	case csClassGate:
		switch typ {
		case csUpDefense:
			if !level(n.DF) {
				fail(6)
				return
			}
			row = c.data.GateDefenseUpgrade[value-1]
		case csUpLife:
			cur, want := csLifeLevel(c.data.GateLifeUpgrade, n.MaxLife), csLifeLevel(c.data.GateLifeUpgrade, value)
			if want != cur+1 {
				w.log.Warn("[Castle Siege] Upgrade NPC Error: Life Gate", "cur", n.MaxLife, "want", value)
			}
			if want == 0 || want > 3 || want != cur+1 {
				fail(6)
				return
			}
			row = c.data.GateLifeUpgrade[want-1]
			newValue = row.Value
		default:
			fail(5)
			return
		}
	case csClassStatue:
		switch typ {
		case csUpDefense:
			if !level(n.DF) {
				fail(6)
				return
			}
			row = c.data.StatueDefenseUpgrade[value-1]
		case csUpRegen:
			if !level(n.RG) {
				fail(6)
				return
			}
			row = c.data.StatueRegenUpgrade[value-1]
		case csUpLife:
			cur, want := csLifeLevel(c.data.StatueLifeUpgrade, n.MaxLife), csLifeLevel(c.data.StatueLifeUpgrade, value)
			if want != cur+1 {
				w.log.Warn("[Castle Siege] Upgrade NPC Error: Life Statue", "cur", n.MaxLife, "want", value)
			}
			if want == 0 || want > 3 || want != cur+1 {
				fail(6)
				return
			}
			row = c.data.StatueLifeUpgrade[want-1]
			newValue = row.Value
		default:
			fail(5)
			return
		}
	default:
		fail(6)
		return
	}
	if o.P.Char.Money < int64(row.Money) {
		fail(3)
		return
	}
	if w.countItems(o, csJewelGuardian, 0) < row.Count {
		fail(4)
		return
	}
	w.deleteItems(o, csJewelGuardian, 0, row.Count)
	o.P.Char.Money -= int64(row.Money)
	w.sendMoney(o)
	csOf(o).Upgraded = true
	st := w.csStore()
	if st == nil {
		return
	}
	group, idx := c.group, o.Index
	w.async(func(ctx context.Context) func() {
		r, err := st.UpgradeCastleNPC(ctx, group, number, index, typ, newValue)
		if err != nil {
			r = 0
		}
		return func() { w.csNPCUpgradeDone(idx, r, number, index, typ, newValue) }
	})
}

// csNPCUpgradeDone is FUN_00423ab0: FUN_0041e350 (defense level; both
// classes take the gate defense table), FUN_0041e400 (regen level) and
// FUN_0041e480 (life).
func (w *World) csNPCUpgradeDone(idx, result, number, index, typ, value int) {
	c := w.cs()
	if result != 0 {
		if n := c.npcEntry(number, index); n != nil && n.Live > 0 {
			t := w.csNPCObj(n)
			if t != nil && t.CS != nil && t.CS.NPCKind != 0 {
				switch {
				case typ == csUpDefense && (number == csClassGate || number == csClassStatue):
					n.DF = value
					def := c.data.GateDefense[clampLevel(value)]
					t.Mon.Defense, t.Mon.MagicDefense = def, def
					t.CS.DFLevel = byte(value)
				case typ == csUpRegen && number == csClassStatue:
					n.RG = value
					t.CS.RGLevel = byte(value)
				case typ == csUpLife:
					n.MaxLife, n.Life = value, value
					t.Mon.Life, t.Mon.MaxLife = float32(value), float32(value)
				}
			}
		}
	}
	if o := w.objects[idx]; o.State == StatePlaying && o.IsPlayer() {
		w.send(o, packetCSNPCUpgrade(byte(result), number, index, typ, value))
	}
}

// handleCSNPCList is B3 (FUN_004219b0): [3] 1 gates, 2 statues.
func (w *World) handleCSNPCList(o *Object, p []byte) {
	if len(p) < 4 || !w.csEnabled() || !o.IsPlayer() || csGuildName(o) == "" {
		return
	}
	if !w.csOwnerGuild(o) || !csMasterOrAssistant(o) {
		w.send(o, packetCSNPCList(2, nil))
		return
	}
	class := 0
	switch p[3] {
	case 1:
		class = csClassGate
	case 2:
		class = csClassStatue
	}
	var list []csNPCListEntry
	for _, n := range w.cs().npcs {
		if n.Class != class {
			continue
		}
		e := csNPCListEntry{Class: n.Class, Index: n.Index, DF: n.DF, RG: n.RG, MaxLife: n.BaseMaxLife, X: n.X, Y: n.Y}
		if t := w.csNPCObj(n); t != nil && t.Alive && t.Mon.Life != 0 && t.CS != nil && t.CS.NPCKind != 0 {
			e.Live = true
			e.MaxLife, e.Life = int(math.Round(float64(t.Mon.MaxLife))), int(math.Round(float64(t.Mon.Life)))
			e.X, e.Y = t.X, t.Y
		}
		list = append(list, e)
	}
	w.send(o, packetCSNPCList(1, list))
}

// --- gates ----------------------------------------------------------------

// csLeverTalk is FUN_00426cb0 (NPC 0xDB): during the battle only the
// defenders, otherwise the owner guild and its alliance may use it.
func (w *World) csLeverTalk(o, lever *Object) {
	c := w.cs()
	result, gate := byte(4), -1
	allowed := false
	if c.state == csStart {
		allowed = csSide(o) == 1
	} else {
		allowed = w.csOwnerGuild(o) || w.csOwnerAlliance(o)
	}
	if allowed {
		result = 2
		if l := csOf(lever).Link; l >= 0 && l < MaxObjects {
			if g := w.objects[l]; g.State == StatePlaying && g.Class == csClassGate {
				result, gate = 1, l
			}
		}
	}
	w.send(o, packetCSLever(result, gate))
	if result == 1 {
		items(o).If = ifState{Use: true, Type: csIfLever}
	}
}

// handleCSGateOperate is B2 12 (FUN_004221f0): [4] 1 open / 0 close,
// [5..6] gate index.
func (w *World) handleCSGateOperate(o *Object, p []byte) {
	if len(p) < 7 || !w.csEnabled() || !o.IsPlayer() || csGuildName(o) == "" {
		return
	}
	c := w.cs()
	if c.state == csStart {
		if csSide(o) != 1 {
			return
		}
	} else if !w.csOwnerGuild(o) && !w.csOwnerAlliance(o) {
		return
	}
	op := p[4]
	gi := int(p[5])<<8 | int(p[6])
	if gi < 0 || gi >= MaxObjects || w.objects[gi].State != StatePlaying || w.objects[gi].Class != csClassGate {
		w.send(o, packetCSGateOperate(2, 0, -1))
		return
	}
	for _, n := range c.npcs {
		if n.Obj == gi && n.Class == csClassGate && n.Live == 2 {
			// The block follows the operation but the effect 13 is added in
			// both cases (CastleSiege.exe 0x41a35b); the viewport
			// (FUN_00515510) re-syncs it with obj+0x975.
			g := w.objects[gi]
			w.csGateBlock(g.X, g.Y, op != 0)
			if eventEffectHook != nil {
				eventEffectHook(w, g, csEffGateOpen, true)
			}
			csOf(g).GateOpen = op
			w.send(o, packetCSGateOperate(1, op, gi))
			w.broadcast(g, packetCSGateState(op, gi))
			return
		}
	}
	w.send(o, packetCSGateOperate(0, 0, -1))
}

// csSignOfLord is the Sign of Lord (14,21) +3 counted by B2 04.
var csSignOfLord = item.Make(14, 21)
