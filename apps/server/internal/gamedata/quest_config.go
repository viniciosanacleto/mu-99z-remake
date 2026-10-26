package gamedata

// QuestConfig is config/data/quest.toml: Data/Quest/*.txt; -1 = any/none.
type QuestConfig struct {
	Quest          QuestFile          `comment:"Quest.txt (CQuest): MonsterClass = the quest NPC; DW..DL 0 no, 1 yes, 2 second class only"`
	QuestObjective QuestObjectiveFile `comment:"QuestObjective.txt (CQuestObjective)"`
	QuestReward    QuestRewardFile    `comment:"QuestReward.txt (CQuestReward)"`
}

type QuestFile struct {
	Rows []QuestRow `toml:",inline"`
}

type QuestRow struct {
	Index, MonsterClass, CurrentState, RequireIndex, RequireState int
	RequireMinLevel, RequireMaxLevel                              int
	DW, DK, FE, MG, DL                                            int
	Comment                                                       string `toml:",omitempty" legacy:"comment"`
}

type QuestObjectiveFile struct {
	Rows []QuestObjectiveRow `toml:",inline"`
}

type QuestObjectiveRow struct {
	Sort, Type, Index, Quantity, Level, Option1, Option2, Option3, NewOption int
	MapNumber, DropMinLevel, DropMaxLevel, ItemDropRate                      int
	RequireIndex, RequireState                                               int
	DW, DK, FE, MG, DL                                                       int
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}

type QuestRewardFile struct {
	Rows []QuestRewardRow `toml:",inline"`
}

type QuestRewardRow struct {
	Sort, Type, Index, Quantity, Level, Option1, Option2, Option3, NewOption int
	RequireIndex, RequireState                                               int
	DW, DK, FE, MG, DL                                                       int
	Comment                                                                  string `toml:",omitempty" legacy:"comment"`
}
