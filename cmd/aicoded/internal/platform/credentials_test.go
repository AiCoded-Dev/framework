package platform_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

func TestAddress(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "https://api.aicoded.cloud",
		"https://api.acme.example/": "https://api.acme.example",
		"https://API.acme.example":  "https://api.acme.example",
		"http://127.0.0.1:8080":     "http://127.0.0.1:8080",
	} {
		t.Setenv("AICODED_PLATFORM", in)
		got, err := platform.Address()
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}
}

func TestAddressRefused(t *testing.T) {
	for _, in := range []string{"http://api.acme.example", "http://localhost:8080", "http://127.0.0.1", "http://127.0.0.1:0",
		"http://127.0.0.1:080", "https://ana:secret@api.acme.example", "https://api.acme.example/v1", "https://api.acme.example?a=b",
		"https://api.acme.example#a", "https://api.acme.example:", "ftp://api.acme.example", "api.acme.example", "https:secret"} {
		t.Setenv("AICODED_PLATFORM", in)
		_, err := platform.Address()
		assert.Equal(t, "E-CLI-005", errs.Code(err), in)
		assert.NotContains(t, err.Error(), "secret", "the value may hold a password")
	}
}

func TestCredentials(t *testing.T) {
	home := testhome.Set(t)
	cr, err := platform.Load("https://a.example")
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{}, cr, "signed out")

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	a := platform.Credentials{Org: "acme", AccessToken: "aicoded_at_a", RefreshToken: "aicoded_rt_a", Expires: at}
	b := platform.Credentials{Org: "beta", AccessToken: "aicoded_at_b", RefreshToken: "aicoded_rt_b", Expires: at}
	require.NoError(t, platform.Save("https://a.example", a))
	require.NoError(t, platform.Save("http://127.0.0.1:8080", b))
	cr, err = platform.Load("https://a.example")
	require.NoError(t, err)
	assert.Equal(t, a, cr)

	path := filepath.Join(home, ".config", "aicoded", "credentials.json")
	assertMode(t, 0o600, path)
	assertMode(t, 0o700, filepath.Dir(path))

	require.NoError(t, platform.Delete("https://a.example"))
	cr, err = platform.Load("https://a.example")
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{}, cr)
	cr, err = platform.Load("http://127.0.0.1:8080")
	require.NoError(t, err)
	assert.Equal(t, b, cr, "each platform has its own")
	require.NoError(t, platform.Delete("http://127.0.0.1:8080"))
	assert.NoFileExists(t, path)
	require.NoError(t, platform.Delete("http://127.0.0.1:8080"))
}

func TestCredentialsOthersCanRead(t *testing.T) {
	home := testhome.Set(t)
	cr := platform.Credentials{Org: "acme", AccessToken: "aicoded_at_a", RefreshToken: "aicoded_rt_a"}
	require.NoError(t, platform.Save("https://a.example", cr))
	path := filepath.Join(home, ".config", "aicoded", "credentials.json")

	require.NoError(t, os.Chmod(path, 0o644))
	_, err := platform.Load("https://a.example")
	assert.Equal(t, "E-CLI-007", errs.Code(err))
	assert.Contains(t, err.Error(), "chmod 600 "+path)
	assert.Equal(t, "E-CLI-007", errs.Code(platform.Save("https://a.example", cr)))
	assert.Equal(t, "E-CLI-007", errs.Code(platform.Delete("https://a.example")))

	require.NoError(t, os.Chmod(path, 0o600))
	require.NoError(t, os.Chmod(filepath.Dir(path), 0o755))
	err = platform.Save("https://a.example", cr)
	assert.Equal(t, "E-CLI-007", errs.Code(err), "a folder others can open")
	assert.Contains(t, err.Error(), "chmod 700 "+filepath.Dir(path))
}

func TestCredentialsDamaged(t *testing.T) {
	home := testhome.Set(t)
	dir := filepath.Join(home, ".config", "aicoded")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"platforms": [`), 0o600))
	_, err := platform.Load("https://a.example")
	assert.ErrorContains(t, err, "is damaged")
}

func assertMode(t *testing.T, want os.FileMode, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, want, fi.Mode().Perm(), path)
}
