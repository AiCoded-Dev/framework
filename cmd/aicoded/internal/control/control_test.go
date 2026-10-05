package control

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/errs"
)

var (
	when   = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	status = devapi.Status{Root: "/work", Gateway: "http://localhost:8080/", Apps: []devapi.App{
		{Name: "shop", Dir: "/work/shop", State: devapi.Running, URL: "http://shop.localhost:8080/", Since: when},
		{Name: "billing", Dir: "/work/billing", State: devapi.Failed, Since: when,
			Problems: []problem.Problem{{App: "billing", Code: "E-CHK-002", Pos: "main.go:3", Message: "undefined: x", Fix: "fix the Go code at this line"}}},
	}}
	entries = []devapi.LogEntry{{Time: when, App: "shop", Source: "app", Level: "INFO", Message: "served", Attrs: map[string]any{"path": "/"}}}
	span    = devapi.Span{TraceID: strings.Repeat("a", 32), SpanID: strings.Repeat("b", 16), App: "shop", Name: "GET /", Start: when, DurationMS: 1.5}
	traces  = []devapi.TraceSummary{{TraceID: span.TraceID, Root: "GET /", Apps: []string{"shop"}, Spans: 1, Start: when, DurationMS: 1.5}}
	sent    = devapi.MailSummary{ID: "sent-1", Folder: "sent", From: "ops@acme.example", To: []string{"a@acme.example"}, Subject: "Hi", Time: when}
	broken  = devapi.Broken{Problems: status.Apps[1].Problems, Errors: entries, Failed: []devapi.Span{span}}
)

// fake answers every call with the values above and keeps the arguments it got.
type fake struct {
	mu   sync.Mutex
	args [][]any
	err  error
}

func (f *fake) got(args ...any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.args = append(f.args, args)
	return f.err
}

func (f *fake) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fake) calls() [][]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.args
}

func (f *fake) Status(context.Context) (devapi.Status, error) { return status, f.got("status") }

func (f *fake) Restart(_ context.Context, app string) (devapi.App, error) {
	return status.Apps[0], f.got("restart", app)
}

func (f *fake) Logs(_ context.Context, q devapi.LogQuery) ([]devapi.LogEntry, error) {
	return entries, f.got("logs", q)
}

func (f *fake) Traces(_ context.Context, q devapi.TraceQuery) ([]devapi.TraceSummary, error) {
	return traces, f.got("traces", q)
}

func (f *fake) Trace(_ context.Context, id string) (devapi.Trace, error) {
	return devapi.Trace{TraceID: id, Spans: []devapi.Span{span}}, f.got("trace", id)
}

func (f *fake) Mail(_ context.Context, app string) ([]devapi.MailSummary, error) {
	return []devapi.MailSummary{sent}, f.got("mail", app)
}

func (f *fake) MailGet(_ context.Context, app, id string) (devapi.Mail, error) {
	return devapi.Mail{MailSummary: sent, Text: "Hello"}, f.got("mail/get", app, id)
}

func (f *fake) MailReceive(_ context.Context, app string, m devapi.InboundMail) (string, error) {
	return "inbox-1", f.got("mail/receive", app, m)
}

func (f *fake) WhatBroke(_ context.Context, app string) (devapi.Broken, error) {
	return broken, f.got("what-broke", app)
}

// tempSocket returns a path for a control socket in a new short temp folder.
func tempSocket(t *testing.T) string {
	dir, err := os.MkdirTemp("", "aicd-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return SocketPath(dir)
}

// serve serves b on a control socket at path until the test ends.
func serve(t *testing.T, path string, b devapi.Backend) {
	l, err := Listen(t.Context(), path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, l, b) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
}

// raw sends a request to the socket at path as it is, and returns the status, the protocol
// header and the body of the answer.
func raw(t *testing.T, path string, req *http.Request) (int, string, string) {
	hc := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}}}
	defer hc.CloseIdleConnections()
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header.Get(header), string(body)
}

func request(t *testing.T, method, path, body string) *http.Request {
	req, err := http.NewRequestWithContext(t.Context(), method, "http://aicoded"+path, strings.NewReader(body))
	require.NoError(t, err)
	return req
}

