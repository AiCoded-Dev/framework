package dev

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
)

// openManual runs the workspace root like openWatching, with hello's values, and with the apps
// manual run by hand.
func openManual(t *testing.T, root string, out io.Writer, manual ...string) *Workspace {
	t.Helper()
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: out, Watch: true, Manual: manual,
		Dev: devconfig.Workspace{Apps: map[string]devconfig.AppValues{"hello": helloValues}}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	return w
}

func TestManualMode(t *testing.T) {
	requireGo(t)
	shortRuntimeDir(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	w := openManual(t, root, &out, "hello")

	a := appState(t, w, "hello")
	require.Equal(t, devapi.Manual, a.State)
	assert.Equal(t, "cd "+root+" && AICODED_RUNNER_DIR="+a.RunnerDir+" go run .", a.Command)
	info, err := os.Stat(a.RunnerDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	assert.NoDirExists(t, filepath.Join(w.cfg.State, "bin"), "nothing is built for an app run by hand")
	assert.Contains(t, out.String(), "aicoded dev: hello runs by hand; start it with: "+a.Command+"\n")
	serves(t, w, "hello", "/", http.StatusServiceUnavailable, "hello runs by hand and is not running now")

	bin := filepath.Join(t.TempDir(), "hello")
	require.NoError(t, build(t.Context(), root, bin))
	for run := 1; run <= 2; run++ {
		cmd := exec.CommandContext(t.Context(), bin)
		cmd.Dir = root
		cmd.Env = []string{runnerproto.EnvRunnerDir + "=" + a.RunnerDir}
		require.NoError(t, cmd.Start())
		serves(t, w, "hello", "/", http.StatusOK, "hi all-roles (token 3 bytes)")
		require.Eventually(t, func() bool {
			return strings.Count(out.String(), "aicoded dev: hello is ready (run by hand)\n") == run
		}, 10*time.Second, 20*time.Millisecond, "every process that reports ready is announced")
		require.NoError(t, cmd.Process.Kill())
		_ = cmd.Wait()
		serves(t, w, "hello", "/", http.StatusServiceUnavailable, "is not running now")
	}
	assert.Equal(t, devapi.Manual, appState(t, w, "hello").State)

	w.Close()
	assert.NoDirExists(t, a.RunnerDir, "Close removes the run folder")
}

func TestManualModeRegenerates(t *testing.T) {
	requireGo(t)
	shortRuntimeDir(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	w := openManual(t, root, &out, "hello")
	settle(t, w)
	a := appState(t, w, "hello")
	runners := func() int { return strings.Count(out.String(), "aicoded dev: hello runs by hand; start it with: ") }

	rewrite(t, filepath.Join(root, "main.go"), `"%s %s \(token`, `"%s %s, changed (token`)
	regenerated(t, w, "hello")
	assert.Equal(t, a, appState(t, w, "hello"), "the app stays manual, with its command, run folder and since")
	assert.Equal(t, 1, runners(), "the runner keeps running")
	assert.NoDirExists(t, filepath.Join(w.cfg.State, "bin"), "a change regenerates the app and builds nothing")

	restarted, err := w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, a, restarted, "preview keeps the runner of an app whose permission list and values are the same")
	assert.Equal(t, 1, runners())

	rewrite(t, filepath.Join(root, manifest.FileName), `(?m)^app: hello$`, "app: hello\nemail: {from: hello@acme.example, to_domains: [acme.example]}")
	require.Eventually(t, func() bool { return runners() == 2 }, 30*time.Second, 50*time.Millisecond)
	settle(t, w)
	assert.Equal(t, a.RunnerDir, appState(t, w, "hello").RunnerDir, "a new permission list brings up a new runner in the same folder")

	require.NoError(t, w.SetManual("hello", false))
	settle(t, w)
	assert.Equal(t, devapi.Running, appState(t, w, "hello").State)
	assert.NoDirExists(t, a.RunnerDir, "leaving manual mode removes the run folder")
	assert.Equal(t, "hi all-roles, changed (token 3 bytes)", gatewayGet(t, w.port, "hello"))

	require.NoError(t, w.SetManual("hello", true))
	settle(t, w)
	a = appState(t, w, "hello")
	assert.Equal(t, devapi.Manual, a.State)
	assert.DirExists(t, a.RunnerDir)
	serves(t, w, "hello", "/", http.StatusServiceUnavailable, "is not running now")
}

// regenerated waits until a cycle of app in w has seen the app's files as they are now.
func regenerated(t *testing.T, w *Workspace, app string) {
	t.Helper()
	s, err := w.slot(app)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		s.cycling.Lock()
		defer s.cycling.Unlock()
		return s.fingerprint == fingerprint(s.dir)
	}, 30*time.Second, 50*time.Millisecond)
}

