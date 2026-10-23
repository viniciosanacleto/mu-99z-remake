package game

import (
	"math"
	"strings"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/frame"
)

// CustomAttack (CCustomAttack, getter FUN_00434630; docs/server-plan/
// 10e-customs-wizardteam.md): a server-side auto attack. It starts from
// the client helper (F3 0A with the CommandManager checks of index 52,
// FUN_00447380), from the manual skill choice (F3 0B, FUN_00447800) or
// from index 52 itself (FUN_00445ab0, unreachable from the chat because
// the hard-coded /attack comes first). /offattack (index 53, FUN_00445d90)
// keeps it running without the client. Every 100 ms (timer 0,
// FUN_004c48b0 -> FUN_00444b10) the bot drinks potions, repairs, reloads
// arrows, casts the buffs and attacks the nearest monster by feeding the
// skill handlers (19 / 1E / 10) the packets the client would send.

// attackState is obj+0x124 (manual mode) and obj+0x20d8..0x20f4.
type attackState struct {
	Manual     bool      // obj+0x124 (F3 0B)
	Active     bool      // obj+0x20d8
	Offline    bool      // obj+0x20d9
	Buff       int       // obj+0x20dc: CustomAttackBuffEnable_AL copy (0 = no buffs)
	Skill      int       // obj+0x20e0
	Map        int       // obj+0x20e2
	X, Y       int       // obj+0x20e3 / 0x20e4
	NextCast   time.Time // obj+0x20e8
	NextPotion time.Time // obj+0x20ec
	Start      time.Time // obj+0x20f0 (time64), obj+0x20f0 for the offline start too
	Target     int       // obj+0x20f4 (-1 none)
}

func init() {
	hardCodedHandlers["/attack"] = (*World).commandAttackHardCoded
	commandHandlers[52] = (*World).commandAttack
	commandHandlers[53] = (*World).commandOffAttack
	f3Handlers[0x0A] = (*World).handleAttackHelper
	f3Handlers[0x0B] = (*World).handleAttackManual
	worldHandlers[0x04] = func(w *World, o *Object, p []byte) { w.attackStop(o) }
	tick100Hooks = append(tick100Hooks, (*World).attackTick)
}

func attackOf(o *Object) *attackState { return &custom(o).Attack }

// attackActive reports whether the auto attack runs (obj+0x20d8).
func attackActive(o *Object) bool { return o.P != nil && o.P.cust != nil && o.P.cust.Attack.Active }

// packetAttackStatus is C1 08 F3 EB active u32 (FUN_00445940), sent to a
// playing client.
func packetAttackStatus(active bool) []byte {
	p := newPacket(frame.C1, 0xF3, 0xEB, 8)
	if active {
		p[4] = 1
	}
	return p
}

// packetAttackHelperOpen is the C1 04 F3 F6 the hard-coded /attack sends
// when the auto attack is off (the client answers with F3 0A or F3 0B).
func packetAttackHelperOpen() []byte { return newPacket(frame.C1, 0xF3, 0xF6, 4) }

func (w *World) sendAttackStatus(o *Object) {
	if o.State == StatePlaying && o.IsPlayer() && o.P.LogoutCountdown < 1 {
		w.send(o, packetAttackStatus(attackActive(o)))
	}
}

// commandAttackHardCoded is the /attack of FUN_00435400: any argument
// other than "off" while the auto attack is off sends F3 F6; otherwise the
// auto attack stops (FUN_00445a20). No CommandManager checks.
func (w *World) commandAttackHardCoded(o *Object, arg string) {
	if !strings.EqualFold(arg, "off") && !attackActive(o) {
		w.send(o, packetAttackHelperOpen())
		return
	}
	w.attackStop(o)
}

// attackStop is FUN_00445a20 (also head 04): clears the manual mode and,
// for a playing character with the attack on, message 0x23C (C1 86 F3 E4
// type 1 color 2), off and the F3 EB status.
func (w *World) attackStop(o *Object) {
	if o.P == nil {
		return
	}
	a := attackOf(o)
	a.Manual = false
	if o.State == StatePlaying && o.IsPlayer() && o.P.LogoutCountdown < 1 && a.Active {
		w.customNoticeMsg(o, 1, 2, 0x23C)
		a.Active = false
		w.sendAttackStatus(o)
	}
}

