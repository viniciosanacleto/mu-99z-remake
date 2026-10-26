package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func seq(vals ...int) func() int {
	i := 0
	return func() int {
		v := vals[i%len(vals)]
		i++
		return v
	}
}

func TestSpawnStreamReader(t *testing.T) {
	src := `
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
`
	// rnd: dir of 257 (9&7=1), three section-1 dirs (10,11,12 -> 2,3,4),
	// section 2 jitter (5%7=5 -> +2, 0%7 -> -3).
	cfg := convertGroup(t, "spawn", map[string]string{"Monster/Spawn/000 - Test.txt": src,
		"Monster/Spawn/notes.txt": "not a spawn file", "Monster/Spawn/1 - Bad.txt": "x"})
	out, err := gamedata.LoadSpawns(cfg, nil, seq(9, 10, 11, 12, 5, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 8 {
		t.Fatalf("records = %d: %+v", len(out), out)
	}
	if out[0] != (gamedata.SpawnRecord{Section: 0, Class: 226, X: 122, Y: 110, Dir: 3}) {
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

func TestMonsterScheduleStream(t *testing.T) {
	dir := convertGroup(t, "monster", map[string]string{"Monster/MonsterSchedule.txt": `0
0 * * * * 20 0 0
end
1
0 * * * 600
end
2
0 229 0 137 087
0 229 2 198 047
end`})
	m, err := gamedata.LoadMonsterSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	g := m[0]
	if g == nil || len(g.Schedule) != 1 || g.Schedule[0] != (gamedata.Schedule{Year: -1, Month: -1, Day: -1, DayOfWeek: -1, Hour: 20}) ||
		g.EnableMsg != -1 || g.NPCSec != 600 || len(g.Monsters) != 2 || g.Monsters[1] != (gamedata.ScheduledMonster{Monster: 229, Map: 2, X: 198, Y: 47}) {
		t.Fatalf("group 0 = %+v", g)
	}
}

func TestMonsterGroup(t *testing.T) {
	cfg := convertGroup(t, "monster", map[string]string{
		"Monster/MonsterList.txt": "0 0 \"Bull Fighter\" 6 100 0 16 20 6 0 28 6 3 0 1 6 450 1600 5 2 130 20 6 0 1 2 3 4 5 6 7 // bf\n" +
			"1000 0 \"Too High\" 6 100 0 16 20 6 0 28 6 3 0 1 6 450 1600 5 2 130 20 6 0 0 0 0 0 0 0 0\nend\n",
		"Monster/MonsterMove.txt": "229 0 137 87 1 1200 229\n2 198 52 2 1200\nend\n",
		"Monster/NpcTalk.txt":     "247 0 130 126 \"Sua habilidade ser\xe1 restaurada\"\n249 * * * \"Bem Vindo\"\nend\n",
		"Monster/Skill/MonsterSkill.txt": "0\n304 \"Witch Queen\" 0 4 0 1 * * * * * * * * * * * * * * * *\nend\n" +
			"1\n305 \"Skipped\" 0 5\nend\n",
		"Monster/Skill/MonsterSkillElement.txt": "0\n1 \"Element\" 0 50 3 * * * 0 0\n2 \"Short\" 4\nend\n",
		"Monster/Skill/MonsterSkillUnit.txt":    "0\n4 \"Unit\" 1 * 0 300 1 2 * * *\nend\n",
	})
	m, err := gamedata.LoadMonsterList(cfg, gamedata.DefaultMonsterRates)
	if err != nil {
		t.Fatal(err)
	}
	if bf := m[0]; len(m) != 1 || bf.Name != "Bull Fighter" || bf.MaxLife != 100 || bf.Resistance != [7]int{1, 2, 3, 4, 5, 6, 7} {
		t.Fatalf("monsters = %+v", m)
	}
	moves, err := gamedata.LoadMonsterMove(cfg)
	if err != nil || len(moves) != 2 || moves[1] != (gamedata.MonsterMoveRow{Monster: 229, Map: 2, X: 198, Y: 52, Dir: 2, KeepTime: 1200}) {
		t.Fatalf("moves (records cut from the token stream) = %+v %v", moves, err)
	}
	talk, err := gamedata.LoadNpcTalk(cfg)
	if err != nil || len(talk) != 2 || talk[0].Message != "Sua habilidade ser\xe1 restaurada" || talk[1].Map != -1 || talk[1].X != -1 {
		t.Fatalf("talk = %+v %v", talk, err)
	}
	f, err := gamedata.LoadMonsterConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r := f.MonsterSkill.Rows; len(r) != 1 || r[0].Skill[0] != [2]int{0, 4} || r[0].Skill[2] != [2]int{-1, -1} {
		t.Fatalf("skills (section 0 only) = %+v", r)
	}
	if e := f.MonsterSkillElement.Rows; len(e) != 2 || e[0].IncAndDecType != -1 || e[1].Type != 4 || e[1].SuccessRate != 0 {
		t.Fatalf("elements (missing columns read 0) = %+v", e)
	}
	if u := f.MonsterSkillUnit.Rows; len(u) != 1 || u[0].ScopeType != -1 || u[0].Slot != [5]int{1, 2, -1, -1, -1} {
		t.Fatalf("units = %+v", u)
	}
}
