package store

import "context"

// GHRSStore answers the CastleSiege GHRS request (C1 08 F8 YYYYMMDD,
// FUN_00442cf0) that the shipped DataServer.exe never handles: the reset
// count of the top-1 character.
type GHRSStore interface {
	TopResets(ctx context.Context) (int, error)
}

func (m *Memory) TopResets(_ context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	top := 0
	for _, c := range m.chars {
		top = max(top, c.Resets)
	}
	return top, nil
}

func (p *Postgres) TopResets(ctx context.Context) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(MAX(resets), 0) FROM characters`).Scan(&n)
	return n, err
}
