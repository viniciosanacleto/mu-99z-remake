package game

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
)

// Data coverage test (docs/server-plan/16-cobertura-dados.md): every file
// under apps/server/config (the JSON converted from MuServer/Data, the
// instance DATA folders and the ConnectServer / JoinServer / DataServer
// files) and apps/server/data/Data (the files kept in their original
// format) must be either read by the original GameServer.exe /
// CastleSiege.exe and parsed by a Go loader (which must succeed on the
// shipped file), listed as pending (read by the original, no Go parser
// yet), or ignored with a reason. A new file that is none of these fails
// the test.
//
// The test lives in package game because several loaders are unexported
// here; the rest are in internal/gamedata and internal/config.

// covEnv gives loaders the data directories and a world built from them.
type covEnv struct {
	t      *testing.T
	data   string // apps/server/config/data
	assets string // apps/server/data/Data
	w      *World
}

// covLoader is a Go loader for one or more files and the original reader
// (loader of FUN_004e8520..FUN_004ec4c0, or the CastleSiege.exe function).
type covLoader struct {
	name, orig string
	load       func(c *covEnv) error
}

func ld(name, orig string, f func(c *covEnv) error) covLoader { return covLoader{name, orig, f} }

func errOnly[T any](_ T, err error) error { return err }

// Loaders by file (paths relative to apps/server). optionalFiles may be
// absent from this installation (the original reads them if present).
var (
	ldClassInfo = ld("gamedata.LoadClassInfo", "FUN_004e85f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadClassInfo(c.data))
	})
)

