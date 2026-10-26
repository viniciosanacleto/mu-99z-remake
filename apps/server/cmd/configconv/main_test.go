package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

// muServer writes a small original MuServer folder.
func muServer(t *testing.T, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"1.ConnectServer/ConnectServer.ini": "[ConnectServerInfo]\r\nConnectServerPortTCP = 44405\r\nMaxConnectionIdle = 60\r\n",
		"1.ConnectServer/ServerList.dat": "// ServerCode   ServerName       IpAddress           Port        Visible\r\n" +
			"   0            \"Servidor 01\"     \"192.168.1.1\"        55900       1\r\nend\r\n",
		"1.ConnectServer/BlackList.txt": "// IpAddress\r\n\"127.0.0.1\"\r\nend",
		"2.DataServer/BadSyntax.txt":    "// Syntax\r\n// \"testblock\"\r\nend",
		"3.JoinServer/JoinServer.ini":   "[JoinServerInfo]\r\nMaxConnectionPerIP = 2\r\n",
		"Data/Event/BloodCastle.dat": "0\r\n// WarningTime   NotifyTime\r\n   5   1\r\nend\r\n" +
			"1\r\n// StartYear StartMonth\r\n * 0\r\nend\r\n",
		"Data/Lang/Por/Message.txt":       "// Index Message\r\n0 \"ser\xe1 fechado\"\r\nend\r\n",
		"Data/Custom/CreationGold.txt":    "; Bonus\r\n[CreationGold]\r\nGoldValue=20\r\n",
		"Data/Custom/Event/DuelEvent.txt": "; #\tACTIVE\t SYNTAX PLAYER\tPATENTE\r\n0\r\n\t1\t\"/irmatamata\"\t1\r\nend\r\n",
		"Data/Item/ItemDrop.txt":          "   12,015      0           0            *         *         *         *         *         *         0          *          *              12                150               500            940            360            20\r\n",
		"Data/Item/test_item.txt":         "notes",
		"Data/Hack/Enc2.dat":              "\x00\x01binary",
		"Data/Terrain/Terrain1.att":       "\x00binary",
	}
	for _, name := range gsconfig.Files {
		files["4.GameServer/DATA/GameServerInfo - "+name+".dat"] = "[GameServerInfo]\r\n" + name + "Key = 1\r\n"
	}
	files["4.GameServer/DATA/GameServerInfo - Common.dat"] = "[GameServerInfo]\r\nServerName = \"Principal\"\r\nServerPort = 55900\r\n"
	for k, v := range extra {
		files[k] = v
	}
	dir := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestConvert(t *testing.T) {
	from := muServer(t, nil)
	to := filepath.Join(t.TempDir(), "config")
	r, err := convert(from, to)
	if err != nil {
		t.Fatal(err)
	}
	// server.json, one instance, 5 Data files (no note, no binary).
	if r.files != 7 {
		t.Fatalf("files = %d", r.files)
	}
	for _, rel := range []string{"data/Item/test_item.json", "data/Hack/Enc2.json", "data/Terrain/Terrain1.json"} {
		if _, err := os.Stat(filepath.Join(to, rel)); !os.IsNotExist(err) {
			t.Errorf("%s written", rel)
		}
	}

	sf, err := jsonconf.LoadServer(filepath.Join(to, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := sf.ConnectServer.Get("ConnectServerInfo", "ConnectServerPortTCP"); v != "44405" {
		t.Fatalf("connect %+v", sf.ConnectServer)
	}
	if sf.ServerList.Sections[0].Rows[0][1] != jsonconf.NewCell("ServerName", "Servidor 01") ||
		sf.BlackList.Sections[0].Rows[0][0] != jsonconf.NewCell("IpAddress", "127.0.0.1") || len(sf.BadSyntax.Sections[0].Rows) != 0 {
		t.Fatalf("tables %+v %+v", sf.ServerList, sf.BlackList)
	}
	if sf.JoinServer.INI().Int("JoinServerInfo", "MaxConnectionPerIP", 0) != 2 {
		t.Fatal("join server")
	}

	gs, err := gsconfig.Load(filepath.Join(to, "gameservers", "4.GameServer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if gs.ServerName != "Principal" || gs.ServerPort != 55900 || gs.Int("Skill", "SkillKey", 0) != 1 {
		t.Fatalf("instance %+v", gs)
	}

	bc, err := jsonconf.LoadTable(filepath.Join(to, "data", "Event", "BloodCastle.json"))
	if err != nil || bc.Section(1).Rows[0][0] != jsonconf.NewCell("StartYear", "*") {
		t.Fatalf("blood castle %+v %v", bc, err)
	}
	msg, err := jsonconf.LoadLines(filepath.Join(to, "data", "Lang", "Por", "Message.json"))
	if err != nil || msg[0].Tokens[1] != "ser\xe1 fechado" {
		t.Fatalf("message %+v %v", msg, err)
	}
	gold, err := jsonconf.LoadINI(filepath.Join(to, "data", "Custom", "CreationGold.json"))
	if err != nil || gold.Int("CreationGold", "GoldValue", 0) != 20 {
		t.Fatalf("creation gold %v", err)
	}
	duel, err := jsonconf.LoadTable(filepath.Join(to, "data", "Custom", "Event", "DuelEvent.json"))
	if err != nil || duel.Section(0).Rows[0][1] != jsonconf.NewCell("SYNTAX_PLAYER", "/irmatamata") {
		t.Fatalf("duel %+v %v", duel, err)
	}
	drop, err := jsonconf.LoadTable(filepath.Join(to, "data", "Item", "ItemDrop.json"))
	if err != nil || drop.Sections[0].Rows[0][0] != jsonconf.NewCell("ItemIndex", "12,015") ||
		drop.Sections[0].Rows[0][17].Name != "DropRate_AL3" || !drop.Sections[0].NoEnd {
		t.Fatalf("item drop %+v %v", drop, err)
	}
	lines, _ := drop.Lines()
	if rows := mutext.Table(lines); len(rows) != 1 || len(rows[0].Tokens) != 18 {
		t.Fatalf("item drop lines %+v", lines)
	}
	if len(r.unnamed) != 0 {
		t.Fatalf("unnamed %v", r.unnamed)
	}
}

func TestConvertErrors(t *testing.T) {
	for name, extra := range map[string]map[string]string{
		"missing ConnectServer.ini": {"1.ConnectServer/ConnectServer.ini": ""},
		"broken table":              {"Data/Item/Item.txt": "0 \"open\nend\n"},
		"non UTF-8 ini":             {"Data/Custom/CustomGHRS.ini": "[WEEK]\nMonday=\xe1\n"},
		"second ini section":        {"4.GameServer/DATA/GameServerInfo - Item.dat": "[Other]\nX=1\n"},
	} {
		from := muServer(t, extra)
		if name == "missing ConnectServer.ini" {
			os.Remove(filepath.Join(from, "1.ConnectServer", "ConnectServer.ini"))
		}
		if _, err := convert(from, t.TempDir()); err == nil {
			t.Errorf("%s: converted", name)
		}
	}
}

func TestUnnamedReport(t *testing.T) {
	from := muServer(t, map[string]string{"Data/Util/Table.txt": "1 2 3\nend\n"})
	r, err := convert(from, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.unnamed) != 1 || !strings.Contains(r.unnamed[0], "Col1=1 Col2=2 Col3=3") {
		t.Fatalf("unnamed %v", r.unnamed)
	}
}
