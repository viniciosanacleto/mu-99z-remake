package gamedata

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

func fixedRnd(vals ...int) func() int {
	i := 0
	return func() int { v := vals[i%len(vals)]; i++; return v }
}

func TestNumTokLikeOriginalTokenizer(t *testing.T) {
	cases := []struct {
		tok  string
		prev int
		want int
	}{{"12", 0, 12}, {"*", 5, -1}, {"N", 7, 7}, {"-3", 0, -3}, {"2.9", 0, 2}, {"", 4, 4}}
	for _, c := range cases {
		if got := numTok(c.tok, c.prev); got != c.want {
			t.Errorf("numTok(%q,%d) = %d want %d", c.tok, c.prev, got, c.want)
		}
	}
	l := mutext.Line{Tokens: []string{"07,000", "0", "0,", "8", "N"}}
	if got := numRow(numFields(l), 0, 6); got[0] != 7 || got[1] != 0 || got[3] != 0 || got[4] != 8 || got[5] != 8 {
		t.Fatalf("numRow = %v", got)
	}
}

func TestItemOptionRateAndMapRate(t *testing.T) {
	r, err := LoadItemOptionRate(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Sections[0][0][:4]; got[0] != 7000 || got[1] != 1500 || got[2] != 1000 || got[3] != 500 {
		t.Fatalf("level common = %v", got)
	}
	if len(r.Sections[3][0]) != 8 || len(r.Sections[4][1]) != 7 || r.Sections[4][1][1] != 7500 {
		t.Fatalf("JoL/Exc = %v %v", r.Sections[3][0], r.Sections[4][1])
	}
	// rand%10000: 6999 -> +0, 7000 -> +1, 8500 -> +2, 9999 -> +3.
	for _, c := range [][2]int{{6999, 0}, {7000, 1}, {8500, 2}, {9999, 3}} {
		if v, ok := r.Pick(0, 0, fixedRnd(c[0])); !ok || v != c[1] {
			t.Errorf("Pick(level, %d) = %d %v want %d", c[0], v, ok, c[1])
		}
	}
	// Exc option (Exc) row 1: 0 / 7500 / 2500 -> 1 or 2 options.
	if v, _ := r.Pick(4, 1, fixedRnd(7499)); v != 1 {
		t.Fatalf("exc count = %d", v)
	}
	if v, _ := r.Pick(4, 1, fixedRnd(7500)); v != 2 {
		t.Fatalf("exc count = %d", v)
	}
	if _, ok := r.Pick(4, -1, fixedRnd(0)); ok {
		t.Fatal("missing row picked")
	}
	rows, err := LoadMapRateInfo(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || MapRateOption(rows, 0, 1, 4) != 1 || MapRateOption(rows, 0, 0, 4) != -1 ||
		MapRateOption(rows, 7, 2, 5) != 2 || MapRateOption(rows, 0, 3, 0) != 0 {
		t.Fatalf("map rate rows = %+v", rows)
	}
}

func TestExcellentOptionRate(t *testing.T) {
	rows, err := LoadExcellentOptionRate(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	// Sword (0,0): weights 60,60,50,20,20,10 -> total 220.
	for _, c := range [][2]int{{0, 1}, {59, 1}, {60, 2}, {170, 8}, {219, 32}} {
		if got := PickExcellentOption(rows, 0, fixedRnd(c[0])); got != c[1] {
			t.Errorf("pick %d = %d want %d", c[0], got, c[1])
		}
	}
	// Shield (6,0): first option is money drop.
	if got := PickExcellentOption(rows, 6*512, fixedRnd(0)); got != 1 {
		t.Fatalf("shield = %d", got)
	}
	if got := PickExcellentOption(rows, 14*512, fixedRnd(0)); got != 0 {
		t.Fatalf("potion = %d", got)
	}
}

func TestItemDropTables(t *testing.T) {
	drops, err := LoadItemDrop(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(drops) != 125 {
		t.Fatalf("%d ItemDrop rows", len(drops))
	}
	j := drops[0] // 12,015 Jewel of Chaos
	if j.Index != 12*512+15 || j.Option != [6]int{-1, -1, -1, -1, -1, -1} || j.MinLevel != 12 || j.MaxLevel != 150 ||
		j.Rate != [4]int{500, 940, 360, 200} {
		t.Fatalf("jewel row = %+v", j)
	}
	// "N" reads as the previous number (Grade 0): ItemOptionRate row 0.
	h := drops[5]
	if h.Index != 7*512 || h.Option != [6]int{} || h.Rate[0] != 4200 {
		t.Fatalf("helm row = %+v", h)
	}
	money, err := LoadItemMoneyDrop(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(money) != 1 || money[0].Amount != -1 || money[0].Rate != 1000 || money[0].Map != -1 {
		t.Fatalf("money = %+v", money)
	}
	guard, err := LoadCustomGuard(dataDir())
	if err != nil || len(guard) != 1 || guard[0] != 835 {
		t.Fatalf("guard = %v %v", guard, err)
	}
	cm, err := LoadCustomMonsterRows(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := FindCustomMonster(cm, 293, 10); !ok || r.MaxLife != 300 || r.SpecialBag != -1 {
		t.Fatalf("custom monster = %+v", r)
	}
	if _, ok := FindCustomMonster(cm, 293, 0); ok {
		t.Fatal("map ignored")
	}
}

func TestEventItemBags(t *testing.T) {
	bags, err := LoadEventItemBags(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(bags) != 70 {
		t.Fatalf("%d bags", len(bags))
	}
	var goblin *BagEntry
	for i := range bags {
		if bags[i].MonsterClass == 78 {
			goblin = &bags[i]
		}
	}
	if goblin == nil || goblin.Bag == nil {
		t.Fatal("no Golden Goblin bag")
	}
	b := goblin.Bag
	if b.ItemRate != 100 || len(b.Bags) != 1 || b.Bags[0].DropRate != 10000 || len(b.Bags[0].Drops) != 1 {
		t.Fatalf("goblin = %+v", b)
	}
	d := b.Bags[0].Drops[0]
	if d.Section != 5 || d.MinCount != 1 || d.MaxCount != 1 || d.Class != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("goblin drop = %+v", d)
	}
	if it := b.Sections[5]; len(it) != 1 || it[0].Index != 14*512+11 || it[0].Level != 8 {
		t.Fatalf("goblin items = %+v", it)
	}
	// Box of Kundun 3: "0,  8" item references.
	for _, e := range bags {
		if e.ItemIndex == 7179 && e.ItemLevel == 10 {
			if it := e.Bag.Sections[5]; len(it) == 0 || it[0].Index != 8 || it[0].Option[1] != 13 {
				t.Fatalf("kundun 3 items = %+v", it[:1])
			}
		}
	}
}

func TestSetItemTables(t *testing.T) {
	types, err := LoadSetItemType(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	r, ok := types.Get(0)
	if !ok || r.StatType != 0 || r.Option != [2]int{2, 0} || r.DropRate != 10000 {
		t.Fatalf("0,0 = %+v", r)
	}
	if types.SetOptionIndex(0, 5) != 2 || types.SetOptionIndex(0, 6) != 0 || types.SetOptionIndex(0, 0) != -1 {
		t.Fatal("set option index")
	}
	if len(types.Order) != len(types.ByIndex) || types.Order[0] != 0 {
		t.Fatal("order")
	}
	opts, err := LoadSetItemOption(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	w := opts[1]
	if w.Index != 1 || w.Name != "Warrior" || w.Tier[0][0] != (SetOptionPair{0, 10}) || w.Tier[0][1].Code != -1 ||
		w.Tier[1][0] != (SetOptionPair{19, 10}) || w.Full[0] != (SetOptionPair{15, 5}) || w.Full[3].Code != -1 {
		t.Fatalf("warrior = %+v", w)
	}
	if w.TierCount() != 6 || opts[3].TierCount() != 2 {
		t.Fatalf("tiers %d %d", w.TierCount(), opts[3].TierCount())
	}
	if opts[0].Index != -1 {
		t.Fatal("empty slot")
	}
}
