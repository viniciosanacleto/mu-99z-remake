package migrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// importMonsterKillCount copies MonsterKillCount (Name, MonsterClass,
// MonsterKillCount; CustomMonsterKillCount.txt, DS 0x50) into
// monster_kill_count (migration 0017_custom_world.sql). Only rows of
// imported characters are kept; a repeated (Name, MonsterClass) keeps the
// first row.
func importMonsterKillCount(ctx context.Context, src Source, tx pgx.Tx, chars map[string]charRef, rep *Report) error {
	list, err := rows(ctx, src, "MonsterKillCount")
	if err != nil {
		return err
	}
	for _, r := range list {
		name := r.str("Name")
		if _, ok := chars[key(name)]; !ok {
			rep.skip("MonsterKillCount %q: character not found", name)
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO monster_kill_count (name, monster_class, kill_count) VALUES ($1,$2,$3)
			ON CONFLICT DO NOTHING`, name, r.num("MonsterClass"), r.num("MonsterKillCount")); err != nil {
			return fmt.Errorf("MonsterKillCount %s: %w", name, err)
		}
	}
	return nil
}

// importDailyReward copies DailyReward (acc, name, date, month), the table
// the DataServer of this build reads and writes for DailyReward.txt
// (FUN_0040c4c0 / FUN_00410980), into daily_reward. The table is not in
// the MuOnline.sql script (the DataServer expects it to exist): with no
// rows nothing is imported. DailyRewardData (Name, index, result) is only
// touched by the WZ_RenameCharacter / WZ_DeleteCharacter procedures, the
// binaries never read it, so it is not imported. Rows of unknown accounts,
// days outside 1..31 or months outside 1..12 are skipped.
func importDailyReward(ctx context.Context, src Source, tx pgx.Tx, accounts map[string]int64, rep *Report) error {
	list, err := rows(ctx, src, "DailyReward")
	if err != nil {
		return err
	}
	for _, orig := range list {
		r := lowerKeys(orig) // the script of the table is not known: any column case
		acc := r.str("acc")
		day, month := r.num("date"), r.num("month")
		if _, ok := accounts[key(acc)]; !ok {
			rep.skip("DailyReward %q: account not found", acc)
			continue
		}
		if day < 1 || day > 31 || month < 1 || month > 12 {
			rep.skip("DailyReward %q: day %d month %d", acc, day, month)
			continue
		}
		name := r.str("name")
		if len(name) > 10 {
			name = name[:10]
		}
		if _, err := tx.Exec(ctx, `INSERT INTO daily_reward (account, name, day, month) VALUES ($1,$2,$3,$4)
			ON CONFLICT DO NOTHING`, acc, name, day, month); err != nil {
			return fmt.Errorf("DailyReward %s: %w", acc, err)
		}
	}
	return nil
}

// lowerKeys returns the row with lower-case column names.
func lowerKeys(r Row) Row {
	out := make(Row, len(r))
	for k, v := range r {
		out[strings.ToLower(k)] = v
	}
	return out
}
