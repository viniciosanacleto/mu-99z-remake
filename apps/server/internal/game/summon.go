package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Summons (Elf summon skills): creation FUN_004fae50, the summon index
// range 0x5014..0x53FB (FUN_004b4ce0), removal FUN_004b5960, the owner
// assist of CAttack (FUN_004b58f0), the follow move FUN_004b6370, the
// "far from owner" teleport of FUN_004b4090 (FUN_0050e180), the life bar
// C1 05 F3 20 and the C2 1F viewport (FUN_00515890).

// summonSkillHook is called by the skills module when a player casts one
// of the summon skills (30..36, 432); true when a monster was summoned.
var summonSkillHook func(w *World, o *Object, skill int) bool

// summonRange is obj+0x376 of a summon (MoveRange 15): farther than this
// from its owner it teleports back.
const summonRange = 15

// summonClass returns the monster class of a summon skill
// (SummonMonster1..7 of Skill.dat, CServerInfo 0x17b0..0x17c8; skill 432
// reads 0x17cc whose key is not in the config dictionary).
func (w *World) summonClass(skill int) (int, bool) {
	switch {
	case skill >= 30 && skill <= 36:
		return w.cfg.Int("Skill", "SummonMonster"+string(rune('1'+skill-30)), 0), true
	case skill == 432:
		return w.cfg.Int("Skill", "SummonMonster8", 0), true
	}
	return 0, false
}

func packetSummonLife(pct int) []byte {
	p := newPacket(frame.C1, 0xF3, 0x20, 5)
	p[4] = byte(pct)
	return p
}

// summonOf returns the player's summon (obj+0x310).
func (w *World) summonOf(o *Object) *Object {
	s := w.summons()[o.Index]
	if s < SummonStart || s >= PlayerStart {
		return nil
	}
	m := w.objects[s]
	if m.Mon == nil || m.Mon.Owner != o.Index {
		return nil
	}
	return m
}

// summons maps a player index to its summon index (the player's obj+0x310).
func (w *World) summons() map[int]int {
	mw := w.mw()
	if mw.summonOf == nil {
		mw.summonOf = map[int]int{}
	}
	return mw.summonOf
}

// summonMonster is FUN_004fae50: not on Icarus (10) or Chaos Castle
// (18..23); an existing summon is dismissed instead (C1 05 F3 20 00).
func (w *World) summonMonster(o *Object, skill int) bool {
	class, ok := w.summonClass(skill)
	if !ok || o.Map == 10 || (o.Map >= 18 && o.Map <= 23) {
		return false
	}
	if w.summonOf(o) != nil {
		w.removeSummon(o)
		w.send(o, packetSummonLife(0))
		return false
	}
	idx := w.allocSummonIndex()
	if idx < 0 {
		return false
	}
	s := w.objects[idx]
	s.reset()
	s.State = StateConnected
	s.Map, s.X, s.Y, s.TX, s.TY, s.Dir = o.Map, o.X-1, o.Y+1, o.X-1, o.Y+1, 2
	if !w.setMonster(s, class) {
		w.deleteMonster(s)
		return false
	}
	m := s.Mon
	m.MTX, m.MTY = s.X, s.Y
	m.Owner = o.Index
	m.Attribute = 4
	m.Target = -1
	m.ActAttack = false
	m.MoveRange = summonRange
	w.summons()[o.Index] = idx
	if pct := summonLifePct(s); pct > 0 {
		w.send(o, packetSummonLife(pct))
	}
	return true
}

func summonLifePct(s *Object) int {
	if s.Mon.MaxLife <= 0 {
		return 0
	}
	return int(s.Mon.Life * 100 / s.Mon.MaxLife)
}

// allocSummonIndex is FUN_004b4ce0: round robin over 0x5014..0x53FB.
func (w *World) allocSummonIndex() int {
	mw := w.mw()
	if mw.nextSummon < SummonStart || mw.nextSummon >= PlayerStart {
		mw.nextSummon = SummonStart
	}
	p := mw.nextSummon
	for n := 0; n < PlayerStart-SummonStart; n++ {
		if w.objects[p].State == StateEmpty {
			mw.nextSummon = p + 1
			return p
		}
		p++
		if p >= PlayerStart {
			p = SummonStart
		}
	}
	return -1
}

// removeSummon is FUN_004b5960: the owner forgets the summon and it is
// deleted (FUN_0050c4a0).
func (w *World) removeSummon(o *Object) {
	s := w.summonOf(o)
	delete(w.summons(), o.Index)
	if s == nil {
		return
	}
	s.Mon.Owner = -1
	w.deleteMonster(s)
}

// summonAssist is FUN_004b58f0 (CAttack with a summoner): an idle summon
// takes the owner's target (emotion 1, count 30).
func (w *World) summonAssist(a, t *Object) {
	if a.Type != TypePlayer {
		return
	}
	s := w.summonOf(a)
	if s == nil || (s.Mon.Target >= 0 && s.Mon.Target < MaxObjects) {
		return
	}
	if t.State != StatePlaying || t == a || t == s {
		return
	}
	s.Mon.Target = t.Index
	s.Mon.Emotion, s.Mon.EmotionCount = emotionAttack, 30
}

