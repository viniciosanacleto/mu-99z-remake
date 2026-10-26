package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Combat: the D7 attack (CAttack FUN_00410a30), the damage pipeline
// (FUN_0040b110 with FUN_0040e1c0 hit check, FUN_0040eaa0 defense,
// FUN_0040ec10 damage), life check and death (FUN_004cda80), monster
// kill experience (FUN_004c5970/FUN_004c5a30), drops (FUN_004b7290) and
// the player death/respawn (FUN_004c37f0/FUN_004c3b10).

// Damage effect values (the damage packet [7]).
const (
	effectIgnoreDefense = 1
	effectExcellent     = 2
	effectCritical      = 3
	effectDouble        = 0x40
	effectCombo         = 0x80
	effectTriple        = 0x100
)

var classKeys = [5]string{"DW", "DK", "FE", "MG", "DL"}

// --- packets -------------------------------------------------------------

// packetDamage is C1 10 D7 (FUN_004da0e0): [3..4] target BE (bit 7 of [3]
// = flag), [5..6] damage BE capped at 65000, [7] effect (low byte),
// [8..11] target life LE (rounded), [12..15] damage LE.
func packetDamage(target, damage int, effect int, life float32, flag bool) []byte {
	p := newPacket(frame.C1, 0xD7, -1, 0x10)
	p[3], p[4] = byte(target>>8), byte(target)
	if flag {
		p[3] |= 0x80
	}
	d := damage
	if d >= 0xFDE9 {
		d = 0xFDE8
	}
	p[5], p[6] = byte(d>>8), byte(d)
	p[7] = byte(effect)
	u32le(p[8:], uint32(int32(math.RoundToEven(float64(life)))))
	u32le(p[12:], uint32(int32(damage)))
	return p
}

// packetDie is C1 08 17 (FUN_004da270): [3..4] dead object BE, [5] skill,
// [6..7] killer BE.
func packetDie(target, skill, killer int) []byte {
	p := newPacket(frame.C1, 0x17, -1, 8)
	p[3], p[4] = byte(target>>8), byte(target)
	p[5] = byte(skill)
	p[6], p[7] = byte(killer>>8), byte(killer)
	return p
}

// packetKillExp is C3 18 9C (FUN_004dab00): [3..4] monster BE (bit 7 of
// [3] = flag), [6..7] experience high word LE, [8..9] low word LE,
// [10..11] damage BE capped at 65000, [16..19] experience, [20..23] next
// experience of the player.
func packetKillExp(monster int, exp uint32, damage int, cur, next uint32) []byte {
	p := newPacket(frame.C3, 0x9C, -1, 0x18)
	p[3], p[4] = byte(monster>>8), byte(monster)
	u16le(p[6:], int(exp>>16))
	u16le(p[8:], int(exp&0xFFFF))
	d := damage
	if d >= 0xFDE9 {
		d = 0xFDE8
	}
	p[10], p[11] = byte(d>>8), byte(d)
	u32le(p[16:], cur)
	u32le(p[20:], next)
	return p
}

// packetSkillUse is C3 08 19 (FUN_00501570): [3] skill, [4..5] user BE,
// [6..7] target BE with bit 7 of [6] = success.
func packetSkillUse(skill, user, target int, success bool) []byte {
	p := newPacket(frame.C3, 0x19, -1, 8)
	p[3] = byte(skill)
	p[4], p[5] = byte(user>>8), byte(user)
	p[6], p[7] = byte(target>>8), byte(target)
	if success {
		p[6] |= 0x80
	}
	return p
}

// packetAttackAction is C1 09 18 (FUN_004da300): [3..4] attacker BE,
// [5] dir, [6] action, [7..8] target BE.
func packetAttackAction(attacker, dir, action, target int) []byte {
	p := newPacket(frame.C1, 0x18, -1, 9)
	p[3], p[4] = byte(attacker>>8), byte(attacker)
	p[5], p[6] = byte(dir), byte(action)
	p[7], p[8] = byte(target>>8), byte(target)
	return p
}