// attackZoneOK is the CustomAttackMapZone test.
func (w *World) attackZoneOK(o *Object) bool { return w.mapZone("CustomAttackMapZone", o.Map) }

// attackSkillOK is FUN_004458a0 (CustomAttack.txt row with a non-buff
// group allowed for the class) followed by FUN_00446b70 (the player has
// the skill).
func (w *World) attackSkillOK(o *Object, num int) (gamedata.AttackSkill, bool) {
	sk, ok := w.cw().attack.Skills[num]
	if !ok || sk.Group == 0 || o.Class < 0 || o.Class >= 5 || sk.Class[o.Class] == 0 {
		return sk, false
	}
	return sk, w.findSkill(o, num) != nil
}

// attackWeaponOK is the FUN_004f8800 weapon test of FUN_00445ab0 (and of
// FUN_00447380 with helper set): weapon skills need their weapon, skills
// 0x29..0x2B (only 0x29 for the helper) a right hand item, and for index
// 52 Triple Shot / skills 0x33, 0x34 a bow in each hand.
func (w *World) attackWeaponOK(o *Object, num int, helper bool) bool {
	if !w.weaponSkillOK(o, num) {
		return false
	}
	if helper {
		return num != 0x29 || !o.P.Inventory[0].IsEmpty()
	}
	if (num == 0x29 || num == 0x2A || num == 0x2B) && o.P.Inventory[0].IsEmpty() {
		return false
	}
	if num == 0x18 || num == 0x33 || num == 0x34 {
		r, l := o.P.Inventory[0], o.P.Inventory[1]
		if r.IsEmpty() || l.IsEmpty() || r.Index < 0x800 || r.Index > 0x9FF || l.Index < 0x800 || l.Index > 0x9FF {
			return false
		}
	}
	return true
}

// attackBegin stores the start of an attack (shared by FUN_00445ab0 and
// FUN_00447380).
func (w *World) attackBegin(o *Object) {
	a := attackOf(o)
	now := w.now()
	a.Active = true
	a.Map, a.X, a.Y = o.Map, o.X, o.Y
	a.NextCast, a.NextPotion, a.Start = now, now, now
	a.Target = -1
}

// commandAttack is index 52 (FUN_00445ab0): CustomAttackSwitch; obj+0x20f8
// (never set, U-CUST-17) gives 0x23D; outside CustomAttackMapZone or in a
// safe zone 0x23E; with the attack on it stops silently. "auto" (empty in
// this server) takes the buff flag of CustomAttackBuffEnable_AL and the
// first CustomAttack.txt attack skill the player has; otherwise the
// arguments are the buff flag and the skill. A skill outside the table, not
// learned or without its weapon gives 0x23F.
func (w *World) commandAttack(o *Object, c command, arg string) {
	if w.cfg.Int("Custom", "CustomAttackSwitch", 0) == 0 {
		return
	}
	if !w.attackZoneOK(o) || w.inSafeZone(o) {
		w.customNoticeMsg(o, 1, 2, 0x23E)
		return
	}
	a := attackOf(o)
	if a.Active {
		a.Active = false
		w.sendAttackStatus(o)
		return
	}
	if arg == "" {
		a.Buff = w.cfg.ALInt("Custom", "CustomAttackBuffEnable", accountLevel(o), 0)
		a.Skill = w.firstAttackSkill(o)
	} else {
		a.Buff = cmdInt(arg, 0)
		a.Skill = cmdInt(arg, 1)
	}
	if a.Buff == 0 && a.Skill == 0 {
		w.customNoticeMsg(o, 1, 2, 0x23F)
		return
	}
	if a.Skill != 0 {
		if _, ok := w.attackSkillOK(o, a.Skill); !ok || !w.attackWeaponOK(o, a.Skill, false) {
			w.customNoticeMsg(o, 1, 2, 0x23F)
			return
		}
	}
	w.attackBegin(o)
	w.sendAttackStatus(o)
}

