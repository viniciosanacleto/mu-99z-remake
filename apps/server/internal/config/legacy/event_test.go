package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestEventGroup(t *testing.T) {
	cfg := convertGroup(t, "event", map[string]string{
		"Event/BloodCastle.dat": "0\n1 1 15 1\nend\n1\n* * * * 20 0 0\nend\n2\n0 10 20\n7 1 1 // out of range\nend\n" +
			"4\n0 100 200\nend\n5\n0 12 34\nend\n9\n1 2\nend\n",
		"Event/BonusManager.dat": "0\n0 * * * * 20 0 0\n25 x // other index: not read\nend\n1\n0 -1 -1 3600 \"Double Exp\"\n1 5 6 60\nend\n" +
			"2\n0 0 200 200 300 400 *\nend\n",
		"Event/CastleSiege.dat":     "0\n5\nend\n1\n1 0 0 0 // Register\nend\n2\n283 1 1 1 0 0 400000 94 227 1\nend\n3\n0 1 2 300000\nend\n8\n10 20 30 40\nend\n",
		"Event/DropEvent.dat":       "2\n0 14,013 0 0 * * * * * * 0 * * * * * 1000 // bless\nend\n",
		"Event/EventEntryLevel.dat": "1\n0 15 130 10 110\nend\n0\n0 15 80 10 60\n1 81 130 61 110\nend\n",
		"Event/EventStageSpawn.dat": "2\n1 2 3 *\nend\n0\n1 1 89 1000\nend\n",
		"Event/GoldenArcher.dat":    "14,013 1 * * * * * * 0 10\nend\n",
		"Event/InvasionManager.dat": "1\n0 1 2 44 3 600 1 \"Golden\"\n0 5 6 7 8 9 10\n30 x\nend\n3\n0 1 44 30 0 1 1 255 255 10 0 1 2000 1\nend\n",
		"Event/Kalima.dat":          "0 1\n5,0\n9 1\nend\n",
	})
	bc, err := gamedata.LoadBloodCastle(cfg)
	if err != nil || bc.EventTime != 15 || len(bc.Schedule) != 1 || bc.Schedule[0].Hour != 20 || bc.Experience[0] != [2]int{10, 20} ||
		bc.GateLife[0] != 100 || bc.StatueX[0] != 12 {
		t.Fatalf("blood castle = %+v %v", bc, err)
	}
	events, bonuses, err := gamedata.LoadBonusManager(cfg)
	if err != nil || events[0].Name != "Double Exp" || events[0].Time != 3600 || len(events[0].Schedule) != 1 ||
		events[1].Name != "" || events[1].StartMsg != 5 || len(bonuses) != 1 || bonuses[0].Value != [4]int{200, 200, 300, 400} || bonuses[0].Map != -1 {
		t.Fatalf("bonus = %+v %+v %v", events[:2], bonuses, err)
	}
	cs, err := gamedata.LoadCastleSiege(cfg)
	if err != nil || cs.CycleDays != 5 || len(cs.NPCs) != 1 || !cs.NPCs[0].DBSave || cs.GateDefenseUpgrade[0].Money != 300000 ||
		cs.GateDefense != [4]int{10, 20, 30, 40} {
		t.Fatalf("castle siege = %+v %v", cs, err)
	}
	_, items, err := gamedata.LoadDropEvent(cfg)
	if err != nil || len(items) != 1 || items[0].Index != 14*512+13 || items[0].Rate != 1000 || items[0].Map != -1 {
		t.Fatalf("drop event = %+v %v", items, err)
	}
	entry, err := gamedata.LoadEventEntryLevel(cfg)
	if err != nil || len(entry[0]) != 2 || len(entry[1]) != 1 || entry.Level(0, 0, 100) != 1 {
		t.Fatalf("entry levels = %+v %v", entry, err)
	}
	stages, err := gamedata.LoadEventStageSpawn(cfg)
	if err != nil || len(stages) != 2 || stages[0] != (gamedata.StageSpawn{Event: 2, Level: 1, Stage: 2, Monster: 3, RegenTime: -1}) {
		t.Fatalf("stages (file order) = %+v %v", stages, err)
	}
	ga, err := gamedata.LoadGoldenArcher(cfg)
	if err != nil || len(ga) != 1 || ga[0].Index != 14*512+13 || ga[0].Rate != 10 {
		t.Fatalf("golden archer = %+v %v", ga, err)
	}
	inv, err := gamedata.LoadInvasions(cfg)
	if err != nil || inv[0].Name != "Golden" || inv[0].Boss != 7 || inv[0].Time != 9 || len(inv[0].Spawns) != 1 ||
		inv[0].Spawns[0].KillMessage != 1 || inv[0].Spawns[0].Regen != 2000 {
		t.Fatalf("invasions (later settings row keeps the name) = %+v %v", inv[0], err)
	}
	ka, err := gamedata.LoadKalima(cfg)
	if err != nil || ka != [6]int{1, 0, 0, 0, 0, 0} {
		t.Fatalf("kalima = %v %v", ka, err)
	}
	if _, err := tryConvertGroup(t, "event", map[string]string{"Event/CastleSiege.dat": "3\n3 1 2 3\nend\n"}); err == nil {
		t.Fatal("upgrade index 3 accepted")
	}
}

