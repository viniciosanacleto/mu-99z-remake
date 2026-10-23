package gamedata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEventFiles(t *testing.T) {
	dir := dataDir()
	ds, err := LoadDevilSquare(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ds.ReadyTime != 5 || ds.StandTime != 1 || ds.EventTime != 20 || ds.CloseTime != 4 || len(ds.Schedule) != 6 {
		t.Fatalf("devil square %+v", ds)
	}
	if s := ds.Schedule[1]; s.Year != -1 || s.Hour != 4 || s.Minute != 30 || s.Second != 0 {
		t.Fatalf("schedule %+v", s)
	}
	if ds.Experience[0][0] != 6000 || ds.Experience[5][9] != 10000 || ds.Money[3][1] != 55000 {
		t.Fatalf("rewards %v %v", ds.Experience, ds.Money)
	}
	el, err := LoadEventEntryLevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ class, level, want int }{
		{1, 14, -1}, {1, 15, 0}, {1, 130, 0}, {1, 131, 1}, {1, 400, 5}, {3, 110, 0}, {3, 111, 1}, {4, 311, 5},
	} {
		if got := el.Level(1, c.class, c.level); got != c.want {
			t.Fatalf("DS level class %d lv %d = %d want %d", c.class, c.level, got, c.want)
		}
	}
	st, err := LoadEventStageSpawn(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range st {
		if s.Event == 2 && s.Level == 0 && s.Stage == 1 && s.Monster == 5 && s.RegenTime == 1000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("stage spawn rows %v", st[:5])
	}
}

func TestLoadBloodCastle(t *testing.T) {
	bc, err := LoadBloodCastle(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if bc.ReadyTime != 5 || bc.StandTime != 1 || bc.EventTime != 15 || bc.CloseTime != 4 || len(bc.Schedule) != 12 {
		t.Fatalf("times %+v", bc)
	}
	if bc.Experience[6] != [2]int{120000, 35000} || bc.Money[2] != [2]int{100000, 50000} ||
		bc.GateLife[0] != 65000 || bc.StatueLife[6] != 565000 || bc.StatueX[3] != 14 || bc.StatueY[3] != 95 {
		t.Fatalf("tables %+v", bc)
	}
}

func TestLoadInvasions(t *testing.T) {
	inv, err := LoadInvasions(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	sk := inv[0]
	if sk == nil || len(sk.Schedule) != 4 || sk.Schedule[0].Hour != 8 || sk.Schedule[0].Minute != 5 ||
		sk.RespawnMsg != 580 || sk.DespawnMsg != -1 || sk.Boss != 55 || sk.BossKilledMsg != 590 || sk.Time != 900 ||
		sk.Name != "Skeleton King Invasion" || len(sk.Groups[0]) != 2 || len(sk.Spawns) != 4 {
		t.Fatalf("skeleton king %+v", sk)
	}
	if s := inv[2].Spawns[0]; s.Monster != 43 || s.X2 != 240 || s.Count != 5 || s.KillMessage != 1 {
		t.Fatalf("golden %+v", s)
	}
	if inv[19] != nil {
		t.Fatal("unused slot filled")
	}
}

func TestLoadBonusManager(t *testing.T) {
	ev, bonus, err := LoadBonusManager(dataDir())
	if err != nil || len(ev) != 20 || len(bonus) != 0 {
		t.Fatalf("installed file: %v %d %v", err, len(bonus), err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Event"), 0o755)
	os.WriteFile(filepath.Join(dir, "Event", "BonusManager.dat"), []byte("0\n0 * * * * 20 0 0\nend\n1\n"+
		"0 5 * 3600 \"Happy Hour\"\nend\n2\n0 0 50 60 70 80 *\n0 3 1000 1000 1000 1000 2\nend\n"), 0o644)
	ev, bonus, err = LoadBonusManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	if e := ev[0]; e == nil || len(e.Schedule) != 1 || e.StartMsg != 5 || e.FinalMsg != -1 || e.Time != 3600 || e.Name != "Happy Hour" {
		t.Fatalf("event %+v", ev[0])
	}
	if len(bonus) != 2 || bonus[0].Value[3] != 80 || bonus[0].Map != -1 || bonus[1].Index != 3 || bonus[1].Map != 2 {
		t.Fatalf("bonus %+v", bonus)
	}
}

func TestLoadChaosCastle(t *testing.T) {
	cc, err := LoadChaosCastle(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if cc.ReadyTime != 5 || cc.StandTime != 1 || cc.EventTime != 10 || cc.CloseTime != 4 || len(cc.Schedule) != 12 {
		t.Fatalf("times %+v", cc)
	}
	if s := cc.Schedule[11]; s.Hour != 23 || s.Minute != 0 || s.DayOfWeek != -1 {
		t.Fatalf("schedule %+v", s)
	}
	if cc.Experience[0] != [2]int{1000, 500} || cc.Experience[1] != [2]int{1500, 100} || cc.Experience[5] != [2]int{3500, 3000} {
		t.Fatalf("experience %v", cc.Experience)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Event"), 0o755)
	os.WriteFile(filepath.Join(dir, "Event", "ChaosCastle.dat"), []byte("2\n6 1 1\n-1 2 2\n3 7 8\nend\n"), 0o644)
	cc, err = LoadChaosCastle(dir)
	if err != nil || cc.Experience[3] != [2]int{7, 8} || cc.Experience[0] != [2]int{} {
		t.Fatalf("invalid levels: %v %v", err, cc)
	}
}

func TestLoadDropEvent(t *testing.T) {
	if _, items, err := LoadDropEvent(dataDir()); err != nil || len(items) != 0 {
		t.Fatalf("installed file: %v %d", err, len(items))
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Event"), 0o755)
	os.WriteFile(filepath.Join(dir, "Event", "DropEvent.dat"), []byte("0\n0 * * * * 21 0 0\nend\n1\n"+
		"0 * * 1800 \"Jewel Rain\"\nend\n2\n0 14,013 0 0 * * * * * * 0 2 * 10 * * 5000\nend\n"), 0o644)
	ev, items, err := LoadDropEvent(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ev[0] == nil || ev[0].Time != 1800 || len(items) != 1 {
		t.Fatalf("events %+v items %+v", ev[0], items)
	}
	if r := items[0]; r.Index != 14*512+13 || r.Map != 2 || r.Monster != -1 || r.MinLevel != 10 ||
		r.AccountLevel != -1 || r.Rate != 5000 || r.Option != [6]int{-1, -1, -1, -1, -1, -1} {
		t.Fatalf("row %+v", r)
	}
}

func TestLoadGoldenArcher(t *testing.T) {
	r, err := LoadGoldenArcher(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(r) != 20 || r[0].Index != 12*512+15 || r[0].Group != 0 || r[0].Rate != 1000 ||
		r[0].Option != [6]int{-1, -1, -1, -1, -1, -1} || r[19].Group != 4 || r[19].Index != 14*512+16 {
		t.Fatalf("rows %+v", r)
	}
}

func TestLoadCastleDeep(t *testing.T) {
	d, err := LoadCastleDeep(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Schedule) != 3 || d.Schedule[1].Hour != 12 || d.Schedule[1].Minute != 0 || d.Schedule[1].DayOfWeek != -1 {
		t.Fatalf("schedule %+v", d.Schedule)
	}
	if len(d.Groups) != 2 || d.Groups[1] != (CastleDeepGroup{2, 50}) {
		t.Fatalf("groups %+v", d.Groups)
	}
	if len(d.Stages) != 6 || d.Stages[2] != (CastleDeepStage{1, 3, 20}) {
		t.Fatalf("stages %+v", d.Stages)
	}
	if len(d.Monsters) != 59 || d.Monsters[0] != (CastleDeepMonster{1, 1, 39, 15, 60, 28, 132, 73}) ||
		d.Monsters[29] != (CastleDeepMonster{1, 3, 295, 1, 60, 58, 132, 101}) {
		t.Fatalf("monsters %d %+v %+v", len(d.Monsters), d.Monsters[0], d.Monsters[29])
	}
	// Unknown sections are skipped; short rows are an error.
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Event"), 0o755)
	os.WriteFile(filepath.Join(dir, "Event", "CastleDeepEvent.dat"), []byte("7\n1 2 3\nend\n1\n1 100\nend\n"), 0o644)
	if d, err := LoadCastleDeep(dir); err != nil || len(d.Groups) != 1 || len(d.Schedule) != 0 {
		t.Fatalf("skip: %v %+v", err, d)
	}
	os.WriteFile(filepath.Join(dir, "Event", "CastleDeepEvent.dat"), []byte("3\n1 1 39 15 60\nend\n"), 0o644)
	if _, err := LoadCastleDeep(dir); err == nil {
		t.Fatal("short row accepted")
	}
}
