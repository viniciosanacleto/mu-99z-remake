package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// testCustomEventStore checks DS F3 E7 (one holder per patente value) and
// DS 0x41 (ranking + n).
func testCustomEventStore(t *testing.T, s Store) {
	cs, ok := s.(CustomEventStore)
	if !ok {
		t.Fatal("no CustomEventStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	acc, err := s.CreateAccount(ctx, "pat"+suffix, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	a, b := "PatA"+suffix, "PatB"+suffix
	for i, n := range []string{a, b} {
		if r, _, err := s.CreateCharacter(ctx, acc.ID, n, byte(i)<<4); err != nil || r != CreateOK {
			t.Fatalf("create %v %v", r, err)
		}
	}
	patente := func(name string) int {
		c, err := s.LoadCharacter(ctx, acc.ID, name)
		if err != nil {
			t.Fatal(err)
		}
		return c.Patente
	}
	if err := cs.SetPatente(ctx, a, 9); err != nil || patente(a) != 9 {
		t.Fatalf("set %v %d", err, patente(a))
	}
	if err := cs.SetPatente(ctx, b, 9); err != nil || patente(b) != 9 || patente(a) != 0 {
		t.Fatalf("move %v %d %d", err, patente(a), patente(b))
	}
	if err := cs.SetPatente(ctx, b, 0); err != nil || patente(b) != 9 {
		t.Fatal("value 0 written")
	}
	for i := 0; i < 2; i++ {
		if err := cs.AddCustomRank(ctx, a, "TopDeath", 1); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := cs.CustomRank(ctx, a, "TopDeath"); err != nil || n != 2 {
		t.Fatalf("rank %d %v", n, err)
	}
	if n, _ := cs.CustomRank(ctx, a, "TopTheft"); n != 0 {
		t.Fatalf("other rank %d", n)
	}
}

func TestMemoryCustomEventStore(t *testing.T) {
	m, _ := NewMemory()
	testCustomEventStore(t, m)
}

func TestPostgresCustomEventStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testCustomEventStore(t, p)
}
