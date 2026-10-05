package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestRedirectAcceptsOnlyLocalPaths(t *testing.T) {
	for _, p := range []string{"/", "/notes/42", "/find?q=a#top", "/a:b"} {
		var rd *redirect
		require.ErrorAs(t, Redirect(p), &rd, p)
	}
	for _, p := range []string{"", "notes", "//evil.example", "/\\evil.example", "https://evil.example/", "javascript:alert(1)",
		"/a\\b", "/a\nb", "/a\x00", " /x", "/\x7f"} {
		assert.Equal(t, "E-WEB-002", errs.Code(Redirect(p)), "%q", p)
	}
}

func TestFail(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	cases := []struct {
		err    error
		status int
		body   string
	}{
		{Error(http.StatusTeapot, "<b>short</b>"), http.StatusTeapot, "&lt;b&gt;short&lt;/b&gt;"},
		{NotFound(), http.StatusNotFound, "This page does not exist."},
		{Forbidden(), http.StatusForbidden, "You do not have access to this page."},
		{errors.New("db password is hunter2"), http.StatusInternalServerError, "Something went wrong."},
		{Error(200, "not an error"), http.StatusInternalServerError, "Something went wrong."},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		fail(w, r, c.err)
		assert.Equal(t, c.status, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, csp, w.Header().Get("Content-Security-Policy"))
		assert.Contains(t, w.Body.String(), c.body)
		assert.NotContains(t, w.Body.String(), "hunter2")
	}

	w := httptest.NewRecorder()
	fail(w, r, Redirect("/notes/1"))
	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/notes/1", w.Header().Get("Location"))
	assert.Equal(t, csp, w.Header().Get("Content-Security-Policy"))
}

func TestSetHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Security-Policy", "default-src *")
	setHeaders(h)
	assert.Equal(t, "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; "+
		"connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'", h.Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "same-origin", h.Get("Referrer-Policy"))
	assert.Equal(t, "same-origin", h.Get("Cross-Origin-Opener-Policy"))
	assert.Equal(t, "same-origin", h.Get("Cross-Origin-Resource-Policy"))
}
