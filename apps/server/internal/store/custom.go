package store

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CustomCommandStore replaces the DataServer requests of the custom
// commands 50..65 and /minhainfo (docs/server-plan/10e-customs.md):
// DS 0x0F 02 (divorce of an offline partner), DS 0x0F 03
// (GiftData), DS 0x0F 0x23 (account info) and DS 0x13 00 / 0x30
// (CustomPick load / save).
type CustomCommandStore interface {
	// ClearMarriage is DS 0x0F 02 (FUN_004089e0): "UPDATE Character SET
	// Married=0, MarryCouple=NULL WHERE Name=name".
	ClearMarriage(ctx context.Context, name string) error
	// TakeGift is DS 0x0F 03 (FUN_00408a70): the GiftData row of (name,
	// index) is created with Count 0 when missing; when maxCount is -1 or
	// Count < maxCount, Count+1 is stored and the result is true.
	TakeGift(ctx context.Context, name string, index, maxCount int) (bool, error)
	// LoadPick is DS 0x13 00 (FUN_00409480): a missing row reads as the
	// zero PickData.
	LoadPick(ctx context.Context, name string) (PickData, error)
	// SavePick is DS 0x13 0x30 (FUN_004097b0): UPDATE or INSERT.
	SavePick(ctx context.Context, name string, d PickData) error
	// AccountInfo is DS 0x0F 0x23 (FUN_00401740): MEMB_INFO.Credits,
	// MEMB_STAT.OnlineHours and the seconds of the current session.
	AccountInfo(ctx context.Context, login string) (AccountInfo, error)
}

// PickSlots and PickTextSize are the 20 x 20-byte CustomPickList filters.
const (
	PickSlots    = 20
	PickTextSize = 20
)

// PickData is a CustomPick row: CustomPickList (varbinary 400) and the
// Money / Jewel / ItemExc / ItemSet switches.
type PickData struct {
	List                   [PickSlots * PickTextSize]byte
	Money, Jewel, Exc, Set int
}

// AccountInfo is the reply of DS 0x0F 0x23.
type AccountInfo struct {
	Credits        int64
	OnlineHours    int
	SessionSeconds int
}

// --- memory -------------------------------------------------------------------------

type memCustom struct {
	gifts   map[string]int // lower(name) + "\x00" + index
	picks   map[string]PickData
	credits map[int64]int64
	bank    map[string][JewelBankTypes]int // lower(login), cashshop.go
}

func (m *Memory) cust() *memCustom {
	if m.custom == nil {
		m.custom = &memCustom{gifts: map[string]int{}, picks: map[string]PickData{}, credits: map[int64]int64{}}
	}
	return m.custom
}

func giftKey(name string, index int) string {
	return strings.ToLower(name) + "\x00" + strconv.Itoa(index)
}

func (m *Memory) ClearMarriage(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.chars {
		if strings.EqualFold(c.Name, name) {
			c.Married, c.MarryCouple = false, ""
		}
	}
	return nil
}

func (m *Memory) TakeGift(_ context.Context, name string, index, maxCount int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.cust()
	k := giftKey(name, index)
	n := c.gifts[k]
	if maxCount == -1 || n < maxCount {
		c.gifts[k] = n + 1
		return true, nil
	}
	c.gifts[k] = n
	return false, nil
}

func (m *Memory) LoadPick(_ context.Context, name string) (PickData, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cust().picks[strings.ToLower(name)], nil
}

func (m *Memory) SavePick(_ context.Context, name string, d PickData) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cust().picks[strings.ToLower(name)] = d
	return nil
}

func (m *Memory) AccountInfo(_ context.Context, login string) (AccountInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, a := range m.accounts {
		if strings.EqualFold(a.Login, login) {
			// The memory store keeps no session times: OnlineHours and
			// SessionSeconds read 0.
			return AccountInfo{Credits: m.cust().credits[id]}, nil
		}
	}
	return AccountInfo{}, ErrNotFound
}

