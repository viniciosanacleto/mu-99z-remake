package jsonconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
)

const serverList = `// ---------------------------------------------------------------------
//              [Max 20 Char]    [000.000.000.000]   [1~65535]   [0-1]
// ServerCode   ServerName       IpAddress           Port        Visible
// ---------------------------------------------------------------------
   0            "Servidor 01"     "192.168.1.1"        55900       1
   19           "EventServer"    "192.168.1.1"        55919       0 // trailing
end
`

func TestTableFromTextPlain(t *testing.T) {
	tb, err := TableFromText([]byte(serverList), mutext.Tokenize, Hints{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tb.MarshalJSON()
	want := `{
  "rows": [
    {"ServerCode": 0, "ServerName": "Servidor 01", "IpAddress": "192.168.1.1", "Port": 55900, "Visible": 1},
    {"ServerCode": 19, "ServerName": "EventServer", "IpAddress": "192.168.1.1", "Port": 55919, "Visible": 0}
  ]
}`
	if string(b) != want {
		t.Fatalf("json:\n%s", b)
	}
	back := &Table{}
	if err := json.Unmarshal(b, back); err != nil {
		t.Fatal(err)
	}
	lines, err := back.Lines()
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := mutext.Tokenize([]byte(serverList))
	if !sameTokens(lines, orig) || lines[0].No != 1 || lines[2].No != 3 {
		t.Fatalf("lines %+v", lines)
	}
}

func TestTableFromTextSections(t *testing.T) {
	src := "0\n// [Minutes]   [Minutes]\n// WarningTime   NotifyTime\n   5   1\nEND\n" +
		"1\n// Year Month\n   *   012\n   * 3 extra\n// DEMOSTRATION\n// 1 2\nend\n" +
		"2\n7 1.5\n// Title Line\n-4 x y z\n// A B C D E\n1 2 3\n4 5\n9\n"
	tb, err := TableFromText([]byte(src), mutext.Tokenize, Hints{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Sections) != 3 || *tb.Sections[0].Index != 0 || tb.Section(2) == nil || !tb.Section(2).NoEnd || tb.Section(5) != nil {
		t.Fatalf("sections %+v", tb.Sections)
	}
	r := tb.Section(1).Rows
	if !reflect.DeepEqual(r[0], Row{{"Year", "*", false}, {"Month", "012", false}}) ||
		!reflect.DeepEqual(r[1], Row{{"Year", "*", false}, {"Month", "3", true}, {"Col3", "extra", false}}) {
		t.Fatalf("section 1 rows %+v", r)
	}
	// The header stays in force; a shorter comment (a title) is no header;
	// a longer one is; an earlier header or a hint of the row's width wins.
	r = tb.Section(2).Rows
	if r[0][0].Name != "Year" || r[0][1] != (Cell{"Month", "1.5", false}) || r[1][0] != (Cell{"Year", "-4", true}) ||
		r[1][3].Name != "Col4" || r[2][2].Name != "C" || r[3][1].Name != "Month" || r[4][0].Name != "Year" {
		t.Fatalf("section 2 rows %+v", r)
	}
	tb, err = TableFromText([]byte("x\n"), mutext.Tokenize, Hints{Headers: [][]string{{"X", "Y"}, {"Only"}}})
	if err != nil || tb.Sections[0].Rows[0][0].Name != "Only" || !tb.Sections[0].NoEnd {
		t.Fatalf("hint %+v %v", tb, err)
	}
	b, _ := tb.MarshalJSON()
	back := &Table{}
	if err := json.Unmarshal(b, back); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if !reflect.DeepEqual(back, tb) {
		t.Fatalf("round trip\n%s", b)
	}
}

func TestTableFromTextScript(t *testing.T) {
	src := "; #\tACTIVE\t SYNTAX PLAYER\tPATENTE\n0\n\t1\t \"/irmatamata\"\t\t1\n" +
		"; #\tLEVEL\tWAITMAP\n\t200\t40\nend\n"
	tb, err := TableFromText([]byte(src), mutext.TokenizeScript,
		Hints{Rename: map[string][]string{"ACTIVE SYNTAX PLAYER PATENTE": {"ACTIVE", "SYNTAX_PLAYER", "PATENTE"}}})
	if err != nil {
		t.Fatal(err)
	}
	r := tb.Section(0).Rows
	if r[0][1] != (Cell{"SYNTAX_PLAYER", "/irmatamata", false}) || r[1][1].Name != "WAITMAP" {
		t.Fatalf("rows %+v", r)
	}
}

func TestTableEncoding(t *testing.T) {
	src := []byte("0 \"ser\xe1 fechado\"\nend\n")
	tb, err := TableFromText(src, mutext.Tokenize, Hints{})
	if err != nil {
		t.Fatal(err)
	}
	if tb.Encoding != Windows1252 || tb.Sections[0].Rows[0][1].Value != "será fechado" {
		t.Fatalf("decoded %+v", tb)
	}
	lines, err := tb.Lines()
	if err != nil || lines[0].Tokens[1] != "ser\xe1 fechado" {
		t.Fatalf("encoded %+v %v", lines, err)
	}
	tb.Sections[0].Rows[0][1].Value = "日本"
	if _, err := tb.Lines(); err == nil {
		t.Fatal("unencodable text accepted")
	}
	if tb, _ := TableFromText([]byte("// t\xe1tulo\n0 x\nend\n"), mutext.Tokenize, Hints{}); tb.Encoding != "" {
		t.Fatal("encoding kept for a comment")
	}
	tb.Encoding = "ebcdic"
	if _, err := tb.Lines(); err == nil {
		t.Fatal("unknown encoding accepted")
	}
}

func TestTableJSONErrors(t *testing.T) {
	for _, src := range []string{
		`{"rows": [], "sections": []}`,
		`{"rows": [{"a": true}]}`,
		`{"rows": [{"a": 1, "a": 2}]}`,
		`{"rows": [[1, 2]]}`,
		`{"rows": [`,
	} {
		if err := json.Unmarshal([]byte(src), &Table{}); err == nil {
			t.Errorf("%s accepted", src)
		}
	}
	tb := &Table{}
	if err := json.Unmarshal([]byte(`{"rows": [{"a": 1}], "end": false}`), tb); err != nil || !tb.Sections[0].NoEnd {
		t.Fatalf("end false: %+v %v", tb, err)
	}
	if lines, _ := tb.Lines(); len(lines) != 1 {
		t.Fatalf("lines %+v", lines)
	}
}

func TestLoadAndWriteTable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "List.json")
	tb, _ := TableFromText([]byte(serverList), mutext.Tokenize, Hints{})
	if err := WriteTable(p, tb); err != nil {
		t.Fatal(err)
	}
	lines, err := LoadLines(p)
	if err != nil {
		t.Fatal(err)
	}
	if rows := mutext.Table(lines); len(rows) != 2 || rows[1].Tokens[1] != "EventServer" {
		t.Fatalf("rows %+v", rows)
	}
	if _, err := LoadLines(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file read")
	}
	os.WriteFile(p, []byte("{"), 0o644)
	if _, err := LoadTable(p); err == nil || !strings.Contains(err.Error(), "List.json") {
		t.Fatalf("bad json: %v", err)
	}
}

const iniSrc = `[ConnectServerInfo]
; comment
ConnectServerPortTCP = 44405
Name = "Servidor 01"
MaxPacketPerSecond = 10// x
connectserverporttcp = 1
[Other]
Neg=-5
`

func TestSettingsFromINI(t *testing.T) {
	s, err := SettingsFromINI([]byte(iniSrc))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.MarshalJSON()
	want := `{
  "ConnectServerInfo": {
    "ConnectServerPortTCP": 44405,
    "Name": "Servidor 01",
    "MaxPacketPerSecond": "10// x"
  },
  "Other": {
    "Neg": -5
  }
}`
	if string(b) != want {
		t.Fatalf("json:\n%s", b)
	}
	back := &Settings{}
	if err := json.Unmarshal(b, back); err != nil || !reflect.DeepEqual(back, s) {
		t.Fatalf("round trip %+v %v", back, err)
	}
	f := back.INI()
	if f.Int("connectserverinfo", "ConnectServerPortTCP", 0) != 44405 || f.Int("ConnectServerInfo", "MaxPacketPerSecond", 0) != 10 ||
		f.Int("Other", "Neg", 0) != -5 {
		t.Fatal("ini view")
	}
	if g := back.SectionINI("Other", "GameServerInfo"); g.Int("GameServerInfo", "Neg", 0) != -5 || g.Has("Other", "Neg") {
		t.Fatal("section view")
	}
	if _, err := SettingsFromINI([]byte("[A]\nx=\xe1\n")); err == nil {
		t.Fatal("non UTF-8 ini accepted")
	}
}

func TestSettingsSet(t *testing.T) {
	s := &Settings{}
	s.Set("Event", "CastleSiegeCycleStartYear", "2026")
	s.Set("event", "castlesiegecyclestartyear", "0")
	s.Set("Event", "Name", "x y")
	if v, ok := s.Get("EVENT", "CastleSiegeCycleStartYear"); !ok || v != "0" || len(s.Sections) != 1 || len(s.Sections[0].Keys) != 2 {
		t.Fatalf("set %+v", s)
	}
	if s.Sections[0].Keys[0] != (Cell{"CastleSiegeCycleStartYear", "0", true}) || s.Sections[0].Keys[1].Number {
		t.Fatalf("cells %+v", s.Sections[0].Keys)
	}
	p := filepath.Join(t.TempDir(), "s.json")
	if err := WriteSettings(p, s); err != nil {
		t.Fatal(err)
	}
	f, err := LoadINI(p)
	if err != nil || f.String("Event", "Name", "", 0) != "x y" {
		t.Fatalf("load %v", err)
	}
	for _, src := range []string{`[]`, `{"A": 1}`, `{"A": {}, "a": {}}`} {
		if err := json.Unmarshal([]byte(src), &Settings{}); err == nil {
			t.Errorf("%s accepted", src)
		}
	}
	empty, _ := (&Settings{}).MarshalJSON()
	if string(empty) != "{}" {
		t.Fatalf("empty %s", empty)
	}
}

func TestServerFile(t *testing.T) {
	cs, _ := SettingsFromINI([]byte(iniSrc))
	sl, _ := TableFromText([]byte(serverList), mutext.Tokenize, Hints{})
	bl, _ := TableFromText([]byte("// IpAddress\n\"127.0.0.1\"\nend\n"), mutext.Tokenize, Hints{})
	p := filepath.Join(t.TempDir(), "server.json")
	if err := WriteServer(p, &ServerFile{ConnectServer: cs, ServerList: sl, BlackList: bl}); err != nil {
		t.Fatal(err)
	}
	f, err := LoadServer(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.ConnectServer, cs) || !reflect.DeepEqual(f.ServerList, sl) ||
		f.BlackList.Sections[0].Rows[0][0] != (Cell{"IpAddress", "127.0.0.1", false}) {
		t.Fatalf("server %+v", f)
	}
	if lines, _ := f.BadSyntax.Lines(); len(lines) != 1 || len(f.JoinServer.Sections) != 0 {
		t.Fatalf("defaults %+v %+v", lines, f.JoinServer)
	}
	os.WriteFile(p, []byte(`{"ServerList": 1}`), 0o644)
	if _, err := LoadServer(p); err == nil {
		t.Fatal("bad server.json accepted")
	}
}

func TestWindows1252Helpers(t *testing.T) {
	if FromWindows1252("Jo\xe3o") != "João" || ToWindows1252("João") != "Jo\xe3o" || ToWindows1252("日") != "?" ||
		ToWindows1252("abc") != "abc" || FromWindows1252("abc") != "abc" {
		t.Fatal("conversion")
	}
}

func TestTokenizerForAndIsINI(t *testing.T) {
	if reflect.ValueOf(TokenizerFor("Custom/Event/DuelEvent.txt")).Pointer() != reflect.ValueOf(mutext.TokenizeScript).Pointer() ||
		reflect.ValueOf(TokenizerFor("Custom/CustomPatente.txt")).Pointer() != reflect.ValueOf(mutext.TokenizeScript).Pointer() ||
		reflect.ValueOf(TokenizerFor("Item/Item.txt")).Pointer() != reflect.ValueOf(mutext.Tokenize).Pointer() {
		t.Fatal("tokenizer choice")
	}
	if !IsINI([]byte("; c\n\n[CreationGold]\nGoldValue=20\n")) || IsINI([]byte("// x\n0\nend\n")) || IsINI([]byte("[]\n")) || IsINI(nil) {
		t.Fatal("IsINI")
	}
}
