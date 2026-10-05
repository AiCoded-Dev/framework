package deps

import (
	"context"
	"crypto/rand"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContacts(t *testing.T) {
	db = testDB(t)
	ctx := t.Context()
	require.NoError(t, prepare(ctx, db))
	require.NoError(t, prepare(ctx, db), "a second start adds nothing")
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM contacts").Scan(&n))
	assert.Len(t, seed, n)

	phones, emails, err := Contacts(ctx, "johndoe123")
	require.NoError(t, err)
	assert.Equal(t, []string{"+1234567890", "+0987654321"}, phones)
	assert.Equal(t, []string{"johndoe@example.com", "john.d@example.com"}, emails)

	_, _, err = Contacts(ctx, "JohnDoe123")
	require.ErrorIs(t, err, ErrNoPerson, "a login matches exactly")
	_, _, err = Contacts(ctx, "johndoe123 ")
	require.ErrorIs(t, err, ErrNoPerson, "a trailing space is part of the login")
	_, _, err = Contacts(ctx, "nobody")
	require.ErrorIs(t, err, ErrNoPerson)

	_, err = db.ExecContext(ctx, "INSERT INTO contacts (login, kind, value) VALUES ('carol', 'fax', '+1')")
	require.Error(t, err, "a contact is a phone or an email")
}

// testDB returns a new, empty database on the MySQL server that AICODED_TEST_MYSQL_DSN names,
// and drops it when the test ends. Without the variable the test is skipped, or fails when
// AICODED_E2E_MYSQL is required. Nothing here prints the DSN, which holds a password.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("AICODED_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("AICODED_E2E_MYSQL") == "required" {
			t.Fatal("AICODED_E2E_MYSQL=required, but AICODED_TEST_MYSQL_DSN is not set")
		}
		t.Skip("AICODED_TEST_MYSQL_DSN is not set")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("AICODED_TEST_MYSQL_DSN is not a valid DSN")
	}
	admin := open(t, cfg)
	name := "test_" + strings.ToLower(rand.Text()[:12])
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`") })
	cfg.DBName = name
	cfg.ParseTime, cfg.Loc = true, time.UTC
	return open(t, cfg)
}

func open(t *testing.T, cfg *mysql.Config) *sql.DB {
	t.Helper()
	c, err := mysql.NewConnector(cfg)
	require.NoError(t, err)
	db := sql.OpenDB(c)
	t.Cleanup(func() { _ = db.Close() })
	return db
}
