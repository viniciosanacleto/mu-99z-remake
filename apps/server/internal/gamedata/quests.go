package gamedata

import (
	"fmt"
	"path/filepath"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// Quest data (CQuest, CQuestObjective, CQuestReward; Data/Quest/*.txt).
// "*" reads as -1.

// QuestInfo is a Quest.txt row: Index MonsterClass CurrentState
// RequireIndex RequireState RequireMinLevel RequireMaxLevel DW DK FE MG DL.
type QuestInfo struct {
	Index, NPC, State             int
	RequireIndex, RequireState    int
	RequireMinLevel, RequireMaxLv int
	Class                         [5]int
}

// QuestObjective is a QuestObjective.txt row: Sort Type Index Quantity
// Level Option1 Option2 Option3 NewOption MapNumber DropMinLevel
// DropMaxLevel ItemDropRate RequireIndex RequireState DW DK FE MG DL.
type QuestObjective struct {
	Sort, Type, Index, Quantity   int
	Level, Option1, Option2       int
	Option3, NewOption            int
	Map, DropMinLevel, DropMaxLvl int
	ItemDropRate                  int
	RequireIndex, RequireState    int
	Class                         [5]int
}

// QuestReward is a QuestReward.txt row: Sort Type Index Quantity Level
// Option1 Option2 Option3 NewOption RequireIndex RequireState DW DK FE MG DL.
type QuestReward struct {
	Sort, Type, Index, Quantity int
	Level, Option1, Option2     int
	Option3, NewOption          int
	RequireIndex, RequireState  int
	Class                       [5]int
}

func loadIntTable(path string, cols int) ([][]int, error) {
	lines, err := mutext.LoadFile(path)
	if err != nil {
		return nil, err
	}
	var out [][]int
	for _, l := range mutext.Table(lines) {
		if len(l.Tokens) < cols {
			return nil, fmt.Errorf("%s line %d: %d columns, want %d", filepath.Base(path), l.No, len(l.Tokens), cols)
		}
		v := make([]int, cols)
		for i := range v {
			if v[i], err = l.Int(i); err != nil {
				return nil, err
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func class5(v []int) [5]int { return [5]int{v[0], v[1], v[2], v[3], v[4]} }

// Quests holds the three quest tables.
type Quests struct {
	Quests     []QuestInfo
	Objectives []QuestObjective
	Rewards    []QuestReward
}

// LoadQuests reads Data/Quest/Quest.txt, QuestObjective.txt and QuestReward.txt.
func LoadQuests(dataDir string) (*Quests, error) {
	dir := filepath.Join(dataDir, "Quest")
	q := &Quests{}
	rows, err := loadIntTable(filepath.Join(dir, "Quest.txt"), 12)
	if err != nil {
		return nil, err
	}
	for _, v := range rows {
		q.Quests = append(q.Quests, QuestInfo{v[0], v[1], v[2], v[3], v[4], v[5], v[6], class5(v[7:])})
	}
	if rows, err = loadIntTable(filepath.Join(dir, "QuestObjective.txt"), 20); err != nil {
		return nil, err
	}
	for _, v := range rows {
		q.Objectives = append(q.Objectives, QuestObjective{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8],
			v[9], v[10], v[11], v[12], v[13], v[14], class5(v[15:])})
	}
	if rows, err = loadIntTable(filepath.Join(dir, "QuestReward.txt"), 16); err != nil {
		return nil, err
	}
	for _, v := range rows {
		q.Rewards = append(q.Rewards, QuestReward{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8],
			v[9], v[10], class5(v[11:])})
	}
	return q, nil
}
