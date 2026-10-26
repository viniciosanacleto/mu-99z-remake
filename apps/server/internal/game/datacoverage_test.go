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
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/protocol/crypto"
)

// Data coverage test (docs/server-plan/16-cobertura-dados.md). The text
// configuration of the original lives in apps/server/config (TOML, see
// cmd/configconv): every config/data/<group>.toml must have the Go loaders
// of the original files it holds, and they must read the shipped file.
// apps/server/data keeps only the binary files (terrain, keys, Lua
// scripts, images): each must be read by a Go loader or ignored with a
// reason. A new file that is none of these fails the test.
//
// The test lives in package game because several loaders are unexported
// here; the rest are in internal/gamedata and internal/config.

// covEnv gives loaders the data directories and a world built from them.
type covEnv struct {
	t    *testing.T
	data string // apps/server/data/Data
	cfg  string // apps/server/config
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

// ldg is a gamedata loader taking the config directory.
func ldg[T any](name, orig string, f func(string) (T, error)) covLoader {
	return ld("gamedata."+name, orig, func(c *covEnv) error { return errOnly(f(c.cfg)) })
}

// --- data/Data: the binary files ---------------------------------------------------

var ldSkillWorld = ld("game.loadSkillWorld", "FUN_004ec0f0", func(c *covEnv) error {
	return errOnly(loadSkillWorld(c.data, c.cfg))
})

var ldTerrains = ld("gamedata.LoadTerrains", "FUN_004eb3a0 (Terrain\\terrain%d.att, maps 1..100)", func(c *covEnv) error {
	return errOnly(gamedata.LoadTerrains(c.data))
})

// coveredFiles are the binary files and their Go loaders (paths relative
// to apps/server/data).
var coveredFiles = map[string]covLoader{
	// FUN_004ea5d0: the packet keys of Hack.
	"Data/Hack/Enc2.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.data, "Hack", "Enc2.dat")))
	}),
	"Data/Hack/Dec1.dat": ld("crypto.LoadKeyFile", "FUN_004ea5d0", func(c *covEnv) error {
		return errOnly(crypto.LoadKeyFile(filepath.Join(c.data, "Hack", "Dec1.dat")))
	}),
	// FUN_004ec0f0: Skill.
	"Data/Skill/SkillElect.hit": ldSkillWorld,
	// FUN_00404a60: the QR image named by ImagemBMP in config/data/xshop.toml.
	"Data/Custom/pixFE31.tmp.bmp": ld("game.rechargeBlock", "FUN_00404a60 -> FUN_00401bf0", func(c *covEnv) error {
		if d := c.w.rechargeBlock(c.w.rechargePath()); d[rcQRSize] == 0 && d[rcQRSize+1] == 0 {
			c.t.Error("ImagemBMP not loaded")
		}
		return nil
	}),
}

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
	"Data/Script/Script/TemplateScript.lua":                 "Lua example, not require'd by ScriptMain.lua",
	"Data/Script/Script/CustomMonsterDie(Boss).lua":         "require commented out in ScriptMain.lua",
	"Data/Script/Script/CustomMonsterDie(Boss) - Copia.lua": "copy, not require'd",
}

// dynamicLoader classifies the files the original finds through a name
// pattern.
func dynamicLoader(rel string) (covLoader, bool) {
	if strings.HasPrefix(rel, "Data/Terrain/") {
		base := strings.ToLower(filepath.Base(rel)) // the original opens "terrain%d.att"
		if strings.HasPrefix(base, "terrain") && strings.HasSuffix(base, ".att") {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(base, "terrain"), ".att"))
			if err == nil && n >= 1 && n <= gamedata.MaxMaps {
				return ldTerrains, true
			}
		}
	}
	return covLoader{}, false
}

