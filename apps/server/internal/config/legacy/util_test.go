package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestHackGroup(t *testing.T) {
	cfg := convertGroup(t, "hack", map[string]string{
		"Hack/HackPacketCheck.txt":     "0 * 0 1000 5 10 // chat\nend\n",
		"Hack/HackSkillSpeedCheck.txt": "16 * * * * * 0 50 300 1 5 2\nend",
	})
	f, err := gamedata.LoadHackConfig(cfg)
	if err != nil || len(f.HackPacketCheck.Rows) != 1 || f.HackPacketCheck.Rows[0].Value != -1 || f.HackPacketCheck.Rows[0].MaxCount != 10 {
		t.Fatalf("packet check = %+v %v", f, err)
	}
	rows, err := gamedata.LoadHackSkillSpeedCheck(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != (gamedata.SkillSpeedRule{Skill: 16, Class: -1, MinLevel: -1, MaxLevel: -1, MinReset: -1,
		MaxReset: -1, MinSpeed: 0, MaxSpeed: 50, MaxDelay: 300, MinCount: 1, MaxCount: 5, Action: 2}) {
		t.Fatalf("rows = %+v", rows)
	}
	if _, err := tryConvertGroup(t, "hack", map[string]string{"Hack/HackPacketCheck.txt": "0 * 0 1000 5\nend\n"}); err == nil {
		t.Fatal("five-column packet rule accepted")
	}
}

func TestUtilGroup(t *testing.T) {
	cfg := convertGroup(t, "util", map[string]string{
		"Util/DamageTable.txt":    "0\n* * * * * * * 50\nend\n1\n7 1 400 * * 275 * * 80\nend\n2\n1 2 3\nend\n",
		"Util/Filter.txt":         "\"fuck\"\n\"\"\nend\n",
		"Util/GameMaster.txt":     "\"teste2\" \"Admin\" 32\n\"nolevel\" \"x\"\nend\n",
		"Util/PetDamageTable.txt": "0 * * * * 1000 1 * *\n* * 900\nend\n",
		"Util/ResetTable.txt":     "0 10 400 400 400 400 1 2 3 4 5 6 7 8\nend\n",
		"Util/ResetLimit.txt":     "0 * 1 2 3 4 5 6 7 8 9 10 11 12\nend\n",
		// Seven numbers per record: a row with the header's four AL rates
		// is cut into further records, like FUN_0047be80.
		"Util/ExperienceTable.txt": "1 100 * * * * 300\n101 * 5 10 * * 50\n* * * * * * 20 301 302 303\nend",
	})
	f, err := gamedata.LoadUtilConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := f.DamageTable
	if len(d.Monster) != 1 || d.Monster[0].DamageRate != 50 || len(d.Player) != 1 || d.Player[0].MonsterIndex != 275 || d.Player[0].DamageRate != 80 {
		t.Fatalf("damage = %+v", d)
	}
	if len(f.Filter.Words) != 2 || len(f.GameMaster.Rows) != 1 || f.GameMaster.Rows[0].GMLevel != 32 {
		t.Fatalf("filter / gm = %+v %+v", f.Filter, f.GameMaster)
	}
	if r := f.ResetTable.Rows; len(r) != 1 || r[0].Level != [4]int{400, 400, 400, 400} || r[0].Point != [4]int{5, 6, 7, 8} {
		t.Fatalf("reset table = %+v", r)
	}
	if r := f.ResetLimit.Rows; len(r) != 1 || r[0].MaxReset != -1 || r[0].LimitMonth != [4]int{9, 10, 11, 12} {
		t.Fatalf("reset limit = %+v", r)
	}
	pets, err := gamedata.LoadPetDamageTable(cfg)
	if err != nil || len(pets) != 2 || pets[1] != (gamedata.PetDamageRow{Pet: 1, MinLevel: -1, MaxLevel: -1, MinReset: -1, MaxReset: -1, DamageRate: 900}) {
		t.Fatalf("pets (token stream) = %+v %v", pets, err)
	}
	tbl, err := gamedata.LoadExperienceTable(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl) != 4 || tbl[3].MinLevel != 301 {
		t.Fatalf("%d records: %+v", len(tbl), tbl)
	}
	for _, c := range []struct{ lv, rs, mr, want int }{{50, 0, 0, 300}, {150, 7, 0, 50}, {150, 11, 0, 20}, {150, 4, 3, 20}} {
		if got := tbl.Rate(c.lv, c.rs, c.mr); got != c.want {
			t.Errorf("Rate(%d,%d,%d) = %d, want %d", c.lv, c.rs, c.mr, got, c.want)
		}
	}
}
