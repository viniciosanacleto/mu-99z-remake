package gamedata

import "testing"

func TestLoadCastleSiege(t *testing.T) {
	cs, err := LoadCastleSiege(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if cs.CycleDays != 7 || len(cs.Schedule) != 9 || len(cs.NPCs) != 24 {
		t.Fatalf("cycle %d schedule %d npcs %d", cs.CycleDays, len(cs.Schedule), len(cs.NPCs))
	}
	// State 1 at day 0 00:00 lasts until state 2 at day 1 23:59.
	if s, _ := cs.ScheduleOf(1); s.Duration != (47*60+59)*60 {
		t.Fatalf("state 1 duration %d", s.Duration)
	}
	// READY (6) day 4 19:00 -> START (7) day 6 20:00; START lasts 2 hours.
	if s, _ := cs.ScheduleOf(6); s.Duration != (2*24+1)*3600 {
		t.Fatalf("state 6 duration %d", s.Duration)
	}
	if s, _ := cs.ScheduleOf(7); s.Duration != 2*3600 {
		t.Fatalf("state 7 duration %d", s.Duration)
	}
	if s, _ := cs.ScheduleOf(9); s.Duration != 0 || s.Day != 7 {
		t.Fatalf("state 9 %+v", s)
	}
	gate := cs.NPCs[4]
	if gate.Class != 277 || gate.Index != 1 || !gate.DBSave || gate.MaxLife != 500000 || gate.X != 93 || gate.Y != 204 {
		t.Fatalf("first gate %+v", gate)
	}
	if n := cs.NPCs[23]; n.Class != 221 || n.Side != 2 || n.DBSave || n.Dir != 5 {
		t.Fatalf("slingshot %+v", n)
	}
	if cs.GateDefenseUpgrade[1] != (CastleUpgrade{2, 3, 3000000}) || cs.StatueRegenUpgrade[2] != (CastleUpgrade{3, 7, 5000000}) ||
		cs.GateLifeUpgrade[0] != (CastleUpgrade{800000, 2, 1000000}) || cs.StatueLifeUpgrade[2] != (CastleUpgrade{2500000, 7, 1000000}) {
		t.Fatalf("upgrades %+v %+v", cs.GateDefenseUpgrade, cs.StatueRegenUpgrade)
	}
	if cs.GateDefense != [4]int{100, 180, 300, 520} || cs.StatueDefense != [4]int{80, 180, 340, 550} {
		t.Fatalf("defense %v %v", cs.GateDefense, cs.StatueDefense)
	}
}
