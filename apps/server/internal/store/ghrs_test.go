package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func testTopResets(t *testing.T, s Store) {
	ctx := context.Background()
	gs := s.(GHRSStore)
	before, err := gs.TopResets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	acc, err := s.CreateAccount(ctx, "ghrs"+suffix, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Top" + suffix
	s.CreateCharacter(ctx, acc.ID, name, 0x10)
	c, err := s.LoadCharacter(ctx, acc.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	c.Resets = before + 7
	if err := s.SaveCharacter(ctx, c); err != nil {
		t.Fatal(err)
	}
	if n, _ := gs.TopResets(ctx); n != before+7 {
		t.Fatalf("top %d want %d", n, before+7)
	}
}

func TestMemoryTopResets(t *testing.T) {
	m, _ := NewMemory()
	testTopResets(t, m)
}

func TestPostgresTopResets(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testTopResets(t, p)
}
