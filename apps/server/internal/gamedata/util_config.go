package gamedata

// HackConfig is config/data/hack.toml: the Data/Hack tables (Enc2.dat and
// Dec1.dat are binary keys and stay in data/Data/Hack).
type HackConfig struct {
	HackPacketCheck     HackPacketCheckFile     `comment:"HackPacketCheck.txt: one rule per packet head (Index) and sub (Value, -1 = any);\nEncrypt -1 = any; MaxDelay in ms; MinCount logs, MaxCount disconnects (0 = no limit)"`
	HackSkillSpeedCheck HackSkillSpeedCheckFile `comment:"HackSkillSpeedCheck.txt: MaxDelay in ms; Action 0~2; -1 = any"`
}

type HackPacketCheckFile struct {
	Rows []HackPacketCheckRow `toml:",inline"`
}

type HackPacketCheckRow struct {
	Index, Value, Encrypt, MaxDelay, MinCount, MaxCount int
	Comment                                             string `toml:",omitempty" legacy:"comment"`
}

type HackSkillSpeedCheckFile struct {
	Rows []HackSkillSpeedRow `toml:",inline"`
}

type HackSkillSpeedRow struct {
	Index, Class, MinLevel, MaxLevel, MinReset, MaxReset, MinSpeed, MaxSpeed int
	MaxDelay, MinCount, MaxCount, Action                                     int
	Comment                                                                  string `toml:",omitempty"`
}

// LoadHackConfig reads config/data/hack.toml.
func LoadHackConfig(configDir string) (*HackConfig, error) {
	return loadConfig[HackConfig](configDir, "hack")
}

// UtilConfig is config/data/util.toml: Data/Util/*.txt; -1 = any.
type UtilConfig struct {
	DamageTable     DamageTableFile     `comment:"DamageTable.txt: DamageRate in percent; the first matching row wins"`
	ExperienceTable ExperienceTableFile `comment:"ExperienceTable.txt: the original reads one rate per row (the file header documents four, per account level)"`
	Filter          FilterFile          `comment:"Filter.txt: words removed from chat"`
	GameMaster      GameMasterFile      `comment:"GameMaster.txt: GM level per account and character"`
	PetDamageTable  PetDamageTableFile  `comment:"PetDamageTable.txt: DamageRate of 1000"`
	ResetLimit      ResetLimitFile      `comment:"ResetLimit.txt: resets allowed per day / week / month and account level"`
	ResetTable      ResetTableFile      `comment:"ResetTable.txt: -1 uses the Command table value"`
}

type DamageTableFile struct {
	Monster []DamageMonsterRow `toml:",inline" comment:"section 0: monster -> player"`
	Player  []DamagePlayerRow  `toml:",inline" comment:"section 1: player -> monster"`
}

type DamageMonsterRow struct {
	MapNumber, MinLevel, MaxLevel, MinReset, MaxReset, MinDefense, MaxDefense, DamageRate int
	Comment                                                                               string `toml:",omitempty" legacy:"comment"`
}

type DamagePlayerRow struct {
	MapNumber, MinLevel, MaxLevel, MinReset, MaxReset int
	MonsterIndex, MonsterMinLevel, MonsterMaxLevel    int
	DamageRate                                        int
	Comment                                           string `toml:",omitempty" legacy:"comment"`
}

type ExperienceTableFile struct {
	Rows []ExperienceTableRow `toml:",inline"`
}

type ExperienceTableRow struct {
	MinLevel, MaxLevel, MinReset, MaxReset, MinMasterReset, MaxMasterReset int
	ExperienceRate                                                         int
	Comment                                                                string `toml:",omitempty"`
}

type FilterFile struct {
	Words []string
}

type GameMasterFile struct {
	Rows []GameMasterRow `toml:",inline"`
}

type GameMasterRow struct {
	Account, Name string
	GMLevel       int
	Comment       string `toml:",omitempty"`
}

type PetDamageTableFile struct {
	Rows []PetDamageTableRow `toml:",inline"`
}

type PetDamageTableRow struct {
	Index, MinLevel, MaxLevel, MinReset, MaxReset, DamageRate int
	Comment                                                   string `toml:",omitempty"`
}

type ResetLimitFile struct {
	Rows []ResetLimitRow `toml:",inline"`
}

type ResetLimitRow struct {
	MinReset, MaxReset              int
	LimitDay, LimitWeek, LimitMonth [4]int
	Comment                         string `toml:",omitempty" legacy:"comment"`
}

type ResetTableFile struct {
	Rows []ResetTableRow `toml:",inline"`
}

type ResetTableRow struct {
	MinReset, MaxReset  int
	Level, Money, Point [4]int
	Comment             string `toml:",omitempty" legacy:"comment"`
}

// LoadUtilConfig reads config/data/util.toml.
func LoadUtilConfig(configDir string) (*UtilConfig, error) {
	return loadConfig[UtilConfig](configDir, "util")
}
