package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
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
)

// openWorkspace runs the workspace root on a free port with values, a temp state folder and out, and
// closes it when the test ends.
func openWorkspace(t *testing.T, root string, values map[string]devconfig.AppValues, out io.Writer) *Workspace {
	t.Helper()
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Dev: devconfig.Workspace{Apps: values}, Out: out})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	return w
}

// appState returns the status of app in w.
func appState(t *testing.T, w *Workspace, app string) devapi.App {
	t.Helper()
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	for _, a := range st.Apps {
		if a.Name == app {
			return a
		}
	}
	t.Fatalf("no app %s in %+v", app, st)
	return devapi.App{}
}

// shortRuntimeDir points XDG_RUNTIME_DIR at a new short folder and returns it.
func shortRuntimeDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "aicoded-rt-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	return dir
}

func TestOpenBusyPort(t *testing.T) {
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	root, state := copyHello(t, t.TempDir(), "hello"), t.TempDir()

	_, err = Open(t.Context(), Config{Root: root, State: state, Dev: devconfig.Workspace{Port: port}, Out: io.Discard})
	assert.Equal(t, "E-DEV-011", errs.Code(err))
	require.ErrorContains(t, err, fmt.Sprintf("port %d is in use", port))
	assert.NoDirExists(t, filepath.Join(state, "bin"), "nothing was built")
}

func TestOneBrokenAppDoesNotStopOthers(t *testing.T) {
	requireGo(t)
	runtime := shortRuntimeDir(t)
	root := t.TempDir()
	copyHello(t, filepath.Join(root, "a"), "hello")
	broken := copyHello(t, filepath.Join(root, "b"), "broken")
	writeFile(t, filepath.Join(broken, "main.go"), "package main\n\nfunc main() {\n\tvar c chan<- int = 1\n\t_ = c\n}\n")
	var out syncBuffer
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues, "broken": helloValues}, &out)

	code, body := fetch(t, w.port, "hello", "/")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "hi all-roles (token 3 bytes)", body)
	code, body = fetch(t, w.port, "broken", "/")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "E-CHK-002")
	assert.Contains(t, body, "chan&lt;- int", "the compiler's message, escaped")
	assert.NotContains(t, body, "chan<- int")

	assert.Equal(t, devapi.Running, appState(t, w, "hello").State)
	failed := appState(t, w, "broken")
	assert.Equal(t, devapi.Failed, failed.State)
	require.NotEmpty(t, failed.Problems)
	assert.Equal(t, "E-CHK-002", failed.Problems[0].Code)
	assert.Contains(t, out.String(), "aicoded dev: broken failed:\n  ")
	assert.Contains(t, out.String(), "aicoded dev: hello at http://hello.localhost:")

	w.Close()
	assert.Equal(t, devapi.Stopped, appState(t, w, "hello").State)
	entries, err := os.ReadDir(runtime)
	require.NoError(t, err)
	assert.Empty(t, entries, "Close stopped hello and removed its sockets")
	assert.NoDirExists(t, filepath.Join(w.cfg.State, "bin"))
}

func TestRestartRecovers(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	main := filepath.Join(copyHello(t, filepath.Join(root, "hello"), "hello"), "main.go")
	good, err := os.ReadFile(main)
	require.NoError(t, err)
	writeFile(t, main, "package main\n\nfunc main() { undefined() }\n")
	var out syncBuffer
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	assert.Equal(t, devapi.Failed, appState(t, w, "hello").State)

	require.NoError(t, os.WriteFile(main, good, 0o600))
	a, err := w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, devapi.Running, a.State)
	assert.Empty(t, a.Problems)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"))
	assert.Contains(t, out.String(), "aicoded dev: hello at http://hello.localhost:", "the first start prints the address")

	_, err = w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "aicoded dev: hello restarted\n")
	bins, err := os.ReadDir(filepath.Join(w.cfg.State, "bin"))
	require.NoError(t, err)
	require.Len(t, bins, 1, "the older builds are removed")
	assert.Equal(t, "hello.3", bins[0].Name())

	for _, name := range []string{"nope", "../x", ""} {
		_, err := w.Restart(t.Context(), name)
		assert.Equal(t, "E-DEV-014", errs.Code(err), "%q", name)
	}
}

