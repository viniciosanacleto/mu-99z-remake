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
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/ini"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
)

// Data coverage test (docs/server-plan/16-cobertura-dados.md): every file
// under apps/server/data/Data and the instance DATA folders must be either
// read by the original GameServer.exe / CastleSiege.exe and parsed by a Go
// loader (which must succeed on the shipped file), listed as pending (read
// by the original, no Go parser yet), or ignored with a reason. A new file
// that is none of these fails the test.
//
// The test lives in package game because several loaders are unexported
// here; the rest are in internal/gamedata and internal/config.

// covEnv gives loaders the data directories and a world built from them.
type covEnv struct {
	t    *testing.T
	data string // apps/server/data/Data
	w    *World
}

// covLoader is a Go loader for one or more files and the original reader
// (loader of FUN_004e8520..FUN_004ec4c0, or the CastleSiege.exe function).
type covLoader struct {
	name, orig string
	load       func(c *covEnv) error
}

func ld(name, orig string, f func(c *covEnv) error) covLoader { return covLoader{name, orig, f} }

func errOnly[T any](_ T, err error) error { return err }

// Loaders by file (paths relative to apps/server/data). optionalFiles may be
// absent from this installation (the original reads them if present).
var (
	ldClassInfo = ld("gamedata.LoadClassInfo", "FUN_004e85f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadClassInfo(c.data))
	})
)

