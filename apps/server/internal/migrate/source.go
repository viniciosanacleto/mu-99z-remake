// Package migrate copies the original MuOnline SQL Server database
// (.re/MuOnline.sql schema, DataServer/JoinServer tables) into the
// PostgreSQL schema of internal/store.
package migrate

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Row is one table row by column name. Values are nil, int64, float64,
// string, []byte or time.Time.
type Row map[string]any

// Source reads whole tables of the original database. A missing table
// yields no rows and no error.
type Source interface {
	Rows(ctx context.Context, table string) ([]Row, error)
}

func (r Row) str(col string) string {
	switch v := r[col].(type) {
	case string:
		return strings.TrimRight(v, " \x00")
	case []byte:
		return strings.TrimRight(string(v), " \x00")
	case int64:
		return strconv.FormatInt(v, 10)
	}
	return ""
}

func (r Row) num(col string) int64 {
	switch v := r[col].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case bool:
		if v {
			return 1
		}
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	case []byte:
		n, _ := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
		return n
	}
	return 0
}

func (r Row) float(col string) float64 {
	switch v := r[col].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	}
	return 0
}

func (r Row) bytes(col string) []byte {
	if v, ok := r[col].([]byte); ok {
		return v
	}
	return nil
}

func (r Row) time(col string) (time.Time, bool) {
	t, ok := r[col].(time.Time)
	return t, ok && !t.IsZero()
}

func (r Row) null(col string) bool { return r[col] == nil }

// fixed returns b padded with pad (or cut) to n bytes, as the DataServer
// does when a varbinary column is shorter than the client block.
func fixed(b []byte, n int, pad byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = pad
	}
	copy(out, b)
	return out
}
