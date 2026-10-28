package game

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/packet"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Player skills (CSkillManager of the original):
//
//   - heads 0x19 (FUN_00500e70, skill on a target), 0x1E (FUN_00501080,
//     skill at a position), 0x10 (FUN_00500b70, the extra targets of an
//     area magic), 0x1B (effect cancel, effect.go);
//   - validation: magic list (FUN_004f8cb0), delay (FUN_004f83b0), kill
//     count (FUN_004f8680), guild status (FUN_004f86e0), class column
//     (FUN_004f8760), weapon skills (FUN_004f8800), safe zone
//     (SkillUseArea.txt, FUN_00502880), mana/BP (FUN_004f8030/FUN_004f80b0);
//   - UseSkill FUN_004f8dd0 / FUN_004f92d0 and the dispatcher FUN_004f9730
//     with one function per skill (see the per-skill comments);
//   - the delayed skill hits (FUN_00510690 messages 0x32/0x35 handled by
//     FUN_004c4280) and the Nova charge (FUN_0050e640).

// skillWorld holds the skill/effect tables of the instance.
type skillWorld struct {
	effects    map[int]effect.Info // EffectList.txt
	damage     map[int][2]int      // SkillDamage.txt section 0: PvM, PvP rate
	effectRate map[int]int         // SkillDamage.txt section 1
	combos     map[[2]int]comboRow
	enabler    []enablerRow
	areaSkills map[int]bool // SkillUseArea.txt section 0
	areas      []useArea    // SkillUseArea.txt section 1
	hitbox     []byte       // SkillElect.hit (36 x 21 x 21)
}

// comboRow is a ComboSkill.txt row (CComboSkill).
type comboRow struct{ Stage, Delay int }

// enablerRow is a SkillEnabler.txt row (CSkillEnabler).
type enablerRow struct{ Skill, Map, PvM, PvP int }

// useArea is a SkillUseArea.txt section 1 row.
type useArea struct{ Map, X1, Y1, X2, Y2 int }

// skillRef is the CSkill of the magic list (0x14 bytes: +4 level,
// +6 skill, +8 index, +0xc DamageMin, +0x10 DamageMax). CSkill::Set
// (FUN_004f5fd0) sets DamageMin = SkillList Damage and DamageMax =
// Damage + Damage/2.
type skillRef struct {
	Num                  int
	Level                int
	DamageMin, DamageMax int
}

func (w *World) skillRefOf(num, level int) *skillRef {
	d := 0
	if info, ok := w.data.Skills[num]; ok {
		d = info.Damage
	} else {
		d = -1
	}
	return &skillRef{Num: num, Level: level, DamageMin: d, DamageMax: d/2 + d}
}

// delayedHit is one FUN_00510690 message of the caster.
type delayedHit struct {
	Code, Target, Skill, Param int
	Send                       bool // param_4 of the Dark Raven message 0x33 (pet_raven.go)
	Due                        time.Time
}

// skillState is the per-object skill state.
type skillState struct {
	LastUse   map[int]time.Time // obj+0x600[skill]
	AreaSkill int               // obj+0x38c: area magic awaiting 0x10 targets
	AreaCount int               // obj+0x38e

	// ComboSkill state (obj+0xc0..): deadline, the two first skills and
	// the stage reached.
	ComboDeadline time.Time
	ComboSkill    [2]int
	ComboStage    int
	LastCombo     time.Time // obj+0x6c4

	NovaCharging bool      // obj+0x5c9
	NovaStage    int       // obj+0x5ca
	NovaTick     time.Time // obj+0x5cc

	DefenseTime time.Time // obj+0x8d4: Defense (skill 18) cast time
	KillCount   int       // obj+0x98c: players killed (guild skills)

	Pending []delayedHit
}

// sk returns the skill state of o, creating it on first use.
func sk(o *Object) *skillState {
	if o.Skill == nil {
		o.Skill = &skillState{LastUse: map[int]time.Time{}, ComboSkill: [2]int{-1, -1}, ComboStage: -1}
	}
	return o.Skill
}

// --- data -------------------------------------------------------------------

// loadSkillWorld reads the Skill tables of config/data/skill.toml and the
// binary SkillElect.hit of the data directory.
func loadSkillWorld(dataDir, configDir string) (*skillWorld, error) {
	sw := &skillWorld{damage: map[int][2]int{}, effectRate: map[int]int{}, combos: map[[2]int]comboRow{},
		areaSkills: map[int]bool{}}
	var err error
	if sw.effects, err = effect.LoadList(configDir); err != nil {
		return nil, err
	}
	f, err := gamedata.LoadSkillConfig(configDir)
	if err != nil {
		return nil, err
	}
	for _, r := range f.SkillDamage.Damage {
		sw.damage[r.SkillIndex] = [2]int{r.DamageRatePvM, r.DamageRatePvP}
	}
	for _, r := range f.SkillDamage.Effect {
		sw.effectRate[r.SkillIndex] = r.EffectRate
	}
	for _, r := range f.ComboSkill.Rows {
		sw.combos[[2]int{r.Class, r.SkillIndex}] = comboRow{Stage: r.Stage, Delay: r.Delay}
	}
	for _, r := range f.SkillEnabler.Rows {
		sw.enabler = append(sw.enabler, enablerRow{r.SkillIndex, r.MapNumber, r.UsePvM, r.UsePvP})
	}
	for _, r := range f.SkillUseArea.Skill {
		sw.areaSkills[r.SkillIndex] = true
	}
	for _, r := range f.SkillUseArea.Area {
		sw.areas = append(sw.areas, useArea{r.MapNumber, r.X1, r.Y1, r.X2, r.Y2})
	}
	// SkillElect.hit (FUN_004f7530): exactly 0x3e04 bytes.
	if b, err := os.ReadFile(filepath.Join(dataDir, "Skill", "SkillElect.hit")); err == nil && len(b) == 0x3e04 {
		sw.hitbox = b
	}
	return sw, nil
}

// --- packets ------------------------------------------------------------------

// packetSkillArea is C3 09 1E (FUN_00501740): [3] skill, [4..5] user BE,
// [6] x, [7] y, [8] dir.
func packetSkillArea(skill, user, x, y, dir int) []byte {
	p := packet.New(frame.C3, 0x1E, -1, 9)
	p[3] = byte(skill)
	p[4], p[5] = byte(user>>8), byte(user)
	p[6], p[7], p[8] = byte(x), byte(y), byte(dir)
	return p
}

// packetNovaStage is C1 07 BA (FUN_0050e640): [3..4] user BE, [5] 0x28,
// [6] charge stage.
func packetNovaStage(user, stage int) []byte {
	p := packet.New(frame.C1, 0xBA, -1, 7)
	p[3], p[4] = byte(user>>8), byte(user)
	p[5], p[6] = 0x28, byte(stage)
	return p
}

