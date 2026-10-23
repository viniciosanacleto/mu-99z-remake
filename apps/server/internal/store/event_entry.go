package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EventEntry is a row of EventEntryCount: today's entries per event
// (Blood Castle, Chaos Castle, Devil Square, DG, Illusion Temple, IG),
// loaded with the character (DataServer FUN_0040c4c0) into obj+0x2098...
type EventEntry struct {
	BC, CC, DS, DG, IT, IG int
}

// EventEntryStore replaces WZ_GetEventEntryInfo / WZ_SetEventEntryInfo.
type EventEntryStore interface {
	// GetEventEntry creates the row when missing and resets the counts on
	// the first read of a new day. Like the procedure, the reset UPDATE
	// writes "@ITCount=@ITCount" (a variable, not the column), so ITCount
	// keeps its old value in the table while 0 is returned.
	GetEventEntry(ctx context.Context, account, name string) (EventEntry, error)
	// SetEventEntry is WZ_SetEventEntryInfo (LastDate untouched).
	SetEventEntry(ctx context.Context, account, name string, e EventEntry) error
}

type eventEntryRow struct {
	e    EventEntry
	last time.Time
}

// eventEntryNow is GETDATE() for the memory store (tests replace it).
var eventEntryNow = time.Now

// newDay is datediff(day, last, now) > 0.
func newDay(last, now time.Time) bool {
	ly, lm, ld := last.Date()
	ny, nm, nd := now.Date()
	return time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC).After(time.Date(ly, lm, ld, 0, 0, 0, 0, time.UTC))
}

func entryKey(account, name string) string {
	return strings.ToLower(account) + "/" + strings.ToLower(name)
}

func (m *Memory) GetEventEntry(_ context.Context, account, name string) (EventEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]*eventEntryRow{}
	}
	now := eventEntryNow()
	r := m.entries[entryKey(account, name)]
	if r == nil {
		r = &eventEntryRow{last: now}
		m.entries[entryKey(account, name)] = r
	}
	if newDay(r.last, now) {
		it := r.e.IT
		r.e = EventEntry{IT: it}
		r.last = now
		return EventEntry{}, nil
	}
	return r.e, nil
}

func (m *Memory) SetEventEntry(_ context.Context, account, name string, e EventEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries == nil {
		m.entries = map[string]*eventEntryRow{}
	}
	r := m.entries[entryKey(account, name)]
	if r == nil {
		r = &eventEntryRow{last: eventEntryNow()}
		m.entries[entryKey(account, name)] = r
	}
	r.e = e
	return nil
}

func (p *Postgres) GetEventEntry(ctx context.Context, account, name string) (EventEntry, error) {
	var e EventEntry
	var reset bool
	err := p.pool.QueryRow(ctx, `WITH ins AS (
			INSERT INTO event_entry_count (account, name) VALUES ($1, $2) ON CONFLICT DO NOTHING
		)
		SELECT bc, cc, ds, dg, it, ig, last_date::date < now()::date FROM event_entry_count
		WHERE account = $1 AND name = $2`, account, name).Scan(&e.BC, &e.CC, &e.DS, &e.DG, &e.IT, &e.IG, &reset)
	if errors.Is(err, pgx.ErrNoRows) {
		// The row was inserted by this statement: defaults.
		return EventEntry{}, nil
	}
	if err != nil {
		return e, err
	}
	if !reset {
		return e, nil
	}
	_, err = p.pool.Exec(ctx, `UPDATE event_entry_count SET bc=0, cc=0, ds=0, dg=0, ig=0, last_date=now()
		WHERE account = $1 AND name = $2`, account, name)
	return EventEntry{}, err
}

func (p *Postgres) SetEventEntry(ctx context.Context, account, name string, e EventEntry) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO event_entry_count (account, name, bc, cc, ds, dg, it, ig)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (account, name) DO UPDATE SET bc=$3, cc=$4, ds=$5, dg=$6, it=$7, ig=$8`,
		account, name, e.BC, e.CC, e.DS, e.DG, e.IT, e.IG)
	return err
}
