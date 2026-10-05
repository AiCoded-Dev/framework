package socket_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

type lifecycle struct {
	runnerv1connect.UnimplementedLifecycleServiceHandler
}

func (lifecycle) Hello(context.Context, *connect.Request[runnerv1.HelloRequest]) (*connect.Response[runnerv1.HelloResponse], error) {
	return connect.NewResponse(&runnerv1.HelloResponse{App: "rooms"}), nil
}

func TestGRPCOverSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), path)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewLifecycleServiceHandler(lifecycle{}))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	c := runnerv1connect.NewLifecycleServiceClient(socket.Client(path), socket.BaseURL, connect.WithGRPC())
	resp, err := c.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	assert.Equal(t, "rooms", resp.Msg.GetApp())

	st, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}

func TestListenReplacesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), runnerproto.AppSocket)
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	l, err := socket.Listen(t.Context(), path)
	require.NoError(t, err)
	require.NoError(t, l.Close())
}

const timeout = 400 * time.Millisecond

// serve serves h with the stream timeout shortened to timeout and returns an HTTP/2 and an
// HTTP/1.1 client for it.
func serve(t *testing.T, h http.HandlerFunc) map[string]*http.Client {
	socket.SetStreamTimeout(t, timeout)
	path := filepath.Join(t.TempDir(), runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), path)
	require.NoError(t, err)
	srv := socket.NewServer(h)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	h1 := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", path)
	}}}
	return map[string]*http.Client{"HTTP/2": socket.Client(path), "HTTP/1.1": h1}
}

func post(t *testing.T, body io.Reader) *http.Request {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, socket.BaseURL, body)
	require.NoError(t, err)
	return req
}

// slowly writes "chunk" 8 times to w, waiting a quarter of the timeout after each.
func slowly(w io.Writer, flush func()) {
	for range 8 {
		_, _ = w.Write([]byte("chunk"))
		flush()
		time.Sleep(timeout / 4)
	}
}

func TestStreamThatMovesOutlivesTheTimeout(t *testing.T) {
	download := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		slowly(w, func() { _ = http.NewResponseController(w).Flush() })
	})
	upload := serve(t, func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(b)
	})
	for proto := range download {
		t.Run(proto, func(t *testing.T) {
			resp, err := download[proto].Do(post(t, nil))
			require.NoError(t, err)
			b, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err, "download")
			assert.Equal(t, strings.Repeat("chunk", 8), string(b))

			pr, pw := io.Pipe()
			go func() {
				slowly(pw, func() {})
				_ = pw.Close()
			}()
			resp, err = upload[proto].Do(post(t, pr))
			require.NoError(t, err, "upload")
			b, err = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, strings.Repeat("chunk", 8), string(b))
		})
	}
}

func TestStalledStreamIsCut(t *testing.T) {
	cut := func(t *testing.T, got <-chan error) {
		t.Helper()
		select {
		case err := <-got:
			require.Error(t, err)
		case <-time.After(20 * timeout):
			t.Fatal("the stalled stream was not cut")
		}
	}
	got := make(chan error, 1)
	upload := serve(t, func(_ http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		got <- err
	})
	download := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		chunk := make([]byte, 64<<10)
		for range 1024 {
			if _, err := w.Write(chunk); err != nil {
				got <- err
				return
			}
		}
		got <- nil
	})
	for proto := range upload {
		t.Run(proto, func(t *testing.T) {
			pr, pw := io.Pipe()
			defer func() { _ = pw.Close() }()
			go func() { _, _ = pw.Write([]byte("chunk")) }()
			req := post(t, pr)
			go func() {
				if resp, err := upload[proto].Do(req); err == nil {
					_ = resp.Body.Close()
				}
			}()
			cut(t, got)

			resp, err := download[proto].Do(post(t, nil))
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			cut(t, got)
		})
	}
}
