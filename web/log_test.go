package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/web/reactive"
)

// marker stands for personal data in an error or a panic.
const marker = "alice@example.com"

// inEnv returns the test session in the runner's environment env.
func inEnv(env string) *runner.Session {
	return &runner.Session{App: testSession.App, Env: env, CSRFKey: testSession.CSRFKey}
}

// logs collects what the default logger writes while a test runs.
type logs struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// line waits for the line logged with msg and returns it.
func (l *logs) line(t *testing.T, msg string) string {
	t.Helper()
	var line string
	require.Eventually(t, func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		for s := range strings.Lines(l.b.String()) {
			if strings.Contains(s, `msg="`+msg+`"`) {
				line = s
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, msg)
	return line
}

func captureLogs(t *testing.T) *logs {
	l := &logs{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(l, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return l
}

func TestErrorsAreLoggedWithoutTheirText(t *testing.T) {
	err := fmt.Errorf("no user %s: %w", marker, fs.ErrNotExist)
	for env, ctx := range map[string]context.Context{
		"no runner": context.Background(),
		"preview":   runner.With(context.Background(), inEnv("preview")),
		"dev":       runner.With(context.Background(), inEnv("dev")),
	} {
		t.Run(env, func(t *testing.T) {
			l := captureLogs(t)
			fail(httptest.NewRecorder(), httptest.NewRequestWithContext(ctx, http.MethodGet, "/x", nil), err)
			callFailure(ctx, "star", err)
			LogError(ctx, "live value refused", err, "route", "rk000001", "var", "name")
			for _, msg := range []string{"request failed", "page call failed", "live value refused"} {
				line := l.line(t, msg)
				if env == "dev" {
					assert.Contains(t, line, marker, msg)
				} else {
					assert.NotContains(t, line, marker, msg)
					assert.Contains(t, line, " err=fs.ErrNotExist\n", msg)
				}
			}
			assert.Contains(t, l.line(t, "live value refused"), " route=rk000001 var=name err=")
		})
	}
}

func TestLiveLogsWithoutText(t *testing.T) {
	openLive := func(t *testing.T, srv *httptest.Server, path string, h http.Header) *websocket.Conn {
		c, _, err := dialLive(t, srv, path, "alice:admin", h)
		require.NoError(t, err)
		readFrame(t, c)
		return c
	}
	sites := map[string]func(t *testing.T, h http.Header){
		"subscribe failed": func(t *testing.T, h http.Header) {
			srv, _ := liveSite(t, func(_, _ *fakeRoute, l *liveRoute) {
				l.subscribe = func(context.Context, *reactive.Conn) error { return errors.New(marker) }
			})
			openLive(t, srv, "/admin/live/__ws", h)
		},
		"panic in live page": func(t *testing.T, h http.Header) {
			srv, _ := liveSite(t, func(_, _ *fakeRoute, l *liveRoute) {
				l.subscribe = func(context.Context, *reactive.Conn) error { panic(marker) }
			})
			openLive(t, srv, "/admin/live/__ws", h)
		},
		"panic in page call": func(t *testing.T, h http.Header) {
			srv, _ := callSite(t, func(context.Context, *Request, string, json.RawMessage) (any, error) { panic(marker) })
			c := openLive(t, srv, "/admin/tool/__ws", h)
			call(t, c, 1, "rk000002", "x", `null`)
			readFrame(t, c)
		},
		"live connection not opened": func(t *testing.T, h http.Header) {
			srv, _ := liveSite(t, nil)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/admin/live/__ws", nil)
			require.NoError(t, err)
			req.Header = h
			req.Header.Set("X-Test-Viewer", "alice:admin")
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", marker)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			_ = resp.Body.Close()
		},
	}
	for msg, run := range sites {
		for _, env := range []string{"preview", "dev"} {
			t.Run(msg+" in "+env, func(t *testing.T) {
				l := captureLogs(t)
				run(t, http.Header{"X-Test-Env": {env}})
				if env == "dev" {
					assert.Contains(t, l.line(t, msg), marker)
				} else {
					assert.NotContains(t, l.line(t, msg), marker)
				}
			})
		}
	}
}
