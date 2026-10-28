package game

import (
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// Dark Raven attack AI (CDarkSpirit, docs/server-plan/09c-chaos-loja-pets.md
// "IA do Dark Raven"):
//
//   - FUN_0046b0c0 (from CalcCharacter): the raven becomes active for a
//     Dark Lord with a usable 13,5 in slot 1 (ravenSync);
//   - FUN_0046aae0 (every 100 ms from FUN_004c48b0, players): one action
//     every (150 - AttackSpeed) * 10 ms by mode: 1 random target
//     (FUN_0046ac40), 2/3 the target set by the owner's attacks or by A7
//     (FUN_0046ad80), with a DarkSpiritRangeAttackRate chance of the
//     splash attack (FUN_0046ae30);
//   - FUN_0046afa0: C1 09 A8 to the owner and its viewport, then message
//     0x33 after 600 ms on the owner's queue (FUN_00510690);
//   - FUN_0046b5a0 (message 0x33, FUN_004c4280): the raven's own CAttack
//     with FUN_0046c660 (hit), FUN_0046c710 (defense), FUN_0046c7b0
//     (damage) and FUN_0046c440 (raven durability, PetDamageTable.txt).

// ravenModeChange answers mode resets with C1 07 A7 00 00 FF FF
// (FUN_0046b0c0 / FUN_0046b3b0 / FUN_0050f6d0).
func packetRavenReset() []byte { return packetPetCommand(0, 0xFFFF) }

// packetRavenAttack is C1 09 A8 00 splash ownerHi ownerLo targetHi
// targetLo (FUN_0046afa0).
func packetRavenAttack(splash bool, owner, target int) []byte {
	p := newPacket(frame.C1, 0xA8, -1, 9)
	if splash {
		p[4] = 1
	}
	p[5], p[6] = byte(owner>>8), byte(owner)
	p[7], p[8] = byte(target>>8), byte(target)
	return p
}

// ravenSync is the CDarkSpirit part of FUN_0046b0c0, run at the end of
// every CalcCharacter of a Dark Lord: the spirit is reset (owner, target,
// stats, next attack) and set up again when slot 1 holds a valid raven.
// A raven that becomes active while a mode other than 0 is kept falls
// back to mode 0 with C1 07 A7 00 00 FF FF.
func (w *World) ravenSync(o *Object, s *statsState) {
	if o.Class != 4 || o.P == nil {
		return
	}
	ps := pet(o)
	wasOff := !ps.Owner
	ps.Owner, ps.Target, ps.Next = false, -1, time.Time{}
	ps.DamageMin, ps.DamageMax, ps.AttackSpeed, ps.SuccessRate = 0, 0, 0, 0
	if s.Items[1].Index != idxDarkRaven || !s.Valid[1] {
		return
	}
	ps.Owner = true
	ps.DamageMin, ps.DamageMax = s.SpiritDamageMin, s.SpiritDamageMax
	ps.AttackSpeed, ps.SuccessRate = s.SpiritSpeed, s.SpiritRate
	if wasOff && ps.Mode != 0 {
		ps.Mode = 0
		w.send(o, packetRavenReset())
	}
}

// ravenReset is the A7 reset of FUN_0050f6d0 (gate move) and of the death
// of the owner (FUN_004c3b10): mode 0 when it was not.
func (w *World) ravenReset(o *Object) {
	if o.P == nil || o.P.Pet == nil || !o.P.Pet.Owner || o.Class != 4 || o.P.Pet.Mode == 0 {
		return
	}
	o.P.Pet.Mode = 0
	w.send(o, packetRavenReset())
}

// ravenSetTarget is FUN_0046b370: every attack of the owner (CAttack,
// FUN_0040b110 / FUN_0046b5a0) becomes the raven target.
func (w *World) ravenSetTarget(a, t *Object) {
	if a.Type != TypePlayer || a.P == nil || a.P.Pet == nil || !a.P.Pet.Owner || a.Class != 4 {
		return
	}
	a.P.Pet.Target = t.Index
}

// ravenTargetLost is FUN_0046b3b0: the target is forgotten; in mode 3 the
// raven goes back to mode 0.
func (w *World) ravenTargetLost(o *Object, idx int) {
	ps := o.P.Pet
	if !ps.Owner || o.Class != 4 || ps.Target != idx {
		return
	}
	ps.Target = -1
	if ps.Mode == 3 {
		ps.Mode = 0
		w.send(o, packetRavenReset())
	}
}

// ravenTick is FUN_0046aae0 for one player.
func (w *World) ravenTick(o *Object) {
	ps := o.P.Pet
	if ps == nil || !ps.Owner || o.State != StatePlaying || o.Type != TypePlayer ||
		o.P.LogoutCountdown > 0 || o.P.Moving || o.Class != 4 {
		return
	}
	if r := o.P.Inventory[1]; r.Index != idxDarkRaven || r.Durability == 0 {
		return
	}
	if w.cellHas(o, gamedata.AttrSafeZone) {
		return
	}
	now := w.now()
	if now.Before(ps.Next) {
		return
	}
	// CHelperOffline (obj+0x2110): the raven acts only when obj+0x224c is
	// set, a field no decompiled code writes (U-PET-04), so it rests.
	if o.P.cust != nil && o.P.cust.Offline.Helper {
		return
	}
	ps.Next = now.Add(time.Duration((150-ps.AttackSpeed)*10) * time.Millisecond)
	switch ps.Mode {
	case 1:
		w.ravenRandom(o)
	case 2, 3:
		w.ravenAttackTarget(o)
	}
}

// ravenRandom is FUN_0046ac40: a random object of the owner's VP2 that an
// area skill could hit (FUN_004f8440) within 5 cells.
func (w *World) ravenRandom(o *Object) {
	var list []int
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || !w.skillTargetOK(o, e.Index, e.Type, -1) {
			continue
		}
		if distance(w.objects[e.Index], o) < 5 {
			list = append(list, e.Index)
		}
	}
	if len(list) == 0 {
		return
	}
	w.ravenStrike(o, list[rnd(len(list))], false)
}

