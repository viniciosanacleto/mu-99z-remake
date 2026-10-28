package game

import (
	"encoding/binary"
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Summon Party (Dark Lord), Teleport Ally (B0, Dark Wizard skill 15) and
// the Castle Siege guild skills
// (docs/server-plan/08c-skills-efeitos-pvp.md, "Skills de guild do Castle
// Siege, Summon Party e Teleport Ally"):
//
//   - 63 Summon Party (FUN_004fc1a0): every other party member that
//     MoveSummon.txt (CMoveSummon, FUN_004bff20) lets in gets a 5-second
//     countdown (obj+0x5d2, messages 272/273) to a cell near the caster
//     (obj+0x5d4 map, 0x5d5 X, 0x5d6 Y); FUN_00510170 counts it down each
//     second and FUN_00510000 moves the member; walking (FUN_004d8b80) or
//     a gate move (FUN_0050f6d0) cancels it with message 272;
//   - B0 Teleport Ally (FUN_005012a0): skill 15 ("Teleport Party") casts
//     the Teleport magic on a party member;
//   - 67..72 and 75 (FUN_004fc970, FUN_004fccc0, FUN_004fce40,
//     FUN_004fd0c0, FUN_004fd1b0, FUN_004fd330, FUN_004fd4a0): only on maps
//     30/31; they reset the kill count (C1 05 B8 01 00) and act on the
//     players of the caster's VP2 within the skill Radio.

// guildSkill dispatches FUN_004f9730 cases 0x3f, 0x43..0x48 and 0x4b.
func (w *World) guildSkill(o *Object, ref *skillRef, target, angle int) bool {
	if o.Type != TypePlayer || o.P == nil {
		return false
	}
	switch ref.Num {
	case 63:
		return w.skillSummonParty(o, ref)
	case 70:
		return w.skillInvisibility(o, ref, target)
	}
	if o.Map != csMap && o.Map != csMap+1 {
		return false
	}
	switch ref.Num {
	case 67:
		return w.skillStern(o, ref, angle)
	case 68:
		return w.skillRemoveArea(o, ref, func(t *Object) { w.removeEffect(t, effStern) })
	case 69:
		return w.skillGreaterMana(o, ref)
	case 71:
		return w.skillRemoveArea(o, ref, func(t *Object) { w.removeEffect(t, effInvisibility) })
	case 72:
		return w.skillRemoveArea(o, ref, func(t *Object) { w.clearEffects(t) })
	case 75:
		return w.skillBrand(o, ref)
	}
	return false
}

// guildSkillKillReset is the kill count reset of these skills
// (obj+0x98c = 0 and C1 05 B8 01 00, FUN_0050e0c0 with ECX = 0).
func (w *World) guildSkillKillReset(o *Object) {
	sk(o).KillCount = 0
	w.send(o, packetKillCount(0))
}

// vp2PlayersInRadio yields the players of o's VP2 within the skill Radio.
func (w *World) vp2PlayersInRadio(o *Object, num int, f func(t *Object)) {
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Type != byte(TypePlayer) {
			continue
		}
		t := w.objects[e.Index]
		if w.radioOK(num, o.X, o.Y, t.X, t.Y) {
			f(t)
		}
	}
}

// sternSector is the angle of FUN_004fc970: the byte holds the target
// cell as (y - Y + 8) << 4 | (x - X + 8); the degrees are
// atan2(dy, dx) * 180 / pi + 90 (dx = 8 - low nibble, dy = 8 - high
// nibble), truncated, plus 360 when negative.
func sternSector(b int) int {
	dx := float32(8 - b&0x0F)
	dy := float32(8 - (b>>4)&0x0F)
	deg := int(float64(float32(math.Atan2(float64(dy), float64(dx))))*180/float64(float32(math.Pi)) + 90)
	if deg < 0 {
		deg += 360
	}
	return deg / 10
}

