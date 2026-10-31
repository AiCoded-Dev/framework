package dev

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/egress"
	"aicoded.dev/framework/runnerproto/server"
	"aicoded.dev/framework/runnerproto/socket"
)

const socketBaseURL = socket.BaseURL

var helloValues = devconfig.AppValues{Settings: map[string]string{"greeting": "hi"}, Secrets: map[string]string{"token": "abc"}}

func requireGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

func helloDir(t *testing.T) string {
	dir, err := filepath.Abs(filepath.Join("testdata", "hello"))
	require.NoError(t, err)
	return dir
}

func helloManifest(t *testing.T) manifest.Manifest {
	m, err := manifest.Load(filepath.Join(helloDir(t), manifest.FileName))
	require.NoError(t, err)
	return m
}

// shortDir returns a new folder whose socket paths fit a Unix socket address.
func shortDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// serveDev serves the app of m in the environment preview through the runner server with the
// providers of aicoded dev: its file stores in a new folder, which it returns, the mail and spans
// of st, the values v and the calls of router, which may be nil. It returns a client of
// runner.sock and the server.
func serveDev(t *testing.T, m manifest.Manifest, v devconfig.AppValues, st *store, router *server.Router) (*http.Client, *server.Server, string) {
	return serveDevEgress(t, m, v, st, router, egress.Options{})
}

// serveDevEgress is serveDev with an egress client of o.
func serveDevEgress(t *testing.T, m manifest.Manifest, v devconfig.AppValues, st *store, router *server.Router, o egress.Options) (*http.Client, *server.Server, string) {
	dir := shortDir(t)
	filesDir := filepath.Join(dir, "files")
	files, err := newFileStore(filesDir, m.Stores())
	require.NoError(t, err)
	t.Cleanup(files.close)
	srv, err := server.New(t.Context(), server.Options{Dir: dir, Manifest: m, Env: server.EnvPreview,
		ViewerKeys: []ed25519.PublicKey{make([]byte, ed25519.PublicKeySize)}, CSRFKey: newCSRFKey(),
		Providers: providers(m, files, st, v, router, egress.New(o))})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	return socket.Client(filepath.Join(dir, runnerproto.RunnerSocket)), srv, filesDir
}

// serveAt serves h on a Unix socket in a short temp dir and returns the socket's path.
func serveAt(t *testing.T, h http.Handler) string { return serveAtWith(t, h, nil) }

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// invoiceSnapshot is a snapshot of billing, which serves GetInvoice.
const invoiceSnapshot = `{
  "app": "billing",
  "methods": {
    "GetInvoice": {"in": "InvoiceID", "out": "Invoice"}
  },
  "types": {
    "Invoice": {"fields": {"ID": {"number": 1, "type": "int64"}, "Total": {"number": 2, "type": "int64"}}},
    "InvoiceID": {"fields": {"ID": {"number": 1, "type": "int64"}}}
  }
}
`

const ledgerSnapshot = `{"app": "ledger", "methods": {"Post": {"in": "Entry", "out": "Entry"}}, "types": {"Entry": {"fields": {"ID": {"number": 1, "type": "int64"}}}}}`

func writeFile(t *testing.T, path, data string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

// serveAtWith serves h like serveAt, with state as the server's ConnState hook.
func serveAtWith(t *testing.T, h http.Handler, state func(net.Conn, http.ConnState)) string {
	path := filepath.Join(shortDir(t), runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), path)
	require.NoError(t, err)
	srv := socket.NewServer(h)
	srv.ConnState = state
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return path
}

// closedConns serves h like serveAt and returns the socket's path and a channel that gets a
// value when a connection to it closes.
func closedConns(t *testing.T, h http.Handler) (string, <-chan struct{}) {
	closed := make(chan struct{}, 1)
	path := serveAtWith(t, h, func(_ net.Conn, s http.ConnState) {
		if s == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	})
	return path, closed
}

// requireClosed waits until closed gets a value.
func requireClosed(t *testing.T, closed <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}

// fakeService serves h over TLS as https://api.example.com and returns options of the egress
// client under which the host resolves to addr and every dial reaches h.
func fakeService(t *testing.T, addr string, h http.HandlerFunc) egress.Options {
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	return egress.Options{
		Resolver: answer(addr),
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		},
		RootCAs: roots,
	}
}

// answer is a resolver that resolves every host to itself.
type answer string

func (a answer) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(string(a))}, nil
}
