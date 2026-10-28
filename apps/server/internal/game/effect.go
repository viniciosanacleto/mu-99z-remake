package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
)

// Effect system (CEffectManager / CEffect of the original):
//
//   - Data/Skill/EffectList.txt (FUN_004786c0): 128 table entries of 0x4c
//     bytes {index, group, item, name[32], save, type, flag, duration,
//     power[4]};
//   - 16 CEffect slots per object at obj+0x5fc, 0x1c bytes each: +4 index
//     (0xFF = empty), +5 group, +6 save, +7 type, +8 flag, +0x10 count
//     (seconds left), +0x14..+0x1a value[4] (u16);
//   - AddEffect FUN_00478ae0, EffectOn FUN_00478ef0, EffectOff
//     FUN_004798d0, RemoveEffect FUN_00478d30, ClearAllEffect
//     FUN_0047a540, the 1-second timer FUN_00478910 with the periodic
//     damage FUN_0047a6b0, and the state packet C1 0C 07 (FUN_0047ab60)
//     whose bit comes from FUN_0047a9e0 (table at 0x47ab10).

// eff returns the effect state of o, creating it on first use.
func eff(o *Object) *effect.State {
	if o.Eff == nil {
		o.Eff = effect.NewState()
	}
	return o.Eff
}

// effOpt returns the effect options of o (zero when it has none).
func effOpt(o *Object) effect.Options {
	if o.Eff == nil {
		return effect.Options{}
	}
	return o.Eff.Opt
}

// sendEffectState is the tail of FUN_0047ab60: to the object itself and,
// outside Chaos Castle (maps 18..23), to the players that see it.
func (w *World) sendEffectState(o *Object, add bool, index int) {
	bit := effect.StateBit(index)
	if bit == 0 {
		return
	}
	p := effect.PacketState(add, bit, o.Index)
	if o.Type == TypePlayer {
		w.send(o, p)
	}
	if o.Map < 18 || o.Map > 23 {
		w.broadcast(o, p)
	}
}

// hasEffectIndex is FUN_00478ec0.
func hasEffectIndex(o *Object, index int) bool {
	if o.Eff == nil {
		return false
	}
	for _, s := range o.Eff.Slots {
		if s.Index >= 0 && s.Index == index {
			return true
		}
	}
	return false
}

// effectSlotOf is FUN_00478e90.
func effectSlotOf(o *Object, index int) *effect.Slot {
	if o.Eff == nil {
		return nil
	}
	for i := range o.Eff.Slots {
		if s := &o.Eff.Slots[i]; s.Index >= 0 && s.Index == index {
			return s
		}
	}
	return nil
}

// addEffect is FUN_00478ae0(obj, 0, index, duration, v1..v4, send): an
// effect of the same group is replaced (with EffectOverwriteMode 0 a
// weaker copy of the same effect is refused); otherwise the first free
// slot is used. The EffectList.txt power columns override the values when
// set, and a duration column overrides the caller's.
func (w *World) addEffect(o *Object, index, duration int, v [4]int, send bool) bool {
	return w.addEffectEx(o, index, duration, v, send, false)
}

