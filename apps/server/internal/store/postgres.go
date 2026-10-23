package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres implements Store on PostgreSQL. All queries are parameterized;
// multi-table operations run in transactions.
type Postgres struct {
	pool *pgxpool.Pool
}

// OpenPostgres connects and applies pending migrations.
func OpenPostgres(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	p := &Postgres{pool: pool}
	if err := p.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return p, nil
}

func (p *Postgres) Close() { p.pool.Close() }

func (p *Postgres) migrate(ctx context.Context) error {
	if _, err := p.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var exists bool
		if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := migrationFS.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

const accountColumns = `id, login, password_hash, personal_id, block_code, account_level,
	coalesce(level_expires_at, 'epoch'::timestamptz), lock_code, ext_class, move_count`

func scanAccount(row pgx.Row) (*Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Login, &a.PasswordHash, &a.PersonalID, &a.BlockCode, &a.Level,
		&a.LevelExpiresAt, &a.Lock, &a.ExtClass, &a.MoveCount)
	if err != nil {
		return nil, notFound(err)
	}
	return &a, nil
}

func (p *Postgres) AccountByLogin(ctx context.Context, login string) (*Account, error) {
	return scanAccount(p.pool.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE login=$1`, login))
}

func (p *Postgres) CreateAccount(ctx context.Context, login, hash, personalID string) (*Account, error) {
	a, err := scanAccount(p.pool.QueryRow(ctx,
		`INSERT INTO accounts(login, password_hash, personal_id) VALUES ($1,$2,$3) RETURNING `+accountColumns,
		login, hash, personalID))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil, ErrDuplicate
	}
	return a, err
}

func (p *Postgres) execOne(ctx context.Context, sql string, args ...any) error {
	tag, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) SetPasswordHash(ctx context.Context, id int64, hash string) error {
	return p.execOne(ctx, `UPDATE accounts SET password_hash=$2 WHERE id=$1`, id, hash)
}

func (p *Postgres) SetAccountLevel(ctx context.Context, id int64, level int, expires time.Time) error {
	return p.execOne(ctx, `UPDATE accounts SET account_level=$2, level_expires_at=$3 WHERE id=$1`, id, level, expires)
}

func (p *Postgres) SetAccountLock(ctx context.Context, id int64, lock int) error {
	return p.execOne(ctx, `UPDATE accounts SET lock_code=$2 WHERE id=$1`, id, lock)
}

func (p *Postgres) SetExtClass(ctx context.Context, id int64, ext int) error {
	return p.execOne(ctx, `UPDATE accounts SET ext_class=$2 WHERE id=$1`, id, ext)
}

func (p *Postgres) ConnectAccount(ctx context.Context, id int64, server, ip string) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO account_sessions(account_id, online, server_name, ip, connected_at)
		VALUES ($1, true, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE SET online=true, server_name=$2, ip=$3, connected_at=now()`, id, server, ip)
	return err
}

// DisconnectAccount follows WZ_DISCONNECT_MEMB: OnlineHours += hours online.
func (p *Postgres) DisconnectAccount(ctx context.Context, id int64) error {
	_, err := p.pool.Exec(ctx, `UPDATE account_sessions SET online=false, disconnected_at=now(),
		online_hours = online_hours + coalesce(floor(extract(epoch FROM now()-connected_at)/3600),0)::int
		WHERE account_id=$1`, id)
	return err
}

func (p *Postgres) ResetSessions(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, `UPDATE account_sessions SET online=false WHERE online`)
	return err
}

