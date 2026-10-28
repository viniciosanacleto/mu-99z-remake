package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testGMStore checks SetGMLevel: the level is loaded with the character,
// the character save does not overwrite it and a missing name reports not
// found.
func testGMStore(t *testing.T, s Store) {
	gs, ok := s.(GMStore)
	if !ok {
		t.Fatal("no GMStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	acc, err := s.CreateAccount(ctx, "gm"+suffix, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Gm" + suffix
	if r, _, err := s.CreateCharacter(ctx, acc.ID, name, 0x10); err != nil || r != CreateOK {
		t.Fatalf("create %v %v", r, err)
	}
	if c, _ := s.LoadCharacter(ctx, acc.ID, name); c == nil || c.GMLevel != 0 {
		t.Fatalf("new character gm level %+v", c)
	}
	c, _ := s.LoadCharacter(ctx, acc.ID, name) // loaded before the change, saved after it
	if found, err := gs.SetGMLevel(ctx, name, 32); err != nil || !found {
		t.Fatalf("set %v %v", found, err)
	}
	if got, _ := s.LoadCharacter(ctx, acc.ID, name); got.GMLevel != 32 {
		t.Fatalf("gm level %d", got.GMLevel)
	}
	c.Level = 2
	if err := s.SaveCharacter(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.LoadCharacter(ctx, acc.ID, name); got.GMLevel != 32 || got.Level != 2 {
		t.Fatalf("after save gm level %d level %d", got.GMLevel, got.Level)
	}
	if found, _ := gs.SetGMLevel(ctx, name, 0); !found {
		t.Fatal("demote")
	}
	if got, _ := s.LoadCharacter(ctx, acc.ID, name); got.GMLevel != 0 {
		t.Fatalf("after demote %d", got.GMLevel)
	}
	if found, err := gs.SetGMLevel(ctx, "Nobody"+suffix, 8); err != nil || found {
		t.Fatalf("missing character %v %v", found, err)
	}
}

func TestMemoryGMStore(t *testing.T) {
	m, _ := NewMemory()
	testGMStore(t, m)
}

func TestPostgresGMStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testGMStore(t, p)
}
