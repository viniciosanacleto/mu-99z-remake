package game

import "github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"

// Placeholder extension types. Each gameplay module replaces its own type
// with the real definition in its own file (and deletes the line here).
type ()

// Lifecycle hooks. Modules append to these in init() instead of editing
// shared files.
var (
	// enterWorldHooks run at the end of enterWorld, after F3 03/F3 10/F3 11
	// and the self C2 12 were sent.
	enterWorldHooks []func(*World, *Object)
	// leaveWorldHooks run when a playing character leaves the map
	// (logout, return to select, server move, disconnect).
	leaveWorldHooks []func(*World, *Object)
	// startHooks run once in New after data loading; they may load module
	// data and spawn objects. Returning an error aborts start-up.
	startHooks []func(*World) error
	// pkExemptHooks: a kill for which one returns true gives no PK
	// (FUN_0050f330 early returns: rival guilds, guild war, ...).
	pkExemptHooks []func(w *World, killer, victim *Object) bool
)

// Cross-module calls. The owning module sets the variable in init(); callers
// must nil-check, so every module compiles and tests on its own.
var (
	// move.go: walk o from (sx, sy) along direction nibbles (0..7), exactly
	// like an accepted D3 (path arrays, TX/TY, C1 08 D3 broadcast).
	setPathHook func(w *World, o *Object, sx, sy int, dirs []byte)
	// move.go: teleport o to (mp, x, y) inside this instance (gate/warp).
	teleportHook func(w *World, o *Object, mp, x, y, dir int)

	// inventory (items module): drop an item / money on the map, owned by
	// object index owner (-1 = anyone); false if the cell or map is full.
	dropMapItemHook  func(w *World, mp, x, y int, it item.Item, owner int) bool
	dropMapMoneyHook func(w *World, mp, x, y int, amount int, owner int) bool
	// items module: send the current life (C1 26 FF) / mana (C1 27 FF)
	// refill packets to a player.
	sendLifeHook func(w *World, o *Object)
	sendManaHook func(w *World, o *Object)

	// stats module: recompute derived stats after equipment or stat changes
	// (CalcCharacter, FUN_004c9790) and read them for combat.
	recalcHook      func(w *World, o *Object)
	combatStatsHook func(w *World, o *Object) CombatStats
	// stats module: experience gain with level up (and party sharing).
	gainExperienceHook func(w *World, o *Object, exp int64)
	// party.go/levelup.go: experience of a monster killed by a party member
	// (FUN_004c5ea0, chosen by FUN_004cda80 when the killer is in a party):
	// adds experience, levels up and sends C3 18 9C to every member in
	// range. damage is the killing hit shown in 9C. Returns false when the
	// killer has no party (use the per-attacker FUN_004c5970 instead).
	partyExperienceHook func(w *World, killer, monster *Object, damage int, flag bool) bool
	// pet.go: experience of a kill for the equipped Dark Horse / Dark Raven
	// (FUN_004c69d0, called by FUN_004c5a30 and FUN_004c5ea0 with the
	// experience the player got, before its level up).
	petExperienceHook func(w *World, o *Object, exp int64)
	// pk.go: a player killed another player (FUN_0050f330): PK count,
	// PK level and the C1 07 F3 08 broadcast.
	playerKillHook func(w *World, killer, victim *Object)
	// guild_war.go: FUN_004864b0, the guilds of a and b are at war with
	// each other (PvP may attack).
	guildWarHook func(w *World, a, b *Object) bool
	// guild_war.go: FUN_004868f0, a player killed another one; true when
	// the kill counted for a guild war (the death path FUN_004cda80 sets
	// victim obj+0x285 = 2 then).
	guildWarKillHook func(w *World, killer, victim *Object) bool
	// guild.go: the guild viewport packets (C2 65 for new players,
	// C2 67 for visible ones) at the end of the viewport send.
	guildViewportHook func(w *World, o *Object)
	// guild.go: the guild part of the player re-show after a teleport
	// (FUN_00516f60 / FUN_00517080).
	guildReshowHook func(w *World, o *Object)

	// NPC talk (30): the monsters module dispatches by NPC class; the items
	// module registers shops and the warehouse.
	npcTalkHooks = map[int]func(w *World, o, npc *Object) bool{}
)

// CombatStats are the derived combat values of an object (players: from
// CalcCharacter; monsters: from MonsterList.txt).
type CombatStats struct {
	DamageMinRight, DamageMaxRight int
	DamageMinLeft, DamageMaxLeft   int
	MagicDamageMin, MagicDamageMax int
	Defense                        int
	MagicDefense                   int
	AttackSuccessRate              int
	DefenseSuccessRate             int
	AttackSpeed                    int
	MagicSpeed                     int
	CriticalDamage                 int
	ExcellentDamage                int

	// combat.go (CAttack, FUN_0040ec10/FUN_0040b110) reads these as well:
	// CriticalDamage/ExcellentDamage are the rates obj+0x844/0x84c (%),
	// the bonuses are the flat additions obj+0x848/0x850.
	CriticalDamageBonus  int // obj+0x848
	ExcellentDamageBonus int // obj+0x850
	IgnoreDefenseRate    int // obj+0x83c (%)
	DoubleDamageRate     int // obj+0x834 (%)
	TripleDamageRate     int // obj+0x838 (%)
}
