package dev

import (
	"context"
	"errors"
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

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

func freePort(t *testing.T) int {
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// useDevConfig gives the test a home of its own, and points Run at a dev.yaml that serves the
// hello app on port, with more lines of the workspace, and at a temp state folder.
func useDevConfig(t *testing.T, root string, port int, more ...string) {
	testhome.Set(t)
	cfg, state := filepath.Join(t.TempDir(), "dev.yaml"), t.TempDir()
	origCfg, origState := configPath, stateDir
	configPath = func() (string, error) { return cfg, nil }
	stateDir = func(string) (string, error) { return state, nil }
	t.Cleanup(func() { configPath, stateDir = origCfg, origState })
	require.NoError(t, os.WriteFile(cfg, fmt.Appendf(nil, `workspaces:
  %s:
    port: %d
    apps:
      hello:
        settings: {greeting: hi}
        secrets: {token: abc}
%s`, root, port, strings.Join(more, "")), 0o600))
}

// copyHello copies the hello app to dir under the name app, with its replace directive pointing
// at this checkout, and returns dir.
func copyHello(t *testing.T, dir, app string) string {
	require.NoError(t, os.CopyFS(dir, os.DirFS(helloDir(t))))
	framework, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	rewrite(t, filepath.Join(dir, "go.mod"), `(?m)^replace aicoded.dev/framework => .*$`, "replace aicoded.dev/framework => "+framework)
	rewrite(t, filepath.Join(dir, manifest.FileName), `(?m)^app: hello$`, "app: "+app)
	return dir
}

func rewrite(t *testing.T, path, pattern, repl string) {
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, regexp.MustCompile(pattern).ReplaceAll(data, []byte(repl)), 0o600))
}

// gatewayGet gets / of app through the gateway on port.
func gatewayGet(t *testing.T, port int, app string) string {
	_, body := fetch(t, port, app, "/")
	return body
}

// fetch gets path of app through the gateway on port and returns the status and the body.
func fetch(t *testing.T, port int, app, path string) (int, string) {
	code, body, err := get(t.Context(), port, app, path)
	require.NoError(t, err)
	return code, body
}

// get is fetch for a condition that must not stop the test.
func get(ctx context.Context, port int, app, path string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, path), nil)
	if err != nil {
		return 0, "", err
	}
	req.Host = fmt.Sprintf("%s.localhost:%d", app, port)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

// waitLine waits until out holds text, and fails when aicoded dev stops first.
func waitLine(t *testing.T, out *syncBuffer, done <-chan error, text string) {
	t.Helper()
	deadline := time.After(2 * time.Minute)
	for !strings.Contains(out.String(), text) {
		select {
		case err := <-done:
			t.Fatalf("aicoded dev stopped: %v\n%s", err, out.String())
		case <-deadline:
			t.Fatalf("no %q in:\n%s", text, out.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// stop cancels aicoded dev and waits for it to return.
func stop(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("aicoded dev did not stop")
	}
}

func TestRun(t *testing.T) {
	requireGo(t)
	root, port := copyHello(t, t.TempDir(), "hello"), freePort(t)
	useDevConfig(t, root, port)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(ctx, root, &out, Options{}) }()
	waitLine(t, &out, done, "http://hello.localhost")
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, port, "hello"), "the first default persona")
	stop(t, cancel, done)
}

func TestRunAsPreview(t *testing.T) {
	requireGo(t)
	root, port := copyHello(t, t.TempDir(), "hello"), freePort(t)
	rewrite(t, filepath.Join(root, "main.go"), `(?m)^\tapp\.Main\(`,
		"\tmux.HandleFunc(\"GET /appenv\", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, app.Env(r.Context())) })\n"+
			"\tmux.HandleFunc(\"GET /panic\", func(http.ResponseWriter, *http.Request) { panic(errors.New(\"broke on table 7\")) })\n\tapp.Main(")
	useDevConfig(t, root, port, "    env: preview\n")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- Run(ctx, root, &out, Options{}) }()
	waitLine(t, &out, done, "http://hello.localhost")
	_, body := fetch(t, port, "hello", "/appenv")
	assert.Equal(t, "preview", body)
	code, _ := fetch(t, port, "hello", "/panic")
	assert.Equal(t, http.StatusInternalServerError, code)
	waitLine(t, &out, done, "panic serving request")
	assert.Contains(t, out.String(), `"value":"*errors.errorString"`, "the panic's kind")
	assert.NotContains(t, out.String(), "broke on table 7", "never its text")
	stop(t, cancel, done)
}

func TestRunWorkspaceServesEveryApp(t *testing.T) {
	requireGo(t)
	root, port := t.TempDir(), freePort(t)
	copyHello(t, filepath.Join(root, "hello"), "hello")
	copyHello(t, filepath.Join(root, "more", "hey"), "hey")
	ws := devconfig.Workspace{Port: port, Apps: map[string]devconfig.AppValues{
		"hello": helloValues,
		"hey":   {Settings: map[string]string{"greeting": "hey"}, Secrets: map[string]string{"token": "abcd"}},
	}}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out syncBuffer
	done := make(chan error, 1)
	go func() { done <- RunWorkspace(ctx, root, ws, t.TempDir(), &out) }()
	waitLine(t, &out, done, "http://hey.localhost")
	assert.Equal(t, "hi all-roles (token 3 bytes)", gatewayGet(t, port, "hello"))
	assert.Equal(t, "hey all-roles (token 4 bytes)", gatewayGet(t, port, "hey"))
	stop(t, cancel, done)
}

