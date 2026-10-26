package game

import (
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// WizardTeam world customs of Data/Custom that the GameServer uses:
// CustomMove (chat words that move the player, FUN_0044f430 /
// FUN_0044f550 from the chat handler FUN_004d7720), CustomGate (cells that
// move the player, FUN_0044b850 / FUN_0044b8b0 in the 1-second proc
// FUN_0050a140), CustomRespawn (FUN_00458770 at the end of the respawn
// FUN_004c53c0), CustomNPCCommand (FUN_00450580 in the NPC talk
// FUN_004c1490) and CustomDeathMessage (FUN_0044ac50 when a monster kills,
// from FUN_004cda80).

type worldCustoms struct {
	moves    []gamedata.CustomMoveRow
	gates    []gamedata.CustomGateRow
	respawn  []gamedata.CustomRespawnRow
	npcCmds  []gamedata.CustomNPCCommandRow
	deathMsg map[int]string
	x        *worldCustomsExt // the other customs (custom_world_ext.go)
}

func init() {
	startHooks = append(startHooks, loadWorldCustoms)
	tick1000Hooks = append(tick1000Hooks, (*World).customGateSecond)
	customMoveHook = (*World).customMove
	npcCommandHook = (*World).customNPCCommand
	customRespawnHook = (*World).arenaOrCustomRespawn
	deathHooks = append(deathHooks, (*World).customDeathMessage)
}

// Hooks called from shared code.
var (
	// customMoveHook runs on every chat line after its log (chat.go).
	customMoveHook func(w *World, o *Object, msg string)
	// npcCommandHook runs after the NpcTalk lines; true ends the talk
	// (npc.go).
	npcCommandHook func(w *World, o, npc *Object) bool
	// customRespawnHook overrides the respawn place (combat.go).
	customRespawnHook func(w *World, o *Object) (mp, x, y int, ok bool)
)

func (w *World) wcs() *worldCustoms {
	c := w.cev()
	if c.world == nil {
		c.world = &worldCustoms{deathMsg: map[int]string{}}
	}
	return c.world
}

func loadWorldCustoms(w *World) error {
	c := &worldCustoms{deathMsg: map[int]string{}}
	if old := w.cev().world; old != nil {
		c.x = old.x
	}
	w.cev().world = c
	if w.configDir == "" {
		return nil
	}
	var err error
	if c.moves, err = gamedata.LoadCustomMove(w.configDir); err != nil {
		return err
	}
	if c.gates, err = gamedata.LoadCustomGate(w.configDir); err != nil {
		return err
	}
	if c.respawn, err = gamedata.LoadCustomRespawn(w.configDir); err != nil {
		return err
	}
	if c.npcCmds, err = gamedata.LoadCustomNPCCommand(w.configDir); err != nil {
		return err
	}
	c.deathMsg, err = gamedata.LoadCustomDeathMessage(w.configDir)
	return err
}

// --- CustomMove ---------------------------------------------------------------------------

// customMove is FUN_0044f430 + FUN_0044f550: the first word of the chat
// line (stricmp) selects a row; the level, reset, master reset, account
// level, PK and busy checks give messages 0xE0, 0xE6..0xEA, 0xE2, 0xE3;
// Atlans refuses Uniria/Dinorant (0x112) and Icarus needs wings, a
// Dinorant or a Fenrir (0xE4). The player leaves the custom events and is
// moved (FUN_0050fd50). The line then goes on as usual.
func (w *World) customMove(o *Object, msg string) {
	word := commandToken(msg, 0, 0x20)
	var r *gamedata.CustomMoveRow
	for i := range w.wcs().moves {
		if strings.EqualFold(w.wcs().moves[i].Command, word) {
			r = &w.wcs().moves[i]
			break
		}
	}
	if r == nil {
		return
	}
	ch := o.P.Char
	refuse := func(n int, args ...any) { w.eventNotice(o, NoticeBlue, n, args...) }
	switch {
	case r.MinLevel != -1 && ch.Level < r.MinLevel:
		refuse(0xE0, r.MinLevel)
		return
	case r.MaxLevel != -1 && r.MaxLevel < ch.Level:
		refuse(0xE6, r.MaxLevel)
		return
	case r.MinReset != -1 && ch.Resets < r.MinReset:
		refuse(0xE7, r.MinReset)
		return
	case r.MaxReset != -1 && r.MaxReset < ch.Resets:
		refuse(0xE8, r.MaxReset)
		return
	case r.MinMReset != -1 && ch.MasterResets < r.MinMReset:
		refuse(0xE9, r.MinMReset)
		return
	case r.MaxMReset != -1 && r.MaxMReset < ch.MasterResets:
		refuse(0xEA, r.MaxMReset)
		return
	case o.P.Account.Level < r.AccountLevel:
		refuse(0xE2)
		return
	case r.PKMove == 0 && o.PKLevel > 4:
		refuse(0xE3)
		return
	case ifBusy(o) || o.Teleporting || w.commandUnsettled(o) || pshopOpen(o):
		refuse(0xE2)
		return
	}
	if n := w.mountRefusal(o, r.Map, false); n != 0 {
		refuse(n)
		return
	}
	w.customEventLeave(o)
	w.gmWarp(o, r.Map, r.X, r.Y)
	w.log.Info("[CustomMove]", "account", o.P.Account.Login, "name", ch.Name, "move", r.Index)
}

// mountRefusal is the map check of CustomMove / CustomGate: Atlans (7)
// refuses a Uniria or a Dinorant in the pet slot (0x112); Icarus (10)
// needs wings or a Dinorant / Fenrir (the gate also accepts a Dark Horse)
// (0xE4).
func (w *World) mountRefusal(o *Object, mp int, gate bool) int {
	pet := w.slotItem(o, 8)
	switch mp {
	case 7:
		if !pet.IsEmpty() && (pet.Index == 0x1A02 || pet.Index == 0x1A03) {
			return 0x112
		}
	case 10:
		if !w.slotItem(o, 7).IsEmpty() {
			return 0
		}
		if pet.Index == 0x1A03 || pet.Index == 0x1A25 || (gate && pet.Index == 0x1A04) {
			if !pet.IsEmpty() {
				return 0
			}
		}
		return 0xE4
	}
	return 0
}

// --- CustomGate ---------------------------------------------------------------------------

// customGateSecond is FUN_0044b850: every playing player standing on the
// exact cell of a row is checked (0xE0, 0xE6..0xE8, 0xE2, 0x112, 0xE4)
// and moved.
func (w *World) customGateSecond() {
	c := w.wcs()
	if len(c.gates) == 0 {
		return
	}
	for i := PlayerStart; i < MaxObjects; i++ {
		o := w.objects[i]
		if o.State != StatePlaying || !o.IsPlayer() || o.P.Char == nil || !canPlayerAct(o) {
			continue
		}
		for k := range c.gates {
			if g := &c.gates[k]; g.Map == o.Map && g.X == o.X && g.Y == o.Y {
				w.customGate(o, g)
				break
			}
		}
	}
}

// customGate is FUN_0044b8b0.
func (w *World) customGate(o *Object, g *gamedata.CustomGateRow) {
	ch := o.P.Char
	refuse := func(n int, args ...any) { w.eventNotice(o, NoticeBlue, n, args...) }
	switch {
	case g.MinLevel != -1 && ch.Level < g.MinLevel:
		refuse(0xE0, g.MinLevel)
		return
	case g.MaxLevel != -1 && g.MaxLevel < ch.Level:
		refuse(0xE6, g.MaxLevel)
		return
	case g.MinReset != -1 && ch.Resets < g.MinReset:
		refuse(0xE7, g.MinReset)
		return
	case g.MaxReset != -1 && g.MaxReset < ch.Resets:
		refuse(0xE8, g.MaxReset)
		return
	case o.P.Account.Level < g.AccountLevel || ifBusy(o) || o.Teleporting || w.commandUnsettled(o) || pshopOpen(o):
		refuse(0xE2)
		return
	}
	if g.ToMap == 7 {
		if n := w.mountRefusal(o, 7, true); n != 0 {
			refuse(n)
			return
		}
	}
	if w.immobilized(o) {
		refuse(0xE2)
		return
	}
	if g.ToMap == 10 {
		if n := w.mountRefusal(o, 10, true); n != 0 {
			refuse(n)
			return
		}
	}
	w.gmWarp(o, g.ToMap, g.ToX, g.ToY)
	w.log.Info("[CustomMove]", "account", o.P.Account.Login, "name", ch.Name, "map", o.Map, "x", o.X, "y", o.Y)
}

// --- CustomRespawn ------------------------------------------------------------------------

// customRespawn is FUN_00458770: the row of the death map and the account
// level gives -1 (the death cell), 0 (the death cell, y + rand()%3), a
// gate (its random cell) or -2 / no row (the normal respawn).
func (w *World) customRespawn(o *Object) (int, int, int, bool) {
	for _, r := range w.wcs().respawn {
		if r.Map != o.Map {
			continue
		}
		al := o.P.Account.Level
		if al < 0 || al > 3 {
			return 0, 0, 0, false
		}
		switch v := r.Gate[al]; {
		case v == -1:
			return o.Map, o.X, o.Y, true
		case v == 0:
			return o.Map, o.X, o.Y + monsterRand()%3, true
		case v > 0:
			g, ok := w.data.Gates[v]
			if !ok {
				return 0, 0, 0, false
			}
			x, y := w.randomGatePosition(v)
			return g.Map, x, y, true
		}
		return 0, 0, 0, false
	}
	return 0, 0, 0, false
}

// --- CustomNPCCommand ---------------------------------------------------------------------

// customNPCCommand is FUN_00450580: an NPC of a row on its exact cell runs
// the row's command for the player (FUN_00435400) and ends the talk.
func (w *World) customNPCCommand(o, npc *Object) bool {
	for _, r := range w.wcs().npcCmds {
		if r.Class == npc.Class && r.Map == npc.Map && r.X == npc.X && r.Y == npc.Y {
			w.runCommand(o, r.Command)
			return true
		}
	}
	return false
}

// --- CustomDeathMessage -------------------------------------------------------------------

// customDeathMessage is FUN_0044ac50: a monster that kills says the
// sentence of its class (object chat, FUN_004d9c60) to the victim.
func (w *World) customDeathMessage(killer, victim *Object) {
	if killer.Type != TypeMonster || !victim.IsPlayer() {
		return
	}
	if text, ok := w.wcs().deathMsg[killer.Class]; ok {
		w.send(victim, packetObjectChat(killer.Index, text))
	}
}
