package gamedata

// MonsterConfig is config/data/monster.toml: Data/Monster/*.txt and
// Data/Monster/Skill/*.txt.
type MonsterConfig struct {
	MonsterList         MonsterListFile         `comment:"MonsterList.txt (CMonsterManager): Index 0~999, Type 0 monster / 1 NPC; RegenTime in seconds"`
	MonsterMove         MonsterMoveFile         `comment:"MonsterMove.txt: NPCs that move between maps; KeepTime in seconds"`
	MonsterSchedule     MonsterScheduleFile     `comment:"MonsterSchedule.txt: monster groups spawned at scheduled times; -1 = any"`
	NpcTalk             NpcTalkFile             `comment:"NpcTalk.txt: NPC chat messages (max 128 characters); -1 = any"`
	MonsterSkill        MonsterSkillFile        `comment:"Skill/MonsterSkill.txt: up to 10 (Type, Unit) pairs per monster; -1 = none"`
	MonsterSkillElement MonsterSkillElementFile `comment:"Skill/MonsterSkillElement.txt"`
	MonsterSkillUnit    MonsterSkillUnitFile    `comment:"Skill/MonsterSkillUnit.txt: Slot = MonsterSkillElement indexes; -1 = none"`
}

type MonsterListFile struct {
	Rows []MonsterListRow `toml:",inline"`
}

type MonsterListRow struct {
	Index, Type                                                     int
	Name                                                            string
	Level, MaxLife, MaxMana, DamageMin, DamageMax, Defense          int
	MagicDefense, AttackRate, DefenseRate, MoveRange, AttackType    int
	AttackRange, ViewRange, MoveSpeed, AttackSpeed, RegenTime       int
	Attribute, ItemRate, MoneyRate, MaxItemLevel, MonsterSkill      int
	Resistance1, Resistance2, Resistance3, Resistance4, Resistance5 int
	Resistance6, Resistance7                                        int
	Comment                                                         string `toml:",omitempty" legacy:"comment"`
}

type MonsterMoveFile struct {
	Rows []MonsterMoveConfigRow `toml:",inline"`
}

type MonsterMoveConfigRow struct {
	Index, MapNumber, MapPosX, MapPosY, Direction, KeepTime int
	Comment                                                 string `toml:",omitempty"`
}

type MonsterScheduleFile struct {
	Schedule []MonsterScheduleTimeRow    `toml:",inline" comment:"section 0: start times of each group"`
	Message  []MonsterScheduleMessageRow `toml:",inline" comment:"section 1: Message.txt indexes; EnableNpcTime in seconds"`
	Monster  []MonsterScheduleMonsterRow `toml:",inline" comment:"section 2: the monsters of each group"`
}

type MonsterScheduleTimeRow struct {
	Group                                                                    int
	StartYear, StartMonth, StartDay, StartDoW, StartHour, StartMin, StartSec int
	Comment                                                                  string `toml:",omitempty"`
}

type MonsterScheduleMessageRow struct {
	Group                                            int
	EnableMessage, DisableMessage, ItsNotTimeMessage int
	EnableNpcTime                                    int
	Comment                                          string `toml:",omitempty"`
}

type MonsterScheduleMonsterRow struct {
	Group, MonsterIndex, MapNumber, LocationX, LocationY int
	Comment                                              string `toml:",omitempty"`
}

type NpcTalkFile struct {
	Rows []NpcTalkRow `toml:",inline"`
}

type NpcTalkRow struct {
	NPCIndex, Map, CordX, CordY int
	Message                     string
	Comment                     string `toml:",omitempty" legacy:"comment"`
}

type MonsterSkillFile struct {
	Rows []MonsterSkillRow `toml:",inline"`
}

// MonsterSkillRow: Skill[i] is the (Type, Unit) pair i+1.
type MonsterSkillRow struct {
	Monster int
	Name    string
	Skill   [10][2]int
	Comment string `toml:",omitempty"`
}

type MonsterSkillElementFile struct {
	Rows []MonsterSkillElementRow `toml:",inline"`
}

type MonsterSkillElementRow struct {
	Index                                         int
	Name                                          string
	Type, SuccessRate, ContinuanceTime            int
	IncAndDecType, IncAndDecValue, NullifiedMagic int
	CharacterClass, CharacterLevel                int
	Comment                                       string `toml:",omitempty"`
}

type MonsterSkillUnitFile struct {
	Rows []MonsterSkillUnitRow `toml:",inline"`
}

type MonsterSkillUnitRow struct {
	Index                                    int
	Name                                     string
	TargetType, ScopeType, ScopeValue, Delay int
	Slot                                     [5]int
	Comment                                  string `toml:",omitempty"`
}

// LoadMonsterConfig reads config/data/monster.toml.
func LoadMonsterConfig(configDir string) (*MonsterConfig, error) {
	return loadConfig[MonsterConfig](configDir, "monster")
}

// SpawnConfig is config/data/spawn.toml: Data/Monster/Spawn/"NNN - Name.txt",
// keyed by file name without ".txt" (NNN is the map).
type SpawnConfig struct {
	Spawn map[string]SpawnFile `comment:"Section: 0 NPC (fixed), 1 area (random cell in Location..LocationT, Quantity monsters), 2 single (+-3 cells),\n3 invasion, 4 event (spawned by their managers). Direction -1 = random."`
}

// SpawnFile keeps the records of a spawn file in file order (the order the
// original creates them).
type SpawnFile struct {
	Rows []SpawnRow `toml:",inline"`
}

type SpawnRow struct {
	Section, MonsterIndex, SpawnRadius, LocationX, LocationY, Direction int
	LocationTX, LocationTY, Quantity                                    int    `toml:",omitempty"` // section 1
	Comment                                                             string `toml:",omitempty"`
}

func loadSpawnConfig(configDir string) (*SpawnConfig, error) {
	return loadConfig[SpawnConfig](configDir, "spawn")
}
