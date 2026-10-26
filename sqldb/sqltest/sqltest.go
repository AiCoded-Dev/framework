// Package sqltest opens, in an app's tests, the test database that aicoded check makes for the
// app: a database of its own with the app's tables, which the tests reach as the app's data user,
// who reads and writes rows and changes no table.
//
//	func TestAddNote(t *testing.T) {
//		db := sqltest.Open(t)
//		...
//	}
//
// aicoded check makes the test database when the app declares sqldb and dev.yaml names a MySQL
// server, and puts its address in AICODED_TEST_DB; elsewhere, Open skips the test.
package sqltest

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// EnvDB is the variable that holds the DSN of the test database.
const EnvDB = "AICODED_TEST_DB"

// Open returns the pool of connections to the test database, closed when the test ends. Times
// are read and written in UTC and one statement runs per call, as with sqldb.Open. It skips the
// test when AICODED_TEST_DB is unset or empty, and fails it when the variable does not hold a
// DSN, without quoting it.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	dsn := os.Getenv(EnvDB)
	if dsn == "" {
		t.Skip(EnvDB + " is not set: aicoded check sets it when the app declares sqldb and dev.yaml names a MySQL server")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(EnvDB + " does not hold a valid MySQL DSN")
	}
	cfg.ParseTime, cfg.Loc = true, time.UTC
	cfg.Collation = "utf8mb4_0900_ai_ci"
	cfg.MultiStatements, cfg.AllowAllFiles, cfg.InterpolateParams = false, false, false
	c, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(EnvDB + " does not hold a usable MySQL DSN")
	}
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
