package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PostgreSQL versions of the WZ_CS_* procedures (castle.go).

func (p *Postgres) CastleTotalInfo(ctx context.Context, group, cycleDays int) (CastleData, bool, error) {
	now := castleNow()
	tag, err := p.pool.Exec(ctx, `INSERT INTO mu_castle_data (map_svr_group, siege_start_date, siege_end_date)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, group, pgDate(now), pgDate(now.AddDate(0, 0, cycleDays)))
	if err != nil {
		return CastleData{}, false, err
	}
	d, _, err := p.CastleTaxInfo(ctx, group)
	return d, tag.RowsAffected() == 1, err
}

func (p *Postgres) CastleNPCs(ctx context.Context, group int) ([]CastleNPCData, error) {
	rows, err := p.pool.Query(ctx, `SELECT npc_number, npc_index, npc_df_level, npc_rg_level, npc_maxhp, npc_hp,
		npc_x, npc_y, npc_dir FROM mu_castle_npc WHERE map_svr_group = $1 ORDER BY npc_number, npc_index`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CastleNPCData
	for rows.Next() {
		var n CastleNPCData
		if err := rows.Scan(&n.Number, &n.Index, &n.DFLevel, &n.RGLevel, &n.MaxHP, &n.HP, &n.X, &n.Y, &n.Dir); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceCastleNPCs(ctx context.Context, group int, npcs []CastleNPCData) error {
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM mu_castle_npc WHERE map_svr_group = $1`, group); err != nil {
			return err
		}
		for _, n := range npcs {
			if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_npc (map_svr_group, npc_number, npc_index, npc_df_level,
				npc_rg_level, npc_maxhp, npc_hp, npc_x, npc_y, npc_dir) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
				ON CONFLICT DO NOTHING`,
				group, n.Number, n.Index, n.DFLevel, n.RGLevel, n.MaxHP, n.HP, n.X, n.Y, n.Dir); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *Postgres) BuyCastleNPC(ctx context.Context, group int, n CastleNPCData) (int, error) {
	tag, err := p.pool.Exec(ctx, `INSERT INTO mu_castle_npc (map_svr_group, npc_number, npc_index, npc_df_level,
		npc_rg_level, npc_maxhp, npc_hp, npc_x, npc_y, npc_dir) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT DO NOTHING`, group, n.Number, n.Index, n.DFLevel, n.RGLevel, n.MaxHP, n.HP, n.X, n.Y, n.Dir)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 4, nil
	}
	return 1, nil
}

func (p *Postgres) RepairCastleNPC(ctx context.Context, group, number, index int) (int, int, int, error) {
	var hp, maxHP int
	err := p.pool.QueryRow(ctx, `UPDATE mu_castle_npc SET npc_hp = npc_maxhp
		WHERE map_svr_group = $1 AND npc_number = $2 AND npc_index = $3 RETURNING npc_hp, npc_maxhp`,
		group, number, index).Scan(&hp, &maxHP)
	if errors.Is(err, pgx.ErrNoRows) {
		return 2, 0, 0, nil
	}
	if err != nil {
		return 0, 0, 0, err
	}
	return 1, hp, maxHP, nil
}

func (p *Postgres) UpgradeCastleNPC(ctx context.Context, group, number, index, typ, value int) (int, error) {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mu_castle_npc
		WHERE map_svr_group = $1 AND npc_number = $2 AND npc_index = $3)`, group, number, index).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var set string
	switch typ {
	case 1:
		set = "npc_df_level = $4"
	case 2:
		set = "npc_rg_level = $4"
	case 3:
		set = "npc_maxhp = $4, npc_hp = $4"
	default:
		return 2, nil
	}
	_, err := p.pool.Exec(ctx, `UPDATE mu_castle_npc SET `+set+`
		WHERE map_svr_group = $1 AND npc_number = $2 AND npc_index = $3`, group, number, index, value)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func (p *Postgres) RemoveCastleNPC(ctx context.Context, group, number, index int) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM mu_castle_npc WHERE map_svr_group = $1 AND npc_number = $2 AND npc_index = $3`,
		group, number, index)
	return err
}

func (p *Postgres) CastleTaxInfo(ctx context.Context, group int) (CastleData, bool, error) {
	var d CastleData
	err := p.pool.QueryRow(ctx, `SELECT siege_start_date, siege_end_date, siege_guildlist_setted, siege_ended,
		castle_occupy, owner_guild, money, tax_rate_chaos, tax_rate_store, tax_hunt_zone
		FROM mu_castle_data WHERE map_svr_group = $1`, group).Scan(&d.StartDate, &d.EndDate, &d.GuildListSet,
		&d.SiegeEnded, &d.Occupied, &d.Owner, &d.Money, &d.TaxChaos, &d.TaxStore, &d.TaxHunt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CastleData{}, false, nil
	}
	if err != nil {
		return CastleData{}, false, err
	}
	d.StartDate, d.EndDate = localDate(d.StartDate), localDate(d.EndDate)
	return d, true, nil
}

func (p *Postgres) ModifyCastleTaxRate(ctx context.Context, group, kind, rate int) (int, int, error) {
	var col string
	switch kind {
	case 1:
		col = "tax_rate_chaos"
	case 2:
		col = "tax_rate_store"
	case 3:
		col = "tax_hunt_zone"
	default:
		return 0, 0, nil
	}
	var stored int
	err := p.pool.QueryRow(ctx, `UPDATE mu_castle_data SET `+col+` = $2 WHERE map_svr_group = $1 RETURNING `+col,
		group, rate).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return 1, stored, nil
}

func (p *Postgres) ModifyCastleMoney(ctx context.Context, group int, delta int64) (int, int64, error) {
	var result int
	var money int64
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var total int64
		err := tx.QueryRow(ctx, `SELECT money FROM mu_castle_data WHERE map_svr_group = $1 FOR UPDATE`, group).Scan(&total)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		result, money = 2, total
		if total+delta >= 0 {
			result, money = 1, total+delta
			if _, err := tx.Exec(ctx, `UPDATE mu_castle_data SET money = $2 WHERE map_svr_group = $1`, group, money); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO mu_castle_money_statistics (map_svr_group, money_change) VALUES ($1, $2)`, group, delta)
		return err
	})
	return result, money, err
}

