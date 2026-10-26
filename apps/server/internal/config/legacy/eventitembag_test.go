package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestEventItemBagGroup(t *testing.T) {
	cfg := convertGroup(t, "eventitembag", map[string]string{
		"EventItemBagManager.txt": "7179 0 * * * * \"Item\\Box of Luck\"\n7179 1 * * * * \"Item\\Box of Luck\"\nend\n",
		"EventItemBag/Item/Box of Luck.txt": "4\n1 5 10000 0 1 1 1 1 1 1 1 // before bag 1: dropped\nend\n" +
			"2\n50 0 0 0\n100 10 3 1\nend\n" +
			"3\n1 10000 *\n0 5000 2\n1 1 1 // duplicate: ignored\nend\n" +
			"4\n1 5 10000 0 1 1 1 1 1 1 1\n1 6 10 0 5 2 1 1 1 1 1 // invalid count\n0 -1 10 0 1 1 2 0 0 0 0\nend\n" +
			"5\n00,006 4 0 * * * * * * 0 //Gladius\n99,000 0 0 * * * * * * 60\nend\n" +
			"6\n14,013 0 0 0 0 0 0 0 0 0\nend\n" +
			"5\n00,007 4 0 * * * * * * 0\nend\n",
		"EventItemBag/Special/Quiz.txt": "not named, not read",
	})
	bags, err := gamedata.LoadEventItemBags(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(bags) != 2 || bags[0].Path != `Item\Box of Luck` || bags[1].ItemLevel != 1 || bags[0].MonsterClass != -1 {
		t.Fatalf("manager = %+v", bags)
	}
	b := bags[0].Bag
	if b.ItemRate != 100 || b.MoneyDrop != 10 || b.BagUseEffect != 3 || b.DropType != 1 {
		t.Fatalf("settings (last row wins) = %+v", b)
	}
	if len(b.Bags) != 2 || b.Bags[0].Index != 0 || b.Bags[1].Index != 1 || b.Bags[1].Map != -1 || b.Bags[1].DropRate != 10000 {
		t.Fatalf("bags (sorted, first index wins) = %+v", b.Bags)
	}
	if d := b.Bags[1].Drops; len(d) != 1 || d[0].Section != 5 {
		t.Fatalf("bag 1 drops (the row before the bag and the invalid count are dropped) = %+v", d)
	}
	if d := b.Bags[0].Drops; len(d) != 1 || d[0].Section != -1 || d[0].Class != [5]int{2, 0, 0, 0, 0} {
		t.Fatalf("bag 0 drops = %+v", d)
	}
	s5 := b.Sections[5]
	if len(s5) != 3 || s5[0].Index != 6 || s5[0].Level != 4 || s5[0].Option != [6]int{-1, -1, -1, -1, -1, -1} ||
		s5[1].Index != -1 || s5[1].Duration != 60 || s5[2].Index != 7 || len(b.Sections[6]) != 1 {
		t.Fatalf("items = %+v", b.Sections)
	}
}