func TestExitMarksFailed(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	copyHello(t, filepath.Join(root, "hello"), "hello")
	copyHello(t, filepath.Join(root, "hey"), "hey")
	var out syncBuffer
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues, "hey": helloValues}, &out)

	fetch(t, w.port, "hello", "/exit")
	require.Eventually(t, func() bool { return appState(t, w, "hello").State == devapi.Failed }, 10*time.Second, 20*time.Millisecond)
	problems := appState(t, w, "hello").Problems
	require.Len(t, problems, 1)
	assert.Equal(t, "E-DEV-004", problems[0].Code)
	assert.Equal(t, "hello exited", problems[0].Message)
	assert.Contains(t, out.String(), "aicoded dev: hello failed:\n  E-DEV-004: hello exited\n")

	code, _ := fetch(t, w.port, "hello", "/")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hey"), "hey keeps running")
	assert.Equal(t, devapi.Running, appState(t, w, "hey").State)
}

func TestBrokenHeldSnapshotFailsOnlyItsApps(t *testing.T) {
	requireGo(t)
	root := t.TempDir()
	for _, app := range []string{"billing", "shop"} {
		writeFile(t, filepath.Join(root, app, manifest.FileName), "app: "+app+"\n")
		writeFile(t, filepath.Join(root, app, "go.mod"), "module "+app+"\n\ngo 1.26.0\n")
	}
	held := filepath.Join(root, "shop", ".aicoded", "services")
	writeFile(t, filepath.Join(held, "ledger.json"), ledgerSnapshot)
	copyHello(t, filepath.Join(root, "hello"), "hello")
	var out syncBuffer
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	assert.Contains(t, out.String(), "aicoded dev: shop holds a snapshot of ledger, which is not in this workspace\n")
	assert.Equal(t, "E-CHK-002", appState(t, w, "shop").Problems[0].Code, "a snapshot of an app outside the workspace stops nothing")
	w.Close()

	writeFile(t, filepath.Join(held, "billing.json"), invoiceSnapshot)
	w = openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, io.Discard)
	for _, app := range []string{"billing", "shop"} {
		a := appState(t, w, app)
		assert.Equal(t, devapi.Failed, a.State, app)
		require.NotEmpty(t, a.Problems, app)
		assert.Equal(t, "E-RPC-013", a.Problems[0].Code, app)
		assert.Contains(t, a.Problems[0].Pos, filepath.Join("shop", ".aicoded", "services", "billing.json")+":", app)
	}
	assert.Equal(t, devapi.Running, appState(t, w, "hello").State)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"))
	bins, err := os.ReadDir(filepath.Join(w.cfg.State, "bin"))
	require.NoError(t, err)
	require.Len(t, bins, 1, "neither billing nor shop was built")
	assert.Equal(t, "hello.1", bins[0].Name())
}

func TestGenerateFailureFailsOnlyTheApp(t *testing.T) {
	root := pagesApp(t, "<p>open</p>\n")
	w := openWorkspace(t, root, nil, io.Discard)
	a := appState(t, w, "hello")
	assert.Equal(t, devapi.Failed, a.State)
	require.NotEmpty(t, a.Problems)
	assert.Equal(t, "E-GEN-030", a.Problems[0].Code)
	assert.Equal(t, "hello", a.Problems[0].App)
	assert.NoDirExists(t, filepath.Join(w.cfg.State, "bin"), "nothing was built")
}

func TestValuesReadAtStart(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var mu sync.Mutex
	var values devconfig.AppValues
	var failure error
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard,
		Values: func(app string) (devconfig.AppValues, error) {
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, "hello", app)
			return values, failure
		}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	a := appState(t, w, "hello")
	assert.Equal(t, devapi.Failed, a.State)
	require.NotEmpty(t, a.Problems)
	assert.Equal(t, "E-DEV-003", a.Problems[0].Code)

	mu.Lock()
	failure = errs.New("E-DEV-002", "dev.yaml is not valid", "fix the file")
	mu.Unlock()
	a, err = w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, "E-DEV-002", a.Problems[0].Code, "a dev.yaml that cannot be read fails the app")

	mu.Lock()
	values, failure = helloValues, nil
	mu.Unlock()
	a, err = w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, devapi.Running, a.State)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"))
}

