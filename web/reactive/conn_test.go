package reactive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serve runs fn on every live connection the returned server accepts. The server has the
// given read and write timeouts.
func serve(t *testing.T, timeout time.Duration, fn func(ctx context.Context, c *Conn)) *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Accept(w, r)
		if err != nil {
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		fn(ctx, c)
	}))
	srv.Config.ReadTimeout = timeout
	srv.Config.WriteTimeout = timeout
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// echoServer accepts one live connection per request. It answers each write with an ack and
// a text patch holding the written value, and each call with its own arguments.
func echoServer(t *testing.T, timeout time.Duration) *httptest.Server {
	return serve(t, timeout, func(ctx context.Context, c *Conn) {
		go func() { _ = c.SendLoop(ctx) }()
		_ = c.ReadFrames(ctx, func(m WriteMsg) {
			c.Ack(ctx, m)
			c.Enqueue(m.RouteKey+"."+m.Var, TextBinding(string(m.Value)))
		}, func(m CallMsg) {
			_ = c.Send(ctx, NewResult(m.ID, m.Args))
		})
	})
}

func wsURL(srv *httptest.Server) string { return "ws" + strings.TrimPrefix(srv.URL, "http") }

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	c, resp, err := websocket.Dial(t.Context(), wsURL(srv), nil)
	require.NoError(t, err)
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, v any) {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, b))
}

func receive(t *testing.T, c *websocket.Conn) map[string]any {
	_, b, err := c.Read(t.Context())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// closeStatus reads until the server closes the connection and returns the close status, or
// -1 if the server keeps it open.
func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var err error
	for err == nil {
		_, _, err = c.Read(ctx)
	}
	return websocket.CloseStatus(err)
}

// The app's server has read and write timeouts; a live connection must outlive them.
func TestConnOutlivesServerTimeouts(t *testing.T) {
	srv := echoServer(t, 200*time.Millisecond)
	c := dial(t, srv)
	time.Sleep(500 * time.Millisecond)
	send(t, c, WriteMsg{T: "write", RouteKey: "rk", Var: "name", Value: json.RawMessage(`"x"`)})
	assert.Equal(t, map[string]any{"t": "ack", "routeKey": "rk", "var": "name"}, receive(t, c))
	assert.Equal(t, map[string]any{"t": "patch", "key": "rk.name", "kind": "text", "value": `"x"`}, receive(t, c))
}

func TestAcceptRefusesOtherOrigins(t *testing.T) {
	opts := &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"https://evil.example"}}}
	_, resp, err := websocket.Dial(t.Context(), wsURL(echoServer(t, time.Minute)), opts)
	require.Error(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestReadLimit(t *testing.T) {
	c := dial(t, echoServer(t, time.Minute))
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(strings.Repeat("a", readLimit+1))))
	_, _, err := c.Read(t.Context())
	assert.Equal(t, websocket.StatusMessageTooBig, websocket.CloseStatus(err))
}

func TestFrameAtReadLimitIsAccepted(t *testing.T) {
	c := dial(t, echoServer(t, time.Minute))
	msg := WriteMsg{T: "write", RouteKey: "rk", Var: "v", Value: json.RawMessage(`""`)}
	base, err := json.Marshal(msg)
	require.NoError(t, err)
	msg.Value = json.RawMessage(`"` + strings.Repeat("a", readLimit-len(base)) + `"`)
	b, err := json.Marshal(msg)
	require.NoError(t, err)
	require.Len(t, b, readLimit)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, b))
	assert.Equal(t, map[string]any{"t": "ack", "routeKey": "rk", "var": "v"}, receive(t, c))
}

func TestCallsAreAnswered(t *testing.T) {
	c := dial(t, echoServer(t, time.Minute))
	send(t, c, CallMsg{T: "call", ID: 7, RouteKey: "rk", Name: "star", Args: json.RawMessage(`{"on":true}`)})
	assert.Equal(t, map[string]any{"t": "result", "id": float64(7), "value": map[string]any{"on": true}}, receive(t, c))
}

// Writes and calls share one budget.
func TestWriteRateLimit(t *testing.T) {
	c := dial(t, echoServer(t, time.Minute))
	for i := range writeBurst + 10 {
		if i%2 == 0 {
			send(t, c, WriteMsg{T: "write", RouteKey: "rk", Var: "v", Value: json.RawMessage(`1`)})
		} else {
			send(t, c, CallMsg{T: "call", ID: uint32(i), RouteKey: "rk", Name: "n", Args: json.RawMessage(`null`)})
		}
	}
	assert.Equal(t, websocket.StatusPolicyViolation, closeStatus(t, c))
}

// Frames the page never sends count against the same budget.
func TestUnknownFramesCountAgainstTheRateLimit(t *testing.T) {
	c := dial(t, echoServer(t, time.Minute))
	for range writeBurst + 10 {
		send(t, c, map[string]string{"t": "ping"})
	}
	assert.Equal(t, websocket.StatusPolicyViolation, closeStatus(t, c))
}

