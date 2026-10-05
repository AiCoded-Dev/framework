// Package testhome gives a test a home folder of its own, so that it never reads or writes the
// user's dev.yaml or the state of their aicoded dev. Go keeps its caches and settings.
package testhome

import (
	"os"
	"path/filepath"
	"testing"
)

// Set points HOME and XDG_CONFIG_HOME at a new temp folder, removed when the test ends, and
// returns it. The folder's path is short, so the Unix sockets of aicoded dev fit below it. It
// first pins GOCACHE, GOPATH, GOMODCACHE and GOENV to where they are now, so the go command
// still finds its build cache and modules without the network.
func Set(t testing.TB) string {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	keep(t, "GOCACHE", filepath.Join(cache, "go-build"))
	keep(t, "GOPATH", filepath.Join(home, "go"))
	keep(t, "GOMODCACHE", filepath.Join(filepath.SplitList(os.Getenv("GOPATH"))[0], "pkg", "mod"))
	keep(t, "GOENV", filepath.Join(config, "go", "env"))

	dir, err := os.MkdirTemp("", "aicd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

// keep sets the variable key to its default def unless it is set.
func keep(t testing.TB, key, def string) {
	if os.Getenv(key) == "" {
		t.Setenv(key, def)
	}
}

// DevYAML writes content as the dev.yaml of home, a folder Set returned.
func DevYAML(t testing.TB, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "aicoded")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
