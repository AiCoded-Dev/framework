package watch_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/watch"
)

const quiet = 100 * time.Millisecond

// start watches a new temp folder, skipping names that start with a dot and node_modules, and
// returns the folder.
func start(t *testing.T) (*watch.Watcher, string) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	w, err := watch.New(root, func(rel string) bool {
		name := filepath.Base(rel)
		return strings.HasPrefix(name, ".") || name == "node_modules"
	}, quiet)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w, root
}

// next returns the next burst, or fails after 5s.
func next(t *testing.T, w *watch.Watcher) []string {
	t.Helper()
	select {
	case paths := <-w.Changes():
		return paths
	case <-time.After(5 * time.Second):
		t.Fatal("no changes")
		return nil
	}
}

// none checks that no burst comes within 3 quiet periods.
func none(t *testing.T, w *watch.Watcher) {
	t.Helper()
	select {
	case paths := <-w.Changes():
		t.Fatalf("unexpected changes %v", paths)
	case <-time.After(3 * quiet):
	}
}

func write(t *testing.T, path, data string) {
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func TestBurstsAreCoalesced(t *testing.T) {
	w, root := start(t)
	a, b := filepath.Join(root, "a.go"), filepath.Join(root, "b.go")
	for i := range 5 {
		write(t, a, strings.Repeat("x", i))
	}
	write(t, b, "b")
	assert.Equal(t, []string{a, b}, next(t, w))
	none(t, w)
}

func TestNewFoldersAreWatched(t *testing.T) {
	w, root := start(t)
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.Mkdir(sub, 0o750))
	assert.Equal(t, []string{sub}, next(t, w))
	file := filepath.Join(sub, "f.go")
	write(t, file, "f")
	assert.Equal(t, []string{file}, next(t, w))

	moved := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.MkdirAll(filepath.Join(moved, "pages"), 0o750))
	write(t, filepath.Join(moved, "pages", "index.html"), "x")
	app := filepath.Join(root, "app")
	require.NoError(t, os.Rename(moved, app))
	assert.Equal(t, []string{app, filepath.Join(app, "pages"), filepath.Join(app, "pages", "index.html")}, next(t, w),
		"a folder moved in brings its files")
	write(t, filepath.Join(app, "pages", "index.html"), "y")
	assert.Equal(t, []string{filepath.Join(app, "pages", "index.html")}, next(t, w))
}

func TestSkippedFoldersAreNeverReported(t *testing.T) {
	w, root := start(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o750))
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o750))
	write(t, filepath.Join(root, "node_modules", "x", "x.js"), "x")
	write(t, filepath.Join(root, ".git", "HEAD"), "x")
	write(t, filepath.Join(root, ".main.go.swp"), "x")
	none(t, w)
	write(t, filepath.Join(root, "main.go"), "x")
	assert.Equal(t, []string{filepath.Join(root, "main.go")}, next(t, w))
}

func TestCloseEndsChanges(t *testing.T) {
	w, _ := start(t)
	require.NoError(t, w.Close())
	_, ok := <-w.Changes()
	assert.False(t, ok)
	_, ok = <-w.Errors()
	assert.False(t, ok)
}
