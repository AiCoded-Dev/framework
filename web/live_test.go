package web

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/web/reactive"
)

// liveRoute is a fakeRoute whose state has live values.
type liveRoute struct {
	fakeRoute
	key       string
	subscribe func(ctx context.Context, conn *reactive.Conn) error
	panics    atomic.Bool // Snapshot panics
}

func (l *liveRoute) NewState() RouteState {
	s := &liveState{fakeState: fakeState{route: &l.fakeRoute}, live: l}
	s.SetAssets(l.assets)
	return s
}

type liveState struct {
	fakeState
	live *liveRoute
}

func (s *liveState) RouteKey() string { return s.live.key }

func (s *liveState) Snapshot(context.Context) map[string]reactive.Binding {
	if s.live.panics.Load() {
		panic("boom")
	}
	return map[string]reactive.Binding{s.live.key + ".v": reactive.TextBinding("<b>" + s.live.name + "</b>")}
}

func (s *liveState) Subscribe(ctx context.Context, _ *Request, conn *reactive.Conn) error {
	if s.live.subscribe == nil {
		return nil
	}
	return s.live.subscribe(ctx, conn)
}

func (s *liveState) HandleWrite(ctx context.Context, _ *Request, conn *reactive.Conn, msg reactive.WriteMsg) {
	s.route.log.add(s.route.name + ".write")
	if string(msg.Value) == `"panic"` {
		panic("boom")
	}
	conn.Ack(ctx, msg)
}

// liveSite serves /, /admin (not live, admin only) and /admin/live (live), reachable over TCP.
func liveSite(t *testing.T, mutate func(root, admin *fakeRoute, live *liveRoute)) (*httptest.Server, *callLog) {
	log := &callLog{}
	root := &fakeRoute{name: "root", access: Access{Roles: []string{"*"}}, log: log, layout: true}
	admin := &fakeRoute{name: "admin", access: Access{Roles: []string{"admin"}}, log: log, children: "/live", layout: true}
	live := &liveRoute{fakeRoute: fakeRoute{name: "live", log: log}, key: "rk000001"}
	if mutate != nil {
		mutate(root, admin, live)
	}
	return serveRoutes(t, map[string]Route{"/": root, "/admin": admin, "/admin/live": live}), log
}