// packetRespawn is C3 24 F3 04 (FUN_004db350): X Y map dir, life/mana/BP
// u16 (capped 65000), [16] experience, [20] money, [24] life, [28] mana,
// [32] BP (u32 LE).
func packetRespawn(x, y, mp, dir int, life, mana float32, bp int, exp, money uint32) []byte {
	p := newPacket(frame.C3, 0xF3, 0x04, 0x24)
	p[4], p[5], p[6], p[7] = byte(x), byte(y), byte(mp), byte(dir)
	cap16 := func(f float32) int {
		if f > 65000 {
			return 65000
		}
		return int(math.RoundToEven(float64(f)))
	}
	u16le(p[8:], cap16(life))
	u16le(p[10:], cap16(mana))
	u16le(p[12:], clamp65000(bp))
	u32le(p[16:], exp)
	u32le(p[20:], money)
	u32le(p[24:], uint32(int32(math.RoundToEven(float64(life)))))
	u32le(p[28:], uint32(int32(math.RoundToEven(float64(mana)))))
	u32le(p[32:], uint32(bp))
	return p
}

// --- object accessors -----------------------------------------------------

func objLevel(o *Object) int {
	if o.Mon != nil {
		return o.Mon.Level
	}
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Level
	}
	return 0
}

func objLife(o *Object) float32 {
	if o.Mon != nil {
		return o.Mon.Life
	}
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Life
	}
	return 0
}

func setObjLife(o *Object, v float32) {
	if o.Mon != nil {
		o.Mon.Life = v
	} else if o.P != nil && o.P.Char != nil {
		o.P.Char.Life = v
	}
}

func accountLevel(o *Object) int {
	if o.P != nil {
		return min(max(o.P.Account.Level, 0), 3)
	}
	return 0
}

func resets(o *Object) int {
	if o.P != nil && o.P.Char != nil {
		return o.P.Char.Resets
	}
	return 0
}

// combatStats returns the CAttack inputs of an object: the MonsterList
// values for monsters, combatStatsHook for players.
func (w *World) combatStats(o *Object) CombatStats {
	if m := o.Mon; m != nil {
		return CombatStats{DamageMinRight: m.DamageMin, DamageMaxRight: m.DamageMax,
			DamageMinLeft: m.DamageMin, DamageMaxLeft: m.DamageMax,
			Defense: m.Defense, MagicDefense: m.MagicDefense,
			AttackSuccessRate: m.AttackRate, DefenseSuccessRate: m.DefenseRate, AttackSpeed: m.AttackSpeed}
	}
	if combatStatsHook != nil {
		return combatStatsHook(w, o)
	}
	return CombatStats{}
}

// --- D7 -------------------------------------------------------------------

// handleAttack is D7 (FUN_00410a30): [3..4] target BE, [5] action, [6] dir.
func (w *World) handleAttack(o *Object, p []byte) {
	if len(p) < 7 {
		return
	}
	w.attackRequest(o, p)
}

func (w *World) attackRequest(o *Object, p []byte) {
	if o.State != StatePlaying {
		return
	}
	target := int(p[3])<<8 | int(p[4])
	if target >= MaxObjects {
		return
	}
	t := w.objects[target]
	if !t.Alive || o.Map != t.Map {
		return
	}
	if w.cellHas(o, gamedata.AttrSafeZone) || w.cellHas(t, gamedata.AttrSafeZone) {
		return
	}
	if o.Type == TypePlayer {
		dx, dy := float32(o.X-t.X), float32(o.Y-t.Y)
		reach := float32(3)
		if o.Class == 2 {
			reach = 6
		}
		if reach < float32(math.Sqrt(float64(dx*dx+dy*dy))) {
			return
		}
	}
	o.Dir = int(p[6])
	w.broadcast(o, packetAttackAction(o.Index, o.Dir, int(p[5]), target))
	w.attack(o, t, 0, false)
}

// monsterAttack is FUN_004b7d00 for ordinary monsters: AttackType 0 or
// >= 100 builds "C1 07 D7 target 0x78 dir" and runs the D7 handler;
// AttackType 1..99 casts its skill (slot 0) through the 0x19 path.
func (w *World) monsterAttack(o, t *Object) {
	m := o.Mon
	if m.AttackType == 0 || m.AttackType > 99 {
		w.attackRequest(o, []byte{0xC1, 0x07, 0xD7, byte(t.Index >> 8), byte(t.Index), 0x78, byte(o.Dir)})
		return
	}
	if w.cellHas(o, gamedata.AttrSafeZone) || w.cellHas(t, gamedata.AttrSafeZone) || !t.Alive {
		return
	}
	w.attack(o, t, m.AttackType, true)
}

// --- CAttack::Attack -------------------------------------------------------