func TestValuesReadBeforeGenerate(t *testing.T) {
	root := pagesApp(t, "<p>open</p>\n")
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard,
		Values: func(string) (devconfig.AppValues, error) {
			return devconfig.AppValues{}, errs.New("E-DEV-002", "dev.yaml is not valid", "fix the file")
		}})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	a := appState(t, w, "hello")
	require.Len(t, a.Problems, 1)
	assert.Equal(t, "E-DEV-002", a.Problems[0].Code, "not the E-GEN-030 of generate")
}

func TestRenamedAppFails(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, manifest.FileName), "app: shop\nsettings: [greeting]\n")
	writeFile(t, filepath.Join(root, "go.mod"), "module shop\n\ngo 1.26.0\n")
	w := openWorkspace(t, root, nil, io.Discard)
	writeFile(t, filepath.Join(root, manifest.FileName), "app: store\nsettings: [greeting]\n")

	a, err := w.Restart(t.Context(), "shop")
	require.NoError(t, err)
	assert.Equal(t, devapi.Failed, a.State)
	require.Len(t, a.Problems, 1)
	assert.Equal(t, "E-DEV-015", a.Problems[0].Code)
	assert.Equal(t, "change app: in "+filepath.Join(root, manifest.FileName)+" back to shop, or wait for aicoded dev to restart the app under its new name", a.Problems[0].Fix)
	assert.Equal(t, "shop", w.manifests()[0].App, "the slot keeps the permission list of the name it runs under")
	code, body := fetch(t, w.port, "shop", "/")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, "<b>E-DEV-015</b>")
}

func TestOpenStartsCalleesFirst(t *testing.T) {
	root := t.TempDir()
	for dir, lines := range map[string]string{
		"a": "app: shop\nsettings: [greeting]\nservices:\n  calls:\n    billing: [Ping]\n",
		"b": "app: billing\nsettings: [greeting]\n",
	} {
		writeFile(t, filepath.Join(root, dir, manifest.FileName), lines)
		writeFile(t, filepath.Join(root, dir, "go.mod"), "module "+dir+"\n\ngo 1.26.0\n")
	}
	var out syncBuffer
	openWorkspace(t, root, nil, &out)
	billing, shop := strings.Index(out.String(), "aicoded dev: billing failed:"), strings.Index(out.String(), "aicoded dev: shop failed:")
	require.GreaterOrEqual(t, billing, 0, out.String())
	assert.Less(t, billing, shop, "billing, which shop calls, starts first")
}

func TestStatus(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, io.Discard)
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, root, st.Root)
	assert.Equal(t, fmt.Sprintf("http://localhost:%d/", w.port), st.Gateway)
	require.Len(t, st.Apps, 1)
	a := st.Apps[0]
	assert.Equal(t, "hello", a.Name)
	assert.Equal(t, root, a.Dir)
	assert.Equal(t, devapi.Running, a.State)
	assert.Equal(t, fmt.Sprintf("http://hello.localhost:%d/", w.port), a.URL)
	assert.False(t, a.CallsOnly)
	assert.WithinDuration(t, time.Now(), a.Since, time.Minute)
	assert.Empty(t, a.Problems)
}

func TestOpenNeedsAbsolutePaths(t *testing.T) {
	_, err := Open(t.Context(), Config{Root: "apps", State: t.TempDir(), Out: io.Discard})
	require.ErrorContains(t, err, "absolute")
	_, err = Open(t.Context(), Config{Root: t.TempDir(), State: "state", Out: io.Discard})
	require.ErrorContains(t, err, "absolute")
}

// waitLogs waits until w holds a log entry that q matches, and returns the entries q matches.
func waitLogs(t *testing.T, w *Workspace, q devapi.LogQuery) []devapi.LogEntry {
	t.Helper()
	var entries []devapi.LogEntry
	require.Eventually(t, func() bool {
		var err error
		entries, err = w.Logs(t.Context(), q)
		return err == nil && len(entries) > 0
	}, 10*time.Second, 20*time.Millisecond, "no log entry for %+v", q)
	return entries
}

