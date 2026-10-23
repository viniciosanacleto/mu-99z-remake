package gsconfig

import (
	"path/filepath"
	"runtime"
	"testing"
)

func dataDir(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "..", "data"}, parts...)...)
}

func TestLoadOriginalInstances(t *testing.T) {
	c, err := Load(dataDir("4.GameServer", "DATA"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerName != "Principal" || c.ServerCode != 0 || c.ServerPort != 55900 ||
		c.ServerVersion != [5]byte{'1', '0', '0', '0', '0'} || c.ServerSerial != "A9kT3wX5rBz7fQ1u" ||
		c.CustomerName != "WizardTeam" || c.MaxUsers != 100 || c.MaxConnectionPerIP != 3 {
		t.Fatalf("common = %+v", c)
	}
	if c.ALInt("Common", "MGCreateLevel", 0, -1) != c.MGCreateLevel[0] {
		t.Fatal("ALInt mismatch")
	}

	cs, err := Load(dataDir("5.GameServerCS", "DATA"))
	if err != nil {
		t.Fatal(err)
	}
	if cs.ServerName != "Castle Siege" || cs.ServerCode != 19 || cs.ServerPort != 55919 || cs.ServerType != 1 {
		t.Fatalf("cs = %+v", cs)
	}
	if !Exists(dataDir("4.GameServer", "DATA")) || Exists(t.TempDir()) {
		t.Fatal("Exists")
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("expected error for empty dir")
	}
}