// serveRoutes serves routes over TCP. The X-Test-Viewer header names the viewer as subject:roles,
// and X-Test-Env the runner's environment, if any.
func serveRoutes(t *testing.T, routes map[string]Route) *httptest.Server {
	h := New(routes, Options{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, roles, _ := strings.Cut(r.Header.Get("X-Test-Viewer"), ":")
		ctx := withViewer(r.Context(), subject, strings.Split(roles, ",")...)
		if env := r.Header.Get("X-Test-Env"); env != "" {
			ctx = runner.With(ctx, inEnv(env))
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialLive opens a live connection as viewer and returns the handshake's status code.
func dialLive(t *testing.T, srv *httptest.Server, path, viewer string, header http.Header) (*websocket.Conn, int, error) {
	if header == nil {
		header = http.Header{}
	}
	header.Set("X-Test-Viewer", viewer)
	c, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http")+path, &websocket.DialOptions{HTTPHeader: header})
	if c != nil {
		t.Cleanup(func() { _ = c.CloseNow() })
	}
	if resp == nil {
		return c, 0, err
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return c, resp.StatusCode, err
}

func readFrame(t *testing.T, c *websocket.Conn) map[string]any {
	_, b, err := c.Read(t.Context())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func TestLiveSendsInitAfterChecks(t *testing.T) {
	srv, log := liveSite(t, nil)
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	init := readFrame(t, c)
	assert.Equal(t, "init", init["t"])
	assert.Equal(t, map[string]any{"rk000001.v": map[string]any{"kind": "text", "value": "<b>live</b>"}}, init["bindings"])
	assert.Equal(t, []string{"root.guard", "admin.guard", "live.guard", "root.data", "admin.data", "live.data"}, log.all())
}

// The hooks of a live connection see the page's path, as they do when the page loads.
func TestLiveHooksSeePagePath(t *testing.T) {
	srv, log := liveSite(t, func(root, _ *fakeRoute, live *liveRoute) { root.paths, live.paths = true, true })
	c, _, err := dialLive(t, srv, "/admin/live/__ws?tab=2", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	for _, want := range []string{"root.guard /admin/live /admin/live?tab=2", "live.guard /admin/live /admin/live?tab=2",
		"root.data /admin/live /admin/live?tab=2", "live.data /admin/live /admin/live?tab=2"} {
		assert.Contains(t, log.all(), want)
	}
	_, status, _ := dialLive(t, srv, "/__ws", "alice:admin", nil)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Contains(t, log.all(), "root.guard / /")
}

// Defect 2: the live connection passes the checks of every route on the path, live or not.
func TestLiveUnderDeniedParentIsRefused(t *testing.T) {
	srv, log := liveSite(t, nil)
	_, status, err := dialLive(t, srv, "/admin/live/__ws", "bob:editor", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status)
	_, status, err = dialLive(t, srv, "/admin/__ws", "bob:editor", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status, "a parent's address is refused like its page")
	assert.Empty(t, log.all())

	srv, _ = liveSite(t, func(_, admin *fakeRoute, _ *liveRoute) { admin.guard = NotFound() })
	_, status, err = dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, status)

	srv, log = liveSite(t, func(_, admin *fakeRoute, _ *liveRoute) { admin.data = Forbidden() })
	_, status, err = dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, []string{"root.guard", "admin.guard", "live.guard", "root.data", "admin.data"}, log.all())
}

func TestLiveRefusesOtherOrigins(t *testing.T) {
	srv, log := liveSite(t, nil)
	for _, origin := range []string{"https://evil.example", "null"} {
		_, status, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", http.Header{"Origin": {origin}})
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, status, origin)
	}
	assert.Empty(t, log.all())
	_, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", http.Header{"Origin": {srv.URL}})
	assert.NoError(t, err, "same origin")
}

func TestLiveNotFound(t *testing.T) {
	srv, _ := liveSite(t, nil)
	for _, p := range []string{"/__ws", "/admin/__ws", "/nope/__ws"} {
		_, status, err := dialLive(t, srv, p, "alice:admin", nil)
		require.Error(t, err)
		assert.Equal(t, http.StatusNotFound, status, p)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/admin/live/__ws", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Viewer", "alice:admin")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "a plain GET is not a live connection")
}

// A panic in app code before the upgrade fails the handshake and holds no place.
func TestLiveSnapshotPanicFailsTheHandshake(t *testing.T) {
	var live *liveRoute
	srv, _ := liveSite(t, func(_, _ *fakeRoute, l *liveRoute) { live = l })
	live.panics.Store(true)
	for range maxConnsPerViewer {
		_, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
		require.Error(t, err)
	}
	live.panics.Store(false)
	_, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	assert.NoError(t, err)
}

func TestLiveStaysOpenAfterSubscribeReturns(t *testing.T) {
	srv, _ := liveSite(t, nil)
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(`{"t":"write","routeKey":"rk000001","var":"v","value":"x"}`)))
	assert.Equal(t, map[string]any{"t": "ack", "routeKey": "rk000001", "var": "v"}, readFrame(t, c))

	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(`{"t":"write","routeKey":"other","var":"v","value":1}`)))
	assert.Equal(t, "unknown_route", readFrame(t, c)["code"])
}

func TestLivePushesPatches(t *testing.T) {
	srv, _ := liveSite(t, func(_, _ *fakeRoute, live *liveRoute) {
		live.subscribe = func(ctx context.Context, conn *reactive.Conn) error {
			conn.Enqueue("rk000001.v", reactive.TextBinding("2"))
			<-ctx.Done()
			return nil
		}
	})
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	assert.Equal(t, map[string]any{"t": "patch", "key": "rk000001.v", "kind": "text", "value": "2"}, readFrame(t, c))
}

func TestLiveClosesOnFailures(t *testing.T) {
	cases := map[string]func(live *liveRoute){
		"subscribe error": func(l *liveRoute) {
			l.subscribe = func(context.Context, *reactive.Conn) error { return assert.AnError }
		},
		"subscribe panic": func(l *liveRoute) {
			l.subscribe = func(context.Context, *reactive.Conn) error { panic("boom") }
		},
	}
	for name, mutate := range cases {
		srv, _ := liveSite(t, func(_, _ *fakeRoute, live *liveRoute) { mutate(live) })
		c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
		require.NoError(t, err, name)
		readFrame(t, c)
		_, _, err = c.Read(t.Context())
		assert.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err), name)
	}

	srv, _ := liveSite(t, nil)
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(`{"t":"write","routeKey":"rk000001","var":"v","value":"panic"}`)))
	_, _, err = c.Read(t.Context())
	assert.Equal(t, websocket.StatusInternalError, websocket.CloseStatus(err), "write panic")
}