// packetKillCount is C1 05 B8 count (FUN_0050e0c0).
func packetKillCount(n int) []byte {
	p := packet.New(frame.C1, 0xB8, -1, 5)
	p[3] = 1
	p[4] = byte(n)
	return p
}

// sendSkillTo is FUN_00501570 with an explicit success flag (CL).
func (w *World) sendSkillTo(a *Object, skill, target int, ok bool) {
	if !w.skillSpeedAllowed(a, skill) { // FUN_00488550 (skillspeed.go)
		return
	}
	p := packetSkillUse(skill, a.Index, target, ok)
	if a.Type == TypePlayer {
		w.send(a, p)
	}
	w.broadcast(a, p)
}

// --- validation ---------------------------------------------------------------

// findSkill is FUN_004f8cb0: the magic list entry for a skill (0x3a, the
// Nova charge, is the Nova skill 0x28).
func (w *World) findSkill(o *Object, num int) *skillRef {
	if num == 0x3a {
		num = 0x28
	}
	if o.P == nil {
		return nil
	}
	for _, s := range o.P.Skills {
		if s.Index >= 0 && s.Index == num {
			return w.skillRefOf(num, int(s.Level))
		}
	}
	return nil
}

// skillDelayOK is FUN_004f83b0: SkillList Delay between two casts.
func (w *World) skillDelayOK(o *Object, num int) bool {
	info, ok := w.data.Skills[num]
	if !ok {
		return false
	}
	st := sk(o)
	now := w.now()
	if last, ok := st.LastUse[num]; ok && now.Sub(last) < time.Duration(info.Delay)*time.Millisecond {
		return false
	}
	st.LastUse[num] = now
	return true
}

func (w *World) guildStatus(o *Object) int {
	if w.hooks.guildStatus != nil {
		return w.hooks.guildStatus(w, o)
	}
	if o.P == nil {
		return -1
	}
	return guildOf(o).Status
}

// skillRequirementsOK is FUN_004f8680 (kill count), FUN_004f86e0 (guild
// status) and FUN_004f8760 (class column).
func (w *World) skillRequirementsOK(o *Object, num int) bool {
	info, ok := w.data.Skills[num]
	if !ok {
		return false
	}
	if info.ReqKillCount > sk(o).KillCount {
		return false
	}
	if r := info.ReqGuildStatus; r != 0 {
		gs := w.guildStatus(o)
		if r&0x20 != gs && r&0x40 != gs && r&0x80 != gs {
			return false
		}
	}
	return o.Class >= 0 && o.Class < 5 && info.Class[o.Class] != 0
}

// weaponSkillOK is FUN_004f8800: the skills granted by a weapon need an
// equipped item with the skill option whose Item.txt skill is that one.
func (w *World) weaponSkillOK(o *Object, num int) bool {
	if customWeaponSkillBypass(o, num) { // /combo and the manual auto attack (command_hardcoded.go)
		return true
	}
	switch num {
	case 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x31, 0x38, 0x3e, 0x4c, 0xdf, 0xe0, 0xe1, 0x104, 0x105, 0x10e:
	default:
		return true
	}
	for s := 0; s < EquipSlots; s++ {
		it := o.P.Inventory[s]
		if !it.IsEmpty() && it.Skill && w.itemSkill(it.Index) == num {
			return true
		}
	}
	return false
}

// handItemsOK reproduces the per-skill weapon checks of FUN_004f8dd0.
func (w *World) handItemsOK(o *Object, num int) bool {
	r, l := o.P.Inventory[0], o.P.Inventory[1]
	ri, li := -1, -1
	if !r.IsEmpty() {
		ri = r.Index
	}
	if !l.IsEmpty() {
		li = l.Index
	}
	switch num {
	case 0x29, 0x2a, 0x2b:
		if ri < 0 || ri > 0x1fff {
			return false
		}
	}
	switch num {
	case 0x3d, 0x3c, 0x4e, 0x41, 0x29, 0x2a, 0x2b, 0x37:
		if ri < 0 || ri > 0x1fff || (ri > 0x9ff && ri < 0xc01) {
			return false
		}
	}
	switch num {
	case 0x34, 0x33, 0x18:
		valid := func(it int) bool { return it >= 0 && it < 0x2000 }
		if !valid(ri) && !valid(li) {
			return false
		}
		if ri == 0x80f && li == 0x807 {
			return false
		}
		if ri == 0x80f && !valid(li) {
			return false
		}
		if li == 0x807 && !valid(ri) {
			return false
		}
		if ri >= 0 && ri < 0x801 {
			return false
		}
		if li > 0x9ff && ri < 0xe01 {
			return false
		}
	}
	return true
}

// manaOK / bpOK are FUN_004f8030 / FUN_004f80b0 (cost * obj+0x8b0 / 100,
// the rate being 100).
func (w *World) manaOK(o *Object, num int) bool {
	info := w.data.Skills[num]
	return float32(info.Mana*100/100) <= o.P.Char.Mana
}

func (w *World) bpOK(o *Object, num int) bool {
	info := w.data.Skills[num]
	return float32(info.BP*100/100) <= o.P.Char.BP
}

// payMana is the tail of FUN_004f8dd0: mana and BP are taken and C1 27 FF
// is sent.
func (w *World) payMana(o *Object, num int) {
	info := w.data.Skills[num]
	c := o.P.Char
	c.Mana = float32(float64(c.Mana) - float64(info.Mana*100/100))
	c.BP -= float32(info.BP * 100 / 100)
	w.send(o, packetMana(int(c.Mana), int(c.BP)))
}

// safeZoneSkillOK is FUN_00502880: inside a safe zone only the skills of
// SkillUseArea.txt section 0 work, and only inside a listed area.
func (w *World) safeZoneSkillOK(o *Object, num int) bool {
	if w.skills == nil || !w.skills.areaSkills[num] {
		return false
	}
	for _, a := range w.skills.areas {
		if a.Map == o.Map && a.X1 <= o.X && o.X <= a.X2 && a.Y1 <= o.Y && o.Y <= a.Y2 {
			return true
		}
	}
	return false
}

// skillEnabled is FUN_004f72c0 (SkillEnabler.txt): the first row of the
// skill for the map (or any map) decides PvM / PvP use.
func (w *World) skillEnabled(skill int, a, t *Object) bool {
	if w.skills == nil {
		return true
	}
	for _, r := range w.skills.enabler {
		if r.Skill != skill || (r.Map != -1 && r.Map != a.Map) {
			continue
		}
		if t.Type == TypeMonster {
			return r.PvM != 0
		}
		if t.Type == TypePlayer {
			return r.PvP != 0
		}
	}
	return true
}