// skillStern is FUN_004fc970 (67): the players of the sector (SkillElect
// hit box) within Radio, not teleporting, get Stern (effect 61,
// SternTimeConstA seconds) and stop where they are (FUN_00511cb0).
func (w *World) skillStern(o *Object, ref *skillRef, angle int) bool {
	sector := sternSector(angle)
	w.guildSkillKillReset(o)
	w.vp2PlayersInRadio(o, ref.Num, func(t *Object) {
		if sector > 35 || !w.hitboxHas(sector, t.X-o.X+10, t.Y-o.Y+10) || t.Teleporting {
			return
		}
		if hasEffectIndex(t, 0x86) || hasEffectIndex(t, 0xA5) {
			return
		}
		w.addEffect(t, effStern, w.cfg.Int("Skill", "SternTimeConstA", 0), [4]int{}, true)
		w.resendPosition(t, t.X, t.Y)
		w.sendSkillTo(o, ref.Num, t.Index, true)
	})
	return true
}

// skillRemoveArea is FUN_004fccc0 (68 Remove Stern: effect 61),
// FUN_004fd1b0 (71 Remove Invisibility: effect 18) and FUN_004fd330 (72
// Remove All Effect: FUN_0047a540): 0x19 to the caster, then the removal
// and 0x19 for every player within Radio.
func (w *World) skillRemoveArea(o *Object, ref *skillRef, remove func(t *Object)) bool {
	w.guildSkillKillReset(o)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	w.vp2PlayersInRadio(o, ref.Num, func(t *Object) {
		remove(t)
		w.sendSkillTo(o, ref.Num, t.Index, true)
	})
	return true
}

// skillGreaterMana is FUN_004fce40 (69): the SkillList effect (9) with
// value GreaterManaConstA for GreaterManaTimeConstA seconds on the caster
// and on the members of its guild (obj+0x308) within Radio.
func (w *World) skillGreaterMana(o *Object, ref *skillRef) bool {
	w.guildSkillKillReset(o)
	e := w.skillEffectIndex(ref.Num)
	v := [4]int{w.cfg.Int("Skill", "GreaterManaConstA", 0) & 0xFFFF}
	secs := w.cfg.Int("Skill", "GreaterManaTimeConstA", 0)
	w.addEffect(o, e, secs, v, true)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	g := guildNumberOf(o)
	if g == 0 {
		return true
	}
	w.vp2PlayersInRadio(o, ref.Num, func(t *Object) {
		if guildNumberOf(t) != g {
			return
		}
		w.addEffect(t, e, secs, v, true)
		w.sendSkillTo(o, ref.Num, t.Index, true)
	})
	return true
}

// skillInvisibility is FUN_004fd0c0 (70): a party member within Range, or
// the caster, becomes invisible (effect 18) for InvisibilityTimeConstA
// seconds.
func (w *World) skillInvisibility(o *Object, ref *skillRef, target int) bool {
	if o.Map != csMap && o.Map != csMap+1 {
		return false
	}
	if target < 0 || target >= MaxObjects || w.objects[target].Type != TypePlayer {
		return false
	}
	t := w.objects[target]
	if t != o {
		op, tp := w.partyNumber(o), w.partyNumber(t)
		if op == -1 || op != tp {
			t = o
		}
	}
	if !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	w.guildSkillKillReset(o)
	w.addEffect(t, effInvisibility, w.cfg.Int("Skill", "InvisibilityTimeConstA", 0), [4]int{}, true)
	w.sendSkillTo(o, ref.Num, t.Index, true)
	return true
}

// skillBrand is FUN_004fd4a0 (75): effect 19 for CServerInfo+0x182c
// seconds (no key reads it: 0, the effect never runs out) on the caster
// and on every player within Radio.
func (w *World) skillBrand(o *Object, ref *skillRef) bool {
	w.guildSkillKillReset(o)
	w.addEffect(o, effBrand, 0, [4]int{}, true)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	w.vp2PlayersInRadio(o, ref.Num, func(t *Object) {
		w.addEffect(t, effBrand, 0, [4]int{}, true)
		w.sendSkillTo(o, ref.Num, t.Index, true)
	})
	return true
}

// --- Summon Party ------------------------------------------------------------------