var coveredFiles = map[string]covLoader{
	"config/server.json": ld("jsonconf.LoadServer + connect / serverlist / DataServer BadSyntax", "ConnectServer.exe, JoinServer.exe, DataServer.exe", func(c *covEnv) error {
		_, err := loadBadSyntax(repoConfig("server.json"))
		return err
	}),
	"config/data/BlackList.json": ld("gamedata.LoadBlackList", "FUN_004e8520 -> FUN_00411ae0", func(c *covEnv) error {
		return errOnly(gamedata.LoadBlackList(c.data))
	}),
	"config/data/Character/DefaultClassFreebies.json": ld("gamedata.LoadDefaultClassFreebies", "FUN_004e85f0 -> FUN_0046cb40", func(c *covEnv) error {
		return errOnly(gamedata.LoadDefaultClassFreebies(c.data))
	}),
	"config/data/Character/DefaultClassInfo.json": ldClassInfo,
	"config/data/CommandManager.json": ld("game.loadCommands", "FUN_004e86f0", func(c *covEnv) error {
		return errOnly(loadCommands(c.data))
	}),
	"config/data/CommandRequirement.json": ld("gamedata.LoadCommandRequirement", "FUN_004e86f0 -> FUN_004402a0", func(c *covEnv) error {
		return errOnly(gamedata.LoadCommandRequirement(c.data))
	}),
	"config/data/ConnectMember.json": ld("game.loadNameList", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(loadNameList(filepath.Join(c.data, "ConnectMember.json")))
	}),
	"config/data/MapManager.json": ld("gamedata.LoadMapManager", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapManager(c.data))
	}),
	"config/data/MapRateInfo.json": ld("gamedata.LoadMapRateInfo", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapRateInfo(c.data))
	}),

	// FUN_004e8a50: Custom.
	"config/data/Custom/CustomArena.json":      ldc("CustomArena", gamedata.LoadCustomArena),
	"config/data/Custom/CustomAttack.json":     ldc("CustomAttack", gamedata.LoadCustomAttack),
	"config/data/Custom/CustomBuyVip.json":     ldc("CustomBuyVip", gamedata.LoadCustomBuyVip),
	"config/data/Custom/CustomEventDrop.json":  ldc("CustomEventDrop", gamedata.LoadCustomEventDrop),
	"config/data/Custom/CustomGift.json":       ldc("CustomGift", gamedata.LoadCustomGift),
	"config/data/Custom/CustomHealthBar.json":  ldc("CustomHealthBar", gamedata.LoadCustomHealthBar),
	"config/data/Custom/CustomJewel.json":      ldc("CustomJewel", gamedata.LoadCustomJewel),
	"config/data/Custom/CustomLukeHelper.json": ldc("CustomLukeHelper", gamedata.LoadCustomLukeHelper),
	"config/data/Custom/CustomMarriage.json":   ldc("CustomMarriage", gamedata.LoadCustomMarriage),
	"config/data/Custom/CustomMonster.json": ld("gamedata.LoadCustomMonsters + LoadCustomMonsterRows", "FUN_004e8a50 -> FUN_00466b30", func(c *covEnv) error {
		if _, err := gamedata.LoadCustomMonsters(c.data); err != nil {
			return err
		}
		return errOnly(gamedata.LoadCustomMonsterRows(c.data))
	}),
	"config/data/Custom/CustomMonsterKillCount.json": ldc("CustomMonsterKillCount", gamedata.LoadCustomMonsterKillCount),
	"config/data/Custom/CustomPick.json":             ldc("CustomPick", gamedata.LoadCustomPick),
	"config/data/Custom/CustomWing.json": ld("gamedata.LoadCustomWings + game.loadCustomTables", "FUN_004e8a50", func(c *covEnv) error {
		if _, err := gamedata.LoadCustomWings(c.data); err != nil {
			return err
		}
		return errOnly(loadCustomTables(c.data))
	}),
	"config/data/Custom/CustomPet.json": ld("game.loadCustomTables", "FUN_004e8a50", func(c *covEnv) error {
		return errOnly(loadCustomTables(c.data))
	}),
	"config/data/Custom/CustomXShop.json": ld("gamedata.LoadXShopPrices", "FUN_004e8a50", func(c *covEnv) error {
		_, ok, err := gamedata.LoadXShopPrices(filepath.Join(c.data, "Custom", "CustomXShop.json"))
		if err == nil && !ok {
			c.t.Error("CustomXShop.json not opened")
		}
		return err
	}),
	"config/data/Custom/CustomXShopRecharge.json": ld("game.rechargeBlock", "FUN_00404a60 (read on request, next to CustomXShop.txt)", func(c *covEnv) error {
		path := c.w.rechargePath()
		if _, err := jsonconf.LoadINI(path); err != nil {
			return err
		}
		c.w.rechargeBlock(path)
		return nil
	}),
	"config/data/Custom/CreationGold.json": ld("gamedata.CreationGoldValue", "FUN_0049a690 (read on use)", func(c *covEnv) error {
		_, err := jsonconf.LoadINI(filepath.Join(c.data, "Custom", "CreationGold.json"))
		gamedata.CreationGoldValue(c.data)
		return err
	}),
	"config/data/Custom/CustomRespawn.json":      ldc("CustomRespawn", gamedata.LoadCustomRespawn),
	"config/data/Custom/CustomMove.json":         ldc("CustomMove", gamedata.LoadCustomMove),
	"config/data/Custom/CustomGate.json":         ldc("CustomGate", gamedata.LoadCustomGate),
	"config/data/Custom/CustomNPCCommand.json":   ldc("CustomNPCCommand", gamedata.LoadCustomNPCCommand),
	"config/data/Custom/CustomDeathMessage.json": ldc("CustomDeathMessage", gamedata.LoadCustomDeathMessage),
	"config/data/Custom/CustomAttribute.json":    ldc("CustomAttributes", gamedata.LoadCustomAttributes),
	"config/data/Custom/SummonScroll.json":       ldc("SummonScroll", gamedata.LoadSummonScroll),
	"config/data/Custom/Event/DuelEvent.json":    ldc("DuelEvent", gamedata.LoadDuelEvent),
	"config/data/Custom/Event/DeathEvent.json":   ldc("DeathEvent", gamedata.LoadDeathEvent),
	"config/data/Custom/Event/RoletaEvent.json":  ldc("RoletaEvent", gamedata.LoadRoletaEvent),
	"config/data/Custom/Event/RaffleEvent.json":  ldc("RaffleEvent", gamedata.LoadRaffleEvent),
	"config/data/Custom/Event/PairEvent.json":    ldc("PairEvent", gamedata.LoadPairEvent),
	"config/data/Custom/Event/SobreEvent.json":   ldc("SobreEvent", gamedata.LoadSobreEvent),
	"config/data/Custom/Event/TheftEvent.json":   ldc("TheftEvent", gamedata.LoadTheftEvent),
	"config/data/Custom/CustomGuard.json":        ldc("CustomGuard", gamedata.LoadCustomGuard),
	"config/data/Custom/CustomMobCombo.json":     ldc("CustomMobCombo", gamedata.LoadCustomMobCombo),
	"config/data/Custom/CustomLuckyWheel.json":   ldc("LuckyWheel", gamedata.LoadLuckyWheel),
	"config/data/Custom/DailyReward.json":        ldc("DailyReward", gamedata.LoadDailyReward),
	"config/data/Custom/CustomPatente.json":      ldc("CustomPatente", gamedata.LoadCustomPatente),
	"config/data/Custom/CustomEvolution.json":    ldc("CustomEvolution", gamedata.LoadCustomEvolution),
	"config/data/Custom/CustomGHRS.json": ld("gamedata.LoadCustomGHRS", "CastleSiege.exe FUN_00442c10", func(c *covEnv) error {
		return errOnly(gamedata.LoadCustomGHRS(c.data))
	}),

	// FUN_004e9d30: Event.
	"config/data/Event/BloodCastle.json":     lde("BloodCastle", gamedata.LoadBloodCastle),
	"config/data/Event/BonusManager.json":    ld("gamedata.LoadBonusManager", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadBonusManager(c.data); return err }),
	"config/data/Event/CastleDeepEvent.json": lde("CastleDeep", gamedata.LoadCastleDeep),
	"config/data/Event/CastleSiege.json":     lde("CastleSiege", gamedata.LoadCastleSiege),
	"config/data/Event/ChaosCastle.json":     lde("ChaosCastle", gamedata.LoadChaosCastle),
	"config/data/Event/DevilSquare.json":     lde("DevilSquare", gamedata.LoadDevilSquare),
	"config/data/Event/DropEvent.json":       ld("gamedata.LoadDropEvent", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadDropEvent(c.data); return err }),
	"config/data/Event/EventEntryLevel.json": lde("EventEntryLevel", gamedata.LoadEventEntryLevel),
	"config/data/Event/EventStageSpawn.json": lde("EventStageSpawn", gamedata.LoadEventStageSpawn),
	"config/data/Event/GoldenArcher.json":    lde("GoldenArcher", gamedata.LoadGoldenArcher),
	"config/data/Event/InvasionManager.json": lde("Invasions", gamedata.LoadInvasions),
	"config/data/Event/Kalima.json":          lde("Kalima", gamedata.LoadKalima),
	"config/data/Custom/CustomQuiz.json":     lde("CustomQuiz", gamedata.LoadCustomQuiz),

	"config/data/EventItemBagManager.json": ld("gamedata.LoadEventItemBags", "FUN_004ea4a0", func(c *covEnv) error {
		return errOnly(gamedata.LoadEventItemBags(c.data))
	}),

	// FUN_004ea5d0: Hack.
	"config/data/Hack/HackPacketCheck.json": ld("game.loadHackRules", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(loadHackRules(c.data))
	}),
	"config/data/Hack/HackSkillSpeedCheck.json": ld("gamedata.LoadHackSkillSpeedCheck", "FUN_004ea5d0 -> FUN_00488340", func(c *covEnv) error {
		return errOnly(gamedata.LoadHackSkillSpeedCheck(c.data))
	}),
	"data/Data/Hack/Enc2.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.assets, "Hack", "Enc2.dat")))
	}),
	"data/Data/Hack/Dec1.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.assets, "Hack", "Dec1.dat")))
	}),

	// FUN_004ea820: Item.
	"config/data/Item/Item.json":                ldi("gamedata.LoadItems", gamedata.LoadItems),
	"config/data/Item/ExcellentOptionRate.json": ldi("gamedata.LoadExcellentOptionRate", gamedata.LoadExcellentOptionRate),
	"config/data/Item/ItemDrop.json":            ldi("gamedata.LoadItemDrop", gamedata.LoadItemDrop),
	"config/data/Item/ItemLevel.json":           ldi("gamedata.LoadItemLevel", gamedata.LoadItemLevel),
	"config/data/Item/ItemLoot.json":            ldi("gamedata.LoadItemLoot", gamedata.LoadItemLoot),
	"config/data/Item/ItemMoneyDrop.json":       ldi("gamedata.LoadItemMoneyDrop", gamedata.LoadItemMoneyDrop),
	"config/data/Item/ItemMove.json":            ldi("gamedata.LoadItemMove", gamedata.LoadItemMove),
	"config/data/Item/ItemOption.json":          ldi("gamedata.LoadItemOptions", gamedata.LoadItemOptions),
	"config/data/Item/ItemOptionRate.json":      ldi("gamedata.LoadItemOptionRate", gamedata.LoadItemOptionRate),
	"config/data/Item/ItemStack.json":           ldi("gamedata.LoadItemStack", gamedata.LoadItemStack),
	"config/data/Item/ItemValue.json":           ldi("gamedata.LoadItemValue", gamedata.LoadItemValue),
	"config/data/Item/SetItemOption.json":       ldi("gamedata.LoadSetItemOption", gamedata.LoadSetItemOption),
	"config/data/Item/SetItemType.json":         ldi("gamedata.LoadSetItemType", gamedata.LoadSetItemType),
	"config/data/Item/WingOption.json":          ldi("gamedata.LoadWingOptions", gamedata.LoadWingOptions),

	// FUN_004eb000: Lang.
	"config/data/Lang/Eng/Message.json": ldMessages, "config/data/Lang/Por/Message.json": ldMessages, "config/data/Lang/Spn/Message.json": ldMessages,
	"config/data/Lang/Eng/Notice.json": ldNotices, "config/data/Lang/Por/Notice.json": ldNotices, "config/data/Lang/Spn/Notice.json": ldNotices,

	"config/data/MapServerInfo.json": ld("gamedata.LoadMapServer", "FUN_004eb520", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapServer(c.data))
	}),

	// FUN_004eb5f0: Monster.
	"config/data/Monster/MonsterList.json": ld("gamedata.LoadMonsterList", "FUN_004eb5f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterList(c.data, gamedata.MonsterRates{}))
	}),
	"config/data/Monster/MonsterMove.json": ld("gamedata.LoadMonsterMove", "FUN_004eb5f0 -> FUN_004b1110", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterMove(c.data))
	}),
	"config/data/Monster/MonsterSchedule.json": ld("gamedata.LoadMonsterSchedule", "FUN_004eb5f0 -> FUN_004bb0c0", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterSchedule(c.data))
	}),
	"config/data/Monster/Skill/MonsterSkillElement.json": ldMonsterSkills,
	"config/data/Monster/Skill/MonsterSkillUnit.json":    ldMonsterSkills,
	"config/data/Monster/Skill/MonsterSkill.json":        ldMonsterSkills,
	"config/data/Monster/NpcTalk.json": ld("gamedata.LoadNpcTalk", "FUN_004eb5f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadNpcTalk(c.data))
	}),

	// FUN_004eba60: Move.
	"config/data/Move/Gate.json": ld("gamedata.LoadGates", "FUN_004eba60", func(c *covEnv) error {
		return errOnly(gamedata.LoadGates(c.data))
	}),
	"config/data/Move/Move.json": ld("game.loadMoves", "FUN_004eba60", func(c *covEnv) error {
		return errOnly(loadMoves(c.data))
	}),
	"config/data/Move/MoveSummon.json": ld("gamedata.LoadMoveSummon", "FUN_004eba60 -> FUN_004bfd00", func(c *covEnv) error {
		return errOnly(gamedata.LoadMoveSummon(c.data))
	}),

	// FUN_004ebc50: Quest.
	"config/data/Quest/Quest.json": ldQuests, "config/data/Quest/QuestObjective.json": ldQuests, "config/data/Quest/QuestReward.json": ldQuests,

	"config/data/ShopManager.json": ld("gamedata.LoadShops", "FUN_004ebfc0", func(c *covEnv) error {
		return errOnly(gamedata.LoadShops(c.data))
	}),

	// FUN_004ec0f0: Skill.
	"config/data/Skill/ComboSkill.json": ldSkillWorld, "config/data/Skill/SkillDamage.json": ldSkillWorld,
	"data/Data/Skill/SkillElect.hit": ldSkillWorld, "config/data/Skill/SkillEnabler.json": ldSkillWorld,
	"config/data/Skill/SkillUseArea.json": ldSkillWorld,
	"config/data/Skill/EffectList.json": ld("game.loadEffectList", "FUN_004ec0f0", func(c *covEnv) error {
		return errOnly(loadEffectList(c.data))
	}),
	"config/data/Skill/SkillList.json": ld("gamedata.LoadSkills", "FUN_004ec0f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadSkills(c.data))
	}),

	// FUN_004ec4c0: Util.
	"config/data/Util/DamageTable.json": ld("game.loadDamageTable", "FUN_004ec4c0", func(c *covEnv) error {
		return errOnly(loadDamageTable(c.data))
	}),
	"config/data/Util/ExperienceTable.json": ld("gamedata.LoadExperienceTable", "FUN_004ec4c0 -> FUN_0047be80", func(c *covEnv) error {
		return errOnly(gamedata.LoadExperienceTable(c.data))
	}),
	"config/data/Util/Filter.json":     ldMoveData,
	"config/data/Util/GameMaster.json": ldMoveData,
	"config/data/Util/PetDamageTable.json": ld("gamedata.LoadPetDamageTable", "FUN_004ec4c0 -> FUN_004d61d0", func(c *covEnv) error {
		return errOnly(gamedata.LoadPetDamageTable(c.data))
	}),
	"config/data/Util/ResetLimit.json": ldCmdWorld,
	"config/data/Util/ResetTable.json": ldCmdWorld,
}