// attack (FUN_0040b110) lives in pvp.go (attackFull).

// hitCheck is FUN_0040e1c0 (the buff/option percentages are 0 here):
// when the attack rate is below the target's defense rate the hit lands
// 5% of the time with 30% damage; otherwise it misses when
// rand % attackRate < defenseRate.
func hitCheck(a, t CombatStats) (reduced, hit bool) {
	ar := max(a.AttackSuccessRate, 0)
	dr := max(t.DefenseSuccessRate, 0)
	if ar < dr {
		return true, rnd(100) <= 4
	}
	if ar == 0 {
		ar = 1
	}
	return false, rnd(ar) >= dr
}

// targetDefense is FUN_0040eaa0: Defense, halved for player targets,
// ignored (effect 1) with IgnoreDefenseRate.
func targetDefense(a, t *Object, ts, as CombatStats, effect *int) int {
	def := ts.Defense
	if t.Type == TypePlayer {
		def = def * 50 / 100
	}
	if rnd(100) < as.IgnoreDefenseRate {
		*effect = effectIgnoreDefense
		def = 0
	}
	return max(def, 0)
}

// attackDamage is FUN_0040ec10 for normal attacks: monsters roll
// DamageMin + rand%(DamageMax-DamageMin); players use the right or left
// hand range (two weapons add both), then the critical (max damage +
// bonus) and excellent (max*120/100 + bonus) rolls. Defense is subtracted
// and the result clamped at 0.
func attackDamage(a, t *Object, as, ts CombatStats, effect *int, def int) int {
	var dmg int
	if a.Type != TypePlayer {
		lo, hi := as.DamageMinRight, as.DamageMaxRight
		rng := max(hi-lo, 1)
		dmg = lo + rnd(rng)
	} else {
		lo, hi := as.DamageMinLeft, as.DamageMaxLeft
		if useRightHand(a) {
			lo, hi = as.DamageMinRight, as.DamageMaxRight
		}
		if twoWeapons(a) {
			lo, hi = as.DamageMinRight+as.DamageMinLeft, as.DamageMaxRight+as.DamageMaxLeft
		}
		rng := max(hi-lo, 1)
		dmg = rnd(rng) + lo
		if rnd(100) < as.CriticalDamage {
			*effect = effectCritical
			dmg = as.CriticalDamageBonus + hi
		}
		if rnd(100) < as.ExcellentDamage {
			*effect = effectExcellent
			dmg = hi*120/100 + as.ExcellentDamageBonus
		}
	}
	return max(dmg-def, 0)
}

// useRightHand: slot 0 holds a weapon of groups 0..3 or 5, or a bow /
// crossbow (group 4) other than arrows (0x80F) (FUN_0040ec10).
func useRightHand(o *Object) bool {
	if o.P == nil {
		return true
	}
	it := o.P.Inventory[0]
	if it.IsEmpty() {
		return false
	}
	idx := it.Index
	return (idx >= 0 && idx < 0x800) || (idx >= 0xA00 && idx < 0xC00) ||
		(idx >= 0x800 && idx < 0xA00 && idx != 0x80F)
}

// twoWeapons: DK, MG, DL (and class 6) with weapons (index < 0x800) in
// both hands (the obj+0xb5/0x195 flags of the original are taken as set).
func twoWeapons(o *Object) bool {
	if o.P == nil {
		return false
	}
	switch o.Class {
	case 1, 3, 4, 6:
	default:
		return false
	}
	r, l := o.P.Inventory[0], o.P.Inventory[1]
	return !r.IsEmpty() && !l.IsEmpty() && r.Index < 0x800 && l.Index < 0x800
}

// sendDamage is FUN_004da0e0: to the attacker and to the target when they
// are players.
func (w *World) sendDamage(a, t *Object, dmg, effect int, flag bool) {
	p := packetDamage(t.Index, dmg, effect, objLife(t), flag)
	if a.Type == TypePlayer {
		w.send(a, p)
	}
	if t.Type == TypePlayer {
		w.send(t, p)
	}
}

func (w *World) sendSkillUse(a, t *Object, skill int, success bool) {
	if !w.skillSpeedAllowed(a, skill) { // FUN_00501570 -> FUN_00488550 (skillspeed.go)
		return
	}
	p := packetSkillUse(skill, a.Index, t.Index, success)
	if a.Type == TypePlayer {
		w.send(a, p)
	}
	w.broadcast(a, p)
}