func TestLiveConnectionsAreLimited(t *testing.T) {
	srv, _ := liveSite(t, nil)
	var first *websocket.Conn
	for range maxConnsPerViewer {
		c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
		require.NoError(t, err)
		first = cmp.Or(first, c)
	}
	_, status, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusTooManyRequests, status)
	_, _, err = dialLive(t, srv, "/admin/live/__ws", "carol:admin", nil)
	require.NoError(t, err, "the limit is per viewer")

	require.NoError(t, first.Close(websocket.StatusNormalClosure, ""))
	assert.Eventually(t, func() bool {
		_, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond, "a closed page frees its place")
}

func TestLiveConnectionsExpire(t *testing.T) {
	old := maxConnAge
	maxConnAge = 200 * time.Millisecond
	t.Cleanup(func() { maxConnAge = old })
	srv, _ := liveSite(t, nil)
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	_, _, err = c.Read(t.Context())
	assert.Equal(t, reactive.StatusReauth, websocket.CloseStatus(err))
}

// gateSite serves / (a layout), /admin (a live gate, admin only) and /admin/live (a live page
// below it).
func gateSite(t *testing.T, mutate func(gate *liveRoute)) (*httptest.Server, *callLog) {
	log := &callLog{}
	root := &fakeRoute{name: "root", access: Access{Roles: []string{"*"}}, log: log, layout: true}
	gate := &liveRoute{fakeRoute: fakeRoute{name: "gate", access: Access{Roles: []string{"admin"}}, log: log}, key: "rk00000a"}
	live := &liveRoute{fakeRoute: fakeRoute{name: "live", log: log}, key: "rk000001"}
	if mutate != nil {
		mutate(gate)
	}
	return serveRoutes(t, map[string]Route{"/": root, "/admin": gate, "/admin/live": live}), log
}

// A gate's live values and Subscribe belong to its own page. A live connection below it gets
// only the page's own values, after the gate's rules and Guard pass.
func TestLiveBelowGate(t *testing.T) {
	srv, log := gateSite(t, func(gate *liveRoute) {
		gate.subscribe = func(context.Context, *reactive.Conn) error { gate.log.add("gate.subscribe"); return nil }
	})

	_, status, err := dialLive(t, srv, "/admin/live/__ws", "bob:editor", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status, "the gate's rule covers the page below")

	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	init := readFrame(t, c)
	assert.Equal(t, map[string]any{"rk000001.v": map[string]any{"kind": "text", "value": "<b>live</b>"}}, init["bindings"])
	assert.Equal(t, []string{"root.guard", "gate.guard", "live.guard", "root.data", "live.data"}, log.all())

	c, _, err = dialLive(t, srv, "/admin/__ws", "alice:admin", nil)
	require.NoError(t, err, "the gate's own page has its own live connection")
	assert.Equal(t, map[string]any{"rk00000a.v": map[string]any{"kind": "text", "value": "<b>gate</b>"}}, readFrame(t, c)["bindings"])
}

// The gate's Guard, not only its rule, refuses the live connection of a page below it.
func TestLiveBelowGateRunsItsGuard(t *testing.T) {
	srv, log := gateSite(t, func(gate *liveRoute) { gate.guard = NotFound() })
	_, status, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, []string{"root.guard", "gate.guard"}, log.all())
}

// A page below a gate cannot write the gate's live values, and the gate's Subscribe never runs
// there.
func TestLiveWriteToGateBelowIsRefused(t *testing.T) {
	subscribed := make(chan struct{}, 1)
	srv, log := gateSite(t, func(gate *liveRoute) {
		gate.subscribe = func(context.Context, *reactive.Conn) error { subscribed <- struct{}{}; return nil }
	})
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(`{"t":"write","routeKey":"rk00000a","var":"v","value":"x"}`)))
	assert.Equal(t, map[string]any{"t": "err", "routeKey": "rk00000a", "var": "v", "msg": "unknown route", "code": reactive.CodeUnknownRoute},
		readFrame(t, c))
	assert.NotContains(t, log.all(), "gate.write")
	assert.Never(t, func() bool { return len(subscribed) > 0 }, 100*time.Millisecond, 10*time.Millisecond,
		"the gate's Subscribe never runs on the page below")
}
