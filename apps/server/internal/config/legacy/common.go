package legacy

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// commonGroup converts the files at the root of Data/.
func commonGroup(dataDir string) ([]byte, error) {
	var c gamedata.CommonConfig
	steps := []step{
		{"MapManager.txt", func(l []mutext.Line) (err error) {
			c.MapManager.Rows, err = Table[gamedata.MapManagerRow](l, false)
			return err
		}},
		{"MapServerInfo.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				for _, r := range s.Rows {
					switch s.Index {
					case 0:
						if len(r.Tokens) < 5 {
							return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
						}
						// The code, group and InitSetVal columns read 0 when they are not numbers.
						code, _ := r.Int(0)
						group, _ := r.Int(1)
						init, _ := r.Int(2)
						port, err := r.Int(4)
						if err != nil {
							return err
						}
						c.MapServerInfo.Server = append(c.MapServerInfo.Server, gamedata.MapServerRow{ServerCode: code,
							ServerGroup: group, InitSetVal: init, IpAddress: tok(r, 3), Port: port, Comment: lineComment(r)})
					case 1:
						var row gamedata.MapServerMoveRow
						if err := Row(r, &row, true); err != nil {
							return err
						}
						c.MapServerInfo.Move = append(c.MapServerInfo.Move, row)
					}
				}
			}
			return nil
		}},
		{"MapRateInfo.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 8)
				row := gamedata.MapRateInfoRow{MapNumber: v[0], ItemDropType: v[1], Comment: lineComment(r)}
				copy(row.Option[:], v[2:])
				c.MapRateInfo.Rows = append(c.MapRateInfo.Rows, row)
			}
			return nil
		}},
	}
	steps = append(steps,
		step{"BlackList.txt", sections(map[int]func([]mutext.Line) error{
			0: func(rows []mutext.Line) error {
				c.BlackList.IpAddress = append(c.BlackList.IpAddress, firstOfRows(rows)...)
				return nil
			},
			1: func(rows []mutext.Line) error {
				c.BlackList.HardwareId = append(c.BlackList.HardwareId, firstOfRows(rows)...)
				return nil
			},
		})},
		step{"CommandManager.txt", func(l []mutext.Line) (err error) {
			c.CommandManager.Rows, err = Table[gamedata.CommandManagerRow](l, false)
			return err
		}},
		step{"CommandRequirement.txt", func(l []mutext.Line) error {
			c.CommandRequirement.Rows = commandRequirements(mutext.Table(l))
			return nil
		}},
		step{"ConnectMember.txt", func(l []mutext.Line) error {
			c.ConnectMember.Account = FirstTokens(l)
			return nil
		}},
	)
	srcs, err := runSteps(dataDir, steps)
	if c.BlackList.IpAddress == nil {
		c.BlackList.IpAddress = []string{}
	}
	if c.BlackList.HardwareId == nil {
		c.BlackList.HardwareId = []string{}
	}
	if c.ConnectMember.Account == nil {
		c.ConnectMember.Account = []string{}
	}
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// firstOfRows is the first token of each row, in UTF-8.
func firstOfRows(rows []mutext.Line) []string {
	var out []string
	for _, r := range rows {
		out = append(out, tok(r, 0))
	}
	return out
}

// commandRequirements reads CommandRequirement.txt like FUN_004402a0: a
// stream of numbers where ReqItemIndex is "*" (one number) or an item
// reference (two numbers).
func commandRequirements(rows []mutext.Line) []gamedata.CommandRequirementRow {
	var f, owner []string
	for _, l := range rows {
		fs := mutext.NumFields(l)
		for range fs {
			owner = append(owner, lineComment(l))
		}
		f = append(f, fs...)
	}
	var out []gamedata.CommandRequirementRow
	pos, prev := 0, 0
	next := func() int {
		tok := ""
		if pos < len(f) {
			tok = f[pos]
		}
		pos++
		prev = mutext.NumTok(tok, prev)
		return prev
	}
	for pos < len(f) {
		r := gamedata.CommandRequirementRow{Comment: owner[pos], ReqItemIndex: "*"}
		r.Index, r.AccountLevel, r.MinLevel, r.MaxLevel = next(), next(), next(), next()
		r.MinReset, r.MaxReset, r.CheckItem = next(), next(), next()
		if cat := next(); cat != -1 {
			r.ReqItemIndex = gamedata.FormatItemRef(cat, next())
		}
		r.ReqItemCount, r.ReqItemLevel, r.ReqEffectState = next(), next(), next()
		r.ReqQuestIndex, r.ReqQuestState = next(), next()
		r.ReqPKLevel, r.ReqPKCount, r.ReqGuildRank = next(), next(), next()
		r.MapIndex, r.MapStartX, r.MapStartY, r.MapEndX, r.MapEndY = next(), next(), next(), next(), next()
		r.DayOfWeek, r.MinHour, r.MaxHour, r.FailMessage = next(), next(), next(), next()
		r.DW, r.DK, r.FE, r.MG, r.DL = next(), next(), next(), next(), next()
		out = append(out, r)
	}
	return out
}
