package dev

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
)

// openWatching runs the workspace root like openWorkspace, and watches its files.
func openWatching(t *testing.T, root string, values map[string]devconfig.AppValues, out io.Writer) *Workspace {
	t.Helper()
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Dev: devconfig.Workspace{Apps: values}, Out: out, Watch: true})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	return w
}

// settle waits until no app of w checks its files for a change or applies a request.
func settle(t *testing.T, w *Workspace) {
	t.Helper()
	require.Eventually(t, func() bool {
		return !slices.ContainsFunc(w.allSlots(), func(s *slot) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.checking
		})
	}, 30*time.Second, 20*time.Millisecond)
}

// serves waits until app answers path with code and a body that contains text.
func serves(t *testing.T, w *Workspace, app, path string, code int, text string) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, body, err := get(t.Context(), w.port, app, path)
		return err == nil && got == code && strings.Contains(body, text)
	}, 30*time.Second, 50*time.Millisecond, "%s%s never answered %d with %q", app, path, code, text)
}

func TestSkip(t *testing.T) {
	for rel, want := range map[string]bool{
		"": false, "shop": false, ".aicoded": false, "shop/.aicoded": false, "shop/pages": false, "shop/assets_gen": false,
		".git": true, "shop/.idea": true, "_old": true, "shop/vendor": true, "node_modules": true, "a/testdata": true,
		"shop/pages/assets_gen": true, "shop/.main.go.swp": true,
	} {
		assert.Equal(t, want, Skip(rel), "%q", rel)
	}
}

func TestFingerprintSkipsGenerated(t *testing.T) {
	dir := copyHello(t, t.TempDir(), "hello")
	before := fingerprint(dir)
	for _, name := range []string{"pages/route_gen.go", "pages/reactive_gen.ts", ".aicoded/rpc.json", "pages/assets_gen/app.js", "node_modules/x.js", ".main.go.swp"} {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	assert.Equal(t, before, fingerprint(dir), "files that generate writes, skipped folders and hidden files do not count")
	writeFile(t, filepath.Join(dir, ".aicoded", "services", "billing.json"), "{}")
	held := fingerprint(dir)
	assert.NotEqual(t, before, held, "a held snapshot counts")
	rewrite(t, filepath.Join(dir, "main.go"), "hello", "hi")
	assert.NotEqual(t, held, fingerprint(dir))
}

func TestWatchRebuildsOnChange(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	settle(t, w)
	rewrite(t, filepath.Join(root, "main.go"), `"%s %s \(token`, `"%s %s, changed (token`)
	serves(t, w, "hello", "/", http.StatusOK, "hi all-roles, changed (token 3 bytes)")
	assert.Contains(t, out.String(), "aicoded dev: hello restarted\n")
}

func TestWatchBrokenThenFixed(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	main := filepath.Join(root, "main.go")
	good, err := os.ReadFile(main)
	require.NoError(t, err)
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues}, io.Discard)
	settle(t, w)

	writeFile(t, main, "package main\n\nfunc main() {\n")
	serves(t, w, "hello", "/", http.StatusServiceUnavailable, "E-CHK-002")
	require.NoError(t, os.WriteFile(main, good, 0o600))
	serves(t, w, "hello", "/", http.StatusOK, "hi all-roles (token 3 bytes)")
}

func TestWatchIgnoresOwnWrites(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	page := filepath.Join(root, "pages", "index.html")
	writeFile(t, page, `<!doctype html><html><head><ssr:access role="*"/><title>x</title></head><body><p>one</p></body></html>`)
	var out syncBuffer
	openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	require.FileExists(t, filepath.Join(root, "pages", "route_gen.go"), "the first cycle generated the page")
	time.Sleep(2 * time.Second)
	assert.NotContains(t, out.String(), "restarted", "the first cycle's own writes start no other")

	rewrite(t, page, "one", "two")
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "aicoded dev: hello restarted\n") }, 30*time.Second, 50*time.Millisecond)
	time.Sleep(2 * time.Second)
	assert.Equal(t, 1, strings.Count(out.String(), "restarted"), "generate rewrote route_gen.go, and that starts no other cycle")
}

func TestWatchNewAndRemovedApp(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	copyHello(t, filepath.Join(root, "hello"), "hello")
	var out syncBuffer
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues, "hey": helloValues}, &out)

	staged := copyHello(t, filepath.Join(t.TempDir(), "hey"), "hey")
	require.NoError(t, os.Rename(staged, filepath.Join(root, "hey")))
	serves(t, w, "hey", "/", http.StatusOK, "hi all-roles (token 3 bytes)")
	assert.Contains(t, out.String(), "aicoded dev: hey at http://hey.localhost:")

	require.NoError(t, os.RemoveAll(filepath.Join(root, "hey")))
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "aicoded dev: hey removed\n") }, 30*time.Second, 50*time.Millisecond)
	code, _ := fetch(t, w.port, "hey", "/")
	assert.Equal(t, http.StatusNotFound, code)
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, st.Apps, 1)
	assert.Equal(t, "hello", st.Apps[0].Name)
}

