package devui_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

const mailCSP = "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; frame-ancestors 'self'"

var token = strings.Repeat("0123456789abcdef", 4)

// cookie is the value of the cookie the login link sets: a MAC of the token.
var cookie = func() string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("aicoded dev ui"))
	return hex.EncodeToString(mac.Sum(nil))
}()

func backend() *fake.Backend {
	return &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Root: "/work", Gateway: "http://localhost:8080/", Apps: []devapi.App{{Name: "shop", State: devapi.Running}}},
		Mail: map[string][]devapi.Mail{"shop": {
			{MailSummary: devapi.MailSummary{ID: "sent-1", Folder: "sent"}, Text: "Hi", HTML: `<p>Hi</p><script>alert(1)</script>`},
			{MailSummary: devapi.MailSummary{ID: "sent-2", Folder: "sent"}, Text: "Only text"},
		}},
	}}
}

// serve serves the dev UI over b on a test server.
func serve(t *testing.T, b deps.Backend) *httptest.Server {
	srv := httptest.NewServer(devui.New(b, token, "/opt/aicoded", io.Discard))
	t.Cleanup(srv.Close)
	return srv
}

// fetch sends a request for path to srv with the headers given, without following redirects,
// and returns the status, the headers and the body of the answer.
func fetch(t *testing.T, srv *httptest.Server, method, path string, header http.Header) (int, http.Header, string) {
	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	require.NoError(t, err)
	req.Header = header
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(body)
}

// dial opens the live connection of the apps page from the browser that logged in, from
// origin, and returns the status of the answer.
func dial(t *testing.T, srv *httptest.Server, origin string) (int, error) {
	c, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/apps/__ws", &websocket.DialOptions{HTTPHeader: loggedIn(origin)})
	if c != nil {
		_ = c.CloseNow()
	}
	if resp == nil {
		return 0, err
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return resp.StatusCode, err
}

// loggedIn returns the headers of a request from the browser that logged in, from origin.
func loggedIn(origin string) http.Header {
	return http.Header{"Cookie": {"__Host-aicoded_dev_ui=" + cookie}, "Origin": {origin}}
}

func TestLogin(t *testing.T) {
	srv := serve(t, backend())
	status, header, _ := fetch(t, srv, http.MethodGet, devui.LoginPath+"?token="+token, nil)
	assert.Equal(t, http.StatusSeeOther, status)
	assert.Equal(t, "/", header.Get("Location"))
	assert.Equal(t, "no-referrer", header.Get("Referrer-Policy"))
	require.Len(t, header.Values("Set-Cookie"), 1)
	c, err := http.ParseSetCookie(header.Get("Set-Cookie"))
	require.NoError(t, err)
	assert.Equal(t, "__Host-aicoded_dev_ui", c.Name)
	assert.Equal(t, cookie, c.Value, "the cookie holds a MAC of the token, never the token")
	assert.Equal(t, "/", c.Path)
	assert.Empty(t, c.Domain)
	assert.True(t, c.Secure)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
}

func TestLoginRefusesWrongLinks(t *testing.T) {
	b := backend()
	srv := serve(t, b)
	for _, query := range []string{
		"",
		"?token=",
		"?token=" + strings.Repeat("0", 64),
		"?token=" + token[:63],
		"?token=" + token + "0",
		"?token=" + strings.ToUpper(token),
		"?t=" + token,
		"?token=" + token + "&token=" + token,
		"?token=" + token + "&next=/apps",
	} {
		status, header, body := fetch(t, srv, http.MethodGet, devui.LoginPath+query, nil)
		assert.Equal(t, http.StatusForbidden, status, query)
		assert.Empty(t, header.Values("Set-Cookie"), query)
		assert.Contains(t, body, "E-DEV-019", query)
		assert.NotContains(t, body, token, query)
	}
	assert.Empty(t, b.Calls())
}

func TestRefusesWithoutTheCookie(t *testing.T) {
	b := backend()
	srv := serve(t, b)
	for _, header := range []http.Header{
		nil,
		{"Cookie": {"__Host-aicoded_dev_ui=" + token}},
		{"Cookie": {"__Host-aicoded_dev_ui=" + cookie[:63]}},
		{"Cookie": {"aicoded_dev_ui=" + cookie}},
	} {
		for _, path := range []string{"/", "/apps", "/apps/__ws", "/_aicoded/assets/aicoded/reactive-MTQXR3MF.js", "/_aicoded/mail/shop/sent-1", "/nope"} {
			status, answer, body := fetch(t, srv, http.MethodGet, path, header)
			assert.Equal(t, http.StatusUnauthorized, status, path)
			assert.Contains(t, body, "E-DEV-019", path)
			assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", answer.Get("Content-Security-Policy"))
		}
	}
	assert.Empty(t, b.Calls(), "no request reached the pages")
}

func TestPagesAfterLogin(t *testing.T) {
	srv := serve(t, backend())
	status, header, _ := fetch(t, srv, http.MethodGet, "/", loggedIn(srv.URL))
	assert.Equal(t, http.StatusFound, status)
	assert.Equal(t, "/apps", header.Get("Location"))
	status, _, body := fetch(t, srv, http.MethodGet, "/apps", loggedIn(srv.URL))
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "<title>aicoded dev</title>")
	assert.Contains(t, body, "shop")

	status, err := dial(t, srv, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, status)
}