func TestLogsSurviveRestart(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	assert.Contains(t, out.String(), "aicoded dev: secret token of hello is shorter than 4 bytes, so its value is not hidden\n")
	code, _ := fetch(t, w.port, "hello", "/leak")
	require.Equal(t, http.StatusOK, code)
	entries := waitLogs(t, w, devapi.LogQuery{App: "hello", Contains: "leaked"})
	assert.Equal(t, "leaked abc", entries[0].Message, "a 3-byte secret is not hidden")
	assert.Equal(t, sourceApp, entries[0].Source)
	assert.Equal(t, "ERROR", entries[0].Level)
	assert.Len(t, entries[0].TraceID, 32)
	assert.Contains(t, out.String(), "hello | plain abc\n")

	_, err := w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	after, err := w.Logs(t.Context(), devapi.LogQuery{App: "hello", Contains: "leaked"})
	require.NoError(t, err)
	assert.Equal(t, entries, after, "the logs of the stopped instance are kept")
}

func TestNoSecretInAnyQuery(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	values := devconfig.AppValues{Settings: map[string]string{"greeting": "hi"}, Secrets: map[string]string{"token": leaked}}
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": values}, &out)
	code, _ := fetch(t, w.port, "hello", "/leak")
	require.Equal(t, http.StatusOK, code)
	waitLogs(t, w, devapi.LogQuery{App: "hello", Contains: "leaked"})
	var traces []devapi.TraceSummary
	require.Eventually(t, func() bool {
		traces, _ = w.Traces(t.Context(), devapi.TraceQuery{ErrorsOnly: true})
		return len(traces) > 0
	}, 10*time.Second, 20*time.Millisecond, "the span of /leak is exported within a second or so")

	st, err := w.Status(t.Context())
	require.NoError(t, err)
	logs, err := w.Logs(t.Context(), devapi.LogQuery{})
	require.NoError(t, err)
	trace, err := w.Trace(t.Context(), traces[0].TraceID)
	require.NoError(t, err)
	broke, err := w.WhatBroke(t.Context(), "")
	require.NoError(t, err)
	mail, err := w.Mail(t.Context(), "hello")
	require.NoError(t, err)
	for name, v := range map[string]any{"status": st, "logs": logs, "traces": traces, "trace": trace, "what broke": broke, "mail": mail} {
		assert.NotContains(t, jsonOf(t, v), leaked, name)
	}
	assert.Contains(t, jsonOf(t, logs), `"message":"leaked [secret token]"`)
	assert.Contains(t, jsonOf(t, logs), `"headers":["Bearer [secret token]"]`)
	assert.Contains(t, jsonOf(t, trace), `"token":"[secret token]"`)
	assert.Contains(t, jsonOf(t, broke), `"error":"leaked [secret token]"`)
	assert.Contains(t, out.String(), "hello | plain [secret token]\n")
	assert.NotContains(t, out.String(), leaked, "nor does the terminal")
}

func TestStopAndStartRequests(t *testing.T) {
	w := testWorkspace("shop", "shop")
	w.slots[1].dup.Store(true)
	for _, name := range []string{"nope", "../x", ""} {
		assert.Equal(t, "E-DEV-014", errs.Code(w.Stop(name)), "%q", name)
		assert.Equal(t, "E-DEV-014", errs.Code(w.Start(name)), "%q", name)
	}
	require.NoError(t, w.Stop("shop"))
	settle(t, w)
	assert.Equal(t, devapi.Stopped, w.appStatus(w.slots[0]).State)
	assert.Equal(t, devapi.Starting, w.appStatus(w.slots[1]).State, "the app that waits for the name is left alone")
	assert.Contains(t, serve(w.gateway, http.MethodGet, "shop.localhost:8080", "/", nil, "").Body.String(), "shop is stopped.")

	w.cancel()
	require.EqualError(t, w.Stop("shop"), "aicoded dev is stopping")
	require.EqualError(t, w.Start("shop"), "aicoded dev is stopping")
}

