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

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.ini")
	if err := os.WriteFile(path, []byte("[ConnectServerInfo]\r\nConnectServerPortTCP = 44405\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Int("connectserverinfo", "connectserverporttcp", -1) != 44405 {
		t.Fatal("value")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.ini")); err == nil {
		t.Fatal("missing file accepted")
	}
}