// ravenAttackTarget is FUN_0046ad80 (modes 2 and 3).
func (w *World) ravenAttackTarget(o *Object) {
	ps := o.P.Pet
	if ps.Target < 0 || ps.Target >= MaxObjects {
		return
	}
	t := w.objects[ps.Target]
	if !t.Alive || t.Live != LiveNormal || t.Map != o.Map {
		w.ravenTargetLost(o, t.Index)
		return
	}
	if distance(t, o) >= 7 {
		return
	}
	if rnd(100) < w.cfg.Int("Character", "DarkSpiritRangeAttackRate", 0) {
		w.ravenSplash(o, t)
		return
	}
	w.ravenStrike(o, t.Index, false)
}

// ravenSplash is FUN_0046ae30: the target and up to 10 others of the
// owner's VP2 within 4 cells of it, all with 60% damage.
func (w *World) ravenSplash(o, t *Object) {
	w.ravenStrike(o, t.Index, true)
	n := 0
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Index < 0 || e.Index == t.Index || !w.skillTargetOK(o, e.Index, e.Type, t.Index) {
			continue
		}
		if distance(w.objects[e.Index], t) >= 4 {
			continue
		}
		w.queueRavenHit(o, e.Index, false, true)
		if n++; n > 9 {
			break
		}
	}
}

// ravenStrike is FUN_0046afa0 with send = 1: the A8 animation to the
// owner and its viewport, then the delayed hit.
func (w *World) ravenStrike(o *Object, target int, splash bool) {
	p := packetRavenAttack(splash, o.Index, target)
	w.send(o, p)
	w.broadcast(o, p)
	w.queueRavenHit(o, target, true, splash)
}

// queueRavenHit is FUN_00510690(0x33, target, 600, send, splash) on the
// owner's message queue (skill.go skillMessages -> ravenAttack).
func (w *World) queueRavenHit(o *Object, target int, send, splash bool) {
	st := sk(o)
	if len(st.Pending) >= 30 {
		return
	}
	param := 0
	if splash {
		param = 1
	}
	st.Pending = append(st.Pending, delayedHit{Code: 0x33, Target: target, Param: param, Send: send,
		Due: w.now().Add(600 * time.Millisecond)})
}

// ravenHitCheck is FUN_0046c660: the raven success rate against the
// target's defense rate (obj+0x370). reduced is the byte the caller
// turns into a 30% hit.
func ravenHitCheck(rate, defRate int) (reduced, hit bool) {
	if rate < defRate {
		return true, rnd(100) < 5
	}
	if rate == 0 {
		rate = 1
	}
	return false, defRate <= rnd(rate)
}

// ravenDefense is FUN_0046c710: Defense + Greater Defense (obj+0x718),
// halved for every target.
func ravenDefense(ts CombatStats, to effectOptions) int {
	return max((to.AddDefense+ts.Defense)*50/100, 0)
}

