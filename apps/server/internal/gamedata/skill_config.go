package gamedata

// SkillConfig is config/data/skill.toml: Data/Skill/*.txt (SkillElect.hit
// is binary and stays in data/Data/Skill).
type SkillConfig struct {
	SkillList    SkillListFile    `comment:"SkillList.txt (CSkillManager): Type -1 = none; DW..DL 0 no, 1 yes, 2 second class only"`
	EffectList   EffectListFile   `comment:"EffectList.txt: ItemIndex \"*\" = none; Duration in seconds, -1 = the caller's duration; -1 = none"`
	SkillDamage  SkillDamageFile  `comment:"SkillDamage.txt: damage rates (of 10000) and effect rates (of 100) per skill"`
	ComboSkill   ComboSkillFile   `comment:"ComboSkill.txt: Class 0~4; Stage 0-1; Delay in ms"`
	SkillEnabler SkillEnablerFile `comment:"SkillEnabler.txt: the first matching row wins; MapNumber -1 = any"`
	SkillUseArea SkillUseAreaFile `comment:"SkillUseArea.txt"`
}

type SkillListFile struct {
	Rows []SkillListRow `toml:",inline"`
}

type SkillListRow struct {
	Index                                            int
	Name                                             string
	Damage, MP, BP, Range, Radio, Delay, Type        int
	Effect, ReqLevel, ReqEnergy, ReqLeadership       int
	ReqKillCount, ReqGuildStatus, DW, DK, FE, MG, DL int
	Comment                                          string `toml:",omitempty" legacy:"comment"`
}

type EffectListFile struct {
	Rows []EffectListRow `toml:",inline"`
}

type EffectListRow struct {
	Index, Group     int
	ItemIndex        string
	Name             string
	Save, Type, Flag int
	Duration         int
	Power            [4]int
	Comment          string `toml:",omitempty"`
}

type SkillDamageFile struct {
	Damage []SkillDamageRow     `toml:",inline" comment:"section 0"`
	Effect []SkillEffectRateRow `toml:",inline" comment:"section 1"`
}

type SkillDamageRow struct {
	SkillIndex, DamageRatePvM, DamageRatePvP int
	Comment                                  string `toml:",omitempty" legacy:"comment"`
}

type SkillEffectRateRow struct {
	SkillIndex, EffectRate int
	Comment                string `toml:",omitempty" legacy:"comment"`
}

type ComboSkillFile struct {
	Rows []ComboSkillRow `toml:",inline"`
}

type ComboSkillRow struct {
	Class, SkillIndex, Stage, Delay int
	Comment                         string `toml:",omitempty" legacy:"comment"`
}

type SkillEnablerFile struct {
	Rows []SkillEnablerRow `toml:",inline"`
}

type SkillEnablerRow struct {
	SkillIndex, MapNumber, UsePvM, UsePvP int
	Comment                               string `toml:",omitempty" legacy:"comment"`
}

type SkillUseAreaFile struct {
	Skill []SkillUseAreaSkillRow `toml:",inline" comment:"section 0: skills limited to the areas of section 1"`
	Area  []SkillUseAreaRow      `toml:",inline" comment:"section 1: map rectangles"`
}

type SkillUseAreaSkillRow struct {
	SkillIndex int
	Comment    string `toml:",omitempty" legacy:"comment"`
}

type SkillUseAreaRow struct {
	MapNumber, X1, Y1, X2, Y2 int
	Comment                   string `toml:",omitempty" legacy:"comment"`
}

// LoadSkillConfig reads config/data/skill.toml.
func LoadSkillConfig(configDir string) (*SkillConfig, error) {
	return loadConfig[SkillConfig](configDir, "skill")
}
