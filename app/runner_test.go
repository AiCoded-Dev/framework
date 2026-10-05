package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

type fakeRunner struct {
	hello    *runnerv1.HelloResponse
	helloErr error
	ready    chan struct{}
	once     sync.Once
	mu       sync.Mutex
	spans    []*runnerv1.Span
}

func newFakeRunner(t *testing.T) (*fakeRunner, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &fakeRunner{
		hello: &runnerv1.HelloResponse{App: "rooms", Env: "dev", Version: "2026.09.1", CsrfKey: bytes.Repeat([]byte{1}, 32),
			ViewerKeys: [][]byte{pub}, Settings: map[string]string{"greeting": "hi"}},
		ready: make(chan struct{}),
	}, priv
}

// serve starts f on a runner socket and returns the runner directory.
func (f *fakeRunner) serve(t *testing.T) string {
	dir := t.TempDir()
	l, err := socket.Listen(t.Context(), filepath.Join(dir, runnerproto.RunnerSocket))
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewLifecycleServiceHandler(f))
	mux.Handle(runnerv1connect.NewTelemetryServiceHandler(f))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return dir
}

func (f *fakeRunner) Hello(context.Context, *connect.Request[runnerv1.HelloRequest]) (*connect.Response[runnerv1.HelloResponse], error) {
	if f.helloErr != nil {
		return nil, f.helloErr
	}
	return connect.NewResponse(f.hello), nil
}

func (f *fakeRunner) Ready(context.Context, *connect.Request[runnerv1.ReadyRequest]) (*connect.Response[runnerv1.ReadyResponse], error) {
	f.once.Do(func() { close(f.ready) })
	return connect.NewResponse(&runnerv1.ReadyResponse{}), nil
}

func (f *fakeRunner) Export(_ context.Context, r *connect.Request[runnerv1.ExportRequest]) (*connect.Response[runnerv1.ExportResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, r.Msg.GetSpans()...)
	return connect.NewResponse(&runnerv1.ExportResponse{}), nil
}

func (f *fakeRunner) exported() []*runnerv1.Span {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.spans)
}

func TestConnectRunner(t *testing.T) {
	f, _ := newFakeRunner(t)
	dir := f.serve(t)
	c, err := connectRunner(t.Context(), dir)
	require.NoError(t, err)
	assert.Len(t, c.keys, 1)
	assert.Equal(t, map[string]string{"greeting": "hi"}, c.session.Settings)
	assert.Equal(t, dir, c.session.Dir)
	assert.NotNil(t, c.session.Files)
	assert.NotNil(t, c.session.Mail)
	assert.NotNil(t, c.session.Rpc)

	_, err = c.telemetry.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "boot"}}}))
	require.NoError(t, err)
	spans := f.exported()
	require.Len(t, spans, 1)
	assert.Equal(t, "boot", spans[0].GetName())

	ctx := runner.With(context.Background(), c.session)
	assert.Equal(t, "rooms", Name(ctx))
	assert.Equal(t, "dev", Env(ctx))
	assert.Equal(t, "2026.09.1", Version(ctx))
	assert.Empty(t, Version(context.Background()))
}

func TestConnectRunnerRefusesOversizedResponses(t *testing.T) {
	f, _ := newFakeRunner(t)
	f.hello.Settings["greeting"] = strings.Repeat("x", runnerproto.MaxMessageBytes)
	_, err := connectRunner(t.Context(), f.serve(t))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
}

func TestConnectRunnerFails(t *testing.T) {
	_, err := connectRunner(t.Context(), "")
	assert.Equal(t, "E-RUN-001", errs.Code(err))
	require.ErrorContains(t, err, "run it by hand with the command aicoded dev --manual <app> prints")

	f, _ := newFakeRunner(t)
	f.hello.CsrfKey = []byte("short")
	_, err = connectRunner(t.Context(), f.serve(t))
	assert.Equal(t, "E-RUN-003", errs.Code(err))
	require.ErrorContains(t, err, fmt.Sprintf("runner protocol %d", runnerproto.ProtocolVersion))

	f, _ = newFakeRunner(t)
	f.hello.ViewerKeys = [][]byte{[]byte("not a key")}
	_, err = connectRunner(t.Context(), f.serve(t))
	assert.Equal(t, "E-RUN-003", errs.Code(err))

	f, _ = newFakeRunner(t)
	f.helloErr = connect.NewError(connect.CodeFailedPrecondition, errors.New("E-RUN-002: protocol mismatch"))
	_, err = connectRunner(t.Context(), f.serve(t))
	require.ErrorContains(t, err, "E-RUN-002")
}
