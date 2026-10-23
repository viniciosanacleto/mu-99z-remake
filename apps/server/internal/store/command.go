package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// CommandStore replaces the DataServer requests of the WizardTeam
// commands: DS 0x0F 00 / 01 (WZ_GetResetInfo / WZ_GetMasterResetInfo and
// WZ_SetResetInfo / WZ_SetMasterResetInfo, table ResetData) and DS 0x0F 04
// (WZ_RenameCharacter).
type CommandStore interface {
	// ResetInfo is WZ_Get(Master)ResetInfo: the row is created when
	// missing and a counter whose period date is in an older day, week or
	// month reads (and is stored) as 0; the dates are not touched.
	ResetInfo(ctx context.Context, account, name string, master bool) (ResetCounts, error)
	// SetResetInfo is WZ_Set(Master)ResetInfo: the dates of the periods
	// that changed become now, the counters are written and the
	// character's ResetCount (and MasterResetCount) are updated.
	SetResetInfo(ctx context.Context, account, name string, master bool, reset, masterReset int, c ResetCounts) error
	// RenameCharacter is WZ_RenameCharacter: false when the new name is
	// taken, otherwise the character and every table keyed by its name
	// (guild, friends, event entries, reset data) follow it.
	RenameCharacter(ctx context.Context, accountID int64, account, oldName, newName string) (bool, error)
}

// ResetCounts are the ResetDay/Wek/Mon (or MasterResetDay/Wek/Mon)
// columns.
type ResetCounts struct {
	Day, Week, Month int
}

// commandNow is the GETDATE() of the procedures (tests replace it).
var commandNow = time.Now

// dayDiff, weekDiff and monthDiff are SQL Server DATEDIFF(day|week|month,
// a, b): the number of day, Sunday-week and month boundaries crossed.
func dayDiff(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

func weekDiff(a, b time.Time) int {
	sa := time.Date(a.Year(), a.Month(), a.Day()-int(a.Weekday()), 0, 0, 0, 0, time.UTC)
	sb := time.Date(b.Year(), b.Month(), b.Day()-int(b.Weekday()), 0, 0, 0, 0, time.UTC)
	return int(sb.Sub(sa).Hours() / (24 * 7))
}

func monthDiff(a, b time.Time) int {
	return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
}

// resetRow is one ResetData row (normal and master halves).
type resetRow struct {
	counts [2]ResetCounts
	dates  [2][3]time.Time // day, week, month
}

func newResetRow(now time.Time) *resetRow {
	r := &resetRow{}
	for i := range r.dates {
		r.dates[i] = [3]time.Time{now, now, now}
	}
	return r
}

func half(master bool) int {
	if master {
		return 1
	}
	return 0
}

// readCounts applies the zeroing of WZ_Get*ResetInfo.
func readCounts(c ResetCounts, d [3]time.Time, now time.Time) (ResetCounts, bool) {
	changed := false
	if dayDiff(d[0], now) > 0 {
		c.Day, changed = 0, true
	}
	if weekDiff(d[1], now) > 0 {
		c.Week, changed = 0, true
	}
	if monthDiff(d[2], now) > 0 {
		c.Month, changed = 0, true
	}
	return c, changed
}

// touchDates applies the date update of WZ_Set*ResetInfo.
func touchDates(d [3]time.Time, now time.Time) [3]time.Time {
	if dayDiff(d[0], now) > 0 {
		d[0] = now
	}
	if weekDiff(d[1], now) > 0 {
		d[1] = now
	}
	if monthDiff(d[2], now) > 0 {
		d[2] = now
	}
	return d
}

// --- memory -------------------------------------------------------------------------

func (m *Memory) resetRowFor(account, name string) *resetRow {
	if m.resets == nil {
		m.resets = map[string]*resetRow{}
	}
	k := entryKey(account, name)
	r := m.resets[k]
	if r == nil {
		r = newResetRow(commandNow())
		m.resets[k] = r
	}
	return r
}

func (m *Memory) ResetInfo(_ context.Context, account, name string, master bool) (ResetCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, h := m.resetRowFor(account, name), half(master)
	c, _ := readCounts(r.counts[h], r.dates[h], commandNow())
	r.counts[h] = c
	return c, nil
}

func (m *Memory) SetResetInfo(_ context.Context, account, name string, master bool, reset, masterReset int, c ResetCounts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, h := m.resetRowFor(account, name), half(master)
	r.dates[h] = touchDates(r.dates[h], commandNow())
	r.counts[h] = c
	for _, ch := range m.chars {
		if strings.EqualFold(ch.Name, name) && m.accountLogin(ch.AccountID, account) {
			ch.Resets = reset
			if master {
				ch.MasterResets = masterReset
			}
		}
	}
	return nil
}

func (m *Memory) accountLogin(id int64, login string) bool {
	a := m.accounts[id]
	return a != nil && strings.EqualFold(a.Login, login)
}

func (m *Memory) RenameCharacter(_ context.Context, accountID int64, account, oldName, newName string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nameTaken(newName) {
		return false, nil
	}
	for _, c := range m.chars {
		if c.AccountID == accountID && strings.EqualFold(c.Name, oldName) {
			c.Name = newName
		}
	}
	if r := m.resets[entryKey(account, oldName)]; r != nil {
		delete(m.resets, entryKey(account, oldName))
		m.resets[entryKey(account, newName)] = r
	}
	if r := m.entries[entryKey(account, oldName)]; r != nil {
		delete(m.entries, entryKey(account, oldName))
		m.entries[entryKey(account, newName)] = r
	}
	m.renameCustom(oldName, newName) // custom.go: GiftData, CustomPick
	if f := m.fr; f != nil {
		if r := f.main[strings.ToLower(oldName)]; r != nil {
			delete(f.main, strings.ToLower(oldName))
			r.name = newName
			f.main[strings.ToLower(newName)] = r
		}
		for _, l := range f.list {
			for g, rel := range l {
				if strings.EqualFold(rel.name, oldName) {
					rel.name = newName
					l[g] = rel
				}
			}
		}
		for _, l := range f.wait {
			for g, n := range l {
				if strings.EqualFold(n, oldName) {
					l[g] = newName
				}
			}
		}
		for _, l := range f.mail {
			for i, ml := range l {
				if strings.EqualFold(ml.From, oldName) {
					ml.From = newName
					l[i] = ml
				}
			}
		}
	}
	return true, nil
}

// --- postgres -----------------------------------------------------------------------

func resetCols(master bool) (counts, dates string) {
	if master {
		return "master_reset_day, master_reset_wek, master_reset_mon",
			"master_date_day, master_date_wek, master_date_mon"
	}
	return "reset_day, reset_wek, reset_mon", "reset_date_day, reset_date_wek, reset_date_mon"
}

func (p *Postgres) ResetInfo(ctx context.Context, account, name string, master bool) (ResetCounts, error) {
	var c ResetCounts
	cc, dc := resetCols(master)
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if err := insertResetRow(ctx, tx, account, name); err != nil {
			return err
		}
		var d [3]time.Time
		if err := tx.QueryRow(ctx, `SELECT `+cc+`, `+dc+` FROM reset_data WHERE account=$1 AND name=$2 FOR UPDATE`,
			account, name).Scan(&c.Day, &c.Week, &c.Month, &d[0], &d[1], &d[2]); err != nil {
			return err
		}
		var changed bool
		if c, changed = readCounts(c, d, commandNow()); !changed {
			return nil
		}
		_, err := tx.Exec(ctx, `UPDATE reset_data SET (`+cc+`) = ($3, $4, $5) WHERE account=$1 AND name=$2`,
			account, name, c.Day, c.Week, c.Month)
		return err
	})
	return c, err
}