// lifeCheck is FUN_004cda80.
func (w *World) lifeCheck(a, t *Object, dmg, effect, skill int) {
	w.lifeCheckEx(a, t, dmg, effect, skill, false)
}

// objectDeathHook runs when t dies in lifeCheck, before the die packet
// (pvp.go: FUN_0050e8a0 death penalty, PK, duel, effects).
var objectDeathHook func(w *World, killer, t *Object)

// lifeCheckEx is FUN_004cda80 with the damage packet flag (param_5).
func (w *World) lifeCheckEx(a, t *Object, dmg, effect, skill int, flag bool) {
	killer := a
	if a.Mon != nil && a.Mon.Owner >= 0 && a.Mon.Owner < MaxObjects {
		killer = w.objects[a.Mon.Owner]
	}
	if killer.Type == TypePlayer && t.Type == TypeMonster && dmg > 0 {
		w.addHit(t, killer.Index, dmg)
	}
	w.summonDamaged(t, dmg) // owner life bar C1 05 F3 20
	if objLife(t) > 0 || !t.Alive {
		w.sendDamage(a, t, dmg, effect, flag)
		return
	}
	// Death.
	if objectDeathHook != nil {
		objectDeathHook(w, killer, t)
	}
	if t.Type == TypeMonster {
		w.sendMsg(t, msgDie, killer.Index, 500, 0)
	}
	t.Alive = false
	t.Live = LiveDying
	if t.Move != nil {
		t.Move.clearPath()
	}
	t.Teleporting = false
	now := w.now()
	if t.Mon != nil {
		t.Mon.Die, t.Mon.DieTime = 1, now
	} else {
		d := &deadState{Die: 1, Time: now, KillerID: killer.Index}
		if killer.Type == TypeMonster {
			d.Respawn = 1
		}
		w.mw().deaths[t.Index] = d
	}
	p := packetDie(t.Index, skill, killer.Index)
	if t.Type == TypePlayer {
		w.send(t, p)
	}
	w.broadcast(t, p)
	if t.Type == TypeMonster && killer.Type == TypePlayer && !noExpMonster(t) {
		// FUN_004cda80: party distribution (FUN_004c5ea0) when the killer
		// is in a party, otherwise FUN_004c5970.
		if partyExperienceHook == nil || !partyExperienceHook(w, killer, t, dmg, false) {
			w.monsterKillExperience(t, killer)
		}
	}
	for _, h := range deathHooks {
		h(w, killer, t)
	}
}

// noExpMonster: summons and the classes FUN_004cda80 gives no experience
// for.
func noExpMonster(t *Object) bool {
	if t.Mon != nil && t.Mon.Owner >= 0 && t.Mon.Owner < MaxObjects {
		return true
	}
	switch t.Class {
	case 0x113, 0x115, 0x116, 0x11B, 0x11E, 0x11F, 0x120, 0x127, 0x154, 0x15C, 0x15D,
		0x169, 0x16A, 0x16B, 0x20C, 0x20D, 0x20F, 0x210, 0x25D, 0x25E, 0x25F:
		return true
	}
	return false
}

// addHit is FUN_004b6b80: accumulate the damage of a player (hits older
// than MonsterGetTopHitDamageUserMaxTime seconds restart the sum).
func (w *World) addHit(t *Object, player, dmg int) {
	m := t.Mon
	now := w.now()
	maxAge := time.Duration(w.cfg.Int("Common", "MonsterGetTopHitDamageUserMaxTime", 0)) * time.Second
	for i := range m.Hits {
		h := &m.Hits[i]
		if h.Index == player && h.Time != (time.Time{}) {
			if now.Sub(h.Time) <= maxAge {
				h.Damage += dmg
			} else {
				h.Damage = dmg
			}
			h.Time = now
			return
		}
	}
	slot := -1
	for i := range m.Hits {
		if m.Hits[i].Time == (time.Time{}) {
			slot = i
			break
		}
	}
	if slot < 0 {
		slot = w.cleanHits(t)
	}
	if slot >= 0 && slot < len(m.Hits) {
		m.Hits[slot] = damageEntry{Index: player, Damage: dmg, Time: now}
	}
}