// summonState is obj+0x5d2..0x5d6.
type summonState struct {
	count   int // obj+0x5d2 (seconds)
	mp      int // obj+0x5d4
	x, y    int // obj+0x5d5 / 0x5d6
	pending bool
}

func (w *World) partySummonOf(o *Object) *summonState {
	q := w.q()
	if q.summons == nil {
		q.summons = map[int]*summonState{}
	}
	s := q.summons[o.Index]
	if s == nil {
		s = &summonState{}
		q.summons[o.Index] = s
	}
	return s
}

// skillSummonParty is FUN_004fc1a0 (63).
func (w *World) skillSummonParty(o *Object, ref *skillRef) bool {
	if pt := w.partyOf(o); pt != nil {
		for _, idx := range pt.members {
			if idx < 0 || idx >= MaxObjects || idx == o.Index {
				continue
			}
			m := w.objects[idx]
			if m.State != StatePlaying || !m.IsPlayer() {
				continue
			}
			switch {
			case !w.moveSummonOK(m, o.Map, o.X, o.Y):
				w.customMsg(m, 0x110)
			case o.Map == csMap && w.hooks.castleBattle != nil && w.hooks.castleBattle(w) && csSide(o) != csSide(m):
				w.customMsg(m, 0x110)
			default:
				x, y, ok := w.randomNearCell(o.Map, o.X, o.Y, 4, 4, 10)
				if !ok {
					x, y = o.X, o.Y
				}
				s := w.partySummonOf(m)
				*s = summonState{count: 5, mp: o.Map, x: x & 0xFF, y: y & 0xFF, pending: true}
				w.customMsg(m, 0x111, s.count)
			}
		}
	}
	w.sendSkillTo(o, ref.Num, o.Index, true)
	return true
}

// summonCancel is the obj+0x5d2 reset of FUN_004d8b80 (walk) and
// FUN_0050f6d0 (gate move): message 272.
func (w *World) summonCancel(o *Object) {
	if w.rq == nil || w.rq.summons == nil {
		return
	}
	if s := w.rq.summons[o.Index]; s != nil && s.pending {
		delete(w.rq.summons, o.Index)
		w.customMsg(o, 0x110)
	}
}

// summonSecond is the obj+0x5d2 part of FUN_00510170 (every second).
func (w *World) summonSecond() {
	if w.rq == nil {
		return
	}
	for idx, s := range w.rq.summons {
		o := w.objects[idx]
		if !s.pending || o.State != StatePlaying || !o.IsPlayer() {
			delete(w.rq.summons, idx)
			continue
		}
		s.count--
		if s.count > 0 {
			w.customMsg(o, 0x111, s.count)
			continue
		}
		if !w.moveSummonOK(o, s.mp, s.x, s.y) {
			delete(w.rq.summons, idx)
			w.customMsg(o, 0x110)
			continue
		}
		w.summonMove(o, s)
	}
}

// summonMove is FUN_00510000: on the same map the Teleport magic (0x19
// skill 6 to itself, FUN_0050e180); otherwise a direct map change with
// C3 08 1C FF map x y dir. Offline objects stay.
func (w *World) summonMove(o *Object, s *summonState) {
	if isOffline(o) {
		return
	}
	delete(w.rq.summons, o.Index)
	if o.Map == s.mp {
		w.sendSkillTo(o, teleportSkill, o.Index, true)
		w.startMagicTeleport(o, s.x, s.y)
		return
	}
	if srv := w.data.MapServer.ServerForMap(w.cfg.ServerCode, s.mp); srv != w.cfg.ServerCode {
		if srv >= 0 {
			w.startServerMove(o, srv, s.mp, s.x, s.y)
		}
		return
	}
	for _, h := range w.hooks.mapChange {
		h(w, o, o.Map, s.mp)
	}
	w.placeAt(o, -1, s.mp, s.x, s.y, o.Dir)
}

// --- MoveSummon -------------------------------------------------------------------