// rangeOK is FUN_004f8130: distance within the SkillList Range (+2 for a
// Dark Lord riding the Dark Horse); Range 0 never passes.
func (w *World) rangeOK(o *Object, num, x1, y1, x2, y2 int) bool {
	info, ok := w.data.Skills[num]
	if !ok || info.Range == 0 {
		return false
	}
	r := float32(info.Range)
	if o.Class == 4 && o.P != nil && !o.P.Inventory[8].IsEmpty() && o.P.Inventory[8].Index == 0x1a04 {
		r = float32(info.Range + 2)
	}
	return r >= dist32(x1, y1, x2, y2)
}

// radioOK is FUN_004f8280 with the SkillList Radio.
func (w *World) radioOK(num, x1, y1, x2, y2 int) bool {
	info, ok := w.data.Skills[num]
	if !ok || info.Radio == 0 {
		return false
	}
	return float32(info.Radio) >= dist32(x1, y1, x2, y2)
}

func dist32(x1, y1, x2, y2 int) float32 {
	dx, dy := float32(x1)-float32(x2), float32(y1)-float32(y2)
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// skillTargetOK is FUN_004f8440: can an area skill of a hit idx (seen
// with viewport type vpType)? Monsters always (summons follow their
// owner); players only when the explicit target, in a duel with the
// caster or in the Castle Siege war rules.
func (w *World) skillTargetOK(a *Object, idx int, vpType byte, mainTarget int) bool {
	if idx < 0 || idx >= MaxObjects || vpType == byte(TypeNPC) {
		return false
	}
	t := w.objects[idx]
	if !t.Alive || t.Live != LiveNormal || t.Teleporting {
		return false
	}
	if (a.Type == TypeMonster && vpType == byte(TypePlayer)) || idx == mainTarget {
		return true
	}
	owner := idx
	if vpType == byte(TypeMonster) {
		if t.Class > 99 && t.Class < 0x6f || t.Class == 0x20b {
			return false
		}
		if t.Mon == nil || t.Mon.Owner < 0 || t.Mon.Owner >= MaxObjects {
			return true
		}
		owner = t.Mon.Owner
	}
	if a.Type == TypePlayer && w.inDuelWith(a, w.objects[owner]) {
		return true
	}
	if a.Type == TypePlayer && w.hooks.guildWar != nil && w.hooks.guildWar(w, a, w.objects[owner]) {
		return true
	}
	if a.Type == TypePlayer && w.hooks.arenaFight != nil && w.hooks.arenaFight(w, a) { // FUN_00443f50 (customarena.go)
		return true
	}
	// The Castle Siege rules are not implemented.
	return false
}

// --- handlers -----------------------------------------------------------------

func skillPlayerOK(o *Object) bool {
	return o.State == StatePlaying && o.IsPlayer() && o.P.Char != nil && canPlayerAct(o) && o.Alive
}

// validateSkill is the common part of 0x19 / 0x1E: magic list, delay and
// requirements.
func (w *World) validateSkill(o *Object, num int, full bool) *skillRef {
	ref := w.findSkill(o, num)
	if ref == nil || !w.skillDelayOK(o, ref.Num) {
		return nil
	}
	if full && !w.skillRequirementsOK(o, ref.Num) {
		return nil
	}
	return ref
}

// handleSkillTarget is 0x19 (FUN_00500e70): [3] skill, [4..5] target BE.
func (w *World) handleSkillTarget(o *Object, p []byte) {
	if len(p) < 6 || !skillPlayerOK(o) {
		return
	}
	target := int(p[4])<<8 | int(p[5])
	if target >= MaxObjects {
		return
	}
	t := w.objects[target]
	if t.State != StatePlaying || (t.P != nil && t.P.LogoutCountdown > 0) || !t.Alive {
		return
	}
	num := int(p[3])
	if w.cellHas(o, gamedata.AttrSafeZone) && !w.safeZoneSkillOK(o, num) {
		return
	}
	ref := w.validateSkill(o, num, true)
	if ref == nil {
		return
	}
	sk(o).AreaSkill = 0
	if num == 0x3a {
		target = 0x3a
	}
	w.useSkill(o, target, ref)
}

// handleSkillArea is 0x1E (FUN_00501080): [3] skill, [4] x, [5] y,
// [6] dir, [8] angle, [9..10] target BE.
func (w *World) handleSkillArea(o *Object, p []byte) {
	if len(p) < 11 || !skillPlayerOK(o) {
		return
	}
	num := int(p[3])
	if w.cellHas(o, gamedata.AttrSafeZone) && !w.safeZoneSkillOK(o, num) {
		return
	}
	ref := w.validateSkill(o, num, true)
	if ref == nil {
		return
	}
	sk(o).AreaSkill = 0
	target := int(p[9])<<8 | int(p[10])
	w.useSkillArea(o, target, ref, int(p[4]), int(p[5]), int(p[6]), int(p[8]))
}

// handleSkillMulti is 0x10 (FUN_00500b70): the targets of an area magic
// cast by 0x1E: [3] skill, [7] count (max 5), 3 bytes per target from [8]
// (index BE). At most 5 packets per cast.
func (w *World) handleSkillMulti(o *Object, p []byte) {
	w.skillMultiHit(o, p, false)
}

func (w *World) skillMultiHit(o *Object, p []byte, sendSkill bool) {
	if len(p) < 8 || !skillPlayerOK(o) {
		return
	}
	num := int(p[3])
	if w.cellHas(o, gamedata.AttrSafeZone) && !w.safeZoneSkillOK(o, num) {
		return
	}
	ref := w.findSkill(o, num)
	if ref == nil || !w.skillDelayOKArea(o, ref.Num) || !w.skillClassOK(o, ref.Num) {
		return
	}
	switch ref.Num {
	case 5, 8, 9, 10, 12, 13, 14, 24, 47, 50, 52, 55, 78:
	default:
		return
	}
	st := sk(o)
	if st.AreaSkill == 0 || st.AreaSkill != ref.Num {
		return
	}
	st.AreaCount++
	if st.AreaCount > 5 {
		return
	}
	if ref.Num == 24 {
		r, l := o.P.Inventory[0], o.P.Inventory[1]
		if r.IsEmpty() || l.IsEmpty() || r.Index < 0x800 || r.Index > 0x9ff || l.Index < 0x800 || l.Index > 0x9ff {
			return
		}
	}
	n := min(int(p[7]), 5)
	for i := 0; i < n && 8+3*i+1 < len(p); i++ {
		idx := int(p[8+3*i])<<8 | int(p[9+3*i])
		if idx >= MaxObjects {
			continue
		}
		t := w.objects[idx]
		if !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
			continue
		}
		send := sendSkill || ref.Num == 0x37
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, SendSkill: send})
	}
}