// firstAttackSkill is FUN_004458e0: the first table skill (ascending
// index) with a non-buff group allowed for the class that the player has.
func (w *World) firstAttackSkill(o *Object) int {
	t := w.cw().attack
	for _, n := range t.Order {
		if _, ok := w.attackSkillOK(o, n); ok {
			return n
		}
	}
	return 0
}

// handleAttackHelper is F3 0A [4..5] skill (FUN_00447380): the client
// helper start. CustomAttackSwitch, obj+0x20f8 (0x23D), the map zone and
// no safe zone, then every CommandManager check of index 52 (delay, GM,
// level, reset, zen and coins, messages 300 / 0x12D..0x135). With the
// attack off: the buff flag of CustomAttackBuffEnable_AL and the skill
// (0x23F as in index 52, Twisting Slash also needs a right hand item);
// the start is charged as index 52. With the attack on it stops. Both
// send F3 EB.
func (w *World) handleAttackHelper(o *Object, p []byte) {
	if len(p) < 6 || o.P.LogoutCountdown > 0 || w.cfg.Int("Custom", "CustomAttackSwitch", 0) == 0 {
		return
	}
	if !w.attackZoneOK(o) || w.inSafeZone(o) {
		return
	}
	c, ok := w.commandRow(52)
	if !ok {
		return
	}
	if !w.commandChecks(o, c) {
		return
	}
	a := attackOf(o)
	if a.Active {
		a.Active = false
		w.sendAttackStatus(o)
		return
	}
	a.Buff = w.cfg.ALInt("Custom", "CustomAttackBuffEnable", accountLevel(o), 0)
	a.Skill = int(int16(uint16(p[4]) | uint16(p[5])<<8))
	if a.Skill != 0 {
		if _, ok := w.attackSkillOK(o, a.Skill); !ok || !w.attackWeaponOK(o, a.Skill, true) {
			w.customNoticeMsg(o, 1, 2, 0x23F)
			return
		}
	} else if a.Buff == 0 {
		w.customNoticeMsg(o, 1, 2, 0x23F)
		return
	}
	w.attackBegin(o)
	w.commandFinish(o, c)
	w.sendAttackStatus(o)
}

// commandRow is FUN_00435340: the CommandManager row of an index.
func (w *World) commandRow(index int) (command, bool) {
	if w.moveData != nil {
		for _, c := range w.moveData.commands {
			if c.Index == index {
				return c, true
			}
		}
	}
	return command{}, false
}

// commandChecks repeats the CommandManager tests of FUN_00435400 for a
// command started outside the chat (switch, delay, GM level, level and
// reset ranges, zen). It sends the same messages.
func (w *World) commandChecks(o *Object, c command) bool {
	al := o.P.Account.Level
	if al < 0 || al > 3 || c.Switch[al] == 0 {
		return false
	}
	m := mv(o)
	if m.CommandTicks == nil {
		m.CommandTicks = map[int]time.Time{}
	}
	ch := o.P.Char
	elapsed := int(w.now().Sub(m.CommandTicks[c.Index]) / time.Second)
	var msgID, val int
	switch {
	case elapsed < c.Delay:
		msgID, val = 300, c.Delay-elapsed
	case c.GMLevel != -1 && w.gmLevel(o) < c.GMLevel:
		w.systemMessage(o, w.messageFor(o, 0x12D, c.Syntax))
		return false
	case c.MinLevel != -1 && ch.Level < c.MinLevel:
		msgID, val = 0x12E, c.MinLevel
	case c.MaxLevel != -1 && ch.Level > c.MaxLevel:
		msgID, val = 0x12F, c.MaxLevel
	case c.MinReset != -1 && ch.Resets < c.MinReset:
		msgID, val = 0x130, c.MinReset
	case c.MaxReset != -1 && ch.Resets > c.MaxReset:
		msgID, val = 0x131, c.MaxReset
	case ch.Money < int64(c.Money[al]):
		msgID, val = 0x132, c.Money[al]
	}
	if msgID != 0 {
		w.systemMessage(o, w.messageFor(o, msgID, val, c.Syntax))
		return false
	}
	return true
}

