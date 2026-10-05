package e2e

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
)

func TestPagesRender(t *testing.T) {
	s := startSite(t)
	resp, _ := s.get(t, "viewer", "/")
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "/home", resp.Header.Get("Location"))

	resp, body := s.get(t, "viewer", "/home")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, `<p id="viewer">viewer</p>`)
	assert.Equal(t, "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; "+
		"connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
		resp.Header.Get("Content-Security-Policy"))
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Regexp(t, `<link rel="stylesheet" href="/_aicoded/assets/pages/index-[^"]+\.css">`, body)
}

func TestValuesAreEscapedByContext(t *testing.T) {
	s := startSite(t)
	q := `<script>alert(1)</script>"&`
	_, body := s.get(t, "viewer", "/home?q="+url.QueryEscape(q)+"&link="+url.QueryEscape("javascript:alert(1)"))
	assert.Contains(t, body, `<p id="echo">&lt;script&gt;alert(1)&lt;/script&gt;&#34;&amp;</p>`)
	assert.Contains(t, body, `id="link" href="about:invalid#aicoded-unsafe-url"`)
	assert.Contains(t, body, `href="/home?q=%3cscript%3ealert%281%29%3c%2fscript%3e%22%26"`)
	assert.Contains(t, body, `<script type="application/json" id="page-data">{"q":"\u003cscript\u003ealert(1)\u003c/script\u003e\"\u0026"}</script>`)
	assert.NotContains(t, body, "<script>alert")
}

func TestAccessByRole(t *testing.T) {
	s := startSite(t)
	for _, c := range []struct {
		persona, path string
		status        int
	}{
		{"viewer", "/notes", http.StatusForbidden},
		{"editor", "/notes", http.StatusOK},
		{"editor", "/notes/abc", http.StatusNotFound},
		{"viewer", "/notes/1", http.StatusForbidden},
		{"editor", "/admin/live", http.StatusForbidden},
		{"admin", "/admin/live", http.StatusOK},
		{"admin", "/admin", http.StatusFound},
	} {
		resp, _ := s.get(t, c.persona, c.path)
		assert.Equal(t, c.status, resp.StatusCode, "%+v", c)
	}
}

// TestAccessSection checks that the permission list says what TestAccessByRole sees.
func TestAccessSection(t *testing.T) {
	s := startSite(t)
	data, err := os.ReadFile(filepath.Join(s.dir, "aicoded.yaml"))
	require.NoError(t, err)
	assert.Equal(t, `app: site
# access is written by aicoded generate from <ssr:access>; do not edit it
access:
  "/admin/live":
    require: ["admin"]
  "/admin/settings":
    require: ["admin"]
  "/home":
    require: ["*"]
  "/notes":
    require: ["editor"]
  "/notes/{id}":
    require: ["editor"]
    guard: true
    calls: ["star"]
`, string(data))
}

// Defect 1: a valid token from a page the viewer may open does not reach the Process hook of
// a form under a parent that denies the viewer.
func TestPostUnderDeniedParent(t *testing.T) {
	s := startSite(t)
	_, page := s.get(t, "editor", "/notes")
	_, settings := s.get(t, "admin", "/admin/settings")
	denied, saved := s.logged(t, "processed", "form", "save", "by", "editor"), s.logged(t, "processed", "form", "save", "by", "admin")
	save := func(persona, token string) *response {
		resp, _ := s.post(t, persona, "/admin/settings", url.Values{
			"_aicoded_form": {field(t, settings, "_aicoded_form")},
			"_aicoded_csrf": {token},
			"motto":         {"x"},
		})
		return resp
	}
	assert.Equal(t, http.StatusForbidden, save("editor", field(t, page, "_aicoded_csrf")).StatusCode)
	assert.Equal(t, http.StatusSeeOther, save("admin", field(t, settings, "_aicoded_csrf")).StatusCode)
	s.waitLogged(t, saved+1, "processed", "form", "save", "by", "admin")
	assert.Equal(t, denied, s.logged(t, "processed", "form", "save", "by", "editor"))
}

func TestNotesFlow(t *testing.T) {
	s := startSite(t)
	// form fills the add form of persona's list page with kv.
	form := func(persona string, kv ...string) url.Values {
		_, page := s.get(t, persona, "/notes")
		v := url.Values{"_aicoded_form": {field(t, page, "_aicoded_form")}, "_aicoded_csrf": {field(t, page, "_aicoded_csrf")}}
		for i := 0; i < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	invalid, added := s.logged(t, "processed", "form", "add", "by", "editor2"), s.logged(t, "processed", "form", "add", "by", "editor")

	for _, bad := range []url.Values{form("editor2", "title", "", "priority", "1"), form("editor2", "title", "x", "priority", "9")} {
		resp, body := s.post(t, "editor2", "/notes", bad)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, body, `id="form-error"`)
	}

	resp, _ := s.post(t, "editor", "/notes", form("editor", "title", "Mine", "priority", "2"))
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	s.waitLogged(t, added+1, "processed", "form", "add", "by", "editor")
	assert.Equal(t, invalid, s.logged(t, "processed", "form", "add", "by", "editor2"), "Process never runs for an invalid form")
	note := resp.Header.Get("Location")
	require.Regexp(t, `^/notes/\d+$`, note)
	resp, body := s.get(t, "editor", note)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, `<h1 id="title">Mine</h1>`)

	denied, renamed := s.logged(t, "processed", "form", "rename", "by", "editor2"), s.logged(t, "processed", "form", "rename", "by", "editor")
	resp, _ = s.get(t, "editor2", note)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the guard hides other viewers' notes")
	_, other := s.get(t, "editor2", "/notes")
	rename := func(persona, token, title string) *response {
		resp, _ := s.post(t, persona, note, url.Values{
			"_aicoded_form": {routeKey("/notes/n_id") + ".rename"},
			"_aicoded_csrf": {token},
			"title":         {title},
		})
		return resp
	}
	assert.Equal(t, http.StatusNotFound, rename("editor2", field(t, other, "_aicoded_csrf"), "Theirs").StatusCode)
	assert.Equal(t, http.StatusSeeOther, rename("editor", field(t, body, "_aicoded_csrf"), "Renamed").StatusCode)
	s.waitLogged(t, renamed+1, "processed", "form", "rename", "by", "editor")
	assert.Equal(t, denied, s.logged(t, "processed", "form", "rename", "by", "editor2"))
	_, body = s.get(t, "editor", note)
	assert.Contains(t, body, `<h1 id="title">Renamed</h1>`)
}

