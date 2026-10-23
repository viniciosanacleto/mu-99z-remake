package store

import (
	"context"
	"strings"
)

// Ranking events, numbered by the DS head that reports them.
const (
	RankingBloodCastle    = 0x3D
	RankingChaosCastle    = 0x3E
	RankingDevilSquare    = 0x3F
	RankingIllusionTemple = 0x40
)

// RankingStore replaces the ranking queries of the DataServer
// (FUN_0040f470 RankingDuel, FUN_0040f670..FUN_0040fb80 RankingBloodCastle,
// RankingChaosCastle, RankingDevilSquare, RankingIllusionTemple): each
// report adds to the character's row, inserting it when missing.
type RankingStore interface {
	AddDuelRanking(ctx context.Context, name string, win, lose int) error
	AddEventRanking(ctx context.Context, event int, name string, score int) error
	// DuelRanking and EventRanking read a row back (zero when missing).
	DuelRanking(ctx context.Context, name string) (win, lose int, err error)
	EventRanking(ctx context.Context, event int, name string) (int, error)
}

type rankKey struct {
	event int
	name  string
}

func (m *Memory) AddDuelRanking(_ context.Context, name string, win, lose int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.duelRank == nil {
		m.duelRank = map[string][2]int{}
	}
	k := strings.ToLower(name)
	r := m.duelRank[k]
	m.duelRank[k] = [2]int{r[0] + win, r[1] + lose}
	return nil
}

func (m *Memory) AddEventRanking(_ context.Context, event int, name string, score int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.eventRank == nil {
		m.eventRank = map[rankKey]int{}
	}
	m.eventRank[rankKey{event, strings.ToLower(name)}] += score
	return nil
}

func (m *Memory) DuelRanking(_ context.Context, name string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.duelRank[strings.ToLower(name)]
	return r[0], r[1], nil
}

func (m *Memory) EventRanking(_ context.Context, event int, name string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.eventRank[rankKey{event, strings.ToLower(name)}], nil
}

func (p *Postgres) AddDuelRanking(ctx context.Context, name string, win, lose int) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO ranking_duel (name, win_score, lose_score) VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET win_score = ranking_duel.win_score + $2,
		lose_score = ranking_duel.lose_score + $3`, name, win, lose)
	return err
}

func (p *Postgres) AddEventRanking(ctx context.Context, event int, name string, score int) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO ranking_event (event, name, score) VALUES ($1, $2, $3)
		ON CONFLICT (event, name) DO UPDATE SET score = ranking_event.score + $3`, event, name, score)
	return err
}

func (p *Postgres) DuelRanking(ctx context.Context, name string) (int, int, error) {
	var win, lose int
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(win_score), 0), COALESCE(SUM(lose_score), 0)
		FROM ranking_duel WHERE name = $1`, name).Scan(&win, &lose)
	return win, lose, err
}

func (p *Postgres) EventRanking(ctx context.Context, event int, name string) (int, error) {
	var n int
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(score), 0) FROM ranking_event WHERE event = $1 AND name = $2`,
		event, name).Scan(&n)
	return n, err
}