// cleanHits is FUN_004b6cc0: drop entries of players that left, changed
// map, are 21+ cells away or hit too long ago; returns the last freed
// slot (or -1).
func (w *World) cleanHits(t *Object) int {
	m := t.Mon
	now := w.now()
	maxAge := time.Duration(w.cfg.Int("Common", "MonsterGetTopHitDamageUserMaxTime", 0)) * time.Second
	free := -1
	for i := range m.Hits {
		h := m.Hits[i]
		if h.Time == (time.Time{}) {
			continue
		}
		p := w.objects[h.Index]
		if p.State == StatePlaying && now.Sub(h.Time) <= maxAge && p.Map == t.Map && distance(t, p) < 0x15 {
			continue
		}
		m.Hits[i] = damageEntry{}
		free = i
	}
	return free
}

// topHitter is FUN_004b6df0: the valid player with the largest damage.
func (w *World) topHitter(t *Object) int {
	m := t.Mon
	now := w.now()
	maxAge := time.Duration(w.cfg.Int("Common", "MonsterGetTopHitDamageUserMaxTime", 0)) * time.Second
	best, bestDmg := -1, 0
	for _, h := range m.Hits {
		if h.Time == (time.Time{}) {
			continue
		}
		p := w.objects[h.Index]
		if p.State != StatePlaying || now.Sub(h.Time) > maxAge || p.Map != t.Map || distance(t, p) >= 0x15 {
			continue
		}
		if bestDmg < h.Damage {
			best, bestDmg = h.Index, h.Damage
		}
	}
	return best
}

// monsterKillExperience is FUN_004c5970: every player in the hit list gets
// FUN_004c5a30 experience for its share of the damage (party sharing is
// not implemented).
func (w *World) monsterKillExperience(t, killer *Object) {
	w.cleanHits(t)
	t.Mon.ExpGiven = 0
	for _, h := range t.Mon.Hits {
		if h.Time == (time.Time{}) || h.Damage == 0 {
			continue
		}
		p := w.objects[h.Index]
		if p.State != StatePlaying || p.P == nil || p.P.Char == nil {
			continue
		}
		exp := w.killExperience(p, t, h.Damage)
		t.Mon.ExpGiven += exp
		ch := p.P.Char
		level := ch.Level
		if petExperienceHook != nil {
			petExperienceHook(w, p, exp) // FUN_004c69d0
		}
		if gainExperienceHook != nil {
			gainExperienceHook(w, p, exp)
		}
		shown := uint32(exp)
		if ch.Level != level {
			shown = 0 // FUN_004c6670 leveled up: 9C shows 0
		}
		w.send(p, packetKillExp(t.Index, shown, h.Damage, uint32(ch.Experience), p.P.NextExp))
	}
}

// killExperience is FUN_004c5a30 for one player.
func (w *World) killExperience(p, t *Object, dmg int) int64 {
	ml := t.Mon.Level
	pl := objLevel(p)
	base := (ml + 25) * ml / 3
	if ml+10 < pl {
		base = (ml + 10) * base / pl
	}
	if ml > 64 {
		base += (ml - 64) * (ml / 4)
	}
	base = max(base, 0)
	maxLife := int(t.Mon.MaxLife)
	if float32(dmg) > t.Mon.MaxLife {
		dmg = maxLife
	}
	exp0 := int64(base/4 + base)
	// obj+0x86c (100) + obj+0x75c (0): the player's experience rate.
	playerRate := int64(100)
	if maxLife < 1 {
		maxLife = 1
	}
	e := int64(dmg) * exp0 / int64(maxLife)
	e = e * playerRate
	e = e * int64(w.cfg.ALInt("Common", "AddExperienceRate", accountLevel(p), 100)) / 100
	mapRate := int64(100)
	if info, ok := w.data.Maps[t.Map]; ok {
		mapRate = int64(info.ExperienceRate)
	}
	e = e * mapRate / 100
	e = e * int64(w.bonusValue(p, BonusExperience, 100)) / 100
	// FUN_004188f0 (CBonusManager, bonus.go) above, then
	// FUN_004670a0 (CustomMonster ExperienceRate), FUN_0047c040
	// (ExperienceTable.txt) and the CustomAttack rate.
	if w.statsData != nil {
		e = e * int64(gamedata.CustomMonsterExperienceRate(w.statsData.customMons, t.Class, t.Map)) / 100
	}
	e = e * int64(w.expTableRate(p)) / 100        // datafiles.go
	e = e * int64(w.customAttackExpRate(p)) / 100 // customattack.go
	return e
}

