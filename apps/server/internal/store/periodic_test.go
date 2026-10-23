package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func testPeriodic(t *testing.T, s PeriodicStore) {
	ctx := context.Background()
	a := uint32(time.Now().UnixNano()%1e9) + 1
	b := a + 1
	// Unknown serials are inserted with 0.
	got, err := s.PeriodicTimes(ctx, []uint32{a, b})
	if err != nil || got[a] != 0 || got[b] != 0 || len(got) != 2 {
		t.Fatalf("first load %v %v", got, err)
	}
	if err := s.SavePeriodic(ctx, map[uint32]int64{a: 1_900_000_000}); err != nil {
		t.Fatal(err)
	}
	s.SavePeriodic(ctx, map[uint32]int64{a: 1_900_000_100})
	got, _ = s.PeriodicTimes(ctx, []uint32{a, b})
	if got[a] != 1_900_000_100 || got[b] != 0 {
		t.Fatalf("reload %v", got)
	}
}

func TestMemoryPeriodic(t *testing.T) {
	m, _ := NewMemory()
	testPeriodic(t, m)
}

func TestPostgresPeriodic(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testPeriodic(t, p)
}
