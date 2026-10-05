package sqldb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/telemetry"
)

func init() {
	redact.Kind(func(e *mysql.MySQLError) string {
		if e.SQLState == [5]byte{} {
			return fmt.Sprintf("MySQL error %d", e.Number)
		}
		return fmt.Sprintf("MySQL error %d (%s)", e.Number, e.SQLState[:])
	})
	redact.Sentinel(sql.ErrNoRows, "sql.ErrNoRows")
	redact.Sentinel(sql.ErrTxDone, "sql.ErrTxDone")
	redact.Sentinel(sql.ErrConnDone, "sql.ErrConnDone")
}

// connector wraps the driver so every query, statement and transaction gets a span. Spans name
// the statement's first keyword only, never its text or arguments.
type connector struct{ driver.Connector }

func (c connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &tracedConn{Conn: conn}, nil
}

type tracedConn struct{ driver.Conn }

// ExecContext runs a statement without arguments directly; with arguments it returns
// driver.ErrSkip, so database/sql prepares it, as the driver would do anyway.
func (c *tracedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := c.Conn.(driver.ExecerContext)
	if !ok || len(args) > 0 {
		return nil, driver.ErrSkip
	}
	ctx, span := start(ctx, "sql.exec", query)
	res, err := e.ExecContext(ctx, query, args)
	end(span, err)
	return res, err
}

func (c *tracedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := c.Conn.(driver.QueryerContext)
	if !ok || len(args) > 0 {
		return nil, driver.ErrSkip
	}
	ctx, span := start(ctx, "sql.query", query)
	rows, err := q.QueryContext(ctx, query, args)
	end(span, err)
	return rows, err
}

func (c *tracedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	p, ok := c.Conn.(driver.ConnPrepareContext)
	if !ok {
		return nil, errors.New("sqldb: the driver cannot prepare with a context")
	}
	st, err := p.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &tracedStmt{Stmt: st, conn: c.Conn, query: query}, nil
}

func (c *tracedConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	b, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errors.New("sqldb: the driver cannot begin with a context")
	}
	ctx, span := start(ctx, "sql.begin", "")
	tx, err := b.BeginTx(ctx, opts)
	end(span, err)
	return tx, err
}

func (c *tracedConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *tracedConn) ResetSession(ctx context.Context) error {
	if r, ok := c.Conn.(driver.SessionResetter); ok {
		return r.ResetSession(ctx)
	}
	return nil
}

func (c *tracedConn) IsValid() bool {
	if v, ok := c.Conn.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *tracedConn) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

type tracedStmt struct {
	driver.Stmt
	conn  driver.Conn
	query string
}

func (s *tracedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	e, ok := s.Stmt.(driver.StmtExecContext)
	if !ok {
		return nil, errors.New("sqldb: the driver cannot execute with a context")
	}
	ctx, span := start(ctx, "sql.exec", s.query)
	res, err := e.ExecContext(ctx, args)
	end(span, err)
	return res, err
}

func (s *tracedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := s.Stmt.(driver.StmtQueryContext)
	if !ok {
		return nil, errors.New("sqldb: the driver cannot query with a context")
	}
	ctx, span := start(ctx, "sql.query", s.query)
	rows, err := q.QueryContext(ctx, args)
	end(span, err)
	return rows, err
}

// CheckNamedValue asks the statement, then its connection, as database/sql does.
func (s *tracedStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if n, ok := s.Stmt.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	if n, ok := s.conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func start(ctx context.Context, name, query string) (context.Context, *telemetry.Span) {
	ctx, span := telemetry.Start(ctx, name)
	if op := operation(query); op != "" {
		span.SetAttr("db.operation", op)
	}
	return ctx, span
}

// end ends span, failed with err unless err is nil or driver.ErrSkip. Outside aicoded dev the
// span keeps only the MySQL error number and state: the message can quote the statement or its
// values.
func end(span *telemetry.Span, err error) {
	if err != nil && !errors.Is(err, driver.ErrSkip) {
		span.RecordError(err)
	}
	span.End()
}

// operation returns the statement's first keyword, such as SELECT.
func operation(query string) string {
	q := strings.TrimLeft(query, " \t\r\n(")
	n := strings.IndexFunc(q, func(r rune) bool { return !unicode.IsLetter(r) })
	if n < 0 {
		n = len(q)
	}
	if n == 0 || n > 16 {
		return ""
	}
	return strings.ToUpper(q[:n])
}
