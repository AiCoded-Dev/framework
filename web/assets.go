package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// contentTypes covers what the asset build emits; anything else is served as bytes.
var contentTypes = map[string]string{
	".js":    "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

var compressible = map[string]bool{".js": true, ".css": true, ".svg": true}

type asset struct {
	data, gz []byte
	etag     string
}

// assetServer serves the app's built assets. Their names carry a content hash, so they are
// cached for good; compressible ones are gzipped once, on first use.
type assetServer struct {
	fsys  fs.FS
	mu    sync.Mutex
	files map[string]*asset
}

func newAssetServer(fsys fs.FS) *assetServer {
	return &assetServer{fsys: fsys, files: map[string]*asset{}}
}

func (a *assetServer) serve(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, r, Error(http.StatusMethodNotAllowed, "Assets can only be read."))
		return
	}
	f := a.load(name)
	if f == nil {
		fail(w, r, NotFound())
		return
	}
	h := w.Header()
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", f.etag)
	if f.gz != nil {
		h.Set("Vary", "Accept-Encoding")
	}
	if r.Header.Get("If-None-Match") == f.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	ct := contentTypes[path.Ext(name)]
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	body := f.data
	if f.gz != nil && acceptsGzip(r) {
		h.Set("Content-Encoding", "gzip")
		body = f.gz
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

// load returns the asset stored under name, or nil when there is none. Missing names are not
// remembered, so probing cannot grow the cache.
func (a *assetServer) load(name string) *asset {
	if a.fsys == nil || name == "" || !fs.ValidPath(name) {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if f, ok := a.files[name]; ok {
		return f
	}
	data, err := fs.ReadFile(a.fsys, name)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(data)
	f := &asset{data: data, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
	if compressible[path.Ext(name)] {
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		_, _ = zw.Write(data)
		_ = zw.Close()
		f.gz = b.Bytes()
	}
	a.files[name] = f
	return f
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(strings.Join(r.Header.Values("Accept-Encoding"), ","), ",") {
		enc, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(enc), "gzip") {
			continue
		}
		q, weighted := strings.CutPrefix(strings.ToLower(strings.TrimSpace(params)), "q=")
		if !weighted {
			return true
		}
		v, err := strconv.ParseFloat(q, 64)
		return err == nil && v > 0
	}
	return false
}
