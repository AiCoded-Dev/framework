package cantbebuilt

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
)

// A database key in the app: the notes app opens its database itself, with sql.Open and an
// address that holds the database's user and password (testdata/notes-dsn). aicoded check
// refuses the call, as it refuses any database an app opens itself, with or without a password.
// The same app that gets its database from sqldb.Open, with no address and no password, passes
// (testdata/own-notes).
func TestDatabaseOpenedWithPassword(t *testing.T) {
	assert.Equal(t, []refusal{
		{"E-LINT-006", "deps/deps.go:28", "app code may not use sql.Open"},
	}, runCheck(t, copyApp(t, "notes-dsn")))

	assert.Empty(t, runCheck(t, copyApp(t, "own-notes")), "the control passes")
}

// Nothing to leak: the runner never hands an app a database key, so code in the app that hunts
// for one, as an exploit of a leaked key would, finds none. The app in testdata/key-hunt declares
// sqldb and runs under the local runner with a real database; aicoded check would refuse its
// code, which runs here as code that got past the checks would. It finds only
// AICODED_RUNNER_DIR in its environment and only sockets in its runner folder, and yet it writes
// a row to its database through sqldb and reads it back.
func TestNothingToLeak(t *testing.T) {
	a := startApp(t, "key-hunt")
	code, body := a.send(t, request(t, http.MethodGet, "/", nil, a.token(t, devconfig.Persona{Name: "ana"})))
	require.Equal(t, http.StatusOK, code, body)
	var found struct {
		Env    []string
		Folder map[string]string
		Row    string
	}
	require.NoError(t, json.Unmarshal([]byte(body), &found))

	assert.Equal(t, []string{"AICODED_RUNNER_DIR"}, found.Env, "the names of the variables of its environment")
	assert.Equal(t, map[string]string{"app.sock": "socket", "mysql.sock": "socket", "runner.sock": "socket"},
		found.Folder, "its runner folder")
	assert.Equal(t, "written and read back", found.Row, "its database works")
}
