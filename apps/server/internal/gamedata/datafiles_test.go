package gamedata

import (
	"reflect"
	"testing"
	"time"
)

func TestLoadDefaultClassFreebiesOriginal(t *testing.T) {
	f, err := LoadDefaultClassFreebies(configDir())
	if err != nil {
		t.Fatal(err)
	}
	for c := 0; c < 5; c++ {
		if f.Money[c] != 0 || f.Points[c] != 3000 {
			t.Errorf("class %d: money %d points %d", c, f.Money[c], f.Points[c])
		}
	}
	if len(f.Items) != 0 || len(f.Effects) != 0 {
		t.Errorf("items %v effects %v, want none (rows are commented)", f.Items, f.Effects)
	}
}

func TestLoadCommandRequirement(t *testing.T) {
	rows, err := LoadCommandRequirement(configDir())
	if err != nil || len(rows) != 0 {
		t.Fatalf("original: %v %v (the table is empty)", rows, err)
	}
}

func TestLoadCustomEvolution(t *testing.T) {
	m, err := LoadCustomEvolution(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[1] != (EvolutionRow{Index: 1, Target: 0, Result: 1}) {
		t.Fatalf("evolution = %+v", m)
	}
}

func TestLoadCustomMobComboAndGuard(t *testing.T) {
	c, err := LoadCustomMobCombo(configDir())
	if err != nil || !reflect.DeepEqual(c, []int{500}) {
		t.Fatalf("mob combo = %v %v", c, err)
	}
	if _, err := LoadCustomGuard(configDir()); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCustomGHRS(t *testing.T) {
	g, err := LoadCustomGHRS(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if g.Week[time.Monday] != 20 || g.Week[time.Friday] != 20 || g.Week[time.Saturday] != 25 ||
		g.Week[time.Sunday] != 25 || g.Hour != 0 || g.Min != 0 || g.Sec != 0 {
		t.Fatalf("ghrs = %+v", g)
	}
}

func TestLoadItemLevel(t *testing.T) {
	rows, err := LoadItemLevel(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 20 {
		t.Fatalf("%d rows", len(rows))
	}
	if rows[0] != (ItemLevelName{12*512 + 11, 0, "Goblin Summoning Orb"}) {
		t.Errorf("first = %+v", rows[0])
	}
	if rows[7] != (ItemLevelName{13*512 + 7, 0, "Archer"}) {
		t.Errorf("row 7 = %+v", rows[7])
	}
}

func TestLoadMonsterMove(t *testing.T) {
	rows, err := LoadMonsterMove(configDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []MonsterMoveRow{{229, 0, 137, 87, 1, 1200}, {229, 2, 198, 52, 2, 1200}, {229, 3, 170, 88, 2, 1200},
		{229, 7, 17, 25, 2, 1200}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v", rows)
	}
}

func TestLoadMonsterSchedule(t *testing.T) {
	m, err := LoadMonsterSchedule(configDir())
	if err != nil || len(m) != 0 {
		t.Fatalf("original = %v %v (all rows are commented)", m, err)
	}

}

func TestLoadMoveSummon(t *testing.T) {
	rows, err := LoadMoveSummon(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 12 {
		t.Fatalf("%d rows", len(rows))
	}
	want := MoveSummonRow{Map: 0, StartX: 1, StartY: 1, EndX: 255, EndY: 255, MinLevel: 1, MaxLevel: -1,
		MinReset: -1, MaxReset: -1, AL: [4]int{1, 1, 1, 1}}
	if rows[0] != want || rows[11].Map != 34 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestLoadHackSkillSpeedCheck(t *testing.T) {
	rows, err := LoadHackSkillSpeedCheck(configDir())
	if err != nil || len(rows) != 0 {
		t.Fatalf("original = %v %v", rows, err)
	}
}

func TestExperienceTable(t *testing.T) {
	tbl, err := LoadExperienceTable(configDir())
	if err != nil || len(tbl) != 0 {
		t.Fatalf("original = %v %v", tbl, err)
	}
	if tbl.Rate(400, 10, 0) != 100 {
		t.Fatal("empty table must give 100")
	}
}

func TestLoadPetDamageTable(t *testing.T) {
	rows, err := LoadPetDamageTable(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 13 || rows[0] != (PetDamageRow{0, -1, -1, -1, -1, 1000}) || rows[12].Pet != 12 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestLoadBlackList(t *testing.T) {
	b, err := LoadBlackList(configDir())
	if err != nil || len(b.IPs) != 0 || len(b.HWIDs) != 0 {
		t.Fatalf("original = %+v %v", b, err)
	}
}
