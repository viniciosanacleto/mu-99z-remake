package game

import (
	"encoding/binary"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/effect"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// Effects with Save = 1 (EffectList.txt) survive logout: the character
// save (GJSetCharacterInfo FUN_00475c30) packs the 16 CEffect slots into
// Character.EffectList with FUN_0047a3d0, 13 bytes each, and the
// character load (FUN_004cc730) re-adds them with AddEffect(obj, 1, ...)
// without packets (docs/server-plan/08c-skills-efeitos-pvp.md).
//
// Record: [0] effect index, [1..4] count (seconds left; for Type 2 the
// Unix end time, CEffect+0xc) big-endian, [5..12] value[0..3] as u16
// big-endian; a free slot, or one without Save, is 13 x 0xFF.

const effectRecordSize = 13

// encodeEffectList is the FUN_0047a3d0 loop of FUN_00475c30.
func encodeEffectList(o *Object) []byte {
	b := make([]byte, store.EffectListSize)
	for i := range b {
		b[i] = 0xFF
	}
	if o.Eff == nil {
		return b
	}
	for i, s := range o.Eff.Slots {
		r := b[i*effectRecordSize : (i+1)*effectRecordSize]
		if s.Index < 0 || s.Index == 0xFF || s.Save == 0 {
			continue
		}
		r[0] = byte(s.Index)
		n := uint32(s.Count)
		if s.Type == 2 {
			n = uint32(s.Expire)
		}
		binary.BigEndian.PutUint32(r[1:5], n)
		for k, v := range s.Value {
			binary.BigEndian.PutUint16(r[5+2*k:], uint16(v))
		}
	}
	return b
}

// effect.LoadList is the effect loop of FUN_004cc730: every record whose
// index is not 0xFF goes through AddEffect with param_2 = 1 (the saved
// duration and values; EffectList.txt powers still override) and no
// packet. The stored list is then normalised to what was accepted.
func (w *World) loadEffectList(o *Object) {
	c := o.P.Char
	for i := 0; i < effect.Slots && (i+1)*effectRecordSize <= len(c.EffectList); i++ {
		r := c.EffectList[i*effectRecordSize : (i+1)*effectRecordSize]
		if r[0] == 0xFF {
			continue
		}
		dur := int(int32(binary.BigEndian.Uint32(r[1:5])))
		var v [4]int
		for k := range v {
			v[k] = int(binary.BigEndian.Uint16(r[5+2*k:]))
		}
		w.addEffectEx(o, int(r[0]), dur, v, false, true)
	}
	c.EffectList = encodeEffectList(o)
}

func registerEffectSave(h *hooks) {
	h.characterLoad = append(h.characterLoad, (*World).loadEffectList)
	h.characterSave = append(h.characterSave, func(w *World, o *Object, c *store.Character) {
		if o.Eff != nil {
			c.EffectList = encodeEffectList(o)
		}
	})
	// The effects are dropped when the character leaves the map, before
	// the final save: keep their records first (prepended, so it runs
	// before effect.go's dropEffects).
	h.leaveWorld = append([]func(*World, *Object){func(w *World, o *Object) {
		if o.P != nil && o.P.Char != nil && o.Eff != nil {
			o.P.Char.EffectList = encodeEffectList(o)
		}
	}}, h.leaveWorld...)
}
