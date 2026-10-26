package gamedata

import (
	"path/filepath"
	"runtime"
	"testing"
)

func dataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "data", "Data")
}

func TestLoadTerrains(t *testing.T) {
	ts, err := LoadTerrains(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if ts[0].File != "Terrain1.att" || ts[0].Width != 256 || ts[0].Height != 256 {
		t.Fatalf("lorencia = %s %dx%d", ts[0].File, ts[0].Width, ts[0].Height)
	}
	// Blood Castle 2..7 reuse terrain12.att, Chaos Castle 2..6 reuse terrain19.att.
	for m := 11; m <= 16; m++ {
		if ts[m].File != "Terrain12.att" {
			t.Errorf("map %d uses %s", m, ts[m].File)
		}
	}
	for m := 18; m <= 23; m++ {
		if ts[m].File != "Terrain19.att" {
			t.Errorf("map %d uses %s", m, ts[m].File)
		}
	}
	if ts[99].File != "Terrain41.att" {
		t.Errorf("map 99 uses %s", ts[99].File)
	}
	// Instances are independent copies.
	ts[11].Set(10, 10, 0xEE)
	if ts[12].Get(10, 10) == 0xEE {
		t.Fatal("terrains share memory")
	}
	if ts[0].Get(-1, 0) != AttrBlock|AttrNoGround || ts[0].Get(0, 256) != AttrBlock|AttrNoGround {
		t.Fatal("out of range cells must be blocked")
	}
	// Lorencia spawn (182,128) is walkable.
	if a := ts[0].Get(182, 128); a&(AttrBlock|AttrNoGround) != 0 {
		t.Fatalf("lorencia spawn attr %#x", a)
	}
}

func TestParseTerrainErrors(t *testing.T) {
	if _, err := ParseTerrain("x", []byte{0, 1}); err == nil {
		t.Fatal("short header")
	}
	if _, err := ParseTerrain("x", []byte{0, 1, 1, 0, 0}); err == nil {
		t.Fatal("short data")
	}
	tr, err := ParseTerrain("x", []byte{0, 1, 1, 1, 2, 3, 4})
	if err != nil || tr.Width != 2 || tr.Get(1, 1) != 4 {
		t.Fatalf("%+v %v", tr, err)
	}
}

func TestMapManagerAndGates(t *testing.T) {
	mm, err := LoadMapManager(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if mm[0].Name != "Lorencia" || mm[0].ViewRange != 12 || mm[0].NonPK != -1 || mm[6].ViewRange != 10 {
		t.Fatalf("map manager = %+v / %+v", mm[0], mm[6])
	}
	gates, err := LoadGates(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	g := gates[1]
	if g.Flag != 1 || g.Map != 0 || g.X1 != 121 || g.Y1 != 232 || g.TargetGate != 2 || g.MinLevel != 20 {
		t.Fatalf("gate 1 = %+v", g)
	}
	ms, err := LoadMapServer(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.Servers) != 2 || ms.ServerForMap(0, 30) != 19 || ms.ServerForMap(0, 0) != 0 || ms.ServerForMap(19, 30) != 19 {
		t.Fatalf("map server = %+v", ms)
	}
}

func TestExperience(t *testing.T) {
	e := NewExperience(1, 1, 1)
	if e.Level[1] != 10 || e.Level[2] != 44 || e.Level[255] != 264*255*255 {
		t.Fatalf("exp = %d %d %d", e.Level[1], e.Level[2], e.Level[255])
	}
	if e.Level[256] != 265*256*256+10 || e.Level[257] != 266*257*257+44 {
		t.Fatalf("exp 256/257 = %d %d", e.Level[256], e.Level[257])
	}
	if e.Pet[2] != 0 || e.Pet[3] != 13*27 {
		t.Fatalf("pet = %d %d", e.Pet[2], e.Pet[3])
	}
	if e.Next(1) != 10 || e.Next(5000) != e.Level[1000] || e.Floor(2) != 10 || e.Floor(0) != 0 {
		t.Fatal("Next/Floor")
	}
}

func TestSkills(t *testing.T) {
	sk, err := LoadSkills(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	eb := sk[17]
	if eb.Name != "Energy Ball" || eb.Damage != 3 || eb.Mana != 1 || eb.Range != 6 || eb.Type != -1 || eb.Class != [5]int{1, 0, 0, 1, 0} {
		t.Fatalf("energy ball = %+v", eb)
	}
	if !eb.CanLearn(1, 0, 0, 0) || eb.CanLearn(1, 0, 0, 1) || eb.CanLearn(1, 0, 0, 5) {
		t.Fatal("CanLearn class column")
	}
	if fb := sk[74]; fb.CanLearn(1, 1000, 1000, 4) != (fb.ReqLevel <= 1) {
		t.Fatal("CanLearn level")
	}
}

func TestItems(t *testing.T) {
	items, err := LoadItems(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	kris := items[0]
	if kris.Name != "Kris" || kris.Width != 1 || kris.Height != 2 || kris.DamageMin != 6 || kris.DamageMax != 11 ||
		kris.AttackSpeed != 50 || kris.Durability != 20 || kris.ReqStrength != 40 || kris.Class != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("kris = %+v", kris)
	}
	// Custom additions at the end of the file.
	if sb := items[29]; sb.Name != "Sonic Blade" || sb.Level != 149 {
		t.Fatalf("sonic blade = %+v", sb)
	}
	if sc := items[15*512+16]; sc.Name != "Scroll of Decay" || sc.ReqEnergy != 242 || sc.BuyMoney != 345000 || sc.Slot != -1 {
		t.Fatalf("scroll = %+v", sc)
	}
	if ItemName(15*512+16) != "15,16" {
		t.Fatal("ItemName")
	}
}

func TestQuests(t *testing.T) {
	q, err := LoadQuests(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Quests) != 16 || q.Quests[0].NPC != 235 || q.Quests[0].RequireIndex != -1 || q.Quests[2].RequireMinLevel != 220 {
		t.Fatalf("quests = %+v", q.Quests[:3])
	}
	o := q.Objectives[2]
	if o.Type != 1 || o.Index != 7191 || o.DropMinLevel != 45 || o.DropMaxLvl != 60 || o.ItemDropRate != 10 {
		t.Fatalf("objective = %+v", o)
	}
	r := q.Rewards[2]
	if r.Sort != 1 || r.Type != 2 || r.Index != 201 || r.RequireIndex != 1 {
		t.Fatalf("reward = %+v", r)
	}
}
