package game

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

// expTableRate follows Util/ExperienceTable.txt (FUN_0047c040): empty in
// this installation (100), first matching record otherwise.
func TestExpTableRate(t *testing.T) {
	b := newBareWorld(t)
	o := b.player(PlayerStart, "exp", 0, 130, 130)
	if len(b.expTable) != 0 || b.expTableRate(o) != 100 {
		t.Fatalf("shipped table %v gives %d", b.expTable, b.expTableRate(o))
	}
	b.expTable = gamedata.ExperienceTable{
		{MinLevel: 1, MaxLevel: 399, MinReset: -1, MaxReset: -1, MinMasterReset: -1, MaxMasterReset: -1, Rate: 300},
		{MinLevel: 400, MaxLevel: -1, MinReset: 5, MaxReset: -1, MinMasterReset: -1, MaxMasterReset: -1, Rate: 50},
	}
	if got := b.expTableRate(o); got != 100 { // level 400, 0 resets: no record
		t.Errorf("rate = %d, want 100", got)
	}
	o.P.Char.Resets = 5
	if got := b.expTableRate(o); got != 50 {
		t.Errorf("rate = %d, want 50", got)
	}
	o.P.Char.Level = 10
	if got := b.expTableRate(o); got != 300 {
		t.Errorf("rate = %d, want 300", got)
	}
}
