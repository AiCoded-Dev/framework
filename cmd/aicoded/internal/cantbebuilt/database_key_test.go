package cantbebuilt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A database key in the app: the notes app opens its database itself, with sql.Open and an
// address that holds the database's user and password (testdata/notes-dsn). aicoded check
// refuses the call. The same app that gets its database from sqldb.Open, with no address and no
// password, passes (testdata/own-notes).
func TestDatabaseOpenedWithPassword(t *testing.T) {
	assert.Equal(t, []refusal{
		{"E-LINT-006", "deps/deps.go:28", "app code may not use sql.Open"},
	}, runCheck(t, copyApp(t, "notes-dsn")))

	assert.Empty(t, runCheck(t, copyApp(t, "own-notes")), "the control passes")
}
