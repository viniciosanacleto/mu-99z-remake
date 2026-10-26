package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// SkillInfo is a Skill/SkillList.txt row (CSkillManager).
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

// LoadSkills reads Data/Skill/SkillList.txt.
func LoadSkills(dataDir string) (map[int]SkillInfo, error) {
	lines, err := jsonconf.LoadLines(filepath.Join(dataDir, "Skill", "SkillList.json"))
	if err != nil {
		return nil, err
	}
	out := map[int]SkillInfo{}
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) != 20 {
			return nil, fmt.Errorf("SkillList.txt line %d: %d columns", l.No, len(l.Tokens))
		}
		v := make([]int, 20)
		for i := range v {
			if i == 1 {
				continue
			}
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		s := SkillInfo{Index: v[0], Name: l.Tokens[1], Damage: v[2], Mana: v[3], BP: v[4], Range: v[5],
			Radio: v[6], Delay: v[7], Type: v[8], Effect: v[9], ReqLevel: v[10], ReqEnergy: v[11],
			ReqLeadership: v[12], ReqKillCount: v[13], ReqGuildStatus: v[14],
			Class: [5]int{v[15], v[16], v[17], v[18], v[19]}}
		out[s.Index] = s
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
