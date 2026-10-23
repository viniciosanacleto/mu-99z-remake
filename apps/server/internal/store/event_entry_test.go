package store

import (
	"context"
	"os"
	"testing"
	"time"
)

func testEventEntries(t *testing.T, s EventEntryStore) {
	ctx := context.Background()
	acc, name := "acc"+time.Now().Format("150405.000"), "Hero"
	e, err := s.GetEventEntry(ctx, acc, name)
	if err != nil || e != (EventEntry{}) {
		t.Fatalf("new row %+v %v", e, err)
	}
	want := EventEntry{BC: 1, CC: 2, DS: 3, DG: 4, IT: 5, IG: 6}
	if err := s.SetEventEntry(ctx, acc, name, want); err != nil {
		t.Fatal(err)
	}
	if e, _ := s.GetEventEntry(ctx, acc, name); e != want {
		t.Fatalf("same day %+v", e)
	}
}

func TestMemoryEventEntries(t *testing.T) {
	m, _ := NewMemory()
	testEventEntries(t, m)
	// A new day resets everything but keeps ITCount in the table, like the
	// "@ITCount=@ITCount" of WZ_GetEventEntryInfo.
	ctx := context.Background()
	acc := "daily"
	m.SetEventEntry(ctx, acc, "x", EventEntry{BC: 2, IT: 4})
	old := eventEntryNow
	eventEntryNow = func() time.Time { return time.Now().Add(24 * time.Hour) }
	defer func() { eventEntryNow = old }()
	if e, _ := m.GetEventEntry(ctx, acc, "x"); e != (EventEntry{}) {
		t.Fatalf("reset %+v", e)
	}
	if r := m.entries[entryKey(acc, "x")]; r.e != (EventEntry{IT: 4}) {
		t.Fatalf("stored %+v", r.e)
	}
}

func TestPostgresEventEntries(t *testing.T) {
	url := os.Getenv("MU_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MU_TEST_DATABASE_URL not set")
	}
	p, err := OpenPostgres(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	testEventEntries(t, p)
	ctx := context.Background()
	p.pool.Exec(ctx, `INSERT INTO event_entry_count (account, name, bc, it, last_date) VALUES ('old', 'x', 2, 4, now() - interval '2 days')
		ON CONFLICT (account, name) DO UPDATE SET bc=2, it=4, last_date=now() - interval '2 days'`)
	if e, _ := p.GetEventEntry(ctx, "old", "x"); e != (EventEntry{}) {
		t.Fatalf("reset %+v", e)
	}
	var bc, it int
	p.pool.QueryRow(ctx, `SELECT bc, it FROM event_entry_count WHERE account='old' AND name='x'`).Scan(&bc, &it)
	if bc != 0 || it != 4 {
		t.Fatalf("stored bc %d it %d", bc, it)
	}
}