// addEffectEx is FUN_00478ae0 with param_2: load is set by the character
// load (FUN_004cc730, effect_save.go), where the duration is the saved
// one (the EffectList.txt duration is not applied). For Type 2 effects
// the duration is the Unix time they end (FUN_004785e0: refused once
// past).
func (w *World) addEffectEx(o *Object, index, duration int, v [4]int, send, load bool) bool {
	info, ok := w.effectTable()[index]
	if !ok {
		return false
	}
	for i, p := range info.Power {
		if p != -1 {
			v[i] = p & 0xFFFF
		} else {
			v[i] &= 0xFFFF
		}
	}
	if !load && info.Duration != -1 {
		duration = info.Duration
		if info.Type == 2 {
			duration += int(w.now().Unix())
		}
	}
	var expire int64
	if info.Type == 2 {
		expire = int64(duration)
		if duration = int(expire - w.now().Unix()); duration < 1 {
			return false
		}
	}
	e := eff(o)
	newSlot := effect.Slot{Index: index, Group: info.Group, Save: info.Save, Type: info.Type, Flag: info.Flag,
		Count: duration, Expire: expire, Value: v}
	for i := range e.Slots {
		s := &e.Slots[i]
		if s.Index < 0 || s.Group != info.Group {
			continue
		}
		if s.Index == index && w.cfg.Int("Common", "EffectOverwriteMode", 0) == 0 {
			for k := range v {
				if v[k] < s.Value[k] {
					return false
				}
			}
		}
		w.applyEffect(o, s, -1, !load)
		if send {
			w.sendEffectState(o, false, s.Index)
		}
		*s = newSlot
		if send {
			w.sendEffectState(o, true, index)
		}
		w.applyEffect(o, s, 1, !load)
		return true
	}
	for i := range e.Slots {
		s := &e.Slots[i]
		if s.Index >= 0 {
			continue
		}
		*s = newSlot
		if send {
			w.sendEffectState(o, true, index)
		}
		w.applyEffect(o, s, 1, !load)
		return true
	}
	return false
}

// removeEffect is FUN_00478d30: drop the first slot holding index.
func (w *World) removeEffect(o *Object, index int) bool {
	s := effectSlotOf(o, index)
	if s == nil {
		return false
	}
	w.effectOff(o, s)
	w.sendEffectState(o, false, s.Index)
	*s = effect.Slot{Index: -1}
	return true
}

// clearEffects is FUN_0047a540: remove every effect of type 0 (the
// timed buffs/debuffs; type 1 such as Game Master survives).
func (w *World) clearEffects(o *Object) {
	if o.Eff == nil {
		return
	}
	for i := range o.Eff.Slots {
		s := &o.Eff.Slots[i]
		if s.Index < 0 || s.Type != 0 {
			continue
		}
		w.effectOff(o, s)
		w.sendEffectState(o, false, s.Index)
		*s = effect.Slot{Index: -1}
	}
}

// effectOn is the 97D/99z part of FUN_00478ef0: ViewSkillState bit and
// the option fields of the effect, then CalcCharacter.
func (w *World) effectOn(o *Object, s *effect.Slot) { w.applyEffect(o, s, 1, true) }

// effectOff is FUN_004798d0 (the same fields subtracted).
func (w *World) effectOff(o *Object, s *effect.Slot) { w.applyEffect(o, s, -1, true) }

func (w *World) applyEffect(o *Object, s *effect.Slot, sign int, recalc bool) {
	bit := effect.StateBit(s.Index)
	if sign > 0 {
		o.ViewSkillState |= bit
	} else {
		o.ViewSkillState &^= bit
	}
	opt := &eff(o).Opt
	v0, v1 := sign*s.Value[0], sign*s.Value[1]
	var se *statsEffects
	if o.P != nil {
		se = &w.statsOf(o).Effects
	}
	switch s.Index {
	case effect.GreaterDamage, 0x5e:
		opt.AddPhysiDamage += v0
		opt.AddMagicDamage += v0
		opt.AddCurseDamage += v0
	case effect.GreaterDefense, 0x25, 0x2d, 0x5f, 0xc4:
		opt.AddDefense += v0
	case effect.ElfBuffer, 0x5b:
		opt.AddPhysiDamage += v0
		opt.AddMagicDamage += v0
		opt.AddCurseDamage += v0
		opt.AddDefense += v1
	case effect.ManaShield, 0x22:
		opt.DamageReduction += v0
	case effect.CriticalDamage:
		opt.AddCriticalDamage += v0
	case effect.GreaterLife:
		if se != nil {
			se.LifeRate[0] += v0
		}
	case effect.GreaterMana:
		if se != nil {
			se.ManaRate[0] += v0
		}
	case effect.SoulPotion:
		if se != nil {
			se.PhysiSpeed += v0
			se.MagicSpeed += v0
		}
		opt.SoulExtra += v1
	case effect.Ice, 0x56, 0x8d:
		// obj+0x296 (slow walk, +300 ms per step) and obj+0x268.
		mv(o).Slow = sign > 0
	case effect.DamageReflect:
		opt.DamageReflect += v0
	}
	if recalc && o.Type == TypePlayer && w.hooks.recalc != nil {
		w.hooks.recalc(w, o)
	}
}

