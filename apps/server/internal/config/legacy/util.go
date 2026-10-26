package legacy

import (
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() {
	register("hack", hackGroup)
	register("util", utilGroup)
}

// hackGroup converts the Data/Hack tables.
func hackGroup(dataDir string) ([]byte, error) {
	var c gamedata.HackConfig
	steps := []step{
		{"Hack/HackPacketCheck.txt", func(l []mutext.Line) (err error) {
			c.HackPacketCheck.Rows, err = Table[gamedata.HackPacketCheckRow](l, false)
			return err
		}},
		{"Hack/HackSkillSpeedCheck.txt", func(l []mutext.Line) error {
			recs, comments := streamComments(mutext.Table(l), 12)
			for i, v := range recs {
				c.HackSkillSpeedCheck.Rows = append(c.HackSkillSpeedCheck.Rows, gamedata.HackSkillSpeedRow{Index: v[0],
					Class: v[1], MinLevel: v[2], MaxLevel: v[3], MinReset: v[4], MaxReset: v[5], MinSpeed: v[6],
					MaxSpeed: v[7], MaxDelay: v[8], MinCount: v[9], MaxCount: v[10], Action: v[11], Comment: comments[i]})
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

// utilGroup converts Data/Util/*.txt.
func utilGroup(dataDir string) ([]byte, error) {
	var c gamedata.UtilConfig
	steps := []step{
		{"Util/DamageTable.txt", sections(map[int]func([]mutext.Line) error{
			0: rowsInto(&c.DamageTable.Monster),
			1: rowsInto(&c.DamageTable.Player),
		})},
		{"Util/ExperienceTable.txt", func(l []mutext.Line) error {
			// Seven numbers per record (FUN_0047be80), whatever the line breaks.
			recs, comments := streamComments(mutext.Table(l), 7)
			for i, v := range recs {
				c.ExperienceTable.Rows = append(c.ExperienceTable.Rows, gamedata.ExperienceTableRow{MinLevel: v[0],
					MaxLevel: v[1], MinReset: v[2], MaxReset: v[3], MinMasterReset: v[4], MaxMasterReset: v[5],
					ExperienceRate: v[6], Comment: comments[i]})
			}
			return nil
		}},
		{"Util/Filter.txt", func(l []mutext.Line) error {
			c.Filter.Words = FirstTokens(l)
			if c.Filter.Words == nil {
				c.Filter.Words = []string{}
			}
			return nil
		}},
		{"Util/GameMaster.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 3 { // rows without a level are skipped
					continue
				}
				lv, err := r.Int(2)
				if err != nil {
					return err
				}
				c.GameMaster.Rows = append(c.GameMaster.Rows, gamedata.GameMasterRow{Account: tok(r, 0), Name: tok(r, 1),
					GMLevel: lv, Comment: lineComment(r)})
			}
			return nil
		}},
		{"Util/PetDamageTable.txt", func(l []mutext.Line) error {
			recs, comments := streamComments(mutext.Table(l), 6)
			for i, v := range recs {
				c.PetDamageTable.Rows = append(c.PetDamageTable.Rows, gamedata.PetDamageTableRow{Index: v[0],
					MinLevel: v[1], MaxLevel: v[2], MinReset: v[3], MaxReset: v[4], DamageRate: v[5], Comment: comments[i]})
			}
			return nil
		}},
		{"Util/ResetLimit.txt", func(l []mutext.Line) (err error) {
			c.ResetLimit.Rows, err = Table[gamedata.ResetLimitRow](l, true)
			return err
		}},
		{"Util/ResetTable.txt", func(l []mutext.Line) (err error) {
			c.ResetTable.Rows, err = Table[gamedata.ResetTableRow](l, true)
			return err
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}
