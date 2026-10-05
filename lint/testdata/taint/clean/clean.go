// Package clean records only what may be recorded, next to outside data.
package clean

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/config"
	"aicoded.dev/framework/telemetry"
	"aicoded.dev/framework/web"
)

// store holds the database and the last login seen.
type store struct {
	db   *sql.DB
	last string
	at   time.Time
}

// Clean records numbers, flags, constants and settings, and returns its errors.
func Clean(ctx context.Context, r *web.Request, db *sql.DB) error {
	login := r.URLParam("login")
	slog.InfoContext(ctx, "started")                                           // clean
	slog.InfoContext(ctx, "length", "n", len(login))                           // clean
	slog.InfoContext(ctx, "length", "n", strconv.Itoa(len(login)))             // clean
	slog.InfoContext(ctx, "id", "id", r.URLParamInt("id"))                     // clean
	slog.InfoContext(ctx, "admin", "admin", login == "admin")                  // clean
	slog.InfoContext(ctx, "staff", "staff", auth.Viewer(ctx).HasRole("staff")) // clean
	slog.InfoContext(ctx, "label", "v", label("ops"))                          // clean
	_ = label(login)
	ctx, span := telemetry.Start(ctx, "users.add") // clean
	defer span.End()
	if err := save(ctx, db, login); err != nil {
		span.RecordError(err) // clean
		return err
	}
	team, err := config.String(ctx, "team_address")
	if err != nil {
		return err
	}
	slog.InfoContext(ctx, "team", "address", team) // clean
	s := &store{db: db, last: login, at: time.Now()}
	slog.InfoContext(ctx, "seen", "at", s.at) // clean
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expired"); err != nil {
		slog.ErrorContext(ctx, "cleanup", "err", err) // clean
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE login = ?", login).Scan(&n); err != nil {
		return err
	}
	slog.InfoContext(ctx, "users", "n", n) // clean
	return nil
}

// save adds login to the users table. Its error is returned, not recorded.
func save(ctx context.Context, db *sql.DB, login string) error {
	if _, err := db.ExecContext(ctx, "INSERT INTO users (login) VALUES (?)", login); err != nil {
		return fmt.Errorf("save %s: %w", login, err)
	}
	return nil
}

// label returns s in brackets. It gets a URL parameter in one call, and a constant in another.
func label(s string) string { return "[" + s + "]" }