// skillDelayOKArea / skillClassOK are FUN_004f83b0 / FUN_004f8760 (the
// 0x10 path checks only these two).
func (w *World) skillDelayOKArea(o *Object, num int) bool { return w.skillDelayOK(o, num) }

func (w *World) skillClassOK(o *Object, num int) bool {
	info, ok := w.data.Skills[num]
	return ok && o.Class >= 0 && o.Class < 5 && info.Class[o.Class] != 0
}

// --- UseSkill -------------------------------------------------------------------

// usePreChecks is the player prefix of FUN_004f8dd0 / FUN_004f92d0:
// weapon skills, hand items and the combo check. ok=false stops the cast.
func (w *World) usePreChecks(o *Object, ref *skillRef) (combo, ok bool) {
	if !w.weaponSkillOK(o, ref.Num) || !w.handItemsOK(o, ref.Num) {
		return false, false
	}
	switch o.Class {
	case 3, 4, 6:
		combo = w.comboCheck(o, ref.Num)
	default:
		q := 2
		if o.Class == 1 {
			q = 3
		}
		if questStateOf(o, q) == 2 {
			combo = w.comboCheck(o, ref.Num)
		}
	}
	if w.customComboCheck(o, ref.Num) { // FUN_004f8d10, /combo (command_hardcoded.go)
		combo = true
	}
	if combo {
		sk(o).LastCombo = w.now()
	}
	return combo, true
}

// questStateOf reads the 2-bit quest state (obj+0x58e, FUN_004dd740).
func questStateOf(o *Object, idx int) int {
	if o.P == nil || o.P.Char == nil {
		return 0
	}
	q := o.P.Char.Quest
	if idx < 0 || idx/4 >= len(q) {
		return 0
	}
	return int(q[idx/4]>>(uint(idx%4)*2)) & 3
}

// comboCheck is FUN_00434360 (ComboSkill.txt, per class): a stage-0
// skill opens the combo, a stage-1 skill within the delay is the second
// hit and a different stage-1 skill within the delay completes it.
func (w *World) comboCheck(o *Object, num int) bool {
	st := sk(o)
	reset := func() {
		st.ComboDeadline = time.Time{}
		st.ComboSkill = [2]int{-1, -1}
		st.ComboStage = -1
	}
	if w.skills == nil {
		return false
	}
	row, ok := w.skills.combos[[2]int{o.Class, num}]
	now := w.now()
	if ok {
		switch row.Stage {
		case 0:
			st.ComboDeadline = now.Add(time.Duration(row.Delay) * time.Millisecond)
			st.ComboSkill[0] = num
			st.ComboStage = 0
			return false
		case 1:
			if !now.After(st.ComboDeadline) && st.ComboSkill[0] != -1 {
				switch st.ComboStage {
				case 0:
					st.ComboDeadline = now.Add(time.Duration(row.Delay) * time.Millisecond)
					st.ComboSkill[1] = num
					st.ComboStage = 1
					return false
				case 1:
					if st.ComboSkill[1] != num {
						reset()
						return true
					}
				}
			}
		}
	}
	reset()
	return false
}

// useSkill is FUN_004f8dd0 (0x19 path) for players.
func (w *World) useSkill(o *Object, target int, ref *skillRef) {
	combo, ok := w.usePreChecks(o, ref)
	if !ok {
		return
	}
	st := sk(o)
	if st.NovaCharging && ref.Num != 0x28 {
		if nova := w.findSkill(o, 0x28); nova != nil {
			w.dispatchSkill(o, nova, 0, o.X, o.Y, 0, false)
			return
		}
	}
	if ref.Num == 0x28 && target != 0x3a {
		w.dispatchSkill(o, ref, target, o.X, o.Y, 0, combo)
		return
	}
	if !w.manaOK(o, ref.Num) || !w.bpOK(o, ref.Num) {
		return
	}
	if w.dispatchSkill(o, ref, target, o.X, o.Y, 0, combo) {
		w.payMana(o, ref.Num)
	}
}

// useSkillArea is FUN_004f92d0 (0x1E path): the C3 09 1E broadcast is
// sent before the mana check.
func (w *World) useSkillArea(o *Object, target int, ref *skillRef, x, y, dir, angle int) {
	combo, ok := w.usePreChecks(o, ref)
	if !ok {
		return
	}
	if st := sk(o); st.NovaCharging && ref.Num != 0x28 {
		if nova := w.findSkill(o, 0x28); nova != nil {
			w.dispatchSkill(o, nova, 0, x, y, 0, false)
			return
		}
	}
	if w.skillSpeedAllowed(o, ref.Num) { // FUN_00501740 (skillspeed.go)
		p := packetSkillArea(ref.Num, o.Index, x, y, dir)
		w.send(o, p)
		w.broadcast(o, p)
	}
	if !w.manaOK(o, ref.Num) || !w.bpOK(o, ref.Num) {
		return
	}
	if w.dispatchSkill(o, ref, target, x, y, angle, combo) {
		w.payMana(o, ref.Num)
	}
}

// dispatchSkill is FUN_004f9730. It returns true when the skill was cast
// (mana and BP are then taken).
func (w *World) dispatchSkill(o *Object, ref *skillRef, target, x, y, angle int, combo bool) bool {
	st := sk(o)
	st.DefenseTime = time.Time{} // obj+0x8d4 = 0
	if o.Move != nil && o.Move.Authority == 1 {
		w.removeEffect(o, effect.Invisibility)
	}
	switch ref.Num {
	case chainLightningSkill:
		return w.skillChainLightning(o, ref, target, combo) // FUN_004fdbd0 (skill_chain.go)
	case 5, 10, 14, 24, 78, 8, 12, 52, 9, 13, 47, 55:
		return w.skillAreaMagic(o, ref, target, combo)
	case 16:
		return w.skillManaShield(o, ref, target)
	case 18:
		return w.skillDefense(o, target)
	case 26:
		return w.skillHeal(o, ref, target)
	case 27:
		return w.skillGreaterDefense(o, ref, target)
	case 28:
		return w.skillGreaterDamage(o, ref, target)
	case 38, 39:
		return w.skillPointArea(o, ref, x, y, combo)
	case 40:
		return w.skillNova(o, ref, target, combo)
	case 41:
		return w.skillTwisting(o, ref, combo)
	case 42:
		return w.skillDelayedArea(o, ref, 0x32, 500, 10, combo)
	case 43:
		return w.skillDeathStab(o, ref, target, angle, combo)
	case 44, 73, 45, 57, 74, 46:
		return w.skillCastleSiege(o, ref, target, combo)
	case 48:
		return w.skillGreaterLife(o, ref)
	case 56:
		return w.skillFrustum(o, ref, angle, combo)
	case 60:
		return w.skillForce(o, ref, target, combo)
	case 61:
		return w.skillFireBurst(o, ref, target, combo)
	case 62:
		return w.skillDelayedArea(o, ref, 0x32, 500, 20, combo)
	case 64:
		return w.skillCriticalDamage(o, ref)
	case 65:
		return w.skillElectricSpark(o, ref, x, y, combo)
	case 76:
		return w.skillDelayedArea(o, ref, 0x35, 500, 10, false)
	case 77:
		return w.skillInfinityArrow(o, ref, target)
	case 30, 31, 32, 33, 34, 35, 36:
		// Summons (FUN_004fae50, summon.go).
		return w.hooks.summonSkill != nil && w.hooks.summonSkill(w, o, ref.Num)
	case 63, 67, 68, 69, 70, 71, 72, 75:
		// Summon Party (FUN_004fc1a0) and the Castle Siege guild skills
		// (maps 30/31 only), skill_guild.go.
		return w.guildSkill(o, ref, target, angle)
	case 50:
		// The monster area attack is not a player skill.
		return false
	}
	return w.skillSingle(o, ref, target, combo)
}

