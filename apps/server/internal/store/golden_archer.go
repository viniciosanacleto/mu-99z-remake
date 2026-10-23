package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// GoldenArcherStore replaces the EventGoldenArcher queries of the
// DataServer (FUN_00413750 count, FUN_004139a0 reward, FUN_00413c80 add).
type GoldenArcherStore interface {
	// RenaCount returns RenaCount, inserting a zero row when missing.
	RenaCount(ctx context.Context, account string) (int, error)
	// AddRena adds n (inserting the row with n when missing).
	AddRena(ctx context.Context, account string, n int) error
	// TakeRena subtracts need when RenaCount >= need (ok reports it).
	TakeRena(ctx context.Context, account string, need int) (ok bool, err error)
}

func (m *Memory) renaTable() map[string]int {
	if m.renas == nil {
		m.renas = map[string]int{}
	}
	return m.renas
}

func (m *Memory) RenaCount(_ context.Context, account string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.renaTable()
	k := strings.ToLower(account)
	if _, ok := t[k]; !ok {
		t[k] = 0
	}
	return t[k], nil
}

func (m *Memory) AddRena(_ context.Context, account string, n int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renaTable()[strings.ToLower(account)] += n
	return nil
}

func (m *Memory) TakeRena(_ context.Context, account string, need int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.renaTable()
	k := strings.ToLower(account)
	c, ok := t[k]
	if !ok || c < need {
		return false, nil
	}
	t[k] = c - need
	return true, nil
}

func (p *Postgres) RenaCount(ctx context.Context, account string) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT rena_count FROM golden_archer WHERE account=$1`, account).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = p.pool.Exec(ctx, `INSERT INTO golden_archer (account) VALUES ($1) ON CONFLICT DO NOTHING`, account)
		return 0, err
	}
	return n, err
}

func (p *Postgres) AddRena(ctx context.Context, account string, n int) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO golden_archer (account, rena_count) VALUES ($1, $2)
		ON CONFLICT (account) DO UPDATE SET rena_count = golden_archer.rena_count + $2`, account, n)
	return err
}

func (p *Postgres) TakeRena(ctx context.Context, account string, need int) (bool, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE golden_archer SET rena_count = rena_count - $2
		WHERE account = $1 AND rena_count >= $2`, account, need)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
