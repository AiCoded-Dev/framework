package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/config"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

type testApp struct {
	runner *fakeRunner
	client *http.Client
	key    ed25519.PrivateKey
	sock   string
	stop   func() error
}

// startApp runs an app with opts under a fake runner, which each of setup changes first.
func startApp(t *testing.T, opts Options, setup ...func(*fakeRunner)) *testApp {
	f, key := newFakeRunner(t)
	for _, s := range setup {
		s(f)
	}
	dir := f.serve(t)
	t.Setenv(runnerproto.EnvRunnerDir, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()
	select {
	case <-f.ready:
	case err := <-done:
		t.Fatalf("app stopped before it was ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("app did not report ready")
	}
	sock := filepath.Join(dir, runnerproto.AppSocket)
	return &testApp{runner: f, client: socket.Client(sock), key: key, sock: sock, stop: func() error { cancel(); return <-done }}
}

func mint(t *testing.T, key ed25519.PrivateKey) string {
	now := time.Now()
	token, err := viewer.Sign(key, viewer.Claims{Subject: "alice", Name: "Alice", Audience: "rooms",
		IssuedAt: now.Unix(), Expires: now.Add(time.Minute).Unix()})
	require.NoError(t, err)
	return token
}

func fetch(ctx context.Context, c *http.Client, path, token, traceparent string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://app"+path, nil)
	if err != nil {
		return 0, "", err
	}
	if token != "" {
		req.Header.Set(viewer.Header, token)
	}
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

func (a *testApp) get(t *testing.T, token, traceparent string) (int, string) {
	code, body, err := fetch(t.Context(), a.client, "/", token, traceparent)
	require.NoError(t, err)
	return code, body
}

func TestRunServesViewers(t *testing.T) {
	a := startApp(t, Options{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		greeting, err := config.String(r.Context(), "greeting")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, greeting, " ", auth.Viewer(r.Context()).Name)
	})})

	code, body := a.get(t, mint(t, a.key), "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "hi Alice", body)

	code, _ = a.get(t, "", "")
	assert.Equal(t, http.StatusUnauthorized, code)
	require.NoError(t, a.stop())
}

// A malformed traceparent must not make the handler run twice (defect 5).
func TestRunSharesTheAppSocketWhenTheRunnerAsks(t *testing.T) {
	for share, want := range map[bool]fs.FileMode{false: 0o600, true: 0o666} {
		a := startApp(t, Options{Handler: http.NotFoundHandler()}, func(f *fakeRunner) { f.hello.AppSocketOpen = share })
		st, err := os.Stat(a.sock)
		require.NoError(t, err)
		assert.Equal(t, want, st.Mode().Perm(), "app_socket_open %v", share)
		require.NoError(t, a.stop())
	}
}

func TestRunTraceparent(t *testing.T) {
	var calls atomic.Int32
	a := startApp(t, Options{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) })})

	code, _ := a.get(t, mint(t, a.key), "00-zz-not-a-trace-01")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, int32(1), calls.Load())

	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	code, _ = a.get(t, mint(t, a.key), parent)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, int32(2), calls.Load())

	require.NoError(t, a.stop())
	sc, ok := tracectx.Parse(parent)
	require.True(t, ok)
	assert.True(t, slices.ContainsFunc(a.runner.exported(), func(s *runnerv1.Span) bool {
		return s.GetName() == "HTTP GET" && bytes.Equal(s.GetTraceId(), sc.TraceID[:]) && bytes.Equal(s.GetParentSpanId(), sc.SpanID[:])
	}))
}

func TestRunRecoversPanics(t *testing.T) {
	var calls atomic.Int32
	a := startApp(t, Options{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		if calls.Add(1) == 1 {
			panic("boom")
		}
	})})
	code, body := a.get(t, mint(t, a.key), "")
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.NotContains(t, body, "boom")
	code, _ = a.get(t, mint(t, a.key), "")
	assert.Equal(t, http.StatusOK, code)
	require.NoError(t, a.stop())
}

func TestPanicValueIsLoggedOnlyInDev(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	h := recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("alice@example.com") }))
	for env, ctx := range map[string]context.Context{
		"no runner": context.Background(),
		"preview":   runner.With(context.Background(), &runner.Session{Env: "preview"}),
		"dev":       runner.With(context.Background(), &runner.Session{Env: "dev"}),
	} {
		logs.Reset()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code, env)
		want := `msg="panic serving request" value=string stack=`
		if env == "dev" {
			want = `msg="panic serving request" value=alice@example.com stack=`
		}
		assert.Contains(t, logs.String(), want, env)
	}
}

