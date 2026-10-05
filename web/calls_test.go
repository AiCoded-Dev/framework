package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type callFunc func(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error)

// callRoute is a page with calls and no live values.
type callRoute struct {
	fakeRoute
	handle callFunc
}

func (c *callRoute) NewState() RouteState {
	s := &callState{fakeState: fakeState{route: &c.fakeRoute}, call: c}
	s.SetAssets(c.assets)
	return s
}

type callState struct {
	fakeState
	call *callRoute
}

func (s *callState) RouteKey() string { return "rk000002" }

func (s *callState) Call(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error) {
	return s.call.handle(ctx, r, name, args)
}

// callSite serves /, /admin (admin only, guarded), /admin/tool (a page with calls) and
// /admin/plain (a page with neither calls nor live values).
func callSite(t *testing.T, handle callFunc) (*httptest.Server, *atomic.Bool) {
	log := &callLog{}
	denied := &atomic.Bool{}
	root := &fakeRoute{name: "root", access: Access{Roles: []string{"*"}}, log: log, layout: true}
	admin := &fakeRoute{name: "admin", access: Access{Roles: []string{"admin"}}, log: log, children: "/tool", layout: true,
		guardFn: func() error {
			if denied.Load() {
				return NotFound()
			}
			return nil
		}}
	tool := &callRoute{fakeRoute: fakeRoute{name: "tool", log: log}, handle: handle}
	plain := &fakeRoute{name: "plain", log: log}
	return serveRoutes(t, map[string]Route{"/": root, "/admin": admin, "/admin/tool": tool, "/admin/plain": plain}), denied
}

func openTool(t *testing.T, srv *httptest.Server) *websocket.Conn {
	c, _, err := dialLive(t, srv, "/admin/tool/__ws", "alice:admin", nil)
	require.NoError(t, err)
	assert.Equal(t, "init", readFrame(t, c)["t"], "a page with calls opens a live connection even without live values")
	return c
}

func call(t *testing.T, c *websocket.Conn, id int, routeKey, name, args string) {
	msg := fmt.Sprintf(`{"t":"call","id":%d,"routeKey":%q,"name":%q,"args":%s}`, id, routeKey, name, args)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(msg)))
}

type panicJSON struct{}

func (panicJSON) MarshalJSON() ([]byte, error) { panic("boom") }

func echo(_ context.Context, _ *Request, name string, args json.RawMessage) (any, error) {
	var v any
	if err := DecodeArgs(args, &v); err != nil {
		return nil, err
	}
	return map[string]any{"name": name, "args": v}, nil
}

func TestCallReturnsResult(t *testing.T) {
	srv, _ := callSite(t, echo)
	c := openTool(t, srv)
	call(t, c, 1, "rk000002", "star", `{"on":true}`)
	assert.Equal(t, map[string]any{"t": "result", "id": float64(1), "value": map[string]any{"name": "star", "args": map[string]any{"on": true}}},
		readFrame(t, c))
}

// Access rules and guards run again before every call.
func TestCallRechecksGuards(t *testing.T) {
	var runs atomic.Int32
	srv, denied := callSite(t, func(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error) {
		runs.Add(1)
		return "ok", nil
	})
	c := openTool(t, srv)
	call(t, c, 1, "rk000002", "x", `null`)
	assert.Equal(t, "result", readFrame(t, c)["t"])

	denied.Store(true)
	call(t, c, 2, "rk000002", "x", `null`)
	assert.Equal(t, map[string]any{"t": "fail", "id": float64(2), "status": float64(404), "msg": "This page does not exist."}, readFrame(t, c))
	assert.Equal(t, int32(1), runs.Load(), "the call never ran")
}

