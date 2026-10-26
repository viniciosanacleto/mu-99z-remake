package gamedata

import "testing"

func TestLoadMonsterListRates(t *testing.T) {
	m, err := LoadMonsterList(configDir(), MonsterRates{110, 105, 105, 105, 105})
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

func TestLoadSpawnsLorencia(t *testing.T) {
	out, err := LoadSpawns(configDir(), func(mp int) bool { return mp == 0 }, seq(0))
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
	talk, err := LoadNpcTalk(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(talk) != 5 || talk[0].Class != 247 || talk[0].Map != 0 || talk[0].X != 130 ||
		talk[2].Map != -1 || talk[4].Class != 240 {
		t.Fatalf("talk = %+v", talk)
	}
}
