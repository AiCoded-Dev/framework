package deps

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/sqldb/sqltest"
)

func TestContacts(t *testing.T) {
	db = testDB(t)
	ctx := t.Context()
	require.NoError(t, fill(ctx, db))
	require.NoError(t, fill(ctx, db), "a second start adds nothing")
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

// testDB returns the test database that aicoded check makes with the tables of
// .aicoded/migration.sql, emptied, or skips the test outside aicoded check.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db := sqltest.Open(t)
	_, err := db.ExecContext(t.Context(), "DELETE FROM contacts")
	require.NoError(t, err)
	return db
}
