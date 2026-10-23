package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func testGoldenArcher(t *testing.T, s GoldenArcherStore) {
	ctx := context.Background()
	acc := "ga" + time.Now().Format("150405.000000")
	if n, err := s.RenaCount(ctx, acc); err != nil || n != 0 {
		t.Fatalf("new %d %v", n, err)
	}
	if ok, _ := s.TakeRena(ctx, acc, 10); ok {
		t.Fatal("took from an empty count")
	}
	for i := 0; i < 12; i++ {
		s.AddRena(ctx, acc, 1)
	}
	if ok, _ := s.TakeRena(ctx, acc, 10); !ok {
		t.Fatal("could not take 10 of 12")
	}
	if n, _ := s.RenaCount(ctx, acc); n != 2 {
		t.Fatalf("left %d", n)
	}
}

func TestMemoryGoldenArcher(t *testing.T) {
	m, _ := NewMemory()
	testGoldenArcher(t, m)
}

func TestPostgresGoldenArcher(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testGoldenArcher(t, p)
}
