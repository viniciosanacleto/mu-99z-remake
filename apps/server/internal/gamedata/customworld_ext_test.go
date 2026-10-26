package gamedata

import (
	"testing"
)

func TestLoadCustomArena(t *testing.T) {
	a, err := LoadCustomArena(configDir())
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
	d, err := LoadCustomEventDrop(configDir())
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
	h, err := LoadCustomLukeHelper(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if h.EventTime != 5 || len(h.Spots) != 3 || h.Spots[1] != (LukeSpot{Key: 1, Map: 2, Class: 258, StartX: 1, StartY: 1,
		EndX: 255, EndY: 255, Bag: 21}) || len(h.Schedule) != 2 || h.Schedule[1] != (Schedule{-1, -1, -1, -1, 18, 0, 0}) {
		t.Fatalf("luke %+v", h)
	}
}

func TestLoadSmallCustoms(t *testing.T) {
	hb, err := LoadCustomHealthBar(configDir())
	if err != nil || len(hb) != 12 {
		t.Fatalf("health bar %+v %v", hb, err)
	}
	if HealthBarSwitch(hb, 100, 3) != 0 || HealthBarSwitch(hb, 278, 30) != 0 || HealthBarSwitch(hb, 3, 0) != 1 ||
		HealthBarSwitch(nil, 3, 0) != 0 {
		t.Fatal("health bar switch")
	}
	kc, err := LoadCustomMonsterKillCount(configDir())
	if err != nil || len(kc) != 0 {
		t.Fatalf("kill count %+v %v", kc, err)
	}
	ss, err := LoadSummonScroll(configDir())
	if err != nil || len(ss.Scrolls) != 1 || ss.Scrolls[0] != (SummonScrollItem{0, 14*512 + 213}) || len(ss.Monsters) != 6 ||
		ss.Monsters[3] != (SummonScrollMonster{Index: 0, Group: 3, Class: 79, Rate: 40}) {
		t.Fatalf("summon %+v %v", ss, err)
	}
	if v := CreationGoldValue(configDir()); v != 20 {
		t.Fatalf("creation gold %d", v)
	}
	if v := CreationGoldValue(t.TempDir()); v != 50 {
		t.Fatalf("creation gold default %d", v)
	}
	dr, err := LoadDailyReward(configDir())
	if err != nil || len(dr) != 31 {
		t.Fatalf("daily %d %v", len(dr), err)
	}
	if r := dr[31]; r != (DailyRewardRow{Day: 31, Index: 512, Level: 15, Skill: 1, Luck: 1, Option: 7, Durability: 0,
		Excellent: 63, Info1: "Small Axe", Info2: "Excelente +13 Full"}) {
		t.Fatalf("day 31 %+v", r)
	}
	p, err := LoadCustomPatente(configDir())
	if err != nil || p != nil {
		t.Fatalf("patente %v %v", p, err)
	}
}