// --- per-skill ----------------------------------------------------------------

func (w *World) targetObj(idx int) *Object {
	if idx < 0 || idx >= MaxObjects {
		return nil
	}
	return w.objects[idx]
}

// skillSingle is FUN_004f9fd0: one target within Range.
func (w *World) skillSingle(o *Object, ref *skillRef, target int, combo bool) bool {
	t := w.targetObj(target)
	if t == nil {
		return true
	}
	if !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, SendSkill: true, Combo: combo})
	if combo {
		w.sendSkillTo(o, 0x3b, target, true)
	}
	return true
}

// skillAreaMagic is FUN_004fa0b0: remember the area magic for the 0x10
// targets; Flame (5), Impale (47), Blast (13) and Fire Scream (78) also
// hit the explicit target.
func (w *World) skillAreaMagic(o *Object, ref *skillRef, target int, combo bool) bool {
	st := sk(o)
	st.AreaSkill, st.AreaCount = ref.Num, 0
	switch ref.Num {
	case 5, 47, 13, 78:
		w.skillSingle(o, ref, target, combo)
	}
	return true
}

// buffTargetOK is the common target rule of FUN_004fa4c0 / FUN_004fa7b0 /
// FUN_004fa9d0 / FUN_004fac10: a player or a summon, in range, and only
// the caster itself on the custom arena map and in Chaos Castle.
func (w *World) buffTargetOK(o *Object, ref *skillRef, t *Object) bool {
	if t.Type != TypePlayer && (t.Mon == nil || t.Mon.Owner < 0 || t.Mon.Owner >= MaxObjects) {
		return false
	}
	if o != t {
		if arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1); o.Map == arena {
			return false
		}
		if o.Map >= 18 && o.Map <= 23 {
			return false
		}
	}
	return w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y)
}

func (w *World) skillEffectIndex(num int) int { return w.data.Skills[num].Effect }

func classRate(w *World, key string, t *Object) int {
	if t.Type != TypePlayer || t.Class < 0 || t.Class > 4 {
		return 100
	}
	return w.cfg.Int("Skill", key+classKeys[t.Class], 0)
}

func addStat(o *Object, i int) int {
	if o.P == nil || o.P.Char == nil {
		return 0
	}
	c := o.P.Char
	var s *statsState
	if o.P.Stats != nil {
		s = o.P.Stats
	} else {
		s = &statsState{}
	}
	switch i {
	case 0:
		return c.Strength + s.AddStr
	case 1:
		return c.Dexterity + s.AddDex
	case 2:
		return c.Vitality + s.AddVit
	case 3:
		return c.Energy + s.AddEne
	}
	return c.Leadership + s.AddLead
}

func divOr0(a, b int) int {
	if b == 0 {
		return 0
	}
	return a / b
}

// skillManaShield is FUN_004fa4c0 (16): same party (two players without a
// party also qualify), value ConstA + Dex/ConstB + Ene/ConstC capped by
// MaxRate, times the target class rate; duration TimeConstA +
// Ene/TimeConstB.
func (w *World) skillManaShield(o *Object, ref *skillRef, target int) bool {
	t := w.targetObj(target)
	if t == nil || w.partyNumber(o) != w.partyNumber(t) || !w.buffTargetOK(o, ref, t) {
		return false
	}
	c := func(k string) int { return w.cfg.Int("Skill", k, 0) }
	v := c("ManaShieldConstA") + divOr0(addStat(o, 1), c("ManaShieldConstB")) + divOr0(addStat(o, 3), c("ManaShieldConstC"))
	if v > c("ManaShieldMaxRate") {
		v = c("ManaShieldMaxRate")
	}
	v = classRate(w, "ManaShieldRate", t) * v / 100
	d := c("ManaShieldTimeConstA") + divOr0(addStat(o, 3), c("ManaShieldTimeConstB"))
	w.addEffect(t, w.skillEffectIndex(ref.Num), d, [4]int{v}, true)
	w.sendSkillTo(o, ref.Num, target, true)
	return true
}

func (w *World) partyNumber(o *Object) int {
	if o.P == nil || o.P.Party == nil {
		return -1
	}
	return o.P.Party.Number
}

// skillDefense is FUN_004fa720 (18): a shield with the skill option in
// the left hand; the cast time obj+0x8d4 lowers the damage taken by
// DefenseConstA% for DefenseTimeConstA seconds (FUN_0040b110). The C1 09
// 18 action 0x12 is broadcast.
func (w *World) skillDefense(o *Object, target int) bool {
	l := o.P.Inventory[1]
	if l.IsEmpty() || l.Index < 0xc00 || l.Index >= 0xe00 || !l.Skill {
		return false
	}
	sk(o).DefenseTime = w.now()
	p := packetAttackAction(o.Index, o.Dir, 0x12, target)
	w.send(o, p)
	w.broadcast(o, p)
	return true
}

// skillHeal is FUN_004fa7b0 (26): life + Ene/HealConstB + HealConstA up
// to the maximum; C1 26 FF to a player target.
func (w *World) skillHeal(o *Object, ref *skillRef, target int) bool {
	t := w.targetObj(target)
	if t == nil || !w.buffTargetOK(o, ref, t) {
		return false
	}
	add := divOr0(addStat(o, 3), w.cfg.Int("Skill", "HealConstB", 0)) + w.cfg.Int("Skill", "HealConstA", 0)
	life := float32(add) + objLife(t)
	if mx := objMaxLife(t); mx < life {
		life = mx
	}
	setObjLife(t, life)
	if t.Type == TypePlayer {
		w.send(t, packetLife(0xFF, int(life)))
	}
	w.sendSkillTo(o, ref.Num, target, true)
	return true
}

