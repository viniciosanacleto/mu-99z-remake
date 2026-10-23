package store

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"
)

// testCustomWorldStore checks DS 0x50 (kill count + 1), the DailyReward
// days of a month with DS 0x68 (no duplicates) and DS F7 06 (gold +=).
func testCustomWorldStore(t *testing.T, s Store) {
	cs, ok := s.(CustomWorldStore)
	if !ok {
		t.Fatal("no CustomWorldStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	name := "Kc" + suffix
	for want := 1; want <= 3; want++ {
		if n, err := cs.AddMonsterKill(ctx, name, 3); err != nil || n != want {
			t.Fatalf("kill %d %v", n, err)
		}
	}
	if n, err := cs.AddMonsterKill(ctx, "kC"+suffix, 4); err != nil || n != 1 {
		t.Fatalf("other class %d %v", n, err)
	}
	if n, _ := cs.AddMonsterKill(ctx, "KC"+suffix, 3); n != 4 {
		t.Fatalf("name case %d", n)
	}

	acc := "dr" + suffix
	for _, d := range []int{5, 1, 5} {
		if err := cs.AddDailyReward(ctx, acc, "Hero", d, 10); err != nil {
			t.Fatal(err)
		}
	}
	if err := cs.AddDailyReward(ctx, acc, "Hero", 7, 9); err != nil {
		t.Fatal(err)
	}
	if days, err := cs.DailyRewardDays(ctx, acc, 10); err != nil || !slices.Equal(days, []int{1, 5}) {
		t.Fatalf("days %v %v", days, err)
	}
	if days, _ := cs.DailyRewardDays(ctx, "DR"+suffix, 9); !slices.Equal(days, []int{7}) {
		t.Fatalf("september %v", days)
	}
	if days, _ := cs.DailyRewardDays(ctx, "none"+suffix, 10); len(days) != 0 {
		t.Fatalf("unknown account %v", days)
	}

	login := "gd" + suffix
	if _, err := s.CreateAccount(ctx, login, "plain:x", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := cs.AddGold(ctx, login, 20); err != nil {
			t.Fatal(err)
		}
	}
	if g, err := cs.Gold(ctx, "GD"+suffix); err != nil || g != 40 {
		t.Fatalf("gold %d %v", g, err)
	}
}

func TestMemoryCustomWorldStore(t *testing.T) {
	m, _ := NewMemory()
	testCustomWorldStore(t, m)
}

func TestPostgresCustomWorldStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testCustomWorldStore(t, p)
}
