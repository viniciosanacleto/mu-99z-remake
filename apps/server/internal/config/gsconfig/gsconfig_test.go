package gsconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/jsonconf"
)

func configPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "..", "config"}, parts...)...)
}

func TestLoadOriginalInstances(t *testing.T) {
	c, err := Load(configPath("gameservers", "4.GameServer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerName != "Principal" || c.ServerCode != 0 || c.ServerPort != 55900 ||
		c.ServerVersion != [5]byte{'1', '0', '0', '0', '0'} || c.ServerSerial != "A9kT3wX5rBz7fQ1u" ||
		c.CustomerName != "WizardTeam" || c.MaxUsers != 100 || c.MaxConnectionPerIP != 3 || c.Name != "4.GameServer" {
		t.Fatalf("common = %+v", c)
	}
	if c.ALInt("Common", "MGCreateLevel", 0, -1) != c.MGCreateLevel[0] {
		t.Fatal("ALInt mismatch")
	}

	cs, err := Load(configPath("gameservers", "5.GameServerCS.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cs.ServerName != "Castle Siege" || cs.ServerCode != 19 || cs.ServerPort != 55919 || cs.ServerType != 1 {
		t.Fatalf("cs = %+v", cs)
	}
	if !Exists(configPath("gameservers", "4.GameServer.json")) || Exists(t.TempDir()) {
		t.Fatal("Exists")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "x.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestMissingSectionAndWriteKeys(t *testing.T) {
	s, err := jsonconf.LoadSettings(configPath("gameservers", "5.GameServerCS.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cs.json")
	if err := jsonconf.WriteSettings(path, s); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteKeys("Event", map[string]string{"CastleSiegeSpecificState": "0", "NewKey": "7"}); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Int("Event", "CastleSiegeSpecificState", -1) != 0 || back.Int("Event", "NewKey", 0) != 7 ||
		back.ServerName != "Castle Siege" {
		t.Fatal("WriteKeys")
	}
	s.Sections = s.Sections[:len(s.Sections)-1]
	if _, err := FromSettings(s, path); err == nil {
		t.Fatal("missing section accepted")
	}
	os.Remove(path)
	if err := c.WriteKeys("Event", map[string]string{"X": "1"}); err == nil {
		t.Fatal("write to a missing file")
	}
}