// optionalFiles are read by the original when present but absent here.
var optionalFiles = map[string]bool{
	"config/data/Custom/CustomPatente.json": true, // FUN_00451f90; LoadCustomPatente accepts a missing file
}

var (
	ldMessages = ld("game.loadMessages", "FUN_004eb000", func(c *covEnv) error { return errOnly(loadMessages(c.data)) })
	ldNotices  = ld("game.loadNotices", "FUN_004eb000", func(c *covEnv) error {
		return errOnly(loadNotices(c.data, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	})
	ldMonsterSkills = ld("game.loadMonsterSkills", "FUN_004eb5f0", func(c *covEnv) error { return errOnly(loadMonsterSkills(c.data)) })
	ldQuests        = ld("gamedata.LoadQuests", "FUN_004ebc50", func(c *covEnv) error { return errOnly(gamedata.LoadQuests(c.data)) })
	ldSkillWorld    = ld("game.loadSkillWorld", "FUN_004ec0f0", func(c *covEnv) error { return errOnly(loadSkillWorld(c.data, c.assets)) })
	ldMoveData      = ld("game.loadMoveData", "FUN_004ec4c0", func(c *covEnv) error { return loadMoveData(c.w) })
	ldCmdWorld      = ld("game.loadCmdWorld", "FUN_004ec4c0", func(c *covEnv) error { return loadCmdWorld(c.w) })
	ldSpawns        = ld("gamedata.LoadSpawns", "FUN_004bbaa0 (Monster\\Spawn\\NNN - *.txt, map known)", func(c *covEnv) error {
		maps, err := gamedata.LoadMapManager(c.data)
		if err != nil {
			return err
		}
		return errOnly(gamedata.LoadSpawns(c.data, func(m int) bool { _, ok := maps[m]; return ok }, func() int { return 0 }))
	})
	ldBags = ld("gamedata.LoadEventItemBags", "FUN_00491220 (EventItemBag\\%s.txt named by the manager)", func(c *covEnv) error {
		return errOnly(gamedata.LoadEventItemBags(c.data))
	})
	ldShops = ld("gamedata.LoadShops", "FUN_004f55e0 (Shop\\%s.txt named by ShopManager)", func(c *covEnv) error {
		return errOnly(gamedata.LoadShops(c.data))
	})
	ldTerrains = ld("gamedata.LoadTerrains", "FUN_004eb3a0 (Terrain\\terrain%d.att, maps 1..100)", func(c *covEnv) error {
		return errOnly(gamedata.LoadTerrains(c.assets))
	})
	ldGSConfig = func(inst string) covLoader {
		return ld("gsconfig.Load", ".\\Data\\GameServerInfo - *.dat (GetPrivateProfile*)", func(c *covEnv) error {
			return errOnly(gsconfig.Load(repoInstance(inst)))
		})
	}
)

func ldc[T any](name string, f func(string) (T, error)) covLoader {
	return ld("gamedata.Load"+strings.TrimPrefix(name, "Load"), "FUN_004e8a50", func(c *covEnv) error { return errOnly(f(c.data)) })
}

func lde[T any](name string, f func(string) (T, error)) covLoader {
	return ld("gamedata.Load"+name, "FUN_004e9d30", func(c *covEnv) error { return errOnly(f(c.data)) })
}

func ldi[T any](name string, f func(string) (T, error)) covLoader {
	return ld(name, "FUN_004ea820", func(c *covEnv) error { return errOnly(f(c.data)) })
}

// pendingFiles are read by the original but have no Go parser yet.
var pendingFiles = map[string]string{}

// The Lua scripts run in the embedded VM (lua.go): ScriptMain.lua with the
// files it requires (FUN_004ebe20 -> FUN_004e6860, require FUN_004a5220)
// and CalcCharacter.lua in its own state (FUN_004e65e0).
func init() {
	l := ld("game.loadScripts", "FUN_004ebe20 -> FUN_004e6860 / FUN_004a5220 / FUN_004e65e0", luaCoverageLoad)
	for _, f := range []string{"ScriptMain.lua", "System/ScriptCore.lua", "Script/WelcomeMessage.lua",
		"Script/CustomWindowName.lua", "Character/CalcCharacter.lua"} {
		coveredFiles["data/Data/Script/"+f] = l
	}
}

// ignoredFiles are never read by GameServer.exe or CastleSiege.exe.
var ignoredFiles = map[string]string{
	"data/Data/Common/Protect/Decrypt.dat":                       "not referenced by any server binary",
	"data/Data/Common/Protect/Encrypt.dat":                       "not referenced by any server binary",
	"config/data/Custom/Crywolf.json":                            "not referenced (no Crywolf system in this build)",
	"config/data/Custom/CustomDailyReward.json":                  "not referenced (the original reads Custom\\DailyReward.txt)",
	"config/data/Custom/CustomMix.json":                          "not referenced by any server binary",
	"data/Data/Custom/pixFE31.tmp.bmp":                           "runtime output: GetTempFileName(\"pix\") + .bmp of CustomXShopRecharge (FUN_00404a60)",
	"config/data/Event/Crywolf.json":                             "not referenced (no Crywolf system in this build)",
	"config/data/Event/IllusionTemple.json":                      "not referenced (CIllusionTemple exists but loads no file)",
	"config/data/Event/Kanturu.json":                             "not referenced (no Kanturu system in this build)",
	"config/data/Shop/239 - Arena Guard.json":                    "not named by ShopManager.txt",
	"data/Data/Script/Script/TemplateScript.lua":                 "Lua example, not require'd by ScriptMain.lua",
	"data/Data/Script/Script/CustomMonsterDie(Boss).lua":         "require commented out in ScriptMain.lua",
	"data/Data/Script/Script/CustomMonsterDie(Boss) - Copia.lua": "copy, not require'd",
	"config/data/EventItemBag/Special/HideAndSeek.json":          "not named by EventItemBagManager.txt",
	"config/data/EventItemBag/Special/Loteria.json":              "not named by EventItemBagManager.txt",
	"config/data/EventItemBag/Special/MataMata.json":             "not named by EventItemBagManager.txt",
	"config/data/EventItemBag/Special/Quiz.json":                 "not named by EventItemBagManager.txt",
	"config/data/EventItemBag/Special/Roleta.json":               "not named by EventItemBagManager.txt",
}

// dynamicLoader classifies the families of files the original finds through
// a manager list or a name pattern.
func dynamicLoader(rel string, bags, shops map[string]bool, maps map[int]gamedata.MapInfo) (covLoader, bool) {
	switch {
	case strings.HasPrefix(rel, "config/data/Monster/Spawn/") && strings.Count(rel, "/") == 4:
		n := filepath.Base(rel)
		if len(n) > 6 && n[3:6] == " - " && strings.HasSuffix(n, ".json") {
			if mp, err := strconv.Atoi(n[:3]); err == nil {
				if _, ok := maps[mp]; ok {
					return ldSpawns, true
				}
			}
		}
	case strings.HasPrefix(rel, "config/data/EventItemBag/"):
		if bags[strings.TrimSuffix(strings.TrimPrefix(rel, "config/data/EventItemBag/"), ".json")] {
			return ldBags, true
		}
	case strings.HasPrefix(rel, "config/data/Shop/"):
		if shops[strings.TrimSuffix(strings.TrimPrefix(rel, "config/data/Shop/"), ".json")] {
			return ldShops, true
		}
	case strings.HasPrefix(rel, "data/Data/Terrain/"):
		base := strings.ToLower(filepath.Base(rel)) // the original opens "terrain%d.att"
		if strings.HasPrefix(base, "terrain") && strings.HasSuffix(base, ".att") {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "terrain"), ".att"))
			if err == nil && n >= 1 && n <= gamedata.MaxMaps {
				return ldTerrains, true
			}
		}
	case strings.HasPrefix(rel, "config/gameservers/") && strings.HasSuffix(rel, ".json"):
		// One file per instance; its sections are the GameServerInfo files.
		inst := strings.TrimSuffix(filepath.Base(rel), ".json")
		if inst == "4.GameServer" || inst == "5.GameServerCS" {
			return ldGSConfig(inst), true
		}
	}
	return covLoader{}, false
}

