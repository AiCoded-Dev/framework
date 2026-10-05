package run

import (
	"context"
	"database/sql"
)

// Count returns the number of rows in table.
func Count(ctx context.Context, db *sql.DB, table string) *sql.Row {
	return db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table)
}

// rows runs queries whose rows have a type of their own.
type rows[R any] interface {
	QueryContext(ctx context.Context, query string, args ...any) (R, error)
}

// First runs constant SQL through rows.
func First[R any](ctx context.Context, q rows[R]) (R, error) { return q.QueryContext(ctx, "SELECT 1") }

// finder has a method named like one that takes SQL, whose signature lint cannot match.
type finder[T any] interface{ Query(filter T) ([]T, error) }

// Match returns the values that match filter.
func Match[T any](f finder[T], filter T) ([]T, error) { return f.Query(filter) }

// Open opens the database at dsn and reaches its driver.
func Open(ctx context.Context, dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	_ = db.Driver()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	return conn.Raw(func(any) error { return nil })
}