// objMaxLife is MaxLife + AddLife (obj+0x114 + obj+0x1b4).
func objMaxLife(o *Object) float32 {
	if o.Mon != nil {
		return o.Mon.MaxLife
	}
	if o.P != nil && o.P.Char != nil {
		return float32(o.P.AddLife) + o.P.Char.MaxLife
	}
	return 0
}

// skillGreaterDefense is FUN_004fa9d0 (27): (Ene/ConstB + ConstA) times
// the target class rate, doubled; GreaterDefenseTimeConstA seconds.
func (w *World) skillGreaterDefense(o *Object, ref *skillRef, target int) bool {
	t := w.targetObj(target)
	if t == nil || !w.buffTargetOK(o, ref, t) {
		return false
	}
	v := divOr0(addStat(o, 3), w.cfg.Int("Skill", "GreaterDefenseConstB", 0)) + w.cfg.Int("Skill", "GreaterDefenseConstA", 0)
	v = classRate(w, "GreaterDefenseRate", t) * v / 100 * 2
	w.addEffect(t, w.skillEffectIndex(ref.Num), w.cfg.Int("Skill", "GreaterDefenseTimeConstA", 0), [4]int{v}, true)
	w.sendSkillTo(o, ref.Num, target, true)
	return true
}

// skillGreaterDamage is FUN_004fac10 (28): (Ene/ConstB + ConstA) times
// the target class rate; GreaterDamageTimeConstA seconds.
func (w *World) skillGreaterDamage(o *Object, ref *skillRef, target int) bool {
	t := w.targetObj(target)
	if t == nil || !w.buffTargetOK(o, ref, t) {
		return false
	}
	v := divOr0(addStat(o, 3), w.cfg.Int("Skill", "GreaterDamageConstB", 0)) + w.cfg.Int("Skill", "GreaterDamageConstA", 0)
	v = classRate(w, "GreaterDamageRate", t) * v / 100
	w.addEffect(t, w.skillEffectIndex(ref.Num), w.cfg.Int("Skill", "GreaterDamageTimeConstA", 0), [4]int{v}, true)
	w.sendSkillTo(o, ref.Num, target, true)
	return true
}

// partyOrSelf returns the party members (5 slots, -1 empty) or nil.
func (w *World) partyMembers(o *Object) []int {
	pt := w.partyOf(o)
	if pt == nil {
		return nil
	}
	return pt.members[:]
}

// skillGreaterLife is FUN_004fb720 (48): Vit/ConstB + ConstA + Ene/ConstC
// capped by MaxRate for Ene/TimeConstB + TimeConstA seconds, to every
// party member within Radio (the class rate multiplies the running value
// member after member, as in the original) or to the caster alone.
func (w *World) skillGreaterLife(o *Object, ref *skillRef) bool {
	c := func(k string) int { return w.cfg.Int("Skill", k, 0) }
	v := divOr0(addStat(o, 2), c("GreaterLifeConstB")) + c("GreaterLifeConstA") + divOr0(addStat(o, 3), c("GreaterLifeConstC"))
	if v > c("GreaterLifeMaxRate") {
		v = c("GreaterLifeMaxRate")
	}
	d := divOr0(addStat(o, 3), c("GreaterLifeTimeConstB")) + c("GreaterLifeTimeConstA")
	e := w.skillEffectIndex(ref.Num)
	if members := w.partyMembers(o); members != nil {
		for _, idx := range members {
			t := w.targetObj(idx)
			if t == nil || !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
				continue
			}
			v = classRate(w, "GreaterLifeRate", t) * v / 100
			w.addEffect(t, e, d, [4]int{v}, true)
			w.sendSkillTo(o, ref.Num, idx, true)
		}
		return true
	}
	v = classRate(w, "GreaterLifeRate", o) * v / 100
	w.addEffect(o, e, d, [4]int{v}, true)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	return true
}

// skillCriticalDamage is FUN_004fc3d0 (64): Lead/ConstB + Ene/ConstA for
// Ene/TimeConstB + TimeConstA seconds, to the party in Radio or the
// caster.
func (w *World) skillCriticalDamage(o *Object, ref *skillRef) bool {
	c := func(k string) int { return w.cfg.Int("Skill", k, 0) }
	v := divOr0(addStat(o, 4), c("GreaterCriticalDamageConstB")) + divOr0(addStat(o, 3), c("GreaterCriticalDamageConstA"))
	d := divOr0(addStat(o, 3), c("GreaterCriticalDamageTimeConstB")) + c("GreaterCriticalDamageTimeConstA")
	e := w.skillEffectIndex(ref.Num)
	if members := w.partyMembers(o); members != nil {
		for _, idx := range members {
			t := w.targetObj(idx)
			if t == nil || !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
				continue
			}
			w.addEffect(t, e, d, [4]int{v}, true)
			w.sendSkillTo(o, ref.Num, idx, true)
		}
		return true
	}
	w.addEffect(o, e, d, [4]int{v}, true)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	return true
}

// skillInfinityArrow is FUN_004fd760 (77): the caster only; the duration
// (CServerInfo+0x1830) has no configuration key, so it is 0 and the
// effect never runs out.
func (w *World) skillInfinityArrow(o *Object, ref *skillRef, target int) bool {
	if target != o.Index {
		return false
	}
	w.addEffect(o, w.skillEffectIndex(ref.Num), 0, [4]int{}, true)
	w.sendSkillTo(o, ref.Num, o.Index, true)
	return true
}

// vp2Targets yields the objects of the caster's VP2 (obj+0x3bc) that
// FUN_004f8440 accepts.
func (w *World) vp2Targets(o *Object, mainTarget int, f func(t *Object) bool) {
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 {
			continue
		}
		if !w.skillTargetOK(o, e.Index, e.Type, mainTarget) {
			continue
		}
		if !f(w.objects[e.Index]) {
			return
		}
	}
}

func (w *World) comboMark(o *Object, t *Object, combo bool) {
	if combo {
		w.sendSkillTo(o, 0x3b, t.Index, true)
	}
}

// skillPointArea is FUN_004fb080 (Decay 38, Ice Storm 39): the point
// must be within Range; up to 10 targets within Radio of it.
func (w *World) skillPointArea(o *Object, ref *skillRef, x, y int, combo bool) bool {
	if !w.rangeOK(o, ref.Num, o.X, o.Y, x, y) {
		return false
	}
	n := 0
	w.vp2Targets(o, -1, func(t *Object) bool {
		if !w.radioOK(ref.Num, x, y, t.X, t.Y) {
			return true
		}
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, t, combo)
		n++
		return n < 10
	})
	return true
}

