package migrate

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// importCastle copies the MuCastle_* tables (Castle Siege, migration
// 0008_castle.sql): schedule/owner/money, the gates and statues, the
// registrations, the siege guild list and the money log.
func importCastle(ctx context.Context, src Source, tx pgx.Tx, rep *Report) error {
	data, err := rows(ctx, src, "MuCastle_DATA")
	if err != nil {
		return err
	}
	for _, r := range data {
		start, _ := r.time("SIEGE_START_DATE")
		end, _ := r.time("SIEGE_END_DATE")
		if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_data (map_svr_group, siege_start_date, siege_end_date,
			siege_guildlist_setted, siege_ended, castle_occupy, owner_guild, money, tax_rate_chaos, tax_rate_store,
			tax_hunt_zone) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
			r.num("MAP_SVR_GROUP"), start.Format("2006-01-02"), end.Format("2006-01-02"),
			r.num("SIEGE_GUILDLIST_SETTED") != 0, r.num("SIEGE_ENDED") != 0, r.num("CASTLE_OCCUPY") != 0,
			r.str("OWNER_GUILD"), money(r, "MONEY"), r.num("TAX_RATE_CHAOS"), r.num("TAX_RATE_STORE"),
			r.num("TAX_HUNT_ZONE")); err != nil {
			return fmt.Errorf("MuCastle_DATA: %w", err)
		}
		rep.CastleRows++
	}
	npcs, err := rows(ctx, src, "MuCastle_NPC")
	if err != nil {
		return err
	}
	for _, r := range npcs {
		created, ok := r.time("NPC_CREATEDATE")
		if !ok {
			created = time.Now()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_npc (map_svr_group, npc_number, npc_index, npc_df_level,
			npc_rg_level, npc_maxhp, npc_hp, npc_x, npc_y, npc_dir, npc_createdate)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
			r.num("MAP_SVR_GROUP"), r.num("NPC_NUMBER"), r.num("NPC_INDEX"), r.num("NPC_DF_LEVEL"),
			r.num("NPC_RG_LEVEL"), r.num("NPC_MAXHP"), r.num("NPC_HP"), r.num("NPC_X"), r.num("NPC_Y"),
			r.num("NPC_DIR"), created); err != nil {
			return fmt.Errorf("MuCastle_NPC: %w", err)
		}
		rep.CastleRows++
	}
	reg, err := rows(ctx, src, "MuCastle_REG_SIEGE")
	if err != nil {
		return err
	}
	for _, r := range reg {
		if r.str("REG_SIEGE_GUILD") == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_reg_siege (map_svr_group, reg_siege_guild, reg_marks,
			is_giveup, seq_num) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
			r.num("MAP_SVR_GROUP"), r.str("REG_SIEGE_GUILD"), r.num("REG_MARKS"), r.num("IS_GIVEUP"),
			r.num("SEQ_NUM")); err != nil {
			return fmt.Errorf("MuCastle_REG_SIEGE: %w", err)
		}
		rep.CastleRows++
	}
	list, err := rows(ctx, src, "MuCastle_SIEGE_GUILDLIST")
	if err != nil {
		return err
	}
	for _, r := range list {
		if r.str("GUILD_NAME") == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_siege_guildlist (map_svr_group, guild_name, guild_id,
			guild_involved, guild_score) VALUES ($1,$2,$3,$4,$5)`, r.num("MAP_SVR_GROUP"), r.str("GUILD_NAME"),
			r.num("GUILD_ID"), r.num("GUILD_INVOLVED") != 0, r.num("GUILD_SCORE")); err != nil {
			return fmt.Errorf("MuCastle_SIEGE_GUILDLIST: %w", err)
		}
		rep.CastleRows++
	}
	stats, err := rows(ctx, src, "MuCastle_MONEY_STATISTICS")
	if err != nil {
		return err
	}
	for _, r := range stats {
		at, ok := r.time("LOG_DATE")
		if !ok {
			at = time.Now()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO mu_castle_money_statistics (map_svr_group, log_date, money_change)
			VALUES ($1,$2,$3)`, r.num("MAP_SVR_GROUP"), at, money(r, "MONEY_CHANGE")); err != nil {
			return fmt.Errorf("MuCastle_MONEY_STATISTICS: %w", err)
		}
		rep.CastleRows++
	}
	return nil
}

// money reads a MONEY column (decimal text from SQL Server, a number in
// scripts).
func money(r Row, col string) int64 {
	switch v := r[col].(type) {
	case []byte:
		f, _ := strconv.ParseFloat(strings.TrimSpace(string(v)), 64)
		return int64(math.Round(f))
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return int64(math.Round(f))
	case float64:
		return int64(math.Round(v))
	}
	return r.num(col)
}
