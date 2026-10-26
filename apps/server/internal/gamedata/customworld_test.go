package gamedata

import "testing"

func TestLoadWorldCustoms(t *testing.T) {
	mv, err := LoadCustomMove(configDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(mv) == 0 || mv[0] != (CustomMoveRow{Index: 1, Command: "/bar", Map: 0, X: 125, Y: 125, MinLevel: -1,
		MaxLevel: -1, MinReset: -1, MaxReset: -1, MinMReset: -1, MaxMReset: -1, AccountLevel: 0, PKMove: 1}) {
		t.Fatalf("move %+v", mv[0])
	}
	for i := 1; i < len(mv); i++ {
		if mv[i].Index <= mv[i-1].Index {
			t.Fatal("not sorted by Id")
		}
	}
	if last := mv[len(mv)-1]; last.Command != "/pvp" || last.Map != 40 {
		t.Fatalf("last %+v", last)
	}
	g, err := LoadCustomGate(configDir())
	if err != nil || len(g) != 6 || g[0] != (CustomGateRow{Index: 1, Map: 7, X: 233, Y: 209, ToMap: 25, ToX: 10, ToY: 116,
		MinLevel: -1, MaxLevel: -1, MinReset: -1, MaxReset: -1, AccountLevel: 0}) {
		t.Fatalf("gate %+v %v", g, err)
	}
	r, err := LoadCustomRespawn(configDir())
	if err != nil || len(r) != 1 || r[0] != (CustomRespawnRow{Map: 6, Gate: [4]int{-1, -1, -1, -1}}) {
		t.Fatalf("respawn %+v %v", r, err)
	}
	n, err := LoadCustomNPCCommand(configDir())
	if err != nil || len(n) != 2 || n[0] != (CustomNPCCommandRow{Class: 247, Map: 0, X: 130, Y: 126, Command: "/reset"}) {
		t.Fatalf("npc %+v %v", n, err)
	}
	d, err := LoadCustomDeathMessage(configDir())
	if err != nil || d[1] == "" || d[579] != "I'm Golden Moster You Are Dead" {
		t.Fatalf("death %d %v", len(d), err)
	}
}
