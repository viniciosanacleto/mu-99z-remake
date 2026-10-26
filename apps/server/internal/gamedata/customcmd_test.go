package gamedata

import (
	"testing"
)

func TestLoadCustomAttackOriginal(t *testing.T) {
	a, err := LoadCustomAttack(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Speeds) != 1 || a.Speeds[0] != (AttackSpeedRow{-1, -1, -1, -1, -1, -1, 1500, 4}) {
		t.Fatalf("speeds %+v", a.Speeds)
	}
	if len(a.Skills) != 40 {
		t.Fatalf("%d skills", len(a.Skills))
	}
	if s := a.Skills[16]; s.Group != 0 || s.Value != 1 || s.Class != [5]int{1, 1, 1, 1, 1} {
		t.Fatalf("mana shield %+v", s)
	}
	if s := a.Skills[41]; s.Group != 3 || s.Value != 1 {
		t.Fatalf("twisting slash %+v", s)
	}
	if a.Order[0] != 1 || a.Order[len(a.Order)-1] != 66 {
		t.Fatalf("order %v", a.Order)
	}
	// Formula of the original file: 1500 - 4 * speed.
	if d := a.AttackDelay(19, 100, 250); d != 500 {
		t.Fatalf("delay %d", d)
	}
}

func TestLoadCustomGift(t *testing.T) {
	// The original file has only the commented demonstration rows.
	orig, err := LoadCustomGift(configDir())
	if err != nil || len(orig) != 0 {
		t.Fatalf("original %v %v", orig, err)
	}
}

func TestLoadCustomMarriageOriginal(t *testing.T) {
	m, err := LoadCustomMarriage(configDir())
	if err != nil {
		t.Fatal(err)
	}
	want := MarriageItem{Enable: 1, Index: 12*512 + 15, Level: 0, Count: 1}
	if m.Marry != want || m.Divorce != want {
		t.Fatalf("%+v", m)
	}
	if len(m.Rewards) != 1 || m.Rewards[0].Index != 13*512+42 {
		t.Fatalf("rewards %+v", m.Rewards)
	}
}

func TestLoadCustomBuyVip(t *testing.T) {
	if orig, err := LoadCustomBuyVip(configDir()); err != nil || len(orig) != 0 {
		t.Fatalf("original %v %v", orig, err)
	}
}

func TestLoadCustomPickOriginal(t *testing.T) {
	p, err := LoadCustomPick(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || p[0] != (PickBlock{Index: 14*512 + 22, Level: -1, AccountLevel: -1}) {
		t.Fatalf("%+v", p)
	}
}