func (p *Postgres) SetCastleSchedule(ctx context.Context, group int, start, end time.Time) error {
	_, err := p.pool.Exec(ctx, `UPDATE mu_castle_data SET siege_start_date = $2, siege_end_date = $3 WHERE map_svr_group = $1`,
		group, pgDate(start), pgDate(end))
	return err
}

func (p *Postgres) SetCastleSiegeEnded(ctx context.Context, group int, ended bool) error {
	_, err := p.pool.Exec(ctx, `UPDATE mu_castle_data SET siege_ended = $2 WHERE map_svr_group = $1`, group, ended)
	return err
}

func (p *Postgres) SetCastleOwner(ctx context.Context, group int, occupied bool, owner string) error {
	_, err := p.pool.Exec(ctx, `UPDATE mu_castle_data SET castle_occupy = $2, owner_guild = $3 WHERE map_svr_group = $1`,
		group, occupied, owner)
	return err
}

func (p *Postgres) ResetCastleTaxInfo(ctx context.Context, group int) (int, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE mu_castle_data SET money = 0, tax_rate_chaos = 0, tax_rate_store = 0,
		tax_hunt_zone = 0 WHERE map_svr_group = $1`, group)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (p *Postgres) ResetCastleSiege(ctx context.Context, group int) (int, error) {
	result := 0
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE mu_castle_data SET siege_guildlist_setted = false, siege_ended = false
			WHERE map_svr_group = $1`, group)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		result = 1
		_, err = tx.Exec(ctx, `DELETE FROM mu_castle_siege_guildlist WHERE map_svr_group = $1`, group)
		return err
	})
	return result, err
}

// hasCastleData is the IF EXISTS (MuCastle_DATA) guard of the resets.
func (p *Postgres) hasCastleData(ctx context.Context, group int) (bool, error) {
	var ok bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mu_castle_data WHERE map_svr_group = $1)`, group).Scan(&ok)
	return ok, err
}

func (p *Postgres) ResetCastleRegSiege(ctx context.Context, group int) error {
	if ok, err := p.hasCastleData(ctx, group); err != nil || !ok {
		return err
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM mu_castle_reg_siege WHERE map_svr_group = $1`, group)
	return err
}

func (p *Postgres) ResetCastleSiegeGuilds(ctx context.Context, group int) error {
	if ok, err := p.hasCastleData(ctx, group); err != nil || !ok {
		return err
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM mu_castle_siege_guildlist WHERE map_svr_group = $1`, group)
	return err
}

func (p *Postgres) RegisterSiegeGuild(ctx context.Context, group int, guild string, chk CastleGuildCheck) (int, error) {
	result := 0
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var count int
		var exists bool
		var owner string
		if err := tx.QueryRow(ctx, `SELECT count(*), coalesce(bool_or(reg_siege_guild = $2::citext), false)
			FROM mu_castle_reg_siege WHERE map_svr_group = $1`, group, guild).Scan(&count, &exists); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT owner_guild::text FROM mu_castle_data WHERE map_svr_group = $1), '')`,
			group).Scan(&owner); err != nil {
			return err
		}
		switch {
		case count >= castleRegMax:
			result = CastleRegFull
		case exists:
			result = CastleRegAlready
		case owner != "" && equalFoldASCII(owner, guild):
			result = CastleRegOwner
		case !chk.Exists:
			result = CastleRegNoGuild
		case chk.MasterLevel < 1:
			result = CastleRegMasterLevel
		case chk.Members < 1:
			result = CastleRegNoMembers
		default:
			result = CastleRegOK
			_, err := tx.Exec(ctx, `INSERT INTO mu_castle_reg_siege (map_svr_group, reg_siege_guild, seq_num)
				VALUES ($1, $2, coalesce((SELECT max(seq_num) FROM mu_castle_reg_siege WHERE map_svr_group = $1), 0) + 1)`,
				group, guild)
			return err
		}
		return nil
	})
	return result, err
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func (p *Postgres) RegisterGuildMark(ctx context.Context, group int, guild string) (int, int, error) {
	var marks int
	err := p.pool.QueryRow(ctx, `UPDATE mu_castle_reg_siege SET reg_marks = reg_marks + 1
		WHERE map_svr_group = $1 AND reg_siege_guild = $2 AND is_giveup = 0 RETURNING reg_marks`, group, guild).Scan(&marks)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return 1, marks, nil
}