func TestRoundTrip(t *testing.T) {
	f, path := &fake{}, tempSocket(t)
	serve(t, path, f)
	c, ctx := Dial(path), t.Context()

	st, err := c.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, status, st)
	app, err := c.Restart(ctx, "shop")
	require.NoError(t, err)
	assert.Equal(t, status.Apps[0], app)
	logs, err := c.Logs(ctx, devapi.LogQuery{App: "shop", Level: "WARN", TraceID: span.TraceID, Contains: "x", Limit: 7})
	require.NoError(t, err)
	assert.Equal(t, entries, logs)
	ts, err := c.Traces(ctx, devapi.TraceQuery{App: "shop", ErrorsOnly: true, Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, traces, ts)
	tr, err := c.Trace(ctx, span.TraceID)
	require.NoError(t, err)
	assert.Equal(t, devapi.Trace{TraceID: span.TraceID, Spans: []devapi.Span{span}}, tr)
	mail, err := c.Mail(ctx, "shop")
	require.NoError(t, err)
	assert.Equal(t, []devapi.MailSummary{sent}, mail)
	m, err := c.MailGet(ctx, "shop", "sent-1")
	require.NoError(t, err)
	assert.Equal(t, devapi.Mail{MailSummary: sent, Text: "Hello"}, m)
	id, err := c.MailReceive(ctx, "shop", devapi.InboundMail{From: "a@b.example", Subject: "Order", Text: "Two towels"})
	require.NoError(t, err)
	assert.Equal(t, "inbox-1", id)
	b, err := c.WhatBroke(ctx, "billing")
	require.NoError(t, err)
	assert.Equal(t, broken, b)
	require.NoError(t, c.Ping(ctx))

	assert.Equal(t, [][]any{
		{"status"},
		{"restart", "shop"},
		{"logs", devapi.LogQuery{App: "shop", Level: "WARN", TraceID: span.TraceID, Contains: "x", Limit: 7}},
		{"traces", devapi.TraceQuery{App: "shop", ErrorsOnly: true, Limit: 3}},
		{"trace", span.TraceID},
		{"mail", "shop"},
		{"mail/get", "shop", "sent-1"},
		{"mail/receive", "shop", devapi.InboundMail{From: "a@b.example", Subject: "Order", Text: "Two towels"}},
		{"what-broke", "billing"},
		{"status"},
	}, f.calls())
}

func TestErrorsCrossTheSocket(t *testing.T) {
	f, path := &fake{}, tempSocket(t)
	serve(t, path, f)
	c := Dial(path)

	coded := errs.At("shop/aicoded.yaml:1", "E-DEV-014", `no app named "shap" in this workspace`, "name an app from the app: line of an aicoded.yaml in this workspace")
	f.fail(coded)
	_, err := c.Restart(t.Context(), "shap")
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, coded, e)

	f.fail(errors.New("no trace " + span.TraceID))
	_, err = c.Trace(t.Context(), span.TraceID)
	require.EqualError(t, err, "no trace "+span.TraceID)
	assert.Empty(t, errs.Code(err))
}

func TestServerChecksTheProtocol(t *testing.T) {
	f, path := &fake{}, tempSocket(t)
	serve(t, path, f)
	for _, value := range []string{"", "2"} {
		req := request(t, http.MethodGet, "/v1/status", "")
		if value != "" {
			req.Header.Set(header, value)
		}
		code, proto, body := raw(t, path, req)
		assert.Equal(t, http.StatusBadRequest, code, value)
		assert.Equal(t, Protocol, proto, "the answer names the server's protocol")
		assert.Contains(t, body, `"code":"E-DEV-013"`)
	}
	assert.Empty(t, f.calls(), "the backend never ran")
}