// skillTwisting is FUN_004fb310 (41): nothing with a staff; up to 10
// targets within Radio of the caster.
func (w *World) skillTwisting(o *Object, ref *skillRef, combo bool) bool {
	n := 0
	w.vp2Targets(o, -1, func(t *Object) bool {
		if r := o.P.Inventory[0]; !r.IsEmpty() && r.Index > 0x9ff && r.Index < 0xc01 {
			return false
		}
		if !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
			return true
		}
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, t, combo)
		n++
		return n < 10
	})
	return true
}

// skillDelayedArea is FUN_004fb440 (Rageful Blow, message 0x32 after
// 500 ms, 10 targets), FUN_004fc0c0 (Earthquake, 20 targets) and
// FUN_004fd690 (Plasma Storm, message 0x35).
func (w *World) skillDelayedArea(o *Object, ref *skillRef, code, delayMs, maxN int, combo bool) bool {
	n := 0
	w.vp2Targets(o, -1, func(t *Object) bool {
		if !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
			return true
		}
		param := 0
		if combo {
			param = 1
		}
		w.queueSkillHit(o, code, t.Index, delayMs, ref.Num, param)
		w.comboMark(o, t, combo)
		n++
		return n < maxN
	})
	return true
}

// skillFrustum is FUN_004fbbb0 (Power Slash 56): the 6x6 frustum in
// front of the caster, rotated by the packet angle; 10 targets.
func (w *World) skillFrustum(o *Object, ref *skillRef, angle int, combo bool) bool {
	fx, fy := skillFrustumPoints(o.X, o.Y, angle, 6, 6)
	n := 0
	w.vp2Targets(o, -1, func(t *Object) bool {
		if !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) || !inFrustum(fx, fy, t.X, t.Y) {
			return true
		}
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, t, combo)
		n++
		return n < 10
	})
	return true
}

// skillDeathStab is FUN_004fb520 (43): 0x19 to the target, the hit, then
// the frustum targets (the target itself is accepted again).
func (w *World) skillDeathStab(o *Object, ref *skillRef, target, angle int, combo bool) bool {
	t := w.targetObj(target)
	if t == nil {
		return false
	}
	w.sendSkillTo(o, ref.Num, target, true)
	w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
	w.comboMark(o, t, combo)
	fx, fy := skillFrustumPoints(o.X, o.Y, angle, 6, 6)
	n := 0
	w.vp2Targets(o, target, func(v *Object) bool {
		if !w.radioOK(ref.Num, o.X, o.Y, v.X, v.Y) || !inFrustum(fx, fy, v.X, v.Y) {
			return true
		}
		w.attackFull(o, v, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, v, combo)
		n++
		return n < 10
	})
	return true
}

// skillFrustumPoints is FUN_004f7c00: the points (-w,h) (w,h) (1,0)
// (-1,0) rotated around Z by angle*360/255 degrees (AngleMatrix) and
// truncated, plus the caster position.
func skillFrustumPoints(x, y, angle int, wd, ht float32) ([4]int, [4]int) {
	deg := float64(int32(angle&0xff) * 360 / 255)
	rad := deg * (math.Pi * 2 / 360)
	sy, cy := float32(math.Sin(rad)), float32(math.Cos(rad))
	pts := [4][2]float32{{-wd, ht}, {wd, ht}, {1, 0}, {-1, 0}}
	var fx, fy [4]int
	for i, p := range pts {
		rx := p[0]*cy + p[1]*-sy
		ry := p[0]*sy + p[1]*cy
		fx[i] = int(rx) + x
		fy[i] = int(ry) + y
	}
	return fx, fy
}

// inFrustum is FUN_004f8340.
func inFrustum(fx, fy [4]int, x, y int) bool {
	j := 3
	for i := 0; i < 4; i++ {
		if (fx[i]-x)*(fy[j]-y)-(fx[j]-x)*(fy[i]-y) < 0 {
			return false
		}
		j = i
	}
	return true
}

// skillNova is FUN_004fb1c0 (40): the 0x3a cast starts charging
// (obj+0x5c9 = 1, stage 0) and answers 0x19 0x3a; any other cast
// releases it: up to 20 targets within Radio get a message 0x32 after
// 600 ms and the caster broadcasts 0x19 0x28.
func (w *World) skillNova(o *Object, ref *skillRef, target int, combo bool) bool {
	st := sk(o)
	if target == 0x3a {
		if !st.NovaCharging {
			st.NovaCharging, st.NovaStage = true, 0
			st.NovaTick = w.now()
			w.sendSkillTo(o, 0x3a, o.Index, true)
			return true
		}
		return false
	}
	if !st.NovaCharging {
		return false
	}
	st.NovaCharging, st.NovaTick = false, time.Time{}
	n := 0
	w.vp2Targets(o, target, func(t *Object) bool {
		if !w.radioOK(ref.Num, o.X, o.Y, t.X, t.Y) {
			return true
		}
		param := 0
		if combo {
			param = 1
		}
		w.queueSkillHit(o, 0x32, t.Index, 600, ref.Num, param)
		w.comboMark(o, t, combo)
		n++
		return n < 20
	})
	w.sendSkillTo(o, ref.Num, o.Index, true)
	return true
}

// novaTick is FUN_0050e640: every 500 ms the charge stage grows and
// C1 07 BA is broadcast; stage 12 releases the Nova.
func (w *World) novaTick(o *Object) {
	st := o.Skill
	if st == nil || !st.NovaCharging {
		return
	}
	now := w.now()
	if !w.skillSpeedAllowed(o, 0x28) { // FUN_00488550 at 0x50e678 (skillspeed.go)
		return
	}
	if now.Sub(st.NovaTick) <= 499*time.Millisecond {
		return
	}
	st.NovaStage++
	st.NovaTick = now
	if st.NovaStage == 12 {
		if nova := w.findSkill(o, 0x28); nova != nil {
			w.dispatchSkill(o, nova, 0, o.X, o.Y, 0, false)
		}
		return
	}
	p := packetNovaStage(o.Index, st.NovaStage)
	w.send(o, p)
	w.broadcast(o, p)
}

// skillCastleSiege is FUN_004fc800 (44, 45, 46, 57, 73, 74): only with
// EnableCSSkillsAllMaps or on maps 30/31; 44/57/73 hit at once, 45/46/74
// through message 0x32.
func (w *World) skillCastleSiege(o *Object, ref *skillRef, target int, combo bool) bool {
	if w.cfg.Int("Skill", "EnableCSSkillsAllMaps", 0) == 0 && o.Map != 30 && o.Map != 31 {
		return false
	}
	t := w.targetObj(target)
	if t == nil || !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	w.sendSkillTo(o, ref.Num, target, true)
	switch ref.Num {
	case 44, 57, 73:
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
	default:
		param := 0
		if combo {
			param = 1
		}
		// The delay of this FUN_00510690 call is passed in a register the
		// decompiler dropped; 250 ms is assumed (U-SKILL-03).
		w.queueSkillHit(o, 0x32, target, 250, ref.Num, param)
	}
	w.comboMark(o, t, combo)
	return true
}

