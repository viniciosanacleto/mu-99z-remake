package store

import (
	"context"
	"sort"
	"strings"
)

// CharacterRef is one row of the editor's character search (cmd/mueditor).
type CharacterRef struct {
	AccountID int64
	Login     string
	Name      string
	Level     int
	Class     byte
	Resets    int
	Online    bool // account_sessions.online
}

// likeEscaper escapes the ILIKE wildcards of a user query.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SearchCharacters lists characters whose name or account login contains
// query (case-insensitive), ordered by name.
func (p *Postgres) SearchCharacters(ctx context.Context, query string, limit int) ([]CharacterRef, error) {
	rows, err := p.pool.Query(ctx, `SELECT a.id, a.login, c.name, c.level, c.class, c.resets, coalesce(s.online, false)
		FROM characters c JOIN accounts a ON a.id = c.account_id
		LEFT JOIN account_sessions s ON s.account_id = a.id
		WHERE c.name ILIKE $1 OR a.login ILIKE $1
		ORDER BY c.name LIMIT $2`, "%"+likeEscaper.Replace(query)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharacterRef
	for rows.Next() {
		var r CharacterRef
		var class int16
		if err := rows.Scan(&r.AccountID, &r.Login, &r.Name, &r.Level, &class, &r.Resets, &r.Online); err != nil {
			return nil, err
		}
		r.Class = byte(class)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AccountOnline reports account_sessions.online.
func (p *Postgres) AccountOnline(ctx context.Context, accountID int64) (bool, error) {
	var on bool
	err := p.pool.QueryRow(ctx, `SELECT coalesce((SELECT online FROM account_sessions WHERE account_id=$1), false)`,
		accountID).Scan(&on)
	return on, err
}

func (m *Memory) SearchCharacters(_ context.Context, query string, limit int) ([]CharacterRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := strings.ToLower(query)
	var out []CharacterRef
	for _, c := range m.chars {
		a := m.accounts[c.AccountID]
		if a == nil {
			continue
		}
		if !strings.Contains(strings.ToLower(c.Name), q) && !strings.Contains(strings.ToLower(a.Login), q) {
			continue
		}
		out = append(out, CharacterRef{
			AccountID: a.ID, Login: a.Login, Name: c.Name, Level: c.Level, Class: c.Class,
			Resets: c.Resets, Online: m.online[a.ID],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) AccountOnline(_ context.Context, accountID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.online[accountID], nil
}
