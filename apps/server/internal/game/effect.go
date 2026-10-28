package game

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
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

const effectSlots = 16

// Effect indexes used by the 97D/99z skills (EffectList.txt "Index").
const (
	effGreaterDamage    = 1
	effGreaterDefense   = 2
	effElfBuffer        = 3
	effManaShield       = 4
	effCriticalDamage   = 5
	effInfinityArrow    = 6
	effGreaterLife      = 8
	effGreaterMana      = 9
	effBlessPotion      = 10
	effSoulPotion       = 11
	effInvisibility     = 18
	effBrand            = 19
	effPoison           = 55
	effIce              = 56
	effIceArrow         = 57
	effFireSlash        = 58
	effPhysiImmunity    = 59
	effMagicImmunity    = 60
	effStern            = 61
	effMonPhysiImmunity = 63
	effMonMagicImmunity = 64
	effDamageReflect    = 71
	effImmunity34       = 0x22 // FUN_0040b110: full immunity (every hit misses)
	effRemovedOnHit     = 0x48 // FUN_0040b110: removed from the target of a damaging hit
)

// effectInfo is an EffectList.txt row.
type effectInfo struct {
	Index, Group, Item int
	Name               string
	Save, Type, Flag   int
	Duration           int // -1 = "*" (the caller's duration)
	Power              [4]int
}

// effectSlot is one CEffect (Index -1 = empty slot).
type effectSlot struct {
	Index, Group     int
	Save, Type, Flag int
	Count            int   // seconds left (+0x10); 0 never expires
	Expire           int64 // +0xc: Unix time the effect ends (Type 2 only)
	Value            [4]int
}

// effectOptions are the CEffect option fields of the object (obj+0x6e8..)
// read by CAttack. Player-only fields that CalcCharacter reads (speeds,
// life/mana rates) live in statsEffects (stats.go).
type effectOptions struct {
	AddPhysiDamage        int // obj+0x6e8
	AddMagicDamage        int // obj+0x6ec
	AddCurseDamage        int // obj+0x6f0
	AddAttackSuccessRate  int // obj+0x70c
	AddDefense            int // obj+0x718
	AddDefenseSuccessRate int // obj+0x71c
	DamageReduction       int // obj+0x744 (Mana Shield)
	DamageReflect         int // obj+0x748
	AddCriticalDamage     int // obj+0x750
	SoulExtra             int // obj+0x780 (Soul Potion value 2, not read here)
}

// effectState is the per-object effect list.
type effectState struct {
	Slots [effectSlots]effectSlot
	Opt   effectOptions
}

func newEffectState() *effectState {
	e := &effectState{}
	for i := range e.Slots {
		e.Slots[i].Index = -1
	}
	return e
}

// eff returns the effect state of o, creating it on first use.
func eff(o *Object) *effectState {
	if o.Eff == nil {
		o.Eff = newEffectState()
	}
	return o.Eff
}

// effOpt returns the effect options of o (zero when it has none).
func effOpt(o *Object) effectOptions {
	if o.Eff == nil {
		return effectOptions{}
	}
	return o.Eff.Opt
}

// loadEffectList reads the EffectList table of config/data/skill.toml
// (FUN_004786c0): indexes 0..0x7f; an item outside 0..0x1fff is none.
func loadEffectList(configDir string) (map[int]effectInfo, error) {
	f, err := gamedata.LoadSkillConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]effectInfo{}
	for i, r := range f.EffectList.Rows {
		e := effectInfo{Index: r.Index, Group: r.Group, Item: -1, Name: r.Name, Save: r.Save, Type: r.Type, Flag: r.Flag,
			Duration: r.Duration, Power: r.Power}
		if r.ItemIndex != "*" {
			cat, idx, err := gamedata.SplitItemRef(r.ItemIndex)
			if err != nil {
				return nil, fmt.Errorf("EffectList row %d: %w", i+1, err)
			}
			if it := cat*0x200 + idx; cat != -1 && it >= 0 && it < 0x2000 {
				e.Item = it
			}
		}
		if e.Index >= 0 && e.Index < 0x80 {
			out[e.Index] = e
		}
	}
	return out, nil
}

// effectStateBit is FUN_0047a9e0: the ViewSkillState bit of effects
// 1..70 (byte table at 0x47ab10 into the switch at 0x47aa9c); 0 = none.
func effectStateBit(index int) uint32 {
	if index < 1 || index > 70 {
		return 0
	}
	return effectBits[index-1]
}

