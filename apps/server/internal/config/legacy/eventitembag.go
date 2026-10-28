package legacy

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// eventItemBagGroup converts Data/EventItemBagManager.txt and the bags it
// names (EventItemBag\<BagPath>.txt); other bag files are not read.
func eventItemBagGroup(dataDir string) ([]byte, error) {
	c := gamedata.EventItemBagConfig{Bag: map[string]gamedata.ItemBagFile{}}
	steps := []step{{"EventItemBagManager.txt", func(l []mutext.Line) error {
		for _, r := range mutext.Table(l) {
			if len(r.Tokens) < 7 {
				return fmt.Errorf("line %d: %d columns", r.No, len(r.Tokens))
			}
			v := mutext.NumRow(r.Tokens, 0, 6)
			c.EventItemBagManager.Rows = append(c.EventItemBagManager.Rows, gamedata.EventItemBagManagerRow{
				ItemIndex: v[0], ItemLevel: v[1], MonsterClass: v[2], SpecialValue: v[3], SpecialLevel: v[4],
				SpecialState: v[5], BagPath: tok(r, 6), Comment: lineComment(r)})
		}
		return nil
	}}}
	srcs, err := runSteps(dataDir, steps)
	if err != nil {
		return nil, err
	}
	for _, r := range c.EventItemBagManager.Rows {
		key := gamedata.BagKey(r.BagPath)
		if _, done := c.Bag[key]; done {
			continue
		}
		rel := "EventItemBag/" + key + ".txt"
		lines, err := File(filepath.Join(dataDir, filepath.FromSlash(tomlconf.ANSI(rel))))
		if err != nil {
			return nil, err
		}
		bag, err := itemBag(lines)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		c.Bag[key] = bag
		srcs = append(srcs, "Data/"+rel)
	}
	return marshal(&c, srcs...)
}

// itemBag reads the sections of a bag file like FUN_0048f2e0. A section 4
// row is attached to a bag of section 3 read before it, so rows naming a
// bag that is not defined yet are dropped here, as the original drops them.
func itemBag(lines []mutext.Line) (gamedata.ItemBagFile, error) {
	var b gamedata.ItemBagFile
	secs, err := mutext.Sections(lines)
	if err != nil {
		return b, err
	}
	seen := map[int]bool{}
	items := map[int]int{} // section -> position in b.Items
	for _, s := range secs {
		for _, l := range s.Rows {
			f := mutext.NumFields(l)
			c := lineComment(l)
			switch {
			case s.Index == 2:
				v := mutext.NumRow(f, 0, 4)
				b.Settings = append(b.Settings, gamedata.ItemBagSettingsRow{ItemRate: v[0], MoneyDrop: v[1],
					BagUseEffect: v[2], DropType: v[3], Comment: c})
			case s.Index == 3:
				v := mutext.NumRow(f, 0, 3)
				seen[v[0]] = true
				b.Bag = append(b.Bag, gamedata.ItemBagRow{BagIndex: v[0], DropRate: v[1], MapNumber: v[2], Comment: c})
			case s.Index == 4:
				v := mutext.NumRow(f, 0, 11)
				if !seen[v[0]] {
					continue
				}
				b.Drop = append(b.Drop, gamedata.ItemBagDropRow{BagIndex: v[0], Section: v[1], SectionRate: v[2],
					SetDropRate: v[3], MinDropCount: v[4], MaxDropCount: v[5], Class: [5]int(v[6:11]), Comment: c})
			case s.Index > 4:
				v := mutext.NumRow(f, 0, 12)
				pos, ok := items[s.Index]
				if !ok {
					pos = len(b.Items)
					items[s.Index] = pos
					b.Items = append(b.Items, gamedata.ItemBagSection{Section: s.Index})
				}
				b.Items[pos].Rows = append(b.Items[pos].Rows, gamedata.ItemBagItemRow{
					ItemIndex: gamedata.FormatItemRef(v[0], v[1]), ItemLevel: v[2], GradeCount: v[3],
					Option: [6]int(v[4:10]), Duration: v[10], Comment: strings.TrimSpace(c)})
			}
		}
	}
	return b, nil
}
