package dev

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestTokenIsMadeOnce(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, tokenFile)
	_, err := readToken(state, false)
	require.ErrorIs(t, err, fs.ErrNotExist, "without create, nothing is made")
	assert.NoFileExists(t, path)

	token, err := readToken(state, true)
	require.NoError(t, err)
	assert.Regexp(t, `^[0-9a-f]{64}$`, token)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode())

	again, err := readToken(state, true)
	require.NoError(t, err)
	assert.Equal(t, token, again, "the token survives a restart")
	again, err = readToken(state, false)
	require.NoError(t, err)
	assert.Equal(t, token, again)

	require.NoError(t, os.Remove(path))
	other, err := readToken(state, true)
	require.NoError(t, err)
	assert.NotEqual(t, token, other, "deleting the file makes a new token")
}

func TestTokenRefusesOtherFiles(t *testing.T) {
	good := strings.Repeat("ab", 32)
	for name, put := range map[string]func(path string) error{
		"folder":     func(path string) error { return os.Mkdir(path, 0o700) },
		"symlink":    func(path string) error { return os.Symlink("elsewhere", path) },
		"group bits": func(path string) error { return writeMode(path, good, 0o640) },
		"other bits": func(path string) error { return writeMode(path, good, 0o604) },
		"short":      func(path string) error { return writeMode(path, good[:63], 0o600) },
		"long":       func(path string) error { return writeMode(path, good+"a", 0o600) },
		"newline":    func(path string) error { return writeMode(path, good+"\n", 0o600) },
		"upper case": func(path string) error { return writeMode(path, strings.ToUpper(good), 0o600) },
		"not hex":    func(path string) error { return writeMode(path, strings.Repeat("zz", 32), 0o600) },
		"empty":      func(path string) error { return writeMode(path, "", 0o600) },
		"symlink to a token": func(path string) error {
			if err := writeMode(path+"-real", good, 0o600); err != nil {
				return err
			}
			return os.Symlink(tokenFile+"-real", path)
		},
	} {
		state := t.TempDir()
		path := filepath.Join(state, tokenFile)
		require.NoError(t, put(path), name)
		for _, create := range []bool{true, false} {
			_, err := readToken(state, create)
			assert.Equal(t, "E-DEV-018", errs.Code(err), name)
			require.ErrorContains(t, err, path+" is not a regular file of mode 0600 that holds 64 hex characters", name)
		}
	}
}

// writeMode writes content to a new file at path with mode perm, whatever the umask.
func writeMode(path, content string, perm os.FileMode) error {
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}