func TestStopAndStart(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var out syncBuffer
	w := openWatching(t, root, map[string]devconfig.AppValues{"hello": helloValues}, &out)
	settle(t, w)

	require.NoError(t, w.Stop("hello"))
	settle(t, w)
	a := appState(t, w, "hello")
	assert.Equal(t, devapi.Stopped, a.State)
	assert.Empty(t, a.Problems)
	code, body := fetch(t, w.port, "hello", "/")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, fmt.Sprintf(`hello is stopped. Start it on the dev UI at <a href="http://localhost:%d/apps/hello">`, w.port))
	assert.Contains(t, out.String(), "aicoded dev: hello stopped\n")

	rewrite(t, filepath.Join(root, "main.go"), `"%s %s \(token`, `"%s %s, changed (token`)
	time.Sleep(time.Second)
	settle(t, w)
	assert.Equal(t, devapi.Stopped, appState(t, w, "hello").State, "a change does not rebuild a stopped app")

	require.NoError(t, w.Start("hello"))
	settle(t, w)
	assert.Equal(t, "hi all-roles, changed (token 3 bytes)", gatewayGet(t, w.port, "hello"))
	since := appState(t, w, "hello").Since
	require.NoError(t, w.Start("hello"))
	settle(t, w)
	a = appState(t, w, "hello")
	assert.Equal(t, devapi.Running, a.State)
	assert.True(t, a.Since.After(since), "Start restarts a running app")

	require.NoError(t, w.Start("hello"))
	require.NoError(t, w.Stop("hello"))
	settle(t, w)
	assert.Equal(t, devapi.Stopped, appState(t, w, "hello").State, "the last request wins")
	a, err := w.Restart(t.Context(), "hello")
	require.NoError(t, err)
	assert.Equal(t, devapi.Running, a.State, "Restart starts a stopped app")
	require.NoError(t, w.Stop("hello"))
	require.NoError(t, w.Start("hello"))
	settle(t, w)
	assert.Equal(t, devapi.Running, appState(t, w, "hello").State, "the last request wins")
}

func TestCycleKeepsServingTheOldApp(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	var starts atomic.Int32
	held, release := make(chan struct{}), make(chan struct{})
	w, err := Open(t.Context(), Config{Root: root, State: t.TempDir(), Out: io.Discard,
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

	done := make(chan error, 1)
	go func() {
		_, err := w.Restart(t.Context(), "hello")
		done <- err
	}()
	<-held
	assert.Equal(t, devapi.Starting, appState(t, w, "hello").State)
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, w.port, "hello"), "the running app answers while the next one is prepared")
	unblock()
	require.NoError(t, <-done)
	assert.Equal(t, devapi.Running, appState(t, w, "hello").State)
}

func TestStoppingAppGetsItsPage(t *testing.T) {
	requireGo(t)
	root := copyHello(t, t.TempDir(), "hello")
	w := openWorkspace(t, root, map[string]devconfig.AppValues{"hello": helloValues}, io.Discard)
	mainGo := filepath.Join(root, "main.go")
	start := func() error { return w.Start("hello") }
	run := func() {
		require.NoError(t, start())
		settle(t, w)
	}
	starting := "<title>hello is starting</title>"

	pageWhileStopping(t, w, start, starting, devapi.Starting, devapi.Running)
	pageWhileStopping(t, w, func() error { return w.Stop("hello") }, "hello is stopped.", devapi.Stopped, devapi.Stopped)
	run()
	rewrite(t, mainGo, `func main\(\) \{`, "func main() { broken()")
	pageWhileStopping(t, w, start, "could not build or start the app", devapi.Failed, devapi.Failed)
	rewrite(t, mainGo, `broken\(\)`, "")
	run()
	pageWhileStopping(t, w, func() error { return w.SetManual("hello", true) }, starting, devapi.Starting, devapi.Manual)

	unanswered, err := w.Logs(t.Context(), devapi.LogQuery{App: "hello", Contains: "did not answer"})
	require.NoError(t, err)
	assert.Empty(t, unanswered)
}

// pageWhileStopping keeps a request to the running app hello of w open and asks for a change
// with ask. Once the app has closed its socket, while it still serves that request, it checks
// that the gateway answers 503 with a page that holds text and that the app is in state while.
// Then it ends the request and checks that the app ends in state.
func pageWhileStopping(t *testing.T, w *Workspace, ask func() error, text string, while, state devapi.State) {
	t.Helper()
	s, err := w.slot("hello")
	require.NoError(t, err)
	s.mu.Lock()
	sock := s.inst.AppSocket()
	s.mu.Unlock()
	ctx, release := context.WithCancel(t.Context())
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/hold", w.port), nil)
	require.NoError(t, err)
	req.Host = fmt.Sprintf("hello.localhost:%d", w.port)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, ask())
	require.Eventually(t, func() bool {
		_, err := os.Stat(sock)
		return errors.Is(err, fs.ErrNotExist)
	}, 30*time.Second, 10*time.Millisecond, "the app did not begin to stop")
	code, body := fetch(t, w.port, "hello", "/")
	during := appState(t, w, "hello").State
	release()
	settle(t, w)
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Contains(t, body, text)
	assert.Equal(t, while, during, "the state matches the page while the app stops")
	assert.Equal(t, state, appState(t, w, "hello").State)
}

