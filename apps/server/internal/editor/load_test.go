package editor

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestLoadReadsServerData(t *testing.T) {
	m, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	ed, err := Load(configDir(), "gameserver", m)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ed.Catalog().Items); n < 300 {
		t.Fatalf("catalog has %d items", n)
	}
	// gameserver.toml [Common]: ServerMaxLevel 400, MaxStatPoint_AL0..3 32767,
	// ExperienceMultiplierConstA/B 1 (exp[1] = (1+9)*1*1*1).
	lim := ed.Limits()
	if lim.MaxLevel != 400 || lim.MaxStat != [4]int{32767, 32767, 32767, 32767} || lim.Exp == nil || lim.Exp.Floor(2) != 10 {
		t.Fatalf("limits = %d %v %v", lim.MaxLevel, lim.MaxStat, lim.Exp != nil)
	}
	if _, err := Load(configDir(), "nope", m); err == nil {
		t.Fatal("missing instance accepted")
	}
}
