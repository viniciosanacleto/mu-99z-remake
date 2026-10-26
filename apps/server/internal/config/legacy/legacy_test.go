package legacy

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/mutext"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/servercfg"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/tomlconf"
)

func TestINIToTOML(t *testing.T) {
	src := "[GameServerInfo]\r\n" +
		";==== header\r\n" +
		"ServerName = \"Servidor 01\"\r\n" +
		"ServerCode=7\r\n" +
		"Rate = 10// per cent\r\n" +
		"Version = 1.00.00\r\n" +
		"Url = http://x/y\r\n" +
		"servercode = 9\r\n" +
		"\r\n" +
		"garbage line\r\n" +
		"Name = Voc\xea\r\n"
	got, err := INIToTOML([]byte(src), map[string]string{"gameserverinfo": "Common"})
	if err != nil {
		t.Fatal(err)
	}
	want := "[Common]\n" +
		"# ==== header\n" +
		"ServerName = \"Servidor 01\"\n" +
		"ServerCode = 7\n" +
		"Rate = 10 # per cent\n" +
		"Version = \"1.00.00\"\n" +
		"Url = \"http://x/y\"\n" +
		"# duplicate, ignored by the original: servercode = 9\n" +
		"\n" +
		"# garbage line\n" +
		"Name = \"Você\"\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if _, err := INIToTOML([]byte("[Other]\nA=1\n"), map[string]string{"x": "y"}); err == nil {
		t.Fatal("unexpected section accepted")
	}
}

type sampleRow struct {
	Code  int
	Name  string
	Skip  int `legacy:"-"`
	Pair  [2]int
	Inner struct{ A, B int }
	Rest  []int
}

func TestRow(t *testing.T) {
	lines, err := mutext.Tokenize([]byte("1 \"Voc\xea\" 2 * 4 5 6 7\n9 x 1\n1 2 3\nend\n"))
	if err != nil {
		t.Fatal(err)
	}
	var r sampleRow
	if err := Row(lines[0], &r, false); err != nil {
		t.Fatal(err)
	}
	want := sampleRow{Code: 1, Name: "Você", Pair: [2]int{2, -1}, Inner: struct{ A, B int }{4, 5}, Rest: []int{6, 7}}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("got %+v", r)
	}
	if err := Row(lines[1], &sampleRow{}, false); err == nil {
		t.Fatal("short row accepted")
	}
	type two struct{ A, B int }
	if err := Row(lines[2], &two{}, false); err == nil || !strings.Contains(err.Error(), "columns") {
		t.Fatalf("long row: %v", err)
	}
	if _, err := Table[two](lines, true); err == nil {
		t.Fatal("non-number accepted")
	}
	rows, err := Table[two](lines[2:], true)
	if err != nil || len(rows) != 1 || rows[0] != (two{1, 2}) {
		t.Fatalf("loose rows = %+v %v", rows, err)
	}
}

// writeTree creates files under a temporary original-layout folder.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestServerRoundTrip(t *testing.T) {
	root := writeTree(t, map[string]string{
		"1.ConnectServer/ConnectServer.ini": "[ConnectServerInfo]\nConnectServerPortTCP = 44405\nMaxConnectionIdle = 30\nMaxConnectionPerIP = 1\nMaxPacketPerSecond = 10",
		"1.ConnectServer/BlackList.txt":     "// ip\n\"127.0.0.1\"\n\"10.0.0.1\"\nend\n",
		"1.ConnectServer/ServerList.dat":    "0 \"Servidor 01\" \"192.168.1.1\" 55900 1\n19 \"Event\" \"192.168.1.1\" 55919 0\nend\n",
		"3.JoinServer/JoinServer.ini":       "[JoinServerInfo]\nJoinServerODBC = MuOnline\nMaxConnectionPerIP = 2\nMaxConnectionPerHID = 3\n",
		"2.DataServer/BadSyntax.txt":        "// Syntax\n\"testblock\"\n\"gm\"\nend\n",
	})
	b, err := Server(root)
	if err != nil {
		t.Fatal(err)
	}
	var f servercfg.File
	if err := tomlconf.Unmarshal(b, &f); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	want := servercfg.File{
		ConnectServer: servercfg.ConnectServer{ConnectServerPortTCP: 44405, MaxConnectionIdle: 30, MaxConnectionPerIP: 1,
			MaxPacketPerSecond: 10, BlackList: []string{"127.0.0.1", "10.0.0.1"},
			ServerList: []servercfg.ServerRow{
				{ServerCode: 0, ServerName: "Servidor 01", IpAddress: "192.168.1.1", Port: 55900, Visible: 1},
				{ServerCode: 19, ServerName: "Event", IpAddress: "192.168.1.1", Port: 55919}}},
		JoinServer: servercfg.JoinServer{JoinServerODBC: "MuOnline", MaxConnectionPerIP: 2, MaxConnectionPerHID: 3},
		DataServer: servercfg.DataServer{BadSyntax: []string{"testblock", "gm"}},
	}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v\nfile:\n%s", f, b)
	}
}

func TestGameServerRoundTrip(t *testing.T) {
	files := map[string]string{}
	for _, name := range gsconfig.Files {
		files["DATA/GameServerInfo - "+name+".dat"] = "[GameServerInfo]\n" + name + "Key = 5\n"
	}
	files["DATA/GameServerInfo - Common.dat"] = "[GameServerInfo]\nServerName = Principal\nServerPort = 55900\n"
	root := writeTree(t, files)
	b, err := GameServer(filepath.Join(root, "DATA"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := gsconfig.Parse(b)
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if c.ServerName != "Principal" || c.ServerPort != 55900 || c.Int("Skill", "SkillKey", 0) != 5 {
		t.Fatalf("config = %+v", c)
	}
	delete(files, "DATA/GameServerInfo - Skill.dat")
	if _, err := GameServer(filepath.Join(writeTree(t, files), "DATA")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestConvert(t *testing.T) {
	root := writeTree(t, map[string]string{
		"1.ConnectServer/ConnectServer.ini": "[ConnectServerInfo]\n",
		"1.ConnectServer/BlackList.txt":     "end\n",
		"1.ConnectServer/ServerList.dat":    "end\n",
		"3.JoinServer/JoinServer.ini":       "[JoinServerInfo]\n",
		"2.DataServer/BadSyntax.txt":        "end\n",
	})
	dst := t.TempDir()
	written, err := Convert(root, dst, map[string]bool{"server": true})
	if err != nil || len(written) != 1 || written[0] != "server.toml" {
		t.Fatalf("written %v, %v", written, err)
	}
	if _, err := servercfg.Load(filepath.Join(dst, "server.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Convert(root, dst, map[string]bool{"gameserver": true}); err == nil {
		t.Fatal("missing instance folders accepted")
	}
}