// Leaving manual mode while a new runner is prepared ends with the app running and nothing failed.
func TestLeaveManualModeWhileRenewing(t *testing.T) {
	requireGo(t)
	shortRuntimeDir(t)
	root := copyHello(t, t.TempDir(), "hello")
	var starts atomic.Int32
	held, release := make(chan struct{}), make(chan struct{})
	var out syncBuffer
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: &out, Manual: []string{"hello"},
		Values: func(string) (devconfig.AppValues, error) {
			if starts.Add(1) == 2 {
				close(held)
				<-release
			}
			return helloValues, nil
		}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	dir := appState(t, w, "hello").RunnerDir

	require.NoError(t, w.Start("hello"))
	<-held
	require.NoError(t, w.SetManual("hello", false))
	unblock()
	settle(t, w)
	assert.Equal(t, devapi.Running, appState(t, w, "hello").State)
	assert.NotContains(t, out.String(), "hello failed", "the new runner is not brought up in the removed run folder")
	assert.NoDirExists(t, dir)
}

func TestManualAppFails(t *testing.T) {
	root := copyHello(t, t.TempDir(), "hello")
	var mu sync.Mutex
	var values devconfig.AppValues
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard, Manual: []string{"hello"},
		Values: func(string) (devconfig.AppValues, error) {
			mu.Lock()
			defer mu.Unlock()
			return values, nil
		}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	a := appState(t, w, "hello")
	assert.Equal(t, devapi.Failed, a.State)
	require.NotEmpty(t, a.Problems)
	assert.Equal(t, "E-DEV-003", a.Problems[0].Code)
	assert.Empty(t, a.Command)

	mu.Lock()
	values = helloValues
	mu.Unlock()
	a, err = w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, devapi.Manual, a.State, "once fixed, the app runs by hand again")
	assert.DirExists(t, a.RunnerDir)
}

func TestOpenRefusesAnUnknownManualApp(t *testing.T) {
	root, state := copyHello(t, t.TempDir(), "hello"), t.TempDir()
	_, err := Open(t.Context(), Config{Root: root, State: state, Out: io.Discard, Manual: []string{"hello", "nope"}})
	assert.Equal(t, "E-DEV-014", errs.Code(err))
	require.ErrorContains(t, err, `no app named "nope"`)
	assert.NoDirExists(t, filepath.Join(state, "bin"), "nothing starts")
}

func TestManualCommand(t *testing.T) {
	assert.Equal(t, "cd /src/shop && AICODED_RUNNER_DIR=/run/aicoded-1 go run .", manualCommand("/src/shop", "/run/aicoded-1"))
	assert.Equal(t, `cd '/src/my shop' && AICODED_RUNNER_DIR='/tmp/it'\''s/aicoded-1' go run .`, manualCommand("/src/my shop", "/tmp/it's/aicoded-1"))
	assert.Equal(t, `cd '/src/$(x)' && AICODED_RUNNER_DIR=/run/a go run .`, manualCommand("/src/$(x)", "/run/a"))
}
