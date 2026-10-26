package gsconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func configPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "..", "config"}, parts...)...)
}

func TestLoadShippedInstances(t *testing.T) {
	c, err := Load(configPath("gameserver", "gameserver.toml"))
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

	cs, err := Load(configPath("gameserver", "gameserver-cs.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cs.ServerName != "Castle Siege" || cs.ServerCode != 19 || cs.ServerPort != 55919 || cs.ServerType != 1 {
		t.Fatalf("cs = %+v", cs)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("expected error for a missing file")
	}
}

const minimal = `[Common]
ServerName = "A very long server name that does not fit"
ServerCode = 3
Number = "10// trailing"
[Character]
[ChaosMix]
[Command]
[Custom]
[Event]
Msg = "Você"
[Item]
[Skill]
`

func TestParseSemantics(t *testing.T) {
	c, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerName != "A very long server name that do" || len(c.ServerName) != 0x1f {
		t.Fatalf("ServerName cut = %q", c.ServerName)
	}
	if c.Int("common", "SERVERCODE", -1) != 3 || c.String("Common", "ServerCode", 0) != "3" {
		t.Fatal("case-insensitive lookup / int as string")
	}
	if c.Int("Common", "Number", 0) != 10 || c.Int("Common", "Missing", 7) != 7 || c.String("Common", "Missing", 0) != "" {
		t.Fatal("GetPrivateProfileIntA semantics")
	}
	if c.String("Event", "Msg", 0) != "Voc\xea" {
		t.Fatalf("strings are Windows-1252: %q", c.String("Event", "Msg", 0))
	}
	if _, err := Parse([]byte("[Common]\n")); err == nil || !strings.Contains(err.Error(), "Character") {
		t.Fatalf("missing table: %v", err)
	}
	if _, err := Parse([]byte(minimal + "[Skill2]\nX = 1.5\n")); err == nil {
		t.Fatal("float value accepted")
	}
}

func TestProfileInt(t *testing.T) {
	for in, want := range map[string]int{"10": 10, "-5x": -5, "+7": 7, "abc": 0, " 42 ; c": 42, "": 0} {
		if got := ProfileInt(in); got != want {
			t.Errorf("ProfileInt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestSetInt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gs.toml")
	src := strings.Replace(minimal, "[Event]\n", "[Event]\n# keep me\r\nCastleSiegeState = 5\r\n", 1)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetInt("Event", "castlesiegestate", 0); err != nil {
		t.Fatal(err)
	}
	if err := c.SetInt("Event", "NewKey", 9); err != nil {
		t.Fatal(err)
	}
	if c.Int("Event", "CastleSiegeState", -1) != 0 || c.Int("Event", "NewKey", -1) != 9 {
		t.Fatal("in-memory values not updated")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "# keep me\r\nCastleSiegeState = 0\r\n") {
		t.Fatalf("file:\n%s", b)
	}
	re, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if re.Int("Event", "CastleSiegeState", -1) != 0 || re.Int("Event", "NewKey", -1) != 9 || re.String("Event", "Msg", 0) != "Voc\xea" {
		t.Fatalf("reloaded file:\n%s", b)
	}
	if strings.Index(string(b), "NewKey") > strings.Index(string(b), "[Item]") {
		t.Fatalf("NewKey written outside [Event]:\n%s", b)
	}
}