func TestRunWorkspaceRefusesTwoAppsWithOneName(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"a", "b"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, dir), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, manifest.FileName), []byte("app: shop\n"), 0o600))
	}
	err := RunWorkspace(t.Context(), root, devconfig.Workspace{}, t.TempDir(), io.Discard)
	assert.Equal(t, "E-DEV-010", errs.Code(err))
	require.ErrorContains(t, err, filepath.Join(root, "a")+" and "+filepath.Join(root, "b"))
	data, err := os.ReadFile(filepath.Join(root, "a", manifest.FileName))
	require.NoError(t, err)
	assert.Equal(t, "app: shop\n", string(data), "nothing is generated")
}

// calling returns the permission list of app, which calls the apps callees.
func calling(app string, callees ...string) manifest.Manifest {
	calls := map[string][]string{}
	for _, c := range callees {
		calls[c] = []string{"Ping"}
	}
	return manifest.Manifest{App: app, Services: manifest.Services{Calls: calls}}
}

func TestStartOrder(t *testing.T) {
	for name, c := range map[string]struct {
		ms   []manifest.Manifest
		want []int
	}{
		"folder order":            {[]manifest.Manifest{calling("a"), calling("b"), calling("c")}, []int{0, 1, 2}},
		"callee first":            {[]manifest.Manifest{calling("shop", "billing"), calling("billing")}, []int{1, 0}},
		"a chain":                 {[]manifest.Manifest{calling("a", "b"), calling("b", "c"), calling("c")}, []int{2, 1, 0}},
		"ties in folder order":    {[]manifest.Manifest{calling("a", "c"), calling("b", "c"), calling("c")}, []int{2, 0, 1}},
		"a callee outside":        {[]manifest.Manifest{calling("shop", "ledger"), calling("billing")}, []int{0, 1}},
		"a cycle in folder order": {[]manifest.Manifest{calling("a", "b"), calling("b", "a")}, []int{0, 1}},
		"a longer cycle":          {[]manifest.Manifest{calling("a", "b"), calling("b", "c"), calling("c", "a")}, []int{0, 2, 1}},
		"a caller of a cycle":     {[]manifest.Manifest{calling("a", "c"), calling("b", "c"), calling("c", "b")}, []int{1, 2, 0}},
	} {
		assert.Equal(t, c.want, startOrder(c.ms), name)
	}
}

func TestRunChecksTheMySQLServer(t *testing.T) {
	root := copyHello(t, t.TempDir(), "hello")
	useDevConfig(t, root, freePort(t), "    mysql: root:s3cret@tcp(db.example:3306)/\n")
	err := Run(t.Context(), root, io.Discard, Options{})
	assert.Equal(t, "E-DEV-007", errs.Code(err))
	assert.NotContains(t, err.Error(), "s3cret")
}

// pagesApp returns an app folder whose only page is page.
func pagesApp(t *testing.T, page string) string {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "aicoded.yaml"), []byte("app: hello\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module hello\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "pages"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "pages", "index.html"), []byte(page), 0o600))
	return root
}

func TestRunNeedsAnApp(t *testing.T) {
	testhome.Set(t)
	orig := configPath
	configPath = func() (string, error) { return "", errors.New("dev.yaml is read") }
	t.Cleanup(func() { configPath = orig })
	assert.Equal(t, "E-MAN-006", errs.Code(Run(t.Context(), t.TempDir(), io.Discard, Options{})))
}

func TestGenerateApp(t *testing.T) {
	const page = `<!doctype html><html><head><ssr:access role="editor, admin"/><title>x</title></head></html>`
	m, err := generateApp(pagesApp(t, page), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"admin", "editor"}, m.Roles(), "the access section generate wrote")

	root := pagesApp(t, page)
	require.NoError(t, os.Remove(filepath.Join(root, manifest.FileName)))
	_, err = generateApp(root, nil)
	assert.Equal(t, "E-MAN-006", errs.Code(err))
	generated, err := filepath.Glob(filepath.Join(root, "pages", "*_gen.go"))
	require.NoError(t, err)
	assert.Empty(t, generated, "nothing is generated without a permission list")
}

func TestDefaultPersonas(t *testing.T) {
	shop := manifest.Manifest{App: "shop", Access: map[string]manifest.Access{"/x": {Require: []string{"editor|admin"}}}}
	billing := manifest.Manifest{App: "billing", Services: manifest.Services{Serves: map[string]manifest.Serve{
		"GetInvoice": {Callers: []string{"shop"}, Require: []string{"finance|editor"}},
	}}}
	assert.Equal(t, []devconfig.Persona{
		{Name: "all-roles", Roles: []string{"admin", "editor", "finance"}},
		{Name: "no-roles"},
	}, DefaultPersonas(shop, billing))
}

func TestReadyLine(t *testing.T) {
	assert.Equal(t, "aicoded dev: billing (calls only)", readyLine("billing", true, 8080))
	assert.Equal(t, "aicoded dev: shop at http://shop.localhost:8080/ (switch persona at http://shop.localhost:8080/_aicoded/persona)",
		readyLine("shop", false, 8080))
}

func TestRunStoppedDuringStart(t *testing.T) {
	root := copyHello(t, t.TempDir(), "hello")
	useDevConfig(t, root, freePort(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, Run(ctx, root, io.Discard, Options{}))
}