// SetCredits sets MEMB_INFO.Credits of an in-memory account (tests).
func (m *Memory) SetCredits(id int64, n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cust().credits[id] = n
}

// renameCustom moves the GiftData and CustomPick rows (WZ_RenameCharacter).
func (m *Memory) renameCustom(oldName, newName string) {
	if m.custom == nil {
		return
	}
	c := m.custom
	old := strings.ToLower(oldName) + "\x00"
	moved := map[string]int{}
	for k, v := range c.gifts {
		if strings.HasPrefix(k, old) {
			delete(c.gifts, k)
			moved[strings.ToLower(newName)+k[len(old)-1:]] = v // keeps "\x00index"
		}
	}
	for k, v := range moved {
		c.gifts[k] = v
	}
	if p, ok := c.picks[strings.ToLower(oldName)]; ok {
		delete(c.picks, strings.ToLower(oldName))
		c.picks[strings.ToLower(newName)] = p
	}
}

// deleteCustom removes the GiftData and CustomPick rows of a deleted
// character (WZ_DeleteCharacter).
func (m *Memory) deleteCustom(name string) {
	if m.custom == nil {
		return
	}
	prefix := strings.ToLower(name) + "\x00"
	for k := range m.custom.gifts {
		if strings.HasPrefix(k, prefix) {
			delete(m.custom.gifts, k)
		}
	}
	delete(m.custom.picks, strings.ToLower(name))
}

// --- postgres -----------------------------------------------------------------------

func (p *Postgres) ClearMarriage(ctx context.Context, name string) error {
	_, err := p.pool.Exec(ctx, `UPDATE characters SET married=false, marry_couple=NULL WHERE name=$1`, name)
	return err
}

func (p *Postgres) TakeGift(ctx context.Context, name string, index, maxCount int) (bool, error) {
	var ok bool
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO gift_data(name, idx, count) VALUES ($1, $2, 0)
			ON CONFLICT (name, idx) DO NOTHING`, name, index); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count FROM gift_data WHERE name=$1 AND idx=$2 FOR UPDATE`,
			name, index).Scan(&n); err != nil {
			return err
		}
		if maxCount != -1 && n >= maxCount {
			return nil
		}
		ok = true
		_, err := tx.Exec(ctx, `UPDATE gift_data SET count=count+1 WHERE name=$1 AND idx=$2`, name, index)
		return err
	})
	return ok, err
}

func (p *Postgres) LoadPick(ctx context.Context, name string) (PickData, error) {
	var d PickData
	var list []byte
	err := p.pool.QueryRow(ctx, `SELECT list, money, jewel, item_exc, item_set FROM custom_pick WHERE name=$1`, name).
		Scan(&list, &d.Money, &d.Jewel, &d.Exc, &d.Set)
	if errors.Is(err, pgx.ErrNoRows) {
		return PickData{}, nil
	}
	copy(d.List[:], list)
	return d, err
}

func (p *Postgres) SavePick(ctx context.Context, name string, d PickData) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO custom_pick(name, list, money, jewel, item_exc, item_set)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (name) DO UPDATE SET list=$2, money=$3, jewel=$4, item_exc=$5, item_set=$6`,
		name, d.List[:], d.Money, d.Jewel, d.Exc, d.Set)
	return err
}

func (p *Postgres) AccountInfo(ctx context.Context, login string) (AccountInfo, error) {
	var info AccountInfo
	err := p.pool.QueryRow(ctx, `SELECT a.credits, coalesce(s.online_hours, 0),
		CASE WHEN s.online AND s.connected_at <= now() THEN floor(extract(epoch FROM now()-s.connected_at))::int ELSE 0 END
		FROM accounts a LEFT JOIN account_sessions s ON s.account_id = a.id WHERE a.login=$1`, login).
		Scan(&info.Credits, &info.OnlineHours, &info.SessionSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountInfo{}, ErrNotFound
	}
	return info, err
}
