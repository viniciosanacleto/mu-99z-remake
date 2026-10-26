package game

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/config/gsconfig"
)

// repoConfig is a path under apps/server/config.
func repoConfig(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "config"}, parts...)...)
}

// instanceConfig loads a copy of config/gameserver/<instance>.toml with
// keys of some tables replaced (or appended): table -> key -> integer value.
func instanceConfig(t *testing.T, instance string, tables map[string]map[string]string) *gsconfig.Config {
	t.Helper()
	b, err := os.ReadFile(repoConfig("gameserver", instance+".toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), instance+".toml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := gsconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for table, kv := range tables {
		for k, v := range kv {
			n, err := strconv.Atoi(v)
			if err != nil {
				t.Fatalf("%s.%s: %v", table, k, err)
			}
			if err := cfg.SetInt(table, k, n); err != nil {
				t.Fatal(err)
			}
		}
	}
	if cfg, err = gsconfig.Load(path); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestBadSyntaxFromOptions(t *testing.T) {
	b := newBareWorld(t)
	if b.badSyntax("testblockX") {
		t.Fatal("shipped BadSyntax list is empty")
	}
	b.badWords = []string{"", "gm"}
	if !b.badSyntax("xgmx") || b.badSyntax("Hero") {
		t.Fatal("strstr check")
	}
}

func TestLuaProfilePathBase(t *testing.T) {
	b := newBareWorld(t)
	if got, want := b.luaProfilePath(`Custom\x.ini`), filepath.Join(repoData(), "Custom", "x.ini"); got != want {
		t.Fatalf("relative path = %s, want %s (data root)", got, want)
	}
	abs := filepath.Join(t.TempDir(), "y.ini")
	if b.luaProfilePath(abs) != abs {
		t.Fatal("absolute path changed")
	}
}
