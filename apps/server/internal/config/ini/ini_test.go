package ini

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = "[GameServerInfo]\r\n" +
	"; comment\r\n" +
	"ServerName = \"Principal\"\r\n" +
	"ServerCode=0\r\n" +
	"servercode = 9\r\n" + // duplicate: first wins
	"MaxPacketPerSecond = 10// trailing junk\r\n" +
	"Negative = -15\r\n" +
	"NoDigits = abc\r\n" +
	"garbage line\r\n" +
	"[Other]\r\n" +
	"Key = value with spaces  \r\n"

func TestParse(t *testing.T) {
	f := Parse([]byte(sample))
	if got := f.String("gameserverinfo", "SERVERNAME", "", 0); got != "Principal" {
		t.Fatalf("ServerName = %q", got)
	}
	if got := f.String("GameServerInfo", "ServerName", "", 5); got != "Prin" {
		t.Fatalf("truncated = %q", got)
	}
	if got := f.String("GameServerInfo", "Missing", "def", 0); got != "def" {
		t.Fatalf("default = %q", got)
	}
	if got := f.Int("GameServerInfo", "ServerCode", -1); got != 0 {
		t.Fatalf("ServerCode = %d", got)
	}
	if got := f.Int("GameServerInfo", "MaxPacketPerSecond", 0); got != 10 {
		t.Fatalf("MaxPacketPerSecond = %d", got)
	}
	if got := f.Int("GameServerInfo", "Negative", 0); got != -15 {
		t.Fatalf("Negative = %d", got)
	}
	if got := f.Int("GameServerInfo", "NoDigits", 7); got != 0 {
		t.Fatalf("NoDigits = %d", got)
	}
	if got := f.Int("GameServerInfo", "Missing", 50); got != 50 {
		t.Fatalf("Missing = %d", got)
	}
	if got := f.String("Other", "Key", "", 0); got != "value with spaces" {
		t.Fatalf("Other.Key = %q", got)
	}
	if !f.Has("other", "key") || f.Has("nosection", "key") {
		t.Fatal("Has")
	}
}

func TestLoadConnectServerIni(t *testing.T) {
	// The original 1.ConnectServer/ConnectServer.ini.
	path := filepath.Join(t.TempDir(), "ConnectServer.ini")
	src := "[ConnectServerInfo]\r\n;==================================================\r\n; Connection Settings\r\n" +
		";==================================================\r\nConnectServerPortTCP = 44405\r\nConnectServerPortUDP = 55557\r\n" +
		"MaxConnectionIdle = 60\r\nMaxConnectionPerIP = 1\r\nMaxPacketPerSecond = 10\r\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.ini")); err == nil {
		t.Fatal("missing file read")
	}
	want := map[string]int{
		"ConnectServerPortTCP": 44405,
		"ConnectServerPortUDP": 55557,
		"MaxConnectionIdle":    60,
		"MaxConnectionPerIP":   1,
		"MaxPacketPerSecond":   10,
	}
	for k, v := range want {
		if got := f.Int("ConnectServerInfo", k, -1); got != v {
			t.Errorf("%s = %d, want %d", k, got, v)
		}
	}
}

func TestFromSections(t *testing.T) {
	f := FromSections(map[string]map[string]string{"GameServerInfo": {"ServerName": "Principal", "ServerPort": "55900"}})
	if f.String("gameserverinfo", "SERVERNAME", "", 0x20) != "Principal" || f.Int("GameServerInfo", "ServerPort", 0) != 55900 {
		t.Fatal("case-insensitive lookup")
	}
	if f.String("GameServerInfo", "ServerName", "", 4) != "Pri" || f.Int("GameServerInfo", "Missing", 7) != 7 ||
		!f.Has("GameServerInfo", "serverport") || f.Has("Other", "ServerPort") {
		t.Fatal("GetPrivateProfile semantics")
	}
}
