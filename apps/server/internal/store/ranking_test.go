package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func testRanking(t *testing.T, s RankingStore) {
	ctx := context.Background()
	name := fmt.Sprintf("R%d", time.Now().UnixNano()%1e9)
	if w, l, err := s.DuelRanking(ctx, name); err != nil || w != 0 || l != 0 {
		t.Fatalf("empty duel %d %d %v", w, l, err)
	}
	s.AddDuelRanking(ctx, name, 1, 0)
	s.AddDuelRanking(ctx, name, 0, 1)
	s.AddDuelRanking(ctx, name, 1, 0)
	if w, l, _ := s.DuelRanking(ctx, name); w != 2 || l != 1 {
		t.Fatalf("duel %d %d", w, l)
	}
	s.AddEventRanking(ctx, RankingBloodCastle, name, 100)
	s.AddEventRanking(ctx, RankingBloodCastle, name, 50)
	s.AddEventRanking(ctx, RankingChaosCastle, name, 7)
	if n, _ := s.EventRanking(ctx, RankingBloodCastle, name); n != 150 {
		t.Fatalf("bc %d", n)
	}
	// Names compare without case, events are separate rows.
	if n, _ := s.EventRanking(ctx, RankingChaosCastle, "r"+name[1:]); n != 7 {
		t.Fatalf("cc %d", n)
	}
	if n, _ := s.EventRanking(ctx, RankingDevilSquare, name); n != 0 {
		t.Fatalf("ds %d", n)
	}
}

func TestMemoryRanking(t *testing.T) {
	m, _ := NewMemory()
	testRanking(t, m)
}

func TestPostgresRanking(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testRanking(t, p)
}
