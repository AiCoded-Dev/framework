// Package e2e runs the test apps under the dev runner and drives them over HTTP and in Chrome.
package e2e

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
)

var personas = []devconfig.Persona{
	{Name: "admin", Roles: []string{"admin", "editor"}},
	{Name: "editor", Roles: []string{"editor"}},
	{Name: "editor2", Roles: []string{"editor"}},
	{Name: "viewer"},
}

type site struct {
	app    string
	port   int
	dir    string
	state  string
	ws     *dev.Workspace
	out    *output
	hc     *http.Client
	onStop func()
}

var (
	once    sync.Once
	running *site
	failed  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	for _, s := range []*site{running, blocks, examples} {
		if s != nil {
			s.stop()
		}
	}
	if rpcApps != nil {
		rpcApps.stop()
	}
	os.Exit(code)
}

// startSite generates, builds and starts the test site once per test binary.
func startSite(t *testing.T) *site {
	t.Helper()
	requireGo(t)
	once.Do(func() { running, failed = launchApp("site", "") })
	require.NoError(t, failed)
	return running
}

func requireGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

// launchApp copies testdata/<name> and runs it under aicoded dev, with the e2e personas and,
// when mysql is not empty, the MySQL server of that admin DSN.
func launchApp(name, mysql string) (*site, error) {
	return launch(filepath.Join("testdata", name), devconfig.Workspace{MySQL: mysql, Personas: personas})
}

// launch copies src, an app or a folder of apps, and runs it under aicoded dev on a free port
// with the workspace settings cfg. Every app must start; the site is the first app with pages.
func launch(src string, cfg devconfig.Workspace) (_ *site, err error) {
	dir, err := copyTree(src)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	state, err := os.MkdirTemp("", "aicoded-e2e-state-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(state)
		}
	}()
	ctx := context.Background()
	out := &output{}
	ws, err := dev.Open(ctx, dev.Config{Root: dir, State: state, Dev: cfg, Out: out})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			ws.Close()
		}
	}()
	st, err := ws.Status(ctx)
	if err != nil {
		return nil, err
	}
	app := ""
	for _, a := range st.Apps {
		if a.State != devapi.Running {
			return nil, fmt.Errorf("%s did not start: %v", a.Name, a.Problems)
		}
		if app == "" && !a.CallsOnly {
			app = a.Name
		}
	}
	if app == "" {
		return nil, fmt.Errorf("no app with pages in %s", src)
	}
	port, err := portOf(st.Gateway)
	if err != nil {
		return nil, err
	}
	return &site{app: app, port: port, dir: dir, state: state, ws: ws, out: out, hc: newClient(port)}, nil
}

// portOf returns the port of the gateway address u.
func portOf(u string) (int, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(parsed.Port())
}

// listenLocal listens on a free port of the loopback interface.
func listenLocal() (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
}

func (s *site) stop() {
	s.hc.CloseIdleConnections()
	s.ws.Close()
	_ = os.RemoveAll(s.dir)
	_ = os.RemoveAll(s.state)
	if s.onStop != nil {
		s.onStop()
	}
}

// copyApp copies testdata/<name>, an app or a folder of apps, as copyTree does.
func copyApp(name string) (string, error) { return copyTree(filepath.Join("testdata", name)) }

// copyTree copies src, an app or a folder of apps, to a temp dir and points the replace
// directive of every go.mod in it at this checkout.
func copyTree(src string) (_ string, err error) {
	dir, err := os.MkdirTemp("", "aicoded-e2e-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		return "", err
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		return "", err
	}
	replace := regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() != "go.mod" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, replace.ReplaceAll(data, []byte("replace aicoded.dev/framework => "+root)), 0o600)
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

func (s *site) url(path string) string {
	return fmt.Sprintf("http://%s.localhost:%d%s", s.app, s.port, path)
}

// client reaches the gateway for any *.localhost name and never follows redirects.
func (s *site) client() *http.Client { return s.hc }

func newClient(port int) *http.Client {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return &http.Client{
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// response is what the site answered to a request whose body do has read and closed.
type response struct {
	StatusCode int
	Header     http.Header
}

func (s *site) do(t *testing.T, persona string, req *http.Request) (*response, string) {
	t.Helper()
	return fetch(t, s.client(), persona, req)
}

// fetch sends req through hc as persona.
func fetch(t *testing.T, hc *http.Client, persona string, req *http.Request) (*response, string) {
	t.Helper()
	req.Header.Set("Cookie", "aicoded_dev_persona="+persona)
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return &response{StatusCode: resp.StatusCode, Header: resp.Header}, string(body)
}

func (s *site) get(t *testing.T, persona, path string) (*response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url(path), nil)
	require.NoError(t, err)
	return s.do(t, persona, req)
}

func (s *site) post(t *testing.T, persona, path string, form url.Values) (*response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url(path), strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", s.url(""))
	return s.do(t, persona, req)
}

// field returns the value of the hidden input name in page.
func field(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(page)
	require.NotNil(t, m, "no %s in the page", name)
	return m[1]
}
