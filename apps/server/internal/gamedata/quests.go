package gamedata

import ()

// Quest data (CQuest, CQuestObjective, CQuestReward; Quest/*.txt).
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

// Quests holds the three quest tables.
type Quests struct {
	Quests     []QuestInfo
	Objectives []QuestObjective
	Rewards    []QuestReward
}

// LoadQuests reads config/data/quest.toml.
func LoadQuests(configDir string) (*Quests, error) {
	f, err := loadConfig[QuestConfig](configDir, "quest")
	if err != nil {
		return nil, err
	}
	q := &Quests{}
	for _, r := range f.Quest.Rows {
		q.Quests = append(q.Quests, QuestInfo{r.Index, r.MonsterClass, r.CurrentState, r.RequireIndex, r.RequireState,
			r.RequireMinLevel, r.RequireMaxLevel, [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}})
	}
	for _, r := range f.QuestObjective.Rows {
		q.Objectives = append(q.Objectives, QuestObjective{r.Sort, r.Type, r.Index, r.Quantity, r.Level, r.Option1,
			r.Option2, r.Option3, r.NewOption, r.MapNumber, r.DropMinLevel, r.DropMaxLevel, r.ItemDropRate,
			r.RequireIndex, r.RequireState, [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}})
	}
	for _, r := range f.QuestReward.Rows {
		q.Rewards = append(q.Rewards, QuestReward{r.Sort, r.Type, r.Index, r.Quantity, r.Level, r.Option1, r.Option2,
			r.Option3, r.NewOption, r.RequireIndex, r.RequireState, [5]int{r.DW, r.DK, r.FE, r.MG, r.DL}})
	}
	return q, nil
}
