package dev

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

// stubApp serves stubHandler on a socket and returns its path.
func stubApp(t *testing.T, pub ed25519.PublicKey, app string) string {
	path := filepath.Join(t.TempDir(), runnerproto.AppSocket)
	l, err := socket.Listen(t.Context(), path)
	require.NoError(t, err)
	srv := socket.NewServer(stubHandler(pub, app))
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path
}

// stubHandler answers with the verified subject, the Host it saw and whether a trace arrived.
func stubHandler(pub ed25519.PublicKey, app string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := viewer.Verify([]ed25519.PublicKey{pub}, r.Header.Get(viewer.Header), app, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		_, traced := tracectx.Parse(r.Header.Get("traceparent"))
		fmt.Fprintf(w, "%s %s %t", c.Subject, r.Host, traced)
	})
}

// ui stands in for the dev UI: it answers "ui" and the path.
var ui = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ui "+r.URL.Path) })

func newTestGateway(t *testing.T, port int) *Gateway {
	signer, err := NewSigner()
	require.NoError(t, err)
	g := NewGateway(port, []devconfig.Persona{{Name: "viewer"}, {Name: "alice", Roles: []string{"hotel-ops"}}}, signer, ui)
	g.Add("rooms", stubApp(t, signer.Public(), "rooms"), newStore("rooms", io.Discard))
	return g
}

func serve(g *Gateway, method, host, path string, header http.Header, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, "http://"+host+path, strings.NewReader(body))
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	return w
}

func TestGatewayReplacesViewerToken(t *testing.T) {
	g := newTestGateway(t, 8080)
	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", http.Header{viewer.Header: {"forged"}}, "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "viewer rooms.localhost:8080 true", w.Body.String())
}

func TestGatewayRefusesForeignHosts(t *testing.T) {
	g := newTestGateway(t, 8080)
	for _, host := range []string{"evil.example:8080", "rooms.localhost:9999", "rooms.localhost", "a.rooms.localhost:8080", "127.0.0.1:8080"} {
		assert.Equal(t, http.StatusMisdirectedRequest, serve(g, http.MethodGet, host, "/", nil, "").Code, "host %s", host)
	}
	assert.Equal(t, http.StatusNotFound, serve(g, http.MethodGet, "billing.localhost:8080", "/", nil, "").Code)
}

func TestGatewayServesTheUIOnTheBareHost(t *testing.T) {
	g := newTestGateway(t, 8080)
	for _, path := range []string{"/", "/apps", personaPath, "/_aicoded/login"} {
		assert.Equal(t, "ui "+path, serve(g, http.MethodGet, "localhost:8080", path, nil, "").Body.String())
	}
	assert.Equal(t, "ui /", serve(g, http.MethodGet, "LOCALHOST:8080", "/", nil, "").Body.String())
	for _, host := range []string{"127.0.0.1:8080", "[::1]:8080", "evil.example:8080", "localhost:9999", "localhost"} {
		w := serve(g, http.MethodGet, host, "/", nil, "")
		assert.Equal(t, http.StatusMisdirectedRequest, w.Code, host)
		assert.NotContains(t, w.Body.String(), "ui", host)
	}
}

func TestGatewayOnPort80AcceptsHostWithoutPort(t *testing.T) {
	g := newTestGateway(t, 80)
	assert.Equal(t, "viewer rooms.localhost true", serve(g, http.MethodGet, "rooms.localhost", "/", nil, "").Body.String())
	assert.Equal(t, http.StatusMisdirectedRequest, serve(g, http.MethodGet, "evil.example", "/", nil, "").Code)
}

func TestPersonaSwitch(t *testing.T) {
	g := newTestGateway(t, 8080)
	form := http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}
	assert.Equal(t, http.StatusForbidden, serve(g, http.MethodPost, "rooms.localhost:8080", personaPath, form, "name=alice").Code)
	for _, origin := range []string{"http://billing.localhost:8080", "null", "https://rooms.localhost:8080"} {
		form.Set("Origin", origin)
		assert.Equal(t, http.StatusForbidden, serve(g, http.MethodPost, "rooms.localhost:8080", personaPath, form, "name=alice").Code, "origin %s", origin)
	}

	form.Set("Origin", "http://rooms.localhost:8080")
	assert.Equal(t, http.StatusBadRequest, serve(g, http.MethodPost, "rooms.localhost:8080", personaPath, form, "name=mallory").Code)
	w := serve(g, http.MethodPost, "rooms.localhost:8080", personaPath, form, "name=alice")
	require.Equal(t, http.StatusSeeOther, w.Code)
	c, err := http.ParseSetCookie(w.Header().Get("Set-Cookie"))
	require.NoError(t, err)
	assert.True(t, c.Secure)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)

	w = serve(g, http.MethodGet, "rooms.localhost:8080", "/", http.Header{"Cookie": {c.Name + "=" + c.Value}}, "")
	assert.Equal(t, "alice rooms.localhost:8080 true", w.Body.String())

	page := serve(g, http.MethodGet, "rooms.localhost:8080", personaPath, nil, "")
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `value="alice"`)
	assert.Contains(t, page.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
}