func TestDataFileCoverage(t *testing.T) {
	root := repoData()
	b := newBareWorld(t) // game.New runs every start hook on the shipped data
	env := &covEnv{t: t, data: repoData("Data"), cfg: repoConfig(), w: b.World}

	var files []string
	err := filepath.WalkDir(filepath.Join(root, "Data"), func(p string, d fs.DirEntry, err error) error {
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
	sort.Strings(files)

	seen := map[string]bool{}
	called := map[string]error{}
	var loaded, ignored int
	for _, rel := range files {
		seen[rel] = true
		l, ok := coveredFiles[rel]
		if !ok {
			l, ok = dynamicLoader(rel)
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
		default:
			t.Errorf("unknown data file %s: text configuration belongs in config/ (cmd/configconv); "+
				"add a binary file to coveredFiles (with its Go loader) or ignoredFiles", rel)
		}
	}
	for rel := range coveredFiles {
		if !seen[rel] {
			t.Errorf("coveredFiles lists %s, which does not exist", rel)
		}
	}
	for rel := range ignoredFiles {
		if !seen[rel] {
			t.Errorf("stale entry %s: file does not exist", rel)
		}
	}
	t.Logf("%d files: %d loaded by Go, %d ignored", len(files), loaded, ignored)
}

// TestDataFileCoverageUnknown checks the classification rejects a file
// nobody claims and accepts the terrain family.
func TestDataFileCoverageUnknown(t *testing.T) {
	cases := map[string]bool{
		"Data/Terrain/Terrain12.att":  true,
		"Data/Terrain/Terrain101.att": false,
		"Data/Item/Item.txt":          false,
		"Data/Brand/New.txt":          false,
	}
	for rel, want := range cases {
		_, ok := dynamicLoader(rel)
		if _, c := coveredFiles[rel]; c {
			ok = true
		}
		if ok != want {
			t.Errorf("%s: classified %v, want %v", rel, ok, want)
		}
	}
}

// --- config/data: the tables -----------------------------------------------------------

var (
	ldMoveData = ld("game.loadMoveData", "FUN_004ec4c0", func(c *covEnv) error { return loadMoveData(c.w) })
	ldCmdWorld = ld("game.loadCmdWorld", "FUN_004ec4c0", func(c *covEnv) error { return loadCmdWorld(c.w) })
)

// configLoaders are the Go loaders of each config/data/<group>.toml file
// (one per original Data/ file it holds).
var configLoaders = map[string][]covLoader{
	// Files at the root of Data/ (FUN_004e8520, FUN_004e86f0, FUN_004e8860, FUN_004eb520).
	"common": {
		ldg("LoadMapManager", "FUN_004e8860", gamedata.LoadMapManager),
		ldg("LoadMapRateInfo", "FUN_004e8860", gamedata.LoadMapRateInfo),
		ldg("LoadMapServer", "FUN_004eb520", gamedata.LoadMapServer),
		ldg("LoadBlackList", "FUN_004e8520 -> FUN_00411ae0", gamedata.LoadBlackList),
		ld("game.loadCommands", "FUN_004e86f0", func(c *covEnv) error { return errOnly(loadCommands(c.cfg)) }),
		ldg("LoadCommandRequirement", "FUN_004e86f0 -> FUN_004402a0", gamedata.LoadCommandRequirement),
		ld("game.loadConnectMembers", "FUN_004e8860", func(c *covEnv) error { return errOnly(loadConnectMembers(c.cfg)) }),
	},
	// FUN_004e85f0: Character.
	"character": {
		ldg("LoadClassInfo", "FUN_004e85f0", gamedata.LoadClassInfo),
		ldg("LoadDefaultClassFreebies", "FUN_004e85f0 -> FUN_0046cb40", gamedata.LoadDefaultClassFreebies),
	},
	// FUN_004e8a50: Custom (CreationGold is read on use by FUN_0049a690,
	// CustomGHRS.ini by CastleSiege.exe FUN_00442c10).
	"custom": {
		ldg("LoadCustomArena", "FUN_004e8a50", gamedata.LoadCustomArena),
		ldg("LoadCustomAttack", "FUN_004e8a50", gamedata.LoadCustomAttack),
		ldg("LoadCustomAttributes", "FUN_004e8a50", gamedata.LoadCustomAttributes),
		ldg("LoadCustomBuyVip", "FUN_004e8a50", gamedata.LoadCustomBuyVip),
		ldg("LoadCustomDeathMessage", "FUN_004e8a50", gamedata.LoadCustomDeathMessage),
		ldg("LoadCustomEventDrop", "FUN_004e8a50", gamedata.LoadCustomEventDrop),
		ldg("LoadCustomEvolution", "FUN_004e8a50", gamedata.LoadCustomEvolution),
		ldg("LoadCustomGHRS", "CastleSiege.exe FUN_00442c10", gamedata.LoadCustomGHRS),
		ldg("LoadCustomGate", "FUN_004e8a50", gamedata.LoadCustomGate),
		ldg("LoadCustomGift", "FUN_004e8a50", gamedata.LoadCustomGift),
		ldg("LoadCustomGuard", "FUN_004e8a50", gamedata.LoadCustomGuard),
		ldg("LoadCustomHealthBar", "FUN_004e8a50", gamedata.LoadCustomHealthBar),
		ldg("LoadCustomJewel", "FUN_004e8a50", gamedata.LoadCustomJewel),
		ldg("LoadLuckyWheel", "FUN_004e8a50", gamedata.LoadLuckyWheel),
		ldg("LoadCustomLukeHelper", "FUN_004e8a50", gamedata.LoadCustomLukeHelper),
		ldg("LoadCustomMarriage", "FUN_004e8a50", gamedata.LoadCustomMarriage),
		ldg("LoadCustomMobCombo", "FUN_004e8a50", gamedata.LoadCustomMobCombo),
		ldg("LoadCustomMonsters", "FUN_004e8a50 -> FUN_00466b30", gamedata.LoadCustomMonsters),
		ldg("LoadCustomMonsterRows", "FUN_004e8a50 -> FUN_00466b30", gamedata.LoadCustomMonsterRows),
		ldg("LoadCustomMonsterKillCount", "FUN_004e8a50", gamedata.LoadCustomMonsterKillCount),
		ldg("LoadCustomMove", "FUN_004e8a50", gamedata.LoadCustomMove),
		ldg("LoadCustomNPCCommand", "FUN_004e8a50", gamedata.LoadCustomNPCCommand),
		ldg("LoadCustomPatente", "FUN_00451f90", gamedata.LoadCustomPatente),
		ldg("LoadCustomPick", "FUN_004e8a50", gamedata.LoadCustomPick),
		ldg("LoadCustomQuiz", "FUN_004e9d30", gamedata.LoadCustomQuiz),
		ldg("LoadCustomRespawn", "FUN_004e8a50", gamedata.LoadCustomRespawn),
		ldg("LoadCustomWings", "FUN_004e8a50", gamedata.LoadCustomWings),
		ld("game.loadCustomTables", "FUN_004e8a50 (CustomWing, CustomPet)", func(c *covEnv) error {
			return errOnly(loadCustomTables(c.cfg))
		}),
		ldg("LoadDailyReward", "FUN_0045a200", gamedata.LoadDailyReward),
		ldg("LoadSummonScroll", "FUN_00505d20", gamedata.LoadSummonScroll),
		ld("gamedata.CreationGoldValue", "FUN_0049a690 (read on use)", func(c *covEnv) error {
			if gamedata.CreationGoldValue(c.cfg) != 20 {
				c.t.Error("CreationGold GoldValue not read")
			}
			return nil
		}),
	},
	// The Custom/Event loaders of FUN_004e8390.
	"custom-event": {
		ldg("LoadDeathEvent", "FUN_0045b890", gamedata.LoadDeathEvent),
		ldg("LoadDuelEvent", "FUN_0045d1f0", gamedata.LoadDuelEvent),
		ldg("LoadPairEvent", "FUN_004cebf0", gamedata.LoadPairEvent),
		ldg("LoadRaffleEvent", "FUN_004e0350", gamedata.LoadRaffleEvent),
		ldg("LoadRoletaEvent", "FUN_004e42e0", gamedata.LoadRoletaEvent),
		ldg("LoadSobreEvent", "FUN_00502c70", gamedata.LoadSobreEvent),
		ldg("LoadTheftEvent", "FUN_00505fa0", gamedata.LoadTheftEvent),
	},
	// FUN_004e9d30: Event (Crywolf.dat, IllusionTemple.dat and Kanturu.dat are not read).
	"event": {
		ldg("LoadBloodCastle", "FUN_004e9d30", gamedata.LoadBloodCastle),
		ld("gamedata.LoadBonusManager", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadBonusManager(c.cfg); return err }),
		ldg("LoadCastleDeep", "FUN_004e9d30", gamedata.LoadCastleDeep),
		ldg("LoadCastleSiege", "FUN_004e9d30", gamedata.LoadCastleSiege),
		ldg("LoadChaosCastle", "FUN_004e9d30", gamedata.LoadChaosCastle),
		ldg("LoadDevilSquare", "FUN_004e9d30", gamedata.LoadDevilSquare),
		ld("gamedata.LoadDropEvent", "FUN_004e9d30", func(c *covEnv) error { _, _, err := gamedata.LoadDropEvent(c.cfg); return err }),
		ldg("LoadEventEntryLevel", "FUN_004e9d30", gamedata.LoadEventEntryLevel),
		ldg("LoadEventStageSpawn", "FUN_004e9d30", gamedata.LoadEventStageSpawn),
		ldg("LoadGoldenArcher", "FUN_004e9d30", gamedata.LoadGoldenArcher),
		ldg("LoadInvasions", "FUN_004e9d30", gamedata.LoadInvasions),
		ldg("LoadKalima", "FUN_004e9d30", gamedata.LoadKalima),
	},
	// FUN_004ea4a0: EventItemBagManager.txt and the EventItemBag\%s.txt bags it names.
	"eventitembag": {
		ldg("LoadEventItemBags", "FUN_004ea4a0 -> FUN_00491220", gamedata.LoadEventItemBags),
	},
	// FUN_004ea5d0: Hack (Enc2.dat / Dec1.dat are binary keys in data/Data/Hack).
	"hack": {
		ld("game.loadHackRules", "FUN_004ea5d0", func(c *covEnv) error { return errOnly(loadHackRules(c.cfg)) }),
		ldg("LoadHackSkillSpeedCheck", "FUN_004ea5d0 -> FUN_00488340", gamedata.LoadHackSkillSpeedCheck),
	},
	// FUN_004ea820: Item.
	"item": {
		ldg("LoadItems", "FUN_004ea820", gamedata.LoadItems),
		ldg("LoadExcellentOptionRate", "FUN_004ea820", gamedata.LoadExcellentOptionRate),
		ldg("LoadItemDrop", "FUN_004ea820", gamedata.LoadItemDrop),
		ldg("LoadItemLevel", "FUN_004ea820", gamedata.LoadItemLevel),
		ldg("LoadItemLoot", "FUN_004ea820", gamedata.LoadItemLoot),
		ldg("LoadItemMoneyDrop", "FUN_004ea820", gamedata.LoadItemMoneyDrop),
		ldg("LoadItemMove", "FUN_004ea820", gamedata.LoadItemMove),
		ldg("LoadItemOptions", "FUN_004ea820", gamedata.LoadItemOptions),
		ldg("LoadItemOptionRate", "FUN_004ea820", gamedata.LoadItemOptionRate),
		ldg("LoadItemStack", "FUN_004ea820", gamedata.LoadItemStack),
		ldg("LoadItemValue", "FUN_004ea820", gamedata.LoadItemValue),
		ldg("LoadSetItemOption", "FUN_004ea820", gamedata.LoadSetItemOption),
		ldg("LoadSetItemType", "FUN_004ea820", gamedata.LoadSetItemType),
		ldg("LoadWingOptions", "FUN_004ea820", gamedata.LoadWingOptions),
	},
	// FUN_004eb000: Lang.
	"lang": {
		ld("game.loadMessages", "FUN_004eb000", func(c *covEnv) error { return errOnly(loadMessages(c.cfg)) }),
		ld("game.loadNotices", "FUN_004eb000", func(c *covEnv) error {
			return errOnly(loadNotices(c.cfg, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
		}),
	},
	// FUN_004eb5f0: Monster (and Monster/Skill).
	"monster": {
		ld("gamedata.LoadMonsterList", "FUN_004eb5f0", func(c *covEnv) error {
			return errOnly(gamedata.LoadMonsterList(c.cfg, gamedata.MonsterRates{}))
		}),
		ldg("LoadMonsterMove", "FUN_004eb5f0 -> FUN_004b1110", gamedata.LoadMonsterMove),
		ldg("LoadMonsterSchedule", "FUN_004eb5f0 -> FUN_004bb0c0", gamedata.LoadMonsterSchedule),
		ldg("LoadNpcTalk", "FUN_004eb5f0", gamedata.LoadNpcTalk),
		ld("game.loadMonsterSkills", "FUN_004eb5f0", func(c *covEnv) error { return errOnly(loadMonsterSkills(c.cfg)) }),
	},
	// FUN_004eba60: Move.
	"move": {
		ldg("LoadGates", "FUN_004eba60", gamedata.LoadGates),
		ld("game.loadMoves", "FUN_004eba60", func(c *covEnv) error { return errOnly(loadMoves(c.cfg)) }),
		ldg("LoadMoveSummon", "FUN_004eba60 -> FUN_004bfd00", gamedata.LoadMoveSummon),
		ld("game.loadMoveSummon", "FUN_004bfd00", func(c *covEnv) error { return errOnly(loadMoveSummon(c.cfg)) }),
	},
	// FUN_004ebc50: Quest.
	"quest": {ldg("LoadQuests", "FUN_004ebc50", gamedata.LoadQuests)},
	// FUN_004ebfc0: ShopManager.txt and the Shop\%s.txt files it names.
	"shop": {ldg("LoadShops", "FUN_004ebfc0 -> FUN_004f55e0", gamedata.LoadShops)},
	// FUN_004ec0f0: Skill.
	"skill": {
		ldSkillWorld,
		ld("game.loadEffectList", "FUN_004ec0f0", func(c *covEnv) error { return errOnly(loadEffectList(c.cfg)) }),
		ldg("LoadSkills", "FUN_004ec0f0", gamedata.LoadSkills),
	},
	// FUN_004bbaa0: Monster\Spawn\NNN - *.txt (map known to MapManager).
	"spawn": {
		ld("gamedata.LoadSpawns", "FUN_004bbaa0", func(c *covEnv) error {
			maps, err := gamedata.LoadMapManager(c.cfg)
			if err != nil {
				return err
			}
			return errOnly(gamedata.LoadSpawns(c.cfg, func(m int) bool { _, ok := maps[m]; return ok }, func() int { return 0 }))
		}),
	},
	// FUN_004ec4c0: Util.
	"util": {
		ld("game.loadDamageTable", "FUN_004ec4c0", func(c *covEnv) error { return errOnly(loadDamageTable(c.cfg)) }),
		ldg("LoadExperienceTable", "FUN_004ec4c0 -> FUN_0047be80", gamedata.LoadExperienceTable),
		ldg("LoadPetDamageTable", "FUN_004ec4c0 -> FUN_004d61d0", gamedata.LoadPetDamageTable),
		ld("game.loadPetDamageTable", "FUN_004d61d0", func(c *covEnv) error { return errOnly(loadPetDamageTable(c.cfg)) }),
		ldMoveData, // Filter.txt, GameMaster.txt
		ldCmdWorld, // ResetTable.txt, ResetLimit.txt
	},
	// FUN_004026e0 (CustomXShop.txt) and FUN_00404a60 (CustomXShopRecharge.txt).
	"xshop": {
		ld("gamedata.LoadXShopPrices", "FUN_004026e0", func(c *covEnv) error {
			_, ok, err := gamedata.LoadXShopPrices(filepath.Join(c.cfg, "data", "xshop.toml"))
			if err == nil && !ok {
				c.t.Error("xshop.toml not opened")
			}
			return err
		}),
		ldg("LoadXShopConfig", "FUN_00404a60", func(dir string) (*gamedata.XShopConfig, error) {
			return gamedata.LoadXShopConfig(filepath.Join(dir, "data", "xshop.toml"))
		}),
	},
}

// TestConfigFileCoverage checks every config/data file has its loaders and
// that they read the shipped file, and that the server and instance files
// load.
func TestConfigFileCoverage(t *testing.T) {
	b := newBareWorld(t)
	env := &covEnv{t: t, data: repoData("Data"), cfg: repoConfig(), w: b.World}
	files, err := filepath.Glob(repoConfig("data", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		group := strings.TrimSuffix(filepath.Base(f), ".toml")
		seen[group] = true
		ls, ok := configLoaders[group]
		if !ok {
			t.Errorf("config/data/%s.toml has no loaders in configLoaders", group)
		}
		for _, l := range ls {
			if err := l.load(env); err != nil {
				t.Errorf("%s: %s failed: %v", group, l.name, err)
			}
		}
	}
	for group := range configLoaders {
		if !seen[group] {
			t.Errorf("configLoaders lists %s, but config/data/%s.toml does not exist", group, group)
		}
	}
	if _, err := servercfg.Load(repoConfig("server.toml")); err != nil {
		t.Error(err)
	}
	instances, _ := filepath.Glob(repoConfig("gameserver", "*.toml"))
	if len(instances) != 2 {
		t.Errorf("%d instance files", len(instances))
	}
	for _, f := range instances {
		if _, err := gsconfig.Load(f); err != nil {
			t.Error(err)
		}
	}
}
