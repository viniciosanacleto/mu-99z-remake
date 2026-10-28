package game

import (
	"fmt"
	"math"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Monster attacks: the attack choice of FUN_004b7d00, the area attack
// FUN_004b7a20 (+ FUN_00500b70), the monster MagicList built by setMonster
// (FUN_004b4fe0 / FUN_004f8b60), the CMonsterSkillManager of
// Monster/Skill/*.txt (FUN_004bdcd0, FUN_004bdd10, FUN_004bde50,
// FUN_004be5b0 and the elements FUN_004bc500..FUN_004bd4c0) and the traps
// (FUN_004b5be0).

var (
	// monsterSkillAttackHook is CSkillManager::UseAttackSkill
	// (FUN_004f8dd0) for a monster casting skill on t; the skills module
	// sets it. Without it the skill is resolved as a plain CAttack with the
	// skill number.
	monsterSkillAttackHook func(w *World, o, t *Object, skill int) bool
	// addEffectHook / removeEffectHook are the buff manager calls the
	// monster skill elements use (FUN_00478ae0 add effect with duration in
	// seconds, FUN_00478d30 remove); set by the buff module.
	addEffectHook    func(w *World, o *Object, effect, value, seconds int) bool
	removeEffectHook func(w *World, o *Object, effect int)
)

// monsterSkillList is the MagicList setMonster builds: slot 0 is the
// AttackType (150 -> 50) and some classes get more skills; nothing when
// AttackType is 0.
func monsterSkillList(class, attackType int) []int {
	if attackType == 0 {
		return nil
	}
	s := []int{attackType}
	if attackType == 150 {
		s[0] = 50
	}
	in := func(v ...int) bool {
		for _, c := range v {
			if c == class {
				return true
			}
		}
		return false
	}
	switch {
	case in(0x42, 0x49):
		s = append(s, 0x11)
	case in(0x4d):
		s = append(s, 3)
	case in(0x59, 0x5f, 0x70, 0x76, 0x7c, 0x82, 0x8f, 0x1b1):
		s = append(s, 3)
	case in(0x90, 0xae, 0xb6, 0xbe, 0x104, 0x91, 0xaf, 0xb7, 0xbf, 0x105, 0x92, 0xb0, 0xb8, 0xc0, 0x106,
		0x93, 0xb1, 0xb9, 0xc1, 0x107, 0x94, 0xb2, 0xba, 0xc2, 0x108, 0xa0, 0xb4, 0xbc, 0xc4, 0x10a,
		0xa3, 0xa5, 0xa7, 0xa9, 0xab, 0xad, 0x1ab):
		s = append(s, 0x11)
	case in(0x95, 0xb3, 0xbb, 0xc3, 0x109):
		s = append(s, 1, 0x11)
	case in(0xa1, 0xb5, 0xbd, 0xc5, 0x10b):
		s = append(s, 1, 0x11, 0x37, 200, 0xc9, 0xca)
	case in(0x211, 0x212):
		s = append(s, 0x32)
	case in(0x215):
		s = append(s, 0xef)
	case in(0x216):
		s = append(s, 0xeb)
	case in(0x217):
		s = append(s, 0x29)
	case in(0x218):
		s = append(s, 0x27)
	case in(0x219):
		s = append(s, 0x38)
	case in(0x21a):
		s = append(s, 0x3e)
	case in(0x21b):
		s = append(s, 0xe6)
	case in(0x231):
		s = append(s, 9, 0x26, 0xee)
	}
	return s
}

// skillSlot returns the skill number of a MagicList slot (-1 = empty).
func (m *monsterState) skillSlot(slot int) int {
	if slot < 0 || slot >= len(m.Skills) {
		return -1
	}
	return m.Skills[slot]
}

// monsterMagic is a monster sending "C1 07 19 slot target" to the magic
// handler FUN_00500e70: the slot must hold a skill.
func (w *World) monsterMagic(o, t *Object, slot int) {
	skill := o.Mon.skillSlot(slot)
	if skill < 0 || !t.Alive {
		return
	}
	if monsterSkillAttackHook != nil && monsterSkillAttackHook(w, o, t, skill) {
		return
	}
	if w.cellHas(o, gamedata.AttrSafeZone) || w.cellHas(t, gamedata.AttrSafeZone) {
		return
	}
	w.attack(o, t, skill, true)
}

// monsterD7 is the "C1 07 D7 target 0x78 dir" a monster feeds to the D7
// handler.
func (w *World) monsterD7(o, t *Object) {
	w.attackRequest(o, []byte{0xC1, 0x07, 0xD7, byte(t.Index >> 8), byte(t.Index), 0x78, byte(o.Dir)})
}

func classIn(class int, v ...int) bool {
	for _, c := range v {
		if c == class {
			return true
		}
	}
	return false
}

// monsterAttackAction is FUN_004b7d00: the per-class choice between the
// D7 attack and the magic slots.
func (w *World) monsterAttackAction(o, t *Object) {
	c := o.Class
	switch {
	case classIn(c, 0x42, 0x49, 0x4d, 0x59, 0x5f, 0x70, 0x76, 0x7c, 0x82, 0x8f, 0x1b1):
		w.monsterMagic(o, t, 1)
	case classIn(c, 0x90, 0xae, 0xb6, 0xbe, 0x104, 0x10c):
		if monsterRand()&1 == 0 {
			w.monsterMagic(o, t, 0)
		} else {
			w.monsterD7(o, t)
		}
	case classIn(c, 0x91, 0x92, 0x93, 0x94, 0xa0, 0xaf, 0xb0, 0xb1, 0xb2, 0xb4, 0xb7, 0xb8, 0xb9, 0xba, 0xbc,
		0xbf, 0xc0, 0xc1, 0xc2, 0xc4, 0x105, 0x106, 0x107, 0x108, 0x10a, 0x10d, 0x10e, 0x10f, 0x110, 0x112):
		if monsterRand()&1 == 0 {
			w.monsterMagic(o, t, 0)
		} else {
			w.monsterD7(o, t)
		}
	case classIn(c, 0x95, 0xb3, 0xbb, 0xc3, 0x109, 0x111):
		w.monsterMagic(o, t, monsterRand()&1)
	case classIn(c, 0xa1, 0xb5, 0xbd, 0xc5, 0x10b, 0x113, 0x152):
		w.monsterMagic(o, t, monsterRand()%6+1)
		w.monsterAreaAttack(o)
	case classIn(c, 0xa3, 0xa5, 0xa7, 0xa9, 0xab, 0xad, 0x1ab):
		w.monsterMagic(o, t, 0)
	case classIn(c, 0x211, 0x212, 0x215, 0x216, 0x217, 0x218, 0x219, 0x21a, 0x21b, 0x231):
		// These send a C1 0C 1E position skill (not reproduced): the
		// first skill is cast on the target instead.
		w.monsterMagic(o, t, 0)
	case o.Mon.AttackType == 0 || o.Mon.AttackType > 99:
		w.monsterD7(o, t)
	default:
		w.monsterMagic(o, t, 0)
	}
}

// packetAreaSkill is C3 09 1E skill user(BE) X Y dir (FUN_00501740).
func packetAreaSkill(skill, user, x, y, dir int) []byte {
	p := newPacket(frame.C3, 0x1E, -1, 9)
	p[3] = byte(skill)
	p[4], p[5] = byte(user>>8), byte(user)
	p[6], p[7], p[8] = byte(x), byte(y), byte(dir)
	return p
}

// areaSkills are the skill numbers FUN_00500b70 accepts for a
// multi-target attack.
var areaSkills = map[int]bool{5: true, 8: true, 9: true, 10: true, 12: true, 13: true, 14: true, 24: true,
	47: true, 50: true, 52: true, 55: true, 78: true}

// monsterAreaAttack is FUN_004b7a20: the C3 09 1E effect of slot 0 at the
// monster, then every player of its VP2 closer than 6 cells (up to 20) is
// hit. The Kalima/Blood Castle bosses (0xa1, 0xb5, 0xbd, 0xc5, 0x10b, 0x113,
// 0x152) hit all of them; the others go through the multi-target handler
// FUN_00500b70 (area skills only, at most 5 targets).
func (w *World) monsterAreaAttack(o *Object) {
	skill := o.Mon.skillSlot(0)
	if skill < 0 {
		return
	}
	w.broadcast(o, packetAreaSkill(skill, o.Index, o.X, o.Y, 0))
	var targets []*Object
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index >= MaxObjects || e.Type != byte(TypePlayer) {
			continue
		}
		t := w.objects[e.Index]
		if distance(o, t) >= 6 {
			continue
		}
		targets = append(targets, t)
		if len(targets) >= 20 {
			break
		}
	}
	if !classIn(o.Class, 0xa1, 0xb5, 0xbd, 0xc5, 0x10b, 0x113, 0x152) {
		if !areaSkills[skill] {
			return
		}
		if len(targets) > 5 {
			targets = targets[:5]
		}
	}
	for _, t := range targets {
		w.attack(o, t, skill, false)
	}
}