func TestPagesRefuseOtherSites(t *testing.T) {
	srv := serve(t, backend())
	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	for _, origin := range []string{"http://shop.localhost" + port, "http://evil.example", "null"} {
		status, _, _ := fetch(t, srv, http.MethodPost, "/apps", loggedIn(origin))
		assert.Equal(t, http.StatusForbidden, status, origin)
		status, err := dial(t, srv, origin)
		require.Error(t, err, origin)
		assert.Equal(t, http.StatusForbidden, status, origin)
	}
	header := loggedIn(srv.URL)
	header.Set("Sec-Fetch-Site", "cross-site")
	status, _, _ := fetch(t, srv, http.MethodPost, "/apps", header)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestMailHTML(t *testing.T) {
	srv := serve(t, backend())
	status, header, body := fetch(t, srv, http.MethodGet, "/_aicoded/mail/shop/sent-1", loggedIn(srv.URL))
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, `<p>Hi</p><script>alert(1)</script>`, body)
	assert.Equal(t, mailCSP, header.Get("Content-Security-Policy"))
	assert.Equal(t, "text/html; charset=utf-8", header.Get("Content-Type"))
	assert.Equal(t, "nosniff", header.Get("X-Content-Type-Options"))
	assert.Equal(t, "no-store", header.Get("Cache-Control"))

	for _, path := range []string{
		"/_aicoded/mail/shop/sent-2", // no HTML part
		"/_aicoded/mail/shop/sent-9",
		"/_aicoded/mail/nope/sent-1",
		"/_aicoded/mail/",
		"/_aicoded/mail/shop",
		"/_aicoded/mail/shop/",
		"/_aicoded/mail//sent-1",
		"/_aicoded/mail/shop/sent-1/x",
	} {
		status, _, _ := fetch(t, srv, http.MethodGet, path, loggedIn(srv.URL))
		assert.Equal(t, http.StatusNotFound, status, path)
	}
}

// panicky is a backend whose Status panics.
type panicky struct{ *fake.Backend }

func (panicky) Status(context.Context) (devapi.Status, error) { panic("the panic's own text") }

func TestPanicGoesToTheTerminal(t *testing.T) {
	var out bytes.Buffer
	w := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/apps", nil)
	r.Header = loggedIn("")
	devui.New(panicky{backend()}, token, "/opt/aicoded", &out).ServeHTTP(w, r)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "aicoded dev could not show this page.")
	assert.NotContains(t, w.Body.String(), "the panic's own text")
	assert.Contains(t, out.String(), "aicoded dev: the dev UI failed on GET \"/apps\": the panic's own text\n")
	assert.Contains(t, out.String(), "devui_test.panicky.Status", "the stack")
}

func TestNewNeedsAToken(t *testing.T) {
	for _, bad := range []string{"", "short", strings.ToUpper(token)} {
		assert.Panics(t, func() { devui.New(backend(), bad, "/opt/aicoded", io.Discard) }, bad)
	}
}