// summonDamaged sends the owner the life bar after a hit on its summon
// (FUN_004cda80).
func (w *World) summonDamaged(t *Object, dmg int) {
	if t.Mon == nil || t.Mon.Owner < 0 || t.Mon.Owner >= MaxObjects || dmg <= 0 {
		return
	}
	if pct := summonLifePct(t); pct > 0 {
		w.send(w.objects[t.Mon.Owner], packetSummonLife(pct))
	}
}

// summonFollow is FUN_004b6370: walk next to the owner (AttackRange cells
// away on each axis, +2 for AttackType >= 100), or to the first free cell
// around it.
func (w *World) summonFollow(o *Object) {
	m := o.Mon
	if m.Owner < 0 || m.Owner >= MaxObjects {
		return
	}
	ow := w.objects[m.Owner]
	if ow.Teleporting {
		return
	}
	r := m.attackReach()
	tx, ty := ow.X, ow.Y
	if o.X < ow.X {
		tx -= r
	}
	if ow.X < o.X {
		tx += r
	}
	if o.Y < ow.Y {
		ty -= r
	}
	if ow.Y < o.Y {
		ty += r
	}
	a := w.attrAt(o.Map, tx, ty)
	if a&0x02 == 0 && a&0x04 == 0 && a&0x08 == 0 {
		m.MTX, m.MTY = tx, ty
		m.ActMove = true
		m.Delay = 1000
		return
	}
	d := dirTo(ow.X-o.X, ow.Y-o.Y)
	for i := 0; i < 8; i++ {
		x, y := ow.X+pathDirs[d][0], ow.Y+pathDirs[d][1]
		if c := w.attrAt(o.Map, x, y); c&(0x02|0x04|0x08) == 0 {
			m.MTX, m.MTY = x, y
			m.ActMove = true
			m.Delay = 1000
			return
		}
		d = (d + 1) % 8
	}
}

// summonKeepUp is the owner check of FUN_004b4090: a summon on another map
// or MoveRange cells away teleports to the owner (FUN_0050e180). Returns
// true when it moved (the AI step ends).
func (w *World) summonKeepUp(o *Object) bool {
	m := o.Mon
	if m.Attribute != 4 || m.Owner < 0 || m.Owner >= MaxObjects {
		return false
	}
	ow := w.objects[m.Owner]
	if ow.Map == o.Map && distance(o, ow) < m.MoveRange {
		return false
	}
	if teleportHook != nil {
		teleportHook(w, o, ow.Map, ow.X, ow.Y, o.Dir)
	} else {
		w.monsterOccupy(o, false)
		o.Map, o.X, o.Y, o.TX, o.TY = ow.Map, ow.X, ow.Y, ow.X, ow.Y
		w.monsterOccupy(o, true)
	}
	return true
}

// summonViewportEntry writes a 24-byte C2 1F entry (FUN_00515890): index BE
// (bit 7 new), class BE, view skill state, X Y TX TY, dir<<4|PK, owner name
// (10), 1 padding byte.
func (w *World) summonViewportEntry(dst []byte, t *Object) {
	dst[0], dst[1] = byte(t.Index>>8), byte(t.Index)
	if t.Live == LiveNew {
		dst[0] |= 0x80
	}
	dst[2], dst[3] = byte(t.Class>>8), byte(t.Class)
	u32le(dst[4:], t.ViewSkillState)
	dst[8], dst[9], dst[10], dst[11] = byte(t.X), byte(t.Y), byte(t.TX), byte(t.TY)
	dst[12] = byte(t.Dir)<<4 | byte(t.PKLevel)&0x0F
	if ow := w.objects[t.Mon.Owner]; ow.P != nil && ow.P.Char != nil {
		putName(dst[13:], ow.P.Char.Name)
	}
}

// sendSummonViewport is FUN_00515890: C2 size 1F count + 24-byte entries
// for the new VP entries of type 2/3 that are summons.
func (w *World) sendSummonViewport(o *Object) {
	var entries [][]byte
	for _, e := range o.VP {
		if e.State != VPNew || (e.Type != byte(TypeMonster) && e.Type != byte(TypeNPC)) ||
			e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		t := w.objects[e.Index]
		if t.Mon == nil || t.Mon.Owner < 0 || t.Mon.Owner >= MaxObjects {
			continue
		}
		b := make([]byte, 24)
		w.summonViewportEntry(b, t)
		entries = append(entries, b)
	}
	if len(entries) == 0 {
		return
	}
	p := newPacket(frame.C2, 0x1F, -1, 5+24*len(entries))
	p[4] = byte(len(entries))
	for i, e := range entries {
		copy(p[5+24*i:], e)
	}
	w.send(o, p)
}

func init() {
	summonSkillHook = func(w *World, o *Object, skill int) bool { return w.summonMonster(o, skill) }
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) {
		if w.mon != nil {
			w.removeSummon(o)
		}
	})
	// The owner's death removes the summon (FUN_004cda80 -> FUN_004b5960).
	deathHooks = append(deathHooks, func(w *World, killer, victim *Object) {
		if victim.Type == TypePlayer {
			w.removeSummon(victim)
		}
	})
}