// --- traps -----------------------------------------------------------------

// isTrap: classes 100..110 and 0x20b (FUN_004b4090).
func isTrap(class int) bool { return (class >= 100 && class <= 0x6E) || class == 0x20B }

// trapAct is FUN_004b5be0: the trap picks a random player of its VP2 (not
// a GM, Authority 2 or gmFull) standing within AttackRange cells (square); with
// AttackRange 0 the first one found. A target means ActAttack with
// AttackSpeed delay, otherwise the delay is MoveSpeed (obj+0x374).
func (w *World) trapAct(o *Object) {
	m := o.Mon
	var cand []int
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index >= MaxObjects || e.Type != byte(TypePlayer) {
			continue
		}
		t := w.objects[e.Index]
		if gmAuthority(t) == 2 || gmFull(t) {
			continue
		}
		if t.State != StatePlaying || !t.Alive || w.cellHas(t, gamedata.AttrSafeZone) {
			continue
		}
		r := m.AttackRange
		if t.Map != o.Map || abs(t.X-o.X) > r || abs(t.Y-o.Y) > r {
			continue
		}
		cand = append(cand, e.Index)
		if r == 0 {
			break
		}
	}
	if len(cand) == 0 {
		m.Target = -1
		m.Delay = m.Info.MoveSpeed
		return
	}
	m.Target = cand[monsterRand()%len(cand)]
	m.ActAttack = true
	m.Delay = m.AttackSpeed
}