func (p *Postgres) ListCharacters(ctx context.Context, accountID int64) ([]CharacterSummary, error) {
	rows, err := p.pool.Query(ctx, `SELECT c.slot, c.name, c.level, c.class, c.ctl_code,
			substring(c.inventory FROM 1 FOR $2), coalesce(g.status, $3)
		FROM characters c LEFT JOIN guild_members g ON g.name = c.name
		WHERE c.account_id=$1 ORDER BY c.slot`, accountID, SummaryItems*ItemDBSize, GuildStatusNone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharacterSummary
	for rows.Next() {
		var s CharacterSummary
		var class, ctl, gs int16
		if err := rows.Scan(&s.Slot, &s.Name, &s.Level, &class, &ctl, &s.Inventory, &gs); err != nil {
			return nil, err
		}
		s.Class, s.CtlCode, s.GuildStatus = byte(class), byte(ctl), byte(gs)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) CharacterExists(ctx context.Context, name string) (bool, error) {
	var ok bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM characters WHERE name=$1)`, name).Scan(&ok)
	return ok, err
}

// CreateCharacter reproduces WZ_CreateCharacter in one transaction.
func (p *Postgres) CreateCharacter(ctx context.Context, accountID int64, name string, class byte) (CreateResult, int, error) {
	result, slot := CreateError, 0
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		// Serialize creations per account (slot choice).
		if _, err := tx.Exec(ctx, `SELECT 1 FROM accounts WHERE id=$1 FOR UPDATE`, accountID); err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM characters WHERE name=$1)`, name).Scan(&taken); err != nil {
			return err
		}
		if taken {
			result = CreateNameExists
			return nil
		}
		err := tx.QueryRow(ctx, `SELECT s FROM generate_series(0,4) s
			WHERE s NOT IN (SELECT slot FROM characters WHERE account_id=$1) ORDER BY s LIMIT 1`, accountID).Scan(&slot)
		if errors.Is(err, pgx.ErrNoRows) {
			result = CreateNoSlot
			return nil
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO characters(account_id, slot, name, class, level, level_up_points,
				strength, dexterity, vitality, energy, leadership, inventory, magic_list, life, max_life, mana, max_mana,
				map, pos_x, pos_y, quest, effect_list)
			SELECT $1, $2, $3, $4, level, level_up_points, strength, dexterity, vitality, energy, leadership,
				inventory, magic_list, life, max_life, mana, max_mana, map, pos_x, pos_y, quest, effect_list
			FROM default_class_templates WHERE class=$4`, accountID, slot, name, int16(class))
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				result = CreateNameExists
				return nil
			}
			return err
		}
		if tag.RowsAffected() == 1 {
			result = CreateOK
		}
		return nil
	})
	return result, slot, err
}

// DeleteCharacter replaces WZ_DeleteCharacter; dependent rows go with ON
// DELETE CASCADE, inside one statement.
func (p *Postgres) DeleteCharacter(ctx context.Context, accountID int64, name string) (bool, error) {
	var ok bool
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM characters WHERE account_id=$1 AND name=$2`, accountID, name)
		if err != nil {
			return err
		}
		ok = tag.RowsAffected() == 1
		if ok {
			_, err = tx.Exec(ctx, `DELETE FROM guild_members WHERE name=$1`, name)
		}
		if ok && err == nil {
			// WZ_DeleteCharacter also deletes CustomPick and GiftData (custom.go).
			_, err = tx.Exec(ctx, `DELETE FROM custom_pick WHERE name=$1`, name)
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM gift_data WHERE name=$1`, name)
			}
		}
		return err
	})
	return ok, err
}

const characterColumns = `id, account_id, slot, name, class, level, level_up_points, experience,
	strength, dexterity, vitality, energy, leadership, money, life, max_life, mana, max_mana, bp, max_bp,
	shield, max_shield, map, pos_x, pos_y, dir, pk_count, pk_level, pk_time, ctl_code,
	inventory, magic_list, effect_list, quest, chat_limit_time, fruit_add_points, fruit_sub_points,
	married, coalesce(marry_couple,''), resets, master_resets, is_new, patente`

func (p *Postgres) LoadCharacter(ctx context.Context, accountID int64, name string) (*Character, error) {
	var c Character
	var class, ctl, mp, x, y, dir int16
	err := p.pool.QueryRow(ctx, `SELECT `+characterColumns+` FROM characters WHERE account_id=$1 AND name=$2`,
		accountID, name).Scan(&c.ID, &c.AccountID, &c.Slot, &c.Name, &class, &c.Level, &c.LevelUpPoints, &c.Experience,
		&c.Strength, &c.Dexterity, &c.Vitality, &c.Energy, &c.Leadership, &c.Money, &c.Life, &c.MaxLife,
		&c.Mana, &c.MaxMana, &c.BP, &c.MaxBP, &c.Shield, &c.MaxShield, &mp, &x, &y, &dir,
		&c.PKCount, &c.PKLevel, &c.PKTime, &ctl, &c.Inventory, &c.MagicList, &c.EffectList, &c.Quest,
		&c.ChatLimitTime, &c.FruitAddPoints, &c.FruitSubPoints, &c.Married, &c.MarryCouple,
		&c.Resets, &c.MasterResets, &c.IsNew, &c.Patente)
	if err != nil {
		return nil, notFound(err)
	}
	c.Class, c.CtlCode = byte(class), byte(ctl)
	c.Map, c.X, c.Y, c.Dir = int(mp), int(x), int(y), int(dir)
	return &c, nil
}

// SaveCharacter replaces the DataServer 0x30 update.
func (p *Postgres) SaveCharacter(ctx context.Context, c *Character) error {
	var couple any
	if c.MarryCouple != "" {
		couple = c.MarryCouple
	}
	return p.execOne(ctx, `UPDATE characters SET level=$2, class=$3, level_up_points=$4, experience=$5,
		strength=$6, dexterity=$7, vitality=$8, energy=$9, leadership=$10, inventory=$11, magic_list=$12,
		money=$13, life=$14, max_life=$15, mana=$16, max_mana=$17, bp=$18, max_bp=$19, map=$20, pos_x=$21,
		pos_y=$22, dir=$23, pk_count=$24, pk_level=$25, pk_time=$26, quest=$27, chat_limit_time=$28,
		effect_list=$29, fruit_add_points=$30, fruit_sub_points=$31, married=$32, marry_couple=$33,
		ctl_code=$34, resets=$35, master_resets=$36, is_new=$37, shield=$38, max_shield=$39, last_login_at=now()
		WHERE id=$1`,
		c.ID, c.Level, int16(c.Class), c.LevelUpPoints, c.Experience, c.Strength, c.Dexterity, c.Vitality,
		c.Energy, c.Leadership, c.Inventory, c.MagicList, c.Money, c.Life, c.MaxLife, c.Mana, c.MaxMana,
		c.BP, c.MaxBP, int16(c.Map), int16(c.X), int16(c.Y), int16(c.Dir), c.PKCount, c.PKLevel, c.PKTime,
		c.Quest, c.ChatLimitTime, c.EffectList, c.FruitAddPoints, c.FruitSubPoints, c.Married, couple,
		int16(c.CtlCode), c.Resets, c.MasterResets, c.IsNew, c.Shield, c.MaxShield)
}

func (p *Postgres) SaveInventory(ctx context.Context, id int64, inv []byte) error {
	return p.execOne(ctx, `UPDATE characters SET inventory=$2 WHERE id=$1`, id, inv)
}

// LoadWarehouse creates an empty vault on first use, as DataServer 05/00 does.
func (p *Postgres) LoadWarehouse(ctx context.Context, accountID int64, number int) (*Warehouse, error) {
	w := Warehouse{AccountID: accountID, Number: number}
	var num int16
	err := p.pool.QueryRow(ctx, `INSERT INTO warehouses(account_id, number, items) VALUES ($1,$2,$3)
		ON CONFLICT (account_id, number) DO UPDATE SET account_id=EXCLUDED.account_id
		RETURNING number, items, money, password`, accountID, number, EmptyItems(WarehouseSize/ItemDBSize)).
		Scan(&num, &w.Items, &w.Money, &w.Password)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (p *Postgres) SaveWarehouse(ctx context.Context, w *Warehouse) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO warehouses(account_id, number, items, money, password) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (account_id, number) DO UPDATE SET items=$3, money=$4, password=$5`,
		w.AccountID, w.Number, w.Items, w.Money, w.Password)
	return err
}