// moveSummonRow is a Move/MoveSummon.txt row (CMoveSummon, 0x34 bytes).
type moveSummonRow struct {
	Map, X1, Y1, X2, Y2 int
	MinLevel, MaxLevel  int
	MinReset, MaxReset  int
	AccountLevel        [4]int
}

// loadMoveSummon reads the MoveSummon table of config/data/move.toml.
func loadMoveSummon(configDir string) ([]moveSummonRow, error) {
	rows, err := gamedata.LoadMoveSummon(configDir)
	if err != nil {
		return nil, err
	}
	var out []moveSummonRow
	for _, r := range rows {
		out = append(out, moveSummonRow{Map: r.Map, X1: r.StartX, Y1: r.StartY, X2: r.EndX, Y2: r.EndY,
			MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset, MaxReset: r.MaxReset, AccountLevel: r.AL})
	}
	return out, nil
}

// moveSummonOK is FUN_004bff20: can o be summoned to (mp, x, y)? Not
// teleporting or dying, not an outlaw (PK level > 4) when
// PKLimitMoveSummon is set without PKLimitFree, outside the castle core
// (map 30 x 161..191, y 188..216) unless the battle runs, no Uniria /
// Dinorant on Atlans, wings or Dinorant / Fenrir on Icarus, no offline
// mode, no running self defense (obj+0x2c4 slots) and a MoveSummon.txt
// rectangle whose level (2/3 for MG, DL and class 6), reset and account
// level limits hold.
func (w *World) moveSummonOK(o *Object, mp, x, y int) bool {
	if o.P == nil || o.P.Char == nil || o.Teleporting || !o.Alive {
		return false
	}
	if w.cfg.Int("Common", "PKLimitFree", 0) == 0 && o.PKLevel > 4 && w.cfg.Int("Common", "PKLimitMoveSummon", 0) != 0 {
		return false
	}
	switch mp {
	case csMap:
		if !(w.hooks.castleBattle != nil && w.hooks.castleBattle(w)) && x >= 0xA1 && x <= 0xBF && y >= 0xBC && y <= 0xD8 {
			return false
		}
	case 7:
		if pet := w.slotItem(o, 8); !pet.IsEmpty() && (pet.Index == 0x1A02 || pet.Index == 0x1A03) {
			return false
		}
	case 10:
		if w.slotItem(o, 7).IsEmpty() {
			if pet := w.slotItem(o, 8); pet.Index != 0x1A03 && pet.Index != 0x1A25 {
				return false
			}
		}
	}
	if isOffline(o) {
		return false
	}
	if s := o.P.Stats; s != nil && s.selfDefense.init {
		limit := w.cfg.Int("Common", "SelfDefenseTime", 0)
		for i, idx := range s.selfDefense.Index {
			if idx >= 0 && w.now().Sub(s.selfDefense.Time[i]).Seconds() < float64(limit) {
				return false
			}
		}
	}
	any := func(v int) bool { return v == -1 }
	lvl := o.P.Char.Level
	for _, r := range w.q().moveSummon {
		if r.Map != mp || x < r.X1 || x > r.X2 || y < r.Y1 || y > r.Y2 {
			continue
		}
		if !any(r.MinLevel) {
			req := r.MinLevel
			if o.Class == 3 || o.Class == 4 || o.Class == 6 {
				req = req * 2 / 3
			}
			if lvl < req {
				return false
			}
		}
		if (!any(r.MaxLevel) && lvl > r.MaxLevel) || (!any(r.MinReset) && resets(o) < r.MinReset) ||
			(!any(r.MaxReset) && resets(o) > r.MaxReset) {
			return false
		}
		return r.AccountLevel[accountLevel(o)] != 0
	}
	return false
}

// --- B0 Teleport Ally ------------------------------------------------------------

// stunned is FUN_0047a470: effects 61, 72, 147 and 178.
func stunned(o *Object) bool {
	for _, e := range [...]int{0x3D, 0x48, 0x93, 0xB2} {
		if hasEffectIndex(o, e) {
			return true
		}
	}
	return false
}

