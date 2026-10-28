package game

import (
	"fmt"
	"math"
	"slices"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/monsterskill"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// Monster attacks: the attack choice of FUN_004b7d00, the area attack
// FUN_004b7a20 (+ FUN_00500b70), the monster MagicList built by setMonster
// (FUN_004b4fe0 / FUN_004f8b60), the CMonsterSkillManager of
// Monster/Skill/*.txt (FUN_004bdcd0, FUN_004bdd10, FUN_004bde50,
// FUN_004be5b0 and the elements FUN_004bc500..FUN_004bd4c0) and the traps
// (FUN_004b5be0).

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
	if w.hooks.monsterSkillAttack != nil && w.hooks.monsterSkillAttack(w, o, t, skill) {
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
	w.broadcast(o, monsterskill.PacketArea(skill, o.Index, o.X, o.Y, 0))
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
	if !slices.Contains([]int{0xa1, 0xb5, 0xbd, 0xc5, 0x10b, 0x113, 0x152}, o.Class) {
		if !monsterskill.AreaSkills[skill] {
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

func registerMonsterSkill(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		d, err := monsterskill.Load(w.configDir)
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
	info := d.ForClass(o.Class)
	if info == nil || info.UnitCount() == 0 {
		return false
	}
	for _, t := range info.Types {
		if t == 0 {
			return true
		}
	}
	return false
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
	info := d.ForClass(o.Class)
	if info == nil || info.UnitCount() == 0 {
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
	w.broadcast(o, monsterskill.PacketUse(u.Index, o.Index, t.Index))
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
func (w *World) runSkillUnit(u *monsterskill.Unit, o, t *Object) {
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

// applySkillElement runs one element (FUN_004be5b0 switch).
func (w *World) applySkillElement(e *monsterskill.Element, o, t *Object) {
	switch e.Type {
	case 0: // FUN_004bc500: stun (effect 61) unless the target has effect 134
		if w.hasEffect(t, 0x86) {
			return
		}
		if w.hooks.addEffect != nil {
			w.hooks.addEffect(w, t, 0x3D, 0, max(e.Time, 0))
		}
	case 2: // FUN_004bc5d0: life, capped at MaxLife + AddLife, C1 26 FF
		if t.P == nil || t.P.Char == nil {
			if t.Mon != nil {
				t.Mon.Life = min(max(t.Mon.Life+float32(monsterskill.ElementDelta(e, t.Mon.Life)), 0), t.Mon.MaxLife)
			}
			return
		}
		c := t.P.Char
		c.Life = min(max(c.Life+float32(monsterskill.ElementDelta(e, c.Life)), 0), float32(t.P.AddLife)+c.MaxLife)
		if w.hooks.sendLife != nil {
			w.hooks.sendLife(w, t)
		}
	case 3: // FUN_004bc770: mana, C1 27 FF
		if t.P == nil || t.P.Char == nil {
			return
		}
		c := t.P.Char
		c.Mana = min(max(c.Mana+float32(monsterskill.ElementDelta(e, c.Mana)), 0), float32(t.P.AddMana)+c.MaxMana)
		if w.hooks.sendMana != nil {
			w.hooks.sendMana(w, t)
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
		it.Durability = float32(math.Max(float64(it.Durability+float32(monsterskill.ElementDelta(e, it.Durability))), 0))
		w.send(t, packetItemDurability(slot, truncDur(it.Durability), 0))
	case 14: // FUN_004bd080: removes effect 28 / 27 from a player
		if t.P != nil && w.hooks.removeEffect != nil && (e.Nullified == 0x1c || e.Nullified == 0x1b) {
			w.hooks.removeEffect(w, t, e.Nullified)
		}
	case 16: // FUN_004bd0f0: magic immunity (effect 63) for Time seconds
		if w.hooks.addEffect != nil {
			w.hooks.addEffect(w, t, 0x3F, e.Nullified, max(e.Time, 0))
		}
	default:
		// 9 (push back FUN_005114f0), 4/5/6/8 and 15..21 are not
		// reproduced; see docs/server-plan/10b-monstros-drops.md.
	}
}
