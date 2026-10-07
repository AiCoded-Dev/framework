package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/explain"
	"aicoded.dev/framework/cmd/aicoded/internal/gittest"
	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/docs"
)

const secret = "s3cr3t-value"

// page replaces the hello app's main.go: its page logs the secret, which aicoded dev must hide.
const page = `package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/secrets"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		token, err := secrets.Get(r.Context(), "token")
		if err != nil {
			http.Error(w, "no token", http.StatusInternalServerError)
			return
		}
		slog.InfoContext(r.Context(), "page served", "token", token.Reveal())
		fmt.Fprint(w, "hello")
	})
	app.Main(app.Options{Handler: mux})
}
`

func requireGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

// helloWorkspace makes, under a temp home, a workspace that holds the hello app with the main.go
// above, an email section, and a dev.yaml that gives it the secret. It returns the workspace's
// root and port.
func helloWorkspace(t *testing.T) (string, int) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	app := filepath.Join(root, "hello")
	require.NoError(t, os.CopyFS(app, os.DirFS(filepath.Join("..", "dev", "testdata", "hello"))))
	gomod := filepath.Join(app, "go.mod")
	data, err := os.ReadFile(gomod)
	require.NoError(t, err)
	data = regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`).ReplaceAll(data, []byte("replace aicoded.dev/framework => "+frameworkDir(t)))
	require.NoError(t, os.WriteFile(gomod, data, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(app, "main.go"), []byte(page), 0o600))
	yaml := filepath.Join(app, "aicoded.yaml")
	list, err := os.ReadFile(yaml)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(yaml, append(list, "email: {from: hello@example.com, to_domains: [example.com]}\n"...), 0o600))
	port := freePort(t)
	testhome.DevYAML(t, home, fmt.Sprintf(`workspaces:
  %s:
    port: %d
    apps:
      hello:
        settings: {greeting: hi}
        secrets: {token: %s}
