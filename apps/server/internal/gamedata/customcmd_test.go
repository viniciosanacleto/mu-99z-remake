package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCustom(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	putOriginal(t, filepath.Join(dir, "Custom", name), body)
	return dir
}

func TestLoadCustomAttackOriginal(t *testing.T) {
	a, err := LoadCustomAttack("../../config/data")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Speeds) != 1 || a.Speeds[0] != (AttackSpeedRow{-1, -1, -1, -1, -1, -1, 1500, 4}) {
		t.Fatalf("speeds %+v", a.Speeds)
	}
	if len(a.Skills) != 40 {
		t.Fatalf("%d skills", len(a.Skills))
	}
	if s := a.Skills[16]; s.Group != 0 || s.Value != 1 || s.Class != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("mana shield %+v", s)
	}
	if s := a.Skills[41]; s.Group != 3 || s.Value != 1 {
		t.Fatalf("twisting slash %+v", s)
	}
	if a.Order[0] != 1 || a.Order[len(a.Order)-1] != 66 {
		t.Fatalf("order %v", a.Order)
	}
	// Formula of the original file: 1500 - 4 * speed.
	if d := a.AttackDelay(19, 100, 250); d != 500 {
		t.Fatalf("delay %d", d)
	}
}

func TestCustomAttackDelayRows(t *testing.T) {
	dir := writeCustom(t, "CustomAttack.txt", `0
19 100 * * * 700 0 0
* * * 10 20 -1 1000 10
end
1
16 0 1 1 1 1 1 1
16 1 0 0 0 0 0 0
300 1 0 1 1 1 1 1
end
`)
	a, err := LoadCustomAttack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Skills[16].Group != 0 || len(a.Skills) != 1 {
		t.Fatalf("first row must win and index 300 is dropped: %+v", a.Skills)
	}
	cases := []struct{ skill, dex, speed, want int }{
		{19, 150, 5, 700},             // first row
		{19, 50, 15, 1000 - 10*15},    // dexterity below row 1, formula row
		{20, 0, 30, (0x177 - 30) * 4}, // no row
		{-5, 0, 10, 1000 - 10*10},     // skill "*"
	}
	for _, c := range cases {
		if g := a.AttackDelay(c.skill, c.dex, c.speed); g != c.want {
			t.Fatalf("%+v: %d", c, g)
		}
	}
	var none *CustomAttack
	if none.AttackDelay(1, 1, 75) != 1200 {
		t.Fatal("nil table")
	}
}

func TestLoadCustomGift(t *testing.T) {
	dir := writeCustom(t, "CustomGift.txt", `0
0 * 1 1 1 1 * * * * 1 10 500 "beginner"
1 2 1 0 1 1 5 * * 3 * 0 0 "elf"
end
1
0 12,015 0 0 0 0 0 0 0 0
0 14,013 1 3 0 1 2 0 0 3600
1 13,000 0 0 1 0 0 63 0 0
end
`)
	g, err := LoadCustomGift(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 2 || g[0].Code != "beginner" || g[0].MaxCount != 1 || g[0].LevelUpPoint != 10 || g[0].Money != 500 {
		t.Fatalf("%+v", g)
	}
	if g[1].Class != 2 || g[1].Switch != [4]int{1, 0, 1, 1} || g[1].MinLevel != 5 || g[1].MaxReset != 3 || g[1].MaxCount != -1 {
		t.Fatalf("%+v", g[1])
	}
	if len(g[0].Items) != 2 || g[0].Items[1] != (GiftItem{Index: 14*512 + 13, Level: 1, Durability: 3, Luck: 1, Option: 2, Duration: 3600}) {
		t.Fatalf("items %+v", g[0].Items)
	}
	if g[1].Items[0].Index != 13*512 || g[1].Items[0].Skill != 1 || g[1].Items[0].Excellent != 63 {
		t.Fatalf("items %+v", g[1].Items)
	}
	// The original file has only the commented demonstration rows.
	orig, err := LoadCustomGift("../../config/data")
	if err != nil || len(orig) != 0 {
		t.Fatalf("original %v %v", orig, err)
	}
}

func TestLoadCustomMarriageOriginal(t *testing.T) {
	m, err := LoadCustomMarriage("../../config/data")
	if err != nil {
		t.Fatal(err)
	}
	want := MarriageItem{Enable: 1, Index: 12*512 + 15, Level: 0, Count: 1}
	if m.Marry != want || m.Divorce != want {
		t.Fatalf("%+v", m)
	}
	if len(m.Rewards) != 1 || m.Rewards[0].Index != 13*512+42 {
		t.Fatalf("rewards %+v", m.Rewards)
	}
}

func TestLoadCustomBuyVip(t *testing.T) {
	dir := writeCustom(t, "CustomBuyVip.txt", `0 1 31 1000 "month"
* 3 7 0 "week"
2 3 1 0 "month"
end
`)
	p, err := LoadCustomBuyVip(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || p["month"] != (VipPlan{0, 1, 31, 1000, "month"}) || p["week"].AccountLevel != -1 {
		t.Fatalf("%+v", p)
	}
	if orig, err := LoadCustomBuyVip("../../config/data"); err != nil || len(orig) != 0 {
		t.Fatalf("original %v %v", orig, err)
	}
}

func TestLoadCustomPickOriginal(t *testing.T) {
	p, err := LoadCustomPick("../../config/data")
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || p[0] != (PickBlock{Index: 14*512 + 22, Level: -1, AccountLevel: -1}) {
		t.Fatalf("%+v", p)
	}
}
