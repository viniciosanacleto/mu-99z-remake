package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Experience and level up: FUN_004c6670 (LevelUp), FUN_004c5ea0 (party
// experience) and the packets C1 30 F3 05 (FUN_004db550) and C1 06 48.

// levelUpSaveInterval is the 60000 ms of FUN_004c6670 between the saves
// a level up triggers (obj+0x5dc).
const levelUpSaveInterval = 60 * time.Second

func registerLevelup(h *hooks) {
	h.gainExperience = func(w *World, o *Object, exp int64) {
		w.levelUp(o, exp, w.cfg.Int("Common", "MaxLevelUp", 0))
	}
	h.partyExperience = func(w *World, killer, monster *Object, damage int, flag bool) bool {
		return w.partyExperience(killer, monster, damage, flag)
	}
}

// levelUpPoints is <Class>LevelUpPoint_AL<n> (CServerInfo 0x3e0 + AL*4 +
// class*16).
func (w *World) levelUpPoints(o *Object) int {
	if o.Class < 0 || o.Class > 4 {
		return 0
	}
	return w.cfg.ALInt("Common", classPrefix[o.Class]+"LevelUpPoint", accountLevelOf(o), 0)
}

func accountLevelOf(o *Object) int {
	if o.P == nil {
		return 0
	}
	return min(max(o.P.Account.Level, 0), 3)
}

// levelUp is FUN_004c6670: add exp, raising at most maxLevelUp levels.
// It returns false when the experience was only added (no level change)
// and true after a level up or when the character is at MaxLevel (message
// 0x10a). Experience uses the unsigned 32-bit arithmetic of obj+0xf0.
func (w *World) levelUp(o *Object, exp int64, maxLevelUp int) bool {
	if o.P == nil || o.P.Char == nil {
		return false
	}
	p, c := o.P, o.P.Char
	maxLevel := w.cfg.Int("Common", "MaxLevel", 0)
	if c.Level >= maxLevel {
		w.systemMessage(o, w.messageFor(o, 0x10a))
		return true
	}
	add := uint32(exp)
	cur := uint32(c.Experience)
	if cur+add < p.NextExp {
		c.Experience = int64(cur + add)
		return false
	}
	for {
		c.Level++
		c.LevelUpPoints += w.levelUpPoints(o)
		if c.Level > w.cfg.Int("Common", "PlusStatMinLevel", 0) && len(c.Quest) > 0 && c.Quest[0]&0x30 == 0x20 {
			c.LevelUpPoints += w.cfg.Int("Common", "PlusStatPoint", 0)
		}
		maxLevelUp--
		take := add
		if maxLevelUp != 0 {
			take = p.NextExp - cur
		}
		add -= take
		cur = p.NextExp
		c.Experience = int64(cur)
		w.expBounds(o)
		cur = uint32(c.Experience)
		if c.Level >= maxLevel {
			break
		}
		if !(p.NextExp <= cur+add) {
			cur += add
			c.Experience = int64(cur)
			break
		}
	}
	w.calcCharacter(o)
	s := w.statsOf(o)
	c.BP = float32(s.MaxBP + s.AddBP)
	c.Life = float32(p.AddLife) + c.MaxLife
	c.Shield = float32(s.AddShield) + c.MaxShield
	c.Mana = float32(p.AddMana) + c.MaxMana
	w.send(o, w.packetLevelUp(o))
	w.broadcast(o, packetLevelUpEffect(o.Index))
	now := w.now()
	if now.Sub(s.lastLevelSave) > levelUpSaveInterval {
		s.lastLevelSave = now
		w.saveCharacter(o, nil)
	}
	return true
}

// expBounds is FUN_0050be30: Experience >= exp[level-1], NextExp = exp[level].
func (w *World) expBounds(o *Object) {
	c := o.P.Char
	if floor := w.data.Experience.Floor(c.Level); uint32(c.Experience) < floor {
		c.Experience = int64(floor)
	}
	o.P.NextExp = w.data.Experience.Next(c.Level)
}

// packetLevelUp is C1 30 F3 05 (FUN_004db550): level, points, maximums
// (u16 clamped to 65000, then u32), fruits, experience.
func (w *World) packetLevelUp(o *Object) []byte {
	p, c, s := o.P, o.P.Char, w.statsOf(o)
	b := newPacket(frame.C1, 0xF3, 0x05, 0x30)
	maxLife := float64(float32(p.AddLife) + c.MaxLife)
	maxMana := float64(float32(p.AddMana) + c.MaxMana)
	clampF := func(f float64) int {
		if f <= 65000 {
			return int(math.RoundToEven(f))
		}
		return 65000
	}
	u16le(b[4:], c.Level)
	u16le(b[6:], c.LevelUpPoints)
	u16le(b[8:], clampF(maxLife))
	u16le(b[10:], clampF(maxMana))
	u16le(b[12:], clamp65000(s.MaxBP+s.AddBP))
	u16le(b[14:], c.FruitAddPoints)
	u16le(b[16:], maxFruit(o.Class, c.Level))
	u16le(b[18:], c.FruitSubPoints)
	u16le(b[20:], maxFruit(o.Class, c.Level))
	u32le(b[24:], uint32(int32(c.LevelUpPoints)))
	u32le(b[28:], uint32(int64(math.RoundToEven(maxLife))))
	u32le(b[32:], uint32(int64(math.RoundToEven(maxMana))))
	u32le(b[36:], uint32(int32(s.MaxBP+s.AddBP)))
	u32le(b[40:], uint32(c.Experience))
	u32le(b[44:], p.NextExp)
	return b
}

// packetLevelUpEffect is C1 06 48 index(BE) 0x10, sent to the viewport.
func packetLevelUpEffect(index int) []byte {
	b := newPacket(frame.C1, 0x48, -1, 6)
	b[3], b[4], b[5] = byte(index>>8), byte(index), 0x10
	return b
}

// packetPartyKillExp is C3 18 9C (FUN_004dab00) with the flag of bit 7 of
// byte 3.
func packetPartyKillExp(monster int, flag bool, exp uint32, damage int, cur, next uint32) []byte {
	b := newPacket(frame.C3, 0x9C, -1, 0x18)
	b[3], b[4] = byte(monster>>8), byte(monster)
	if flag {
		b[3] |= 0x80
	}
	u16le(b[6:], int(exp>>16))
	u16le(b[8:], int(exp&0xFFFF))
	d := damage
	if d >= 0xFDE9 {
		b[10], b[11] = 0xFD, 0xE8
	} else {
		b[10], b[11] = byte(d>>8), byte(d)
	}
	u32le(b[16:], cur)
	u32le(b[20:], next)
	return b
}

// monsterExpLevel returns the level and maximum life of a monster (from
// the monsters module).
func monsterExpLevel(m *Object) (int, float32) {
	if m.Mon == nil {
		return 0, 0
	}
	return m.Mon.Level, m.Mon.MaxLife
}

// partyExpRate is Party<General|Special>Experience<count>.
func (w *World) partyExpRate(special bool, count int) int {
	name := "PartyGeneralExperience"
	if special {
		name = "PartySpecialExperience"
	}
	return w.cfg.Int("Common", name+string(rune('0'+count)), 0)
}

// partyExperience is FUN_004c5ea0, called when the killer of a monster is
// in a party. It returns false when the killer has no party (the caller
// then uses the single-player distribution FUN_004c5970).
func (w *World) partyExperience(killer, monster *Object, damage int, flag bool) bool {
	if killer.P == nil || monster.Type != TypeMonster {
		return false
	}
	pt := w.partyOf(killer)
	if pt == nil {
		return false
	}
	mlvl, _ := monsterExpLevel(monster)
	near := func(idx int) *Object {
		if idx < 0 || idx >= MaxObjects {
			return nil
		}
		m := w.objects[idx]
		if m.P == nil || m.P.Char == nil || m.Map != monster.Map || objDistance(m, monster) >= 10 {
			return nil
		}
		return m
	}
	maxLevel := 0
	for _, idx := range pt.members {
		if m := near(idx); m != nil && m.P.Char.Level > maxLevel {
			maxLevel = m.P.Char.Level
		}
	}
	totalLevel, count, classes, distinct := 0, 0, 0, 0
	for _, idx := range pt.members {
		m := near(idx)
		if m == nil {
			continue
		}
		lv := m.P.Char.Level
		if lv+200 < maxLevel {
			lv += 200
		}
		totalLevel += lv
		if bit := 1 << uint(m.Class&31); classes&bit == 0 {
			classes |= bit
			distinct++
		}
		count++
	}
	if count == 0 {
		return true
	}
	rate := w.partyExpRate(distinct >= 3, count) * count
	avg := totalLevel / count
	base := (mlvl + 25) * mlvl / 3
	if mlvl+10 < avg {
		base = (mlvl + 10) * base / avg
	}
	if mlvl >= 65 {
		if count == 1 {
			base += (mlvl - 64) * (mlvl / 4)
		} else {
			base += 200 - killer.P.Char.Level/5
		}
	}
	base = max(base, 0)
	exp := base/4 + base
	if monster.Mon != nil {
		monster.Mon.ExpGiven = 0
	}
	for _, idx := range pt.members {
		m := near(idx)
		if m == nil {
			continue
		}
		t := uint32(m.P.Char.Level*exp*rate) / uint32(totalLevel) / 100
		s := w.statsOf(m)
		mrate := int64((count-1)*s.Effects.PartyExpRate + s.ExperienceRate + s.Effects.ExpRate)
		v := uint64(uint32(int32(t) * int32(w.cfg.ALInt("Common", "AddExperienceRate", accountLevelOf(m), 0))))
		v = uint64(int64(v)*mrate) / 100
		v = v * uint64(w.mapExpRate(m.Map)) / 100
		v = v * uint64(w.bonusValue(m, BonusExperience, 100)) / 100 // FUN_004188f0
		v = v * uint64(gamedata.CustomMonsterExperienceRate(w.statsData.customMons, monster.Class, monster.Map)) / 100
		v = v * uint64(w.expTableRate(m)) / 100
		v = v * uint64(w.customAttackExpRate(m)) / 100 // CustomAttack rate (customattack.go)
		if monster.Mon != nil {
			monster.Mon.ExpGiven += int64(v / uint64(count))
		}
		if w.hooks.petExperience != nil {
			w.hooks.petExperience(w, m, int64(int32(v/uint64(count)))) // FUN_004c69d0 (EBX = share)
		}
		leveled := w.levelUp(m, int64(v), w.cfg.Int("Common", "MaxLevelUp", 0))
		shown := uint32(v)
		if leveled {
			shown = 0
		}
		c := m.P.Char
		w.send(m, packetPartyKillExp(monster.Index, flag, shown, damage, uint32(c.Experience), m.P.NextExp))
	}
	return true
}

// mapExpRate is the MapManager ExperienceRate (100 without a row).
func (w *World) mapExpRate(mp int) int {
	if info, ok := w.data.Maps[mp]; ok {
		return info.ExperienceRate
	}
	return 100
}

// expTableRate is FUN_0047c040 on Util/ExperienceTable.txt (level obj+0xe6,
// resets obj+0x6dc, master resets obj+0x6e0); the table is empty in this
// installation, so it gives 100.
func (w *World) expTableRate(o *Object) int {
	if o.P == nil || o.P.Char == nil {
		return 100
	}
	c := o.P.Char
	return w.expTable.Rate(c.Level, c.Resets, c.MasterResets)
}
