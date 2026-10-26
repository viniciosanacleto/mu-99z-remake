package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomArena(t *testing.T) {
	a, err := LoadCustomArena(eventData)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Schedule[0]) != 1 || a.Schedule[0][0] != (Schedule{-1, -1, -1, -1, 20, 0, 0}) ||
		a.Schedule[5][0] != (Schedule{-1, -1, -1, -1, 21, 15, 0}) || len(a.Schedule[6]) != 0 {
		t.Fatalf("schedule %+v", a.Schedule[:7])
	}
	r := a.Rules[1]
	if !r.Loaded || r.Name != "[BKxBK]" || r.AlarmTime != 5 || r.StandTime != 1 || r.EventTime != 10 || r.CloseTime != 4 ||
		r.StartGate != 100 || r.FinalGate != 170 || r.MinUser != 5 || r.MaxUser != 100 || r.MaxDeath != -1 ||
		r.MinLevel != -1 || r.RewardBag != -1 || r.Class != [5]int{0, 1, 0, 0, 0} || r.MinMasterLevel != 0 {
		t.Fatalf("rule %+v", r)
	}
	if a.Rules[5].StartGate != 10 || a.Rules[5].Class != [5]int{1, 1, 1, 1, 1} || a.Rules[6].Loaded {
		t.Fatalf("rule 5 %+v", a.Rules[5])
	}
}

func TestLoadCustomEventDropAndLuke(t *testing.T) {
	d, err := LoadCustomEventDrop(eventData)
	if err != nil {
		t.Fatal(err)
	}
	if d.Schedule[0][0] != (Schedule{-1, -1, -1, -1, 20, 0, 0}) ||
		d.Rules[0] != (CustomEventDropRule{Name: "Custom Event Drop", Map: 0, X: 145, Y: 135, Radius: 5, AlarmTime: 5, EventTime: 3}) {
		t.Fatalf("drop %+v %+v", d.Schedule[0], d.Rules[0])
	}
	if len(d.Items[0]) != 12 || d.Items[0][0] != (CustomEventDropItem{Index: 12*512 + 12, Level: 0, Count: 3, Delay: 0}) ||
		d.Items[0][11] != (CustomEventDropItem{Index: 14*512 + 16, Count: 3, Delay: 110}) {
		t.Fatalf("items %+v", d.Items[0])
	}
	h, err := LoadCustomLukeHelper(eventData)
	if err != nil {
		t.Fatal(err)
	}
	if h.EventTime != 5 || len(h.Spots) != 3 || h.Spots[1] != (LukeSpot{Key: 1, Map: 2, Class: 258, StartX: 1, StartY: 1,
		EndX: 255, EndY: 255, Bag: 21}) || len(h.Schedule) != 2 || h.Schedule[1] != (Schedule{-1, -1, -1, -1, 18, 0, 0}) {
		t.Fatalf("luke %+v", h)
	}
}

func TestLoadSmallCustoms(t *testing.T) {
	hb, err := LoadCustomHealthBar(eventData)
	if err != nil || len(hb) != 12 {
		t.Fatalf("health bar %+v %v", hb, err)
	}
	if HealthBarSwitch(hb, 100, 3) != 0 || HealthBarSwitch(hb, 278, 30) != 0 || HealthBarSwitch(hb, 3, 0) != 1 ||
		HealthBarSwitch(nil, 3, 0) != 0 {
		t.Fatal("health bar switch")
	}
	kc, err := LoadCustomMonsterKillCount(eventData)
	if err != nil || len(kc) != 0 {
		t.Fatalf("kill count %+v %v", kc, err)
	}
	ss, err := LoadSummonScroll(eventData)
	if err != nil || len(ss.Scrolls) != 1 || ss.Scrolls[0] != (SummonScrollItem{0, 14*512 + 213}) || len(ss.Monsters) != 6 ||
		ss.Monsters[3] != (SummonScrollMonster{Index: 0, Group: 3, Class: 79, Rate: 40}) {
		t.Fatalf("summon %+v %v", ss, err)
	}
	if v := CreationGoldValue(eventData); v != 20 {
		t.Fatalf("creation gold %d", v)
	}
	if v := CreationGoldValue(t.TempDir()); v != 50 {
		t.Fatalf("creation gold default %d", v)
	}
	dr, err := LoadDailyReward(eventData)
	if err != nil || len(dr) != 31 {
		t.Fatalf("daily %d %v", len(dr), err)
	}
	if r := dr[31]; r != (DailyRewardRow{Day: 31, Index: 512, Level: 15, Skill: 1, Luck: 1, Option: 7, Durability: 0,
		Excellent: 63, Info1: "Small Axe", Info2: "Excelente +13 Full"}) {
		t.Fatalf("day 31 %+v", r)
	}
	p, err := LoadCustomPatente(eventData)
	if err != nil || p != nil {
		t.Fatalf("patente %v %v", p, err)
	}
}

func TestLoadCustomFixtures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.MkdirAll(filepath.Join(dir, "Custom"), 0o755); err != nil {
			t.Fatal(err)
		}
		putOriginal(t, filepath.Join(dir, "Custom", name), text)
	}
	write("CustomMonsterKillCount.txt", "0\n5 3 1 0 1 1 10 100 \"Spiders\"\n2 4 1 1 1 1 2 0 \"Two\"\n5 9 1 1 1 1 1 1 \"dup\"\nend\n"+
		"1\n5 12,015 1 0 0 1 2 3 4 60\n7 12,015 0 0 0 0 0 0 0 0\nend\n")
	kc, err := LoadCustomMonsterKillCount(dir)
	if err != nil || len(kc) != 2 || kc[0].DBIndex != 2 || kc[1].Monster != 3 || kc[1].Message != "Spiders" ||
		kc[1].AL != [4]int{1, 0, 1, 1} || len(kc[1].Items) != 1 ||
		kc[1].Items[0] != (KillCountItem{Index: 12*512 + 15, Level: 1, Luck: 1, Option: 2, Excellent: 3, SetOption: 4, Duration: 60}) {
		t.Fatalf("kill count %+v %v", kc, err)
	}
	write("CustomPatente.txt", "0\n7 ; first\n-3\n12x\nend\n")
	p, err := LoadCustomPatente(dir)
	if err != nil || len(p) != 3 || p[0] != 7 || p[1] != -3 || p[2] != 12 {
		t.Fatalf("patente %v %v", p, err)
	}
}