// Fixtures of the original readers (moved from the gamedata loaders).
func TestEventFixtures(t *testing.T) {
	cfg := convertGroup(t, "event", map[string]string{
		"Event/BonusManager.dat": "0\n0 * * * * 20 0 0\nend\n1\n0 5 * 3600 \"Happy Hour\"\nend\n2\n0 0 50 60 70 80 *\n0 3 1000 1000 1000 1000 2\nend\n",
		"Event/ChaosCastle.dat":  "2\n6 1 1\n-1 2 2\n3 7 8\nend\n",
		"Event/DropEvent.dat": "0\n0 * * * * 21 0 0\nend\n1\n0 * * 1800 \"Jewel Rain\"\nend\n2\n" +
			"0 14,013 0 0 * * * * * * 0 2 * 10 * * 5000\nend\n",
		"Event/CastleDeepEvent.dat": "7\n1 2 3\nend\n1\n1 100\nend\n",
		"Event/Kalima.dat":          "// c\n2 1\n6 1\n-1 1\n0 1\nend\n3 1\n",
	})
	ev, bonus, err := gamedata.LoadBonusManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if e := ev[0]; e == nil || len(e.Schedule) != 1 || e.StartMsg != 5 || e.FinalMsg != -1 || e.Time != 3600 || e.Name != "Happy Hour" {
		t.Fatalf("event %+v", ev[0])
	}
	if len(bonus) != 2 || bonus[0].Value[3] != 80 || bonus[0].Map != -1 || bonus[1].Index != 3 || bonus[1].Map != 2 {
		t.Fatalf("bonus %+v", bonus)
	}
	cc, err := gamedata.LoadChaosCastle(cfg)
	if err != nil || cc.Experience[3] != [2]int{7, 8} || cc.Experience[0] != [2]int{} {
		t.Fatalf("invalid levels: %v %v", err, cc)
	}
	dev, items, err := gamedata.LoadDropEvent(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if dev[0] == nil || dev[0].Time != 1800 || len(items) != 1 {
		t.Fatalf("events %+v items %+v", dev[0], items)
	}
	if r := items[0]; r.Index != 14*512+13 || r.Map != 2 || r.Monster != -1 || r.MinLevel != 10 ||
		r.AccountLevel != -1 || r.Rate != 5000 || r.Option != [6]int{-1, -1, -1, -1, -1, -1} {
		t.Fatalf("row %+v", r)
	}
	// Unknown sections are skipped.
	if d, err := gamedata.LoadCastleDeep(cfg); err != nil || len(d.Groups) != 1 || len(d.Schedule) != 0 {
		t.Fatalf("skip: %v %+v", err, d)
	}
	if r, err := gamedata.LoadKalima(cfg); err != nil || r != [gamedata.KalimaLevels]int{1, 0, 1, 0, 0, 0} {
		t.Fatalf("kalima rows %v %v", r, err)
	}
	if _, err := tryConvertGroup(t, "event", map[string]string{"Event/CastleDeepEvent.dat": "3\n1 1 39 15 60\nend\n"}); err == nil {
		t.Fatal("short row accepted")
	}
}
