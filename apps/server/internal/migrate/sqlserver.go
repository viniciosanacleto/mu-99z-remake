package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/microsoft/go-mssqldb" // sqlserver driver
)

// SQLServerSource reads a live MuOnline database, e.g.
// "sqlserver://sa:pass@host:1433?database=MuOnline".
type SQLServerSource struct {
	db *sql.DB
}

// OpenSQLServer connects to the original database.
func OpenSQLServer(ctx context.Context, url string) (*SQLServerSource, error) {
	db, err := sql.Open("sqlserver", url)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLServerSource{db: db}, nil
}

// Close closes the connection.
func (s *SQLServerSource) Close() error { return s.db.Close() }

// Rows implements Source: SELECT * of a dbo table (no rows when the table
// does not exist in this database version).
func (s *SQLServerSource) Rows(ctx context.Context, table string) ([]Row, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = 'dbo' AND TABLE_NAME = @p1", table).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM [dbo].[%s]", strings.ReplaceAll(table, "]", "")))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := Row{}
		for i, c := range cols {
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
