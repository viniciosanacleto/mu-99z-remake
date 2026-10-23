package game

import "strings"

// WizardTeam player commands of FUN_00435400 (CommandManager indexes 5..17):
// stat points, PK clear, zen, inventory clear and stat redistribution.

func init() {
	for i := 0; i < 5; i++ {
		typ := i
		commandHandlers[5+i] = func(w *World, o *Object, c command, arg string) { w.commandAddPoint(o, c, arg, typ) }
	}
	commandHandlers[10] = (*World).commandPKClear
	commandHandlers[11] = (*World).commandMoney
	commandHandlers[16] = (*World).commandClearInventory
	commandHandlers[17] = (*World).commandRedistribute
	tick1000Hooks = append(tick1000Hooks, (*World).autoPointSecond)
}

// autoPointState is obj+0x63c (types with an amount) and obj+0x640..0x650
// (points left to spread per stat).
type autoPointState struct {
	Count  int
	Amount [5]int
}

// cAtoi is the C atoi FUN_00435210 applies to a command token: optional
// spaces and sign, then digits up to the first other character.
func cAtoi(s string) int {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}

// cmdInt is FUN_00435210: atoi of argument token n.
func cmdInt(arg string, n int) int { return cAtoi(commandToken(arg, n, 0x3C)) }

// commandAddPoint is /f /a /v /e /c (FUN_004375e0, type 0..4): Command
// (leadership) only for the Dark Lord (message 0x144); "auto" stores the
// amount spread at every level up (FUN_004377c0); otherwise 0 < amount <=
// free points (message 0x145) are added at once (message 0x146).
func (w *World) commandAddPoint(o *Object, c command, arg string, typ int) {
	if typ == 4 && o.Class != 4 {
		w.systemMessage(o, w.messageFor(o, 0x144))
		return
	}
	if commandToken(arg, 0, 5) == "auto" {
		w.commandAddPointAuto(o, c, arg, typ)
		return
	}
	n := cmdInt(arg, 0)
	ch := o.P.Char
	if n <= 0 || n > ch.LevelUpPoints {
		w.systemMessage(o, w.messageFor(o, 0x145))
		return
	}
	if !w.addStatPoint(o, typ, n) {
		return
	}
	w.send(o, w.packetCharRefresh(o))
	w.systemMessage(o, w.messageFor(o, 0x146, n, ch.LevelUpPoints))
	w.log.Info("[CommandAddPoint]", "account", o.P.Account.Login, "name", ch.Name, "type", typ, "amount", n)
	w.commandDone(o, c)
}

func autoPoints(o *Object) *autoPointState {
	if o.P.autoPoints == nil {
		o.P.autoPoints = &autoPointState{}
	}
	return o.P.autoPoints
}

func (a *autoPointState) recount() {
	a.Count = 0
	for _, v := range a.Amount {
		if v > 0 {
			a.Count++
		}
	}
}

// commandAddPointAuto is FUN_004377c0: amount <= 0 turns the stat off
// (message 0x148), otherwise message 0x147. The CommandAddPointAutoEnable_AL
// keys are read by the original but never tested.
func (w *World) commandAddPointAuto(o *Object, c command, arg string, typ int) {
	n := cmdInt(arg, 1)
	a := autoPoints(o)
	if n < 1 {
		a.Amount[typ] = 0
		a.recount()
		w.systemMessage(o, w.messageFor(o, 0x148))
	} else {
		a.Amount[typ] = n
		a.recount()
		w.systemMessage(o, w.messageFor(o, 0x147, n))
		w.log.Info("[CommandAddPointAuto]", "account", o.P.Account.Login, "name", o.P.Char.Name, "type", typ, "amount", n)
	}
	w.commandDone(o, c)
}

// autoPointSecond is FUN_00435180 (timer 6, every second): players with
// auto types and free points get them spread (FUN_0043cd20).
func (w *World) autoPointSecond() {
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if !noticePlayer(o) || o.P.Char == nil || o.P.autoPoints == nil {
			continue
		}
		if o.P.autoPoints.Count > 0 && o.P.Char.LevelUpPoints != 0 {
			w.spreadAutoPoints(o)
		}
	}
}

// spreadAutoPoints is FUN_0043cd20: each auto stat in order takes
// ceil(points / types left), at most its remaining amount and up to
// MaxStatPoint_AL; a stat that reaches the cap stops being automatic.
func (w *World) spreadAutoPoints(o *Object) {
	a, ch := o.P.autoPoints, o.P.Char
	stats := [5]*int{&ch.Strength, &ch.Dexterity, &ch.Vitality, &ch.Energy, &ch.Leadership}
	maxStat := w.cfg.ALInt("Common", "MaxStatPoint", o.P.Account.Level, 0)
	left := a.Count
	for i := range a.Amount {
		if a.Amount[i] <= 0 {
			continue
		}
		if left == 0 {
			break
		}
		share := ch.LevelUpPoints / left
		if ch.LevelUpPoints%left > 0 {
			share++
		}
		share = min(share, a.Amount[i])
		if *stats[i]+share > maxStat {
			share = maxStat - *stats[i]
		}
		*stats[i] += share
		ch.LevelUpPoints -= share
		left--
		if *stats[i] >= maxStat {
			share = a.Amount[i]
		}
		a.Amount[i] -= share
		a.recount()
	}
	w.calcCharacter(o)
	w.send(o, w.packetCharRefresh(o))
}

