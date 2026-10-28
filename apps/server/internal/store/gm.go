package store

import (
	"context"
	"strings"
)

// GMStore sets the GM level of a character (characters.gm_level), the
// level the original reads from Util/GameMaster.txt (FUN_0047db10). The
// game reads it with the character, so a change applies on the next
// login; found is false when the character does not exist.
type GMStore interface {
	SetGMLevel(ctx context.Context, name string, level int) (found bool, err error)
}

func (m *Memory) SetGMLevel(_ context.Context, name string, level int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, name) {
			c.GMLevel = level
			return true, nil
		}
	}
	return false, nil
}

func (p *Postgres) SetGMLevel(ctx context.Context, name string, level int) (bool, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE characters SET gm_level=$2 WHERE name=$1`, name, int16(level))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
