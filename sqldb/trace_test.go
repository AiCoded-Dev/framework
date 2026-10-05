package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

func TestExecWithoutArgs(t *testing.T) {
	f := newFixture(t)
	_, err := f.db.ExecContext(f.ctx, " create table notes (owner varchar(64) default 'alice')")
	require.NoError(t, err)

	assert.Len(t, f.conn.execs, 1)
	require.Len(t, f.spans, 1)
	assert.Equal(t, "sql.exec", f.spans[0].GetName())
	assert.Equal(t, map[string]string{"db.operation": "CREATE"}, f.spans[0].GetAttributes())
}

func TestExecWithArgsIsPrepared(t *testing.T) {
	f := newFixture(t)
	_, err := f.db.ExecContext(f.ctx, "INSERT INTO notes VALUES (?)", "alice")
	require.NoError(t, err)

	assert.Empty(t, f.conn.execs)
	assert.Equal(t, []any{"alice"}, f.conn.args)
	require.Len(t, f.spans, 1)
	assert.Equal(t, "sql.exec", f.spans[0].GetName())
	assert.Equal(t, map[string]string{"db.operation": "INSERT"}, f.spans[0].GetAttributes())
	assert.Empty(t, f.spans[0].GetError())
}

func TestQueryConvertsArgs(t *testing.T) {
	f := newFixture(t)
	rows, err := f.db.QueryContext(f.ctx, "SELECT body FROM notes WHERE room = ?", room{12})
	require.NoError(t, err)
	require.NoError(t, rows.Close())

	assert.Equal(t, []any{"room 12"}, f.conn.args)
	require.Len(t, f.spans, 1)
	assert.Equal(t, "sql.query", f.spans[0].GetName())
	assert.Equal(t, map[string]string{"db.operation": "SELECT"}, f.spans[0].GetAttributes())
}

func TestTxAndErrors(t *testing.T) {
	f := newFixture(t)
	tx, err := f.db.BeginTx(f.ctx, nil)
	require.NoError(t, err)
	f.conn.err = &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'alice' for key 'owner'"}
	_, err = tx.ExecContext(f.ctx, "INSERT INTO notes VALUES (?)", "alice")
	require.ErrorIs(t, err, f.conn.err)
	f.conn.err = errors.New("near 'alice'")
	_, err = tx.ExecContext(f.ctx, "UPDATE notes SET owner = 'alice'")
	require.Error(t, err)
	require.NoError(t, tx.Rollback())

	require.Len(t, f.spans, 3)
	assert.Equal(t, "sql.begin", f.spans[0].GetName())
	assert.Empty(t, f.spans[0].GetError())
	assert.Equal(t, "MySQL error 1062", f.spans[1].GetError(), "the number only: a message can quote values")
	assert.Equal(t, "*errors.errorString", f.spans[2].GetError(), "the type only")
}

func TestOperation(t *testing.T) {
	for query, want := range map[string]string{
		"select 1":                        "SELECT",
		"\n\t(SELECT 1) UNION (SELECT 2)": "SELECT",
		"/* alice */ SELECT 1":            "",
		"alicealicealicealice":            "",
		"":                                "",
	} {
		assert.Equal(t, want, operation(query), query)
	}
}

type fixture struct {
	ctx   context.Context
	db    *sql.DB
	conn  *fakeConn
	spans []*runnerv1.Span
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{conn: &fakeConn{}}
	f.ctx = runner.With(context.Background(), &runner.Session{Export: func(s *runnerv1.Span) { f.spans = append(f.spans, s) }})
	f.db = sql.OpenDB(connector{fakeConnector{f.conn}})
	t.Cleanup(func() { _ = f.db.Close() })
	return f
}

// room is a type only the fake driver's CheckNamedValue can convert.
type room struct{ n int }

type fakeConnector struct{ conn *fakeConn }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) { return c.conn, nil }
func (fakeConnector) Driver() driver.Driver                          { return nil }

// fakeConn records the statements it runs without arguments and the arguments its
// statements receive, and fails every statement with err.
type fakeConn struct {
	execs []string
	args  []any
	err   error
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("no context") }
func (c *fakeConn) Begin() (driver.Tx, error)           { return nil, errors.New("no context") }
func (c *fakeConn) Close() error                        { return nil }

func (c *fakeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.execs = append(c.execs, query)
	return driver.RowsAffected(0), c.err
}

func (c *fakeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.execs = append(c.execs, query)
	return fakeRows{}, c.err
}

func (c *fakeConn) PrepareContext(context.Context, string) (driver.Stmt, error) {
	return &fakeStmt{c}, nil
}

func (c *fakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return fakeTx{}, nil
}

func (c *fakeConn) CheckNamedValue(nv *driver.NamedValue) error {
	if r, ok := nv.Value.(room); ok {
		nv.Value = fmt.Sprintf("room %d", r.n)
		return nil
	}
	return driver.ErrSkip
}

type fakeStmt struct{ c *fakeConn }

func (s *fakeStmt) Close() error                               { return nil }
func (s *fakeStmt) NumInput() int                              { return -1 }
func (s *fakeStmt) Exec([]driver.Value) (driver.Result, error) { return nil, errors.New("no context") }
func (s *fakeStmt) Query([]driver.Value) (driver.Rows, error)  { return nil, errors.New("no context") }

func (s *fakeStmt) ExecContext(_ context.Context, args []driver.NamedValue) (driver.Result, error) {
	s.record(args)
	return driver.RowsAffected(1), s.c.err
}

func (s *fakeStmt) QueryContext(_ context.Context, args []driver.NamedValue) (driver.Rows, error) {
	s.record(args)
	return fakeRows{}, s.c.err
}

func (s *fakeStmt) record(args []driver.NamedValue) {
	for _, a := range args {
		s.c.args = append(s.c.args, a.Value)
	}
}

type fakeRows struct{}

func (fakeRows) Columns() []string         { return []string{"body"} }
func (fakeRows) Close() error              { return nil }
func (fakeRows) Next([]driver.Value) error { return io.EOF }

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }
