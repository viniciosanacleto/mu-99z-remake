package store

import (
	"context"
	"sort"
	"strings"
)

// CustomWorldStore replaces the DataServer requests of the custom
// world customs (docs/server-plan/10f-gm-eventos-custom.md, block 12):
// MonsterKillCount (DS 0x50), DailyReward (character load and DS 0x68)
// and the Jewel of Creation gold bonus (DS F7 06).
type CustomWorldStore interface {
	// AddMonsterKill is DS 0x50 (DataServer FUN_0040e6d0): the
	// MonsterKillCount row of (name, class) gets one more kill, or is
	// inserted with 1. It returns the new count.
	AddMonsterKill(ctx context.Context, name string, class int) (int, error)
	// DailyRewardDays is the DailyReward part of the character load
	// (FUN_0040c4c0): the claimed days (1..31) of the account in the
	// given month number (MONTH(GETDATE())), sorted.
	DailyRewardDays(ctx context.Context, account string, month int) ([]int, error)
	// AddDailyReward is one day of DS 0x68 (FUN_00410980): the row is
	// inserted unless (account, day, month) already exists.
	AddDailyReward(ctx context.Context, account, name string, day, month int) error
	// AddGold is DS F7 06 (FUN_00410c50): "UPDATE MEMB_INFO SET Gold =
	// ISNULL(Gold,0) + n" (accounts.gold).
	AddGold(ctx context.Context, account string, n int) error
	// Gold reads accounts.gold back (0 for an unknown account).
	Gold(ctx context.Context, account string) (int64, error)
}

type killKey struct {
	name  string
	class int
}

type memCustomWorld struct {
	kills map[killKey]int
	daily map[string]map[[2]int]string
	gold  map[string]int64
}

func (m *Memory) cw() *memCustomWorld {
	if m.cworld == nil {
		m.cworld = &memCustomWorld{kills: map[killKey]int{}, daily: map[string]map[[2]int]string{},
			gold: map[string]int64{}}
	}
	return m.cworld
}

func (m *Memory) AddMonsterKill(_ context.Context, name string, class int) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := killKey{strings.ToLower(name), class}
	m.cw().kills[k]++
	return m.cw().kills[k], nil
}

func (m *Memory) DailyRewardDays(_ context.Context, account string, month int) ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []int
	for k := range m.cw().daily[strings.ToLower(account)] {
		if k[1] == month {
			out = append(out, k[0])
		}
	}
	sort.Ints(out)
	return out, nil
}

func (m *Memory) AddDailyReward(_ context.Context, account, name string, day, month int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := strings.ToLower(account)
	if m.cw().daily[a] == nil {
		m.cw().daily[a] = map[[2]int]string{}
	}
	if _, ok := m.cw().daily[a][[2]int{day, month}]; !ok {
		m.cw().daily[a][[2]int{day, month}] = name
	}
	return nil
}

func (m *Memory) AddGold(_ context.Context, account string, n int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cw().gold[strings.ToLower(account)] += int64(n)
	return nil
}

func (m *Memory) Gold(_ context.Context, account string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cw().gold[strings.ToLower(account)], nil
}

func (p *Postgres) AddMonsterKill(ctx context.Context, name string, class int) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `INSERT INTO monster_kill_count (name, monster_class, kill_count) VALUES ($1, $2, 1)
		ON CONFLICT (name, monster_class) DO UPDATE SET kill_count = monster_kill_count.kill_count + 1
		RETURNING kill_count`, name, class).Scan(&n)
	return n, err
}

func (p *Postgres) DailyRewardDays(ctx context.Context, account string, month int) ([]int, error) {
	rows, err := p.pool.Query(ctx, `SELECT day FROM daily_reward WHERE account = $1 AND month = $2 ORDER BY day`,
		account, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var d int
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Postgres) AddDailyReward(ctx context.Context, account, name string, day, month int) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO daily_reward (account, name, day, month) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, account, name, day, month)
	return err
}

func (p *Postgres) AddGold(ctx context.Context, account string, n int) error {
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET gold = gold + $2 WHERE login = $1`, account, n)
	return err
}

func (p *Postgres) Gold(ctx context.Context, account string) (int64, error) {
	var n int64
	err := p.pool.QueryRow(ctx, `SELECT COALESCE((SELECT gold FROM accounts WHERE login = $1), 0)`, account).Scan(&n)
	return n, err
}
