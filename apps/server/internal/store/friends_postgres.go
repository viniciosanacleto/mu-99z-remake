package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (p *Postgres) FriendGuidCreate(ctx context.Context, name string) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO friend_main(name) SELECT name FROM characters WHERE name=$1
		ON CONFLICT (name) DO NOTHING`, name)
	return err
}

func (p *Postgres) FriendMain(ctx context.Context, name string) (int, int, bool, error) {
	var guid, total int
	err := p.pool.QueryRow(ctx, `SELECT guid, memo_total FROM friend_main WHERE name=$1`, name).Scan(&guid, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	return guid, total, err == nil, err
}

func (p *Postgres) names(ctx context.Context, sql string, guid int) ([]string, error) {
	rows, err := p.pool.Query(ctx, sql, guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (p *Postgres) FriendNames(ctx context.Context, guid int) ([]string, error) {
	return p.names(ctx, `SELECT friend_name FROM friend_list WHERE guid=$1 ORDER BY friend_name`, guid)
}

func (p *Postgres) WaitingFriends(ctx context.Context, guid int) ([]string, error) {
	return p.names(ctx, `SELECT friend_name FROM wait_friend WHERE guid=$1 ORDER BY friend_name`, guid)
}

// guidOf returns the friend_main guid of a name (ok false when missing).
func guidOf(ctx context.Context, tx pgx.Tx, name string) (int, string, bool, error) {
	var guid int
	var canon string
	err := tx.QueryRow(ctx, `SELECT guid, name FROM friend_main WHERE name=$1`, name).Scan(&guid, &canon)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	return guid, canon, err == nil, err
}

func exists(ctx context.Context, tx pgx.Tx, sql string, args ...any) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(`+sql+`)`, args...).Scan(&ok)
	return ok, err
}

// WaitFriendAdd follows WZ_WaitFriendAdd.
func (p *Postgres) WaitFriendAdd(ctx context.Context, name, friend string) (int, error) {
	code := 1
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		u, uName, ok, err := guidOf(ctx, tx, name)
		if err != nil || !ok {
			code = 0
			return err
		}
		f, fName, ok, err := guidOf(ctx, tx, friend)
		if err != nil || !ok {
			code = 0
			return err
		}
		var lvl int
		if err := tx.QueryRow(ctx, `SELECT level FROM characters WHERE name=$1`, friend).Scan(&lvl); err != nil {
			code = 7
			return nil
		}
		if lvl < 6 {
			code = 6
			return nil
		}
		if ok, err := exists(ctx, tx, `SELECT 1 FROM friend_list WHERE guid=$1 AND friend_guid=$2`, u, f); err != nil || ok {
			code = 4
			return err
		}
		if ok, err := exists(ctx, tx, `SELECT 1 FROM friend_list WHERE guid=$1 AND friend_guid=$2`, f, u); err != nil {
			return err
		} else if ok {
			_, err := tx.Exec(ctx, `UPDATE friend_list SET del=0 WHERE guid=$1 AND friend_guid=$2`, f, u)
			code = 8
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO friend_list(guid, friend_guid, friend_name, del) VALUES ($1,$2,$3,1)`, u, f, fName); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO wait_friend(guid, friend_guid, friend_name) VALUES ($1,$2,$3)`, f, u, uName)
		return err
	})
	return code, err
}

// FriendAdd follows WZ_FriendAdd.
func (p *Postgres) FriendAdd(ctx context.Context, name, friend string) (int, error) {
	code := 1
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		f, fName, ok, err := guidOf(ctx, tx, friend)
		if err != nil || !ok {
			code = 5
			return err
		}
		u, _, ok, err := guidOf(ctx, tx, name)
		if err != nil || !ok {
			code = 3
			return err
		}
		if ok, err := exists(ctx, tx, `SELECT 1 FROM friend_list WHERE guid=$1 AND friend_guid=$2`, u, f); err != nil || ok {
			code = 2
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO friend_list(guid, friend_guid, friend_name) VALUES ($1,$2,$3)`, u, f, fName); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM wait_friend WHERE guid=$1 AND friend_guid=$2`, u, f)
		return err
	})
	return code, err
}

// WaitFriendDel follows WZ_WaitFriendDel.
func (p *Postgres) WaitFriendDel(ctx context.Context, name, friend string) (int, error) {
	code := 2
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		u, _, ok, err := guidOf(ctx, tx, name)
		if err != nil || !ok {
			code = 0
			return err
		}
		f, _, ok, err := guidOf(ctx, tx, friend)
		if err != nil || !ok {
			code = 0
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM wait_friend WHERE guid=$1 AND friend_guid=$2`, u, f)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			code = 0
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE friend_list SET del=1 WHERE guid=$1 AND friend_guid=$2`, f, u)
		return err
	})
	return code, err
}