func TestClientChecksTheProtocol(t *testing.T) {
	for _, value := range []string{"", "2"} {
		path := tempSocket(t)
		var lc net.ListenConfig
		l, err := lc.Listen(t.Context(), "unix", path)
		require.NoError(t, err)
		srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if value != "" {
				w.Header().Set(header, value)
			}
			_, _ = io.WriteString(w, `{"root":"/work","gateway":"http://localhost:8080/"}`)
		})}
		go func() { _ = srv.Serve(l) }()
		t.Cleanup(func() { _ = srv.Close() })

		err = Dial(path).Ping(t.Context())
		assert.Equal(t, "E-DEV-013", errs.Code(err), value)
		_, err = Listen(t.Context(), path)
		assert.Equal(t, "E-DEV-013", errs.Code(err), "a running aicoded dev of another version keeps its socket")
		assert.FileExists(t, path)
	}
}

func TestListenMode(t *testing.T) {
	path := tempSocket(t)
	l, err := Listen(t.Context(), path)
	require.NoError(t, err)
	defer l.Close()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSocket, info.Mode().Type())
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestListenRefusesALiveSocket(t *testing.T) {
	f, path := &fake{}, tempSocket(t)
	serve(t, path, f)
	_, err := Listen(t.Context(), path)
	assert.Equal(t, "E-DEV-012", errs.Code(err))
	require.ErrorContains(t, err, "aicoded dev already runs for /work at http://localhost:8080/")
	require.ErrorContains(t, err, "use the running one at http://localhost:8080/, or stop it first")
	require.NoError(t, Dial(path).Ping(t.Context()), "the running one still answers")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Listen(ctx, path)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, Dial(path).Ping(t.Context()), "a Listen cut short removes nothing")
}

func TestListenKeepsASocketThatDoesNotAnswer(t *testing.T) {
	path := tempSocket(t)
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	var mu sync.Mutex
	var conns []net.Conn
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	_, err = Listen(t.Context(), path)
	assert.Equal(t, "E-DEV-012", errs.Code(err))
	require.ErrorContains(t, err, "did not answer")
	assert.FileExists(t, path)
}

func TestListenReplacesAStaleSocket(t *testing.T) {
	path := tempSocket(t)
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "unix", path)
	require.NoError(t, err)
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, l.Close())
	require.FileExists(t, path, "a crashed aicoded dev leaves its socket behind")

	f := &fake{}
	serve(t, path, f)
	require.NoError(t, Dial(path).Ping(t.Context()))
}

func TestListenRefusesAnotherFile(t *testing.T) {
	path := tempSocket(t)
	require.NoError(t, os.WriteFile(path, []byte("notes"), 0o600))
	_, err := Listen(t.Context(), path)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-DEV-016", e.Code)
	assert.Equal(t, path+" is not a socket", e.Msg)
	assert.Equal(t, "remove "+path+" if nothing else uses it, then start aicoded dev again", e.Fix)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "notes", string(data), "the file is left in place")
}

func TestListenRefusesALongPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("d", 100))
	require.NoError(t, os.Mkdir(dir, 0o700))
	path := SocketPath(dir)
	_, err := Listen(t.Context(), path)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-DEV-017", e.Code)
	assert.Equal(t, "the control socket path "+path+" is too long for a Unix socket", e.Msg)
	assert.Equal(t, "use a home folder with a shorter path", e.Fix)
	assert.NoFileExists(t, path)
}

func TestServerRefusesBadBodies(t *testing.T) {
	f, path := &fake{}, tempSocket(t)
	serve(t, path, f)
	for name, c := range map[string]struct {
		body string
		code int
	}{
		"oversized":     {`{"App":"` + strings.Repeat("a", maxBody) + `"}`, http.StatusRequestEntityTooLarge},
		"unknown field": {`{"App":"shop","Path":"/etc"}`, http.StatusBadRequest},
		"trailing data": {`{"App":"shop"} {}`, http.StatusBadRequest},
		"not JSON":      {`shop`, http.StatusBadRequest},
	} {
		req := request(t, http.MethodPost, "/v1/logs", c.body)
		req.Header.Set(header, Protocol)
		code, _, _ := raw(t, path, req)
		assert.Equal(t, c.code, code, name)
	}
	assert.Empty(t, f.calls(), "the backend never ran")
}
