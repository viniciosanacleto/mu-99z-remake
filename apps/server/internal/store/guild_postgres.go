package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (p *Postgres) LoadGuilds(ctx context.Context) ([]Guild, error) {
	rows, err := p.pool.Query(ctx, `SELECT id, name, master, mark, score, notice, type, union_id, rival
		FROM guilds ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var out []Guild
	idx := map[int]int{}
	for rows.Next() {
		var g Guild
		var id int64
		if err := rows.Scan(&id, &g.Name, &g.Master, &g.Mark, &g.Score, &g.Notice, &g.Type, &g.Union, &g.Rival); err != nil {
			rows.Close()
			return nil, err
		}
		g.Number = int(id)
		idx[g.Number] = len(out)
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := p.pool.Query(ctx, `SELECT m.guild_id, m.name, m.status FROM guild_members m
		JOIN guilds g ON g.id = m.guild_id ORDER BY m.guild_id, (m.name = g.master) DESC, m.seq`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var gid int64
		var m GuildMember
		if err := mrows.Scan(&gid, &m.Name, &m.Status); err != nil {
			return nil, err
		}
		if i, ok := idx[int(gid)]; ok {
			out[i].Members = append(out[i].Members, m)
		}
	}
	return out, mrows.Err()
}

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

func (p *Postgres) GuildCreate(ctx context.Context, name, master string, mark []byte, status int) (int, int, error) {
	var number int64
	code := 0
	if len(mark) != 32 {
		mark = append(append([]byte(nil), mark...), make([]byte, 32)...)[:32]
	}
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO guilds(name, master, mark) VALUES ($1, $2, $3) RETURNING id`,
			name, master, mark).Scan(&number)
		if isUniqueViolation(err) {
			code = 1
			return errRollback
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO guild_members(name, guild_id, level, status) VALUES ($1, $2, 1, $3)`,
			master, number, status)
		if isUniqueViolation(err) {
			code = 2
			return errRollback
		}
		return err
	})
	if errors.Is(err, errRollback) {
		return 0, code, nil
	}
	return int(number), code, err
}

var errRollback = errors.New("store: rollback")

func (p *Postgres) GuildDelete(ctx context.Context, name string) error {
	// guild_members rows go with ON DELETE CASCADE.
	_, err := p.pool.Exec(ctx, `DELETE FROM guilds WHERE name=$1`, name)
	return err
}

func (p *Postgres) GuildMemberAdd(ctx context.Context, guild, name string, status int) (bool, error) {
	tag, err := p.pool.Exec(ctx, `INSERT INTO guild_members(name, guild_id, level, status)
		SELECT $2, id, 1, $3 FROM guilds WHERE name=$1 ON CONFLICT (name) DO NOTHING`, guild, name, status)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM guild_members WHERE name=$1)`, name).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, ErrNotFound
		}
		return false, nil
	}
	return true, nil
}

func (p *Postgres) GuildMemberDelete(ctx context.Context, name string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM guild_members WHERE name=$1`, name)
	return err
}

func (p *Postgres) SetGuildMemberStatus(ctx context.Context, name string, status int) error {
	_, err := p.pool.Exec(ctx, `UPDATE guild_members SET status=$2 WHERE name=$1`, name, status)
	return err
}

func (p *Postgres) SetGuildType(ctx context.Context, guild string, typ int) error {
	_, err := p.pool.Exec(ctx, `UPDATE guilds SET type=$2 WHERE name=$1`, guild, typ)
	return err
}

func (p *Postgres) SetGuildNotice(ctx context.Context, guild, notice string) error {
	_, err := p.pool.Exec(ctx, `UPDATE guilds SET notice=$2 WHERE name=$1`, guild, notice)
	return err
}

func (p *Postgres) SetGuildScore(ctx context.Context, guild string, score int) error {
	_, err := p.pool.Exec(ctx, `UPDATE guilds SET score=$2 WHERE name=$1`, guild, score)
	return err
}

func (p *Postgres) SetGuildUnion(ctx context.Context, union int, numbers ...int) error {
	_, err := p.pool.Exec(ctx, `UPDATE guilds SET union_id=$1 WHERE id = ANY($2)`, union, numbers)
	return err
}

func (p *Postgres) SetGuildRival(ctx context.Context, rival int, numbers ...int) error {
	_, err := p.pool.Exec(ctx, `UPDATE guilds SET rival=$1 WHERE id = ANY($2)`, rival, numbers)
	return err
}