func TestMainPrintsTheErrorTextOnlyInDev(t *testing.T) {
	failing := Options{Handler: http.NotFoundHandler(), OnStart: func(context.Context) error {
		return fmt.Errorf("seed alice@example.com: %w", fs.ErrExist)
	}}
	for env, want := range map[string]string{
		"preview": "fs.ErrExist\n",
		"dev":     "app: start: seed alice@example.com: file already exists\n",
	} {
		f, _ := newFakeRunner(t)
		f.hello.Env = env
		t.Setenv(runnerproto.EnvRunnerDir, f.serve(t))
		var out bytes.Buffer
		assert.True(t, runMain(t.Context(), failing, &out), env)
		assert.Equal(t, want, out.String(), env)
	}

	t.Setenv(runnerproto.EnvRunnerDir, "")
	var out bytes.Buffer
	assert.True(t, runMain(t.Context(), failing, &out))
	assert.Equal(t, "E-RUN-001\n", out.String(), "with no runner, no text")
}

func TestRunDrainsOnShutdown(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	a := startApp(t, Options{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(entered)
			<-release
		}
		fmt.Fprint(w, "done")
	})})
	token := mint(t, a.key)
	type result struct {
		code int
		body string
		err  error
	}
	slow := make(chan result, 1)
	go func() {
		code, body, err := fetch(context.Background(), a.client, "/slow", token, "")
		slow <- result{code, body, err}
	}()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- a.stop() }()
	require.Eventually(t, func() bool {
		_, _, err := fetch(t.Context(), socket.Client(a.sock), "/", token, "")
		return err != nil
	}, 5*time.Second, 10*time.Millisecond, "the app kept accepting connections after shutdown began")
	close(release)
	assert.Equal(t, result{http.StatusOK, "done", nil}, <-slow)
	require.NoError(t, <-stopped)
}

func TestRunOnStart(t *testing.T) {
	got := make(chan string, 1)
	a := startApp(t, Options{
		Handler: http.NotFoundHandler(),
		OnStart: func(ctx context.Context) error {
			v, err := config.String(ctx, "greeting")
			got <- v
			return err
		},
	})
	assert.Equal(t, "hi", <-got)
	require.NoError(t, a.stop())
}

func TestRunNeedsHandlerOrRPC(t *testing.T) {
	require.Error(t, Run(t.Context(), Options{}))
}

// token mints a token for c the way the runner does for this app.
func (a *testApp) token(t *testing.T, c viewer.Claims) string {
	now := time.Now()
	c.Audience, c.IssuedAt, c.Expires = "rooms", now.Unix(), now.Add(time.Minute).Unix()
	token, err := viewer.Sign(a.key, c)
	require.NoError(t, err)
	return token
}

// call calls Ping with a token for c, as the runner forwards a call.
func (a *testApp) call(t *testing.T, c viewer.Claims) (string, error) {
	req := connect.NewRequest(&runnerv1.ServeRequest{Method: "Ping"})
	req.Header().Set(viewer.Header, a.token(t, c))
	resp, err := runnerv1connect.NewAppServiceClient(a.client, socket.BaseURL, connect.WithGRPC()).Serve(t.Context(), req)
	if err != nil {
		return "", err
	}
	return string(resp.Msg.GetPayload()), nil
}

func ping(apps bool, rules ...string) *rpc.Server {
	return rpc.NewServer(rpc.Method{Name: "Ping", Callers: []string{"shop"}, Require: rules, Apps: apps,
		Call: func(ctx context.Context, _ []byte) ([]byte, error) {
			return []byte("pong from " + Name(ctx) + " to " + rpc.Caller(ctx)), nil
		}})
}

func TestRunServesCallsWithoutPages(t *testing.T) {
	a := startApp(t, Options{RPC: ping(true)})
	out, err := a.call(t, viewer.Claims{Caller: "shop", App: true})
	require.NoError(t, err)
	assert.Equal(t, "pong from rooms to shop", out)

	code, _ := a.get(t, mint(t, a.key), "")
	assert.Equal(t, http.StatusNotFound, code, "an app without pages has none")
	require.NoError(t, a.stop())
}

func TestRunSeparatesCallsFromPages(t *testing.T) {
	a := startApp(t, Options{RPC: ping(false, "*"), Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "page")
	})})
	code, body := a.get(t, mint(t, a.key), "")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "page", body)
	_, err := a.call(t, viewer.Claims{Subject: "alice", Caller: "shop"})
	require.NoError(t, err)

	code, _, err = fetch(t.Context(), a.client, runnerv1connect.AppServiceServeProcedure, mint(t, a.key), "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, code, "a viewer token on the call path")
	code, _, err = fetch(t.Context(), a.client, "/", a.token(t, viewer.Claims{Subject: "alice", Caller: "shop"}), "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, code, "a call token on a page")
	require.NoError(t, a.stop())
}