// gmAuthority is obj+0x250 (the GameMaster.txt level kept by the movement
// module).
func gmAuthority(o *Object) int {
	if o.Move == nil || o.P == nil {
		return 0
	}
	return o.Move.Authority
}

// gmFull stands for the original's Authority == 8 tests (PvP and Castle
// Siege immunity, no pushback, /hide, /superdrop, trade block, xshop GM
// packets). The rewrite gives the same privileges to every level from 8
// up, so the level 32 of GameMaster.txt is a superset of 8 (a project
// decision, U-GM-05).
func gmFull(o *Object) bool { return o.Move != nil && o.Move.Authority >= 8 }

// --- CMonsterSkillManager ------------------------------------------------------

// monsterSkillElement is a MonsterSkillElement.txt row.
type monsterSkillElement struct {
	Index       int
	Name        string
	Type        int // +0x3c
	SuccessRate int // +0x40
	Time        int // +0x44 ContinuanceTime (s)
	IncDecType  int // +0x48
	IncDecValue int // +0x4c
	Nullified   int // +0x50
}

// monsterSkillUnit is a MonsterSkillUnit.txt row (200 slots of 100 bytes).
type monsterSkillUnit struct {
	Index      int // +0x38 (sent in C1 08 69)
	TargetType int // +0x3c: 1 target, 2 self
	ScopeType  int // +0x40: -1 single, 0 distance, 1 area table
	ScopeValue int // +0x44
	Delay      int // +0x48
	Elements   [5]*monsterSkillElement
}

// monsterSkillInfo is a MonsterSkill.txt row (0x58 bytes).
type monsterSkillInfo struct {
	Monster int
	Types   [10]int
	Units   [10]*monsterSkillUnit
}

type monsterSkillData struct {
	byClass map[int]*monsterSkillInfo
}

