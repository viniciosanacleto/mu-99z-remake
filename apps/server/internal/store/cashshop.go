package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CreditStore replaces the DataServer requests of CCustomXShop
// (docs/server-plan/10e-customs.md) on MEMB_INFO.Credits
// (accounts.credits).
type CreditStore interface {
	// Credits is DS 0x0F 0x20 (FUN_00401040): "SELECT ISNULL(Credits,0)".
	Credits(ctx context.Context, login string) (int64, error)
	// DebitCredits is DS 0x0F 0x21 (FUN_00401300): "UPDATE MEMB_INFO SET
	// Credits=Credits-amount OUTPUT inserted.Credits WHERE ... AND
	// Credits>=amount". When nothing was debited ok is false and balance
	// is the current credits (the DS reads them again). An amount of 0 is
	// refused with a balance of 0.
	DebitCredits(ctx context.Context, login string, amount int64) (ok bool, balance int64, err error)
	// RefundCredits is DS 0x0F 0x22 (FUN_00401610): "UPDATE MEMB_INFO SET
	// Credits=Credits+amount". An amount of 0 does nothing.
	RefundCredits(ctx context.Context, login string, amount int64) error
}

// JewelBankTypes is the number of CustomJewelBank columns: Chaos, Life,
// Soul, Bless, Creation, Guardian, Harmony, GemStone, LowStone, HighStone.
const JewelBankTypes = 10

// JewelBankStore replaces DS 0xF7 04 / 05 of CCustomJewelBank (the
// CustomJewelBank table, one row per account).
type JewelBankStore interface {
	// JewelBank is DS 0xF7 05 (FUN_00410510): a missing row reads as 0.
	JewelBank(ctx context.Context, login string) ([JewelBankTypes]int, error)
	// AddJewelBank is DS 0xF7 04 (FUN_00410280): the column of typ gets
	// += delta, or a new row is inserted with delta in that column.
	AddJewelBank(ctx context.Context, login string, typ, delta int) error
}

// jewelBankColumns are the CustomJewelBank columns in type order.
var jewelBankColumns = [JewelBankTypes]string{"chaos", "life", "soul", "bless", "creation", "guardian",
	"harmony", "gem_stone", "low_stone", "high_stone"}

// --- memory -------------------------------------------------------------------------

func (m *Memory) accountIDByLogin(login string) (int64, bool) {
	for id, a := range m.accounts {
		if strings.EqualFold(a.Login, login) {
			return id, true
		}
	}
	return 0, false
}

func (m *Memory) Credits(_ context.Context, login string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.accountIDByLogin(login)
	if !ok {
		return 0, ErrNotFound
	}
	return m.cust().credits[id], nil
}

func (m *Memory) DebitCredits(_ context.Context, login string, amount int64) (bool, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.accountIDByLogin(login)
	if !ok || amount <= 0 {
		return false, 0, nil
	}
	c := m.cust()
	if c.credits[id] < amount {
		return false, c.credits[id], nil
	}
	c.credits[id] -= amount
	return true, c.credits[id], nil
}

func (m *Memory) RefundCredits(_ context.Context, login string, amount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.accountIDByLogin(login); ok && amount > 0 {
		m.cust().credits[id] += amount
	}
	return nil
}

func (m *Memory) JewelBank(_ context.Context, login string) ([JewelBankTypes]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cust().bank[strings.ToLower(login)], nil
}

func (m *Memory) AddJewelBank(_ context.Context, login string, typ, delta int) error {
	if typ < 0 || typ >= JewelBankTypes {
		return fmt.Errorf("jewel bank type %d", typ)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.cust()
	if c.bank == nil {
		c.bank = map[string][JewelBankTypes]int{}
	}
	k := strings.ToLower(login)
	v := c.bank[k]
	v[typ] += delta
	c.bank[k] = v
	return nil
}

// --- postgres -----------------------------------------------------------------------

func (p *Postgres) Credits(ctx context.Context, login string) (int64, error) {
	var n int64
	err := p.pool.QueryRow(ctx, `SELECT credits FROM accounts WHERE login=$1`, login).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return n, err
}

func (p *Postgres) DebitCredits(ctx context.Context, login string, amount int64) (bool, int64, error) {
	if amount <= 0 {
		return false, 0, nil
	}
	var n int64
	err := p.pool.QueryRow(ctx, `UPDATE accounts SET credits=credits-$2 WHERE login=$1 AND credits>=$2
		RETURNING credits`, login, amount).Scan(&n)
	if err == nil {
		return true, n, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, 0, err
	}
	n, err = p.Credits(ctx, login)
	if errors.Is(err, ErrNotFound) {
		return false, 0, nil
	}
	return false, n, err
}

func (p *Postgres) RefundCredits(ctx context.Context, login string, amount int64) error {
	if amount <= 0 {
		return nil
	}
	_, err := p.pool.Exec(ctx, `UPDATE accounts SET credits=credits+$2 WHERE login=$1`, login, amount)
	return err
}

func (p *Postgres) JewelBank(ctx context.Context, login string) ([JewelBankTypes]int, error) {
	var v [JewelBankTypes]int
	err := p.pool.QueryRow(ctx, `SELECT chaos, life, soul, bless, creation, guardian, harmony, gem_stone,
		low_stone, high_stone FROM custom_jewel_bank WHERE account=$1`, login).
		Scan(&v[0], &v[1], &v[2], &v[3], &v[4], &v[5], &v[6], &v[7], &v[8], &v[9])
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	return v, err
}

func (p *Postgres) AddJewelBank(ctx context.Context, login string, typ, delta int) error {
	if typ < 0 || typ >= JewelBankTypes {
		return fmt.Errorf("jewel bank type %d", typ)
	}
	col := jewelBankColumns[typ]
	_, err := p.pool.Exec(ctx, `INSERT INTO custom_jewel_bank (account, `+col+`) VALUES ($1, $2)
		ON CONFLICT (account) DO UPDATE SET `+col+` = custom_jewel_bank.`+col+` + $2`, login, delta)
	return err
}
