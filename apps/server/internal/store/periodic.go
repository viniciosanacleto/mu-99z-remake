package store

import "context"

// PeriodicStore replaces the PeriodicItem queries of the DataServer
// (FUN_004189b0 load, FUN_00418d30 save).
type PeriodicStore interface {
	// PeriodicTimes returns the time of each serial; a missing serial is
	// inserted with 0 and reported as 0.
	PeriodicTimes(ctx context.Context, serials []uint32) (map[uint32]int64, error)
	// SavePeriodic updates (or inserts) the time of each serial.
	SavePeriodic(ctx context.Context, times map[uint32]int64) error
}

func (m *Memory) PeriodicTimes(_ context.Context, serials []uint32) (map[uint32]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.periodic == nil {
		m.periodic = map[uint32]int64{}
	}
	out := make(map[uint32]int64, len(serials))
	for _, s := range serials {
		if _, ok := m.periodic[s]; !ok {
			m.periodic[s] = 0
		}
		out[s] = m.periodic[s]
	}
	return out, nil
}

func (m *Memory) SavePeriodic(_ context.Context, times map[uint32]int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.periodic == nil {
		m.periodic = map[uint32]int64{}
	}
	for s, t := range times {
		m.periodic[s] = t
	}
	return nil
}

func (p *Postgres) PeriodicTimes(ctx context.Context, serials []uint32) (map[uint32]int64, error) {
	out := make(map[uint32]int64, len(serials))
	for _, s := range serials {
		var t int64
		err := p.pool.QueryRow(ctx, `INSERT INTO periodic_item (item_serial) VALUES ($1)
			ON CONFLICT (item_serial) DO UPDATE SET time = periodic_item.time RETURNING time`, int64(s)).Scan(&t)
		if err != nil {
			return nil, err
		}
		out[s] = t
	}
	return out, nil
}

func (p *Postgres) SavePeriodic(ctx context.Context, times map[uint32]int64) error {
	for s, t := range times {
		if _, err := p.pool.Exec(ctx, `INSERT INTO periodic_item (item_serial, time) VALUES ($1, $2)
			ON CONFLICT (item_serial) DO UPDATE SET time = $2`, int64(s), t); err != nil {
			return err
		}
	}
	return nil
}
