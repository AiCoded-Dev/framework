package web

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

type site struct {
	log    *callLog
	routes map[string]*fakeRoute
	h      http.Handler
}

func newSite(mutate func(map[string]*fakeRoute)) *site {
	log := &callLog{}
	rs := map[string]*fakeRoute{
		"/":             {name: "root", access: Access{Roles: []string{"*"}}, children: "/home", layout: true},
		"/home":         {name: "home"},
		"/notes":        {name: "notes", access: Access{Roles: []string{"editor"}}},
		"/note/n_id":    {name: "note", access: Access{Roles: []string{"editor"}}, layout: true},
		"/note/n_id/ed": {name: "edit", access: Access{Roles: []string{"owner"}}},
		"/admin":        {name: "admin", access: Access{Roles: []string{"admin"}}, children: "/panel", layout: true},
		"/admin/panel":  {name: "panel"},
	}
	if mutate != nil {
		mutate(rs)
	}
	routes := map[string]Route{}
	for p, r := range rs {
		r.log = log
		routes[p] = r
	}
	return &site{log: log, routes: rs, h: New(routes, Options{})}
}

func (s *site) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, r)
	return w
}

func post(target, subject, formID string, roles ...string) *http.Request {
	r := request(http.MethodPost, target, subject, url.Values{FormField: {formID}, CSRFField: {token(subject)}}, roles...)
	r.Header.Set("Origin", "http://site.test")
	return r
}

// upload posts fields and a file of size bytes to /notes as multipart/form-data, with no
// announced length, as a chunked body arrives.
func upload(t *testing.T, fields url.Values, size int) *http.Request {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, values := range fields {
		for _, v := range values {
			require.NoError(t, mw.WriteField(name, v))
		}
	}
	fw, err := mw.CreateFormFile("f", "f.bin")
	require.NoError(t, err)
	_, err = fw.Write(make([]byte, size))
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	r := request(http.MethodPost, "/notes", "alice", nil, "editor")
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Body, r.ContentLength = io.NopCloser(&body), -1
	return r
}

func TestPageRendersRootToLeaf(t *testing.T) {
	s := newSite(nil)
	w := s.serve(request(http.MethodGet, "/home", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><home></home></root>", w.Body.String())
	assert.Equal(t, []string{"root.guard", "home.guard", "root.init", "home.init", "root.data", "home.data"}, s.log.all())
	assert.Equal(t, csp, w.Header().Get("Content-Security-Policy"), "a hook cannot replace the CSP")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "home", w.Header().Get("X-Route"))
}

func TestPostRunsProcessAfterChecks(t *testing.T) {
	s := newSite(nil)
	w := s.serve(post("/notes", "alice", "notes.form", "editor"))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"root.guard", "notes.guard", "root.init", "notes.init", "notes.process", "root.data", "notes.data"}, s.log.all())
}

// Defect 1: a child's Process must never run when a parent denies the viewer.
func TestPostUnderDeniedParentNeverProcesses(t *testing.T) {
	s := newSite(nil)
	w := s.serve(post("/admin/panel", "bob", "panel.form", "editor"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, s.log.all())

	s = newSite(func(rs map[string]*fakeRoute) { rs["/note/n_id"].guard = NotFound() })
	w = s.serve(post("/note/7/ed", "alice", "edit.form", "editor", "owner"))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, []string{"root.guard", "note.guard"}, s.log.all())
}

func TestAccessRulesAreANDed(t *testing.T) {
	s := newSite(nil)
	assert.Equal(t, http.StatusForbidden, s.serve(request(http.MethodGet, "/note/7/ed", "alice", nil, "owner")).Code)
	assert.Equal(t, http.StatusForbidden, s.serve(request(http.MethodGet, "/note/7/ed", "alice", nil, "editor")).Code)
	assert.Equal(t, http.StatusOK, s.serve(request(http.MethodGet, "/note/7/ed", "alice", nil, "editor", "owner")).Code)
}

func TestRouteWithoutAnyRuleIsDenied(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) { rs["/"].access = Access{} })
	assert.Equal(t, http.StatusForbidden, s.serve(request(http.MethodGet, "/home", "alice", nil)).Code)
	assert.Empty(t, s.log.all())
}