// handleAttackManual is F3 0B skill u32 (size 8, FUN_00447800): -1 or an
// active attack stops it. Otherwise, without the offline attack, an open
// interface, death or teleport: outside a safe zone ("Use /attack fora da
// cidade, perto dos monstros.") with a skill 1..213 of SkillList that the
// player has, the manual attack starts ("Attack: %s. Use /attack off para
// parar."); a bad skill gives "A magia selecionada nao esta na lista do
// personagem...". Not charged.
func (w *World) handleAttackManual(o *Object, p []byte) {
	if len(p) != 8 || o.P.LogoutCountdown > 0 {
		return
	}
	skill := int(int32(uint32(p[4]) | uint32(p[5])<<8 | uint32(p[6])<<16 | uint32(p[7])<<24))
	a := attackOf(o)
	if skill == -1 || a.Active {
		w.attackStop(o)
		return
	}
	if a.Offline || ifBusy(o) || !o.Alive || w.dying(o) || o.Teleporting {
		return
	}
	if w.inSafeZone(o) {
		w.customNotice(o, 1, 2, "Use /attack fora da cidade, perto dos monstros.")
		return
	}
	info, ok := w.data.Skills[skill]
	if skill < 1 || skill > 0xD5 || !ok || w.findSkill(o, skill) == nil {
		w.customNotice(o, 1, 2, "A magia selecionada nao esta na lista do personagem. Selecione uma habilidade disponivel.")
		return
	}
	a.Manual = true
	a.Buff, a.Skill = 0, skill
	w.attackBegin(o)
	w.sendAttackStatus(o)
	w.customNotice(o, 1, 0, "Attack: %s. Use /attack off para parar.", info.Name)
}

// commandOffAttack is index 53 (FUN_00445d90): the manual attack refuses
// ("Use /attack off antes de ativar o modo offline."); with
// CustomAttackOfflineSwitch the attack must be on (0x241). The automatic
// reset turns off when CommandResetMove_AL is set, the client is closed
// and the attack continues offline (obj+0x20d9, start time reset). Not
// charged.
func (w *World) commandOffAttack(o *Object, c command, arg string) {
	a := attackOf(o)
	if a.Manual {
		w.customNotice(o, 1, 2, "Use /attack off antes de ativar o modo offline.")
		return
	}
	if w.cfg.Int("Custom", "CustomAttackOfflineSwitch", 0) == 0 || detached(o) {
		return
	}
	if !a.Active {
		w.customNoticeMsg(o, 1, 2, 0x241)
		return
	}
	if w.cfg.ALInt("Command", "CommandResetMove", accountLevel(o), 0) != 0 && cmdOf(o).AutoReset {
		cmdOf(o).AutoReset = false
	}
	a.Offline = true
	a.Start = w.now()
	w.reconnectSave(o) // FUN_004e1350 (reconnect.go)
	w.detachClient(o)
	w.log.Info("[CustomAttack] Offline attack", "account", o.P.Account.Login, "name", o.P.Char.Name)
}

// customAttackExpRate is the CustomAttack factor of FUN_004c5a30 /
// FUN_004c5ea0: with CustomAttackSwitch and the attack on,
// CustomAttackExperienceRate_AL (online) or
// CustomAttackOfflineExperienceRate_AL (offline); 100 otherwise.
func (w *World) customAttackExpRate(o *Object) int {
	if w.cfg.Int("Custom", "CustomAttackSwitch", 0) == 0 || !attackActive(o) {
		return 100
	}
	if o.P.cust.Attack.Offline {
		return w.cfg.ALInt("Custom", "CustomAttackOfflineExperienceRate", accountLevel(o), 0)
	}
	return w.cfg.ALInt("Custom", "CustomAttackExperienceRate", accountLevel(o), 0)
}

// --- the 100 ms bot ------------------------------------------------------------------

// attackTick runs FUN_00444b10 for every playing player.
func (w *World) attackTick() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.P.cust == nil {
			continue
		}
		w.attackProc(o)
	}
}

