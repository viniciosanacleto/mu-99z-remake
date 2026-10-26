package store

import (
	"context"
	"strings"
)

// CustomEventStore replaces the DataServer requests of the custom
// custom events: DS F3 E7 (FUN_00477060 -> DataServer FUN_00410ae0,
// patente) and DS 0x41 (FUN_00476fb0 -> FUN_004108f0, ranking column + 1).
type CustomEventStore interface {
	// SetPatente clears the patente from its current holder and gives it
	// to the character; values <= 0 are ignored (the DataServer only
	// writes when the value is positive).
	SetPatente(ctx context.Context, name string, patente int) error
	// AddCustomRank adds n to the named ranking of a character.
	AddCustomRank(ctx context.Context, name, rank string, n int) error
	// CustomRank reads a ranking back (0 when missing).
	CustomRank(ctx context.Context, name, rank string) (int, error)
}

type customRankKey struct{ name, rank string }

func (m *Memory) SetPatente(_ context.Context, name string, patente int) error {
	if patente <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chars {
		if c.Patente == patente {
			c.Patente = 0
		}
	}
	for _, c := range m.chars {
		if c.Name == name {
			c.Patente = patente
		}
	}
	return nil
}

func (m *Memory) AddCustomRank(_ context.Context, name, rank string, n int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.customRank == nil {
		m.customRank = map[customRankKey]int{}
	}
	m.customRank[customRankKey{strings.ToLower(name), rank}] += n
	return nil
}

func (m *Memory) CustomRank(_ context.Context, name, rank string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.customRank[customRankKey{strings.ToLower(name), rank}], nil
}

func (p *Postgres) SetPatente(ctx context.Context, name string, patente int) error {
	if patente <= 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE characters SET patente = 0 WHERE patente = $1`, patente); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE characters SET patente = $1 WHERE name = $2`, patente, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) AddCustomRank(ctx context.Context, name, rank string, n int) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO custom_rank (name, rank, score) VALUES ($1, $2, $3)
		ON CONFLICT (name, rank) DO UPDATE SET score = custom_rank.score + $3`, name, rank, n)
	return err
}

func (p *Postgres) CustomRank(ctx context.Context, name, rank string) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(score), 0) FROM custom_rank WHERE name = $1 AND rank = $2`,
		name, rank).Scan(&n)
	return n, err
}
