package dev

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := StatePath("/work/a")
	require.NoError(t, err)
	assert.NoDirExists(t, path, "StatePath creates nothing")
	dir, err := StateDir("/work/a")
	require.NoError(t, err)
	assert.Equal(t, path, dir)
	assert.Equal(t, filepath.Join(home, ".local", "state", "aicoded"), filepath.Dir(dir))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	other, err := StateDir("/work/b")
	require.NoError(t, err)
	assert.NotEqual(t, dir, other, "each workspace has its own folder")

	require.NoError(t, os.Chmod(dir, 0o750))
	_, err = StateDir("/work/a")
	assert.Equal(t, "E-DEV-009", errs.Code(err))
}