// loadMonsterSkills reads the Monster/Skill tables of config/data/monster.toml.
func loadMonsterSkills(configDir string) (*monsterSkillData, error) {
	f, err := gamedata.LoadMonsterConfig(configDir)
	if err != nil {
		return nil, err
	}
	elems := map[int]*monsterSkillElement{}
	for _, r := range f.MonsterSkillElement.Rows {
		e := &monsterSkillElement{Index: r.Index, Name: r.Name, Type: r.Type, SuccessRate: r.SuccessRate,
			Time: r.ContinuanceTime, IncDecType: r.IncAndDecType, IncDecValue: r.IncAndDecValue, Nullified: r.NullifiedMagic}
		elems[e.Index] = e
	}
	units := map[int]*monsterSkillUnit{}
	for _, r := range f.MonsterSkillUnit.Rows {
		u := &monsterSkillUnit{Index: r.Index, TargetType: r.TargetType, ScopeType: r.ScopeType, ScopeValue: r.ScopeValue,
			Delay: r.Delay}
		for i, slot := range r.Slot {
			u.Elements[i] = elems[slot]
		}
		if u.Index >= 0 && u.Index < 200 {
			units[u.Index] = u
		}
	}
	d := &monsterSkillData{byClass: map[int]*monsterSkillInfo{}}
	for _, r := range f.MonsterSkill.Rows {
		info := &monsterSkillInfo{Monster: r.Monster}
		for i, p := range r.Skill {
			info.Types[i] = p[0]
			info.Units[i] = units[p[1]]
		}
		if info.Monster >= 0 && info.Monster < 1000 {
			d.byClass[info.Monster] = info
		}
	}
	return d, nil
}

func init() {
	startHooks = append(startHooks, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		d, err := loadMonsterSkills(w.configDir)
		if err != nil {
			return fmt.Errorf("monster skills: %w", err)
		}
		w.mw().skills = d
		return nil
	})
}

// hasMonsterSkill is FUN_004bdcd0: the class has a MonsterSkill row with a
// unit and a slot of Type 0.
func (w *World) hasMonsterSkill(o *Object) bool {
	d := w.mw().skills
	if d == nil {
		return false
	}
	info := d.byClass[o.Class]
	if info == nil || info.unitCount() == 0 {
		return false
	}
	for _, t := range info.Types {
		if t == 0 {
			return true
		}
	}
	return false
}

func (info *monsterSkillInfo) unitCount() int {
	n := 0
	for _, u := range info.Units {
		if u != nil {
			n++
		}
	}
	return n
}

// packetMonsterSkill is C1 08 69 unit monster(LE) target(LE)
// (FUN_004daa40).
func packetMonsterSkill(unit, monster, target int) []byte {
	p := newPacket(frame.C1, 0x69, -1, 8)
	p[3] = byte(unit)
	u16le(p[4:], monster)
	u16le(p[6:], target)
	return p
}

// useMonsterSkill is FUN_004bde50: a random unit among the Type 0 slots
// (FUN_004bdd10), C1 08 69 to the viewport and the elements on the target
// (TargetType 2 = itself) or, with a scope, on every live player of the
// VP2 inside ScopeValue.
func (w *World) useMonsterSkill(o, t *Object) {
	d := w.mw().skills
	if d == nil {
		return
	}
	info := d.byClass[o.Class]
	if info == nil || info.unitCount() == 0 {
		return
	}
	var slots []int
	for i, ty := range info.Types {
		if ty == 0 {
			slots = append(slots, i)
		}
	}
	if len(slots) == 0 {
		return
	}
	u := info.Units[slots[monsterRand()%len(slots)]]
	if u == nil {
		return
	}
	if u.TargetType == 2 {
		t = o
	}
	w.broadcast(o, packetMonsterSkill(u.Index, o.Index, t.Index))
	if u.ScopeType == -1 {
		w.runSkillUnit(u, o, t)
		if u.TargetType == 1 {
			w.skillUnitAttack(o, t)
		}
		return
	}
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		p := w.objects[e.Index]
		if p.Type != TypePlayer || !p.Alive || p.Live != LiveNormal {
			continue
		}
		// ScopeType 1 uses the skill area table (FUN_004bd920 /
		// FUN_004f75d0); it is approximated by the same distance test.
		if distance(o, p) < u.ScopeValue {
			w.runSkillUnit(u, o, p)
			if u.TargetType == 1 {
				w.skillUnitAttack(o, p)
			}
		}
	}
}