// FriendDel follows WZ_FriendDel.
func (p *Postgres) FriendDel(ctx context.Context, name, friend string) (int, error) {
	code := 1
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		u, _, ok, err := guidOf(ctx, tx, name)
		if err != nil || !ok {
			code = 3
			return err
		}
		f, _, ok, err := guidOf(ctx, tx, friend)
		if err != nil || !ok {
			code = 5
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM friend_list WHERE guid=$1 AND friend_guid=$2`, u, f)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			code = 2
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE friend_list SET del=1 WHERE guid=$1 AND friend_guid=$2`, f, u)
		return err
	})
	return code, err
}

func (p *Postgres) SetFriendDel(ctx context.Context, name string, del int) error {
	_, err := p.pool.Exec(ctx, `UPDATE friend_list SET del=$2 WHERE guid=(SELECT guid FROM friend_main WHERE name=$1)`, name, del)
	return err
}

// WriteMail follows WZ_WriteMail and the Memo/Photo update.
func (p *Postgres) WriteMail(ctx context.Context, m Mail) (int, int, error) {
	result, guid := 0, 0
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var total int
		err := tx.QueryRow(ctx, `SELECT guid, memo_total FROM friend_main WHERE name=$1 FOR UPDATE`, m.To).Scan(&guid, &total)
		if errors.Is(err, pgx.ErrNoRows) {
			result = 2
			return nil
		}
		if err != nil {
			return err
		}
		if total > 49 {
			result = 5
			return nil
		}
		var lvl int
		if err := tx.QueryRow(ctx, `SELECT level FROM characters WHERE name=$1`, m.To).Scan(&lvl); err != nil {
			result = 2
			return nil
		}
		if lvl < 6 {
			result = 6
			return nil
		}
		var idx int
		if err := tx.QueryRow(ctx, `UPDATE friend_main SET memo_count=memo_count+1, memo_total=memo_total+1
			WHERE guid=$1 RETURNING memo_count`, guid).Scan(&idx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO friend_mail(memo_index, guid, friend_name, subject, b_read, memo, photo, dir, act)
			VALUES ($1,$2,$3,$4,0,$5,$6,$7,$8)`, idx, guid, m.From, m.Subject, m.Memo, m.Photo, int16(m.Dir), int16(m.Act)); err != nil {
			return err
		}
		result = idx
		return nil
	})
	return result, guid, err
}

func (p *Postgres) MailList(ctx context.Context, guid int) ([]Mail, error) {
	rows, err := p.pool.Query(ctx, `SELECT memo_index, friend_name, w_date, subject, b_read FROM friend_mail
		WHERE guid=$1 ORDER BY memo_index`, guid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mail
	for rows.Next() {
		var m Mail
		var read int16
		if err := rows.Scan(&m.Index, &m.From, &m.Date, &m.Subject, &read); err != nil {
			return nil, err
		}
		m.Read = read != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p *Postgres) ReadMail(ctx context.Context, name string, index int) (*Mail, error) {
	var m Mail
	var dir, act, read int16
	err := p.pool.QueryRow(ctx, `UPDATE friend_mail SET b_read=1
		WHERE memo_index=$2 AND guid=(SELECT guid FROM friend_main WHERE name=$1)
		RETURNING memo_index, friend_name, w_date, subject, b_read, memo, photo, dir, act`, name, index).
		Scan(&m.Index, &m.From, &m.Date, &m.Subject, &read, &m.Memo, &m.Photo, &dir, &act)
	if err != nil {
		return nil, notFound(err)
	}
	m.Read, m.Dir, m.Act = read != 0, byte(dir), byte(act)
	return &m, nil
}

// DelMail follows WZ_DelMail.
func (p *Postgres) DelMail(ctx context.Context, name string, index int) (int, error) {
	code := 1
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		u, _, ok, err := guidOf(ctx, tx, name)
		if err != nil || !ok {
			code = 2
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM friend_mail WHERE memo_index=$1 AND guid=$2`, index, u)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			code = 4
			return nil
		}
		_, err = tx.Exec(ctx, `UPDATE friend_main SET memo_total=memo_total-1 WHERE guid=$1`, u)
		return err
	})
	return code, err
}