// --- monster death: drops ---------------------------------------------------

// monsterDie is the msgDie handler (FUN_004b70b0 -> FUN_004b7290).
func (w *World) monsterDie(o *Object, killer int) {
	w.kundunDie(o, killer) // FUN_004a3150 (kalima.go)
	w.monsterDrop(o, killer)
	o.Mon.Delay = 500
	if ow := o.Mon.Owner; ow >= 0 && ow < MaxObjects { // summon: FUN_004b5960
		w.removeSummon(w.objects[ow])
	}
}

// noDropMonster lists the classes FUN_004b7290 never drops for.
func noDropMonster(class int) bool {
	switch class {
	case 0x84, 0x85, 0x86, 0xF7, 0xF9, 0x115, 0x116, 0x11B, 0x120, 0x1CC, 0x1CD, 0x1CE,
		0x20C, 0x20D, 0x20F, 0x210, 0x25D, 0x25E, 0x25F:
		return true
	}
	return false
}

// monsterDrop is FUN_004b7290, implemented in drop.go (monsterItemDrop:
// bags, quest items (questDropHook), ItemDrop/ItemMoneyDrop, set and
// excellent drops with their options, zen).
func (w *World) monsterDrop(o *Object, killer int) {
	w.monsterItemDrop(o, killer)
}

// buildDropTable is FUN_004ba760: for every monster level 0..149 the
// droppable items (DropItem != 0) whose Level L satisfies
// L-2 <= level <= L+4, at most 100 per level, by ascending index.
func buildDropTable(items map[int]gamedata.ItemInfo) [150][]int {
	var t [150][]int
	for lvl := 0; lvl < 150; lvl++ {
		for idx := 0; idx < item.MaxIndex; idx++ {
			ii, ok := items[idx]
			if !ok || ii.DropItem == 0 || ii.Level < 0 {
				continue
			}
			if lvl <= ii.Level+4 && ii.Level-2 <= lvl && len(t[lvl]) < 100 {
				t[lvl] = append(t[lvl], idx)
			}
		}
	}
	return t
}

// pickDropItem is FUN_004ba8c0: a random entry of the level table.
func (w *World) pickDropItem(level int) int {
	if level < 0 || level >= 150 {
		return -1
	}
	list := w.mw().drops[level]
	if len(list) == 0 {
		return -1
	}
	return list[rnd(len(list))]
}

// --- DamageTable.txt --------------------------------------------------------

// damageRule is a Data/Util/DamageTable.txt row (CDamageTable):
// section 0 (monster -> player, FUN_004b0d90) and section 1 (player ->
// monster, FUN_004b0e70).
type damageRule struct {
	Map                int
	MinLevel, MaxLevel int
	MinReset, MaxReset int
	Class              int // section 1: monster class
	MinMLevel          int // section 0: MinMasterReset (<1) / section 1: monster min level
	MaxMLevel          int // section 0: MaxMasterReset (>=0) / section 1: monster max level
	Rate               int
}

// loadDamageTable reads the DamageTable table of config/data/util.toml.
func loadDamageTable(configDir string) ([2][]damageRule, error) {
	var out [2][]damageRule
	f, err := gamedata.LoadUtilConfig(configDir)
	if err != nil {
		return out, err
	}
	for _, r := range f.DamageTable.Monster {
		out[0] = append(out[0], damageRule{Map: r.MapNumber, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset,
			MaxReset: r.MaxReset, Class: -1, MinMLevel: r.MinDefense, MaxMLevel: r.MaxDefense, Rate: r.DamageRate})
	}
	for _, r := range f.DamageTable.Player {
		out[1] = append(out[1], damageRule{Map: r.MapNumber, MinLevel: r.MinLevel, MaxLevel: r.MaxLevel, MinReset: r.MinReset,
			MaxReset: r.MaxReset, Class: r.MonsterIndex, MinMLevel: r.MonsterMinLevel, MaxMLevel: r.MonsterMaxLevel,
			Rate: r.DamageRate})
	}
	return out, nil
}

