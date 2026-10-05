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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
)

// watched is a copy of the rpc pair that aicoded dev runs and watches.
type watched struct {
	root string
	ws   *dev.Workspace
	hc   *http.Client
	port int
}

func watchRPC(t *testing.T) *watched {
	requireGo(t)
	root, err := copyApp("rpc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	state, err := os.MkdirTemp("", "aicoded-e2e-state-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(state) })
	l, err := listenLocal()
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	ws, err := dev.Open(t.Context(), dev.Config{Root: root, State: state, Out: &output{}, Watch: true,
		Dev: devconfig.Workspace{Port: port, Personas: personas}})
	require.NoError(t, err)
	t.Cleanup(ws.Close)
	return &watched{root: root, ws: ws, hc: newClient(port), port: port}
}

// until gets path of shop as the editor until ok accepts the answer, for at most a minute, and
// returns the body of that answer.
func (w *watched) until(t *testing.T, path string, ok func(status int, body string) bool) string {
	t.Helper()
	var body string
	require.Eventually(t, func() bool {
		status, b, err := w.get(t.Context(), path)
		body = b
		return err == nil && ok(status, b)
	}, time.Minute, 200*time.Millisecond)
	return body
}

// get gets path of shop as the editor.
func (w *watched) get(ctx context.Context, path string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://shop.localhost:%d%s", w.port, path), nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Cookie", "aicoded_dev_persona=editor")
	resp, err := w.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data), err
}

// app returns the status of app, or one with no state when there is none.
func (w *watched) app(ctx context.Context, name string) devapi.App {
	st, _ := w.ws.Status(ctx)
	for _, a := range st.Apps {
		if a.Name == name {
			return a
		}
	}
	return devapi.App{}
}

func edit(t *testing.T, path, pattern, repl string) []byte {
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	re := regexp.MustCompile(pattern)
	require.True(t, re.Match(data), "%s has no %s", path, pattern)
	require.NoError(t, os.WriteFile(path, re.ReplaceAll(data, []byte(repl)), 0o600))
	return data
}

func TestWatchTemplateEdit(t *testing.T) {
	w := watchRPC(t)
	w.until(t, "/status", func(status int, _ string) bool { return status == http.StatusOK })
	edit(t, filepath.Join(w.root, "shop", "pages", "status", "index.html"), `<p id="pong">`, `<p id="note">watched</p><p id="pong">`)
	w.until(t, "/status", func(_ int, body string) bool { return strings.Contains(body, `<p id="note">watched</p>`) })
}

func TestWatchBreakAndFix(t *testing.T) {
	w := watchRPC(t)
	w.until(t, "/invoices/1", func(status int, _ string) bool { return status == http.StatusOK })
	main := filepath.Join(w.root, "shop", "main.go")
	orig := edit(t, main, `(?m)^func main\(\) \{$`, "func main() {\n\tbroken(")

	w.until(t, "/status", func(status int, body string) bool {
		return status == http.StatusServiceUnavailable && strings.Contains(body, "E-CHK-002")
	})
	assert.Equal(t, devapi.Failed, w.app(t.Context(), "shop").State)
	assert.Equal(t, devapi.Running, w.app(t.Context(), "billing").State, "billing keeps running")

	require.NoError(t, os.WriteFile(main, orig, 0o600))
	body := w.until(t, "/invoices/1", func(status int, _ string) bool { return status == http.StatusOK })
	assert.Contains(t, body, `<h1 id="customer">Hotel Lisboa</h1>`, "billing answers shop's calls again")
}

func TestWatchBreakingRPCChange(t *testing.T) {
	w := watchRPC(t)
	w.until(t, "/status", func(status int, _ string) bool { return status == http.StatusOK })
	snapshot := filepath.Join(w.root, "billing", ".aicoded", "rpc.json")
	before, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	edit(t, filepath.Join(w.root, "billing", "rpc", "rpc.go"), `(?s)// Missing refuses.*\z`, "")

	require.Eventually(t, func() bool { return w.app(t.Context(), "billing").State == devapi.Failed }, time.Minute, 200*time.Millisecond)
	billing := w.app(t.Context(), "billing")
	require.NotEmpty(t, billing.Problems)
	assert.Equal(t, "E-RPC-013", billing.Problems[0].Code)
	assert.True(t, strings.HasPrefix(billing.Problems[0].Pos, filepath.Join(w.root, "shop", ".aicoded", "services", "billing.json")+":"), billing.Problems[0].Pos)
	after, err := os.ReadFile(snapshot)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "billing's snapshot is not renumbered")
	assert.Equal(t, devapi.Running, w.app(t.Context(), "shop").State)
}
