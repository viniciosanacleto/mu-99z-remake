package legacy

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() { register("move", moveGroup) }

// moveGroup converts Data/Move/*.txt.
func moveGroup(dataDir string) ([]byte, error) {
	var c gamedata.MoveConfig
	steps := []step{
		{"Move/Gate.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 12 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				v := make([]int, len(r.Tokens))
				for i := range v {
					var err error
					if v[i], err = r.Int(i); err != nil {
						return err
					}
				}
				// MaxReset and the account level column are optional (-1).
				v = append(v, -1, -1)
				c.Gate.Rows = append(c.Gate.Rows, gamedata.GateRow{GateIndex: v[0], GateFlag: v[1], MapNumber: v[2],
					RangeStartX: v[3], RangeStartY: v[4], RangeEndX: v[5], RangeEndY: v[6], TargetGate: v[7], TargetDir: v[8],
					MinLevel: v[9], MaxLevel: v[10], MinReset: v[11], MaxReset: v[12], AccountLevel: v[13],
					Comment: lineComment(r)})
			}
			return nil
		}},
		{"Move/Move.txt", func(l []mutext.Line) (err error) {
			c.Move.Rows, err = Table[gamedata.MoveRow](l, false)
			return err
		}},
		{"Move/MoveSummon.txt", func(l []mutext.Line) error {
			recs, comments := streamComments(mutext.Table(l), 13)
			for i, v := range recs {
				c.MoveSummon.Rows = append(c.MoveSummon.Rows, gamedata.MoveSummonConfigRow{MapNumber: v[0],
					RangeStartX: v[1], RangeStartY: v[2], RangeEndX: v[3], RangeEndY: v[4], MinLevel: v[5], MaxLevel: v[6],
					MinReset: v[7], MaxReset: v[8], AL0: v[9], AL1: v[10], AL2: v[11], AL3: v[12], Comment: comments[i]})
			}
			return nil
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}
