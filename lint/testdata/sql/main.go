// Command sql is an app whose code runs SQL.
package main

import (
	"context"
	"database/sql"
	"fmt"
)

const columns = "id, body"

// querier is what *sql.DB and *sql.Tx share.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func main() {}

// run runs SQL that is a constant, and SQL that is not.
func run(ctx context.Context, db *sql.DB, tx *sql.Tx, q querier, table string) {
	_, _ = db.ExecContext(ctx, "INSERT INTO notes (body) VALUES (?)", "x")
	_, _ = db.Exec("UPDATE notes SET body = ? WHERE id = ?", "x", 1)
	_ = db.QueryRowContext(ctx, "SELECT "+columns+" FROM notes WHERE id = ?", 1)
	_, _ = tx.ExecContext(ctx, "DELETE FROM `notes` WHERE id = ?", 1)
	_, _ = (*sql.DB).ExecContext(db, ctx, "/* tags */ REPLACE INTO app.tags (name) VALUES (?)", "x")
	_, _ = db.ExecContext(ctx, "DELETE FROM "+table)
	_, _ = q.QueryContext(ctx, "SELECT * FROM "+table)
	_, _ = tx.Prepare(fmt.Sprintf("SELECT %d", 1))
	query := db.QueryContext
	_, _ = query(ctx, "SELECT 1")
}

// rows is a querier whose rows have a type of their own.
type rows[R any] interface {
	QueryContext(ctx context.Context, query string, args ...any) (R, error)
}

// store holds the database.
type store struct{ *sql.DB }

// cache queries through its field.
type cache[R any] struct{ q rows[R] }

// forms runs SQL that is not a constant in every form lint reads.
func forms(ctx context.Context, db *sql.DB, s store, table string) {
	_, _ = (*sql.DB).ExecContext(db, ctx, "DELETE FROM "+table)
	_, _ = s.QueryContext(ctx, "SELECT * FROM "+table)
	_, _ = interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	}(db).QueryContext(ctx, table)
	_, _ = db.QueryContext(statement(table))
	_, _ = (*sql.DB).QueryContext(on(db, table))
	_, _ = viaRows[*sql.Rows](ctx, db, table)
	_, _ = viaConstraint[*sql.Rows](ctx, db, table)
	cache[*sql.Rows]{q: db}.find(ctx, table)
	_, _ = db.ExecContext(ctx, "/*! DELETE FROM notes */ SELECT 1")
	_, _ = db.ExecContext(ctx, "SELECT 1 /*M! , 2 */")
}

func statement(table string) (context.Context, string) {
	return context.Background(), "SELECT * FROM " + table
}

func on(db *sql.DB, table string) (*sql.DB, context.Context, string) {
	return db, context.Background(), table
}

func viaRows[R any](ctx context.Context, q rows[R], table string) (R, error) {
	return q.QueryContext(ctx, table)
}

func viaConstraint[R any, Q interface {
	QueryContext(context.Context, string, ...any) (R, error)
}](ctx context.Context, q Q, table string) (R, error) {
	return q.QueryContext(ctx, table)
}

func (c cache[R]) find(ctx context.Context, table string) { _, _ = c.q.QueryContext(ctx, table) }

// finder has a method named like one that takes SQL, whose signature lint cannot match.
type finder[T any] interface{ Query(filter T) ([]T, error) }

func find[T any](f finder[T], filter T) { _, _ = f.Query(filter) }

// keep runs constant SQL through the same forms.
func keep(ctx context.Context, q rows[*sql.Rows], s store) {
	_, _ = viaRowsConstant(ctx, q)
	_, _ = s.QueryContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(1000) */ * FROM notes")
}

func viaRowsConstant[R any](ctx context.Context, q rows[R]) (R, error) {
	return q.QueryContext(ctx, "WITH old AS (SELECT id FROM drafts) DELETE FROM drafts WHERE id IN (SELECT id FROM old)")
}

// kinds runs constant statements of the kinds lint allows and of kinds it refuses.
func kinds(ctx context.Context, db *sql.DB) {
	_, _ = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS notes (id INT PRIMARY KEY)")
	_, _ = db.ExecContext(ctx, "ALTER TABLE notes ADD COLUMN body TEXT")
	_, _ = db.ExecContext(ctx, "CREATE INDEX notes_body ON notes (body)")
	_, _ = db.ExecContext(ctx, "CREATE UNIQUE INDEX notes_id ON notes (id)")
	_, _ = db.ExecContext(ctx, "DROP INDEX notes_body ON notes")
	_, _ = db.ExecContext(ctx, "DROP TABLE notes")
	_ = db.QueryRowContext(ctx, "(SELECT 1)")
	_, _ = db.ExecContext(ctx, "SET @q = 'x'")
	_, _ = db.ExecContext(ctx, "PREPARE s FROM @q")
	_, _ = db.ExecContext(ctx, "EXECUTE s")
	_, _ = db.ExecContext(ctx, "CALL do_it()")
	_, _ = db.ExecContext(ctx, "TRUNCATE TABLE notes")
	_, _ = db.ExecContext(ctx, "LOAD DATA INFILE 'x' INTO TABLE notes")
}

// statements runs constant SQL that holds more than one statement or reaches a file of the
// database server, and SQL that only looks so.
func statements(ctx context.Context, db *sql.DB) {
	_, _ = db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?; DROP TABLE notes", 1)
	_, _ = db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?;\n;DELETE FROM notes", 1)
	_ = db.QueryRowContext(ctx, "SELECT body FROM notes INTO OUTFILE '/tmp/notes'")
	_ = db.QueryRowContext(ctx, "select body from notes into /* c */ dumpfile '/tmp/notes'")
	_ = db.QueryRowContext(ctx, "SELECT load_file ('/etc/passwd')")
	_, _ = db.ExecContext(ctx, "DELETE FROM drafts WHERE id = ?;", 1)
	_, _ = db.ExecContext(ctx, "DELETE FROM drafts WHERE body = ';' -- ; DROP TABLE notes\n;\n", 1)
	_ = db.QueryRowContext(ctx, "SELECT body INTO @body FROM notes WHERE body <> 'INTO OUTFILE' /* LOAD_FILE */")
}