func TestWatchRescansAfterLostEvents(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	copyHello(t, filepath.Join(root, "hello"), "hello")
	var out syncBuffer
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues, "hey": helloValues}, &out)
	settle(t, w)
	require.NoError(t, w.watcher.Close(), "no change reaches aicoded dev from now on")

	rewrite(t, filepath.Join(root, "hello", "main.go"), `"%s %s \(token`, `"%s %s, changed (token`)
	copyHello(t, filepath.Join(root, "hey"), "hey")
	w.watchError(fmt.Errorf("watch: %w", fsnotify.ErrEventOverflow))
	serves(t, w, "hello", "/", http.StatusOK, "hi all-roles, changed (token 3 bytes)")
	serves(t, w, "hey", "/", http.StatusOK, "hi all-roles (token 3 bytes)")
	assert.Contains(t, out.String(), "aicoded dev: watch: fsnotify: queue or buffer overflow\n")
}

func TestRescan(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	first := copyHello(t, filepath.Join(root, "a"), "hello")
	var out syncBuffer
	values := map[string]devconfig.AppValues{"hello": helloValues, "hey": helloValues, "hola": helloValues}
	w := openWorkspace(t, root, values, &out)

	second := copyHello(t, filepath.Join(root, "b"), "hello")
	w.Rescan(t.Context())
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, st.Apps, 2)
	assert.Equal(t, devapi.Running, st.Apps[0].State)
	assert.Equal(t, devapi.Failed, st.Apps[1].State)
	require.Len(t, st.Apps[1].Problems, 1)
	assert.Equal(t, "E-DEV-010", st.Apps[1].Problems[0].Code)
	assert.Contains(t, st.Apps[1].Problems[0].Message, first+" and "+second)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"), "the app that was there first keeps its address")

	rewrite(t, filepath.Join(second, manifest.FileName), `(?m)^app: hello$`, "app: hey")
	w.Rescan(t.Context())
	assert.Equal(t, devapi.Running, appState(t, w, "hey").State)
	rewrite(t, filepath.Join(first, manifest.FileName), `(?m)^app: hello$`, "app: hola")
	w.Rescan(t.Context())
	assert.Equal(t, devapi.Running, appState(t, w, "hola").State)
	assert.Contains(t, out.String(), "aicoded dev: hello removed\n")
	code, _ := fetch(t, w.port, "hello", "/")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, 1, strings.Count(out.String(), "removed"), "the waiting duplicate went silently")

	bad := filepath.Join(root, "c")
	writeFile(t, filepath.Join(bad, manifest.FileName), "app: Not-Valid\n")
	line := "aicoded dev: " + bad + ": " + filepath.Join(bad, manifest.FileName) + ":1: E-MAN-004: "
	w.Rescan(t.Context())
	w.Rescan(t.Context())
	assert.Equal(t, 1, strings.Count(out.String(), line), "a folder whose permission list does not load is named once")
	require.NoError(t, os.RemoveAll(bad))
	w.Rescan(t.Context())
	writeFile(t, filepath.Join(bad, manifest.FileName), "app: Not-Valid\n")
	w.Rescan(t.Context())
	assert.Equal(t, 2, strings.Count(out.String(), line), "and again once it came back")
}

func TestWatchKeepsADuplicateWaiting(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	first := copyHello(t, filepath.Join(root, "a"), "hello")
	var out syncBuffer
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)

	staged := copyHello(t, filepath.Join(t.TempDir(), "b"), "hello")
	rewrite(t, filepath.Join(staged, "main.go"), `"%s %s \(token`, `"%s %s, second (token`)
	require.NoError(t, os.Rename(staged, filepath.Join(root, "b")))
	require.Eventually(t, func() bool { return strings.Contains(out.String(), "E-DEV-010") }, 30*time.Second, 50*time.Millisecond)
	time.Sleep(2 * time.Second)
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, st.Apps, 2)
	assert.Equal(t, devapi.Failed, st.Apps[1].State)
	require.Len(t, st.Apps[1].Problems, 1)
	assert.Equal(t, "E-DEV-010", st.Apps[1].Problems[0].Code)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"), "the app that was there first keeps its address")

	require.NoError(t, os.RemoveAll(first))
	serves(t, w, "hello", "/", http.StatusOK, "hi all-roles, second (token 3 bytes)")
}

func TestWatchThroughALink(t *testing.T) {
	requireGo(t)
	dir := copyHello(t, t.TempDir(), "hello")
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(dir, link))
	w := openWatching(t, link, map[string]devconfig.AppValues{"hello": helloValues}, io.Discard)
	settle(t, w)
	rewrite(t, filepath.Join(dir, "main.go"), `"%s %s \(token`, `"%s %s, changed (token`)
	serves(t, w, "hello", "/", http.StatusOK, "hi all-roles, changed (token 3 bytes)")
}