// nearPos is FUN_00513bf0: same map and |dx|, |dy| <= r.
func nearPos(o *Object, mp, x, y, r int) bool {
	return o.Map == mp && x-r <= o.X && o.X <= x+r && y-r <= o.Y && o.Y <= y+r
}

// attackProc is FUN_00444b10.
func (w *World) attackProc(o *Object) {
	a := attackOf(o)
	if a.Manual {
		w.attackManualProc(o)
		return
	}
	if !a.Active {
		return
	}
	al := accountLevel(o)
	now := w.now()
	if !a.Offline {
		if a.Buff == 0 && a.Skill == 0 {
			w.attackStop(o)
			return
		}
		if lim := w.cfg.ALInt("Custom", "CustomAttackMaxTimeLimit", al, 0); lim > 0 && !now.Before(a.Start.Add(time.Duration(lim)*time.Second)) {
			w.customNoticeMsg(o, 1, 2, 0x242)
			a.Active = false
			w.sendAttackStatus(o)
			return
		}
		if !w.attackZoneOK(o) || w.inSafeZone(o) || !nearPos(o, a.Map, a.X, a.Y, 5) {
			w.attackStop(o)
			return
		}
		w.attackActions(o)
		return
	}
	if a.Buff == 0 && a.Skill == 0 {
		w.removeOffline(o)
		return
	}
	if lim := w.cfg.ALInt("Custom", "CustomAttackOfflineMaxTimeLimit", al, 0); lim >= 1 && !now.Before(a.Start.Add(time.Duration(lim)*time.Second)) {
		w.removeOffline(o)
		return
	}
	if w.cfg.ALInt("Custom", "CustomAttackOfflineKeepEnable", al, 0) != 0 {
		w.attackActions(o)
		return
	}
	// The short obj+0x5d2 (U-CUST-05) would add a 10-cell range test from
	// the start position; it is never set here.
	if w.attackZoneOK(o) && !w.inSafeZone(o) {
		w.attackActions(o)
		return
	}
	w.removeOffline(o)
}

// attackActions is the tail of FUN_00444b10: potions (FUN_00445e60),
// repair (FUN_004461e0), arrows (FUN_00446350), buffs (FUN_00446560) and
// the attack (FUN_00446890).
func (w *World) attackActions(o *Object) {
	w.attackPotions(o)
	w.attackRepair(o)
	w.attackArrows(o)
	w.attackBuffs(o)
	w.attackSkillCast(o)
}

func lifePercent(o *Object) float32 {
	top := float32(o.P.AddLife) + o.P.Char.MaxLife
	if top <= 0 {
		return 100
	}
	return o.P.Char.Life * 100 / top
}

func manaPercent(o *Object) float32 {
	top := float32(o.P.AddMana) + o.P.Char.MaxMana
	if top <= 0 {
		return 100
	}
	return o.P.Char.Mana * 100 / top
}

// firstItemSlot is FUN_00494880: the first slot 0..75 holding index.
func firstItemSlot(o *Object, index int) int {
	for s := 0; s < BagEnd; s++ {
		if it := o.P.Inventory[s]; !it.IsEmpty() && it.Index == index {
			return s
		}
	}
	return 0xFF
}

