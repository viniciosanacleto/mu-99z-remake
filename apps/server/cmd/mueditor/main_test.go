package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func serverConfig() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "config")
}

func TestParseFlags(t *testing.T) {
	t.Setenv("MU_DATABASE_URL", "")
	t.Setenv("MU_EDITOR_PASSWORD", "")
	if _, err := parseFlags(nil); err == nil {
		t.Fatal("missing -db accepted")
	}
	c, err := parseFlags([]string{"-db", "postgres://x", "-password", "p"})
	if err != nil {
		t.Fatal(err)
	}
	if c.addr != "127.0.0.1:8080" || c.password != "p" || c.instance != "gameserver" || c.configDir != "config" {
		t.Fatalf("config = %+v", c)
	}
}

func TestNewEditorLoadsServerData(t *testing.T) {
	m, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	ed, err := newEditor(serverConfig(), "gameserver", m)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(ed.Catalog().Items); n < 300 {
		t.Fatalf("catalog has %d items", n)
	}
	// gameserver.toml [Common]: ServerMaxLevel 400, MaxStatPoint_AL0..3 32767,
	// ExperienceMultiplierConstA/B 1 (exp[1] = (1+9)*1*1*1).
	lim := ed.Limits()
	if lim.MaxLevel != 400 || lim.MaxStat != [4]int{32767, 32767, 32767, 32767} || lim.Exp == nil || lim.Exp.Floor(2) != 10 {
		t.Fatalf("limits = %d %v %v", lim.MaxLevel, lim.MaxStat, lim.Exp != nil)
	}
	if _, err := newEditor(serverConfig(), "nope", m); err == nil {
		t.Fatal("missing instance accepted")
	}
}
