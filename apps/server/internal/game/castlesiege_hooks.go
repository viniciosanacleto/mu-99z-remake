package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"

// Wiring of the castle module: client heads, NPC talks and the hooks the
// other modules call (the castle rules of their original functions).

var (
	// gate97Hook decides gate 0x61 (the crown room) in gObjMoveGate
	// (FUN_0050f6d0); teleport.go calls it.
	gate97Hook func(w *World, o *Object) bool
	// castleAttackHook is the map 30 test at the start of FUN_0040b110
	// (pvp.go attackFull).
	castleAttackHook func(w *World, a, t *Object) bool
	// castlePvPHook is the map 30 branch of FUN_0040dca0 (pvp.go).
	castlePvPHook func(w *World, a, t *Object) (allowed, decided bool)
	// castleDamageRateHook is the CastleSiegeDamageRate1/2 branch of the
	// PvP damage (pvp.go).
	castleDamageRateHook func(w *World, a, t *Object) (rate int, ok bool)
	// castleRespawnHook is the map 30 branch of FUN_004c53c0 (combat.go
	// respawnPlayer): a gate, or (gate -1) a fixed cell.
	castleRespawnHook func(w *World, o *Object) (gate, mp, x, y int, ok bool)
	// monsterEntryHook adjusts a C2 13 entry (FUN_00515510: life stone
	// flags, crown and gate effects); monster.go calls it.
	monsterEntryHook func(w *World, viewer, t *Object, entry []byte)
	// castleDropHook intercepts the castle scrolls of the item drop
	// FUN_004988f0 (drop.go dropSpecialItem).
	castleDropHook func(w *World, o *Object, it item.Item, x, y int) (handled, ok bool)
	// monsterSearchHook replaces the enemy search of special monsters
	// (FUN_004b4090 -> FUN_004b2ec0 for mercenaries; monster_ai.go).
	monsterSearchHook func(w *World, o *Object) bool
)

// handleCSPacket is the B2 sub dispatch of ProtocolCore.
func (w *World) handleCSPacket(o *Object, p []byte) {
	if len(p) < 4 {
		return
	}
	switch p[3] {
	case 0x00:
		w.handleCSState(o, p)
	case 0x01:
		w.handleCSRegister(o, p)
	case 0x02:
		w.handleCSGiveUp(o, p)
	case 0x03:
		w.handleCSRegInfo(o, p)
	case 0x04:
		w.handleCSRegMark(o, p)
	case 0x05:
		w.handleCSNPCBuy(o, p)
	case 0x06:
		w.handleCSNPCRepair(o, p)
	case 0x07:
		w.handleCSNPCUpgrade(o, p)
	case 0x08:
		w.handleCSTaxInfo(o, p)
	case 0x09:
		w.handleCSTaxRate(o, p)
	case 0x10:
		w.handleCSMoneyOut(o, p)
	case 0x12:
		w.handleCSGateOperate(o, p)
	case 0x1B:
		w.handleCSMinimap(o, p)
	case 0x1C:
		w.handleCSMinimapClose(o, p)
	case 0x1D:
		w.handleCSCommand(o, p)
	case 0x1F:
		w.handleCSHuntZone(o, p)
	}
}

// csGate97 is the gate 0x61 branch of FUN_0050f6d0: during the battle the
// defenders, and the attackers once the crown room is open; otherwise the
// owner guild and its alliance.
func (w *World) csGate97(o *Object) bool {
	c := w.cs()
	if c == nil {
		return false
	}
	if w.csSyncState() == csStart {
		if csSide(o) != 1 && !c.crownRoom {
			return false
		}
		return csSide(o) != 0
	}
	return w.csOwnerGuild(o) || w.csOwnerAlliance(o)
}

// csMonsterEntry is the castle part of FUN_00515510.
func (w *World) csMonsterEntry(viewer, t *Object, e []byte) {
	switch t.Class {
	case csClassStone:
		flags := byte(1)
		if w.csSyncState() == csStart && csSide(viewer) == csSide(t) {
			flags = 0x81
		}
		if n := guildNumberOf(viewer); n != 0 {
			if c := w.cs(); c != nil && c.stoneOf(n) == t.Index {
				flags |= 0x80
			}
		}
		if t.CS != nil {
			flags |= (t.CS.StoneLvl & 7) << 4
		}
		e[2] = flags
	case csClassCrown:
		if c := w.cs(); c != nil && eventEffectHook != nil {
			if c.crownOpen != hasEffectIndex(t, csEffCrown) {
				eventEffectHook(w, t, csEffCrown, c.crownOpen)
				u32le(e[4:], t.ViewSkillState)
			}
		}
	case csClassGate:
		if t.CS != nil && eventEffectHook != nil {
			open := t.CS.GateOpen != 0
			if open != hasEffectIndex(t, csEffGateOpen) {
				eventEffectHook(w, t, csEffGateOpen, open)
				u32le(e[4:], t.ViewSkillState)
			}
		}
	}
}