// Defect N2 and defect 4: Init runs only after the token check, and Process only for a valid form.
func TestTokenIsCheckedBeforeInit(t *testing.T) {
	s := newSite(nil)
	r := request(http.MethodPost, "/notes", "alice", url.Values{FormField: {"notes.form"}, CSRFField: {token("bob")}}, "editor")
	assert.Equal(t, http.StatusForbidden, s.serve(r).Code)
	assert.Equal(t, []string{"root.guard", "notes.guard"}, s.log.all())

	s = newSite(nil)
	r = request(http.MethodPost, "/notes", "alice", url.Values{FormField: {"notes.form"}}, "editor")
	assert.Equal(t, http.StatusForbidden, s.serve(r).Code)

	s = newSite(func(rs map[string]*fakeRoute) { rs["/notes"].invalid = true })
	assert.Equal(t, http.StatusOK, s.serve(post("/notes", "alice", "notes.form", "editor")).Code)
	assert.NotContains(t, s.log.all(), "notes.process")
}

func TestCrossSitePostIsRefused(t *testing.T) {
	s := newSite(nil)
	r := post("/notes", "alice", "notes.form", "editor")
	r.Header.Set("Origin", "https://evil.example")
	assert.Equal(t, http.StatusForbidden, s.serve(r).Code)
	assert.Empty(t, s.log.all())
}

func TestUnknownFormAndMethods(t *testing.T) {
	s := newSite(nil)
	assert.Equal(t, http.StatusBadRequest, s.serve(post("/notes", "alice", "other.form", "editor")).Code)
	w := s.serve(request(http.MethodDelete, "/notes", "alice", nil, "editor"))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "GET, HEAD, POST", w.Header().Get("Allow"))
	w = s.serve(post("/admin", "alice", "admin.form", "admin"))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "GET, HEAD", w.Header().Get("Allow"))
}

func TestBodyLimit(t *testing.T) {
	s := newSite(nil)
	big := url.Values{FormField: {"notes.form"}, CSRFField: {token("alice")}, "x": {strings.Repeat("a", maxBodyBytes)}}
	r := request(http.MethodPost, "/notes", "alice", big, "editor")
	assert.Equal(t, http.StatusRequestEntityTooLarge, s.serve(r).Code)
	r = request(http.MethodPost, "/notes", "alice", big, "editor")
	r.ContentLength = -1
	assert.Equal(t, http.StatusRequestEntityTooLarge, s.serve(r).Code, "no announced length")
	assert.Equal(t, http.StatusRequestEntityTooLarge, s.serve(upload(t, nil, maxBodyBytes)).Code, "multipart")
	assert.Equal(t, http.StatusRequestEntityTooLarge, s.serve(upload(t, url.Values{"x": {strings.Repeat("a", 20<<20)}}, 0)).Code,
		"fields over the form memory")
	assert.NotContains(t, s.log.all(), "notes.init")
}

func TestUploadedFilesAreRemoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	s := newSite(nil)
	w := s.serve(upload(t, url.Values{FormField: {"notes.form"}, CSRFField: {token("alice")}}, maxFormMemory+1))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, s.log.all(), "notes.process")
	assert.Equal(t, http.StatusForbidden, s.serve(upload(t, nil, maxFormMemory+1)).Code)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestParentRedirectsToDefaultChild(t *testing.T) {
	s := newSite(nil)
	w := s.serve(request(http.MethodGet, "/admin/", "alice", nil, "admin"))
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/admin/panel", w.Header().Get("Location"))
	assert.Equal(t, http.StatusForbidden, s.serve(request(http.MethodGet, "/admin", "bob", nil)).Code)

	s = newSite(func(rs map[string]*fakeRoute) {
		rs["/s_org"] = &fakeRoute{name: "org", children: "/panel", layout: true}
		rs["/s_org/panel"] = &fakeRoute{name: "orgpanel"}
	})
	w = s.serve(request(http.MethodGet, "/%5Cevil.example", "alice", nil))
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/%5Cevil.example/panel", w.Header().Get("Location"), "a URL parameter cannot point the redirect off the site")
}

func TestParentWithoutDefaultChildIsNotFound(t *testing.T) {
	for _, sub := range []string{"", ".", "/"} {
		s := newSite(func(rs map[string]*fakeRoute) { rs["/admin"].children = sub })
		w := s.serve(request(http.MethodGet, "/admin", "alice", nil, "admin"))
		assert.Equal(t, http.StatusNotFound, w.Code, "default child %q", sub)
		assert.Empty(t, w.Header().Get("Location"), "a parent never redirects to itself")
	}
}

