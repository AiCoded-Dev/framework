// Package socket connects apps and runners over Unix sockets using HTTP/2 without TLS.
package socket

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"
)

// BaseURL is the base URL for clients returned by Client; the host part is ignored.
const BaseURL = "http://runner"

// streamTimeout is how long a stream may make no progress before the server cuts it.
var streamTimeout = 60 * time.Second

// Listen creates a Unix socket at path that only its owner can use, replacing a stale one.
func Listen(ctx context.Context, path string) (net.Listener, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

// NewServer returns a server for h that speaks HTTP/1.1 and HTTP/2 without TLS and has timeouts set.
// A stream that moves in neither direction for 60 seconds is cut; one that keeps moving may run
// as long as it needs.
func NewServer(h http.Handler) *http.Server {
	var p http.Protocols
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return &http.Server{
		Handler:           progress(h, streamTimeout),
		Protocols:         &p,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       streamTimeout,
		WriteTimeout:      streamTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

// progress moves a stream's read and write deadlines d ahead before every read of the request
// body and every write or flush of the response, so the server's timeouts cut only a stream
// that stops moving.
func progress(h http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := &stream{ResponseWriter: w, rc: http.NewResponseController(w), d: d}
		if r.Body != nil && r.Body != http.NoBody {
			r.Body = &body{ReadCloser: r.Body, s: s}
		}
		h.ServeHTTP(s, r)
	})
}

type stream struct {
	http.ResponseWriter
	rc *http.ResponseController
	d  time.Duration
}

func (s *stream) extend() {
	t := time.Now().Add(s.d)
	_ = s.rc.SetReadDeadline(t)
	_ = s.rc.SetWriteDeadline(t)
}

func (s *stream) Write(p []byte) (int, error) {
	s.extend()
	return s.ResponseWriter.Write(p)
}

func (s *stream) Flush() {
	s.extend()
	_ = s.rc.Flush()
}

func (s *stream) Unwrap() http.ResponseWriter { return s.ResponseWriter }

type body struct {
	io.ReadCloser
	s *stream
}

func (b *body) Read(p []byte) (int, error) {
	b.s.extend()
	return b.ReadCloser.Read(p)
}

// Client returns an HTTP/2 client that dials the Unix socket at path.
func Client(path string) *http.Client {
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	return &http.Client{Transport: &http.Transport{
		Protocols: &p,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}}
}