var coveredFiles = map[string]covLoader{
	"Data/BlackList.txt": ld("gamedata.LoadBlackList", "FUN_004e8520 -> FUN_00411ae0", func(c *covEnv) error {
		return errOnly(gamedata.LoadBlackList(c.data))
	}),
	"Data/Character/DefaultClassFreebies.txt": ld("gamedata.LoadDefaultClassFreebies", "FUN_004e85f0 -> FUN_0046cb40", func(c *covEnv) error {
		return errOnly(gamedata.LoadDefaultClassFreebies(c.data))
	}),
	"Data/Character/DefaultClassInfo.txt": ldClassInfo,
	"Data/CommandManager.txt": ld("game.loadCommands", "FUN_004e86f0", func(c *covEnv) error {
		return errOnly(loadCommands(c.data))
	}),
	"Data/CommandRequirement.txt": ld("gamedata.LoadCommandRequirement", "FUN_004e86f0 -> FUN_004402a0", func(c *covEnv) error {
		return errOnly(gamedata.LoadCommandRequirement(c.data))
	}),
	"Data/ConnectMember.txt": ld("game.loadNameList", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(loadNameList(filepath.Join(c.data, "ConnectMember.txt")))
	}),
	"Data/MapManager.txt": ld("gamedata.LoadMapManager", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapManager(c.data))
	}),
	"Data/MapRateInfo.txt": ld("gamedata.LoadMapRateInfo", "FUN_004e8860", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapRateInfo(c.data))
	}),

	// FUN_004e8a50: Custom.
	"Data/Custom/CustomArena.txt":      ldc("CustomArena", gamedata.LoadCustomArena),
	"Data/Custom/CustomAttack.txt":     ldc("CustomAttack", gamedata.LoadCustomAttack),
	"Data/Custom/CustomBuyVip.txt":     ldc("CustomBuyVip", gamedata.LoadCustomBuyVip),
	"Data/Custom/CustomEventDrop.txt":  ldc("CustomEventDrop", gamedata.LoadCustomEventDrop),
	"Data/Custom/CustomGift.txt":       ldc("CustomGift", gamedata.LoadCustomGift),
	"Data/Custom/CustomHealthBar.txt":  ldc("CustomHealthBar", gamedata.LoadCustomHealthBar),
	"Data/Custom/CustomJewel.txt":      ldc("CustomJewel", gamedata.LoadCustomJewel),
	"Data/Custom/CustomLukeHelper.txt": ldc("CustomLukeHelper", gamedata.LoadCustomLukeHelper),
	"Data/Custom/CustomMarriage.txt":   ldc("CustomMarriage", gamedata.LoadCustomMarriage),
	"Data/Custom/CustomMonster.txt": ld("gamedata.LoadCustomMonsters + LoadCustomMonsterRows", "FUN_004e8a50 -> FUN_00466b30", func(c *covEnv) error {
		if _, err := gamedata.LoadCustomMonsters(c.data); err != nil {
			return err
		}
		return errOnly(gamedata.LoadCustomMonsterRows(c.data))
	}),
	"Data/Custom/CustomMonsterKillCount.txt": ldc("CustomMonsterKillCount", gamedata.LoadCustomMonsterKillCount),
	"Data/Custom/CustomPick.txt":             ldc("CustomPick", gamedata.LoadCustomPick),
	"Data/Custom/CustomWing.txt": ld("gamedata.LoadCustomWings + game.loadCustomTables", "FUN_004e8a50", func(c *covEnv) error {
		if _, err := gamedata.LoadCustomWings(c.data); err != nil {
			return err
		}
		return errOnly(loadCustomTables(c.data))
	}),
	"Data/Custom/CustomPet.txt": ld("game.loadCustomTables", "FUN_004e8a50", func(c *covEnv) error {
		return errOnly(loadCustomTables(c.data))
	}),
	"Data/Custom/CustomXShop.txt": ld("gamedata.LoadXShopPrices", "FUN_004e8a50", func(c *covEnv) error {
		_, ok, err := gamedata.LoadXShopPrices(filepath.Join(c.data, "Custom", "CustomXShop.txt"))
		if err == nil && !ok {
			c.t.Error("CustomXShop.txt not opened")
		}
		return err
	}),
	"Data/Custom/CustomXShopRecharge.txt": ld("game.rechargeBlock", "FUN_00404a60 (read on request, next to CustomXShop.txt)", func(c *covEnv) error {
		path := c.w.rechargePath()
		if _, err := ini.Load(path); err != nil {
			return err
		}
		c.w.rechargeBlock(path)
		return nil
	}),
	"Data/Custom/CreationGold.txt": ld("gamedata.CreationGoldValue", "FUN_0049a690 (read on use)", func(c *covEnv) error {
		_, err := ini.Load(filepath.Join(c.data, "Custom", "CreationGold.txt"))
		gamedata.CreationGoldValue(c.data)
		return err
	}),
	"Data/Custom/CustomRespawn.txt":      ldc("CustomRespawn", gamedata.LoadCustomRespawn),
	"Data/Custom/CustomMove.txt":         ldc("CustomMove", gamedata.LoadCustomMove),
	"Data/Custom/CustomGate.txt":         ldc("CustomGate", gamedata.LoadCustomGate),
	"Data/Custom/CustomNPCCommand.txt":   ldc("CustomNPCCommand", gamedata.LoadCustomNPCCommand),
	"Data/Custom/CustomDeathMessage.txt": ldc("CustomDeathMessage", gamedata.LoadCustomDeathMessage),
	"Data/Custom/CustomAttribute.txt":    ldc("CustomAttributes", gamedata.LoadCustomAttributes),
	"Data/Custom/SummonScroll.txt":       ldc("SummonScroll", gamedata.LoadSummonScroll),
	"Data/Custom/Event/DuelEvent.txt":    ldc("DuelEvent", gamedata.LoadDuelEvent),
	"Data/Custom/Event/DeathEvent.txt":   ldc("DeathEvent", gamedata.LoadDeathEvent),
	"Data/Custom/Event/RoletaEvent.txt":  ldc("RoletaEvent", gamedata.LoadRoletaEvent),
	"Data/Custom/Event/RaffleEvent.txt":  ldc("RaffleEvent", gamedata.LoadRaffleEvent),
	"Data/Custom/Event/PairEvent.txt":    ldc("PairEvent", gamedata.LoadPairEvent),
	"Data/Custom/Event/SobreEvent.txt":   ldc("SobreEvent", gamedata.LoadSobreEvent),
	"Data/Custom/Event/TheftEvent.txt":   ldc("TheftEvent", gamedata.LoadTheftEvent),
	"Data/Custom/CustomGuard.txt":        ldc("CustomGuard", gamedata.LoadCustomGuard),
	"Data/Custom/CustomMobCombo.txt":     ldc("CustomMobCombo", gamedata.LoadCustomMobCombo),
	"Data/Custom/CustomLuckyWheel.txt":   ldc("LuckyWheel", gamedata.LoadLuckyWheel),
	"Data/Custom/DailyReward.txt":        ldc("DailyReward", gamedata.LoadDailyReward),
	"Data/Custom/CustomPatente.txt":      ldc("CustomPatente", gamedata.LoadCustomPatente),
	"Data/Custom/CustomEvolution.txt":    ldc("CustomEvolution", gamedata.LoadCustomEvolution),
	"Data/Custom/CustomGHRS.ini": ld("gamedata.LoadCustomGHRS", "CastleSiege.exe FUN_00442c10", func(c *covEnv) error {
		return errOnly(gamedata.LoadCustomGHRS(c.data))
	}),

	// FUN_004e9d30: Event.
	"Data/Event/BloodCastle.dat":     lde("BloodCastle", gamedata.LoadBloodCastle),
	"Data/Event/BonusManager.dat":    ld("gamedata.LoadBonusManager", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadBonusManager(c.data); return err }),
	"Data/Event/CastleDeepEvent.dat": lde("CastleDeep", gamedata.LoadCastleDeep),
	"Data/Event/CastleSiege.dat":     lde("CastleSiege", gamedata.LoadCastleSiege),
	"Data/Event/ChaosCastle.dat":     lde("ChaosCastle", gamedata.LoadChaosCastle),
	"Data/Event/DevilSquare.dat":     lde("DevilSquare", gamedata.LoadDevilSquare),
	"Data/Event/DropEvent.dat":       ld("gamedata.LoadDropEvent", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadDropEvent(c.data); return err }),
	"Data/Event/EventEntryLevel.dat": lde("EventEntryLevel", gamedata.LoadEventEntryLevel),
	"Data/Event/EventStageSpawn.dat": lde("EventStageSpawn", gamedata.LoadEventStageSpawn),
	"Data/Event/GoldenArcher.dat":    lde("GoldenArcher", gamedata.LoadGoldenArcher),
	"Data/Event/InvasionManager.dat": lde("Invasions", gamedata.LoadInvasions),
	"Data/Event/Kalima.dat":          lde("Kalima", gamedata.LoadKalima),
	"Data/Custom/CustomQuiz.txt":     lde("CustomQuiz", gamedata.LoadCustomQuiz),

	"Data/EventItemBagManager.txt": ld("gamedata.LoadEventItemBags", "FUN_004ea4a0", func(c *covEnv) error {
		return errOnly(gamedata.LoadEventItemBags(c.data))
	}),

	// FUN_004ea5d0: Hack.
	"Data/Hack/HackPacketCheck.txt": ld("game.loadHackRules", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(loadHackRules(c.data))
	}),
	"Data/Hack/HackSkillSpeedCheck.txt": ld("gamedata.LoadHackSkillSpeedCheck", "FUN_004ea5d0 -> FUN_00488340", func(c *covEnv) error {
		return errOnly(gamedata.LoadHackSkillSpeedCheck(c.data))
	}),
	"Data/Hack/Enc2.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.data, "Hack", "Enc2.dat")))
	}),
	"Data/Hack/Dec1.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.data, "Hack", "Dec1.dat")))
	}),

	// FUN_004ea820: Item.
	"Data/Item/Item.txt":                ldi("gamedata.LoadItems", gamedata.LoadItems),
	"Data/Item/ExcellentOptionRate.txt": ldi("gamedata.LoadExcellentOptionRate", gamedata.LoadExcellentOptionRate),
	"Data/Item/ItemDrop.txt":            ldi("gamedata.LoadItemDrop", gamedata.LoadItemDrop),
	"Data/Item/ItemLevel.txt":           ldi("gamedata.LoadItemLevel", gamedata.LoadItemLevel),
	"Data/Item/ItemLoot.txt":            ldi("gamedata.LoadItemLoot", gamedata.LoadItemLoot),
	"Data/Item/ItemMoneyDrop.txt":       ldi("gamedata.LoadItemMoneyDrop", gamedata.LoadItemMoneyDrop),
	"Data/Item/ItemMove.txt":            ldi("gamedata.LoadItemMove", gamedata.LoadItemMove),
	"Data/Item/ItemOption.txt":          ldi("gamedata.LoadItemOptions", gamedata.LoadItemOptions),
	"Data/Item/ItemOptionRate.txt":      ldi("gamedata.LoadItemOptionRate", gamedata.LoadItemOptionRate),
	"Data/Item/ItemStack.txt":           ldi("gamedata.LoadItemStack", gamedata.LoadItemStack),
	"Data/Item/ItemValue.txt":           ldi("gamedata.LoadItemValue", gamedata.LoadItemValue),
	"Data/Item/SetItemOption.txt":       ldi("gamedata.LoadSetItemOption", gamedata.LoadSetItemOption),
	"Data/Item/SetItemType.txt":         ldi("gamedata.LoadSetItemType", gamedata.LoadSetItemType),
	"Data/Item/WingOption.txt":          ldi("gamedata.LoadWingOptions", gamedata.LoadWingOptions),

	// FUN_004eb000: Lang.
	"Data/Lang/Eng/Message.txt": ldMessages, "Data/Lang/Por/Message.txt": ldMessages, "Data/Lang/Spn/Message.txt": ldMessages,
	"Data/Lang/Eng/Notice.txt": ldNotices, "Data/Lang/Por/Notice.txt": ldNotices, "Data/Lang/Spn/Notice.txt": ldNotices,

	"Data/MapServerInfo.txt": ld("gamedata.LoadMapServer", "FUN_004eb520", func(c *covEnv) error {
		return errOnly(gamedata.LoadMapServer(c.data))
	}),

	// FUN_004eb5f0: Monster.
	"Data/Monster/MonsterList.txt": ld("gamedata.LoadMonsterList", "FUN_004eb5f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterList(c.data, gamedata.MonsterRates{}))
	}),
	"Data/Monster/MonsterMove.txt": ld("gamedata.LoadMonsterMove", "FUN_004eb5f0 -> FUN_004b1110", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterMove(c.data))
	}),
	"Data/Monster/MonsterSchedule.txt": ld("gamedata.LoadMonsterSchedule", "FUN_004eb5f0 -> FUN_004bb0c0", func(c *covEnv) error {
		return errOnly(gamedata.LoadMonsterSchedule(c.data))
	}),
	"Data/Monster/Skill/MonsterSkillElement.txt": ldMonsterSkills,
	"Data/Monster/Skill/MonsterSkillUnit.txt":    ldMonsterSkills,
	"Data/Monster/Skill/MonsterSkill.txt":        ldMonsterSkills,
	"Data/Monster/NpcTalk.txt": ld("gamedata.LoadNpcTalk", "FUN_004eb5f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadNpcTalk(c.data))
	}),

	// FUN_004eba60: Move.
	"Data/Move/Gate.txt": ld("gamedata.LoadGates", "FUN_004eba60", func(c *covEnv) error {
		return errOnly(gamedata.LoadGates(c.data))
	}),
	"Data/Move/Move.txt": ld("game.loadMoves", "FUN_004eba60", func(c *covEnv) error {
		return errOnly(loadMoves(c.data))
	}),
	"Data/Move/MoveSummon.txt": ld("gamedata.LoadMoveSummon", "FUN_004eba60 -> FUN_004bfd00", func(c *covEnv) error {
		return errOnly(gamedata.LoadMoveSummon(c.data))
	}),

	// FUN_004ebc50: Quest.
	"Data/Quest/Quest.txt": ldQuests, "Data/Quest/QuestObjective.txt": ldQuests, "Data/Quest/QuestReward.txt": ldQuests,

	"Data/ShopManager.txt": ld("gamedata.LoadShops", "FUN_004ebfc0", func(c *covEnv) error {
		return errOnly(gamedata.LoadShops(c.data))
	}),

	// FUN_004ec0f0: Skill.
	"Data/Skill/ComboSkill.txt": ldSkillWorld, "Data/Skill/SkillDamage.txt": ldSkillWorld,
	"Data/Skill/SkillElect.hit": ldSkillWorld, "Data/Skill/SkillEnabler.txt": ldSkillWorld,
	"Data/Skill/SkillUseArea.txt": ldSkillWorld,
	"Data/Skill/EffectList.txt": ld("game.loadEffectList", "FUN_004ec0f0", func(c *covEnv) error {
		return errOnly(loadEffectList(c.data))
	}),
	"Data/Skill/SkillList.txt": ld("gamedata.LoadSkills", "FUN_004ec0f0", func(c *covEnv) error {
		return errOnly(gamedata.LoadSkills(c.data))
	}),

	// FUN_004ec4c0: Util.
	"Data/Util/DamageTable.txt": ld("game.loadDamageTable", "FUN_004ec4c0", func(c *covEnv) error {
		return errOnly(loadDamageTable(c.data))
	}),
	"Data/Util/ExperienceTable.txt": ld("gamedata.LoadExperienceTable", "FUN_004ec4c0 -> FUN_0047be80", func(c *covEnv) error {
		return errOnly(gamedata.LoadExperienceTable(c.data))
	}),
	"Data/Util/Filter.txt":     ldMoveData,
	"Data/Util/GameMaster.txt": ldMoveData,
	"Data/Util/PetDamageTable.txt": ld("gamedata.LoadPetDamageTable", "FUN_004ec4c0 -> FUN_004d61d0", func(c *covEnv) error {
		return errOnly(gamedata.LoadPetDamageTable(c.data))
	}),
	"Data/Util/ResetLimit.txt": ldCmdWorld,
	"Data/Util/ResetTable.txt": ldCmdWorld,
}