// ravenDamage is FUN_0046c7b0: the raven min/max raised by a valid
// scepter in the right hand (magic power / 2 + level * 2, times the
// durability factor, in %), a DarkSpiritCriticalDamageRate roll for the
// maximum (effect 3) and a DarkSpiritExcellentDamageRate roll for 120% of
// the maximum (effect 2), minus the defense.
func (w *World) ravenDamage(o *Object, def int, effect *int) int {
	ps := o.P.Pet
	lo, hi := ps.DamageMin, ps.DamageMax
	if s := w.playerStats(o); s != nil {
		r := s.Items[0]
		if validIndex(r.Index) && s.Valid[0] && r.MagicPower != 0 && r.Index >= 0x400 && r.Index < 0x600 {
			pc := int(float32(int(uint32(r.MagicPower)>>1)+r.Level*2) * r.DurFactor)
			lo += pc * lo / 100
			hi += pc * hi / 100
		}
	}
	span := max(hi-lo, 1)
	dmg := lo + rnd(span)
	if rnd(100) < w.cfg.Int("Character", "DarkSpiritCriticalDamageRate", 0) {
		*effect = effectCritical
		dmg = hi
	}
	if rnd(100) < w.cfg.Int("Character", "DarkSpiritExcellentDamageRate", 0) {
		*effect = effectExcellent
		dmg = hi * 120 / 100
	}
	return max(dmg-def, 0)
}

// ravenAttack is FUN_0046b5a0 (message 0x33): the raven of a hits t.
// send is the message's first parameter (the miss packet asks for it),
// splash cuts the damage to 60%.
func (w *World) ravenAttack(a, t *Object, send, splash bool) bool {
	if a == t || !partyUsable(a) || (t.Type == TypePlayer && !partyUsable(t)) || a.Map != t.Map ||
		a.P == nil || a.P.Pet == nil {
		return false
	}
	if w.cellHas(a, gamedata.AttrSafeZone) || w.cellHas(t, gamedata.AttrSafeZone) {
		return false
	}
	if w.hooks.castleAttack != nil && !w.hooks.castleAttack(w, a, t) {
		return false
	}
	if wa := w.guildWarOf(guildNumberOf(a)); wa != nil && wa.active {
		// A battle soccer outside its START state swallows every attack;
		// a war ignores monsters (guild_soccer.go).
		if wa.typ == 1 && w.soccerState() != soccerStart {
			return true
		}
		if wa.typ == 0 && t.Type == TypeMonster {
			return false
		}
	}
	if t.Type == TypeNPC || !t.Alive || t.Live != LiveNormal || t.Teleporting {
		return false
	}
	if t.Type == TypeMonster {
		if t.Mon.Attribute == 6 || (t.Class > 99 && t.Class < 0x6F) || t.Class == 0x20B ||
			t.Class == 0xDD || t.Class == 0xDE || t.Mon.Owner == a.Index {
			return false
		}
	}
	if !w.pvpAllowed(a, t) {
		return false
	}
	w.ravenSetTarget(a, t)
	w.summonAssist(a, t)
	duel := w.inDuelWith(a, t)
	if duel {
		now := w.now()
		dl(a).Tick, dl(t).Tick = now, now
	}
	if a.Move != nil && a.Move.Authority == 1 {
		w.removeEffect(a, effInvisibility)
	}
	miss := hasEffectIndex(t, effImmunity34) || hasEffectIndex(t, effPhysiImmunity)
	if !miss && t.Type == TypeMonster {
		if t.Class == soccerBallClass {
			// The ball rolls 2 cells away from the owner (message 6 handled
			// at once, FUN_004b48f0 -> FUN_005114f0) and the hit misses.
			w.knockBack(t, a, 2)
			miss = true
		} else {
			miss = hasEffectIndex(t, effMonPhysiImmunity)
		}
	}
	as := w.ravenCombat(a)
	ts := w.combatStats(t)
	to := effOpt(t)
	if !miss {
		reduced, hit := ravenHitCheck(as.SuccessRate, ts.DefenseSuccessRate)
		if !hit {
			miss = true
		} else {
			return w.ravenHit(a, t, ts, to, reduced, splash, duel)
		}
	}
	_ = send // FUN_0040e090 with skill 0: no 0x19 either way
	w.sendDamage(a, t, 0, 0, false)
	return true
}

// ravenCombat returns the raven values of the owner.
func (w *World) ravenCombat(a *Object) *petState { return a.P.Pet }