func TestInvalidFramesClose(t *testing.T) {
	for name, frame := range map[string]struct {
		typ  websocket.MessageType
		data string
	}{
		"not JSON":    {websocket.MessageText, `{"t":"write"`},
		"binary":      {websocket.MessageBinary, `{"t":"write","routeKey":"rk","var":"v","value":1}`},
		"negative id": {websocket.MessageText, `{"t":"call","id":-1,"routeKey":"rk","name":"n","args":null}`},
	} {
		t.Run(name, func(t *testing.T) {
			c := dial(t, echoServer(t, time.Minute))
			require.NoError(t, c.Write(t.Context(), frame.typ, []byte(frame.data)))
			assert.Equal(t, websocket.StatusUnsupportedData, closeStatus(t, c))
		})
	}
}

// A nil handler ignores its frames, and so does the reader for frames of any other type.
func TestIgnoredFrames(t *testing.T) {
	c := dial(t, serve(t, time.Minute, func(ctx context.Context, c *Conn) {
		_ = c.ReadFrames(ctx, func(m WriteMsg) { c.Ack(ctx, m) }, nil)
	}))
	send(t, c, CallMsg{T: "call", ID: 1, RouteKey: "rk", Name: "n", Args: json.RawMessage(`null`)})
	send(t, c, map[string]string{"t": "patch", "key": "rk.v", "kind": "html", "value": "<b>"})
	send(t, c, WriteMsg{T: "write", RouteKey: "rk", Var: "v", Value: json.RawMessage(`1`)})
	assert.Equal(t, map[string]any{"t": "ack", "routeKey": "rk", "var": "v"}, receive(t, c))
}

func TestRejectSendsErr(t *testing.T) {
	c := dial(t, serve(t, time.Minute, func(ctx context.Context, c *Conn) {
		_ = c.ReadFrames(ctx, func(m WriteMsg) { c.Reject(ctx, m, "too long", CodeValidation) }, nil)
	}))
	send(t, c, WriteMsg{T: "write", RouteKey: "rk", Var: "v", Value: json.RawMessage(`"xxxx"`)})
	assert.Equal(t, map[string]any{"t": "err", "routeKey": "rk", "var": "v", "msg": "too long", "code": "validation_failed"}, receive(t, c))
}

// Values enqueued for one key before the send loop runs go out as one patch with the last
// value.
func TestPatchesCoalesce(t *testing.T) {
	c := dial(t, serve(t, time.Minute, func(ctx context.Context, c *Conn) {
		for i := range 100 {
			c.Enqueue("rk.n", TextBinding(strconv.Itoa(i)))
		}
		go func() { _ = c.SendLoop(ctx) }()
		_ = c.ReadFrames(ctx, nil, nil)
	}))
	assert.Equal(t, map[string]any{"t": "patch", "key": "rk.n", "kind": "text", "value": "99"}, receive(t, c))

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	_, _, err := c.Read(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "no second patch")
}

func TestSendLoopStopsWithContext(t *testing.T) {
	done := make(chan error, 1)
	dial(t, serve(t, time.Minute, func(ctx context.Context, c *Conn) {
		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		done <- c.SendLoop(ctx)
	}))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "SendLoop did not stop")
	}
}

// A client that stops reading is cut off once a patch has waited sendTimeout.
func TestSlowClientIsClosed(t *testing.T) {
	done := make(chan error, 1)
	start := time.Now()
	dial(t, serve(t, time.Minute, func(ctx context.Context, c *Conn) {
		big := TextBinding(strings.Repeat("x", 1<<20))
		for i := range 64 {
			c.Enqueue(strconv.Itoa(i), big)
		}
		done <- c.SendLoop(ctx)
	}))
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.GreaterOrEqual(t, time.Since(start), sendTimeout)
	case <-time.After(sendTimeout + 10*time.Second):
		require.FailNow(t, "slow client was not closed")
	}
}

func TestFrames(t *testing.T) {
	b, err := json.Marshal(NewInit(map[string]Binding{"rk.a": TextBinding("<b>"), "rk.b": HTMLBinding("<i>x</i>")}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"t":"init","bindings":{"rk.a":{"kind":"text","value":"<b>"},"rk.b":{"kind":"html","value":"<i>x</i>"}}}`, string(b))
	b, err = json.Marshal(NewErr("rk", "v", "too long", CodeValidation))
	require.NoError(t, err)
	assert.JSONEq(t, `{"t":"err","routeKey":"rk","var":"v","msg":"too long","code":"validation_failed"}`, string(b))
	b, err = json.Marshal(NewResult(3, json.RawMessage(`{"a":1}`)))
	require.NoError(t, err)
	assert.JSONEq(t, `{"t":"result","id":3,"value":{"a":1}}`, string(b))
	b, err = json.Marshal(NewFail(3, 404, "gone"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"t":"fail","id":3,"status":404,"msg":"gone"}`, string(b))
}
