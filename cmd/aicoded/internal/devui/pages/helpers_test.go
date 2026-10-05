package pages_test

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/pages"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
)

// executable is the path of aicoded the pages are given; it holds a space.
const executable = "/opt/dev tools/aicoded"

// session and developer are what the dev UI's front gives every request with a valid cookie.
var (
	session   = &runner.Session{App: "aicoded-dev", Env: "dev", Version: "dev", CSRFKey: bytes.Repeat([]byte{7}, 32)}
	developer = identity.Identity{Subject: "developer", Name: "Developer", Roles: []string{"developer"}}
)

// as returns ctx with the session and the viewer v.
func as(ctx context.Context, v identity.Identity) context.Context {
	return identity.With(runner.With(ctx, session), v)
}

// serve serves the pages over b on a test server, as the front does for the developer.
func serve(t *testing.T, b *fake.Backend) *httptest.Server {
	t.Helper()
	h := pages.NewHandler(deps.New(b, executable))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(as(r.Context(), developer)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fetch sends req to srv and returns the status and the body of the answer. It never follows
// a redirect.
func fetch(t *testing.T, srv *httptest.Server, req *http.Request) (int, string) {
	t.Helper()
	hc := &http.Client{
		Transport:     srv.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// get fetches the page at path.
func get(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	return fetch(t, srv, req)
}

// post fetches the page at path for its form token and posts form to it as the <ssr:form>
// named name, from the page's own origin. It never follows a redirect.
func post(t *testing.T, srv *httptest.Server, path, name string, form url.Values) (int, string) {
	t.Helper()
	return postFrom(t, srv, path, path, name, form)
}

// postFrom is post with the form and its token taken from the page at from.
func postFrom(t *testing.T, srv *httptest.Server, from, path, name string, form url.Values) (int, string) {
	t.Helper()
	_, page := get(t, srv, from)
	id := regexp.MustCompile(`name="_aicoded_form" value="([^"]*\.` + regexp.QuoteMeta(name) + `)"`).FindStringSubmatch(page)
	require.NotNil(t, id, "no form %s on %s", name, from)
	token := regexp.MustCompile(`name="_aicoded_csrf" value="([^"]*)"`).FindStringSubmatch(page)
	require.NotNil(t, token, "no form token on %s", from)
	body := url.Values{"_aicoded_form": {id[1]}, "_aicoded_csrf": {token[1]}}
	for k, v := range form {
		body[k] = v
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+path, strings.NewReader(body.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", srv.URL)
	return fetch(t, srv, req)
}

// liveKey returns the key of the first live part of page after the element with the id.
func liveKey(t *testing.T, page, id string) string {
	t.Helper()
	i := strings.Index(page, ` id="`+id+`"`)
	require.GreaterOrEqual(t, i, 0, "no element with the id %s", id)
	m := regexp.MustCompile(`data-ssr-bind="([^"]+)"`).FindStringSubmatch(page[i:])
	require.NotNil(t, m, "no live part after the element with the id %s", id)
	return m[1]
}

// elementText returns the text of the element with the id in page, up to the first end tag in
// it, without the tags and unescaped: the text a browser shows.
func elementText(t *testing.T, page, id string) string {
	t.Helper()
	i := strings.Index(page, ` id="`+id+`"`)
	require.GreaterOrEqual(t, i, 0, "no element with the id %s", id)
	s := page[i+strings.Index(page[i:], ">")+1:]
	s, _, _ = strings.Cut(s, "</")
	return html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, ""))
}

// live opens the live connection of the page at path, which may hold a query, from the
// page's own origin.
func live(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	p, query, _ := strings.Cut(path, "?")
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + strings.TrimSuffix(p, "/") + "/__ws"
	if query != "" {
		u += "?" + query
	}
	c, resp, err := websocket.Dial(t.Context(), u, &websocket.DialOptions{
		HTTPClient: srv.Client(),
		HTTPHeader: http.Header{"Origin": {srv.URL}},
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

// nextPatch returns the value of the next patch of the live value key, the key the page
// shows in data-ssr-bind or its part after the route's key, such as a variable's name. It
// waits at most 10s. The first patch of a page comes from the update deps.Follow makes at once.
func nextPatch(t *testing.T, c *websocket.Conn, key string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		_, data, err := c.Read(ctx)
		require.NoError(t, err)
		var f struct {
			T     string          `json:"t"`
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		require.NoError(t, json.Unmarshal(data, &f))
		if f.T == "patch" && (f.Key == key || strings.HasSuffix(f.Key, "."+key)) {
			var v string
			require.NoError(t, json.Unmarshal(f.Value, &v))
			return v
		}
	}
}