func TestDataFileCoverage(t *testing.T) {
	root := repoConfig("..")
	data := repoConfig("data")
	b := newBareWorld(t) // game.New runs every start hook on the shipped data
	env := &covEnv{t: t, data: data, assets: repoData("Data"), w: b.World}

	bagList, err := gamedata.LoadEventItemBags(data)
	if err != nil {
		t.Fatal(err)
	}
	bags := map[string]bool{}
	for _, e := range bagList {
		bags[strings.ReplaceAll(e.Path, "\\", "/")] = true
	}
	shopList, err := gamedata.LoadShops(data)
	if err != nil {
		t.Fatal(err)
	}
	shops := map[string]bool{}
	for _, s := range shopList {
		shops[s.Path] = true
	}
	maps, err := gamedata.LoadMapManager(data)
	if err != nil {
		t.Fatal(err)
	}

	var files []string
	for _, dir := range []string{"config", "data/Data"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Name() == ".DS_Store" {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(files)

	seen := map[string]bool{}
	called := map[string]error{}
	var loaded, ignored, pending int
	for _, rel := range files {
		seen[rel] = true
		l, ok := coveredFiles[rel]
		if !ok {
			l, ok = dynamicLoader(rel, bags, shops, maps)
		}
		switch {
		case ok:
			loaded++
			key := l.name + "|" + l.orig
			if _, done := called[key]; !done {
				called[key] = l.load(env)
			}
			if err := called[key]; err != nil {
				t.Errorf("%s: %s failed: %v", rel, l.name, err)
			}
		case ignoredFiles[rel] != "":
			ignored++
		case pendingFiles[rel] != "":
			pending++
			t.Logf("pending: %s (%s)", rel, pendingFiles[rel])
		default:
			t.Errorf("unknown data file %s: add it to coveredFiles (with its Go loader), pendingFiles or ignoredFiles", rel)
		}
	}
	for rel := range coveredFiles {
		if !seen[rel] && !optionalFiles[rel] {
			t.Errorf("coveredFiles lists %s, which does not exist", rel)
		}
	}
	for _, m := range []map[string]string{ignoredFiles, pendingFiles} {
		for rel := range m {
			if !seen[rel] {
				t.Errorf("stale entry %s: file does not exist", rel)
			}
		}
	}
	t.Logf("%d files: %d loaded by Go, %d pending, %d ignored", len(files), loaded, pending, ignored)
}

// TestDataFileCoverageUnknown checks the classification rejects a file
// nobody claims and accepts the dynamic families.
func TestDataFileCoverageUnknown(t *testing.T) {
	maps := map[int]gamedata.MapInfo{0: {}}
	bags := map[string]bool{"Item/Box of Luck": true}
	shops := map[string]bool{"253 - Potion Girl Amy": true}
	cases := map[string]bool{
		"config/data/Monster/Spawn/000 - Lorencia.json":     true,
		"config/data/Monster/Spawn/000 - Lorencia.txt":      false, // not converted
		"config/data/Monster/Spawn/077 - Unknown.json":      false, // map not in MapManager
		"config/data/Monster/Spawn/backup/006 - Arena.json": false, // subfolders are skipped
		"config/data/EventItemBag/Item/Box of Luck.json":    true,
		"config/data/EventItemBag/Item/New Box.json":        false,
		"config/data/Shop/253 - Potion Girl Amy.json":       true,
		"config/data/Shop/999 - New.json":                   false,
		"data/Data/Terrain/Terrain12.att":                   true,
		"data/Data/Terrain/Terrain101.att":                  false,
		"config/gameservers/4.GameServer.json":              true,
		"config/gameservers/6.GameServerNew.json":           false,
		"config/data/Brand/New.json":                        false,
	}
	for rel, want := range cases {
		_, ok := dynamicLoader(rel, bags, shops, maps)
		if _, c := coveredFiles[rel]; c {
			ok = true
		}
		if ok != want {
			t.Errorf("%s: classified %v, want %v", rel, ok, want)
		}
	}
}
