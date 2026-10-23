package game

import (
	"math"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// CAttack::Attack in full (FUN_0040b110) with PvP: the PvP permission
// FUN_0040dca0, hit check FUN_0040e1c0, defense FUN_0040eaa0, physical
// damage FUN_0040ec10, magic damage FUN_0040f380, the skill effects
// FUN_0040e300, the shield (SD) split FUN_00410370, PvP/PvM rates,
// reflect, life/mana refills, self defense, and the player death part
// of FUN_004cda80 (FUN_0050e8a0 / FUN_0050ec70 death penalty, PK,
// duel, kill count).

// attackArgs are the extra parameters of FUN_0040b110.
type attackArgs struct {
	Skill     int       // skill number (local_28), 0 = normal attack
	Ref       *skillRef // the CSkill (players), nil = none
	SendSkill bool      // param_4: send C3 08 19 to the target
	Fixed     int       // param_6: fixed damage (reflect), 0 = computed
	Hits      int       // param_7: hit number of multi-hit skills
	Combo     bool      // param_8
}

// Skills whose damage uses the DK/DL damage multiplier (FUN_0040b110
// after the minimum damage).
func dkSkill(s int) bool {
	switch s {
	case 19, 20, 21, 22, 23, 41, 42, 43, 44, 46, 49, 51, 52, 55, 56, 57, 232, 236, 269, 344:
		return true
	}
	return false
}

func dlSkill(s int) bool {
	switch s {
	case 60, 61, 62, 65, 74, 78, 238:
		return true
	}
	return false
}

// physicalSkill: the skills that use the physical damage (FUN_0040ec10)
// even for the DW / MG classes.
func physicalSkill(s int) bool {
	switch s {
	case 0, 19, 20, 21, 22, 23, 41, 47, 55, 56, 57, 236, 344:
		return true
	}
	return false
}

func (w *World) playerStats(o *Object) *statsState {
	if o.P == nil {
		return nil
	}
	return w.statsOf(o)
}

// attack keeps the original entry point of the combat module (monsters
// and the D7 handler).
func (w *World) attack(a, t *Object, skill int, sendSkill bool) bool {
	return w.attackFull(a, t, attackArgs{Skill: skill, SendSkill: sendSkill})
}

// attackOwner is the player behind an attacker (the summoner of a summon).
func (w *World) attackOwner(a *Object) *Object {
	if a.Mon != nil && validIdx(a.Mon.Owner) {
		return w.objects[a.Mon.Owner]
	}
	return a
}

func playerReadyForCombat(o *Object) bool {
	return o.Type != TypePlayer || (o.P != nil && o.P.LogoutCountdown < 1)
}

// attackFull is FUN_0040b110.
func (w *World) attackFull(a, t *Object, args attackArgs) bool {
	if a == t || !playerReadyForCombat(a) || !playerReadyForCombat(t) || a.Map != t.Map ||
		a.State != StatePlaying || t.State != StatePlaying {
		return false
	}
	if w.cellHas(a, gamedata.AttrSafeZone) || w.cellHas(t, gamedata.AttrSafeZone) {
		return false
	}
	if t.Type == TypeNPC || !t.Alive || t.Live != LiveNormal || t.Teleporting {
		return false
	}
	if t.Type == TypeMonster {
		if t.Mon.Attribute == 6 || (t.Class > 99 && t.Class < 0x6F) || t.Class == 0x20B ||
			t.Class == 0xDD || t.Class == 0xDE {
			return false
		}
		if a.Type == TypePlayer && t.Mon.Owner == a.Index {
			return false
		}
	}
	// Castle gates, statues and canons only during the battle, by attackers.
	if castleAttackHook != nil && !castleAttackHook(w, a, t) {
		return false
	}
	// Guild war / battle soccer rules of FUN_0040b110 (guild_soccer.go).
	if r, stop := w.guildWarAttackRule(a, t); stop {
		return r
	}
	// WizardTeam custom events (Death, Theft, Sobre, Duel, Pair checks of
	// FUN_0040b110; customevent.go).
	if customEventAttackHook != nil && !customEventAttackHook(w, a, t) {
		return false
	}
	if owner := w.attackOwner(a); owner.Type == TypePlayer && !w.pvpAllowed(owner, t) {
		return false
	}
	skill := args.Skill
	if skill != 0 && !w.skillEnabled(skill, a, t) {
		return false
	}
	w.summonAssist(a, t)   // FUN_004b58f0 (summon.go)
	w.ravenSetTarget(a, t) // FUN_0046b370 (pet_raven.go)
	duel := a.Type == TypePlayer && w.inDuelWith(a, t)
	if duel {
		now := w.now()
		dl(a).Tick, dl(t).Tick = now, now
	}
	if a.Move != nil && a.Move.Authority == 1 {
		w.removeEffect(a, effInvisibility)
	}
	// Immunities.
	miss := hasEffectIndex(t, effImmunity34) ||
		(hasEffectIndex(t, effPhysiImmunity) && skill == 0) ||
		(hasEffectIndex(t, effMagicImmunity) && skill != 0) ||
		(t.Type == TypeMonster && ((hasEffectIndex(t, effMonPhysiImmunity) && skill == 0) ||
			(hasEffectIndex(t, effMonMagicImmunity) && skill != 0)))
	if !miss && t.Type == TypeMonster && t.Class == soccerBallClass {
		// The battle soccer ball rolls 2 cells away (message 6 handled at
		// once, FUN_004b48f0 -> FUN_005114f0, pushback.go) and the hit
		// misses.
		w.knockBack(t, a, 2)
		miss = true
	}
	if miss {
		w.sendMiss(a, t, args)
		return true
	}
	// Maces 2,4 / 2,5: 1/30 chance to cast Ice with a normal attack.
	if args.Fixed == 0 && a.P != nil {
		if r := a.P.Inventory[0]; !r.IsEmpty() && r.Durability > 0 && (r.Index == 0x404 || r.Index == 0x405) {
			if rnd(30) == 0 {
				skill = 7
				args.Skill, args.Ref, args.SendSkill = 7, w.skillRefOf(7, 0), true
			}
		}
	}
	as, ts := w.combatStats(a), w.combatStats(t)
	ao, to := effOpt(a), effOpt(t)
	effect := 0
	stuck := false
	dmg := 0
	if args.Fixed == 0 {
		reduced, hit := hitCheckFull(as, ts, ao, to)
		if !hit {
			w.sendMiss(a, t, args)
			return true
		}
		def := defenseFull(a, t, ts, as, to, &effect)
		switch {
		case a.Type == TypePlayer && skill != 0 && (a.Class == 0 || a.Class == 3 || a.Class == 5) && !physicalSkill(skill):
			dmg = w.magicDamage(a, t, as, ao, args.Ref, skill, &effect, def)
		default:
			dmg = w.physicalDamage(a, t, as, ao, args.Ref, skill, &effect, def)
		}
		if reduced {
			dmg = dmg * 30 / 100
		}
		if t.P != nil {
			for _, r := range w.playerStats(t).DamageReduction {
				dmg = dmg - r*dmg/100
			}
		}
		if to.DamageReduction > 0 {
			dmg = dmg - to.DamageReduction*dmg/100
		}
		if t.Skill != nil && !t.Skill.DefenseTime.IsZero() &&
			w.now().Sub(t.Skill.DefenseTime) < time.Duration(w.cfg.Int("Skill", "DefenseTimeConstA", 0))*time.Second {
			dmg = dmg - w.cfg.Int("Skill", "DefenseConstA", 0)*dmg/100
		}
		minDmg := max(objLevel(a)/10, 1)
		if dmg < minDmg {
			dmg = minDmg + rnd(minDmg)
		}
		if a.Type == TypePlayer && skill != 0 {
			switch {
			case dkSkill(skill) || (skill == 47 && rides(a)):
				if a.Class == 1 {
					dmg = w.playerStats(a).DKDamageMultiplier * dmg / 100
				} else {
					dmg = dmg * 200 / 100
				}
			case dlSkill(skill):
				dmg = w.playerStats(a).DLDamageMultiplier * dmg / 100
			case skill == 76:
				lv := 0
				if l := objLevel(a); l > 300 {
					lv = l - 300
				}
				dmg = (lv/5 + 200) * dmg / 100
			case skill == chainLightningSkill:
				dmg = chainLightningRate(args.Hits) * dmg / 100 // skill_chain.go
			}
		}
		if rnd(100) < as.DoubleDamageRate {
			effect |= effectDouble
			dmg *= 2
		}
		if rnd(100) < as.TripleDamageRate {
			effect |= effectTriple
			dmg *= 3
		}
		if args.Combo && a.Type == TypePlayer && a.Class >= 0 && a.Class < 5 {
			effect |= effectCombo
			p := classKeys[a.Class]
			sum := addStat(a, 0) + addStat(a, 1) + addStat(a, 3)
			dmg = w.cfg.Int("Character", p+"ComboDamageConstB", 0)*divOr0(sum, w.cfg.Int("Character", p+"ComboDamageConstA", 0)) + dmg*2
			dmg = w.cfg.Int("Character", p+"ComboDamageConstC", 0) * dmg / 100
			skill = 59
		}
	} else if skill != 79 {
		effect = 4
		key := "ReflectDamageRatePvM"
		if a.Type == TypePlayer && t.Type == TypePlayer {
			key = "ReflectDamageRatePvP"
		}
		dmg = w.cfg.Int("Character", key, 0) * args.Fixed / 100
		if a.Type == TypePlayer && t.Type == TypePlayer && validClass(a) && validClass(t) {
			dmg = w.cfg.Int("Character", classKeys[a.Class]+"ReflectDamageRateTo"+classKeys[t.Class], 0) * dmg / 100
		}
	}
	if a.Type == TypePlayer && validClass(a) {
		if t.Type != TypePlayer {
			general := w.cfg.Int("Character", "GeneralDamageRatePvM", 100)
			dmg = w.cfg.Int("Character", classKeys[a.Class]+"DamageRatePvM", 100) * (general * dmg / 100) / 100
		} else if validClass(t) {
			general := w.cfg.Int("Character", "GeneralDamageRatePvP", 100)
			cls := w.cfg.Int("Character", classKeys[a.Class]+"DamageRatePvP", 100)
			to := w.cfg.Int("Character", classKeys[a.Class]+"DamageRateTo"+classKeys[t.Class], 100)
			dmg = to * (cls * (general * dmg / 100) / 100) / 100
			arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
			switch {
			case duel:
				dmg = w.cfg.Int("Character", "DuelDamageRate", 100) * dmg / 100
			case a.Map == arena && t.Map == arena:
				dmg = w.cfg.Int("Character", "CustomArenaDamageRate", 100) * dmg / 100
			case a.Map > 0x11 && a.Map < 0x18 && t.Map > 0x11 && t.Map < 0x18:
				dmg = w.cfg.Int("Character", "ChaosCastleDamageRate", 100) * dmg / 100
			default:
				// CastleSiegeDamageRate1/2 on map 30 (castlesiege_battle.go).
				if castleDamageRateHook != nil {
					if r, ok := castleDamageRateHook(w, a, t); ok {
						dmg = r * dmg / 100
					}
				}
			}
		}
	}
	dmg = max(dmg, 0)
	if a.Type == TypeMonster && t.Type == TypePlayer {
		dmg = w.damageTableRate(0, t, nil) * dmg / 100
	}
	if a.Type == TypePlayer && t.Type == TypeMonster {
		dmg = w.damageTableRate(1, a, t) * dmg / 100
	}
	if a.Type == TypePlayer && effect != 4 {
		w.refillRolls(a, [4]int{0, 0, 0, 0})
	}
	if skill != 0 && args.Hits < 2 {
		w.skillHitEffect(a, t, skill, dmg)
		if args.SendSkill {
			num := skill
			if args.Ref != nil {
				num = args.Ref.Num
			}
			w.sendSkillUse(a, t, num, true)
		}
	}
	sd := 0
	if a.Type == TypePlayer && t.Type == TypePlayer {
		sd = w.shieldDamage(a, t, dmg)
		life := objLife(t)
		if v := float32(dmg - sd); v <= life {
			life -= v
		} else {
			life = 0
		}
		setObjLife(t, life)
		c := t.P.Char
		if int(c.Shield) < sd {
			c.Shield = 0
		} else {
			c.Shield -= float32(sd)
		}
	} else {
		life := objLife(t)
		if v := float32(dmg); v <= life {
			life -= v
		} else {
			life = 0
		}
		setObjLife(t, life)
	}
	if t.Type == TypeMonster {
		t.Mon.LastAttacker = a.Index
		w.sendMsg(t, msgAttacked, a.Index, 100, 0)
	}
	if dmg > 0 && a.Type == TypePlayer && t.Type == TypePlayer {
		arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
		self := effect != 4 && !duel && !(a.Map == arena && t.Map == arena) &&
			!(a.Map > 0x11 && a.Map < 0x18 && t.Map > 0x11 && t.Map < 0x18)
		if info, ok := w.data.Maps[a.Map]; ok && info.NonPK == 2 {
			self = false
		}
		if guildWarHook != nil && guildWarHook(w, a, t) {
			self = false
		}
		if self {
			w.addSelfDefense(a, t)
		}
	}
	if a.Move != nil {
		a.Move.Flag270 = 0
	}
	if dmg < 1 {
		w.sendDamage(a, t, 0, effect, false)
		return true
	}
	w.removeEffect(t, effRemovedOnHit)
	if t.Type == TypePlayer {
		if effect != 4 {
			ts := w.playerStats(t)
			if rnd(100) < ts.Opt109 {
				w.queueReflect(t, a, dmg)
			} else if r := ts.DamageReflect + to.DamageReflect; r > 0 {
				w.queueReflect(t, a, r*dmg/100)
			}
			w.refillRolls(t, [4]int{ts.Opt110, -1, ts.Opt111, 0})
		}
		// DamageStuckRate: a player without a pet may get the stuck flag.
		if !hasPet(t) && validClass(t) && rnd(100) < w.cfg.Int("Character", classKeys[t.Class]+"DamageStuckRate", 0) {
			stuck = true
		}
	}
	w.lifeCheckEx(a, t, dmg-sd, effect, skill, stuck)
	return true
}

func validClass(o *Object) bool { return o.Class >= 0 && o.Class < 5 }

// rides: slot 8 holds Dinorant (13,3), Uniria (13,2) or Fenrir (13,37).
func rides(o *Object) bool {
	if o.P == nil {
		return false
	}
	it := o.P.Inventory[8]
	return !it.IsEmpty() && (it.Index == 0x1a02 || it.Index == 0x1a03 || it.Index == 0x1a25)
}

func hasPet(o *Object) bool {
	if o.P == nil {
		return false
	}
	it := o.P.Inventory[8]
	return !it.IsEmpty() && it.Durability > 0
}

// refillRolls are the four "full refill" rolls of FUN_0040b110 (life,
// shield, mana, BP for the attacker; life, mana, BP for the target, -1 =
// no roll). The rates (obj+0x92c..0x940) are 0 in this build, except the
// target life/mana (obj+0x924/0x928 = stats Opt110/Opt111).
func (w *World) refillRolls(o *Object, rates [4]int) {
	c := o.P.Char
	for i, r := range rates {
		if r < 0 {
			continue
		}
		if rnd(100) >= r {
			continue
		}
		switch i {
		case 0:
			c.Life = objMaxLife(o)
			w.send(o, packetLife(0xFF, int(c.Life)))
		case 1:
			c.Shield = float32(w.statsOf(o).AddShield) + c.MaxShield
			w.send(o, packetLife(0xFF, int(c.Life)))
		case 2:
			c.Mana = float32(o.P.AddMana) + c.MaxMana
			w.send(o, packetMana(int(c.Mana), int(c.BP)))
		case 3:
			c.BP = float32(o.P.AddBP) + c.MaxBP
			w.send(o, packetMana(int(c.Mana), int(c.BP)))
		}
	}
}

// queueReflect is FUN_00510550(10, attacker, 10, damage) on the target:
// 10 ms later the target hits the attacker with that fixed damage.
func (w *World) queueReflect(t, a *Object, dmg int) {
	w.queueSkillHit(t, 10, a.Index, 10, 0, dmg)
}

// sendMiss is FUN_0040e090: D7 with no damage, plus 0x19 when asked.
func (w *World) sendMiss(a, t *Object, args attackArgs) {
	w.sendDamage(a, t, 0, 0, false)
	if args.SendSkill && args.Skill != 0 {
		num := args.Skill
		if args.Ref != nil {
			num = args.Ref.Num
		}
		w.sendSkillUse(a, t, num, true)
	}
}

// hitCheckFull is FUN_0040e1c0 with the effect fields: attack rate
// obj+0x35c + 0x70c, defense rate obj+0x370 + 0x71c (the percent fields
// are 0).
func hitCheckFull(as, ts CombatStats, ao, to effectOptions) (reduced, hit bool) {
	ar := max(as.AttackSuccessRate+ao.AddAttackSuccessRate, 0)
	dr := max(ts.DefenseSuccessRate+to.AddDefenseSuccessRate, 0)
	if ar < dr {
		return true, rnd(100) <= 4
	}
	if ar == 0 {
		ar = 1
	}
	return false, rnd(ar) >= dr
}

// defenseFull is FUN_0040eaa0: Defense + Greater Defense (obj+0x718),
// halved for players, ignored on the IgnoreDefenseRate roll.
func defenseFull(a, t *Object, ts, as CombatStats, to effectOptions, effect *int) int {
	def := to.AddDefense + ts.Defense
	if t.Type == TypePlayer {
		def = def * 50 / 100
	}
	if rnd(100) < as.IgnoreDefenseRate {
		*effect = effectIgnoreDefense
		def = 0
	}
	return max(def, 0)
}

// physicalDamage is FUN_0040ec10.
func (w *World) physicalDamage(a, t *Object, as CombatStats, ao effectOptions, ref *skillRef, skill int, effect *int, def int) int {
	var dmg int
	if a.Type != TypePlayer {
		lo := as.DamageMinRight + ao.AddPhysiDamage
		hi := as.DamageMaxRight + ao.AddPhysiDamage
		dmg = lo + rnd(max(hi-lo, 1))
	} else {
		sLo, sHi := 0, 0
		if ref != nil {
			sLo, sHi = ref.DamageMin, ref.DamageMax
			if a.Class == 4 {
				c := func(k string) int { return w.cfg.Int("Character", k, 0) }
				switch skill {
				case 62:
					pet := 0
					if it := a.P.Inventory[8]; !it.IsEmpty() && w.statsData != nil {
						pet = w.playerStats(a).Items[8].PetLevel
					}
					add := c("EarthquakeDamageConstC")*pet + divOr0(addStat(a, 4), c("EarthquakeDamageConstB")) +
						divOr0(addStat(a, 0), c("EarthquakeDamageConstA"))
					sLo, sHi = sLo+add, sHi+add
				case 65:
					add := c("ElectricSparkDamageConstB") + divOr0(addStat(a, 4), c("ElectricSparkDamageConstA"))
					sLo, sHi = sLo+add, sHi+add
				default:
					add := divOr0(addStat(a, 0), c("DLSkillDamageConstA")) + divOr0(addStat(a, 3), c("DLSkillDamageConstB"))
					sLo, sHi = sLo+add, sHi+add
				}
			}
		}
		var lo, hi int
		if twoWeapons(a) {
			if w.cfg.Int("Skill", "TwoWeaponsDamageEnable", 100) == 0 {
				lo = as.DamageMinRight + as.DamageMinLeft + sLo
				hi = as.DamageMaxRight + as.DamageMaxLeft + sHi
			} else {
				p := classKeys[min(max(a.Class, 0), 4)]
				lo = (as.DamageMinRight + as.DamageMinLeft + sLo) * w.cfg.Int("Skill", p+"TwoWeaponsDamageMinConstA", 100) / 100
				hi = (as.DamageMaxRight + as.DamageMaxLeft + sHi) * w.cfg.Int("Skill", p+"TwoWeaponsDamageMaxConstA", 100) / 100
			}
		} else if useRightHand(a) {
			lo, hi = as.DamageMinRight+sLo, as.DamageMaxRight+sHi
		} else {
			lo, hi = as.DamageMinLeft+sLo, as.DamageMaxLeft+sHi
		}
		lo += ao.AddPhysiDamage
		hi += ao.AddPhysiDamage
		dmg = rnd(max(hi-lo, 1)) + lo
		if rnd(100) < as.CriticalDamage {
			*effect = effectCritical
			dmg = as.CriticalDamageBonus + ao.AddCriticalDamage + hi
		}
		if rnd(100) < as.ExcellentDamage {
			*effect = effectExcellent
			dmg = hi*120/100 + as.ExcellentDamageBonus
		}
	}
	dmg = max(dmg-def, 0)
	return w.skillDamageRate(skill, t, dmg)
}

// magicDamage is FUN_0040f380 (DW / MG skills).
func (w *World) magicDamage(a, t *Object, as CombatStats, ao effectOptions, ref *skillRef, skill int, effect *int, def int) int {
	lo, hi := as.MagicDamageMin, as.MagicDamageMax
	if skill == 0x28 {
		c := func(k string) int { return w.cfg.Int("Character", k, 0) }
		stage := 0
		if a.Skill != nil {
			stage = a.Skill.NovaStage
		}
		add := divOr0(addStat(a, 0), c("NovaDamageConstC")) + stage*c("NovaDamageConstB")*c("NovaDamageConstA")
		lo, hi = lo+add, hi+add
	} else if ref != nil {
		lo, hi = lo+ref.DamageMin, hi+ref.DamageMax
	}
	lo += ao.AddMagicDamage
	hi += ao.AddMagicDamage
	if s := w.playerStats(a); s != nil {
		if r := s.Items[0]; validIndex(r.Index) && s.Valid[0] && (r.Index < 0x200 || (r.Index > 0x9ff && r.Index < 0xc00)) {
			rise := int(math.RoundToEven(float64(float32(int(uint32(r.MagicPower)>>1)+r.Level*2) * r.DurFactor)))
			lo = lo + rise*lo/100
			hi = hi + rise*hi/100
		}
	}
	dmg := rnd(max(hi-lo, 1)) + lo
	if rnd(100) < as.CriticalDamage {
		*effect = effectCritical
		dmg = as.CriticalDamageBonus + ao.AddCriticalDamage + hi
	}
	if rnd(100) < as.ExcellentDamage {
		*effect = effectExcellent
		dmg = hi*120/100 + as.ExcellentDamageBonus
	}
	dmg = max(dmg-def, 0)
	return w.skillDamageRate(skill, t, dmg)
}

// skillDamageRate applies SkillDamage.txt section 0 (FUN_0049f5a0): the
// PvM rate against monsters, the PvP rate otherwise.
func (w *World) skillDamageRate(skill int, t *Object, dmg int) int {
	if skill == 0 || w.skills == nil {
		return dmg
	}
	r, ok := w.skills.damage[skill]
	if !ok {
		return dmg
	}
	if t.Type == TypeMonster {
		return r[0] * dmg / 100
	}
	return r[1] * dmg / 100
}

// shieldDamage is FUN_00410370: the part of a PvP hit taken by the
// shield (SD). The rate comes from CServerInfo+0x494 (copied to
// obj+0x8b8 by FUN_0050bc70), which no configuration key loads in this
// build: it is 0, so the shield never absorbs damage.
func (w *World) shieldDamage(a, t *Object, dmg int) int {
	const shieldRate = 0 // CServerInfo+0x494
	rate := shieldRate
	if rnd(100) < 0 { // obj+0x840 (attacker) - obj+0x85c (target): 0
		rate = 0
	}
	rate = min(max(rate, 0), 100)
	sd := rate * dmg / 100
	if cur := int(t.P.Char.Shield); cur < sd {
		sd = cur
	}
	return sd
}

// skillHitEffect is FUN_0040e300: the effects of a skill hit (poison,
// ice, push-back...). Effects are refused by Chaos Castle/CS objects, by
// a resisting element (FUN_0050cfc0, not reproduced: the elements are 0)
// and by the SkillDamage.txt section 1 rate.
func (w *World) skillHitEffect(a, t *Object, skill, dmg int) {
	if t.Type != TypePlayer {
		switch c := t.Class; {
		case c > 0xcb && c < 0xd2, c > 0xd6 && c < 0xdc, c == 0x115, c == 0x116, c == 0x11b, c == 0x120:
			return
		}
	}
	if w.skills != nil {
		if r, ok := w.skills.effectRate[skill]; ok && rnd(100) >= r {
			return
		}
	}
	c := func(k string) int { return w.cfg.Int("Skill", k, 0) }
	e := w.data.Skills[skill].Effect
	switch skill {
	case 1: // Poison
		w.addEffect(t, e, c("PoisonEffectTime"), [4]int{a.Index, c("PoisonEffectConstA"), c("PoisonEffectConstB"), 0}, true)
	case 38: // Decay
		w.addEffect(t, e, c("DecayEffectTime"), [4]int{a.Index, c("DecayEffectConstA"), c("DecayEffectConstB"), 0}, true)
	case 7: // Ice
		w.addEffect(t, e, c("IceEffectTime"), [4]int{a.Index}, true)
	case 39: // Ice Storm (FUN_00500550)
		w.addEffect(t, e, c("IceStormEffectTime"), [4]int{a.Index}, true)
	case 51: // Ice Arrow
		w.addEffect(t, e, c("IceArrowEffectTime"), [4]int{}, true)
	case 55: // Fire Slash (FUN_005005d0): defense down
		v := c("FireSlashConstA") + divOr0(addStat(a, 3), c("FireSlashConstB"))
		v = min(v, c("FireSlashMaxRate"))
		w.addEffect(t, e, c("FireSlashTimeConstA"), [4]int{v}, true)
	case 3, 19, 20, 21, 22, 23:
		// Lightning and the DK basic skills push the target back
		// (message 2 after 150 ms, FUN_00510550; pushback.go).
		w.queuePushBack(t, a, 150)
	}
}

// --- PvP permission ------------------------------------------------------------

// pvpAllowed is FUN_0040dca0 (attacker a, target t): non-player targets
// are always allowed; GM targets (Authority 8 is checked on the target)
// never; then party, guild war, event map and the NonPK / level rules.
// The alliance and Castle Siege rules are not implemented.
func (w *World) pvpAllowed(a, t *Object) bool {
	if t.Type != TypePlayer {
		return true
	}
	if !w.scriptCheckUserTarget(a, t) { // BridgeFunction_OnCheckUserTarget (FUN_004e7290, lua.go)
		return false
	}
	if t.Move != nil && t.Move.Authority == 8 {
		return false
	}
	if w.cfg.Int("Common", "PartyDisableKillBetweenMembers", 0) != 0 {
		if pa, pt := w.partyNumber(a), w.partyNumber(t); pa >= 0 && pa == pt {
			return false
		}
	}
	// Guild war (FUN_004864b0, guild_war.go) allows the hit unless the
	// map is NonPK.
	if guildWarHook != nil && guildWarHook(w, a, t) && !w.mapNonPK(t.Map) {
		return true
	}
	arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
	if a.Map == arena && t.Map == arena && arena >= 0 {
		return arenaFightHook != nil && arenaFightHook(w, a) // FUN_00443f50 (customarena.go)
	}
	ds := func(m int) bool { return m == 9 || m == 32 }
	bc := func(m int) bool { return m >= 0xb && m <= 0x11 }
	if ds(a.Map) && ds(t.Map) && w.cfg.Int("Event", "DevilSquareDuelEnable", 0) == 0 {
		return false
	}
	if bc(a.Map) && bc(t.Map) && w.cfg.Int("Event", "BloodCastleDuelEnable", 0) == 0 {
		return false
	}
	if a.Map > 0x11 && a.Map < 0x18 && t.Map > 0x11 && t.Map < 0x18 {
		return chaosCastleStateHook != nil && chaosCastleStateHook(w, t.Map-18) == 4
	}
	// Map 30 during the Castle Siege battle (castlesiege_battle.go).
	if castlePvPHook != nil {
		if ok, decided := castlePvPHook(w, a, t); decided {
			return ok
		}
	}
	if w.cfg.Int("Common", "EnableKillLevel1", 0) != 0 || (objLevel(t) > 5 && objLevel(a) > 5) {
		return !w.mapNonPK(t.Map)
	}
	return false
}

// mapNonPK is FUN_004ae780: MapManager NonPK == 1, or the NonPK key when
// the column is "*".
func (w *World) mapNonPK(mp int) bool {
	if info, ok := w.data.Maps[mp]; ok && info.NonPK != -1 {
		return info.NonPK == 1
	}
	return w.cfg.Int("Common", "NonPK", 0) != 0
}

// --- player death ----------------------------------------------------------------

// objectDeathHook runs from lifeCheck (FUN_004cda80) when t dies, before
// the die packet.
func (w *World) onObjectDeath(killer, t *Object) {
	if killer.Type == TypePlayer && t.Type == TypePlayer {
		st := sk(killer)
		if st.KillCount != 0xff {
			st.KillCount++
		}
		w.send(killer, packetKillCount(st.KillCount))
		// FUN_004868f0: the kill may count for a guild war.
		if guildWarKillHook != nil {
			guildWarKillHook(w, killer, t)
		}
	}
	if t.Type == TypePlayer {
		w.playerDeath(killer, t)
		if killer.Type == TypePlayer && !w.inDuelWith(killer, t) && playerKillHook != nil {
			playerKillHook(w, killer, t)
		}
		w.duelKillEnd(killer, t)
	}
	w.clearEffects(t)
	if t.Skill != nil {
		t.Skill.NovaCharging = false
		t.Skill.Pending = nil
	}
}

// playerDeath is FUN_0050e8a0 (victim t): kill count reset, PK death
// announce, the duel score and the death penalty FUN_0050ec70.
func (w *World) playerDeath(killer, t *Object) {
	duel := w.inDuelWith(killer, t)
	if !duel && t.PKLevel > 4 && w.cfg.Int("Common", "PKDeathAnnounce", 0) != 0 && killer.Type == TypePlayer {
		w.noticeAll(0, w.messageFor(t, 0x17f, objName(t), objName(killer)))
	}
	sk(t).KillCount = 0
	w.send(t, packetKillCount(0))
	if t.Map >= 0x12 && t.Map < 0x18 {
		return // Chaos Castle handles its own deaths
	}
	if duel {
		w.duelScore(killer, t)
		return
	}
	w.deathPenalty(killer, t)
}

// duelKillEnd is FUN_00477430 (duel.go).
func (w *World) duelKillEnd(killer, t *Object) { w.duelKill(killer, t) }

// deathPenalty is FUN_0050ec70: experience loss by PK level
// (ExperienceDeduceRate1..5 of the current level's experience span) and
// money loss (MoneyDeduceRate). The item drop of DropItemRate1..5 is not
// reproduced (all rates are 0 in this installation).
func (w *World) deathPenalty(killer, t *Object) {
	c := t.P.Char
	var rate int
	switch lv := t.PKLevel; {
	case lv < 3:
		rate = w.cfg.Int("Common", "ExperienceDeduceRate1", 0)
	case lv == 3:
		rate = w.cfg.Int("Common", "ExperienceDeduceRate2", 0)
	case lv == 4:
		rate = w.cfg.Int("Common", "ExperienceDeduceRate3", 0)
	case lv == 5:
		rate = w.cfg.Int("Common", "ExperienceDeduceRate4", 0)
	default:
		rate = w.cfg.Int("Common", "ExperienceDeduceRate5", 0)
	}
	changed := false
	if rate > 0 && w.data.Experience != nil {
		floor := w.data.Experience.Floor(c.Level)
		next := w.data.Experience.Next(c.Level)
		loss := uint32((uint64(next-floor) * uint64(uint32(rate))) / 100)
		if loss != 0 {
			exp := uint32(c.Experience) - loss
			if exp < floor {
				exp = floor
			}
			c.Experience = int64(exp)
			changed = true
		}
	}
	if mr := w.cfg.Int("Common", "MoneyDeduceRate", 0); mr > 0 && c.Money != 0 {
		loss := int64(uint32(mr) * uint32(c.Money) / 100)
		if loss != 0 {
			c.Money -= loss
			w.sendMoney(t)
		}
	}
	if changed {
		w.send(t, w.packetCharRefresh(t))
	}
}

// packetCharRefresh is the WizardTeam C1 A4 F3 E0 (FUN_004dba70): level,
// points, experience, stats, life/mana/BP (u16 capped and u32) and the
// class, then u32 reset count (obj+0x6dc) at 48, master resets (obj+0x6e0)
// at 52 and account level (obj+0x618) at 56. The other custom fields
// (obj+0x61c..0x62c) are not identified and are sent as 0.
func (w *World) packetCharRefresh(o *Object) []byte {
	c := o.P.Char
	s := w.statsOf(o)
	p := newPacket(frame.C1, 0xF3, 0xE0, 0xA4)
	u16le(p[4:], c.Level)
	u16le(p[6:], c.LevelUpPoints)
	u32le(p[8:], uint32(c.Experience))
	u32le(p[12:], o.P.NextExp)
	for i, v := range []int{c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership} {
		u16le(p[16+2*i:], clamp65000(v))
	}
	cap16 := func(f float32) int {
		if f > 65000 {
			return 65000
		}
		return int(math.RoundToEven(float64(f)))
	}
	maxLife := float32(o.P.AddLife) + c.MaxLife
	maxMana := float32(o.P.AddMana) + c.MaxMana
	u16le(p[26:], cap16(c.Life))
	u16le(p[28:], cap16(maxLife))
	u16le(p[30:], cap16(c.Mana))
	u16le(p[32:], cap16(maxMana))
	u16le(p[34:], clamp65000(int(c.BP)))
	u16le(p[36:], clamp65000(s.MaxBP+s.AddBP))
	u16le(p[38:], c.FruitAddPoints)
	u16le(p[40:], maxFruit(o.Class, c.Level))
	u16le(p[42:], c.FruitSubPoints)
	u16le(p[44:], maxFruit(o.Class, c.Level))
	u16le(p[46:], o.Class)
	u32le(p[48:], uint32(c.Resets))
	u32le(p[52:], uint32(c.MasterResets))
	u32le(p[56:], uint32(o.P.Account.Level))
	auth := 0
	if o.Move != nil {
		auth = o.Move.Authority
	}
	u32le(p[112:], uint32(auth))
	u32le(p[116:], uint32(c.LevelUpPoints))
	u32le(p[120:], uint32(int32(math.RoundToEven(float64(c.Life)))))
	u32le(p[124:], uint32(int32(math.RoundToEven(float64(maxLife)))))
	u32le(p[128:], uint32(int32(math.RoundToEven(float64(c.Mana)))))
	u32le(p[132:], uint32(int32(math.RoundToEven(float64(maxMana)))))
	u32le(p[136:], uint32(int32(c.BP)))
	u32le(p[140:], uint32(s.MaxBP+s.AddBP))
	for i, v := range []int{c.Strength, c.Dexterity, c.Vitality, c.Energy, c.Leadership} {
		u32le(p[144+4*i:], uint32(v))
	}
	return p
}

func init() {
	objectDeathHook = func(w *World, killer, t *Object) { w.onObjectDeath(killer, t) }
}
