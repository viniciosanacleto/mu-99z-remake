package store

import (
	"bytes"
	"context"
	"sort"
	"strings"
)

// ReconnectDataSize is the Data column of ReconnectData (binary(400)).
const ReconnectDataSize = 400

// ReconnectRow is one ReconnectData row.
type ReconnectRow struct {
	Name       string
	ServerCode int
	Data       []byte
	Time       int
}

// ReconnectStore replaces the ReconnectData queries of the DataServer:
//   - SaveReconnect is C0 00 (FUN_0041a4e0): "SELECT Name FROM
//     ReconnectData WHERE Name=" then, when a row exists, "UPDATE
//     ReconnectData SET Data=? WHERE Name=" (every row of the name, the
//     server code and time are kept), else "INSERT ... (Name, ServerCode,
//     Data, Time)";
//   - DeleteReconnect is C0 01 (FUN_0041a760): "DELETE ... WHERE Name=
//     AND ServerCode=";
//   - ReconnectRows is the replay sent to a game server when it connects
//     (FUN_0041a7f0): "SELECT * FROM ReconnectData WHERE ServerCode=".
type ReconnectStore interface {
	SaveReconnect(ctx context.Context, r ReconnectRow) error
	DeleteReconnect(ctx context.Context, name string, serverCode int) error
	ReconnectRows(ctx context.Context, serverCode int) ([]ReconnectRow, error)
}

func reconnectData(b []byte) []byte {
	d := make([]byte, ReconnectDataSize)
	copy(d, b)
	return d
}

type reconnectKey struct {
	name string // lower case
	code int
}

type memReconnect map[reconnectKey]*ReconnectRow

func (m *Memory) reconnectTable() memReconnect {
	if m.reconnect == nil {
		m.reconnect = memReconnect{}
	}
	return m.reconnect
}

func (m *Memory) SaveReconnect(_ context.Context, r ReconnectRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.reconnectTable()
	found := false
	for k, row := range t {
		if k.name == strings.ToLower(r.Name) {
			row.Data = reconnectData(r.Data)
			found = true
		}
	}
	if !found {
		t[reconnectKey{strings.ToLower(r.Name), r.ServerCode}] = &ReconnectRow{Name: r.Name, ServerCode: r.ServerCode,
			Data: reconnectData(r.Data), Time: r.Time}
	}
	return nil
}

func (m *Memory) DeleteReconnect(_ context.Context, name string, serverCode int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reconnectTable(), reconnectKey{strings.ToLower(name), serverCode})
	return nil
}

func (m *Memory) ReconnectRows(_ context.Context, serverCode int) ([]ReconnectRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ReconnectRow
	for _, row := range m.reconnectTable() {
		if row.ServerCode == serverCode {
			out = append(out, ReconnectRow{Name: row.Name, ServerCode: row.ServerCode, Data: bytes.Clone(row.Data),
				Time: row.Time})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (p *Postgres) SaveReconnect(ctx context.Context, r ReconnectRow) error {
	tag, err := p.pool.Exec(ctx, `UPDATE reconnect_data SET data=$2 WHERE name=$1`, r.Name, reconnectData(r.Data))
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO reconnect_data (name, server_code, data, time) VALUES ($1,$2,$3,$4)
		ON CONFLICT (name, server_code) DO UPDATE SET data = EXCLUDED.data`,
		r.Name, r.ServerCode, reconnectData(r.Data), r.Time)
	return err
}

func (p *Postgres) DeleteReconnect(ctx context.Context, name string, serverCode int) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM reconnect_data WHERE name=$1 AND server_code=$2`, name, serverCode)
	return err
}

func (p *Postgres) ReconnectRows(ctx context.Context, serverCode int) ([]ReconnectRow, error) {
	rows, err := p.pool.Query(ctx, `SELECT name, server_code, data, time FROM reconnect_data
		WHERE server_code=$1 ORDER BY name`, serverCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReconnectRow
	for rows.Next() {
		var r ReconnectRow
		if err := rows.Scan(&r.Name, &r.ServerCode, &r.Data, &r.Time); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
