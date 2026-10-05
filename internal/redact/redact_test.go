package redact_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/rpc"
	_ "aicoded.dev/framework/sqldb"
	"aicoded.dev/framework/web"
)

// opError is an app's own error type that wraps another.
type opError struct{ err error }

func (e *opError) Error() string { return "alice: " + e.err.Error() }
func (e *opError) Unwrap() error { return e.err }

var duplicate = &mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2', '3', '0', '0', '0'},
	Message: "Duplicate entry 'alice' for key 'users.PRIMARY'"}

func TestError(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{duplicate, "MySQL error 1062 (23000)"},
		{&mysql.MySQLError{Number: 1366, Message: "Incorrect string value: 'alice' for column 'name'"}, "MySQL error 1366"},
		{&fs.PathError{Op: "write", Path: "alice/cv.png", Err: fmt.Errorf("%w: %w", fs.ErrInvalid,
			errs.New("E-FILE-001", `"alice/cv.png" is not a valid name`, "rename it"))}, "fs.PathError write: [fs.ErrInvalid, E-FILE-001]"},
		{&fs.PathError{Op: "alice", Path: "cv.png", Err: fs.ErrNotExist}, "fs.PathError: fs.ErrNotExist"},
		{fmt.Errorf("add alice: %w", fmt.Errorf("insert: %w", duplicate)), "MySQL error 1062 (23000)"},
		{&opError{fmt.Errorf("user %q: %w", "alice", sql.ErrNoRows)}, "*redact_test.opError: sql.ErrNoRows"},
		{errors.Join(context.Canceled, errors.New("rollback for alice failed")), "[context.Canceled, *errors.errorString]"},
		{errs.New("E-MAIL-002", `"alice@@example.com" is not a plain ASCII address`, "fix it"), "E-MAIL-002"},
		{rpc.Error(rpc.NotFound, "no person alice"), "rpc not_found"},
		{web.Error(403, "alice may not see this"), "HTTP 403"},
		{web.Redirect("/users/alice"), "web.Redirect"},
		{fmt.Errorf("query for alice: %w", context.DeadlineExceeded), "context.DeadlineExceeded"},
		{connect.NewError(connect.CodeNotFound, errors.New("alice/cv.png")), "not_found"},
		{connect.NewError(connect.CodeFailedPrecondition, errs.New("E-RUN-002", "alice", "update")), "failed_precondition E-RUN-002"},
		{errors.New("alice"), "*errors.errorString"},
	} {
		got := redact.Error(c.err)
		assert.Equal(t, c.want, got, c.err.Error())
		assert.NotContains(t, got, "alice")
	}
}

func TestValue(t *testing.T) {
	assert.Equal(t, "string", redact.Value("alice"))
	assert.Equal(t, "fs.PathError open: fs.ErrNotExist", redact.Value(&fs.PathError{Op: "open", Path: "alice", Err: fs.ErrNotExist}))
	assert.Equal(t, "int", redact.Value(42))
	assert.Equal(t, "<nil>", redact.Error(nil))
}

func TestFor(t *testing.T) {
	err := fmt.Errorf("add alice: %w", fs.ErrExist)
	for env, want := range map[string][2]string{
		"":        {"fs.ErrExist", "string"},
		"preview": {"fs.ErrExist", "string"},
		"dev":     {"add alice: file already exists", "alice"},
	} {
		ctx := runner.With(context.Background(), &runner.Session{Env: env})
		assert.Equal(t, want[0], redact.For(ctx, err), env)
		assert.Equal(t, want[1], redact.ValueFor(ctx, "alice"), env)
		assert.Equal(t, env == "dev", redact.Dev(ctx), env)
		assert.Equal(t, "<nil>", redact.For(ctx, nil), env)
		assert.Equal(t, "<nil>", redact.ValueFor(ctx, nil), env)
	}
	assert.Equal(t, "fs.ErrExist", redact.For(context.Background(), err), "no runner")
	assert.Equal(t, "string", redact.ValueFor(context.Background(), "alice"), "no runner")
}
