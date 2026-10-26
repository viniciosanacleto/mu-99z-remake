package gamedata

import "testing"

func TestLoadClassInfo(t *testing.T) {
	c, err := LoadClassInfo(configDir())
	if err != nil {
		t.Fatal(err)
	}
	dk := c[1]
	if dk.Strength != 28 || dk.Dexterity != 20 || dk.Vitality != 25 || dk.Energy != 10 ||
		dk.MaxLife != 110 || dk.MaxMana != 20 || dk.LevelLife != 2 || dk.LevelMana != 0.5 ||
		dk.VitalityToLife != 3 || dk.EnergyToMana != 1 {
		t.Fatalf("DK = %+v", dk)
	}
	// The Dark Lord row really has MaxLife 40090 in this installation.
	if dl := c[4]; dl.Lead != 25 || dl.MaxLife != 40090 || dl.LevelLife != 1.5 {
		t.Fatalf("DL = %+v", dl)
	}
	if len(c) != 5 {
		t.Fatalf("%d classes", len(c))
	}
}

func TestLoadItemOptions(t *testing.T) {
	o, err := LoadItemOptions(configDir())
	if err != nil {
		t.Fatal(err)
	}
	r := o[2][0]
	if r.OptionIndex != 80 || r.Value != 4 || r.ItemMin != 0 || r.ItemMax != 4*512+511 ||
		r.Skill != -1 || r.Luck != -1 || r.Additional != 1 || r.Excellent != -1 {
		t.Fatalf("first slot-2 rule %+v", r)
	}
	if !r.Matches(1, 0, 0, 1, 0) || r.Matches(1, 0, 0, 0, 0) || r.Matches(5*512, 0, 0, 1, 0) {
		t.Fatal("Matches")
	}
	exc := o[8][0]
	if exc.OptionIndex != 92 || exc.Excellent != 32 || exc.Matches(0, 0, 0, 0, 16) || !exc.Matches(0, 0, 0, 0, 0x20) {
		t.Fatalf("slot-8 rule %+v", exc)
	}
}

func TestLoadWingAndCustomTables(t *testing.T) {
	wo, err := LoadWingOptions(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if w := wo[12*512]; w.DefenseConstA != 3 || w.IncDamageConstA != 112 || w.DecDamageConstA != 88 {
		t.Fatalf("wings of elf %+v", w)
	}
	if w := wo[13*512+30]; w.DefenseConstA != 3 || w.MaxNewOption != 4 {
		t.Fatalf("cape %+v", w)
	}
	cw, err := LoadCustomWings(configDir())
	if err != nil {
		t.Fatal(err)
	}
	c := cw[12*512+180]
	if c.Index != 15 || c.DefenseConstA != 3 || c.Option[0] != [2]int{80, 4} || c.Option[2] != [2]int{83, 4} ||
		c.NewOption[0] != [2]int{110, 5} || c.NewOption[3] != [2]int{108, 3} {
		t.Fatalf("custom wing %+v", c)
	}
	at, err := LoadCustomAttributes(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if a := at[7*512+106]; a.DamageReflect != 20 || a.DamageMin != 10 || a.DamageMax != 20 || a.ExcellentDamageRate != 30 {
		t.Fatalf("attribute %+v", a)
	}
	cm, err := LoadCustomMonsters(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if CustomMonsterExperienceRate(cm, 293, 10) != 100 || CustomMonsterExperienceRate(cm, 1, 0) != 100 {
		t.Fatal("custom monster rate")
	}
	rows := []CustomMonster{{Monster: 5, Map: -1, ExperienceRate: 250}, {Monster: -1, Map: 2, ExperienceRate: -1}}
	if CustomMonsterExperienceRate(rows, 5, 9) != 250 || CustomMonsterExperienceRate(rows, 6, 2) != 100 {
		t.Fatal("first-match rule")
	}
}