func TestCallFailures(t *testing.T) {
	old := callTimeout
	callTimeout = 100 * time.Millisecond
	t.Cleanup(func() { callTimeout = old })
	srv, _ := callSite(t, func(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error) {
		switch name {
		case "taken":
			return nil, Error(http.StatusConflict, "That name is taken.")
		case "status":
			return nil, Error(http.StatusOK, "Not an error.")
		case "leak":
			return nil, errors.New("db password is hunter2")
		case "redirect":
			return nil, Redirect("/x")
		case "panic":
			panic("boom")
		case "unencodable":
			return func() {}, nil
		case "encoder panic":
			return panicJSON{}, nil
		case "slow":
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return "ok", nil
	})
	c := openTool(t, srv)

	for i, tc := range []struct {
		routeKey, name string
		status         float64
		msg            string
	}{
		{"rk000002", "taken", 409, "That name is taken."},
		{"rk000002", "status", 500, "Something went wrong."},
		{"rk000002", "leak", 500, "Something went wrong."},
		{"rk000002", "redirect", 500, "Something went wrong."},
		{"rk000002", "panic", 500, "Something went wrong."},
		{"rk000002", "unencodable", 500, "Something went wrong."},
		{"rk000002", "encoder panic", 500, "Something went wrong."},
		{"rk000002", "slow", 504, "The call took too long."},
		{"other", "x", 404, "This call is not on this page."},
	} {
		call(t, c, i, tc.routeKey, tc.name, `null`)
		assert.Equal(t, map[string]any{"t": "fail", "id": float64(i), "status": tc.status, "msg": tc.msg}, readFrame(t, c), tc.name)
	}
	call(t, c, 99, "rk000002", "fine", `null`)
	assert.Equal(t, "result", readFrame(t, c)["t"], "the connection survives failed calls")
}

func TestCallsInFlightAreLimited(t *testing.T) {
	release := make(chan struct{})
	srv, _ := callSite(t, func(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error) {
		<-release
		return name, nil
	})
	c := openTool(t, srv)
	for i := range maxCallsInFlight + 1 {
		call(t, c, i, "rk000002", "wait", `null`)
	}
	assert.Equal(t, map[string]any{"t": "fail", "id": float64(maxCallsInFlight), "status": float64(http.StatusTooManyRequests),
		"msg": "Too many calls at once."}, readFrame(t, c))
	close(release)
	for range maxCallsInFlight {
		assert.Equal(t, "result", readFrame(t, c)["t"])
	}
}

func TestLiveNeedsValuesOrCalls(t *testing.T) {
	srv, _ := callSite(t, echo)
	_, status, err := dialLive(t, srv, "/admin/plain/__ws", "alice:admin", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, status)
}

func TestDecodeArgs(t *testing.T) {
	type in struct {
		On bool `json:"on"`
	}
	var v in
	require.NoError(t, DecodeArgs(json.RawMessage(`{"on":true}`), &v))
	assert.True(t, v.On)
	require.NoError(t, DecodeArgs(json.RawMessage(`null`), &v))
	for _, bad := range []string{``, `{"on":1}`, `{"off":true}`, `{"on":true} {}`, `{"on":true}]`, `[`} {
		var he *HTTPError
		require.ErrorAs(t, DecodeArgs(json.RawMessage(bad), &v), &he, bad)
		assert.Equal(t, http.StatusBadRequest, he.Status, bad)
	}
}

// Calls on a page below a gate re-run the gate's Guard, and a gate's calls are not on that page.
func TestCallsBelowGate(t *testing.T) {
	log := &callLog{}
	var denied atomic.Bool
	root := &fakeRoute{name: "root", access: Access{Roles: []string{"*"}}, log: log, layout: true}
	gate := &callRoute{fakeRoute: fakeRoute{name: "gate", access: Access{Roles: []string{"admin"}}, log: log,
		guardFn: func() error {
			if denied.Load() {
				return NotFound()
			}
			return nil
		}}, handle: echo}
	live := &liveRoute{fakeRoute: fakeRoute{name: "live", log: log}, key: "rk000001"}
	srv := serveRoutes(t, map[string]Route{"/": root, "/admin": gate, "/admin/live": live})
	c, _, err := dialLive(t, srv, "/admin/live/__ws", "alice:admin", nil)
	require.NoError(t, err)
	readFrame(t, c)

	call(t, c, 1, "rk000002", "star", `{}`)
	assert.Equal(t, map[string]any{"t": "fail", "id": float64(1), "status": float64(404), "msg": "This call is not on this page."}, readFrame(t, c))

	denied.Store(true)
	call(t, c, 2, "rk000002", "star", `{}`)
	assert.Equal(t, map[string]any{"t": "fail", "id": float64(2), "status": float64(404), "msg": "This page does not exist."}, readFrame(t, c),
		"the gate's Guard runs again before the call")
}
