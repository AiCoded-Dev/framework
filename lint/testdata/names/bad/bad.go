// Package bad uses names of allowed packages that app code may not use.
package bad

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"hash/crc32"
	"log/slog"
	"net/http"
	"time"

	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"
	"aicoded.dev/framework/web/reactive"
)

// driverOf has the method that reaches a database's driver.
type driverOf interface{ Driver() driver.Driver }

// preparer has a method named like one for generated code, with another signature.
type preparer interface {
	Prepare(query string) (*sql.Stmt, error)
}

// Use uses names that app code may not use.
func Use(ctx context.Context, db *sql.DB, r *web.Request, in *form.Input[string], hr *http.Request) error {
	_, _ = rpc.Call(ctx, "billing", "Ping", nil)
	_ = r.CSRFToken()
	in.Process(hr, "name", false, true)
	_ = reactive.HTMLBinding("<b>")
	_, _ = sql.Open("mysql", "")
	_ = db.Driver()
	var d driverOf = db
	_ = d.Driver()
	_ = http.Get
	conn, _ := db.Conn(ctx)
	_ = conn.Raw(func(any) error { return nil })
	slog.SetDefault(slog.Default())
	rand.Reader = nil
	time.Local = time.UTC
	return nil
}

// Keep uses only names that app code may use.
func Keep() {
	var p preparer = (*sql.DB)(nil)
	_, _ = p.Prepare("SELECT 1")
	_ = http.StatusText(http.StatusOK) + http.MethodPost + r().URLParam("id")
	_ = sql.ErrNoRows
	_, _ = sql.Named("id", 1), sql.ErrConnDone
	slog.Info("kept", "now", time.Now())
}

func r() *web.Request { return &web.Request{} }

// driverAs has the method that reaches a database's driver, with a type parameter as its result.
type driverAs[T any] interface{ Driver() T }

func drive[T any](d driverAs[T]) T { return d.Driver() }

// Change changes variables of the standard library, and values of its types through pointers.
func Change(locs []*time.Location) {
	_ = &time.Local
	crc32.IEEETable[0]++
	for _, time.Local = range locs {
	}
	*slog.Default() = slog.Logger{}
	l := slog.With()
	*l = slog.Logger{}
	*time.Local = *time.UTC
	var own time.Location
	at := &own
	*at = time.Location{}
}

// counter is a type of the app's own.
type counter struct{ n int }

// Count changes values of its own types, and of types that are not named, through pointers.
func Count(c *counter, n *int) {
	*c = counter{n: 1}
	*n = 2
}

// Reflect reaches package reflect through a value of the standard library.
func Reflect(ct *sql.ColumnType, err error) {
	_ = ct.ScanType()
	var e *json.UnmarshalTypeError
	if errors.As(err, &e) {
		m, _ := e.Type.MethodByName("QueryContext")
		_ = m.Func.Interface()
	}
}

// logger is slog.Logger under the app's own name.
type logger slog.Logger

// own wraps a value of its own and one of the standard library.
type own struct {
	n   int
	loc time.Time
}

// Bypass replaces the default logger through a type parameter and through a conversion, and
// writes through pointers to std and own types.
func Bypass(h slog.Handler, out *time.Time, o *own) {
	set(slog.Default(), *slog.New(h))
	*(*logger)(slog.Default()) = logger(*slog.New(h))
	*out = time.Now()
	o.loc = time.Now()
	o.n = 1
}

func set[T any](p *T, v T) { *p = v }
