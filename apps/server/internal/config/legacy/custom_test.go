package legacy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestCustomAttackDelayRows(t *testing.T) {
	dir := convertGroup(t, "custom", map[string]string{"Custom/" + "CustomAttack.txt": `0
19 100 * * * 700 0 0
* * * 10 20 -1 1000 10
end
1
16 0 1 1 1 1 1 1
16 1 0 0 0 0 0 0
300 1 0 1 1 1 1 1
end
`})
	a, err := gamedata.LoadCustomAttack(dir)
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
	var none *gamedata.CustomAttack
	if none.AttackDelay(1, 1, 75) != 1200 {
		t.Fatal("nil table")
	}
}

func TestLoadCustomJewelSections(t *testing.T) {
	dir := convertGroup(t, "custom", map[string]string{"Custom/" + "CustomJewel.txt": `0
0   14,200  1  0  1   60 61 62 63   50000000
3   14 201  0  1  0   10 20 30 40   7 // two tokens for the item
60  14,202  1  1  1   1 1 1 1 1
end
1
0   0  12  0  *  *  6  *  15  1
end
2
0   1  0  1  2  1  5
end
3
0   2  1  0  1  6  1
end
`})
	c, err := gamedata.LoadCustomJewel(dir)
	if err != nil {
		t.Fatal(err)
	}
	j, ok := c.Find(14*512 + 200)
	if !ok || j.Index != 0 || j.Weapon != 1 || j.Armor != 0 || j.Wing != 1 || j.Rate != [4]int{60, 61, 62, 63} ||
		j.SalePrice != 50000000 {
		t.Fatalf("slot 0 %+v", j)
	}
	if j.MinLevel != 0 || j.MaxLevel != 12 || j.Skill != 0 || j.Luck != -1 || j.MinOption != -1 ||
		j.MaxOption != 6 || j.MinExc != -1 || j.MaxExc != 15 || j.MaxSetOption != 1 {
		t.Fatalf("requirements %+v", j)
	}
	if j.Success != (gamedata.JewelChange{Level: 1, Skill: 0, Luck: 1, Option: 2, Excellent: 1, SetOption: 5}) || j.Failure != (gamedata.JewelChange{Level: 2, Skill: 1, Luck: 0, Option: 1, Excellent: 6, SetOption: 1}) {
		t.Fatalf("changes %+v %+v", j.Success, j.Failure)
	}
	if j, ok := c.Find(14*512 + 201); !ok || j.Index != 3 || j.SalePrice != 7 {
		t.Fatalf("slot 3 %+v", j)
	}
	// Index 60 is beyond the 50 slots.
	if _, ok := c.Find(14*512 + 202); ok {
		t.Fatal("index 60 loaded")
	}
}