func init() {
	worldHandlers[0xB2] = (*World).handleCSPacket
	worldHandlers[0xB3] = (*World).handleCSNPCList
	worldHandlers[0xB4] = (*World).handleCSRegList
	worldHandlers[0xB5] = (*World).handleCSSiegeList
	worldHandlers[0xB7] = func(w *World, o *Object, p []byte) {
		if len(p) < 4 {
			return
		}
		switch p[3] {
		case 0x01:
			w.handleCSWeaponUse(o, p)
		case 0x04:
			w.handleCSWeaponHit(o, p)
		}
	}
	worldHandlers[0xB9] = func(w *World, o *Object, p []byte) {
		if len(p) < 4 {
			return
		}
		switch p[3] {
		case 0x02:
			w.handleCSOwnerMark(o, p)
		case 0x05:
			w.handleCSHuntEnter(o, p)
		}
	}

	// NPC talk (FUN_004c1490 cases 0xD8..0xE0).
	csTalk := func(f func(w *World, o, npc *Object)) func(w *World, o, npc *Object) bool {
		return func(w *World, o, npc *Object) bool {
			if w.cs() != nil {
				f(w, o, npc)
			}
			return true
		}
	}
	npcTalkHooks[csClassCrown] = csTalk((*World).csCrownTalk)
	npcTalkHooks[csClassSwitch1] = csTalk((*World).csSwitchTalk)
	npcTalkHooks[csClassSwitch2] = csTalk((*World).csSwitchTalk)
	npcTalkHooks[csClassLever] = csTalk((*World).csLeverTalk)
	npcTalkHooks[csClassKeeper] = csTalk(func(w *World, o, _ *Object) { w.csKeeperTalk(o) })
	npcTalkHooks[csClassSlingA] = func(w *World, o, npc *Object) bool { w.csSlingTalk(o, npc); return true }
	npcTalkHooks[csClassSlingD] = func(w *World, o, npc *Object) bool { w.csSlingTalk(o, npc); return true }
	npcTalkHooks[csClassSenior] = func(w *World, o, npc *Object) bool {
		if !pshopOpen(o) { // obj+0x2468: personal shop open
			w.csSeniorTalk(o)
		}
		return true
	}
	npcTalkHooks[csClassGuard] = func(w *World, o, npc *Object) bool { w.csGuardTalk(o, npc); return true }

	deathHooks = append(deathHooks, func(w *World, killer, victim *Object) {
		if w.cs() == nil || victim.Mon == nil || victim.CS == nil {
			return
		}
		switch {
		case victim.CS.NPCKind == 1:
			w.csNPCKilled(victim) // FUN_004b70b0 -> FUN_0041e180
		case victim.Class == csClassStone:
			w.csStoneGone(victim)
		case victim.Class == csClassArcher || victim.Class == csClassSpear:
			c := w.cs()
			c.mercenaries = max(c.mercenaries-1, 0) // FUN_004b2e90
		}
	})

	castleBattleHook = func(w *World) bool { return w.csBattleOn() }
	castleOwnerHook = func(w *World, guild string) bool {
		_, _, _, owner := w.csSyncValues()
		return guild != "" && guild == owner
	}
	gate97Hook = (*World).csGate97
	castleAttackHook = (*World).csAttackAllowed
	castlePvPHook = (*World).csPvP
	castleDamageRateHook = (*World).csDamageRate
	castleRespawnHook = (*World).csRespawn
	monsterEntryHook = (*World).csMonsterEntry
	castleDropHook = (*World).csDropItem
	monsterSearchHook = func(w *World, o *Object) bool { return w.cs() != nil && w.csMercenarySearch(o) }
	pkExemptHooks = append(pkExemptHooks, (*World).csPKExempt)
}