// dropEffects removes every effect without packets or recalculation (the
// object is leaving the map; the option fields go back to zero).
func (w *World) dropEffects(o *Object) {
	if o.Eff == nil {
		return
	}
	for i := range o.Eff.Slots {
		if s := &o.Eff.Slots[i]; s.Index >= 0 {
			w.applyEffect(o, s, -1, false)
		}
	}
	o.Eff = nil
}

// effectSecond is FUN_00478910 for one object: count down every effect,
// run the periodic damage and remove the expired ones.
func (w *World) effectSecond(o *Object) {
	if o.Eff == nil {
		return
	}
	for i := range o.Eff.Slots {
		s := &o.Eff.Slots[i]
		if s.Index < 0 || s.Count == 0 {
			continue
		}
		s.Count--
		w.effectTickDamage(o, s)
		if o.Eff == nil {
			return
		}
		if s.Count == 0 && s.Index >= 0 {
			w.effectOff(o, s)
			w.sendEffectState(o, false, s.Index)
			*s = effect.Slot{Index: -1}
		}
	}
}

// effectTickDamage is FUN_0047a6b0 for the 97D/99z damage-over-time
// effects (55 Poison and the Decay copy of it): every value[1] seconds
// the object loses life*value[2]/100 (capped by value[3] when set),
// dealt by value[0] and shown with damage effect 5 (type 2).
func (w *World) effectTickDamage(o *Object, s *effect.Slot) {
	if s.Index != effect.Poison || s.Value[1] == 0 || s.Count%s.Value[1] != 0 {
		return
	}
	if o.Type == TypeNPC || !o.Alive || s.Value[0] >= MaxObjects {
		return
	}
	a := w.objects[s.Value[0]]
	dmg := int(float64(objLife(o)) * float64(s.Value[2]) / 100)
	if s.Value[3] != 0 && dmg > s.Value[3] {
		dmg = s.Value[3]
	}
	life := objLife(o)
	if float32(dmg) <= life {
		life -= float32(dmg)
	} else {
		life = 0
	}
	setObjLife(o, life)
	if dmg > 0 && a.State == StatePlaying {
		w.lifeCheck(a, o, dmg, 5, 0)
	}
}

// effectTable returns the EffectList.txt table (empty when not loaded).
func (w *World) effectTable() map[int]effect.Info {
	if w.skills == nil || w.skills.effects == nil {
		return map[int]effect.Info{}
	}
	return w.skills.effects
}

// handleCancelEffect is 1B (FUN_00501220): [3] skill; the effect of that
// skill (SkillList.txt Effect column) is removed from the player.
func (w *World) handleCancelEffect(o *Object, p []byte) {
	if len(p) < 4 || !canPlayerAct(o) {
		return
	}
	info, ok := w.data.Skills[int(p[3])]
	if !ok {
		return
	}
	w.removeEffect(o, info.Effect)
}

func registerEffect(h *hooks) {
	h.worldHandlers[0x1B] = (*World).handleCancelEffect
	h.moveEffect = func(w *World, o *Object, e int) bool { return hasEffectIndex(o, e) }
	// Blood Castle gates/statues get 0x3F/0x40 (monster physical / magic
	// immunity, FUN_0040b110 makes every hit miss).
	h.eventEffect = func(w *World, o *Object, effect int, add bool) {
		if add {
			w.addEffect(o, effect, 0, [4]int{}, true)
		} else {
			w.removeEffect(o, effect)
		}
	}
	h.questEffect = func(w *World, o *Object, effect, value int) {
		w.addEffect(o, effect, value, [4]int{}, true)
	}
	h.tick1000 = append(h.tick1000, func(w *World) {
		for _, o := range w.objects {
			if o.State == StatePlaying && o.Eff != nil {
				w.effectSecond(o)
			}
		}
	})
	h.leaveWorld = append(h.leaveWorld, func(w *World, o *Object) { w.dropEffects(o) })
}
