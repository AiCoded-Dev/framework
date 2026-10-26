package e2e

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/migrations"
	"aicoded.dev/framework/internal/testmysql"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/sqldb/sqltest"
)

// The checked-in test apps hold exactly what aicoded generate writes, so no test runs on a stale
// page, client, server or snapshot.
func TestTestdataIsGenerated(t *testing.T) {
	abs := func(parts ...string) string {
		dir, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, parts...)...))
		require.NoError(t, err)
		return dir
	}
	rpc := map[string]string{"shop": abs("rpc", "shop"), "billing": abs("rpc", "billing")}
	for dir, ws := range map[string]map[string]string{
		abs("site"):    nil,
		abs("blocks"):  nil,
		abs("desk"):    nil,
		rpc["shop"]:    rpc,
		rpc["billing"]: rpc,
	} {
		_, err := generate.Run(dir, generate.Options{Frozen: true, Workspace: ws})
		assert.NoError(t, err, "run aicoded generate in %s and commit the result", dir)
	}
}

// The examples hold exactly what aicoded generate writes, as the security checks will require.
func TestExamplesAreGenerated(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "examples"))
	require.NoError(t, err)
	ws := map[string]string{
		"contacts":         filepath.Join(root, "contacts"),
		"people":           filepath.Join(root, "people"),
		"room-maintenance": filepath.Join(root, "room-maintenance"),
	}
	for _, dir := range ws {
		_, err := generate.Run(dir, generate.Options{Frozen: true, Workspace: ws})
		assert.NoError(t, err, "run aicoded generate in %s and commit the result", dir)
	}
}

// The examples pass aicoded check --frozen, lint included, as every app must, under an aicoded
// installed from this checkout, which their go.mod replaces the framework with, and their tests
// pass on the test databases it makes on the MySQL server of AICODED_TEST_MYSQL_DSN.
func TestExamplesPassCheck(t *testing.T) {
	requireGo(t)
	dsn := testmysql.DSN(t)
	checkout, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	root := filepath.Join(checkout, "examples")
	r, err := check.Run(t.Context(), root, check.Options{Frozen: true, FrameworkDir: checkout,
		MySQL: func() (string, error) { return dsn, nil }})
	require.NoError(t, err)
	var out strings.Builder
	require.NoError(t, r.WriteText(&out))
	assert.True(t, r.OK(), out.String())
}

// The examples' tests of their databases run, and skip nothing, on a test database made as
// aicoded check makes it, from the app's draft.
func TestExampleTestsUseTheTestDatabase(t *testing.T) {
	requireGo(t)
	admin, err := dev.NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	for _, app := range exampleApps {
		t.Run(app, func(t *testing.T) {
			dir := filepath.Join("..", "..", "..", "..", "examples", app)
			m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
			require.NoError(t, err)
			steps, warning, err := migrations.Steps(t.Context(), dir, m, nil)
			require.NoError(t, err)
			require.Empty(t, warning)
			env := "test-" + strings.ToLower(rand.Text()[:8])
			t.Cleanup(func() { _ = admin.Drop(context.Background(), app, env) })
			users, err := admin.Migrate(t.Context(), app, env, steps)
			require.NoError(t, err)
			cfg := mysql.NewConfig()
			cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = users.Data.Network, users.Data.Address, users.Data.User, users.Data.Password, users.Data.Database

			cmd := exec.CommandContext(t.Context(), "go", "test", "-json", "-count=1", "./deps/")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), sqltest.EnvDB+"="+cfg.FormatDSN())
			out, err := cmd.Output()
			require.NoError(t, err, "go test ./deps/ in examples/%s failed:\n%s", app, out)
			var ran int
			for line := range strings.Lines(string(out)) {
				var e struct{ Action, Test string }
				require.NoError(t, json.Unmarshal([]byte(line), &e))
				if e.Test != "" && e.Action == "skip" {
					t.Errorf("%s skipped", e.Test)
				}
				if e.Test != "" && e.Action == "pass" {
					ran++
				}
			}
			assert.Positive(t, ran)
		})
	}
}