// commandPKClear is /pk (FUN_004379c0): only for PK level >= 4 (message
// 0x149). CommandPKClearType 0 clears for free (C1 07 F3 08 to the player
// and its viewport, message 0x14A, command charge); otherwise the price is
// CommandPKClearMoney_AL x PKCount (at most 2,000,000,000, message 0x132
// when short) and the command delay/charge is not applied.
func (w *World) commandPKClear(o *Object, c command, arg string) {
	ch := o.P.Char
	if o.PKLevel < 4 {
		w.systemMessage(o, w.messageFor(o, 0x149))
		return
	}
	clear := func() {
		o.PKLevel, ch.PKLevel = PKCommoner, PKCommoner
		ch.PKCount, ch.PKTime = 0, 0
		w.sendPKLevel(o)
		w.systemMessage(o, w.messageFor(o, 0x14A))
		w.log.Info("[CommandPKClear]", "account", o.P.Account.Login, "name", ch.Name, "pklevel", o.PKLevel)
	}
	if w.cfg.Int("Command", "CommandPKClearType", 0) == 0 {
		clear()
		w.commandDone(o, c)
		return
	}
	cost := int64(w.cfg.ALInt("Command", "CommandPKClearMoney", o.P.Account.Level, 0)) * int64(ch.PKCount)
	cost = min(uint32Wrap(cost), 2000000000)
	if ch.Money < cost {
		w.systemMessage(o, w.messageFor(o, 0x132, cost, c.Syntax))
		return
	}
	clear()
	ch.Money -= cost
	w.sendMoney(o)
}

// uint32Wrap is the unsigned 32-bit product of the original.
func uint32Wrap(v int64) int64 { return int64(uint32(v)) }

// commandMoney is /zen (FUN_00437c20): amount < 1 sets the zen to 0;
// otherwise it is added up to 2,000,000,000 (C3 10 22 FE money).
func (w *World) commandMoney(o *Object, c command, arg string) {
	n := cmdInt(arg, 0)
	ch := o.P.Char
	switch {
	case n < 1:
		ch.Money = 0
	case ch.Money+int64(n) < 2000000001:
		ch.Money += int64(n)
	default:
		ch.Money = 2000000000
	}
	w.sendMoney(o)
	w.log.Info("[CommandMoney]", "account", o.P.Account.Login, "name", ch.Name, "money", n)
	w.commandDone(o, c)
}

// clearInventoryKeep are the items /clearinv leaves in the bag.
func clearInventoryKeep(idx int) bool { return idx == 0x1A13 || idx == 0x1C40 || idx == 0x1CDF }

// commandClearInventory is /clearinv (FUN_00438a90): every bag slot
// 12..75 except the kept items is deleted with C1 05 28 slot, empty slots
// included (message 0x16C).
func (w *World) commandClearInventory(o *Object, c command, arg string) {
	for slot := BagStart; slot < BagEnd; slot++ {
		if clearInventoryKeep(o.P.Inventory[slot].Index) {
			continue
		}
		w.deleteInventory(o, slot)
		w.send(o, packetItemDelete(slot, 1))
		w.send(o, packetPeriodicDelete(slot)) // FUN_004d3fd0, every slot
	}
	w.systemMessage(o, w.messageFor(o, 0x16C))
	w.log.Info("[CommandClearInventory]", "account", o.P.Account.Login, "name", o.P.Char.Name)
	w.commandDone(o, c)
}

// commandRedistribute is /readd (FUN_00438cc0): the stats go back to the
// DefaultClassInfo values and the difference returns as free points
// (leadership only for the Dark Lord), message 0x16D.
func (w *World) commandRedistribute(o *Object, c command, arg string) {
	ch := o.P.Char
	var ci struct{ Str, Dex, Vit, Ene, Lead int }
	if o.Class < 5 && w.statsData != nil {
		d := w.statsData.classes[o.Class]
		ci.Str, ci.Dex, ci.Vit, ci.Ene, ci.Lead = d.Strength, d.Dexterity, d.Vitality, d.Energy, d.Lead
	}
	back := (ch.Strength - ci.Str) + (ch.Dexterity - ci.Dex) + (ch.Vitality - ci.Vit) + (ch.Energy - ci.Ene)
	ch.Strength, ch.Dexterity, ch.Vitality, ch.Energy = ci.Str, ci.Dex, ci.Vit, ci.Ene
	if o.Class == 4 {
		back += ch.Leadership - ci.Lead
		ch.Leadership = ci.Lead
	}
	ch.LevelUpPoints += back
	w.send(o, w.packetCharRefresh(o))
	w.saveCharacter(o, nil) // FUN_00475c30
	w.calcCharacter(o)
	w.systemMessage(o, w.messageFor(o, 0x16D))
	w.log.Info("[CommandRedistribute]", "account", o.P.Account.Login, "name", ch.Name)
	w.commandDone(o, c)
}
