package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestSkillGroup(t *testing.T) {
	cfg := convertGroup(t, "skill", map[string]string{
		"Skill/SkillList.txt": "1 \"Poison\" 12 42 0 6 0 0 1 55 0 0 0 0 0 1 0 0 1 0\nend\n",
		"Skill/EffectList.txt": "1 1 * \"Greater Damage\" 1 0 0 * * * * *\n" +
			"30 30 13 66 \"Santa Ring\" 1 2 0 3600 10 * * * // item effect\n" +
			"31 31 13 600 \"Out of range\" 1 2 0 60 1 2 3 4\nend\n",
		"Skill/SkillDamage.txt":  "0\n41 100\n42 90 80 // pvp\nend\n1\n1 100 // Poison\nend\n2\n9 9\nend\n",
		"Skill/ComboSkill.txt":   "1 19 0 3000 extra\nend\n",
		"Skill/SkillEnabler.txt": "41 40 1 1 // map 40\n41 * 1 0\nend\n",
		"Skill/SkillUseArea.txt": "0\n16 //Soul Barrier\nend\n1\n30 10 20 30 40\nend\n",
	})
	sk, err := gamedata.LoadSkills(cfg)
	if err != nil || sk[1].Name != "Poison" || sk[1].Effect != 55 || sk[1].Class != [5]int{1, 0, 0, 1, 0} {
		t.Fatalf("skills = %+v %v", sk, err)
	}
	f, err := gamedata.LoadSkillConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e := f.EffectList.Rows
	if len(e) != 3 || e[0].ItemIndex != "*" || e[0].Duration != -1 || e[1].ItemIndex != "13,066" || e[1].Name != "Santa Ring" ||
		e[1].Power != [4]int{10, -1, -1, -1} || e[2].ItemIndex != "13,600" || e[2].Power != [4]int{1, 2, 3, 4} {
		t.Fatalf("effects = %+v", e)
	}
	d := f.SkillDamage
	if len(d.Damage) != 1 || d.Damage[0] != (gamedata.SkillDamageRow{SkillIndex: 42, DamageRatePvM: 90, DamageRatePvP: 80, Comment: "pvp"}) ||
		len(d.Effect) != 1 || d.Effect[0].EffectRate != 100 {
		t.Fatalf("damage (rows without a PvP rate are skipped) = %+v", d)
	}
	if len(f.ComboSkill.Rows) != 1 || f.ComboSkill.Rows[0].Delay != 3000 || len(f.SkillEnabler.Rows) != 2 ||
		f.SkillEnabler.Rows[1].MapNumber != -1 || len(f.SkillUseArea.Skill) != 1 || f.SkillUseArea.Area[0].Y2 != 40 {
		t.Fatalf("tables = %+v", f)
	}
	if _, err := tryConvertGroup(t, "skill", map[string]string{"Skill/EffectList.txt": "1 1 *\nend\n"}); err == nil {
		t.Fatal("effect without a name accepted")
	}
	if _, err := tryConvertGroup(t, "skill", map[string]string{"Skill/SkillList.txt": "1 \"x\" 1\nend\n"}); err == nil {
		t.Fatal("short SkillList row accepted")
	}
}
