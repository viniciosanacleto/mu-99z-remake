package gamedata

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// writeData writes files (relative path -> content) under a temp data dir.
func writeData(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadDefaultClassFreebiesOriginal(t *testing.T) {
	f, err := LoadDefaultClassFreebies(dataDir())
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

func TestLoadDefaultClassFreebiesRows(t *testing.T) {
	dir := writeData(t, map[string]string{"Character/DefaultClassFreebies.txt": `0
1 500 10
end
1
0 12,015 3 255 1 1 2 63 0 60 // kept
7 12,015 0 0 0 0 0 0 0 0      // class 7 dropped
end
2
4 29 1 10 20 30 40 900
end`})
	f, err := LoadDefaultClassFreebies(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Money[1] != 500 || f.Points[1] != 10 {
		t.Errorf("money/points %v %v", f.Money, f.Points)
	}
	want := []FreebieItem{{Class: 0, Index: 12*512 + 15, Level: 3, Durability: 255, Skill: 1, Luck: 1, Option: 2,
		Excellent: 63, SetOption: 0, Duration: 60}}
	if !reflect.DeepEqual(f.Items, want) {
		t.Errorf("items = %+v", f.Items)
	}
	if len(f.Effects) != 1 || f.Effects[0] != (FreebieEffect{4, 29, 1, [4]int{10, 20, 30, 40}, 900}) {
		t.Errorf("effects = %+v", f.Effects)
	}
}

func TestLoadCommandRequirement(t *testing.T) {
	rows, err := LoadCommandRequirement(dataDir())
	if err != nil || len(rows) != 0 {
		t.Fatalf("original: %v %v (the table is empty)", rows, err)
	}
	// "*" as ReqItemIndex is a single token; an item reference is two.
	dir := writeData(t, map[string]string{"CommandRequirement.txt": `
1 * 10 * * * 0 * 1 0 * * * * * * 0 1 1 255 255 * * * 50 1 1 1 1 1
2 3 * * * * 1 14,013 5 0 * * * * * * * * * * * 6 18 22 51 1 0 0 0 0
end`})
	rows, err = LoadCommandRequirement(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	r := rows[0]
	if r.Index != 1 || r.AccountLevel != -1 || r.MinLevel != 10 || r.ReqItem != -1 || r.ReqItemCount != 1 ||
		r.EndY != 255 || r.FailMessage != 50 || r.Class != [5]int{1, 1, 1, 1, 1} {
		t.Errorf("row 0 = %+v", r)
	}
	r = rows[1]
	if r.ReqItem != 14*512+13 || r.ReqItemCount != 5 || r.DayOfWeek != 6 || r.MinHour != 18 || r.MaxHour != 22 ||
		r.FailMessage != 51 || r.Class != [5]int{1, 0, 0, 0, 0} {
		t.Errorf("row 1 = %+v", r)
	}
}

func TestLoadCustomEvolution(t *testing.T) {
	m, err := LoadCustomEvolution(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 || m[1] != (EvolutionRow{Index: 1, Target: 0, Result: 1}) {
		t.Fatalf("evolution = %+v", m)
	}
}

func TestLoadCustomMobComboAndGuard(t *testing.T) {
	c, err := LoadCustomMobCombo(dataDir())
	if err != nil || !reflect.DeepEqual(c, []int{500}) {
		t.Fatalf("mob combo = %v %v", c, err)
	}
	if _, err := LoadCustomGuard(dataDir()); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCustomGHRS(t *testing.T) {
	g, err := LoadCustomGHRS(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if g.Week[time.Monday] != 20 || g.Week[time.Friday] != 20 || g.Week[time.Saturday] != 25 ||
		g.Week[time.Sunday] != 25 || g.Hour != 0 || g.Min != 0 || g.Sec != 0 {
		t.Fatalf("ghrs = %+v", g)
	}
	dir := writeData(t, map[string]string{"Custom/CustomGHRS.ini": "[WEEK]\nSunday=3\n[TIMEUPDATE]\nHour=23\nSec=9\n"})
	if g, err = LoadCustomGHRS(dir); err != nil {
		t.Fatal(err)
	}
	if g.Week != [7]int{3} || g.Hour != 23 || g.Min != 0 || g.Sec != 9 {
		t.Fatalf("ghrs = %+v (missing keys read 0)", g)
	}
}

func TestLoadItemLevel(t *testing.T) {
	rows, err := LoadItemLevel(dataDir())
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
	rows, err := LoadMonsterMove(dataDir())
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
	m, err := LoadMonsterSchedule(dataDir())
	if err != nil || len(m) != 0 {
		t.Fatalf("original = %v %v (all rows are commented)", m, err)
	}
	dir := writeData(t, map[string]string{"Monster/MonsterSchedule.txt": `0
0 * * * * 20 0 0
end
1
0 * * * 600
end
2
0 229 0 137 087
0 229 2 198 047
end`})
	if m, err = LoadMonsterSchedule(dir); err != nil {
		t.Fatal(err)
	}
	g := m[0]
	if g == nil || len(g.Schedule) != 1 || g.Schedule[0] != (Schedule{-1, -1, -1, -1, 20, 0, 0}) ||
		g.EnableMsg != -1 || g.NPCSec != 600 || len(g.Monsters) != 2 || g.Monsters[1] != (ScheduledMonster{229, 2, 198, 47}) {
		t.Fatalf("group 0 = %+v", g)
	}
}

func TestLoadMoveSummon(t *testing.T) {
	rows, err := LoadMoveSummon(dataDir())
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
	rows, err := LoadHackSkillSpeedCheck(dataDir())
	if err != nil || len(rows) != 0 {
		t.Fatalf("original = %v %v", rows, err)
	}
	dir := writeData(t, map[string]string{"Hack/HackSkillSpeedCheck.txt": "16 * * * * * 0 50 300 1 5 2\nend"})
	if rows, err = LoadHackSkillSpeedCheck(dir); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != (SkillSpeedRule{16, -1, -1, -1, -1, -1, 0, 50, 300, 1, 5, 2}) {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestExperienceTable(t *testing.T) {
	tbl, err := LoadExperienceTable(dataDir())
	if err != nil || len(tbl) != 0 {
		t.Fatalf("original = %v %v", tbl, err)
	}
	if tbl.Rate(400, 10, 0) != 100 {
		t.Fatal("empty table must give 100")
	}
	// Seven numbers per record: a fourth-column-style row with the header's
	// four AL rates would be cut into further records, like FUN_0047be80.
	dir := writeData(t, map[string]string{"Util/ExperienceTable.txt": `1 100 * * * * 300
101 * 5 10 * * 50
* * * * * * 20
end`})
	if tbl, err = LoadExperienceTable(dir); err != nil {
		t.Fatal(err)
	}
	if len(tbl) != 3 {
		t.Fatalf("%d records", len(tbl))
	}
	cases := []struct{ lv, rs, mr, want int }{
		{50, 0, 0, 300}, {150, 7, 0, 50}, {150, 11, 0, 20}, {150, 4, 3, 20},
	}
	for _, c := range cases {
		if got := tbl.Rate(c.lv, c.rs, c.mr); got != c.want {
			t.Errorf("Rate(%d,%d,%d) = %d, want %d", c.lv, c.rs, c.mr, got, c.want)
		}
	}
	dir = writeData(t, map[string]string{"Util/ExperienceTable.txt": "1 100 * * * * 300 301 302 303\nend"})
	if tbl, err = LoadExperienceTable(dir); err != nil {
		t.Fatal(err)
	}
	if len(tbl) != 2 || tbl[1].MinLevel != 301 {
		t.Fatalf("stream = %+v", tbl)
	}
}

func TestLoadPetDamageTable(t *testing.T) {
	rows, err := LoadPetDamageTable(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 13 || rows[0] != (PetDamageRow{0, -1, -1, -1, -1, 1000}) || rows[12].Pet != 12 {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestLoadBlackList(t *testing.T) {
	b, err := LoadBlackList(dataDir())
	if err != nil || len(b.IPs) != 0 || len(b.HWIDs) != 0 {
		t.Fatalf("original = %+v %v", b, err)
	}
	dir := writeData(t, map[string]string{"BlackList.txt": `0
"10.0.0.1"
end
1
"ABCDEF"
end`})
	if b, err = LoadBlackList(dir); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(b.IPs, []string{"10.0.0.1"}) || !reflect.DeepEqual(b.HWIDs, []string{"ABCDEF"}) {
		t.Fatalf("blacklist = %+v", b)
	}
}
