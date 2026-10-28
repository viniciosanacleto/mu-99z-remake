package main

import (
	"testing"

	"github.com/viniciosanacleto/mu-remake/apps/server/internal/game"
	"github.com/viniciosanacleto/mu-remake/apps/server/internal/store"
)

func TestNewEditorUsesFirstInstance(t *testing.T) {
	m, err := store.NewMemory()
	if err != nil {
		t.Fatal(err)
	}
	ed, err := newEditor(&config{configDir: "../../config", instances: " gameserver , nope"}, m, game.NewHub())
	if err != nil {
		t.Fatal(err)
	}
	if ed.Limits().MaxLevel != 400 { // gameserver.toml ServerMaxLevel
		t.Fatalf("max level %d", ed.Limits().MaxLevel)
	}
	if _, err := newEditor(&config{configDir: "../../config", instances: "nope,gameserver"}, m, game.NewHub()); err == nil {
		t.Fatal("missing first instance accepted")
	}
}
