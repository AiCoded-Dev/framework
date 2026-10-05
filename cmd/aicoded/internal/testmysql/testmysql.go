// Package testmysql gives tests an admin connection to a MySQL 8 server from the DSN in
// AICODED_TEST_MYSQL_DSN. Without it a test is skipped, or fails when AICODED_E2E_MYSQL is
// "required". Nothing here prints the DSN, which holds a password.
package testmysql

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// DSN returns the admin DSN.
func DSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("AICODED_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("AICODED_E2E_MYSQL") == "required" {
			t.Fatal("AICODED_E2E_MYSQL=required, but AICODED_TEST_MYSQL_DSN is not set")
		}
		t.Skip("AICODED_TEST_MYSQL_DSN is not set")
	}
	return dsn
}

// Config returns the parsed admin DSN.
func Config(t testing.TB) *mysql.Config {
	t.Helper()
	cfg, err := mysql.ParseDSN(DSN(t))
	if err != nil {
		t.Fatal("AICODED_TEST_MYSQL_DSN is not a valid DSN")
	}
	return cfg
}

// Admin returns an admin connection, closed when the test ends.
func Admin(t testing.TB) *sql.DB {
	t.Helper()
	c, err := mysql.NewConnector(Config(t))
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("cannot reach the MySQL server of AICODED_TEST_MYSQL_DSN: %v", err)
	}
	return db
}

// User creates a database and a user identified with plugin that may use only that database,
// drops both when the test ends, and returns them with the user's password. The account's host
// is '%', since a server in Docker sees clients at the bridge address.
func User(t testing.TB, admin *sql.DB, plugin string) (user, password, database string) {
	t.Helper()
	suffix := strings.ToLower(rand.Text()[:12])
	user, database, password = "t-"+suffix, "t-"+suffix, rand.Text()
	account := fmt.Sprintf("'%s'@'%%'", user)
	for _, s := range []string{
		"CREATE DATABASE `" + database + "`",
		"CREATE USER " + account + " IDENTIFIED WITH " + plugin + " BY '" + password + "'",
		"GRANT ALL PRIVILEGES ON `" + database + "`.* TO " + account,
	} {
		if _, err := admin.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("prepare a test user: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP USER IF EXISTS "+account)
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+database+"`")
	})
	return user, password, database
}