// damageTableRate returns the first matching rate (100 when none).
func (w *World) damageTableRate(sec int, p, mon *Object) int {
	any := func(v int) bool { return v == -1 }
	pl, rs := objLevel(p), resets(p)
	for _, r := range w.mw().damage[sec] {
		if !any(r.Map) && r.Map != p.Map {
			continue
		}
		if (!any(r.MinLevel) && pl < r.MinLevel) || (!any(r.MaxLevel) && pl > r.MaxLevel) ||
			(!any(r.MinReset) && rs < r.MinReset) || (!any(r.MaxReset) && rs > r.MaxReset) {
			continue
		}
		if sec == 0 {
			if (!any(r.MinMLevel) && r.MinMLevel >= 1) || (!any(r.MaxMLevel) && r.MaxMLevel < 0) {
				continue
			}
		} else if mon != nil {
			ml := objLevel(mon)
			if (!any(r.Class) && r.Class != mon.Class) || (!any(r.MinMLevel) && ml < r.MinMLevel) ||
				(!any(r.MaxMLevel) && ml > r.MaxMLevel) {
				continue
			}
		}
		return r.Rate
	}
	return 100
}

// --- player death -----------------------------------------------------------

// playerDeathProc is the player part of FUN_004c37f0/FUN_004c3b10: one
// second (obj+0x290 = 0 for players) after death the player leaves the
// viewports (live 8) and on the next pass respawns at the RespawnGate of
// the map with full life and mana (C3 24 F3 04).
func (w *World) playerDeathProc() {
	mw := w.mw()
	now := w.now()
	for idx, d := range mw.deaths {
		o := w.objects[idx]
		if o.State != StatePlaying || o.P == nil || o.P.Char == nil {
			delete(mw.deaths, idx)
			continue
		}
		switch d.Die {
		case 2:
			w.respawnPlayer(o)
			delete(mw.deaths, idx)
		case 1:
			if now.Sub(d.Time) > 1000*time.Millisecond {
				d.Die = 2
				o.Live = LiveRemoved
			}
		}
	}
}

// respawnPlayer is the respawn branch of FUN_004c3b10.
func (w *World) respawnPlayer(o *Object) {
	c := o.P.Char
	o.Alive = true
	o.ActionState = 0
	o.Teleporting = false
	c.Life = float32(o.P.AddLife) + c.MaxLife
	c.Mana = float32(o.P.AddMana) + c.MaxMana
	c.BP = c.MaxBP + float32(o.P.AddBP)
	gate := w.respawnGate(o.Map)
	fixed, fx, fy, fmp := false, 0, 0, 0
	if castleRespawnHook != nil { // map 30 (castlesiege_battle.go)
		if g, m, x, y, ok := castleRespawnHook(w, o); ok {
			if g >= 0 {
				gate = g
			} else {
				fixed, fmp, fx, fy = true, m, x, y
			}
		}
	}
	mp := o.Map
	if g, ok := w.data.Gates[gate]; ok {
		mp = g.Map
	}
	x, y := w.randomGatePosition(gate)
	if fixed {
		mp, x, y = fmp, fx, fy
	}
	w.ravenReset(o)                           // CDarkSpirit mode 0 (pet_raven.go)
	if sx, sy, ok := w.soccerRespawn(o); ok { // respawn type 2, battle soccer (guild_soccer.go)
		mp, x, y = o.Map, sx, sy
	}
	if customRespawnHook != nil { // CustomRespawn.txt (FUN_00458770, custom_world.go)
		if m, cx, cy, ok := customRespawnHook(w, o); ok {
			mp, x, y = m, cx, cy
		}
	}
	w.scriptUserRespawn(o) // BridgeFunction_OnUserRespawn (FUN_004e71c0, lua.go)
	if o.Move != nil {
		w.setOccupancy(o, o.Move.OldX, o.Move.OldY, false)
		o.Move.clearPath()
	} else {
		w.occupy(o, false)
	}
	w.viewportClear(o)
	o.Map, o.X, o.Y, o.TX, o.TY = mp, x, y, x, y
	if o.Move != nil {
		o.Move.OldX, o.Move.OldY = x, y
		w.setOccupancy(o, x, y, true)
	} else {
		w.occupy(o, true)
	}
	o.Live = LiveNew
	w.send(o, packetRespawn(x, y, mp, o.Dir, c.Life, c.Mana, int(c.BP), uint32(c.Experience), uint32(c.Money)))
	w.showSelf(o)
}

func init() {
	worldHandlers[0xD7] = (*World).handleAttack
	leaveWorldHooks = append(leaveWorldHooks, func(w *World, o *Object) {
		if w.mon != nil {
			delete(w.mon.deaths, o.Index)
		}
	})
}
