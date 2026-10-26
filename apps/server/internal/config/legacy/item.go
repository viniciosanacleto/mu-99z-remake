package legacy

import (
	"fmt"
	"reflect"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func init() { register("item", itemGroup) }

// itemGroup converts Data/Item/*.txt into config/data/item.toml.
func itemGroup(dataDir string) ([]byte, error) {
	var c gamedata.ItemConfig
	steps := []step{
		{"Item/Item.txt", func(l []mutext.Line) error { return items(l, &c.Item) }},
		{"Item/ItemOption.txt", func(l []mutext.Line) (err error) {
			c.ItemOption.Rows, err = Table[gamedata.ItemOptionRow](l, true)
			return err
		}},
		{"Item/ItemOptionRate.txt", func(l []mutext.Line) error { return optionRates(l, &c.ItemOptionRate) }},
		{"Item/ExcellentOptionRate.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 5 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				c.ExcellentOptionRate.Rows = append(c.ExcellentOptionRate.Rows, gamedata.ExcellentOptionRateRow{
					Index: mutext.NumTok(r.Tokens[0], 0), ItemMinIndex: r.Tokens[1], ItemMaxIndex: r.Tokens[2],
					Name: tomlconf.UTF8(r.Tokens[3]), DropRate: mutext.NumTok(r.Tokens[4], 0), Comment: lineComment(r)})
			}
			return nil
		}},
		{"Item/ItemDrop.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 19)
				c.ItemDrop.Rows = append(c.ItemDrop.Rows, gamedata.ItemDropRow{
					ItemIndex: gamedata.FormatItemRef(v[0], v[1]), ItemLevel: v[2], ItemGrade: v[3],
					LevelOption: v[4], SkillOption: v[5], LuckOption: v[6], JoLOption: v[7], ExcOption: v[8], SetOption: v[9],
					Duration: v[10], MapNumber: v[11], MonsterIndex: v[12], MonsterMinLevel: v[13], MonsterMaxLevel: v[14],
					DropRate: [4]int{v[15], v[16], v[17], v[18]}, Comment: lineComment(r)})
			}
			return nil
		}},
		{"Item/ItemMoneyDrop.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 7)
				c.ItemMoneyDrop.Rows = append(c.ItemMoneyDrop.Rows, gamedata.ItemMoneyDropRow{
					MapNumber: v[0], MonsterIndex: v[1], MonsterMinLevel: v[2], MonsterMaxLevel: v[3], AccountLevel: v[4],
					MoneyAmount: v[5], MoneyDropRate: v[6], Comment: lineComment(r)})
			}
			return nil
		}},
		{"Item/ItemLevel.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 3 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				v := mutext.NumRow(mutext.NumFields(mutext.Line{Tokens: r.Tokens[:2]}), 0, 3)
				c.ItemLevel.Rows = append(c.ItemLevel.Rows, gamedata.ItemLevelRow{
					ItemIndex: gamedata.FormatItemRef(v[0], v[1]), ItemLevel: v[2], Name: tomlconf.UTF8(r.Tokens[2]),
					Comment: lineComment(r)})
			}
			return nil
		}},
		{"Item/ItemLoot.txt", func(l []mutext.Line) (err error) {
			c.ItemLoot.Rows, err = Table[gamedata.ItemLootRow](l, true)
			return err
		}},
		{"Item/ItemMove.txt", func(l []mutext.Line) (err error) {
			c.ItemMove.Rows, err = Table[gamedata.ItemMoveRow](l, true)
			return err
		}},
		{"Item/ItemStack.txt", func(l []mutext.Line) error {
			secs, err := mutext.Sections(l)
			if err != nil {
				return err
			}
			for _, s := range secs {
				var err error
				switch s.Index {
				case 0:
					var rows []gamedata.ItemStackRow
					rows, err = Rows[gamedata.ItemStackRow](s.Rows, true)
					c.ItemStack.Stack = append(c.ItemStack.Stack, rows...)
				case 1:
					var rows []gamedata.ItemStackPackRow
					rows, err = Rows[gamedata.ItemStackPackRow](s.Rows, true)
					c.ItemStack.Pack = append(c.ItemStack.Pack, rows...)
				}
				if err != nil {
					return err
				}
			}
			return nil
		}},
		{"Item/ItemValue.txt", func(l []mutext.Line) (err error) {
			c.ItemValue.Rows, err = Table[gamedata.ItemValueRow](l, true)
			return err
		}},
		{"Item/SetItemType.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				v := mutext.NumRow(mutext.NumFields(r), 0, 6)
				c.SetItemType.Rows = append(c.SetItemType.Rows, gamedata.SetItemTypeRow{
					ItemIndex: gamedata.FormatItemRef(v[0], v[1]), StatType: v[2], OptionIndex1: v[3], OptionIndex2: v[4],
					ItemDropRate: v[5], Comment: lineComment(r)})
			}
			return nil
		}},
		{"Item/SetItemOption.txt", func(l []mutext.Line) error {
			for _, r := range mutext.Table(l) {
				if len(r.Tokens) < 2+34 {
					return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
				}
				v := mutext.NumRow(r.Tokens, 2, 34)
				row := gamedata.SetItemOptionRow{Index: mutext.NumTok(r.Tokens[0], 0), Name: tomlconf.UTF8(r.Tokens[1]),
					Comment: lineComment(r)}
				for k := range row.Option {
					row.Option[k] = [2]int{v[2*k], v[2*k+1]}
				}
				for k := range row.FullOption {
					row.FullOption[k] = [2]int{v[24+2*k], v[24+2*k+1]}
				}
				c.SetItemOption.Rows = append(c.SetItemOption.Rows, row)
			}
			return nil
		}},
		{"Item/WingOption.txt", func(l []mutext.Line) (err error) {
			c.WingOption.Rows, err = Table[gamedata.WingOptionRow](l, true)
			return err
		}},
	}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	return marshal(&c, srcs...)
}

