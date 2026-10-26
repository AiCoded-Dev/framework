package draft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/manifest"
)

// withDB is a permission list that declares sqldb.
var withDB = manifest.Manifest{App: "shop", Data: []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}}

// app returns an app folder whose draft holds text.
func app(t *testing.T, text string) string {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".aicoded"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, File), []byte(text), 0o600))
	return dir
}

// found returns the code and position of each problem of err.
func found(err error) []string {
	var out []string
	for _, p := range problem.From("shop", err) {
		out = append(out, p.Code+" "+p.Pos)
	}
	return out
}

func TestCheck(t *testing.T) {
	stmts, err := Check(app(t, "CREATE TABLE a (x INT);\n\nCREATE INDEX i ON a (x);\n"), withDB)
	require.NoError(t, err)
	assert.Equal(t, []string{"CREATE TABLE a (x INT)", "CREATE INDEX i ON a (x)"}, stmts)

	stmts, err = Check(t.TempDir(), withDB)
	require.NoError(t, err, "no draft")
	assert.Nil(t, stmts)
}

func TestCheckRefuses(t *testing.T) {
	for name, c := range map[string]struct {
		text string
		want []string
	}{
		"destructive":        {"CREATE TABLE a (x INT);\nDROP TABLE b;\n", []string{"E-MIG-003 .aicoded/migration.sql:2"}},
		"executable comment": {"CREATE TABLE a (x INT);\n/*!50000 CREATE TABLE b (y INT) */;", []string{"E-MIG-002 .aicoded/migration.sql:2"}},
		"over 64 KiB":        {"-- " + strings.Repeat("x", 64<<10), []string{"E-MIG-001 .aicoded/migration.sql"}},
		"no statement":       {"-- only a comment\n", []string{"E-MIG-001 .aicoded/migration.sql"}},
	} {
		stmts, err := Check(app(t, c.text), withDB)
		assert.Nil(t, stmts, name)
		assert.Equal(t, c.want, found(err), name)
	}
	_, err := Read(app(t, strings.Repeat("x", 100<<10)))
	assert.ErrorContains(t, err, "the draft holds 102400 bytes, more than the 65536 it may", "the whole size, read before the draft")
}

func TestCheckWithoutDatabase(t *testing.T) {
	_, err := Check(app(t, "CREATE TABLE a (x INT);"), manifest.Manifest{App: "shop"})
	assert.Equal(t, []string{"E-MIG-006 .aicoded/migration.sql"}, found(err))
}

func TestReadStaysInTheApp(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, File), 0o750))
	_, err := Read(dir)
	require.ErrorContains(t, err, ".aicoded/migration.sql: E-MIG-001: the draft is not a regular file", "a folder")

	dir = t.TempDir()
	outside := filepath.Join(t.TempDir(), "schema.sql")
	require.NoError(t, os.WriteFile(outside, []byte("CREATE TABLE a (x INT);"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".aicoded"), 0o750))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, File)))
	_, err = Read(dir)
	assert.Equal(t, []string{"E-MIG-001 .aicoded/migration.sql"}, found(err), "a link out of the app's folder")

	require.NoError(t, os.Remove(filepath.Join(dir, File)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.sql"), []byte("CREATE TABLE a (x INT);"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join("..", "schema.sql"), filepath.Join(dir, File)))
	text, err := Read(dir)
	require.NoError(t, err, "a link inside the app's folder")
	assert.Equal(t, "CREATE TABLE a (x INT);", string(text))
}
