package game

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game/item"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

// hooks is the table through which the gameplay modules plug into the core
// of one World: lifecycle lists (enterWorld, leaveWorld, start, tick100,
// tick1000, ...), cross-module calls and the packet and command dispatch
// tables. Each module fills its part in a register function; newHooks calls
// them all in a fixed order. Every World owns its table, so tests can stub a
// hook on their own World without touching shared state.
//
// A nil function hook means the owning module did not register one; callers
// must nil-check them.
type hooks struct {
	// blacklist.go
	// acceptFilter decides whether a new GameServer connection from ip is
	// accepted (world.go).
	acceptFilter func(w *World, ip string) bool

	// castlesiege_hooks.go
	// gate97 decides gate 0x61 (the crown room) in gObjMoveGate
	// (FUN_0050f6d0); teleport.go calls it.
	gate97 func(w *World, o *Object) bool
	// castleAttack is the map 30 test at the start of FUN_0040b110
	// (pvp.go attackFull).
	castleAttack func(w *World, a, t *Object) bool
	// castlePvP is the map 30 branch of FUN_0040dca0 (pvp.go).
	castlePvP func(w *World, a, t *Object) (allowed, decided bool)
	// castleDamageRate is the CastleSiegeDamageRate1/2 branch of the
	// PvP damage (pvp.go).
	castleDamageRate func(w *World, a, t *Object) (rate int, ok bool)
	// castleRespawn is the map 30 branch of FUN_004c53c0 (combat.go
	// respawnPlayer): a gate, or (gate -1) a fixed cell.
	castleRespawn func(w *World, o *Object) (gate, mp, x, y int, ok bool)
	// monsterEntry adjusts a C2 13 entry (FUN_00515510: life stone
	// flags, crown and gate effects); monster.go calls it.
	monsterEntry func(w *World, viewer, t *Object, entry []byte)
	// castleDrop intercepts the castle scrolls of the item drop
	// FUN_004988f0 (drop.go dropSpecialItem).
	castleDrop func(w *World, o *Object, it item.Item, x, y int) (handled, ok bool)
	// monsterSearch replaces the enemy search of special monsters
	// (FUN_004b4090 -> FUN_004b2ec0 for mercenaries; monster_ai.go).
	monsterSearch func(w *World, o *Object) bool

	// chat.go
	// partyMembers returns the object indexes of o's party (up to 5,
	// obj+0x2f0 party number); "~" messages go to each of them.
	partyMembers func(w *World, o *Object) []int
	// guildChat routes "@" messages: kind 0 guild chat
	// (FUN_00483500(0)), 1 alliance chat "@@" (FUN_00483500(1), only with
	// a union), 2 guild notice "@>" (master only, C1 49 A0 06 ... through
	// the ExDB). msg starts after the prefix character(s).
	guildChat func(w *World, o *Object, kind int, msg []byte)
	// chatAnswer sees every normal chat line before the command check
	// (FUN_0044f430: an event answer check, probably CustomQuiz).
	chatAnswer func(w *World, o *Object, msg string)
	// globalWhisper forwards a whisper to another instance (DataServer
	// 0x72 / 0x73 relay). It returns false when the name is not online
	// anywhere; nil means "not found".
	globalWhisper func(w *World, from *Object, target string, msg []byte) bool

	// cmdrequirement.go
	// commandRequirement is FUN_00440660; true blocks the command.
	commandRequirement func(w *World, o *Object, index int) bool

	// combat.go
	// objectDeath runs when t dies in lifeCheck, before the die packet
	// (pvp.go: FUN_0050e8a0 death penalty, PK, duel, effects).
	objectDeath func(w *World, killer, t *Object)

	// command.go
	// commandHandlers are the implemented commands by CommandManager index.
	// Every other index is not implemented yet.
	commandHandlers map[int]func(w *World, o *Object, c command, arg string)
	// hardCodedHandlers are the implemented hard-coded commands (lower case).
	hardCodedHandlers map[string]func(w *World, o *Object, arg string)
	// commandScript is the Lua OnCommandManager bridge (FUN_004e6ba0):
	// returning true means the script handled the command.
	commandScript func(w *World, o *Object, index int, arg string) bool
	// globalPost relays /post to every instance (ExDB C1 52 20).
	globalPost func(w *World, o *Object, msg string)

	// command_player2.go
	// commandEventBlock reports the custom event flags tested by both
	// DS replies (obj+0x24f0 and three per-index event tables) that make them
	// answer "Proibido usar o comando no evento."; those events do not exist
	// yet (U-CUST-07).
	commandEventBlock func(w *World, o *Object) bool

	// custom_world.go
	// customMove runs on every chat line after its log (chat.go).
	customMove func(w *World, o *Object, msg string)
	// npcCommand runs after the NpcTalk lines; true ends the talk
	// (npc.go).
	npcCommand func(w *World, o, npc *Object) bool
	// customRespawn overrides the respawn place (combat.go).
	customRespawn func(w *World, o *Object) (mp, x, y int, ok bool)

	// custom_world_ext.go
	// viewEntry runs for each C2 12 entry built for viewer
	// (FUN_00515160) and for the own entry (FUN_005167b0).
	viewEntry func(w *World, viewer, t *Object, entry []byte)
	// viewportAdd runs after the player part of the viewport add
	// (FUN_00514700).
	viewportAdd func(w *World, o *Object)

	// customarena.go
	// arenaMove is the arena part of CMove::Move (command.go moveTo):
	// false stops the move.
	arenaMove func(w *World, o *Object, gate int) bool
	// arenaFight is FUN_00443f50 for the attacker (pvp.go, skill.go).
	arenaFight func(w *World, a *Object) bool

	// customevent.go
	// commandEvent runs before the commands of FUN_00435400 (name is
	// the first word); true stops the line.
	commandEvent func(w *World, o *Object, name string) bool
	// customEventAttack is false when an event forbids the attack
	// (pvp.go attackFull).
	customEventAttack func(w *World, a, t *Object) bool
	// tradeRequest is true when the trade request was consumed or
	// refused (trade.go handleTradeRequest).
	tradeRequest func(w *World, o, t *Object) bool

	// devilsquare.go
	// eventTimeHandlers are the branches of 91 by event type (1 Devil Square,
	// 2 Blood Castle, 4 Chaos Castle, 5 Illusion...): nil result = no reply.
	eventTimeHandlers map[byte]func(w *World, o *Object, p []byte) []byte

	// dispatch.go
	// packetBlock refuses an in-game head before its handler (the
	// Custom event checks of 36, 40 and AA in FUN_004d6690;
	// customevent.go).
	packetBlock func(w *World, o *Object, head byte) bool
	// worldHandlers maps in-game heads to handlers; gameplay files fill it
	// in their register functions.
	worldHandlers map[byte]func(*World, *Object, []byte)
	// faHandlers and fbHandlers map the sub-codes of the custom heads FA
	// and FB (FUN_004d6690: FA only for C1 packets longer than 3 bytes, the
	// sub-code at [3]); xshop.go registers the head handlers.
	faHandlers map[byte]func(*World, *Object, []byte)
	fbHandlers map[byte]func(*World, *Object, []byte)
	// f3Handlers maps F3 sub-codes (other than 00-03 and 09) to in-game
	// handlers; modules fill it in their register functions.
	f3Handlers map[byte]func(*World, *Object, []byte)

	// duel.go
	// tradeOpen lets the trade module report an open trade.
	tradeOpen func(o *Object) bool

	// effect_save.go
	// characterLoad run in enterWorld after the inventory and skills are
	// decoded and before CharSet / CalcCharacter (FUN_004cc730 order).
	characterLoad []func(w *World, o *Object)

	// enterworld.go
	// characterSave run when saveCharacter builds the DataServer 0x30
	// record (FUN_00475c30 also sends the pet save 0x34); they may adjust it.
	characterSave []func(w *World, o *Object, c *store.Character)

	// event.go
	// death run at the end of the death branch of FUN_004cda80 (the
	// map-specific calls: Blood Castle, Chaos Castle, Devil Square...).
	death []func(w *World, killer, victim *Object)
	// itemBag is CItemBagManager::DropItem (FUN_00491430): bag type,
	// bag index, map and position; set by the drop module.
	itemBag func(w *World, o *Object, bag, index, mp, x, y int) bool
	// eventEffect adds or removes an effect on an object (FUN_00478ae0 /
	// FUN_00478d30); set by the effect module. The Blood Castle gate and
	// statue use 0x3F and 0x40 while they must not be attacked.
	eventEffect func(w *World, o *Object, effect int, add bool)

	// ext.go
	// enterWorld run at the end of enterWorld, after F3 03/F3 10/F3 11
	// and the self C2 12 were sent.
	enterWorld []func(*World, *Object)
	// leaveWorld run when a playing character leaves the map
	// (logout, return to select, server move, disconnect).
	leaveWorld []func(*World, *Object)
	// start run once in New after data loading; they may load module
	// data and spawn objects. Returning an error aborts start-up.
	start []func(*World) error
	// pkExempt: a kill for which one returns true gives no PK
	// (FUN_0050f330 early returns: rival guilds, guild war, ...).
	pkExempt []func(w *World, killer, victim *Object) bool
	// move.go: walk o from (sx, sy) along direction nibbles (0..7), exactly
	// like an accepted D3 (path arrays, TX/TY, C1 08 D3 broadcast).
	setPath func(w *World, o *Object, sx, sy int, dirs []byte)
	// move.go: teleport o to (mp, x, y) inside this instance (gate/warp).
	teleport func(w *World, o *Object, mp, x, y, dir int)
	// inventory (items module): drop an item / money on the map, owned by
	// object index owner (-1 = anyone); false if the cell or map is full.
	dropMapItem  func(w *World, mp, x, y int, it item.Item, owner int) bool
	dropMapMoney func(w *World, mp, x, y int, amount int, owner int) bool
	// items module: send the current life (C1 26 FF) / mana (C1 27 FF)
	// refill packets to a player.
	sendLife func(w *World, o *Object)
	sendMana func(w *World, o *Object)
	// stats module: recompute derived stats after equipment or stat changes
	// (CalcCharacter, FUN_004c9790) and read them for combat.
	recalc      func(w *World, o *Object)
	combatStats func(w *World, o *Object) CombatStats
	// stats module: experience gain with level up (and party sharing).
	gainExperience func(w *World, o *Object, exp int64)
	// party.go/levelup.go: experience of a monster killed by a party member
	// (FUN_004c5ea0, chosen by FUN_004cda80 when the killer is in a party):
	// adds experience, levels up and sends C3 18 9C to every member in
	// range. damage is the killing hit shown in 9C. Returns false when the
	// killer has no party (use the per-attacker FUN_004c5970 instead).
	partyExperience func(w *World, killer, monster *Object, damage int, flag bool) bool
	// pet.go: experience of a kill for the equipped Dark Horse / Dark Raven
	// (FUN_004c69d0, called by FUN_004c5a30 and FUN_004c5ea0 with the
	// experience the player got, before its level up).
	petExperience func(w *World, o *Object, exp int64)
	// pk.go: a player killed another player (FUN_0050f330): PK count,
	// PK level and the C1 07 F3 08 broadcast.
	playerKill func(w *World, killer, victim *Object)
	// guild_war.go: FUN_004864b0, the guilds of a and b are at war with
	// each other (PvP may attack).
	guildWar func(w *World, a, b *Object) bool
	// guild_war.go: FUN_004868f0, a player killed another one; true when
	// the kill counted for a guild war (the death path FUN_004cda80 sets
	// victim obj+0x285 = 2 then).
	guildWarKill func(w *World, killer, victim *Object) bool
	// guild.go: the guild viewport packets (C2 65 for new players,
	// C2 67 for visible ones) at the end of the viewport send.
	guildViewport func(w *World, o *Object)
	// guild.go: the guild part of the player re-show after a teleport
	// (FUN_00516f60 / FUN_00517080).
	guildReshow func(w *World, o *Object)
	// NPC talk (30): the monsters module dispatches by NPC class; the items
	// module registers shops and the warehouse.
	npcTalk map[int]func(w *World, o, npc *Object) bool

	// freebies.go
	// newCharacter runs after the world entry of a character whose
	// IsNewChar column was set (enterworld.go).
	newCharacter func(w *World, o *Object)

	// guild.go
	// castleOwner reports whether a guild owns the castle (FUN_00429b00,
	// Castle Siege module).
	castleOwner func(w *World, guild string) bool

	// invasion.go
	// mapEventState builds the C1 05 0B packet sent on map entry (reshow).
	mapEventState func(w *World, o *Object) []byte

	// killcount.go
	// monsterKill runs in the monster drop FUN_004b7290 for a monster
	// that is not a summon, with its top hitter (drop.go).
	monsterKill func(w *World, mon, p *Object)

	// lukehelper.go
	// npcPreTalk runs after the quest talk and before the NpcTalk lines
	// (FUN_004c2eb0); true ends the talk (npc.go).
	npcPreTalk func(w *World, o, npc *Object) bool

	// mapitem.go
	// itemViewport is called by the 1-second viewport cycle.
	itemViewport func(w *World, o *Object, phase int)
	// sameParty reports whether two players are in the same party (party
	// module; nil means never).
	sameParty func(w *World, a, b *Object) bool
	// itemPicked / itemDropped let the events follow special items
	// (FUN_00416750 / FUN_004166b0).
	itemPicked  []func(w *World, o *Object, it item.Item)
	itemDropped []func(w *World, o *Object, it item.Item, mapItem int)

	// monster_skill.go
	// monsterSkillAttack is CSkillManager::UseAttackSkill
	// (FUN_004f8dd0) for a monster casting skill on t; the skills module
	// sets it. Without it the skill is resolved as a plain CAttack with the
	// skill number.
	monsterSkillAttack func(w *World, o, t *Object, skill int) bool
	// addEffect / removeEffect are the buff manager calls the
	// monster skill elements use (FUN_00478ae0 add effect with duration in
	// seconds, FUN_00478d30 remove); set by the buff module.
	addEffect    func(w *World, o *Object, effect, value, seconds int) bool
	removeEffect func(w *World, o *Object, effect int)

	// monsterschedule.go
	// npcSchedule is FUN_004bb8d0; true ends the NPC talk.
	npcSchedule func(w *World, o, npc *Object) bool

	// move.go
	// moveEffect reports whether o has effect index e active (the
	// obj+0x5fc effect list, FUN_00478ec0). The effects module sets it.
	moveEffect func(w *World, o *Object, e int) bool
	// chaosCastleState returns the state of Chaos Castle room 0..5
	// (DAT_010d6130 + room*0x748); the Chaos Castle module sets it.
	chaosCastleState func(w *World, room int) int

	// npc.go
	// npcClose runs for 31 (close the NPC window): the module that opened
	// the window (shop, warehouse, chaos box...) resets its interface state.
	npcClose func(w *World, o *Object)

	// quest.go
	// questTalk runs before the NpcTalk.txt messages (npc.go).
	questTalk func(w *World, o, npc *Object) bool
	// questDrop is the quest item drop of the monster death
	// (combat.go): true replaces the normal drop.
	questDrop func(w *World, mon *Object) bool
	// questEffect applies a reward of type 7 (FUN_00478ae0, effect
	// index and duration); set by the effect module.
	questEffect func(w *World, o *Object, effect, value int)

	// shop.go
	// tradeCancel is set by trade.go.
	tradeCancel func(w *World, o *Object)
	// spawnNPC creates an NPC of class at (mp, x, y, dir) and returns it
	// (monsters module). CShopManager spawns the shop NPCs itself
	// (FUN_004f55e0): they are not in Data/Monster/Spawn.
	spawnNPC func(w *World, class, mp, x, y, dir int) *Object

	// skill.go
	// guildStatus overrides obj+0x30c (default: guild.go Status, -1 without
	// a guild); used by tests.
	guildStatus func(w *World, o *Object) int

	// skillspeed.go
	// skillSpeed is FUN_00488550 for a player and a skill: false stops
	// the caller.
	skillSpeed func(w *World, o *Object, skill int) bool

	// summon.go
	// summonSkill is called by the skills module when a player casts one
	// of the summon skills (30..36, 432); true when a monster was summoned.
	summonSkill func(w *World, o *Object, skill int) bool

	// summonscroll.go
	// summonScroll is the SummonScroll branch of the 23 handler
	// (drop.go): handled when the item is a scroll; ok when it was used
	// (removed from the inventory), otherwise the drop is refused.
	summonScroll func(w *World, o *Object, it item.Item, x, y int) (handled, ok bool)

	// teleport.go
	// castleBattle reports the Castle Siege battle (FUN_004c5220,
	// DAT_010d8d94 == 7); the castle module sets it.
	castleBattle func(w *World) bool
	// beforeGateMove run at the start of a player's gate move: the
	// original closes trade, personal shop, warehouse, duel and others there
	// (FUN_0045cfb0, FUN_0045f680, FUN_004d1580, FUN_004e5260, FUN_00503ee0,
	// FUN_005076f0; not identified one by one yet).
	beforeGateMove []func(w *World, o *Object)
	// mapChange run when a gate move changes the map (FUN_00493680 +
	// FUN_004a3480, not identified yet).
	mapChange []func(w *World, o *Object, from, to int)

	// tick.go
	// tick100 are registered by the movement and monster modules.
	tick100  []func(*World)
	tick1000 []func(*World)
	// viewportEnd run after the 1-second viewport cycle (the tail of
	// FUN_0050a020: FUN_004c3b10 state transitions).
	viewportEnd []func(*World)

	// trade.go
	// guildNumber returns obj+0x308 (guild number) for the 37 packet.
	guildNumber func(w *World, o *Object) uint32

	// viewport.go
	monsterViewport func(*World, *Object)

	// warehouse.go
	// warehouseLoaded is set by pet.go (DS 0x09 for the vault pets).
	warehouseLoaded func(w *World, o *Object)
	// warehouseSave is set by pet.go (DS 0x34 for the vault pets).
	warehouseSave func(w *World, o *Object)

	// windowtime.go
	// windowClose runs in the DF position fix (move.go).
	windowClose func(w *World, o *Object)
}

