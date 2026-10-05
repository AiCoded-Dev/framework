package live

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web/reactive"
)

// open serves one live connection to a new state of the page after its Data ran. The server
// sends the snapshot, runs push and then applies the page's writes.
func open(t *testing.T, push func(rs *ReactiveState)) *websocket.Conn {
	s := NewRoute(NewDP(nil)).NewState().(*state)
	require.NoError(t, s.Data(t.Context(), nil, nil))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := reactive.Accept(w, r)
		if err != nil {
			return
		}
		ctx := r.Context()
		_ = conn.Send(ctx, reactive.NewInit(s.Snapshot(ctx)))
		go func() { _ = conn.SendLoop(ctx) }()
		push(&ReactiveState{ctx: ctx, s: s, conn: conn})
		_ = conn.ReadFrames(ctx, func(msg reactive.WriteMsg) { s.HandleWrite(ctx, nil, conn, msg) }, nil)
	}))
	t.Cleanup(srv.Close)
	c, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func read(t *testing.T, c *websocket.Conn) map[string]any {
	_, b, err := c.Read(t.Context())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func write(t *testing.T, c *websocket.Conn, name, value string) {
	b, err := json.Marshal(reactive.WriteMsg{T: "write", RouteKey: routeKey, Var: name, Value: json.RawMessage(value)})
	require.NoError(t, err)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, b))
}

func html(values map[string]any) []any {
	var out []any
	for _, v := range values {
		if b := v.(map[string]any); b["kind"] == "html" {
			out = append(out, b["value"])
		}
	}
	return out
}

func noPush(*ReactiveState) {}

// logs collects what the app logs while a test runs.
type logs struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func captureLogs(t *testing.T) *logs {
	l := &logs{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(l, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return l
}

func TestSnapshot(t *testing.T) {
	bindings := read(t, open(t, noPush))["bindings"].(map[string]any)
	assert.Len(t, bindings, 4, "count, the list, the class and the condition")
	assert.Equal(t, map[string]any{"kind": "text", "value": "1"}, bindings[routeKey+".count"])
	assert.ElementsMatch(t, []any{"<tr><td>a</td></tr>", `<p class="n1">x</p>`, ""}, html(bindings))
}

func TestWriteUpdatesEverySiteOfTheVariable(t *testing.T) {
	c := open(t, noPush)
	read(t, c)
	write(t, c, "count", `"5"`)
	acked, patches := false, map[string]any{}
	for !acked || len(patches) < 3 {
		switch f := read(t, c); f["t"] {
		case "ack":
			acked = true
		case "patch":
			patches[f["key"].(string)] = map[string]any{"kind": f["kind"], "value": f["value"]}
		default:
			t.Fatalf("unexpected frame %v", f)
		}
	}
	assert.Equal(t, map[string]any{"kind": "text", "value": "5"}, patches[routeKey+".count"])
	assert.ElementsMatch(t, []any{`<p class="n5">x</p>`, "<p>many</p>"}, html(patches), "the list does not read count")
}

func TestRefusedWrites(t *testing.T) {
	logged := captureLogs(t)
	for _, w := range []struct{ name, value, code, msg string }{
		{"count", `"-1"`, reactive.CodeValidation, "The count must not be negative."},
		{"count", `"1001"`, reactive.CodeValidation, "The value was not accepted."},
		{"count", `"abc"`, reactive.CodeDecode, "The value has the wrong type."},
		{"items", `["x"]`, reactive.CodeValidation, "unknown variable"},
		{"Count", `"2"`, reactive.CodeValidation, "unknown variable"},
	} {
		c := open(t, noPush)
		read(t, c)
		write(t, c, w.name, w.value)
		assert.Equal(t, map[string]any{"t": "err", "routeKey": routeKey, "var": w.name, "msg": w.msg, "code": w.code}, read(t, c), w)
	}
	assert.Equal(t, 1, strings.Count(logged.String(), "level="), "only the error that is not a web.Error is logged")
	assert.Contains(t, logged.String(), `level=ERROR msg="live value refused" route=`+routeKey+` var=count err=*errors.errorString`)
	assert.NotContains(t, logged.String(), "the store takes counts", "with no runner, the error's text is not logged")
	assert.NotContains(t, logged.String(), "1001")
}

func TestBlocksUseThePageEscapers(t *testing.T) {
	c := open(t, func(rs *ReactiveState) { rs.SetItems([]string{`<script>x()</script>`}) })
	read(t, c)
	f := read(t, c)
	assert.Equal(t, "patch", f["t"])
	assert.Equal(t, "html", f["kind"])
	assert.Equal(t, "<tr><td>&lt;script&gt;x()&lt;/script&gt;</td></tr>", f["value"])
}
