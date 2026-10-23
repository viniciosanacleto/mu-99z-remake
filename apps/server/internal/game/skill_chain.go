package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Chain Lightning, skill 215 (0xD7), FUN_004fdbd0, reached from the
// dispatcher FUN_004f9730 before its jump table (skill == 0xD7).
//
// SkillList.txt of this server has no row 215, so the magic list check
// (FUN_004f8cb0) refuses it before the dispatcher in practice; the code
// is reproduced for completeness.

// chainLightningSkill is the skill number of Chain Lightning.
const chainLightningSkill = 0xD7

// skillChainLightning is FUN_004fdbd0:
//  1. the target must be within Range of the caster (FUN_004f8130);
//  2. players pass the HackSkillSpeedCheck (FUN_004e8290 + FUN_00488550);
//  3. the chain: three slots start as the target; the caster's VP2 is
//     scanned in order and the first two other objects accepted by
//     FUN_004f8440 and within Radio of the *target* (FUN_004f8280) fill
//     slots 1 and 2;
//  4. slot i gets the delayed message 0x34 (FUN_00510690) after
//     200*(i+1) ms with hit number i+1, so FUN_0040b110 deals 100%, 70%
//     and 50% of the damage; with combo, C3 08 19 skill 0x3B goes to
//     each slot (FUN_00501570, CL = 1);
//  5. C1 10 BF 0A is sent to the caster (FUN_00504f00) and to its VP2
//     (FUN_00512730).
//
// It returns false (no mana is taken) when the target is invalid, out of
// range or the speed check fails.
func (w *World) skillChainLightning(o *Object, ref *skillRef, target int, combo bool) bool {
	t := w.targetObj(target)
	if t == nil {
		return false
	}
	if !w.rangeOK(o, ref.Num, o.X, o.Y, t.X, t.Y) {
		return false
	}
	if !w.skillSpeedAllowed(o, ref.Num) {
		return false
	}
	chain := [3]int{target, target, target}
	n := 1
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index == target {
			continue
		}
		if !w.skillTargetOK(o, e.Index, e.Type, target) {
			continue
		}
		c := w.objects[e.Index]
		if !w.radioOK(ref.Num, t.X, t.Y, c.X, c.Y) {
			continue
		}
		chain[n] = e.Index
		if n++; n > 2 {
			break
		}
	}
	for i, idx := range chain {
		w.queueSkillHit(o, 0x34, idx, 200*(i+1), ref.Num, i+1)
		if combo {
			w.sendSkillTo(o, 0x3b, idx, true)
		}
	}
	p := packetChainLightning(ref.Num, o.Index, chain)
	w.send(o, p)
	w.broadcast(o, p)
	return true
}

// chainLightningRate is the FUN_0040b110 factor of skill 215 by hit
// number (param_7): 70% for the second hit, 50% for the third, 100%
// otherwise.
func chainLightningRate(hit int) int {
	switch hit {
	case 2:
		return 70
	case 3:
		return 50
	}
	return 100
}

// packetChainLightning is C1 10 BF 0A (FUN_004fdbd0): [4] skill high,
// [5] skill low, [6..7] caster index u16 LE, [8] count (always 3), [9]
// left uninitialised by the original (0 here), then the three target
// indexes u16 LE.
func packetChainLightning(skill, caster int, chain [3]int) []byte {
	p := newPacket(frame.C1, 0xBF, 0x0A, 16)
	p[4], p[5] = byte(skill>>8), byte(skill)
	u16le(p[6:], caster)
	p[8] = 3
	for i, idx := range chain {
		u16le(p[10+2*i:], idx)
	}
	return p
}