// skillUnitAttack is FUN_004be030: only some event monsters follow the
// unit with a CAttack.
func (w *World) skillUnitAttack(o, t *Object) {
	c := o.Class
	if (c > 0x1c5 && c < 0x1cc) || (c > 0x1f7 && c < 0x20a) || c == 0x232 || c == 0x233 || c == 0x235 {
		w.attack(o, t, 0, false)
	}
}

// runSkillUnit is FUN_004be5b0: each element passes rand % 100 <=
// SuccessRate (type 0 minus the target's obj+0x868, 0 here).
func (w *World) runSkillUnit(u *monsterSkillUnit, o, t *Object) {
	for _, e := range u.Elements {
		if e == nil {
			continue
		}
		if monsterRand()%100 > e.SuccessRate {
			continue
		}
		w.applySkillElement(e, o, t)
	}
}

// elementDelta is the IncAndDecType arithmetic of the life / mana /
// durability elements: 1/101 add value% of the current amount, 2/102
// subtract it, 11/111 add value, 12/112 subtract value (truncated).
func elementDelta(e *monsterSkillElement, cur float32) int {
	d := 0
	switch e.IncDecType {
	case 1, 0x65:
		d = int(float64(cur) * float64(e.IncDecValue) / 100)
	case 2, 0x66:
		d += int(float64(cur) * float64(e.IncDecValue) / -100)
	}
	switch e.IncDecType {
	case 0xb, 0x6f:
		d += e.IncDecValue
	case 0xc, 0x70:
		d -= e.IncDecValue
	}
	return d
}

// applySkillElement runs one element (FUN_004be5b0 switch).
func (w *World) applySkillElement(e *monsterSkillElement, o, t *Object) {
	switch e.Type {
	case 0: // FUN_004bc500: stun (effect 61) unless the target has effect 134
		if w.hasEffect(t, 0x86) {
			return
		}
		if addEffectHook != nil {
			addEffectHook(w, t, 0x3D, 0, max(e.Time, 0))
		}
	case 2: // FUN_004bc5d0: life, capped at MaxLife + AddLife, C1 26 FF
		if t.P == nil || t.P.Char == nil {
			if t.Mon != nil {
				t.Mon.Life = min(max(t.Mon.Life+float32(elementDelta(e, t.Mon.Life)), 0), t.Mon.MaxLife)
			}
			return
		}
		c := t.P.Char
		c.Life = min(max(c.Life+float32(elementDelta(e, c.Life)), 0), float32(t.P.AddLife)+c.MaxLife)
		if sendLifeHook != nil {
			sendLifeHook(w, t)
		}
	case 3: // FUN_004bc770: mana, C1 27 FF
		if t.P == nil || t.P.Char == nil {
			return
		}
		c := t.P.Char
		c.Mana = min(max(c.Mana+float32(elementDelta(e, c.Mana)), 0), float32(t.P.AddMana)+c.MaxMana)
		if sendManaHook != nil {
			sendManaHook(w, t)
		}
	case 7: // FUN_004bcbe0: durability of a random armor slot (2..7)
		if t.P == nil {
			return
		}
		slot := monsterRand()%6 + 2
		it := &t.P.Inventory[slot]
		if it.IsEmpty() {
			return
		}
		it.Durability = float32(math.Max(float64(it.Durability+float32(elementDelta(e, it.Durability))), 0))
		w.send(t, packetItemDurability(slot, truncDur(it.Durability), 0))
	case 14: // FUN_004bd080: removes effect 28 / 27 from a player
		if t.P != nil && removeEffectHook != nil && (e.Nullified == 0x1c || e.Nullified == 0x1b) {
			removeEffectHook(w, t, e.Nullified)
		}
	case 16: // FUN_004bd0f0: magic immunity (effect 63) for Time seconds
		if addEffectHook != nil {
			addEffectHook(w, t, 0x3F, e.Nullified, max(e.Time, 0))
		}
	default:
		// 9 (push back FUN_005114f0), 4/5/6/8 and 15..21 are not
		// reproduced; see docs/server-plan/10b-monstros-drops.md.
	}
}