// attackPotions is FUN_00445e60: with CustomAttackPotion_AL and the
// potion delay over, ten rounds: life below CustomAttackPotionRate1 drinks
// (14,3), (14,2), (14,1) or the apple (14,0); mana below Rate2 (14,6),
// (14,5), (14,4). As in the original the slot found first is reused for
// the rest of the rounds (and for the mana); every use pushes the next
// potion time by CustomAttackPotionDelay_AL.
func (w *World) attackPotions(o *Object) {
	al := accountLevel(o)
	a := attackOf(o)
	if w.cfg.ALInt("Custom", "CustomAttackPotion", al, 0) == 0 || w.now().Before(a.NextPotion) {
		return
	}
	delay := time.Duration(w.cfg.ALInt("Custom", "CustomAttackPotionDelay", al, 0)) * time.Millisecond
	rate1 := float32(w.cfg.Int("Custom", "CustomAttackPotionRate1", 0))
	rate2 := float32(w.cfg.Int("Custom", "CustomAttackPotionRate2", 0))
	slot := 0xFF
	use := func() {
		if slot < 0x4C {
			w.handleUseItem(o, []byte{0xC1, 0x06, 0x26, byte(slot), 0xFF, 0x00})
			a.NextPotion = w.now().Add(delay)
		}
	}
	for i := 0; i < 10; i++ {
		if lifePercent(o) < rate1 {
			for _, idx := range []int{14*512 + 3, 14*512 + 2, 14*512 + 1, 14 * 512} {
				if slot != 0xFF {
					break
				}
				slot = firstItemSlot(o, idx)
			}
			use()
		}
		if manaPercent(o) < rate2 {
			for _, idx := range []int{14*512 + 6, 14*512 + 5, 14*512 + 4} {
				if slot != 0xFF {
					break
				}
				slot = firstItemSlot(o, idx)
			}
			use()
		}
	}
}

// attackRepair is FUN_004461e0: with CustomAttackRepair_AL every
// equipped item below its durability is repaired and paid (FUN_00497260),
// with the C1 08 34 money reply and CalcCharacter.
func (w *World) attackRepair(o *Object) {
	if w.cfg.ALInt("Custom", "CustomAttackRepair", accountLevel(o), 0) == 0 {
		return
	}
	for s := 0; s < EquipSlots; s++ {
		it := o.P.Inventory[s]
		if it.IsEmpty() || it.Durability >= float32(w.itemMaxDurability(it)) {
			continue
		}
		if w.repairItem(o, s) {
			w.send(o, packetRepairResult(uint32(o.P.Char.Money)))
			if recalcHook != nil {
				recalcHook(w, o)
			}
		}
	}
}

// attackArrows is FUN_00446350: without the effects 6 and 0x8F, a
// crossbow in the right hand reloads bolts (4,7) into the left hand and a
// bow in the left hand reloads arrows (4,15) into the right hand, through
// the item move handler (24). The CItem+8 hand test is not identified
// (U-CUST-18).
func (w *World) attackArrows(o *Object) {
	if hasEffectIndex(o, 6) || hasEffectIndex(o, 0x8F) {
		return
	}
	const bolts, arrows = 4*512 + 7, 4*512 + 15
	r, l := o.P.Inventory[0], o.P.Inventory[1]
	bow := func(idx int) bool { return idx >= 0x800 && idx < 0xA00 }
	reload := func(ammo, dst int) {
		if w.countItems(o, ammo, -1) <= 0 {
			return
		}
		if s := firstItemSlot(o, ammo); s < 0x4C {
			w.handleItemMove(o, []byte{0xC1, 0x0E, 0x24, 0, byte(s), 0, 0, 0, 0, 0, 0, 0, 0, byte(dst)})
		}
	}
	if !r.IsEmpty() && bow(r.Index) && r.Index != arrows && (l.Index != bolts || l.Durability < 1) {
		reload(bolts, 1)
	}
	l = o.P.Inventory[1]
	r = o.P.Inventory[0]
	if !l.IsEmpty() && bow(l.Index) && l.Index != bolts && (r.Index != arrows || r.Durability < 1) {
		reload(arrows, 0)
	}
}