func TestCloseEndsRequests(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, manifest.FileName), "app: shop\n")
	writeFile(t, filepath.Join(root, "go.mod"), "module shop\n\ngo 1.26.0\n")
	w := openWorkspace(t, root, nil, io.Discard)
	s, err := w.slot("shop")
	require.NoError(t, err)
	held := make(chan struct{})
	w.gateway.Add("shop", serveAt(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(held)
		<-r.Context().Done()
	})), s.store)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = get(t.Context(), w.port, "shop", "/")
	}()
	<-held
	start := time.Now()
	w.Close()
	assert.Less(t, time.Since(start), 2*time.Second, "Close ended the request instead of waiting for it")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the request did not end")
	}
}

// unbuilt writes an app named name at dir that aicoded dev fails at once, without building it.
func unbuilt(t *testing.T, dir, name string) {
	writeFile(t, filepath.Join(dir, manifest.FileName), "app: "+name+"\n")
}

// uiGet gets path from the dev UI on port, with the cookie header cookie when it is not "",
// without following redirects, and returns the status, the headers and the body of the answer.
func uiGet(t *testing.T, port int, path, cookie string) (int, http.Header, string) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	require.NoError(t, err)
	req.Host = fmt.Sprintf("localhost:%d", port)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(body)
}

func TestOpenServesTheDevUI(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	unbuilt(t, filepath.Join(root, "shop"), "shop")
	var out syncBuffer
	w, err := Open(t.Context(), Config{Root: root, State: state, Out: &out, Login: true})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	token, err := readToken(state, false)
	require.NoError(t, err)
	link := fmt.Sprintf("http://localhost:%d/_aicoded/login?token=%s", w.port, token)
	assert.True(t, strings.HasPrefix(out.String(), "aicoded dev: the dev UI is at "+link+"\n"), "the link comes before the apps start:\n%s", out.String())

	status, _, _ := uiGet(t, w.port, "/apps", "")
	assert.Equal(t, http.StatusUnauthorized, status)
	status, header, _ := uiGet(t, w.port, "/_aicoded/login?token="+token, "")
	require.Equal(t, http.StatusSeeOther, status)
	c, err := http.ParseSetCookie(header.Get("Set-Cookie"))
	require.NoError(t, err)
	status, _, body := uiGet(t, w.port, "/apps", c.Name+"="+c.Value)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "<title>aicoded dev</title>")
	assert.Contains(t, body, "shop", "the pages show the workspace")

	w.server.ErrorLog.Print("http: superfluous response.WriteHeader call")
	_, rest, _ := strings.Cut(out.String(), "\n")
	assert.Contains(t, rest, "aicoded dev: http: superfluous response.WriteHeader call\n", "the gateway's errors go to the terminal")
	assert.NotContains(t, rest, token, "only the first line holds the token")
}

func TestOpenWithoutLogin(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	unbuilt(t, filepath.Join(root, "shop"), "shop")
	var out syncBuffer
	w, err := Open(t.Context(), Config{Root: root, State: state, Out: &out})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	token, err := readToken(state, false)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out.String(), fmt.Sprintf("aicoded dev: the dev UI is at http://localhost:%d/; run aicoded dev in %s to get its login link\n", w.port, root)), out.String())
	assert.NotContains(t, out.String(), token)
}

func TestOpenRefusesABadToken(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	unbuilt(t, filepath.Join(root, "shop"), "shop")
	require.NoError(t, os.Mkdir(filepath.Join(state, tokenFile), 0o700))
	var out syncBuffer
	_, err := Open(t.Context(), Config{Root: root, State: state, Out: &out})
	assert.Equal(t, "E-DEV-018", errs.Code(err))
	assert.Empty(t, out.String(), "nothing started")
}
