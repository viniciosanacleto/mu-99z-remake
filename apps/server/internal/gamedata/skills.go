package gamedata

import ()

// SkillInfo is a SkillList.txt row (CSkillManager).
type SkillInfo struct {
	Index          int
	Name           string
	Damage         int
	Mana           int
	BP             int
	Range          int
	Radio          int
	Delay          int
	Type           int // -1 = "*"
	Effect         int
	ReqLevel       int
	ReqEnergy      int
	ReqLeadership  int
	ReqKillCount   int
	ReqGuildStatus int
	Class          [5]int // DW DK FE MG DL
}

// LoadSkills reads the SkillList table of config/data/skill.toml.
func LoadSkills(configDir string) (map[int]SkillInfo, error) {
	f, err := LoadSkillConfig(configDir)
	if err != nil {
		return nil, err
	}
	out := map[int]SkillInfo{}
	for _, r := range f.SkillList.Rows {
		out[r.Index] = SkillInfo{Index: r.Index, Name: r.Name, Damage: r.Damage, Mana: r.MP, BP: r.BP, Range: r.Range,
			Radio: r.Radio, Delay: r.Delay, Type: r.Type, Effect: r.Effect, ReqLevel: r.ReqLevel, ReqEnergy: r.ReqEnergy,
			ReqLeadership: r.ReqLeadership, ReqKillCount: r.ReqKillCount, ReqGuildStatus: r.ReqGuildStatus,
			Class: [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}}
	}
	return out, nil
}

// CanLearn reproduces the checks of FUN_004f8b60 for players: level,
// energy (base + add), leadership (base + add) and the class column; classes
// above 4 have no column and never qualify.
func (s SkillInfo) CanLearn(level, energy, leadership, class int) bool {
	if level < s.ReqLevel || energy < s.ReqEnergy || leadership < s.ReqLeadership {
		return false
	}
	return class >= 0 && class < 5 && s.Class[class] != 0
}
