package legacy

import (
	"fmt"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// skillGroup converts Data/Skill/*.txt.
func skillGroup(dataDir string) ([]byte, error) {
	var c gamedata.SkillConfig
	steps := []step{
		{"Skill/SkillList.txt", func(l []mutext.Line) (err error) {
			c.SkillList.Rows, err = Table[gamedata.SkillListRow](l, false)
			return err
		}},
		{"Skill/EffectList.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				row, err := effectRow(r)
				if err != nil {
					return err
				}
				c.EffectList.Rows = append(c.EffectList.Rows, row)
			}
			return nil
		}},
		{"Skill/SkillDamage.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				for _, r := range s.Rows {
					// Every row starts with two numbers (FUN_004f7530 readers).
					var two struct{ A, B int }
					if err := Row(r, &two, true); err != nil {
						return err
					}
					switch s.Index {
					case 0:
						if len(r.Tokens) < 3 { // no PvP rate: the row is ignored
							continue
						}
						var row gamedata.SkillDamageRow
						if err := Row(r, &row, true); err != nil {
							return err
						}
						c.SkillDamage.Damage = append(c.SkillDamage.Damage, row)
					case 1:
						var row gamedata.SkillEffectRateRow
						if err := Row(r, &row, true); err != nil {
							return err
						}
						c.SkillDamage.Effect = append(c.SkillDamage.Effect, row)
					}
				}
			}
			return nil
		}},
		{"Skill/ComboSkill.txt", func(l []mutext.Line) (err error) {
			c.ComboSkill.Rows, err = Table[gamedata.ComboSkillRow](l, true)
			return err
		}},
		{"Skill/SkillEnabler.txt", func(l []mutext.Line) (err error) {
			c.SkillEnabler.Rows, err = Table[gamedata.SkillEnablerRow](l, true)
			return err
		}},
		{"Skill/SkillUseArea.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				switch s.Index {
				case 0:
					rows, err := Rows[gamedata.SkillUseAreaSkillRow](s.Rows, true)
					if err != nil {
						return err
					}
					c.SkillUseArea.Skill = append(c.SkillUseArea.Skill, rows...)
				case 1:
					rows, err := Rows[gamedata.SkillUseAreaRow](s.Rows, true)
					if err != nil {
						return err
					}
					c.SkillUseArea.Area = append(c.SkillUseArea.Area, rows...)
				}
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

// effectRow reads an EffectList.txt row (FUN_004786c0): Index Group, then
// the item as two numbers (category index) or a single "*", then Name Save
// Type Flag Duration Power1..4.
func effectRow(l mutext.Line) (gamedata.EffectListRow, error) {
	var err error
	v := func(i int) int {
		n, e := l.Int(i)
		if e != nil && err == nil {
			err = e
		}
		return n
	}
	r := gamedata.EffectListRow{Index: v(0), Group: v(1), ItemIndex: "*", Comment: lineComment(l)}
	col := 2
	if cat := v(col); cat != -1 {
		col++
		r.ItemIndex = gamedata.FormatItemRef(cat, v(col))
	}
	col++
	if col >= len(l.Tokens) {
		return r, fmt.Errorf("line %d: missing name", l.No)
	}
	r.Name = tok(l, col)
	r.Save, r.Type, r.Flag, r.Duration = v(col+1), v(col+2), v(col+3), v(col+4)
	for i := range r.Power {
		r.Power[i] = v(col + 5 + i)
	}
	return r, err
}