// ravenHit is the damage part of FUN_0046b5a0 after a successful roll.
func (w *World) ravenHit(a, t *Object, ts CombatStats, to effectOptions, reduced, splash, duel bool) bool {
	effect := 0
	dmg := w.ravenDamage(a, ravenDefense(ts, to), &effect)
	if reduced {
		dmg = dmg * 30 / 100
	}
	if splash {
		dmg = dmg * 60 / 100
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
	// Minimum (level + obj+0x5e4) / 10, at least 1; no random part here.
	dmg = max(dmg, max(objLevel(a)/10, 1))
	w.ravenDurability(a, dmg)
	if validClass(a) {
		if t.Type != TypePlayer {
			general := w.cfg.Int("Character", "GeneralDamageRatePvM", 100)
			dmg = w.cfg.Int("Character", classKeys[a.Class]+"DamageRatePvM", 100) * (general * dmg / 100) / 100
		} else if validClass(t) {
			general := w.cfg.Int("Character", "GeneralDamageRatePvP", 100)
			cls := w.cfg.Int("Character", classKeys[a.Class]+"DamageRatePvP", 100)
			toCls := w.cfg.Int("Character", classKeys[a.Class]+"DamageRateTo"+classKeys[t.Class], 100)
			dmg = toCls * (cls * (general * dmg / 100) / 100) / 100
			arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
			switch {
			case duel:
				dmg = w.cfg.Int("Character", "DuelDamageRate", 100) * dmg / 100
			case a.Map == arena && t.Map == arena:
				dmg = w.cfg.Int("Character", "CustomArenaDamageRate", 100) * dmg / 100
			case a.Map > 0x11 && a.Map < 0x18 && t.Map > 0x11 && t.Map < 0x18:
				dmg = w.cfg.Int("Character", "ChaosCastleDamageRate", 100) * dmg / 100
			default:
				if w.hooks.castleDamageRate != nil {
					if r, ok := w.hooks.castleDamageRate(w, a, t); ok {
						dmg = r * dmg / 100
					}
				}
			}
		}
		if t.Type == TypeMonster && (t.Class == 0x115 || t.Class == 0x11B) {
			// Castle gate / guardian statue: Bless Potion +20%, then
			// CastleSiegeDamageRate3 (the obj+0x990 sum is not kept).
			if hasEffectIndex(a, effBlessPotion) {
				dmg += dmg * 20 / 100
			}
			dmg = w.cfg.Int("Character", "CastleSiegeDamageRate3", 100) * dmg / 100
		}
	}
	dmg = max(dmg, 0)
	sd := 0
	if t.Type == TypePlayer {
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
	if dmg > 0 && t.Type == TypePlayer && w.ravenSelfDefense(a, t, effect, duel) {
		w.addSelfDefense(a, t)
	}
	if a.Move != nil {
		a.Move.Flag270 = 0
	}
	if dmg < 1 {
		w.sendDamage(a, t, 0, effect, false)
		return true
	}
	w.removeEffect(t, effRemovedOnHit)
	stuck := false
	if t.Type == TypePlayer {
		ts := w.playerStats(t)
		if effect != 4 {
			if rnd(100) < ts.Opt109 {
				w.queueReflect(t, a, dmg)
			} else if r := ts.DamageReflect + to.DamageReflect; r > 0 {
				w.queueReflect(t, a, r*dmg/100)
			}
			w.refillRolls(t, [4]int{ts.Opt110, -1, ts.Opt111, 0})
		}
		if !hasPet(t) && validClass(t) && rnd(100) < w.cfg.Int("Character", classKeys[t.Class]+"DamageStuckRate", 0) {
			stuck = true
		}
	}
	w.lifeCheckEx(a, t, dmg-sd, effect, 0, stuck)
	return true
}

// ravenSelfDefense is the self defense test of FUN_0046b5a0 (player
// hits): none in a duel, between rival guilds or guilds at war, for a
// Castle Siege side during the battle, in the custom arena, in Chaos
// Castle or on a map without PK.
func (w *World) ravenSelfDefense(a, t *Object, effect int, duel bool) bool {
	if a == t || effect == 4 || duel {
		return false
	}
	if kn, vn := guildNumberOf(a), guildNumberOf(t); kn != 0 && vn != 0 && w.guilds != nil &&
		w.guildMgr().relation(kn, vn) == 2 {
		return false
	}
	if w.atWar(a, t) {
		return false
	}
	if a.Map == csMap && w.hooks.castleBattle != nil && w.hooks.castleBattle(w) && csSide(a) != 0 {
		return false
	}
	arena := w.cfg.Int("Custom", "CustomArenaMapNumber", -1)
	if a.Map == arena && t.Map == arena {
		return false
	}
	if a.Map > 0x11 && a.Map < 0x18 && t.Map > 0x11 && t.Map < 0x18 {
		return false
	}
	if info, ok := w.data.Maps[a.Map]; ok && info.NonPK == 2 {
		return false
	}
	return true
}

// ravenDurability is FUN_0046c440: PetDamageTable.txt row 5 scales the
// damage (rate / 1000), 2% of it fills the counter CItem+0x28 and a full
// counter (PetDurabilityRate * 800 / 100 * obj+0x91c / 100, obj+0x91c
// being always 100) costs one durability point: C1 06 2A 01 dur 00 and the
// pet info A9 (raven, inventory, slot 1).
func (w *World) ravenDurability(o *Object, dmg int) {
	it := &o.P.Inventory[1]
	if it.Index != idxDarkRaven {
		return
	}
	ps := o.P.Pet
	dmg = w.petDamageRate(o, 5, dmg)
	ps.DurCount = int(uint16(ps.DurCount + dmg*2/100))
	limit := w.cfg.Int("Common", "PetDurabilityRate", 0) * 800 / 100 * 100 / 100
	if ps.DurCount < limit {
		return
	}
	before := w.ravenDurFactor(o)
	d := it.Durability - 1
	if d < 1 {
		d = 0
	}
	it.Durability = d
	ps.DurCount = 0
	if w.ravenDurFactor(o) != before && w.hooks.recalc != nil {
		w.hooks.recalc(w, o) // FUN_0048d990 -> CalcCharacter
	}
	w.send(o, packetItemDurability(1, truncDur(it.Durability), 0))
	w.send(o, packetPetInfo(0, 0, 1, it.PetLevel, it.PetExp))
}

// ravenDurFactor is CItem+0xac of the raven (FUN_0048d990 recomputes it).
func (w *World) ravenDurFactor(o *Object) float32 {
	it := o.P.Inventory[1]
	if w.statsData == nil {
		return 1
	}
	info, ok := w.data.Items[it.Index]
	if !ok {
		return 1
	}
	return w.statsData.convertItem(info, it, it.PetLevel).DurFactor
}

// petDamageRow is a Util/PetDamageTable.txt row (CPetDamageTable).
type petDamageRow struct {
	Index, MinLevel, MaxLevel, MinReset, MaxReset, Rate int
}

// petDamageRate is FUN_004d6360: the first row of the pet index whose
// level / reset range holds the owner gives dmg * rate / 1000.
func (w *World) petDamageRate(o *Object, index, dmg int) int {
	any := func(v int) bool { return v == -1 }
	lvl, rs := objLevel(o), resets(o)
	for _, r := range w.q().petDamage {
		if r.Index != index || (!any(r.MinLevel) && lvl < r.MinLevel) || (!any(r.MaxLevel) && lvl > r.MaxLevel) ||
			(!any(r.MinReset) && rs < r.MinReset) || (!any(r.MaxReset) && rs > r.MaxReset) {
			continue
		}
		return r.Rate * dmg / 1000
	}
	return dmg
}

// loadPetDamageTable reads the PetDamageTable table of config/data/util.toml.
func loadPetDamageTable(configDir string) ([]petDamageRow, error) {
	rows, err := gamedata.LoadPetDamageTable(configDir)
	if err != nil {
		return nil, err
	}
	var out []petDamageRow
	for _, r := range rows {
		out = append(out, petDamageRow{r.Pet, r.MinLevel, r.MaxLevel, r.MinReset, r.MaxReset, r.DamageRate})
	}
	return out, nil
}

func registerPetRaven(h *hooks) {
	h.start = append(h.start, func(w *World) error {
		if w.configDir == "" {
			return nil
		}
		rows, err := loadPetDamageTable(w.configDir)
		if err != nil {
			return err
		}
		w.q().petDamage = rows
		return nil
	})
	h.tick100 = append(h.tick100, func(w *World) {
		for i := PlayerStart; i < MaxObjects; i++ {
			if o := w.objects[i]; o.State == StatePlaying && o.P != nil && o.P.Pet != nil {
				w.ravenTick(o)
			}
		}
	})
	h.beforeGateMove = append(h.beforeGateMove, func(w *World, o *Object) { w.ravenReset(o) })
}
