package sqldb

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
)

// maxOpen stays below the runner's limit of connections per app.
const maxOpen = 16

// Open returns the pool of connections to the app's database. Times are read and written in
// UTC, one statement runs per call, and statements with arguments are always prepared. Open
// fails with E-MAN-010 when aicoded.yaml does not declare the data source sqldb.
func Open(ctx context.Context) (*sql.DB, error) {
	s := runner.From(ctx)
	if s == nil || s.Dir == "" {
		return nil, runner.ErrNoRunner
	}
	sock := filepath.Join(s.Dir, runnerproto.MySQLSocket)
	if _, err := os.Stat(sock); errors.Is(err, fs.ErrNotExist) {
		return nil, errs.New("E-MAN-010", "the app's SQL database is not declared",
			"add - source: sqldb with its classes to data in aicoded.yaml")
	}
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User = "unix", sock, "app"
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Collation = "utf8mb4_0900_ai_ci"
	cfg.AllowNativePasswords = true
	cfg.MultiStatements, cfg.AllowAllFiles, cfg.InterpolateParams = false, false, false
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector{c})
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}
