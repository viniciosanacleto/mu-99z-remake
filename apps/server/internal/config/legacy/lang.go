package legacy

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() { register("lang", langGroup) }

// langGroup converts Data/Lang/<Eng|Por|Spn>/Message.txt and Notice.txt.
func langGroup(dataDir string) ([]byte, error) {
	var c gamedata.LangConfig
	var steps []step
	for i, lang := range c.Languages() {
		name := [3]string{"Eng", "Por", "Spn"}[i]
		steps = append(steps,
			step{"Lang/" + name + "/Message.txt", func(l []mutext.Line) error {
				for _, r := range mutext.Table(l) {
					n, err := r.Int(0)
					if err != nil || len(r.Tokens) < 2 {
						return fmt.Errorf("line %d: bad row", r.No)
					}
					lang.Message.Rows = append(lang.Message.Rows, gamedata.MessageRow{Index: n, Text: tok(r, 1),
						Comment: lineComment(r)})
				}
				return nil
			}},
			step{"Lang/" + name + "/Notice.txt", func(l []mutext.Line) (err error) {
				lang.Notice.Rows, err = Table[gamedata.NoticeRow](l, false)
				return err
			}})
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}
