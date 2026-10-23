package store

import (
	"context"
	"strings"
)

// BanStore replaces the DataServer requests of the GM ban commands
// (DS 0x0F 05 WZ_BanAccount and DS 0x0F 06 WZ_BanCharacter, FUN_00409040
// and FUN_00409220 of DataServer.exe): the account's bloc_code
// (accounts.block_code) or the character's CtlCode (characters.ctl_code)
// takes the value; found is false when the account or character does not
// exist (the procedures' @Result 0).
type BanStore interface {
	BanAccount(ctx context.Context, login string, code int) (found bool, err error)
	BanCharacter(ctx context.Context, name string, code int) (found bool, err error)
}

func (m *Memory) BanAccount(_ context.Context, login string, code int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if strings.EqualFold(a.Login, login) {
			a.BlockCode = code
			return true, nil
		}
	}
	return false, nil
}

func (m *Memory) BanCharacter(_ context.Context, name string, code int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, name) {
			c.CtlCode = byte(code)
			return true, nil
		}
	}
	return false, nil
}

func (p *Postgres) BanAccount(ctx context.Context, login string, code int) (bool, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE accounts SET block_code=$2 WHERE login=$1`, login, int16(code))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (p *Postgres) BanCharacter(ctx context.Context, name string, code int) (bool, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE characters SET ctl_code=$2 WHERE name=$1`, name, int16(code))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
