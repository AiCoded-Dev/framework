package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
)

var (
	rpcOnce sync.Once
	rpcApps *workspace
	rpcErr  error
)

// workspace is a folder of apps running under aicoded dev.
type workspace struct {
	root, state string
	port        int
	out         *output
	hc          *http.Client
	cancel      context.CancelFunc
	done        chan error
}

// output keeps what aicoded dev printed.
type output struct {
	mu sync.Mutex
	b  strings.Builder
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

// lines returns the log lines of app.
func (o *output) lines(app string) string {
	var b strings.Builder
	for line := range strings.Lines(o.String()) {
		if rest, ok := strings.CutPrefix(line, app+" | "); ok {
			b.WriteString(rest)
		}
	}
	return b.String()
}

// startRPC runs testdata/rpc, the shop and billing apps, once per test binary.
func startRPC(t *testing.T) *workspace {
	t.Helper()
	requireGo(t)
	rpcOnce.Do(func() { rpcApps, rpcErr = runWorkspace("rpc", "aicoded dev: shop at ") })
	require.NoError(t, rpcErr)
	return rpcApps
}

// runWorkspace copies testdata/<name> and runs every app in it as aicoded dev does, with the
// e2e personas. It returns once aicoded dev has printed ready.
func runWorkspace(name, ready string) (_ *workspace, err error) {
	root, err := copyApp(name)
	if err != nil {
		return nil, err
	}
	w := &workspace{root: root, out: &output{}, done: make(chan error, 1)}
	defer func() {
		if err != nil {
			w.stop()
		}
	}()
	if w.state, err = os.MkdirTemp("", "aicoded-e2e-state-"); err != nil {
		return nil, err
	}
	l, err := listenLocal()
	if err != nil {
		return nil, err
	}
	w.port = l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	w.hc = newClient(w.port)
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	ws := devconfig.Workspace{Port: w.port, Personas: personas}
	go func() { w.done <- dev.RunWorkspace(ctx, root, ws, w.state, w.out) }()
	deadline := time.After(3 * time.Minute)
	for !strings.Contains(w.out.String(), ready) {
		if strings.Contains(w.out.String(), " failed:\n") {
			return nil, fmt.Errorf("an app failed:\n%s", w.out.String())
		}
		select {
		case err := <-w.done:
			w.done <- err
			return nil, fmt.Errorf("aicoded dev stopped: %w\n%s", err, w.out.String())
		case <-deadline:
			return nil, fmt.Errorf("aicoded dev did not print %q:\n%s", ready, w.out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
	return w, nil
}

func (w *workspace) stop() {
	if w.cancel != nil {
		w.hc.CloseIdleConnections()
		w.cancel()
		select {
		case <-w.done:
		case <-time.After(30 * time.Second):
		}
	}
	_ = os.RemoveAll(w.root)
	_ = os.RemoveAll(w.state)
}

func (w *workspace) get(t *testing.T, persona, app, path string) (*response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://%s.localhost:%d%s", app, w.port, path), nil)
	require.NoError(t, err)
	return fetch(t, w.hc, persona, req)
}

func TestCallAsTheViewer(t *testing.T) {
	w := startRPC(t)
	resp, body := w.get(t, "editor", "shop", "/invoices/1")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, `<h1 id="customer">Hotel Lisboa</h1>`)
	assert.Contains(t, body, `<p id="viewer">editor</p>`, "billing saw the viewer")
	assert.Contains(t, body, `<li>Towels: 1200</li><li>Soap: 300</li>`)
	assert.Contains(t, body, `<p id="due">2026-10-31</p>`)
	assert.Contains(t, body, `<p id="paid">false</p>`, "a pointer to false arrives")

	resp, _ = w.get(t, "viewer", "shop", "/invoices/1")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "billing refuses a viewer without the editor role")
}

func TestCallAsTheApp(t *testing.T) {
	w := startRPC(t)
	_, body := w.get(t, "viewer", "shop", "/status")
	assert.Contains(t, body, `<p id="pong">caller=shop viewer=</p>`, "Ping ran for shop itself, with no viewer")
	assert.Contains(t, body, "E-RPC-010", "Secret takes no calls from an app on its own")
}

func TestCallErrors(t *testing.T) {
	w := startRPC(t)
	_, body := w.get(t, "viewer", "shop", "/checks")
	assert.Contains(t, body, `<p id="fail">internal error</p>`)
	assert.Contains(t, body, "no invoice 7")
	assert.NotContains(t, body, "ledger")
	assert.Eventually(t, func() bool { return strings.Contains(w.out.lines("billing"), "ledger db-7 is locked by job 4411") },
		5*time.Second, 20*time.Millisecond, "billing logs its own error")
	for line := range strings.Lines(w.out.String()) {
		if strings.Contains(line, "ledger") {
			assert.True(t, strings.HasPrefix(line, "billing | "), "only billing logs its error, not shop or the runner: %s", line)
		}
	}
}

// TestRPCPermissionLists checks that the permission lists say what the tests above see.
func TestRPCPermissionLists(t *testing.T) {
	w := startRPC(t)
	shop, err := manifest.Load(filepath.Join(w.root, "shop", manifest.FileName))
	require.NoError(t, err)
	assert.Equal(t, map[string][]string{"billing": {"Fail", "GetInvoice", "Missing", "Ping", "Secret"}}, shop.Services.Calls)

	billing, err := manifest.Load(filepath.Join(w.root, "billing", manifest.FileName))
	require.NoError(t, err)
	assert.Empty(t, billing.Access)
	ping := billing.Services.Serves["Ping"]
	assert.Equal(t, []string{"shop"}, ping.Callers)
	assert.Empty(t, ping.Require)
	assert.True(t, ping.Apps)
	assert.Equal(t, []string{"editor"}, billing.Services.Serves["GetInvoice"].Require)
	assert.False(t, billing.Services.Serves["Secret"].Apps)
}

func TestBrokenSnapshotFailsTheCallee(t *testing.T) {
	requireGo(t)
	root, err := copyApp("rpc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	src := filepath.Join(root, "billing", "rpc", "rpc.go")
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	paid := regexp.MustCompile(`(?m)^\tPaid\s+\*bool\n`)
	require.True(t, paid.Match(data))
	require.NoError(t, os.WriteFile(src, paid.ReplaceAll(data, nil), 0o600))
	snapshot := filepath.Join(root, "billing", ".aicoded", "rpc.json")
	before, err := os.ReadFile(snapshot)
	require.NoError(t, err)

	w, err := dev.Open(t.Context(), dev.Config{Root: root, State: t.TempDir(), Dev: devconfig.Workspace{Personas: personas}, Out: io.Discard})
	require.NoError(t, err)
	defer w.Close()
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	require.Len(t, st.Apps, 2)
	billing, shop := st.Apps[0], st.Apps[1]
	assert.Equal(t, devapi.Failed, billing.State)
	require.NotEmpty(t, billing.Problems)
	assert.Equal(t, "E-RPC-013", billing.Problems[0].Code)
	assert.Contains(t, billing.Problems[0].Pos, filepath.Join(root, "shop", ".aicoded", "services", "billing.json")+":")
	after, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "billing's snapshot is not renumbered")
	require.Len(t, shop.Problems, 1)
	assert.Equal(t, "E-DEV-004", shop.Problems[0].Code, "shop's snapshot still matches billing's, but its OnStart calls billing")
}