func TestGatewayErrorPage(t *testing.T) {
	g := newTestGateway(t, 8080)
	g.Fail("rooms", []problem.Problem{{App: "rooms", Code: "E-CHK-002", Pos: "main.go:3", Message: "<script>alert(1)</script>",
		Fix: "fix <b>it</b>", Docs: errs.DocsBase + "E-CHK-002"}})
	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, pageCSP, w.Header().Get("Content-Security-Policy"))
	body := w.Body.String()
	assert.Contains(t, body, "<title>rooms is not running</title>")
	assert.Contains(t, body, "<b>E-CHK-002</b> at <code>main.go:3</code>")
	assert.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, body, "Fix: fix &lt;b&gt;it&lt;/b&gt;")
	assert.Contains(t, body, `<a href="https://aicoded.dev/docs/errors/E-CHK-002">`)
	assert.NotContains(t, body, "<script>")
	assert.Equal(t, http.StatusOK, serve(g, http.MethodGet, "rooms.localhost:8080", personaPath, nil, "").Code, "the persona page still works")

	g.Fail("rooms", []problem.Problem{{Message: "m", Docs: "javascript:alert(1)"}})
	body = serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Body.String()
	assert.Contains(t, body, `href="#ZgotmplZ"`, "a docs link that is not a web address is no link")
	assert.NotContains(t, body, `href="javascript:`)
}

func TestGatewayStartingAndRemove(t *testing.T) {
	g := newTestGateway(t, 8080)
	g.Starting("rooms")
	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "1", w.Header().Get("Refresh"))
	assert.Contains(t, w.Body.String(), "<title>rooms is starting</title>")

	g.Remove("rooms")
	assert.Equal(t, http.StatusNotFound, serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Code)
}

func TestGatewayNoPersonas(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	g := NewGateway(8080, nil, signer, nil)
	g.Add("rooms", stubApp(t, signer.Public(), "rooms"), newStore("rooms", io.Discard))
	assert.Equal(t, "no-roles rooms.localhost:8080 true", serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Body.String())
	assert.Contains(t, serve(g, http.MethodGet, "rooms.localhost:8080", personaPath, nil, "").Body.String(), `value="no-roles"`)
}

func TestGatewayAddClosesOld(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	g := NewGateway(8080, nil, signer, nil)
	path, closed := closedConns(t, stubHandler(signer.Public(), "rooms"))
	g.Add("rooms", path, newStore("rooms", io.Discard))
	require.Equal(t, http.StatusOK, serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Code)
	g.Add("rooms", stubApp(t, signer.Public(), "rooms"), newStore("rooms", io.Discard))
	requireClosed(t, closed, "the replaced proxy kept its idle connection")
	assert.Equal(t, http.StatusOK, serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Code)
}

func TestGatewayStopped(t *testing.T) {
	g := newTestGateway(t, 8080)
	g.Stopped("rooms")
	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, pageCSP, w.Header().Get("Content-Security-Policy"))
	assert.Contains(t, w.Body.String(),
		`rooms is stopped. Start it on the dev UI at <a href="http://localhost:8080/apps/rooms">http://localhost:8080/apps/rooms</a>.`)
	assert.Equal(t, http.StatusOK, serve(g, http.MethodGet, "rooms.localhost:8080", personaPath, nil, "").Code, "the persona page still works")
}

// TestGatewayAppDidNotAnswer swaps the standard logger, so no test of the package may run in
// parallel with it.
func TestGatewayAppDidNotAnswer(t *testing.T) {
	var std bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&std)
	t.Cleanup(func() { log.SetOutput(orig) })
	signer, err := NewSigner()
	require.NoError(t, err)
	g := NewGateway(8080, nil, signer, nil)
	var out bytes.Buffer
	st := newStore("rooms", &out)
	g.Add("rooms", filepath.Join(t.TempDir(), runnerproto.AppSocket), st)

	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Equal(t, pageCSP, w.Header().Get("Content-Security-Policy"))
	assert.Contains(t, w.Body.String(), "<title>rooms did not answer</title>")
	entries := st.logs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, sourceRunner, entries[0].Source)
	assert.Equal(t, "ERROR", entries[0].Level)
	assert.Contains(t, entries[0].Message, "gateway: the app did not answer: dial unix ")
	assert.Contains(t, out.String(), "rooms | gateway: the app did not answer: dial unix ", "it is printed")
	assert.Empty(t, std.String(), "nothing goes to the standard logger")
}

func TestGatewayAnswerCutOff(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	g := NewGateway(8080, nil, signer, nil)
	var out bytes.Buffer
	st := newStore("rooms", &out)
	g.Add("rooms", serveAt(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "short")
	})), st)

	serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	entries := st.logs.All()
	require.NotEmpty(t, entries)
	assert.Contains(t, entries[0].Message, "gateway: httputil: ReverseProxy read error during body copy: unexpected EOF")
	for _, e := range entries {
		assert.Equal(t, "WARN", e.Level, e.Message)
	}
	assert.Empty(t, out.String(), "nothing is printed")
}

func TestGatewayPersonas(t *testing.T) {
	assert.Equal(t, []devconfig.Persona{{Name: "viewer"}, {Name: "alice", Roles: []string{"hotel-ops"}}}, newTestGateway(t, 8080).Personas())
	assert.Equal(t, []devconfig.Persona{{Name: "no-roles"}}, NewGateway(8080, nil, nil, nil).Personas())
}

func TestGatewayManual(t *testing.T) {
	g := newTestGateway(t, 8080)
	g.AddManual("rooms", filepath.Join(t.TempDir(), runnerproto.AppSocket), "cd '/src/<rooms>' && go run .")
	w := serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, pageCSP, w.Header().Get("Content-Security-Policy"))
	assert.Contains(t, w.Body.String(), "rooms runs by hand and is not running now. Start it with:")
	assert.Contains(t, w.Body.String(), "<pre>cd &#39;/src/&lt;rooms&gt;&#39; &amp;&amp; go run .</pre>")

	g.AddManual("rooms", stubApp(t, g.signer.Public(), "rooms"), "go run .")
	assert.Equal(t, "viewer rooms.localhost:8080 true", serve(g, http.MethodGet, "rooms.localhost:8080", "/", nil, "").Body.String())
}
