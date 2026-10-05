package devconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/internal/errs"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	ws, err := devconfig.Load(path, "/src/apps")
	require.NoError(t, err)
	assert.Equal(t, devconfig.DefaultPort, ws.Port)
	assert.Empty(t, ws.Personas, "the caller picks the default personas")

	require.NoError(t, os.WriteFile(path, []byte(`workspaces:
  /src/apps:
    port: 9000
    mysql: root:pw@unix(/var/run/mysqld/mysqld.sock)/
    personas:
      - {name: alice, roles: [hotel-ops]}
    apps:
      rooms:
        settings: {greeting: hi}
        secrets: {api_key: s3cret}
  /src/other:
    port: 9100
`), 0o600))
	ws, err = devconfig.Load(path, "/src/apps")
	require.NoError(t, err)
	assert.Equal(t, 9000, ws.Port)
	assert.Equal(t, "root:pw@unix(/var/run/mysqld/mysqld.sock)/", ws.MySQL)
	assert.Equal(t, []devconfig.Persona{{Name: "alice", Roles: []string{"hotel-ops"}}}, ws.Personas)
	assert.Equal(t, "s3cret", ws.Apps["rooms"].Secrets["api_key"])
}

func TestLoadEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte("workspaces:\n  /src/apps:\n    env: preview\n  /src/other:\n    port: 9100\n"+
		"  /src/empty:\n    env:\n  /src/dev:\n    env: dev\n"), 0o600))
	for ws, want := range map[string]devconfig.Env{"/src/apps": devconfig.EnvPreview, "/src/other": devconfig.EnvDev,
		"/src/empty": devconfig.EnvDev, "/src/dev": devconfig.EnvDev} {
		got, err := devconfig.Load(path, ws)
		require.NoError(t, err)
		assert.Equal(t, want, got.Env, ws)
	}
}

func TestLoadRefusesOtherEnvs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	for _, env := range []string{"prod", "Preview", `""`, "[dev]", "supersecretvalue"} {
		require.NoError(t, os.WriteFile(path, []byte("workspaces:\n  /src/apps:\n    port: 9000\n    env: "+env+"\n"), 0o600))
		_, err := devconfig.Load(path, "/src/apps")
		assert.Equal(t, "E-DEV-020", errs.Code(err), env)
		require.ErrorContains(t, err, "dev.yaml:4", env)
		assert.NotContains(t, err.Error(), "superse", "the value is never quoted")
	}
}

func TestLoadRefusesSharedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte("workspaces: {}\n"), 0o600))
	require.NoError(t, os.Chmod(path, 0o644))
	_, err := devconfig.Load(path, "/src/apps")
	assert.Equal(t, "E-DEV-001", errs.Code(err))
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte("workspaces:\n  /src/apps:\n    prot: 9000\n"), 0o600))
	_, err := devconfig.Load(path, "/src/apps")
	assert.Equal(t, "E-DEV-002", errs.Code(err))
}

func TestLoadKeepsValuesOutOfErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`workspaces:
  /src/apps:
    apps:
      rooms:
        secrets: supersecretvalue
`), 0o600))
	_, err := devconfig.Load(path, "/src/apps")
	require.ErrorContains(t, err, "dev.yaml:5")
	assert.Equal(t, "E-DEV-002", errs.Code(err))
	assert.NotContains(t, err.Error(), "superse")
}