// skillForce is FUN_004fbd40 (60): without the Force Wave weapon option
// (obj+0x5d8) a single hit; with it up to 10 targets on the line to the
// target.
func (w *World) skillForce(o *Object, ref *skillRef, target int, combo bool) bool {
	t := w.targetObj(target)
	if t == nil || !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	w.sendSkillTo(o, ref.Num, target, true)
	if o.P.Stats == nil || !o.P.Stats.Skill42 {
		w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref})
		return true
	}
	// FUN_004bd920 / FUN_004f75d0 (the line test) lost their arguments;
	// the frustum towards the target is used instead (U-SKILL-04).
	ang := angleByte(o.X, o.Y, t.X, t.Y)
	fx, fy := skillFrustumPoints(o.X, o.Y, ang, 1.5, 4)
	n := 0
	w.vp2Targets(o, target, func(v *Object) bool {
		if v != t && !inFrustum(fx, fy, v.X, v.Y) {
			return true
		}
		w.attackFull(o, v, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, v, combo)
		n++
		return n < 10
	})
	return true
}

// angleByte returns the 0..255 angle from (x1,y1) to (x2,y2).
func angleByte(x1, y1, x2, y2 int) int {
	deg := math.Atan2(float64(y2-y1), float64(x2-x1)) * 180 / math.Pi
	deg -= 90
	for deg < 0 {
		deg += 360
	}
	return int(deg*255/360) & 0xff
}

// skillFireBurst is FUN_004fbf00 (61): the target, then up to 10 others
// within Radio of the target.
func (w *World) skillFireBurst(o *Object, ref *skillRef, target int, combo bool) bool {
	t := w.targetObj(target)
	if t == nil || !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	w.sendSkillTo(o, ref.Num, target, true)
	w.attackFull(o, t, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
	w.comboMark(o, t, combo)
	n := 0
	w.vp2Targets(o, target, func(v *Object) bool {
		if v == t || !w.radioOK(ref.Num, t.X, t.Y, v.X, v.Y) {
			return true
		}
		w.attackFull(o, v, attackArgs{Skill: ref.Num, Ref: ref, Combo: combo})
		w.comboMark(o, v, combo)
		n++
		return n < 10
	})
	return true
}

// skillElectricSpark is FUN_004fc640 (65): the SkillElect.hit sector of
// the cast angle (36 sectors of 10 degrees, 21x21 cells around the
// caster); message 0x32 after 1000 ms to up to 20 targets.
func (w *World) skillElectricSpark(o *Object, ref *skillRef, x, y int, combo bool) bool {
	deg := int(math.Atan2(float64(y-o.Y), float64(x-o.X)) * 180 / math.Pi)
	if deg < 0 {
		deg += 360
	}
	n := 0
	w.vp2Targets(o, -1, func(t *Object) bool {
		if !w.hitboxHas(deg/10, t.X-o.X+10, t.Y-o.Y+10) {
			return true
		}
		param := 0
		if combo {
			param = 1
		}
		w.queueSkillHit(o, 0x32, t.Index, 1000, ref.Num, param)
		w.comboMark(o, t, combo)
		n++
		return n < 20
	})
	return true
}

// hitboxHas reads DAT_015ae9dc[dx + (sector*21 + dy)*21].
func (w *World) hitboxHas(sector, dx, dy int) bool {
	if w.skills == nil || w.skills.hitbox == nil || sector < 0 || sector >= 36 || dx < 0 || dx >= 21 || dy < 0 || dy >= 21 {
		return false
	}
	return w.skills.hitbox[dx+(sector*21+dy)*21] != 0
}

// --- delayed hits ---------------------------------------------------------------

// queueSkillHit is FUN_00510690 on the caster's queue (30 slots).
func (w *World) queueSkillHit(o *Object, code, target, delayMs, skill, param int) {
	st := sk(o)
	if len(st.Pending) >= 30 {
		return
	}
	st.Pending = append(st.Pending, delayedHit{Code: code, Target: target, Skill: skill, Param: param,
		Due: w.now().Add(time.Duration(delayMs) * time.Millisecond)})
}

// skillMessages is the FUN_004c48b0 loop over the caster's queue with
// FUN_004c4280: 0x32 attacks with the combo flag, 0x35 also sends 0x19.
func (w *World) skillMessages(o *Object) {
	st := o.Skill
	if st == nil || len(st.Pending) == 0 {
		return
	}
	now := w.now()
	keep := st.Pending[:0]
	var due []delayedHit
	for _, m := range st.Pending {
		if now.After(m.Due) {
			due = append(due, m)
		} else {
			keep = append(keep, m)
		}
	}
	st.Pending = keep
	for _, m := range due {
		t := w.targetObj(m.Target)
		if t == nil || t.State != StatePlaying || !o.Alive {
			continue
		}
		var ref *skillRef
		if m.Skill != 0 && o.P != nil {
			if ref = w.findSkill(o, m.Skill); ref == nil {
				continue
			}
		}
		switch m.Code {
		case 10:
			// Reflect (FUN_00510550 message 10): a fixed-damage hit back.
			w.attackFull(o, t, attackArgs{Fixed: m.Param})
		case 0x32:
			w.attackFull(o, t, attackArgs{Skill: m.Skill, Ref: ref, Combo: m.Param&1 != 0})
		case 0x34:
			w.attackFull(o, t, attackArgs{Skill: m.Skill, Ref: ref, Hits: m.Param})
		case 0x35:
			w.attackFull(o, t, attackArgs{Skill: m.Skill, Ref: ref, SendSkill: true, Combo: m.Param&1 != 0})
		case 0x33:
			// Dark Raven hit (FUN_0046b5a0, pet_raven.go).
			w.ravenAttack(o, t, m.Send, m.Param != 0)
		}
	}
}

func registerSkill(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			w.skills = &skillWorld{}
			return nil
		}
		sw, err := loadSkillWorld(w.dataDir, w.configDir)
		if err != nil {
			return fmt.Errorf("skill data: %w", err)
		}
		w.skills = sw
		return nil
	})
	h.worldHandlers[0x19] = (*World).handleSkillTarget
	h.worldHandlers[0x1E] = (*World).handleSkillArea
	h.worldHandlers[0x10] = (*World).handleSkillMulti
	h.tick100 = append(h.tick100, func(w *World) {
		for i := PlayerStart; i < MaxObjects; i++ {
			o := w.objects[i]
			if o.State != StatePlaying || o.Skill == nil {
				continue
			}
			w.novaTick(o)
			w.skillMessages(o)
		}
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { o.Skill = nil })
}
