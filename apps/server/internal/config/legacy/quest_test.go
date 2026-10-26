package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestQuestGroup(t *testing.T) {
	cfg := convertGroup(t, "quest", map[string]string{
		"Quest/Quest.txt":          "1 235 3 0 2 150 * 1 1 1 0 0\nend\n",
		"Quest/QuestObjective.txt": "0 1 7191 1 0 0 0 0 0 * 45 60 10 0 1 1 1 1 0 0 // Scroll of Emperor\nend\n",
		"Quest/QuestReward.txt":    "0 1 200 10 0 0 0 0 0 0 1 1 1 1 0 0 extra // Gain Points\nend\n",
	})
	q, err := gamedata.LoadQuests(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Quests) != 1 || q.Quests[0].NPC != 235 || q.Quests[0].RequireMaxLv != -1 || q.Quests[0].Class != [5]int{1, 1, 1, 0, 0} {
		t.Fatalf("quests = %+v", q.Quests)
	}
	if len(q.Objectives) != 1 || q.Objectives[0].Index != 7191 || q.Objectives[0].Map != -1 || q.Objectives[0].ItemDropRate != 10 {
		t.Fatalf("objectives = %+v", q.Objectives)
	}
	if len(q.Rewards) != 1 || q.Rewards[0].Quantity != 10 || q.Rewards[0].RequireState != 1 {
		t.Fatalf("rewards = %+v", q.Rewards)
	}
	if _, err := tryConvertGroup(t, "quest", map[string]string{"Quest/Quest.txt": "1 235 3\nend\n"}); err == nil {
		t.Fatal("short row accepted")
	}
}