// csTeleportGateY is FUN_0041f400 (map 30, caster Y < 0x90): around the
// castle gate lines 0x71 / 0xa0 / 0xcb the destination Y is kept on the
// caster's side.
func csTeleportGateY(casterY, y int) int {
	for _, g := range [...]int{0x71, 0xA0, 0xCB} {
		if g-5 <= casterY && casterY <= g+1 && g+1 <= y {
			return (g - 1) & 0xFF
		}
		if g+3 <= casterY && casterY <= g+9 && y <= g+3 {
			return (g + 3) & 0xFF
		}
	}
	return y
}

// csCrownClamp is FUN_00426e80: a target inside the crown room (x
// 0xa1..0xbe, y 0xbd..0xd8) stays in it (x 0xa4..0xbb, y 0xc0..0xd5).
func csCrownClamp(t *Object, x, y int) (int, int) {
	if t.X > 0xA0 && t.X < 0xBF && t.Y > 0xBC && t.Y < 0xD9 {
		x = min(max(x, 0xA4), 0xBB)
		if y < 0xC0 {
			return x, 0xC0
		}
		y = min(y, 0xD5)
	}
	return x, y
}

// handleTeleportAlly is B0 (FUN_005012a0): [3..4] target (little
// endian), [5] X, [6] Y. A missing target means the caster; both must be
// in the same party and on the same map, the caster not stunned; the
// destination must pass FUN_0050d0a0 for the target (8 cells around it,
// no safe zone). The caster needs skill 15; a delay failure answers the
// caster with C3 08 1C 00 (FUN_004bf340). Then 0x19, the Teleport magic
// on the target (FUN_0050e180) and the caster pays mana and BP.
func (w *World) handleTeleportAlly(o *Object, p []byte) {
	if len(p) < 7 || !o.IsPlayer() || o.P.Char == nil || !canPlayerAct(o) {
		return
	}
	ti := int(binary.LittleEndian.Uint16(p[3:5]))
	if ti >= MaxObjects {
		return
	}
	t := w.objects[ti]
	if t.State != StatePlaying || (t.P != nil && t.P.LogoutCountdown > 0) {
		ti, t = o.Index, o
	}
	op, tp := w.partyNumber(o), w.partyNumber(t)
	if op < 0 || tp < 0 || op != tp || stunned(o) || o.Map != t.Map {
		return
	}
	x, y := int(p[5]), int(p[6])
	tt := w.terrain(t.Map)
	if !t.Alive || t.Teleporting || x < t.X-8 || x > t.X+8 || y < t.Y-8 || y > t.Y+8 ||
		tt.Get(t.X, t.Y)&gamedata.AttrSafeZone != 0 || tt.Get(x, y)&gamedata.AttrSafeZone != 0 {
		return
	}
	if o.Map == csMap {
		if o.Y < 0x90 {
			y = csTeleportGateY(o.Y, y)
		}
		x, y = csCrownClamp(t, x, y)
	}
	const teleportAlly = 15
	if w.findSkill(o, teleportAlly) == nil {
		return
	}
	if !w.skillDelayOK(o, teleportAlly) {
		w.send(o, packetTeleport(0, o.Map, o.X, o.Y, o.Dir))
		return
	}
	if !w.manaOK(o, teleportAlly) || !w.bpOK(o, teleportAlly) {
		return
	}
	w.sendSkillTo(o, teleportAlly, ti, true)
	w.startMagicTeleport(t, x, y)
	w.payMana(o, teleportAlly)
}

func registerSkillGuild(h *hooks) {
	h.worldHandlers[0xB0] = (*World).handleTeleportAlly
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		rows, err := loadMoveSummon(w.configDir)
		if err != nil {
			return err
		}
		w.q().moveSummon = rows
		return nil
	})
	h.tick1000 = append(h.tick1000, (*World).summonSecond)
	h.beforeGateMove = append(h.beforeGateMove, func(w *World, o *Object) { w.summonCancel(o) })
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) {
		if w.rq != nil && w.rq.summons != nil {
			delete(w.rq.summons, o.Index)
		}
	})
}