func (p *Postgres) SetResetInfo(ctx context.Context, account, name string, master bool, reset, masterReset int, c ResetCounts) error {
	cc, dc := resetCols(master)
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if err := insertResetRow(ctx, tx, account, name); err != nil {
			return err
		}
		var d [3]time.Time
		if err := tx.QueryRow(ctx, `SELECT `+dc+` FROM reset_data WHERE account=$1 AND name=$2 FOR UPDATE`,
			account, name).Scan(&d[0], &d[1], &d[2]); err != nil {
			return err
		}
		d = touchDates(d, commandNow())
		if _, err := tx.Exec(ctx, `UPDATE reset_data SET (`+cc+`, `+dc+`) = ($3, $4, $5, $6, $7, $8)
			WHERE account=$1 AND name=$2`, account, name, c.Day, c.Week, c.Month, d[0], d[1], d[2]); err != nil {
			return err
		}
		var err error
		if master {
			_, err = tx.Exec(ctx, `UPDATE characters SET resets=$3, master_resets=$4 FROM accounts
				WHERE characters.account_id = accounts.id AND accounts.login=$1 AND characters.name=$2`,
				account, name, reset, masterReset)
		} else {
			_, err = tx.Exec(ctx, `UPDATE characters SET resets=$3 FROM accounts
				WHERE characters.account_id = accounts.id AND accounts.login=$1 AND characters.name=$2`,
				account, name, reset)
		}
		return err
	})
}

// insertResetRow creates the row with every date at commandNow (the
// in-memory store does the same in newResetRow).
func insertResetRow(ctx context.Context, tx pgx.Tx, account, name string) error {
	_, err := tx.Exec(ctx, `INSERT INTO reset_data (account, name, reset_date_day, reset_date_wek, reset_date_mon,
		master_date_day, master_date_wek, master_date_mon) VALUES ($1, $2, $3, $3, $3, $3, $3, $3)
		ON CONFLICT DO NOTHING`, account, name, commandNow())
	return err
}

func execOneTx(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) RenameCharacter(ctx context.Context, accountID int64, account, oldName, newName string) (bool, error) {
	ok := false
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM characters WHERE name=$1)`, newName).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return nil
		}
		// friend_main.name follows characters.name (ON UPDATE CASCADE).
		if err := execOneTx(ctx, tx, `UPDATE characters SET name=$2 WHERE name=$1 AND account_id=$3`,
			oldName, newName, accountID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE event_entry_count SET name=$2 WHERE name=$1 AND account=$3`,
			oldName, newName, account); err != nil {
			return err
		}
		for _, q := range []string{
			`UPDATE guilds SET master=$2 WHERE master=$1`,
			`UPDATE guild_members SET name=$2 WHERE name=$1`,
			`UPDATE reset_data SET name=$2 WHERE name=$1`,
			`UPDATE friend_list SET friend_name=$2 WHERE friend_name=$1`,
			`UPDATE friend_mail SET friend_name=$2 WHERE friend_name=$1`,
			`UPDATE wait_friend SET friend_name=$2 WHERE friend_name=$1`,
			`UPDATE gift_data SET name=$2 WHERE name=$1`,
			`UPDATE custom_pick SET name=$2 WHERE name=$1`,
		} {
			if _, err := tx.Exec(ctx, q, oldName, newName); err != nil {
				return err
			}
		}
		ok = true
		return nil
	})
	if err != nil {
		var pgErr interface{ SQLState() string }
		if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
			return false, nil // unique violation: the name was taken meanwhile
		}
		return false, err
	}
	return ok, nil
}