func TestFormTokens(t *testing.T) {
	s := startSite(t)
	_, page := s.get(t, "editor", "/notes")
	_, adminPage := s.get(t, "admin", "/notes")
	id := field(t, page, "_aicoded_form")
	for name, token := range map[string]string{"no token": "", "another viewer's token": field(t, adminPage, "_aicoded_csrf")} {
		resp, _ := s.post(t, "editor", "/notes", url.Values{"_aicoded_form": {id}, "_aicoded_csrf": {token}, "title": {"x"}, "priority": {"1"}})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, name)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url("/notes"),
		strings.NewReader(url.Values{"_aicoded_form": {id}, "_aicoded_csrf": {field(t, page, "_aicoded_csrf")}}.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://other.localhost:1")
	resp, _ := s.do(t, "editor", req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "cross-site")
}

func TestBodyLimit(t *testing.T) {
	s := startSite(t)
	resp, _ := s.post(t, "editor", "/notes", url.Values{"x": {strings.Repeat("a", 33<<20)}})
	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestAssetsServed(t *testing.T) {
	s := startSite(t)
	_, body := s.get(t, "admin", "/admin/live")
	assert.Contains(t, body, "/_aicoded/assets/aicoded/reactive-")
	links := regexp.MustCompile(`(?:src|href)="(/_aicoded/assets/[^"]+)"`).FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, links)
	for _, m := range links {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url(m[1]), nil)
		require.NoError(t, err)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, _ := s.do(t, "admin", req)
		assert.Equal(t, http.StatusOK, resp.StatusCode, m[1])
		assert.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"), m[1])
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"), m[1])
	}
}

// The list page is a gate: its access rule covers the note pages below it, and its markup and
// add form are not on them.
func TestListIsAGate(t *testing.T) {
	s := startSite(t)
	note := addNote(t, s, "editor", "Gated")
	resp, page := s.get(t, "editor", note)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotContains(t, page, `id="notes"`, "the list is not on the note page")
	assert.NotContains(t, page, `name="priority"`, "nor its add form")

	resp, _ = s.get(t, "viewer", note)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "the list's role rule covers the note page")

	_, list := s.get(t, "editor", "/notes")
	added := s.logged(t, "processed", "form", "add", "by", "editor")
	resp, _ = s.post(t, "editor", note, url.Values{
		"_aicoded_form": {field(t, list, "_aicoded_form")},
		"_aicoded_csrf": {field(t, list, "_aicoded_csrf")},
		"title":         {"Through the gate"},
		"priority":      {"1"},
	})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "the add form is not on the note page")
	addNote(t, s, "editor", "After the gate")
	s.waitLogged(t, added+1, "processed", "form", "add", "by", "editor")
}

// addNote adds a note as persona through the form and returns its path.
func addNote(t *testing.T, s *site, persona, title string) string {
	t.Helper()
	_, page := s.get(t, persona, "/notes")
	resp, _ := s.post(t, persona, "/notes", url.Values{
		"_aicoded_form": {field(t, page, "_aicoded_form")},
		"_aicoded_csrf": {field(t, page, "_aicoded_csrf")},
		"title":         {title},
		"priority":      {"1"},
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	return resp.Header.Get("Location")
}

// logged returns how many of the app's log entries have the message msg and the attributes kv,
// given as key, value pairs.
func (s *site) logged(t *testing.T, msg string, kv ...string) int {
	t.Helper()
	entries, err := s.ws.Logs(t.Context(), devapi.LogQuery{App: s.app, Contains: msg, Limit: 1000})
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		if e.Message == msg && hasAttrs(e.Attrs, kv) {
			n++
		}
	}
	return n
}

// hasAttrs reports whether attrs holds every key, value pair of kv.
func hasAttrs(attrs map[string]any, kv []string) bool {
	for i := 0; i < len(kv); i += 2 {
		if v, ok := attrs[kv[i]]; !ok || fmt.Sprint(v) != kv[i+1] {
			return false
		}
	}
	return true
}

// waitLogged waits until the app has logged msg with the attributes kv n times. The app's lines
// arrive in order, so once a line is in, every line logged before it is too: a test counts a
// refused action by attributes that only the refused actor's line has, after it waits for a
// later allowed one.
func (s *site) waitLogged(t *testing.T, n int, msg string, kv ...string) {
	t.Helper()
	assert.Eventually(t, func() bool { return s.logged(t, msg, kv...) == n }, 5*time.Second, 20*time.Millisecond,
		"want %s %v logged exactly %d times", msg, kv, n)
}