// attackBuffs is FUN_00446560: with CustomAttackBuffEnable_AL (or the
// offline one) and the buff flag, every group 0 skill of the table that
// the class may use and the player has is cast when its mana and BP are
// there: Value 0 on the player, otherwise also on each party member alive
// on the map within the skill range, whenever the target lacks the
// skill's effect. No buff skill at all clears the flag.
func (w *World) attackBuffs(o *Object) {
	a := attackOf(o)
	al := accountLevel(o)
	enable := w.cfg.ALInt("Custom", "CustomAttackBuffEnable", al, 0)
	if a.Offline {
		enable = w.cfg.ALInt("Custom", "CustomAttackOfflineBuffEnable", al, 0)
	}
	if enable == 0 || a.Buff == 0 {
		return
	}
	t := w.cw().attack
	n := 0
	for _, num := range t.Order {
		sk := t.Skills[num]
		if sk.Group != 0 || o.Class < 0 || o.Class >= 5 || sk.Class[o.Class] == 0 {
			continue
		}
		ref := w.findSkill(o, num)
		if ref == nil {
			continue
		}
		n++
		if !w.manaOK(o, num) || !w.bpOK(o, num) {
			continue
		}
		eff := w.skillEffectIndex(num)
		pt := w.partyOf(o)
		if sk.Value == 0 || pt == nil {
			if !hasEffectIndex(o, eff) {
				w.attackCast(o, o, sk, ref)
			}
			continue
		}
		rng := 0
		if info, ok := w.data.Skills[num]; ok {
			rng = info.Range
		}
		for _, idx := range pt.members {
			if idx < 0 || idx >= MaxObjects {
				continue
			}
			m := w.objects[idx]
			if !m.Alive || m.Live != LiveNormal || m.Map != o.Map || objDistance(o, m) > rng {
				continue
			}
			if !hasEffectIndex(m, eff) {
				w.attackCast(o, m, sk, ref)
			}
		}
	}
	if n == 0 {
		a.Buff = 0
	}
}

// attackSkillCast is FUN_00446890: the attack skill must still be in the
// table (non-buff, class allowed) and learned, otherwise it is cleared;
// when the cast delay is over and mana and BP are there, the nearest
// monster in range (FUN_004469a0) is attacked.
func (w *World) attackSkillCast(o *Object) {
	a := attackOf(o)
	if a.Skill == 0 {
		return
	}
	sk, ok := w.attackSkillOK(o, a.Skill)
	if !ok {
		a.Skill = 0
		return
	}
	if w.now().Before(a.NextCast) || !w.manaOK(o, a.Skill) || !w.bpOK(o, a.Skill) {
		return
	}
	if w.attackTarget(o, a.Skill) {
		w.attackCast(o, w.objects[a.Target], sk, w.findSkill(o, a.Skill))
	}
}

// attackManualProc is FUN_00447a40: the manual attack stops when the
// player dies, is dying or teleporting, opens an interface, changes map,
// enters a safe zone or loses the skill. Otherwise the nearest target is
// hit with the skill (group from the table, 1 if missing) and the next
// cast waits the skill delay (or the speed formula), at least 150 ms.
func (w *World) attackManualProc(o *Object) {
	a := attackOf(o)
	if !a.Active {
		a.Manual = false
		return
	}
	ref := w.findSkill(o, a.Skill)
	if !o.Alive || w.dying(o) || o.Teleporting || ifBusy(o) || o.Map != a.Map || w.inSafeZone(o) || ref == nil {
		w.attackStop(o)
		return
	}
	if w.now().Before(a.NextCast) || !w.manaOK(o, a.Skill) || !w.bpOK(o, a.Skill) || !w.attackTarget(o, a.Skill) {
		return
	}
	sk, ok := w.cw().attack.Skills[a.Skill]
	if !ok || sk.Group <= 0 {
		sk = gamedata.AttackSkill{Index: a.Skill, Group: 1}
	}
	w.attackCast(o, w.objects[a.Target], sk, ref)
	d := w.skillDelay(a.Skill)
	if d < 1 {
		d = w.attackSpeedDelay(o, a.Skill)
	}
	a.NextCast = w.now().Add(time.Duration(max(d, 150)) * time.Millisecond)
}

// attackTarget is FUN_004469a0: the nearest attackable monster of VP2
// (distance below 100) inside the skill range or radio; the index goes to
// obj+0x20f4.
func (w *World) attackTarget(o *Object, num int) bool {
	best := 100
	a := attackOf(o)
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Type != byte(TypeMonster) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		if !w.skillTargetOK(o, e.Index, byte(TypeMonster), -1) {
			continue
		}
		t := w.objects[e.Index]
		d := objDistance(o, t)
		if d >= best {
			continue
		}
		if w.rangeOK(o, num, o.X, o.Y, t.X, t.Y) || w.radioOK(num, o.X, o.Y, t.X, t.Y) {
			a.Target, best = e.Index, d
		}
	}
	return best != 100
}

