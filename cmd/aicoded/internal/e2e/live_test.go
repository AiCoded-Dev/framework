package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialLive opens the live connection at path as persona and returns the handshake's status.
func (s *site) dialLive(t *testing.T, persona, path string) (*websocket.Conn, int, error) {
	t.Helper()
	h := http.Header{"Cookie": {"aicoded_dev_persona=" + persona}, "Origin": {s.url("")}}
	c, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(s.url(path), "http"),
		&websocket.DialOptions{HTTPClient: s.client(), HTTPHeader: h})
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

type frame struct {
	T        string                     `json:"t"`
	ID       int                        `json:"id"`
	Key      string                     `json:"key"`
	Kind     string                     `json:"kind"`
	Value    json.RawMessage            `json:"value"`
	Var      string                     `json:"var"`
	Msg      string                     `json:"msg"`
	Code     string                     `json:"code"`
	Status   int                        `json:"status"`
	Bindings map[string]json.RawMessage `json:"bindings"`
}

// text returns the value of a patch frame.
func (f frame) text() string {
	var s string
	_ = json.Unmarshal(f.Value, &s)
	return s
}

// next returns the first frame that matches, waiting at most 10s.
func next(t *testing.T, c *websocket.Conn, match func(frame) bool) frame {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		_, b, err := c.Read(ctx)
		require.NoError(t, err)
		var f frame
		require.NoError(t, json.Unmarshal(b, &f))
		if match(f) {
			return f
		}
	}
}

// Defect 2 end to end, under a layout and under a gate.
func TestLiveUnderDeniedParent(t *testing.T) {
	s := startSite(t)
	_, status, err := s.dialLive(t, "editor", "/admin/live/__ws")
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status)

	note := addNote(t, s, "editor", "Live")
	_, status, err = s.dialLive(t, "viewer", note+"/__ws")
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status, "the list's role rule covers the live connection")
}

func TestLiveRoundTrip(t *testing.T) {
	s := startSite(t)
	c, _, err := s.dialLive(t, "admin", "/admin/live/__ws")
	require.NoError(t, err)

	init := next(t, c, func(f frame) bool { return f.T == "init" })
	var key string
	for k, raw := range init.Bindings {
		if strings.HasSuffix(k, ".message") {
			key = k
			assert.JSONEq(t, `{"kind":"text","value":"<img src=x onerror=\"document.title='pwned'\">"}`, string(raw))
		}
	}
	require.NotEmpty(t, key)
	routeKey := strings.TrimSuffix(key, ".message")

	next(t, c, func(f frame) bool { return f.T == "patch" && strings.HasSuffix(f.Key, ".ticks") })

	write := func(value string) {
		b, err := json.Marshal(map[string]any{"t": "write", "routeKey": routeKey, "var": "message", "value": value})
		require.NoError(t, err)
		require.NoError(t, c.Write(t.Context(), websocket.MessageText, b))
	}
	write("<b>hi</b>")
	next(t, c, func(f frame) bool { return f.T == "ack" })
	block := next(t, c, func(f frame) bool { return f.T == "patch" && f.Kind == "html" && strings.Contains(f.text(), "hi") })
	assert.Contains(t, block.text(), "&lt;b&gt;hi&lt;/b&gt;")
	assert.NotContains(t, block.text(), "<b>")

	write(strings.Repeat("x", 201))
	refused := next(t, c, func(f frame) bool { return f.T == "err" })
	assert.Equal(t, "validation_failed", refused.Code)
	assert.Equal(t, "The message is too long.", refused.Msg)
}

// routeKey is how the generator names a route on the wire.
func routeKey(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:4])
}

func TestPageCall(t *testing.T) {
	s := startSite(t)
	note := addNote(t, s, "editor", "Callable")

	_, status, err := s.dialLive(t, "editor2", note+"/__ws")
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, status, "the guard runs before the connection opens")

	c, _, err := s.dialLive(t, "editor", note+"/__ws")
	require.NoError(t, err)
	next(t, c, func(f frame) bool { return f.T == "init" })
	// A call with bad arguments would run with starred false, so only its line has that needle.
	bads, goods := s.logged(t, "called", "call", "star", "starred", "false"), s.logged(t, "called", "call", "star", "starred", "true")

	bad := star(t, c, 1, `{"starred":"yes"}`)
	assert.Equal(t, "fail", bad.T)
	assert.Equal(t, http.StatusBadRequest, bad.Status)

	res := star(t, c, 2, `{"starred":true}`)
	assert.Equal(t, "result", res.T)
	assert.JSONEq(t, `{"starred":true,"title":"Callable"}`, string(res.Value))

	s.waitLogged(t, goods+1, "called", "call", "star", "starred", "true")
	assert.Equal(t, bads, s.logged(t, "called", "call", "star", "starred", "false"), "a call with bad arguments never runs")
}

// Ruling 17: every call checks access and guards again, so a note handed over while its page is
// open can no longer be starred from that page.
func TestPageCallRechecksGuard(t *testing.T) {
	s := startSite(t)
	note := addNote(t, s, "editor", "Handed over")
	c, _, err := s.dialLive(t, "editor", note+"/__ws")
	require.NoError(t, err)
	next(t, c, func(f frame) bool { return f.T == "init" })
	assert.Equal(t, "result", star(t, c, 1, `{"starred":true}`).T)

	_, page := s.get(t, "editor", note)
	resp, _ := s.post(t, "editor", note, url.Values{
		"_aicoded_form": {routeKey("/notes/n_id") + ".give"},
		"_aicoded_csrf": {field(t, page, "_aicoded_csrf")},
		"to":            {"editor2"},
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	resp, _ = s.get(t, "editor2", note)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the note is editor2's now")

	failed := star(t, c, 2, `{"starred":true}`)
	assert.Equal(t, "fail", failed.T)
	assert.Equal(t, http.StatusNotFound, failed.Status)
}

// star calls the note page's star over c with args and returns the answer to call id.
func star(t *testing.T, c *websocket.Conn, id int, args string) frame {
	t.Helper()
	msg := fmt.Sprintf(`{"t":"call","id":%d,"routeKey":%q,"name":"star","args":%s}`, id, routeKey("/notes/n_id"), args)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, []byte(msg)))
	return next(t, c, func(f frame) bool { return (f.T == "result" || f.T == "fail") && f.ID == id })
}