func (p *Postgres) GiveUpSiegeGuild(ctx context.Context, group int, guild string, giveUp bool) (int, int, error) {
	var del int
	g := 0
	if giveUp {
		g = 1
	}
	err := p.pool.QueryRow(ctx, `UPDATE mu_castle_reg_siege r SET is_giveup = $3, reg_marks = 0
		FROM (SELECT reg_marks FROM mu_castle_reg_siege WHERE map_svr_group = $1 AND reg_siege_guild = $2 FOR UPDATE) old
		WHERE r.map_svr_group = $1 AND r.reg_siege_guild = $2 RETURNING old.reg_marks`, group, guild, g).Scan(&del)
	if errors.Is(err, pgx.ErrNoRows) {
		return 2, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return 1, del, nil
}

func (p *Postgres) GuildMarkRegInfo(ctx context.Context, group int, guild string) (CastleRegGuild, bool, error) {
	var r CastleRegGuild
	var giveUp int
	err := p.pool.QueryRow(ctx, `SELECT reg_siege_guild, reg_marks, is_giveup, seq_num FROM mu_castle_reg_siege
		WHERE map_svr_group = $1 AND reg_siege_guild = $2`, group, guild).Scan(&r.Guild, &r.Marks, &giveUp, &r.Seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return CastleRegGuild{}, false, nil
	}
	r.GiveUp = giveUp != 0
	return r, err == nil, err
}

func (p *Postgres) regRows(ctx context.Context, q string, group int) ([]CastleRegGuild, error) {
	rows, err := p.pool.Query(ctx, q, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CastleRegGuild
	for rows.Next() {
		var r CastleRegGuild
		var giveUp int
		if err := rows.Scan(&r.Guild, &r.Marks, &giveUp, &r.Seq); err != nil {
			return nil, err
		}
		r.GiveUp = giveUp != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) RegisteredSiegeGuilds(ctx context.Context, group int) ([]CastleRegGuild, error) {
	return p.regRows(ctx, `SELECT reg_siege_guild, reg_marks, is_giveup, seq_num FROM mu_castle_reg_siege
		WHERE map_svr_group = $1 ORDER BY seq_num ASC LIMIT 100`, group)
}

func (p *Postgres) CalcRegGuildList(ctx context.Context, group int) ([]CastleRegGuild, error) {
	return p.regRows(ctx, `SELECT reg_siege_guild, reg_marks, is_giveup, seq_num FROM mu_castle_reg_siege
		WHERE map_svr_group = $1 AND is_giveup = 0 ORDER BY seq_num DESC LIMIT 100`, group)
}

func (p *Postgres) SetSiegeGuildList(ctx context.Context, group int, list []CastleSiegeGuild) (int, error) {
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM mu_castle_siege_guildlist WHERE map_svr_group = $1`, group); err != nil {
			return err
		}
		for _, g := range list {
			if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_siege_guildlist (map_svr_group, guild_name, guild_id,
				guild_involved, guild_score) VALUES ($1, $2, $3, $4, $5)`, group, g.Guild, g.Side, g.Involved, g.Score); err != nil {
				return err
			}
		}
		return nil
	})
	return 0, err
}

func (p *Postgres) SiegeGuildList(ctx context.Context, group int) ([]CastleSiegeGuild, error) {
	rows, err := p.pool.Query(ctx, `SELECT guild_name, guild_id, guild_involved, guild_score FROM mu_castle_siege_guildlist
		WHERE map_svr_group = $1`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CastleSiegeGuild
	for rows.Next() {
		var g CastleSiegeGuild
		if err := rows.Scan(&g.Guild, &g.Side, &g.Involved, &g.Score); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (p *Postgres) CheckSiegeGuildList(ctx context.Context, guild string) (bool, error) {
	var ok bool
	err := p.pool.QueryRow(ctx, `SELECT CASE
		WHEN coalesce((SELECT siege_ended FROM mu_castle_data ORDER BY map_svr_group DESC LIMIT 1), false) THEN false
		WHEN EXISTS(SELECT 1 FROM mu_castle_siege_guildlist WHERE guild_name = $1) THEN true
		ELSE EXISTS(SELECT 1 FROM mu_castle_reg_siege WHERE reg_siege_guild = $1 AND is_giveup = 0) END`, guild).Scan(&ok)
	return ok, err
}

// pgDate passes the calendar day of t (in its own location) to a date
// column; localDate reads it back as 00:00 local time, like the
// 'yyyy-mm-dd 00:00:00' strings of the procedures.
func pgDate(t time.Time) string { return t.Format("2006-01-02") }

func localDate(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}
