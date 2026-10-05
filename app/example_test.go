package app_test

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/sqldb"
)

// Deps is what every page of the app shares. An app declares it in its deps package.
type Deps struct {
	DB *sql.DB
}

// Start opens the database before the app serves. Its context reaches the runner, so Start can
// also read settings and secrets; an error stops the app.
func (d *Deps) Start(ctx context.Context) error {
	db, err := sqldb.Open(ctx)
	if err != nil {
		return err
	}
	d.DB = db
	slog.InfoContext(ctx, "started", "version", app.Version(ctx))
	return nil
}

// NewHandler stands for pages.NewHandler, which aicoded generate writes from the app's pages.
func NewHandler(*Deps) http.Handler { return http.NotFoundHandler() }

// The main of an app with pages and a database.
func Example() {
	d := &Deps{}
	app.Main(app.Options{Handler: NewHandler(d), OnStart: d.Start})
}
