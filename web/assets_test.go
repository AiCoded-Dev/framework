package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testAssets = fstest.MapFS{
	"pages/index-ABC.js":  {Data: []byte(strings.Repeat("console.log(1);", 100))},
	"pages/index-ABC.css": {Data: []byte("body{margin:0}")},
	"img/logo-123.png":    {Data: []byte{0x89, 'P', 'N', 'G'}},
}

func getAsset(t *testing.T, h http.Handler, method, name string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), method, assetsPrefix+name, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAssets(t *testing.T) {
	h := New(nil, Options{Assets: testAssets})

	w := getAsset(t, h, http.MethodGet, "pages/index-ABC.css", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/css; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, csp, w.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "body{margin:0}", w.Body.String())
	etag := w.Header().Get("ETag")
	assert.NotEmpty(t, etag)

	w = getAsset(t, h, http.MethodGet, "pages/index-ABC.css", http.Header{"If-None-Match": {etag}})
	assert.Equal(t, http.StatusNotModified, w.Code)
	assert.Empty(t, w.Header().Get("Content-Type"))
	assert.Empty(t, w.Body.String())

	w = getAsset(t, h, http.MethodGet, "pages/index-ABC.js", http.Header{"Accept-Encoding": {"br, gzip"}})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
	zr, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	plain, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Equal(t, testAssets["pages/index-ABC.js"].Data, plain)

	w = getAsset(t, h, http.MethodGet, "pages/index-ABC.js", http.Header{"If-None-Match": {w.Header().Get("ETag")}})
	assert.Equal(t, http.StatusNotModified, w.Code)
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

	w = getAsset(t, h, http.MethodGet, "pages/index-ABC.js", http.Header{"Accept-Encoding": {"gzip; q=0.0"}})
	assert.Empty(t, w.Header().Get("Content-Encoding"), "q=0 refuses gzip")

	w = getAsset(t, h, http.MethodGet, "img/logo-123.png", http.Header{"Accept-Encoding": {"gzip"}})
	assert.Empty(t, w.Header().Get("Content-Encoding"), "images are not compressed again")
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))

	w = getAsset(t, h, http.MethodHead, "pages/index-ABC.css", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestAssetsRefuse(t *testing.T) {
	h := New(nil, Options{Assets: testAssets})
	for _, name := range []string{"missing.js", "../web/assets.go", "pages", "", "pages/../pages/index-ABC.css"} {
		assert.Equal(t, http.StatusNotFound, getAsset(t, h, http.MethodGet, name, nil).Code, name)
	}
	w := getAsset(t, h, http.MethodPost, "pages/index-ABC.css", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "GET, HEAD", w.Header().Get("Allow"))

	assert.Equal(t, http.StatusNotFound, getAsset(t, New(nil, Options{}), http.MethodGet, "pages/index-ABC.css", nil).Code)
}