// skillDelay is FUN_004f7b40 (SkillList Delay).
func (w *World) skillDelay(num int) int {
	if info, ok := w.data.Skills[num]; ok {
		return info.Delay
	}
	return 0
}

// attackSpeedDelay is FUN_004457f0 with the player's dexterity and
// attack speed (magic speed for classes 0 and 5).
func (w *World) attackSpeedDelay(o *Object, num int) int {
	s := w.statsOf(o)
	speed := s.AttackSpeed
	if o.Class == 0 || o.Class == 5 {
		speed = s.MagicSpeed
	}
	return w.cw().attack.AttackDelay(num, o.P.Char.Dexterity, speed)
}

// attackCast is FUN_00446c10: Heal (0x1A) only below half life of the
// target; when the cast delay is over and mana and BP are there the
// packet of the group is fed to the skill handlers (2: 1E + 10, 3: 1E,
// 4 and the rest: 19) and the next cast waits the skill delay or the speed
// formula.
func (w *World) attackCast(o, t *Object, sk gamedata.AttackSkill, ref *skillRef) {
	if t == nil || ref == nil {
		return
	}
	if ref.Num == 0x1A && t.P != nil && lifePercent(t) > 50 {
		return
	}
	a := attackOf(o)
	if w.now().Before(a.NextCast) || !w.manaOK(o, ref.Num) || !w.bpOK(o, ref.Num) {
		return
	}
	switch sk.Group {
	case 2:
		w.attackAreaMulti(o, t, ref.Num)
	case 3:
		w.attackArea(o, t, ref.Num)
	default:
		w.handleSkillTarget(o, []byte{0xC1, 0x07, 0x19, byte(ref.Num), byte(t.Index >> 8), byte(t.Index), 0})
	}
	d := w.skillDelay(ref.Num)
	if d == 0 {
		d = w.attackSpeedDelay(o, ref.Num)
	}
	a.NextCast = w.now().Add(time.Duration(d) * time.Millisecond)
}

// attackAngle is the atan2 angle of FUN_00447070 in 1/255 turns.
func attackAngle(x1, y1, x2, y2 int) byte {
	deg := int(math.Atan2(float64(y2-y1), float64(x2-x1)) * 180 / math.Pi)
	if deg < 0 {
		deg += 360
	}
	return byte(deg * 0xFF / 360)
}

// attackArea is FUN_00447070: C1 0C 1E skill tx ty angle 0 angle
// target(BE) 0 (the two atan2 inputs are in registers, U-CUST-19).
func (w *World) attackArea(o, t *Object, num int) {
	ang := attackAngle(o.X, o.Y, t.X, t.Y)
	w.handleSkillArea(o, []byte{0xC1, 0x0C, 0x1E, byte(num), byte(t.X), byte(t.Y), ang, 0, ang,
		byte(t.Index >> 8), byte(t.Index), 0})
}

// attackAreaMulti is FUN_00446df0: the 1E cast at the target followed by
// a 10 packet with up to ten attackable monsters of VP2 inside the radio.
func (w *World) attackAreaMulti(o, t *Object, num int) {
	w.attackArea(o, t, num)
	p := []byte{0xC1, 0x08, 0x10, byte(num), byte(o.X), byte(o.Y), 0, 0}
	n := 0
	for _, e := range o.VP2 {
		if e.State == VPEmpty || e.Type != byte(TypeMonster) || e.Index < 0 || e.Index >= MaxObjects {
			continue
		}
		if !w.skillTargetOK(o, e.Index, byte(TypeMonster), t.Index) {
			continue
		}
		m := w.objects[e.Index]
		if !w.radioOK(num, o.X, o.Y, m.X, m.Y) {
			continue
		}
		p = append(p, byte(e.Index>>8), byte(e.Index), 0)
		p[1] += 3
		n++
		p[7] = byte(n)
		if n > 9 {
			break
		}
	}
	w.handleSkillMulti(o, p)
}
