package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestDateDiffs(t *testing.T) {
	d := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.Local) }
	cases := []struct {
		a, b          time.Time
		day, wek, mon int
	}{
		{d(2026, 10, 22, 1), d(2026, 10, 22, 23), 0, 0, 0},
		{d(2026, 10, 22, 23), d(2026, 10, 23, 0), 1, 0, 0},
		// 2026-10-24 is a Saturday, the 25th a Sunday: a week boundary.
		{d(2026, 10, 24, 12), d(2026, 10, 25, 12), 1, 1, 0},
		{d(2026, 10, 31, 12), d(2026, 11, 1, 12), 1, 1, 1},
		{d(2026, 12, 31, 12), d(2027, 1, 1, 12), 1, 0, 1},
	}
	for i, c := range cases {
		if g := dayDiff(c.a, c.b); g != c.day {
			t.Fatalf("%d: day %d", i, g)
		}
		if g := weekDiff(c.a, c.b); g != c.wek {
			t.Fatalf("%d: week %d", i, g)
		}
		if g := monthDiff(c.a, c.b); g != c.mon {
			t.Fatalf("%d: month %d", i, g)
		}
	}
}

func testCommandStore(t *testing.T, s Store) {
	cs, ok := s.(CommandStore)
	if !ok {
		t.Fatal("no CommandStore")
	}
	ctx := context.Background()
	suffix := fmt.Sprintf("%06d", time.Now().UnixNano()/1000%1000000)
	login := "cmd" + suffix
	acc, err := s.CreateAccount(ctx, login, "plain:x", "")
	if err != nil {
		t.Fatal(err)
	}
	name := "Res" + suffix
	if r, _, err := s.CreateCharacter(ctx, acc.ID, name, 0x10); err != nil || r != CreateOK {
		t.Fatalf("create %v %v", r, err)
	}
	defer func() { commandNow = time.Now }()
	now := time.Date(2026, 10, 21, 10, 0, 0, 0, time.Local) // Wednesday
	commandNow = func() time.Time { return now }

	c, err := cs.ResetInfo(ctx, login, name, false)
	if err != nil || c != (ResetCounts{}) {
		t.Fatalf("new row %+v %v", c, err)
	}
	if err := cs.SetResetInfo(ctx, login, name, false, 7, 0, ResetCounts{3, 4, 5}); err != nil {
		t.Fatal(err)
	}
	if c, _ = cs.ResetInfo(ctx, login, name, false); c != (ResetCounts{3, 4, 5}) {
		t.Fatalf("same day %+v", c)
	}
	// The master half is independent.
	if c, _ = cs.ResetInfo(ctx, login, name, true); c != (ResetCounts{}) {
		t.Fatalf("master %+v", c)
	}
	ch, _ := s.LoadCharacter(ctx, acc.ID, name)
	if ch.Resets != 7 {
		t.Fatalf("ResetCount %d", ch.Resets)
	}
	// Next day: only the daily counter reads 0 (and stays 0).
	now = now.AddDate(0, 0, 1)
	if c, _ = cs.ResetInfo(ctx, login, name, false); c != (ResetCounts{0, 4, 5}) {
		t.Fatalf("next day %+v", c)
	}
	if err := cs.SetResetInfo(ctx, login, name, false, 8, 0, ResetCounts{1, 5, 6}); err != nil {
		t.Fatal(err)
	}
	// Next week (Sunday): day and week.
	now = time.Date(2026, 10, 25, 10, 0, 0, 0, time.Local)
	if c, _ = cs.ResetInfo(ctx, login, name, false); c != (ResetCounts{0, 0, 6}) {
		t.Fatalf("next week %+v", c)
	}
	if err := cs.SetResetInfo(ctx, login, name, true, 0, 2, ResetCounts{1, 1, 1}); err != nil {
		t.Fatal(err)
	}
	ch, _ = s.LoadCharacter(ctx, acc.ID, name)
	if ch.Resets != 0 || ch.MasterResets != 2 {
		t.Fatalf("master reset %d %d", ch.Resets, ch.MasterResets)
	}

	// Rename: a taken name fails, otherwise the reset row follows.
	other := "Oth" + suffix
	s.CreateCharacter(ctx, acc.ID, other, 0)
	if ok, err := cs.RenameCharacter(ctx, acc.ID, login, name, other); err != nil || ok {
		t.Fatalf("rename to a taken name %v %v", ok, err)
	}
	newName := "New" + suffix
	if ok, err := cs.RenameCharacter(ctx, acc.ID, login, name, newName); err != nil || !ok {
		t.Fatalf("rename %v %v", ok, err)
	}
	if _, err := s.LoadCharacter(ctx, acc.ID, newName); err != nil {
		t.Fatal(err)
	}
	if ex, _ := s.CharacterExists(ctx, name); ex {
		t.Fatal("old name still exists")
	}
	if c, _ = cs.ResetInfo(ctx, login, newName, true); c != (ResetCounts{1, 1, 1}) {
		t.Fatalf("renamed row %+v", c)
	}
}

func TestMemoryCommandStore(t *testing.T) {
	m, _ := NewMemory()
	testCommandStore(t, m)
}

func TestPostgresCommandStore(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testCommandStore(t, p)
}