var effectBits = func() [70]uint32 {
	// Bytes 0x47ab10..0x47ab55 read from the binary.
	sel := [70]byte{
		0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
		0x0a, 0x1c, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x0b,
		0x0a, 0x0b, 0x0c, 0x0d, 0x1c, 0x1c, 0x12, 0x13, 0x14, 0x14,
		0x14, 0x1c, 0x1c, 0x0c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c,
		0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x1c, 0x14,
		0x14, 0x14, 0x14, 0x14, 0x15, 0x16, 0x17, 0x18, 0x03, 0x01,
		0x19, 0x1c, 0x1a, 0x1b, 0x0d, 0x15, 0x16, 0x00, 0x01, 0x03,
	}
	cases := [...]uint32{4, 8, 0x2000000, 0x100, 0x800, 0x4000000, 0x400, 0x10, 0x2000, 0x8000,
		0x10000, 0x20000, 0x40000, 0x80000, 0x400000, 0x800000, 0x4000, 0x200, 0x10000000,
		0x1000000, 0x8000000, 1, 2, 0x20, 0x40, 0x1000, 0x200000, 0x80, 0}
	var t [70]uint32
	for i, s := range sel {
		t[i] = cases[s]
	}
	return t
}()

// packetEffectState is C1 0C 07 (FUN_0047ab60): [3] 1 = added / 0 =
// removed, [4..7] ViewSkillState bit (LE), [8..9] object BE.
func packetEffectState(add bool, bit uint32, index int) []byte {
	p := newPacket(frame.C1, 0x07, -1, 0x0C)
	if add {
		p[3] = 1
	}
	u32le(p[4:], bit)
	p[8], p[9] = byte(index>>8), byte(index)
	return p
}

// sendEffectState is the tail of FUN_0047ab60: to the object itself and,
// outside Chaos Castle (maps 18..23), to the players that see it.
func (w *World) sendEffectState(o *Object, add bool, index int) {
	bit := effectStateBit(index)
	if bit == 0 {
		return
	}
	p := packetEffectState(add, bit, o.Index)
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
func effectSlotOf(o *Object, index int) *effectSlot {
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
	newSlot := effectSlot{Index: index, Group: info.Group, Save: info.Save, Type: info.Type, Flag: info.Flag,
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
	*s = effectSlot{Index: -1}
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
		*s = effectSlot{Index: -1}
	}
}

// effectOn is the 97D/99z part of FUN_00478ef0: ViewSkillState bit and
// the option fields of the effect, then CalcCharacter.
func (w *World) effectOn(o *Object, s *effectSlot) { w.applyEffect(o, s, 1, true) }

// effectOff is FUN_004798d0 (the same fields subtracted).
func (w *World) effectOff(o *Object, s *effectSlot) { w.applyEffect(o, s, -1, true) }

func (w *World) applyEffect(o *Object, s *effectSlot, sign int, recalc bool) {
	bit := effectStateBit(s.Index)
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
	case effGreaterDamage, 0x5e:
		opt.AddPhysiDamage += v0
		opt.AddMagicDamage += v0
		opt.AddCurseDamage += v0
	case effGreaterDefense, 0x25, 0x2d, 0x5f, 0xc4:
		opt.AddDefense += v0
	case effElfBuffer, 0x5b:
		opt.AddPhysiDamage += v0
		opt.AddMagicDamage += v0
		opt.AddCurseDamage += v0
		opt.AddDefense += v1
	case effManaShield, 0x22:
		opt.DamageReduction += v0
	case effCriticalDamage:
		opt.AddCriticalDamage += v0
	case effGreaterLife:
		if se != nil {
			se.LifeRate[0] += v0
		}
	case effGreaterMana:
		if se != nil {
			se.ManaRate[0] += v0
		}
	case effSoulPotion:
		if se != nil {
			se.PhysiSpeed += v0
			se.MagicSpeed += v0
		}
		opt.SoulExtra += v1
	case effIce, 0x56, 0x8d:
		// obj+0x296 (slow walk, +300 ms per step) and obj+0x268.
		mv(o).Slow = sign > 0
	case effDamageReflect:
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
			*s = effectSlot{Index: -1}
		}
	}
}

// effectTickDamage is FUN_0047a6b0 for the 97D/99z damage-over-time
// effects (55 Poison and the Decay copy of it): every value[1] seconds
// the object loses life*value[2]/100 (capped by value[3] when set),
// dealt by value[0] and shown with damage effect 5 (type 2).
func (w *World) effectTickDamage(o *Object, s *effectSlot) {
	if s.Index != effPoison || s.Value[1] == 0 || s.Count%s.Value[1] != 0 {
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
func (w *World) effectTable() map[int]effectInfo {
	if w.skills == nil || w.skills.effects == nil {
		return map[int]effectInfo{}
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
