package legacy

import (
	"fmt"
	"strconv"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// characterGroup converts Data/Character/*.txt.
func characterGroup(dataDir string) ([]byte, error) {
	var c gamedata.CharacterConfig
	steps := []step{
		{"Character/DefaultClassInfo.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 12 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				var iv struct{ Class, Str, Dex, Vit, Ene, Lead int }
				if err := Row(r, &iv, true); err != nil {
					return err
				}
				var fv [6]float64
				for i := range fv {
					v, err := float32Value(r.Tokens[6+i])
					if err != nil {
						return fmt.Errorf("line %d column %d: %w", r.No, 7+i, err)
					}
					fv[i] = v
				}
				c.DefaultClassInfo.Rows = append(c.DefaultClassInfo.Rows, gamedata.DefaultClassInfoRow{Class: iv.Class,
					Strength: iv.Str, Dexterity: iv.Dex, Vitality: iv.Vit, Energy: iv.Ene, Leadership: iv.Lead,
					MaxLife: fv[0], MaxMana: fv[1], LevelLife: fv[2], LevelMana: fv[3], VitalityToLife: fv[4],
					EnergyToMana: fv[5], Comment: lineComment(r)})
			}
			return nil
		}},
		{"Character/DefaultClassFreebies.txt", sections(map[int]func([]mutext.Line) error{
			0: func(rows []mutext.Line) error {
				recs, comments := streamComments(rows, 3)
				for i, v := range recs {
					c.DefaultClassFreebies.Class = append(c.DefaultClassFreebies.Class, gamedata.FreebieClassRow{
						Class: v[0], InitialMoney: v[1], InitialPoint: v[2], Comment: comments[i]})
				}
				return nil
			},
			1: func(rows []mutext.Line) error {
				recs, comments := streamComments(rows, 11)
				for i, v := range recs {
					c.DefaultClassFreebies.Item = append(c.DefaultClassFreebies.Item, gamedata.FreebieItemRow{
						Class: v[0], ItemIndex: gamedata.FormatItemRef(v[1], v[2]), ItemLevel: v[3], Durability: v[4],
						SkillSwitch: v[5], LuckSwitch: v[6], Aditional: v[7], Excellent: v[8], SetOption: v[9],
						Duration: v[10], Comment: comments[i]})
				}
				return nil
			},
			2: func(rows []mutext.Line) error {
				recs, comments := streamComments(rows, 8)
				for i, v := range recs {
					c.DefaultClassFreebies.Effect = append(c.DefaultClassFreebies.Effect, gamedata.FreebieEffectRow{
						Class: v[0], EffectIndex: v[1], EffectType: v[2], Power: [4]int(v[3:7]), Duration: v[7],
						Comment: comments[i]})
				}
				return nil
			},
		})},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// float32Value parses a float column the way the loader stores it (a
// float32): the decimal value is kept when it rounds to the same float32,
// otherwise the float32 itself.
func float32Value(s string) (float64, error) {
	v32, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	v64, err := strconv.ParseFloat(s, 64)
	if err != nil || float32(v64) != float32(v32) {
		return v32, nil
	}
	return v64, nil
}
