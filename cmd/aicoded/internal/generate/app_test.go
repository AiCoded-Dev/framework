package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestLoadApp(t *testing.T) {
	dir := t.TempDir()
	_, err := loadApp(dir, "")
	assert.Equal(t, "E-GEN-037", errs.Code(err))
	app, err := loadApp(dir, "example.com/m/ui")
	require.NoError(t, err)
	assert.Equal(t, App{Dir: dir, Module: "example.com/m/ui"}, app, "a module path given needs no go.mod")

	write(t, dir, "go.mod", "// comment\nmodule \"example.com/notes\"\n\ngo 1.25.0\n")
	app, err = loadApp(dir, "")
	require.NoError(t, err)
	assert.Equal(t, App{Dir: dir, Module: "example.com/notes"}, app)
	app, err = loadApp(dir, "example.com/m/ui")
	require.NoError(t, err)
	assert.Equal(t, "example.com/m/ui", app.Module, "a module path given wins over go.mod")

	require.NoError(t, os.Mkdir(filepath.Join(dir, "deps"), 0o755))
	app, err = loadApp(dir, "")
	require.NoError(t, err)
	assert.True(t, app.Deps)

	write(t, dir, "go.mod", "go 1.25.0\n")
	_, err = loadApp(dir, "")
	assert.Equal(t, "E-GEN-037", errs.Code(err))
}

func TestModulePath(t *testing.T) {
	for gomod, want := range map[string]string{
		"module example.com/a\n":                "example.com/a",
		"module\texample.com/a // the app\n":    "example.com/a",
		"module \"example.com/a\" // the app\n": "example.com/a",
		"modulex example.com/a\n":               "",
		"// module example.com/a\n":             "",
	} {
		assert.Equal(t, want, modulePath([]byte(gomod)), gomod)
	}
}