func (p *Postgres) LoadOptions(ctx context.Context, id int64) (*Options, error) {
	var o Options
	var g, q, w, e, cw int16
	err := p.pool.QueryRow(ctx, `SELECT skill_keys, game_option, q_key, w_key, e_key, chat_window
		FROM character_options WHERE character_id=$1`, id).Scan(&o.SkillKeys, &g, &q, &w, &e, &cw)
	if err != nil {
		return nil, notFound(err)
	}
	o.GameOption, o.QKey, o.WKey, o.EKey, o.ChatWindow = byte(g), byte(q), byte(w), byte(e), byte(cw)
	return &o, nil
}

func (p *Postgres) SaveOptions(ctx context.Context, id int64, o *Options) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO character_options(character_id, skill_keys, game_option, q_key, w_key, e_key, chat_window)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (character_id) DO UPDATE SET skill_keys=$2, game_option=$3,
		q_key=$4, w_key=$5, e_key=$6, chat_window=$7`,
		id, o.SkillKeys, int16(o.GameOption), int16(o.QKey), int16(o.WKey), int16(o.EKey), int16(o.ChatWindow))
	return err
}

func (p *Postgres) NextItemSerial(ctx context.Context) (uint32, error) {
	var v int64
	err := p.pool.QueryRow(ctx, `SELECT nextval('item_serial_seq')`).Scan(&v)
	return uint32(v), err
}