func TestLoadLuckyWheelColumns(t *testing.T) {
	dir := convertGroup(t, "custom", map[string]string{"Custom/" + "CustomLuckyWheel.txt": "0 12 3 4 1 0 5 32\n0 1 1 1 1 1 1 1\nend\n"})
	w, err := gamedata.LoadLuckyWheel(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Skill is the fifth column and Luck the sixth; the first row wins.
	if r := w[0]; r != (gamedata.WheelReward{Index: 0, Type: 12, Item: 3, Level: 4, Skill: 1, Luck: 0, Option: 5, Excellent: 32}) {
		t.Fatalf("row %+v", r)
	}
}

func TestCustomGiftAndVip(t *testing.T) {
	dir := convertGroup(t, "custom", map[string]string{
		"Custom/CustomGift.txt": `1
9 12,015 0 0 0 0 0 0 0 0 // gift 9 is not read yet: dropped
end
0
0 * 1 1 1 1 * * * * 1 10 500 "beginner"
1 2 1 0 1 1 5 * * 3 * 0 0 "elf"
end
1
0 12,015 0 0 0 0 0 0 0 0
0 14,013 1 3 0 1 2 0 0 3600
1 13,000 0 0 1 0 0 63 0 0
end
`,
		"Custom/CustomBuyVip.txt": "0 1 31 1000 \"month\"\n* 3 7 0 \"week\"\n2 3 1 0 \"month\"\nend\n",
	})
	g, err := gamedata.LoadCustomGift(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 2 || g[0].Code != "beginner" || g[0].MaxCount != 1 || g[0].LevelUpPoint != 10 || g[0].Money != 500 {
		t.Fatalf("%+v", g)
	}
	if g[1].Class != 2 || g[1].Switch != [4]int{1, 0, 1, 1} || g[1].MinLevel != 5 || g[1].MaxReset != 3 || g[1].MaxCount != -1 {
		t.Fatalf("%+v", g[1])
	}
	if len(g[0].Items) != 2 || g[0].Items[1] != (gamedata.GiftItem{Index: 14*512 + 13, Level: 1, Durability: 3, Luck: 1, Option: 2, Duration: 3600}) {
		t.Fatalf("items %+v", g[0].Items)
	}
	if g[1].Items[0].Index != 13*512 || g[1].Items[0].Skill != 1 || g[1].Items[0].Excellent != 63 {
		t.Fatalf("items %+v", g[1].Items)
	}
	p, err := gamedata.LoadCustomBuyVip(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 2 || p["month"] != (gamedata.VipPlan{NewAccountLevel: 1, Days: 31, Money: 1000, Code: "month"}) || p["week"].AccountLevel != -1 {
		t.Fatalf("%+v", p)
	}
}

func TestCustomKillCountPatenteAndProfiles(t *testing.T) {
	dir := convertGroup(t, "custom", map[string]string{
		"Custom/CustomMonsterKillCount.txt": "0\n5 3 1 0 1 1 10 100 \"Spiders\"\n2 4 1 1 1 1 2 0 \"Two\"\n5 9 1 1 1 1 1 1 \"dup\"\nend\n" +
			"1\n5 12,015 1 0 0 1 2 3 4 60\n7 12,015 0 0 0 0 0 0 0 0\nend\n",
		"Custom/CustomPatente.txt": "0\n7 ; first\n-3\n12x\nend\n",
		"Custom/CustomGHRS.ini":    "[WEEK]\nSunday=3\n[TIMEUPDATE]\nHour=23\nSec=9\n",
		"Custom/CreationGold.txt":  "; comment\n[CreationGold]\nGoldValue=20\n",
		"Custom/CustomWing.txt":    "10 12,150 4 139 2 61 2 80 4 85 1 83 4 110 5 111 5 109 5 108 5 3 \"alaevobkv2\"\n11 12,151 4 139 2 61 2 80 4 85 1 83 4 110 5 111 5 109 5 108 5\nend\n",
	})
	kc, err := gamedata.LoadCustomMonsterKillCount(dir)
	if err != nil || len(kc) != 2 || kc[0].DBIndex != 2 || kc[1].Monster != 3 || kc[1].Message != "Spiders" ||
		kc[1].AL != [4]int{1, 0, 1, 1} || len(kc[1].Items) != 1 ||
		kc[1].Items[0] != (gamedata.KillCountItem{Index: 12*512 + 15, Level: 1, Luck: 1, Option: 2, Excellent: 3, SetOption: 4, Duration: 60}) {
		t.Fatalf("kill count %+v %v", kc, err)
	}
	p, err := gamedata.LoadCustomPatente(dir)
	if err != nil || len(p) != 3 || p[0] != 7 || p[1] != -3 || p[2] != 12 {
		t.Fatalf("patente %v %v", p, err)
	}
	g, err := gamedata.LoadCustomGHRS(dir)
	if err != nil || g.Week[time.Sunday] != 3 || g.Week[time.Monday] != 0 || g.Hour != 23 || g.Min != 0 || g.Sec != 9 {
		t.Fatalf("ghrs = %+v %v (missing keys read 0)", g, err)
	}
	if v := gamedata.CreationGoldValue(dir); v != 20 {
		t.Fatalf("creation gold %d", v)
	}
	wings, err := gamedata.LoadCustomWings(dir)
	if err != nil || wings[12*512+150].ModelType != 3 || wings[12*512+151].ModelType != 0 || wings[12*512+150].Option[0] != [2]int{80, 4} {
		t.Fatalf("wings %+v %v", wings, err)
	}
}

func TestCustomEventGroup(t *testing.T) {
	// A missing cell reads as -1 (ints) or 0xFF (bytes); ';' and '//'
	// start comments.
	cfg := convertGroup(t, "custom-event", map[string]string{
		"Custom/Event/SobreEvent.txt":  "; comment\n0\n 1 \"/x y\" // tail\nend\n",
		"Custom/Event/DuelEvent.txt":   "0\n1 \"/duel\" 1\n200 40 22 14 6 71 11\nend\n1\n0 20 30 5 2\nend\n",
		"Custom/Event/RaffleEvent.txt": "0\n1\nend\n1\n0 23 59\nend\n",
	})
	so, err := gamedata.LoadSobreEvent(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !so.Active || so.Syntax != "/x y" || so.Patente || so.Level != -1 || so.Place != (gamedata.EventPlace{Map: 0xFF, X: 0xFF, Y: 0xFF}) ||
		so.MinPlayers != 0xFF || so.Starts != nil {
		t.Fatalf("defaults %+v", so)
	}
	du, err := gamedata.LoadDuelEvent(cfg)
	if err != nil || !du.Patente || du.Fight != (gamedata.EventPlace{Map: 6, X: 71, Y: 11}) ||
		du.Starts[0] != (gamedata.EventStart{Day: 0, Hour: 20, Minute: 30, Time: 5, Extra: 2}) {
		t.Fatalf("duel %+v %v", du, err)
	}
	ra, err := gamedata.LoadRaffleEvent(cfg)
	if err != nil || !ra.Active || ra.Starts[0] != (gamedata.EventStart{Day: 0, Hour: 23, Minute: 59, Extra: -1}) {
		t.Fatalf("raffle %+v %v", ra, err)
	}
	if _, err := LoadEventScript(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("missing file read")
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	os.WriteFile(bad, []byte("0\n1\n"), 0o600)
	if _, err := LoadEventScript(bad); err == nil {
		t.Fatal("section without end accepted")
	}
}

func TestXShopGroup(t *testing.T) {
	cfg := convertGroup(t, "xshop", map[string]string{
		"Custom/CustomXShop.txt":         "0\n3 -5\n16 9\nend\n1\n3 2 -1\n0 600 7 // Overflow\nend\n",
		"Custom/CustomXShopRecharge.txt": "; c\n[Recarga]\nAtivo=1\nTitular=Jo\xe3o\nImagemBMP=pix.bmp\n",
	})
	path := filepath.Join(cfg, "data", "xshop.toml")
	x, ok, err := gamedata.LoadXShopPrices(path)
	if err != nil || !ok || x.Category[3] != 0 || x.Price(3*512+2) != 0 {
		t.Fatalf("negative prices %+v %v %v", x, ok, err)
	}
	// 0,600 is 600 = 1,88: the original adds Type*512 + Index without
	// checking Index.
	if x.Price(600) != 7 {
		t.Fatalf("overflowing index %d", x.Price(600))
	}
	f, err := gamedata.LoadXShopConfig(path)
	if err != nil || f.CustomXShopRecharge.Ativo != 1 || f.CustomXShopRecharge.Titular != "Jo\xe3o" || f.CustomXShopRecharge.ImagemBMP != "pix.bmp" {
		t.Fatalf("recharge %+v %v", f, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `Titular = "João"`) {
		t.Fatalf("file:\n%s", b)
	}
}