// marshal encodes a group file with the generated-file header.
func marshal(v any, srcs ...string) ([]byte, error) {
	b, err := tomlconf.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append([]byte(header(srcs...)+"\n"), b...), nil
}

// lineComment is the Comment column of a line, in UTF-8.
func lineComment(l mutext.Line) string { return tomlconf.UTF8(l.Comment) }

// itemCategories are the ItemTable fields by category number.
func items(lines []mutext.Line, t *gamedata.ItemTable) error {
	secs, err := mutext.Sections(lines)
	if err != nil {
		return err
	}
	cats := t.Categories()
	for _, s := range secs {
		if s.Index < 0 || s.Index >= len(cats) {
			return fmt.Errorf("unknown category %d", s.Index)
		}
		dst := cats[s.Index]
		for _, l := range s.Rows {
			row := reflect.New(dst.Type().Elem())
			if err := Row(l, row.Interface(), false); err != nil {
				return fmt.Errorf("category %d: %w", s.Index, err)
			}
			dst.Set(reflect.Append(dst, row.Elem()))
		}
	}
	return nil
}

// optionRates reads the six sections of ItemOptionRate.txt: Index then
// OptionRateCounts[section] rates, read as numbers (FUN_0049e4b0).
func optionRates(lines []mutext.Line, f *gamedata.ItemOptionRateFile) error {
	secs, err := mutext.Sections(lines)
	if err != nil {
		return err
	}
	dst := [6]*[]gamedata.OptionRateRow{&f.Level, &f.Skill, &f.Luck, &f.Option, &f.Excellent, &f.Set}
	for _, s := range secs {
		if s.Index < 0 || s.Index > 5 {
			continue
		}
		n := gamedata.OptionRateCounts[s.Index]
		for _, l := range s.Rows {
			v := mutext.NumRow(mutext.NumFields(l), 0, n+1)
			*dst[s.Index] = append(*dst[s.Index], gamedata.OptionRateRow{Index: v[0], Rate: v[1:], Comment: lineComment(l)})
		}
	}
	return nil
}