func TestMatching(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) {
		rs["/note/n_id"].param = "id"
		rs["/tag/s_name"] = &fakeRoute{name: "tag", param: "name"}
	})
	assert.Equal(t, http.StatusNotFound, s.serve(request(http.MethodGet, "/nope", "alice", nil)).Code)
	assert.Equal(t, http.StatusNotFound, s.serve(request(http.MethodGet, "/note//ed", "alice", nil, "editor", "owner")).Code)

	assert.Equal(t, http.StatusOK, s.serve(request(http.MethodGet, "/note/42/ed/", "alice", nil, "editor", "owner")).Code)
	assert.Contains(t, s.log.all(), "note.param=42")
	assert.Equal(t, http.StatusOK, s.serve(request(http.MethodGet, "/note/007/ed", "alice", nil, "editor", "owner")).Code)
	assert.Equal(t, http.StatusOK, s.serve(request(http.MethodGet, "/note/9223372036854775807/ed", "alice", nil, "editor", "owner")).Code)

	for _, seg := range []string{"n_id", "x", "-1", "+1", "1.5", "1e3", "0x1f", "%EF%BC%91", "9223372036854775808"} {
		assert.Equal(t, http.StatusNotFound, s.serve(request(http.MethodGet, "/note/"+seg+"/ed", "alice", nil, "editor", "owner")).Code,
			"an n_ folder takes only digits that fit int64: %s", seg)
	}

	for _, seg := range []string{"Ada", "n_5", "s_name", "-1"} {
		assert.Equal(t, http.StatusOK, s.serve(request(http.MethodGet, "/tag/"+seg, "alice", nil)).Code, seg)
		assert.Contains(t, s.log.all(), "tag.param="+seg, "an s_ folder takes any one segment, a folder name included")
	}
}

func TestURLParamInt(t *testing.T) {
	r := &Request{params: map[string]string{"id": "42", "name": "Ada"}}
	assert.Equal(t, int64(42), r.URLParamInt("id"))
	assert.Equal(t, int64(0), r.URLParamInt("name"))
	assert.Equal(t, int64(0), r.URLParamInt("missing"))
	assert.Equal(t, "Ada", r.URLParam("name"))
}

func TestSiblingParameterFoldersPanic(t *testing.T) {
	for msg, routes := range map[string][]string{
		"/org has two parameter folders, n_a and s_b":   {"/org/n_a", "/org/s_b"},
		"/org has two parameter folders, n_id and s_id": {"/org/s_id/x", "/org/n_id"},
	} {
		func() {
			defer func() {
				err, _ := recover().(error)
				assert.Equal(t, "E-WEB-006", errs.Code(err))
				assert.ErrorContains(t, err, msg)
			}()
			New(map[string]Route{routes[0]: &fakeRoute{}, routes[1]: &fakeRoute{}}, Options{})
		}()
	}
}

func TestHookErrors(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) { rs["/home"].data = Redirect("/notes") })
	w := s.serve(request(http.MethodGet, "/home", "alice", nil))
	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/notes", w.Header().Get("Location"))

	s = newSite(func(rs map[string]*fakeRoute) { rs["/home"].name = "broken" })
	w = s.serve(request(http.MethodGet, "/home", "alice", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "render failed")
	assert.NotContains(t, w.Body.String(), "<root>")
}

func TestRequestsWithoutRunnerOrViewerAreRefused(t *testing.T) {
	s := newSite(nil)
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/home", nil)
	assert.Equal(t, http.StatusInternalServerError, s.serve(r).Code)

	r = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/home", nil)
	r = r.WithContext(withViewer(context.Background(), ""))
	assert.Equal(t, http.StatusUnauthorized, s.serve(r).Code)
	assert.Empty(t, s.log.all())
}

func TestWriteAssetsOncePerTag(t *testing.T) {
	root, child := &fakeState{}, &fakeState{}
	root.SetAssets([]string{"<a>", "<b>"})
	child.SetAssets([]string{"<b>", "<c>"})
	root.child = child
	var b strings.Builder
	require.NoError(t, root.WriteAssets(&b))
	assert.Equal(t, "<a><b><c>", b.String())
}

