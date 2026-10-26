package legacy

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/gamedata"
)

func TestMoveGroup(t *testing.T) {
	cfg := convertGroup(t, "move", map[string]string{
		"Move/Gate.txt": "1 1 0 121 232 123 233 2 0 20 * *\n2 2 1 107 247 110 247 0 1 20 * * 5\n3 1 1 1 1 2 2 4 0 0 * * * 2\nend\n",
		"Move/Move.txt": "1 \"Arena\" 2000 50 * * * 1 1 0 1 50 // pvp\nend\n",
		"Move/MoveSummon.txt": "0 1 1 255 255 1 * * * 1 1 1 1\n" +
			"6 1 1\n255 255 1 * * * 1 1 1 1\nend\n",
	})
	gates, err := gamedata.LoadGates(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if g := gates[1]; g.MaxReset != -1 || g.AccountLvl != -1 || g.TargetGate != 2 || g.MinLevel != 20 || g.MaxLevel != -1 {
		t.Fatalf("12-column gate = %+v", g)
	}
	if gates[2].MaxReset != 5 || gates[2].AccountLvl != -1 || gates[3].AccountLvl != 2 {
		t.Fatalf("optional columns = %+v %+v", gates[2], gates[3])
	}
	f, err := gamedata.LoadMoveConfig(cfg)
	if err != nil || len(f.Move.Rows) != 1 || f.Move.Rows[0].AL2 != 0 || f.Move.Rows[0].GateNumber != 50 || f.Move.Rows[0].MaxLevel != -1 {
		t.Fatalf("move = %+v %v", f, err)
	}
	ms, err := gamedata.LoadMoveSummon(cfg)
	if err != nil || len(ms) != 2 || ms[1].Map != 6 || ms[1].EndX != 255 || ms[1].AL != [4]int{1, 1, 1, 1} {
		t.Fatalf("move summon (token stream) = %+v %v", ms, err)
	}
	if _, err := tryConvertGroup(t, "move", map[string]string{"Move/Gate.txt": "1 1 0 1 1 1 1 0 0 0 *\nend\n"}); err == nil {
		t.Fatal("11-column gate accepted")
	}
	if _, err := tryConvertGroup(t, "move", map[string]string{"Move/Move.txt": "1 \"Arena\" 2000 50 * * * 1 1 0 1\nend\n"}); err == nil {
		t.Fatal("short Move row accepted")
	}
}

func TestCommonGroup(t *testing.T) {
	cfg := convertGroup(t, "common", map[string]string{
		"MapManager.txt":    "0 * 12 100 100 100 1000 0 1 1 * * * 1 1 1 1 * * \"Lorencia\"\nend\n",
		"MapServerInfo.txt": "0\n0 0 1 \"192.168.1.1\" 55900\nx 0 0 \"10.0.0.1\" 55919\nend\n1\n0 0 30 19\n19 1 30 *\nend\n",
		"MapRateInfo.txt":   "* 0 0 0 0 0 * * // Drop Common\n*,1 1,1 1 1 1 *\nend\n",
	})
	maps, err := gamedata.LoadMapManager(cfg)
	if err != nil || maps[0].Name != "Lorencia" || maps[0].NonPK != -1 || maps[0].ViewRange != 12 || maps[0].RespawnGate != -1 {
		t.Fatalf("maps = %+v %v", maps, err)
	}
	ms, err := gamedata.LoadMapServer(cfg)
	if err != nil || len(ms.Servers) != 2 || ms.Servers[1].Code != 0 || ms.Servers[1].Address != "10.0.0.1" ||
		ms.ServerForMap(0, 30) != 19 || ms.ServerForMap(19, 30) != 19 || len(ms.Moves) != 2 {
		t.Fatalf("map server = %+v %v", ms, err)
	}
	rates, err := gamedata.LoadMapRateInfo(cfg)
	if err != nil || len(rates) != 2 || rates[0].Option != [6]int{0, 0, 0, 0, -1, -1} ||
		rates[1] != (gamedata.MapRateInfo{Map: -1, DropType: 1, Option: [6]int{1, 1, 1, 1, 1, -1}}) {
		t.Fatalf("rates = %+v %v", rates, err)
	}
}

func TestCommonRootFiles(t *testing.T) {
	cfg := convertGroup(t, "common", map[string]string{
		// "*" as ReqItemIndex is a single token; an item reference is two.
		"CommandRequirement.txt": `
1 * 10 * * * 0 * 1 0 * * * * * * 0 1 1 255 255 * * * 50 1 1 1 1 1
2 3 * * * * 1 14,013 5 0 * * * * * * * * * * * 6 18 22 51 1 0 0 0 0
end`,
		"BlackList.txt":      "0\n\"10.0.0.1\"\nend\n1\n\"ABCDEF\"\nend",
		"CommandManager.txt": "0 1 1 1 1 \"/move\" 0 * * * * * 0 0 0 0\nend\n",
		"ConnectMember.txt":  "\"admin\"\nend\n",
	})
	rows, err := gamedata.LoadCommandRequirement(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	r := rows[0]
	if r.Index != 1 || r.AccountLevel != -1 || r.MinLevel != 10 || r.ReqItem != -1 || r.ReqItemCount != 1 ||
		r.EndY != 255 || r.FailMessage != 50 || r.Class != [5]int{1, 1, 1, 1, 1} {
		t.Errorf("row 0 = %+v", r)
	}
	r = rows[1]
	if r.ReqItem != 14*512+13 || r.ReqItemCount != 5 || r.DayOfWeek != 6 || r.MinHour != 18 || r.MaxHour != 22 ||
		r.FailMessage != 51 || r.Class != [5]int{1, 0, 0, 0, 0} {
		t.Errorf("row 1 = %+v", r)
	}
	b, err := gamedata.LoadBlackList(cfg)
	if err != nil || len(b.IPs) != 1 || b.IPs[0] != "10.0.0.1" || len(b.HWIDs) != 1 || b.HWIDs[0] != "ABCDEF" {
		t.Fatalf("blacklist = %+v %v", b, err)
	}
	f, err := gamedata.LoadCommonConfig(cfg)
	if err != nil || len(f.CommandManager.Rows) != 1 || f.CommandManager.Rows[0].Syntax != "/move" || f.CommandManager.Rows[0].GMLevel != -1 ||
		len(f.ConnectMember.Account) != 1 || f.ConnectMember.Account[0] != "admin" {
		t.Fatalf("commands / members = %+v %v", f, err)
	}
	if _, err := tryConvertGroup(t, "common", map[string]string{"CommandManager.txt": "0 1 1 1 1 \"/move\" 0\nend\n"}); err == nil {
		t.Fatal("short command row accepted")
	}
}