// newHooks builds the hook table of one World. The register functions run
// in a fixed order, which matters for the hook lists (enterWorld,
// leaveWorld, start, tick100, tick1000, ...): they run in registration order.
func newHooks() *hooks {
	h := &hooks{
		commandHandlers: map[int]func(w *World, o *Object, c command, arg string){
			0: (*World).commandMove,
			4: (*World).commandPost,
		},
		hardCodedHandlers: map[string]func(w *World, o *Object, arg string){},
		eventTimeHandlers: map[byte]func(w *World, o *Object, p []byte) []byte{},
		worldHandlers:     map[byte]func(*World, *Object, []byte){},
		faHandlers:        map[byte]func(*World, *Object, []byte){},
		fbHandlers:        map[byte]func(*World, *Object, []byte){},
		f3Handlers:        map[byte]func(*World, *Object, []byte){},
		npcTalk:           map[int]func(w *World, o, npc *Object) bool{},
	}
	registerBlacklist(h)
	registerBloodcastle(h)
	registerBonus(h)
	registerCastledeep(h)
	registerCastlesiege(h)
	registerCastlesiegeBattle(h)
	registerCastlesiegeHooks(h)
	registerCastlesiegeTax(h)
	registerChaos(h)
	registerChaoscastle(h)
	registerChat(h)
	registerCmdrequirement(h)
	registerCombat(h)
	registerCommandCustom(h)
	registerCommandGm(h)
	registerCommandHardcoded(h)
	registerCommandPlayer(h)
	registerCommandPlayer2(h)
	registerCreationgold(h)
	registerCustomWorld(h)
	registerCustomWorldExt(h)
	registerCustomarena(h)
	registerCustomattack(h)
	registerCustomevent(h)
	registerHideSeekCommands(h)
	registerCustomeventDuel(h)
	registerCustomeventPair(h)
	registerCustomeventRoleta(h)
	registerCustomeventSobre(h)
	registerCustomeventdrop(h)
	registerCustompick(h)
	registerCustomquiz(h)
	registerCustomstore(h)
	registerDailyreward(h)
	registerDatafiles(h)
	registerDatauses(h)
	registerDevilsquare(h)
	registerEventTimeHandler(h)
	registerDrop(h)
	registerDropevent(h)
	registerDuel(h)
	registerEffect(h)
	registerEffectSave(h)
	registerEvent(h)
	registerEventEntry(h)
	registerFreebies(h)
	registerFriend(h)
	registerGhrs(h)
	registerGoldenarcher(h)
	registerGuild(h)
	registerGuildSoccer(h)
	registerGuildWar(h)
	registerHealthbar(h)
	registerHub(h)
	registerIllusiontemple(h)
	registerIntegration(h)
	registerInvasion(h)
	registerInventory(h)
	registerItemmove(h)
	registerItemuse(h)
	registerJewelbank(h)
	registerKalima(h)
	registerKillcount(h)
	registerLevelup(h)
	registerLua(h)
	registerLuckywheel(h)
	registerLukehelper(h)
	registerMapitem(h)
	registerMapItemHandlers(h)
	registerMonster(h)
	registerSpawnNPC(h)
	registerMonsterSkill(h)
	registerMonsterschedule(h)
	registerMove(h)
	registerNotice(h)
	registerNpc(h)
	registerOffline(h)
	registerOptions(h)
	registerParty(h)
	registerPartyLoot(h)
	registerPatente(h)
	registerPeriodic(h)
	registerPet(h)
	registerPetRaven(h)
	registerPk(h)
	registerPshop(h)
	registerPushback(h)
	registerPvp(h)
	registerQuest(h)
	registerReconnect(h)
	registerShop(h)
	registerNPCClose(h)
	registerShopSpawn(h)
	registerSkill(h)
	registerSkillGuild(h)
	registerSkillspeed(h)
	registerStats(h)
	registerSummon(h)
	registerSummonscroll(h)
	registerTeleport(h)
	registerTrade(h)
	registerWarehouse(h)
	registerWindowtime(h)
	registerXshop(h)
	registerXshopRecharge(h)
	return h
}