// A page below a gate renders without it: the gate's access rule and Guard run, its forms,
// Data, markup and assets do not.
func TestGateRendersOnlyItsOwnPage(t *testing.T) {
	gated := func(rs map[string]*fakeRoute) {
		rs["/notes"].assets = []string{"<notes.js>"}
		rs["/notes/n_id"] = &fakeRoute{name: "item"}
	}
	s := newSite(gated)
	w := s.serve(request(http.MethodGet, "/notes/7", "alice", nil, "editor"))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><item></item></root>", w.Body.String())
	assert.Equal(t, []string{"root.guard", "notes.guard", "item.guard", "root.init", "item.init", "root.data", "item.data"}, s.log.all())

	s = newSite(gated)
	w = s.serve(request(http.MethodGet, "/notes", "alice", nil, "editor"))
	require.Equal(t, http.StatusOK, w.Code, "a gate opened itself shows its own page")
	assert.Contains(t, w.Body.String(), "<notes><notes.js>")
}

// Defect 1 holds through a gate: its rules run before any hook of the pages below it.
func TestGateRulesApplyBelow(t *testing.T) {
	below := func(rs map[string]*fakeRoute) { rs["/notes/n_id"] = &fakeRoute{name: "item"} }
	s := newSite(below)
	assert.Equal(t, http.StatusForbidden, s.serve(post("/notes/7", "bob", "item.form", "viewer")).Code)
	assert.Empty(t, s.log.all())

	s = newSite(func(rs map[string]*fakeRoute) { below(rs); rs["/notes"].guard = NotFound() })
	assert.Equal(t, http.StatusNotFound, s.serve(post("/notes/7", "alice", "item.form", "editor")).Code)
	assert.Equal(t, []string{"root.guard", "notes.guard"}, s.log.all())
}

func TestGateFormIsNotOnThePageBelow(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) { rs["/notes/n_id"] = &fakeRoute{name: "item"} })
	w := s.serve(post("/notes/7", "alice", "notes.form", "editor"))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, s.log.all(), "notes.init")
	assert.NotContains(t, s.log.all(), "notes.process")
}

func TestRootGate(t *testing.T) {
	gate := func(rs map[string]*fakeRoute) { rs["/"].layout = false }
	s := newSite(gate)
	w := s.serve(request(http.MethodGet, "/home", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<home></home>", w.Body.String())
	assert.Equal(t, []string{"root.guard", "home.guard", "home.init", "home.data"}, s.log.all())

	s = newSite(gate)
	w = s.serve(request(http.MethodGet, "/", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code, "a root gate shows itself instead of redirecting")
	assert.Equal(t, "<root></root>", w.Body.String())
}

func TestLayoutWithNothingBelowRendersItself(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) { rs["/notes"].layout = true })
	w := s.serve(request(http.MethodGet, "/notes", "alice", nil, "editor"))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><notes></notes></root>", w.Body.String())
}

// A layout below a gate still renders the pages below it, and every Guard on the path runs.
func TestLayoutBelowGate(t *testing.T) {
	s := newSite(func(rs map[string]*fakeRoute) {
		rs["/a"] = &fakeRoute{name: "a"}
		rs["/a/b"] = &fakeRoute{name: "b", layout: true}
		rs["/a/b/n_id"] = &fakeRoute{name: "leaf"}
	})
	w := s.serve(request(http.MethodGet, "/a/b/5", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><b><leaf></leaf></b></root>", w.Body.String())
	assert.Equal(t, []string{"root.guard", "a.guard", "b.guard", "leaf.guard",
		"root.init", "b.init", "leaf.init", "root.data", "b.data", "leaf.data"}, s.log.all())
}

// A parameter folder can be a gate: opened itself it shows its own page, and its Guard sees the
// parameter on the pages below it.
func TestParameterFolderGate(t *testing.T) {
	gate := func(rs map[string]*fakeRoute) {
		rs["/s_org"] = &fakeRoute{name: "org", param: "org"}
		rs["/s_org/panel"] = &fakeRoute{name: "orgpanel"}
	}
	s := newSite(gate)
	w := s.serve(request(http.MethodGet, "/acme", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><org></org></root>", w.Body.String())

	s = newSite(gate)
	w = s.serve(request(http.MethodGet, "/acme/panel", "alice", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "<root><orgpanel></orgpanel></root>", w.Body.String())
	assert.Equal(t, []string{"root.guard", "org.guard", "org.param=acme", "orgpanel.guard",
		"root.init", "orgpanel.init", "root.data", "orgpanel.data"}, s.log.all())
}