// optionalFiles are read by the original when present but absent here.
var optionalFiles = map[string]bool{
	"Data/Custom/CustomPatente.txt": true, // FUN_00451f90; LoadCustomPatente accepts a missing file
}

var (
	ldMessages = ld("game.loadMessages", "FUN_004eb000", func(c *covEnv) error { return errOnly(loadMessages(c.data)) })
	ldNotices  = ld("game.loadNotices", "FUN_004eb000", func(c *covEnv) error {
		return errOnly(loadNotices(c.data, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	})
	ldMonsterSkills = ld("game.loadMonsterSkills", "FUN_004eb5f0", func(c *covEnv) error { return errOnly(loadMonsterSkills(c.data)) })
	ldQuests        = ld("gamedata.LoadQuests", "FUN_004ebc50", func(c *covEnv) error { return errOnly(gamedata.LoadQuests(c.data)) })
	ldSkillWorld    = ld("game.loadSkillWorld", "FUN_004ec0f0", func(c *covEnv) error { return errOnly(loadSkillWorld(c.data)) })
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
		return errOnly(gamedata.LoadTerrains(c.data))
	})
	ldGSConfig = func(inst string) covLoader {
		return ld("gsconfig.Load", ".\\Data\\GameServerInfo - *.dat (GetPrivateProfile*)", func(c *covEnv) error {
			return errOnly(gsconfig.Load(filepath.Join(c.data, "..", inst, "DATA")))
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
		coveredFiles["Data/Script/"+f] = l
	}
}

// ignoredFiles are never read by GameServer.exe or CastleSiege.exe.
var ignoredFiles = map[string]string{
	"Data/Common/Protect/Decrypt.dat":                       "not referenced by any server binary",
	"Data/Common/Protect/Encrypt.dat":                       "not referenced by any server binary",
	"Data/Custom/Crywolf.dat":                               "not referenced (no Crywolf system in this build)",
	"Data/Custom/CustomDailyReward.txt":                     "not referenced (the original reads Custom\\DailyReward.txt)",
	"Data/Custom/CustomMix.txt":                             "not referenced by any server binary",
	"Data/Event/Crywolf.dat":                                "not referenced (no Crywolf system in this build)",
	"Data/Event/IllusionTemple.dat":                         "not referenced (CIllusionTemple exists but loads no file)",
	"Data/Event/Kanturu.dat":                                "not referenced (no Kanturu system in this build)",
	"Data/Item/test_item.txt":                               "not referenced by any server binary",
	"Data/Monster/BALANCE_CHANGELOG.txt":                    "documentation",
	"Data/Shop/_ALTERACOES.txt":                             "documentation",
	"Data/Shop/239 - Arena Guard.txt":                       "not named by ShopManager.txt",
	"Data/Script/Script/TemplateScript.lua":                 "Lua example, not require'd by ScriptMain.lua",
	"Data/Script/Script/CustomMonsterDie(Boss).lua":         "require commented out in ScriptMain.lua",
	"Data/Script/Script/CustomMonsterDie(Boss) - Copia.lua": "copy, not require'd",
	"Data/EventItemBag/AJUSTE_JOIAS_20_PORCENTO.txt":        "not named by EventItemBagManager.txt",
	"Data/EventItemBag/Special/HideAndSeek.txt":             "not named by EventItemBagManager.txt",
	"Data/EventItemBag/Special/Loteria.txt":                 "not named by EventItemBagManager.txt",
	"Data/EventItemBag/Special/MataMata.txt":                "not named by EventItemBagManager.txt",
	"Data/EventItemBag/Special/Quiz.txt":                    "not named by EventItemBagManager.txt",
	"Data/EventItemBag/Special/Roleta.txt":                  "not named by EventItemBagManager.txt",
}

// dynamicLoader classifies the families of files the original finds through
// a manager list or a name pattern.
func dynamicLoader(rel string, bags, shops map[string]bool, maps map[int]gamedata.MapInfo) (covLoader, bool) {
	switch {
	case strings.HasPrefix(rel, "Data/Monster/Spawn/") && strings.Count(rel, "/") == 3:
		n := filepath.Base(rel)
		if len(n) > 6 && n[3:6] == " - " {
			if mp, err := strconv.Atoi(n[:3]); err == nil {
				if _, ok := maps[mp]; ok {
					return ldSpawns, true
				}
			}
		}
	case strings.HasPrefix(rel, "Data/EventItemBag/"):
		if bags[strings.TrimSuffix(strings.TrimPrefix(rel, "Data/EventItemBag/"), ".txt")] {
			return ldBags, true
		}
	case strings.HasPrefix(rel, "Data/Shop/"):
		if shops[strings.TrimSuffix(strings.TrimPrefix(rel, "Data/Shop/"), ".txt")] {
			return ldShops, true
		}
	case strings.HasPrefix(rel, "Data/Terrain/"):
		base := strings.ToLower(filepath.Base(rel)) // the original opens "terrain%d.att"
		if strings.HasPrefix(base, "terrain") && strings.HasSuffix(base, ".att") {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "terrain"), ".att"))
			if err == nil && n >= 1 && n <= gamedata.MaxMaps {
				return ldTerrains, true
			}
		}
	case strings.HasPrefix(rel, "4.GameServer/DATA/GameServerInfo - "), strings.HasPrefix(rel, "5.GameServerCS/DATA/GameServerInfo - "):
		inst := rel[:strings.Index(rel, "/")]
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(rel), "GameServerInfo - "), ".dat")
		for _, f := range gsconfig.Files {
			if f == name {
				return ldGSConfig(inst), true
			}
		}
	}
	return covLoader{}, false
}

func TestDataFileCoverage(t *testing.T) {
	root := repoData()
	data := repoData("Data")
	b := newBareWorld(t) // game.New runs every start hook on the shipped data
	env := &covEnv{t: t, data: data, w: b.World}

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
	for _, dir := range []string{"Data", "4.GameServer/DATA", "5.GameServerCS/DATA"} {
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
		"Data/Monster/Spawn/000 - Lorencia.txt":         true,
		"Data/Monster/Spawn/077 - Unknown.txt":          false, // map not in MapManager
		"Data/Monster/Spawn/backup/006 - Arena.txt":     false, // subfolders are skipped
		"Data/EventItemBag/Item/Box of Luck.txt":        true,
		"Data/EventItemBag/Item/New Box.txt":            false,
		"Data/Shop/253 - Potion Girl Amy.txt":           true,
		"Data/Shop/999 - New.txt":                       false,
		"Data/Terrain/Terrain12.att":                    true,
		"Data/Terrain/Terrain101.att":                   false,
		"4.GameServer/DATA/GameServerInfo - Common.dat": true,
		"4.GameServer/DATA/GameServerInfo - New.dat":    false,
		"Data/Brand/New.txt":                            false,
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
