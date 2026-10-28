package legacy

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// questGroup converts Data/Quest/*.txt: tables of numbers ("*" = -1);
// columns past the expected ones are ignored.
func questGroup(dataDir string) ([]byte, error) {
	var c gamedata.QuestConfig
	steps := []step{
		{"Quest/Quest.txt", func(l []mutext.Line) (err error) {
			c.Quest.Rows, err = Table[gamedata.QuestRow](l, true)
			return err
		}},
		{"Quest/QuestObjective.txt", func(l []mutext.Line) (err error) {
			c.QuestObjective.Rows, err = Table[gamedata.QuestObjectiveRow](l, true)
			return err
		}},
		{"Quest/QuestReward.txt", func(l []mutext.Line) (err error) {
			c.QuestReward.Rows, err = Table[gamedata.QuestRewardRow](l, true)
			return err
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}