`, root, port, secret))
	return root, port
}

// frameworkDir returns this framework checkout.
func frameworkDir(t *testing.T) string {
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	return dir
}

func freePort(t *testing.T) int {
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// logBuffer keeps what a hosted aicoded dev printed.
type logBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// connect serves aicoded mcp for dir, with this framework checkout, over in-memory transports
// until the test ends, and returns a client session.
func connect(t *testing.T, dir string, logs io.Writer) *mcpsdk.ClientSession {
	return connectWith(t, dir, logs, scaffold.Framework{Dir: frameworkDir(t)})
}

func connectWith(t *testing.T, dir string, logs io.Writer, fw scaffold.Framework) *mcpsdk.ClientSession {
	ct, st := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, dir, st, logs, fw) }()
	cs, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cs.Close()
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(time.Minute):
			t.Error("Serve did not return")
		}
		cancel()
	})
	return cs
}

func call(t *testing.T, cs *mcpsdk.ClientSession, tool string, args any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	return res
}

// text returns the text content of res.
func text(res *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// result decodes the structured content of a successful res.
func result[T any](t *testing.T, res *mcpsdk.CallToolResult) T {
	t.Helper()
	require.False(t, res.IsError, text(res))
	data, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

// try calls tool and decodes its result. It reports false instead of failing the test, so it
// may run inside Eventually.
func try[T any](ctx context.Context, cs *mcpsdk.ClientSession, tool string, args any) (T, bool) {
	var v T
	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil || res.IsError {
		return v, false
	}
	data, err := json.Marshal(res.StructuredContent)
	return v, err == nil && json.Unmarshal(data, &v) == nil
}

func get(t *testing.T, port int, url string) (int, string) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	require.NoError(t, err)
	req.Host = strings.TrimSuffix(strings.TrimPrefix(url, "http://"), "/")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestToolsListed(t *testing.T) {
	testhome.Set(t)
	cs := connect(t, t.TempDir(), io.Discard)
	res, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	readOnly := map[string]bool{}
	var openWorld []string
	for _, tool := range res.Tools {
		readOnly[tool.Name] = tool.Annotations.ReadOnlyHint
		if *tool.Annotations.OpenWorldHint {
			openWorld = append(openWorld, tool.Name)
		}
		if !tool.Annotations.ReadOnlyHint {
			assert.False(t, *tool.Annotations.DestructiveHint, tool.Name)
		}
	}
	assert.Equal(t, map[string]bool{
		"app_create": false, "check": false, "preview": false, "mail_receive": false, "publish": false,
		"what_broke": true, "describe": true, "howto": true, "logs": true, "traces": true, "trace": true,
		"mail_list": true, "mail_get": true, "release_status": true,
	}, readOnly)
	assert.ElementsMatch(t, []string{"publish", "release_status"}, openWorld, "only these reach the platform")
}

func TestHowto(t *testing.T) {
	testhome.Set(t)
	cs := connect(t, t.TempDir(), io.Discard)

	topics := result[howtoOut](t, call(t, cs, "howto", map[string]any{})).Topics
	ts, err := docs.Topics()
	require.NoError(t, err)
	entries, err := explain.List()
	require.NoError(t, err)
	require.Len(t, topics, len(ts)+len(entries), "the guides, the tasks and the changelog, then the error codes")
	for i, tp := range ts {
		assert.Equal(t, topic{Name: tp.Path, Title: tp.Title, Summary: tp.Summary}, topics[i])
	}
	assert.Contains(t, topics, topic{Name: "changelog", Title: "Changelog for AI assistants", Summary: "Changes to the API that AI assistants build apps with. Newest first."})
	assert.Contains(t, topics, topic{Name: "E-DEV-001", Title: "dev.yaml is readable by other users"})

	page := result[howtoOut](t, call(t, cs, "howto", map[string]any{"topic": "E-DEV-001"})).Text
	assert.True(t, strings.HasPrefix(page, "# E-DEV-001:"), page)
	guide := result[howtoOut](t, call(t, cs, "howto", map[string]any{"topic": "guides/overview"})).Text
	overview, err := docs.Read("guides/overview")
	require.NoError(t, err)
	assert.Equal(t, overview, guide)
	changelog := result[howtoOut](t, call(t, cs, "howto", map[string]any{"topic": "changelog"})).Text
	assert.True(t, strings.HasPrefix(changelog, "# Changelog for AI assistants"), changelog)

	for _, topic := range []string{"E-NOPE-001", "../README", "README", "guides/nope", "guides/../../go.mod", "errors/E-DEV-001"} {
		res := call(t, cs, "howto", map[string]any{"topic": topic})
		assert.True(t, res.IsError, topic)
		assert.Contains(t, text(res), "E-CLI-001", topic)
	}
}

func TestResources(t *testing.T) {
	testhome.Set(t)
	cs := connect(t, t.TempDir(), io.Discard)
	for uri, start := range map[string]string{
		"aicoded://errors/E-DEV-012":     "# E-DEV-012:",
		"aicoded://changelog":            "# Changelog for AI assistants",
		"aicoded://docs/guides/overview": "# Overview: how an app works\n",
	} {
		res, err := cs.ReadResource(t.Context(), &mcpsdk.ReadResourceParams{URI: uri})
		require.NoError(t, err, uri)
		require.Len(t, res.Contents, 1)
		assert.Equal(t, "text/markdown", res.Contents[0].MIMEType)
		assert.True(t, strings.HasPrefix(res.Contents[0].Text, start), uri)
	}

	list, err := cs.ListResources(t.Context(), nil)
	require.NoError(t, err)
	byURI := map[string]*mcpsdk.Resource{}
	for _, r := range list.Resources {
		byURI[r.URI] = r
	}
	topics, err := docs.Topics()
	require.NoError(t, err)
	for _, tp := range topics {
		uri := "aicoded://docs/" + tp.Path
		if tp.Path == "changelog" {
			uri = "aicoded://changelog"
		}
		require.Contains(t, byURI, uri)
		assert.Equal(t, tp.Path, byURI[uri].Name)
		assert.Equal(t, tp.Title, byURI[uri].Title)
		assert.Equal(t, tp.Summary, byURI[uri].Description)
	}
	assert.Contains(t, byURI, "aicoded://errors/E-DEV-012")
}

func TestInstructionsNameTheOverview(t *testing.T) {
	testhome.Set(t)
	cs := connect(t, t.TempDir(), io.Discard)
	assert.Contains(t, cs.InitializeResult().Instructions, "howto guides/overview")
	assert.Contains(t, cs.InitializeResult().Instructions, "call publish with the app and a summary")
	assert.Contains(t, cs.InitializeResult().Instructions, "Call release_status with that id")
}

func TestAppNamesAreChecked(t *testing.T) {
	home := testhome.Set(t)
	dir := t.TempDir()
	cs := connect(t, dir, io.Discard)
	for _, name := range []string{"../x", "a/b", "A", ""} {
		for tool, args := range map[string]map[string]any{
			"preview":      {"app": name},
			"logs":         {"app": name},
			"traces":       {"app": name},
			"mail_list":    {"app": name},
			"mail_get":     {"app": name, "id": "1"},
			"mail_receive": {"app": name, "from": "a@b.example", "subject": "s", "text": "t"},
			"what_broke":   {"app": name},
			"check":        {"app": name},
			"describe":     {"app": name},
			"app_create":   {"name": name},
			"publish":      {"app": name, "summary": "First"},
		} {
			if name == "" && (tool == "logs" || tool == "traces" || tool == "what_broke" || tool == "check" || tool == "describe") {
				continue // no app means every app
			}
			res := call(t, cs, tool, args)
			assert.True(t, res.IsError, "%s %q", tool, name)
			want := "E-DEV-014"
			if tool == "app_create" {
				want = "E-CLI-002"
			}
			assert.Contains(t, text(res), want, "%s %q", tool, name)
		}
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no folder was created")
	assert.NoDirExists(t, filepath.Join(home, ".local", "state"), "no aicoded dev was started")
}

func TestSchemasAreChecked(t *testing.T) {
	home := testhome.Set(t)
	cs := connect(t, t.TempDir(), io.Discard)
	for tool, args := range map[string]map[string]any{
		"trace":          {"trace_id": "../x"},
		"logs":           {"level": "TRACE"},
		"traces":         {"limit": 201},
		"preview":        {"app": "hello", "path": "/etc"},
		"howto":          {"topic": "E-DEV-001", "file": "/etc/passwd"},
		"publish":        {"app": "hello", "summary": "First", "path": "/etc"},
		"release_status": {"id": "../pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"},
	} {
		res := call(t, cs, tool, args)
		assert.True(t, res.IsError, tool)
		assert.Contains(t, text(res), `validating "arguments"`, tool)
	}
	assert.NoDirExists(t, filepath.Join(home, ".local", "state"), "no aicoded dev was started")
}

func TestAppCreate(t *testing.T) {
	requireGo(t)
	testhome.Set(t)
	t.Setenv("GOPROXY", "off")
	dir := t.TempDir()
	cs := connect(t, dir, io.Discard)
	out := result[appCreateOut](t, call(t, cs, "app_create", map[string]any{"name": "shop"}))
	assert.Equal(t, filepath.Join(dir, "shop"), out.Dir)
	assert.Contains(t, out.Next, "preview")
	assert.Contains(t, out.Next, "AGENTS.md")
	assert.FileExists(t, filepath.Join(dir, "shop", "aicoded.yaml"))
	assert.FileExists(t, filepath.Join(dir, "shop", "AGENTS.md"))
}

func TestAppCreateNeedsAFramework(t *testing.T) {
	testhome.Set(t)
	dir := t.TempDir()
	cs := connectWith(t, dir, io.Discard, scaffold.Framework{})
	res := call(t, cs, "app_create", map[string]any{"name": "shop"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "E-CLI-003")
	assert.NoDirExists(t, filepath.Join(dir, "shop"))
}

func TestPreviewHostsDev(t *testing.T) {
	requireGo(t)
	root, port := helloWorkspace(t)
	var logs logBuffer
	cs := connect(t, root, &logs)

	app := result[devapi.App](t, call(t, cs, "preview", map[string]any{"app": "hello"}))
	assert.Equal(t, devapi.Running, app.State)
	assert.Equal(t, fmt.Sprintf("http://hello.localhost:%d/", port), app.URL)
	code, body := get(t, port, app.URL)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "hello", body)

	var entries []devapi.LogEntry
	require.Eventually(t, func() bool {
		out, ok := try[logsOut](t.Context(), cs, "logs", map[string]any{"app": "hello", "contains": "page served"})
		entries = out.Entries
		return ok && len(entries) > 0
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, "INFO", entries[0].Level)
	assert.Contains(t, logs.String(), "aicoded dev: hello at "+app.URL, "the hosted aicoded dev prints to logs")
}

func TestAttachesToRunningDev(t *testing.T) {
	requireGo(t)
	root, port := helloWorkspace(t)
	w, err := dev.Host(t.Context(), root, io.Discard, dev.Options{})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	var logs logBuffer
	cs := connect(t, filepath.Join(root, "hello"), &logs)

	app := result[devapi.App](t, call(t, cs, "preview", map[string]any{"app": "hello"}))
	assert.Equal(t, devapi.Running, app.State)
	assert.Equal(t, fmt.Sprintf("http://hello.localhost:%d/", port), app.URL)
	st, err := w.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("http://localhost:%d/", port), st.Gateway, "one gateway serves both")
	assert.Empty(t, logs.String(), "aicoded mcp hosted no aicoded dev of its own")
}

func TestReattachesAfterDevStops(t *testing.T) {
	requireGo(t)
	root, _ := helloWorkspace(t)
	w, err := dev.Host(t.Context(), root, io.Discard, dev.Options{})
	require.NoError(t, err)
	closeDev := sync.OnceFunc(w.Close)
	t.Cleanup(closeDev)
	var logs logBuffer
	cs := connect(t, root, &logs)
	result[devapi.App](t, call(t, cs, "preview", map[string]any{"app": "hello"}))

	closeDev()
	assert.True(t, call(t, cs, "logs", map[string]any{}).IsError, "the aicoded dev it used is gone")
	app := result[devapi.App](t, call(t, cs, "preview", map[string]any{"app": "hello"}))
	assert.Equal(t, devapi.Running, app.State)
	assert.Contains(t, logs.String(), "aicoded dev: hello at ", "aicoded mcp now hosts its own")
}

func TestNoSecretsInToolOutput(t *testing.T) {
	requireGo(t)
	root, port := helloWorkspace(t)
	var logs logBuffer
	cs := connect(t, root, &logs)

	app := result[devapi.App](t, call(t, cs, "preview", map[string]any{"app": "hello"}))
	code, _ := get(t, port, app.URL)
	require.Equal(t, http.StatusOK, code)
	var entries []devapi.LogEntry
	require.Eventually(t, func() bool {
		out, ok := try[logsOut](t.Context(), cs, "logs", map[string]any{"contains": "page served"})
		entries = out.Entries
		return ok && len(entries) > 0
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, "[secret token]", entries[0].Attrs["token"])
	state, err := dev.StatePath(root)
	require.NoError(t, err)
	token, err := os.ReadFile(filepath.Join(state, "ui-token"))
	require.NoError(t, err)

	for tool, args := range map[string]map[string]any{
		"preview":    {"app": "hello"},
		"logs":       {},
		"traces":     {},
		"what_broke": {},
		"describe":   {},
		"check":      {},
	} {
		res := call(t, cs, tool, args)
		require.False(t, res.IsError, "%s: %s", tool, text(res))
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.NotContains(t, string(data), secret, tool)
		assert.NotContains(t, string(data), string(token), tool)
	}
	assert.NotContains(t, logs.String(), secret, "nor does the terminal")
	assert.NotContains(t, logs.String(), string(token), "aicoded mcp prints no login link")
	assert.Contains(t, logs.String(), fmt.Sprintf("aicoded dev: the dev UI is at http://localhost:%d/; run aicoded dev in %s to get its login link\n", port, root))
}

func TestMailReceiveAndList(t *testing.T) {
	requireGo(t)
	root, _ := helloWorkspace(t)
	cs := connect(t, root, io.Discard)

	id := result[idOut](t, call(t, cs, "mail_receive", map[string]any{
		"app": "hello", "from": "guest@example.com", "subject": "Towels", "text": "Two more, please.",
	})).ID
	require.NotEmpty(t, id)
	mail := result[mailListOut](t, call(t, cs, "mail_list", map[string]any{"app": "hello"})).Mail
	require.Len(t, mail, 1)
	assert.Equal(t, id, mail[0].ID)
	assert.Equal(t, "Towels", mail[0].Subject)
	got := result[devapi.Mail](t, call(t, cs, "mail_get", map[string]any{"app": "hello", "id": id}))
	assert.Equal(t, "Two more, please.", got.Text)
}

func TestPublishTools(t *testing.T) {
	testhome.Set(t)
	p := platformtest.New(t)
	t.Setenv("AICODED_PLATFORM", p.URL)
	p.SignIn(t)
	dir := gittest.NewApp(t, "demo")
	var logs logBuffer
	cs := connect(t, dir, &logs)
	var answers []*mcpsdk.CallToolResult
	callTool := func(tool string, args any) *mcpsdk.CallToolResult {
		res := call(t, cs, tool, args)
		answers = append(answers, res)
		return res
	}

	out := result[publishOut](t, callTool("publish", map[string]any{"app": "demo", "summary": "First"}))
	assert.Regexp(t, `^pub_[a-z2-7]{26}$`, out.ID)
	assert.Equal(t, publishOut{ID: out.ID, App: "demo", SHA: gittest.Head(t, dir), CreatedApp: true, Next: out.Next}, out)
	assert.Contains(t, out.Next, `call release_status with id "`+out.ID+`"`)
	require.Len(t, p.Uploads(), 1)
	assert.Equal(t, "First", p.Uploads()[0].Summary)
	got := result[platform.Publish](t, callTool("release_status", map[string]any{"id": out.ID}))
	assert.Equal(t, "passed", got.Status)
	assert.Equal(t, out.ID, got.ID)

	res := callTool("publish", map[string]any{"app": "demo", "summary": "Again"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "E-PUB-007: demo already published the commit ")

	gittest.Write(t, dir, map[string]string{"main.go": "package main\n\nfunc main() { x }\n"})
	gittest.Commit(t, dir, "Break it")
	res = callTool("publish", map[string]any{"app": "demo", "summary": "Broken"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "demo: main.go:3: E-CHK-002: ")
	assert.Contains(t, text(res), "\nE-PUB-004: aicoded check --frozen --no-tests found problems in demo, so nothing was sent\n")
	assert.Len(t, p.Uploads(), 1)

	res = callTool("publish", map[string]any{"app": "demo", "summary": ""})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), `validating "arguments"`)
	res = callTool("release_status", map[string]any{"id": "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "E-PUB-013: ")

	for _, res := range answers {
		data, err := json.Marshal(res)
		require.NoError(t, err)
		assert.NotRegexp(t, `aicoded_(at|rt)_`, string(data))
	}
	assert.NotRegexp(t, `aicoded_(at|rt)_`, logs.String())
}
