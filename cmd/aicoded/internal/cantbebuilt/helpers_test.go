// Package cantbebuilt shows what cannot be built. Each test makes one attempt at a mistake
// behind a well-known kind of leak from AI-built apps, and shows the code that refuses it; a
// control without the mistake shows that nothing else refuses it. docs/cant-be-built.md explains
// them.
package cantbebuilt

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/testmysql"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/viewer"
)

// checkout returns this framework checkout, which the test apps replace the framework with.
func checkout(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	return dir
}

// copyApp copies the app testdata/<name> into a temp folder, with the replace line of its go.mod
// pointing at this checkout, and returns the folder.
func copyApp(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", name))))
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	require.NoError(t, err)
	replace := regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`)
	require.Regexp(t, replace, string(data))
	data = replace.ReplaceAll(data, []byte("replace aicoded.dev/framework => "+checkout(t)))
	require.NoError(t, os.WriteFile(gomod, data, 0o600))
	return dir
}

// requireGo skips the test when the go command, which builds the app, is not in PATH.
func requireGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

// refusal is what the builder sees of a problem: its code, where it is and what it says.
type refusal struct{ Code, Pos, Message string }

// runCheck runs aicoded check --frozen on the app in dir, which writes nothing and requires the
// generated files to be current, and returns the problems it found.
func runCheck(t *testing.T, dir string) []refusal {
	t.Helper()
	requireGo(t)
	r, err := check.Run(t.Context(), dir, check.Options{Frozen: true, FrameworkDir: checkout(t)})
	require.NoError(t, err)
	require.Len(t, r.Apps, 1)
	return refusals(r.Apps[0].Problems)
}

func refusals(ps []problem.Problem) []refusal {
	var out []refusal
	for _, p := range ps {
		out = append(out, refusal{p.Code, p.Pos, p.Message})
	}
	return out
}

// runningApp is a test app run by the local runner with no company login in front of it: the
// test sends its requests straight to the app's socket.
type runningApp struct {
	*dev.Instance
	signer *dev.Signer
	client *http.Client
	// out is what the app printed.
	out *syncBuffer
}

// startApp builds the app testdata/<name> and runs it under the local runner, as aicoded dev
// does, until the test ends. An app that declares sqldb gets a name of its own and a database
// on the test MySQL, dropped when the test ends; with no test MySQL the test is skipped.
func startApp(t *testing.T, name string) *runningApp {
	t.Helper()
	requireGo(t)
	dir := copyApp(t, name)
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	require.NoError(t, err)
	svc := dev.Services{StateDir: t.TempDir()}
	if m.SQLDB() {
		m.App += "-" + strings.ToLower(rand.Text()[:6])
		svc.MySQL = testDatabase(t, m.App)
	}
	signer, err := dev.NewSigner()
	require.NoError(t, err)
	a := &runningApp{signer: signer, out: &syncBuffer{}}
	a.Instance, err = dev.Start(t.Context(), dir, m, devconfig.AppValues{}, svc, signer, a.out)
	require.NoError(t, err)
	t.Cleanup(a.Stop)
	a.client = &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", a.AppSocket())
		}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	t.Cleanup(a.client.CloseIdleConnections)
	return a
}

// testDatabase returns the admin of the test MySQL for the local runner, and drops the database
// and the user the runner makes for app when the test ends.
func testDatabase(t *testing.T, app string) *dev.MySQLAdmin {
	t.Helper()
	admin := testmysql.Admin(t)
	a, err := dev.NewMySQLAdmin(testmysql.DSN(t))
	require.NoError(t, err)
	name := "ac-dev-" + app
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP USER IF EXISTS '"+name+"'@'%'")
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`")
	})
	return a
}

// token returns a token for persona p to use the app, signed now by the key the app trusts: the
// one the company login signs with.
func (a *runningApp) token(t *testing.T, p devconfig.Persona) string {
	t.Helper()
	token, err := a.signer.Mint(p, a.Name, time.Now())
	require.NoError(t, err)
	return token
}

// request returns a request for path with token in the header that carries it, or with no token
// when it is empty.
func request(t *testing.T, method, path string, body io.Reader, token string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, "http://app.test"+path, body)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set(viewer.Header, token)
	}
	return req
}

// send sends req straight to the app's socket over HTTP/1.1, as a browser would, without
// following a redirect, and returns the status and the body. It closes an upgraded connection
// at once.
func (a *runningApp) send(t *testing.T, req *http.Request) (int, string) {
	t.Helper()
	resp, err := a.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusSwitchingProtocols {
		return resp.StatusCode, ""
	}
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// syncBuffer is a buffer that the app's output can be written to while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
