package gamedata

import "testing"

func TestLoadMonsterListRates(t *testing.T) {
	m, err := LoadMonsterList(dataDir(), MonsterRates{110, 105, 105, 105, 105})
	if err != nil {
		t.Fatal(err)
	}
	bf := m[0]
	// 0 0 "Bull Fighter" 6 100 0 16 20 6 0 28 6 3 0 1 6 450 1600 5 2 130 20 6 0 ...
	if bf == nil || bf.Name != "Bull Fighter" || bf.Level != 6 || bf.MaxLife != 110 || bf.Life != 110 ||
		bf.DamageMin != 16 || bf.DamageMax != 21 || bf.Defense != 6 || bf.AttackRate != 29 ||
		bf.DefenseRate != 6 || bf.MoveRange != 3 || bf.AttackType != 0 || bf.AttackRange != 1 ||
		bf.ViewRange != 6 || bf.MoveSpeed != 450 || bf.AttackSpeed != 1600 || bf.RegenTime != 5 ||
		bf.Attribute != 2 || bf.ItemRate != 130 || bf.MoneyRate != 20 || bf.MaxItemLevel != 6 {
		t.Fatalf("Bull Fighter = %+v", bf)
	}
	if lich := m[6]; lich.AttackType != 2 || lich.AttackRange != 4 || lich.ViewRange != 7 {
		t.Fatalf("Lich = %+v", lich)
	}
	if npc := m[240]; npc == nil || npc.Type == 0 {
		t.Fatalf("NPC 240 = %+v", npc)
	}
	if monsterRate(2000000000, 200) != 2100000000 {
		t.Fatal("rate cap")
	}
}

func seq(vals ...int) func() int {
	i := 0
	return func() int {
		v := vals[i%len(vals)]
		i++
		return v
	}
}

func TestParseSpawnFileSections(t *testing.T) {
	src := []byte(`
0
226 0 122 110 3
257 0 132 90 -1
end
1
0 5 130 78 139 87 -1 3 // comment
end
2
7 10 100 100 2
end
3
042 30 010 010 240 240 -1 01 0 7 //Red Dragon
end
5
1 2 3 4 5
end
`)
	// rnd: dir of 257 (9&7=1), three section-1 dirs (10,11,12 -> 2,3,4),
	// section 2 jitter (5%7=5 -> +2, 0%7 -> -3).
	out, err := parseSpawnFile(src, 0, seq(9, 10, 11, 12, 5, 0), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 8 {
		t.Fatalf("records = %d: %+v", len(out), out)
	}
	if out[0] != (SpawnRecord{Section: 0, Class: 226, X: 122, Y: 110, Dir: 3}) {
		t.Fatalf("NPC = %+v", out[0])
	}
	if out[1].Dir != 1 {
		t.Fatalf("random dir = %+v", out[1])
	}
	for i, d := range []int{2, 3, 4} {
		r := out[2+i]
		if r.Section != 1 || r.Class != 0 || r.Dis != 5 || r.X != 130 || r.Y != 78 || r.TX != 139 || r.TY != 87 || r.Dir != d {
			t.Fatalf("area %d = %+v", i, r)
		}
	}
	if r := out[5]; r.Section != 2 || r.X != 102 || r.Y != 97 || r.Dir != 2 || r.Dis != 10 {
		t.Fatalf("single = %+v", r)
	}
	// Section 3 rows are read as five-number records (FUN_004bbc70).
	if r := out[6]; r.Section != 3 || r.Class != 42 || r.Dis != 30 || r.X != 10 || r.Y != 10 || r.Dir != 240 {
		t.Fatalf("invasion = %+v", r)
	}
	if r := out[7]; r.Section != 3 || r.Class != 240 || r.Dis != -1 || r.X != 1 || r.Y != 0 || r.Dir != 7 {
		t.Fatalf("invasion tail = %+v", r)
	}
}

func TestLoadSpawnsLorencia(t *testing.T) {
	out, err := LoadSpawns(dataDir(), func(mp int) bool { return mp == 0 }, seq(0))
	if err != nil {
		t.Fatal(err)
	}
	var npc, area int
	for _, r := range out {
		if r.Map != 0 {
			t.Fatalf("map %d", r.Map)
		}
		switch r.Section {
		case SpawnNPC:
			npc++
		case SpawnArea:
			area++
		}
	}
	if npc != 13 || area != 39*8 {
		t.Fatalf("npc=%d area=%d", npc, area)
	}
}

func TestLoadNpcTalk(t *testing.T) {
	talk, err := LoadNpcTalk(dataDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(talk) != 5 || talk[0].Class != 247 || talk[0].Map != 0 || talk[0].X != 130 ||
		talk[2].Map != -1 || talk[4].Class != 240 {
		t.Fatalf("talk = %+v", talk)
	}
}
