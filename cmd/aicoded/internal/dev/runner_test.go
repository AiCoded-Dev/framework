package dev

import (
	"crypto/ed25519"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

func newService() *runnerService {
	return newRunnerService(
		manifest.Manifest{App: "rooms", Settings: []string{"greeting"}, Secrets: []string{"api_key"}},
		devconfig.AppValues{Settings: map[string]string{"greeting": "hi", "extra": "x"}, Secrets: map[string]string{"api_key": "s3cret", "other": "y"}},
		devconfig.EnvPreview, []byte(strings.Repeat("k", 32)), [][]byte{make([]byte, ed25519.PublicKeySize)}, newStore("rooms", io.Discard))
}

func serveRunner(t *testing.T, svc *runnerService) *http.Client {
	return serveSocket(t, svc.handler())
}

func TestHello(t *testing.T) {
	c := runnerv1connect.NewLifecycleServiceClient(serveRunner(t, newService()), socket.BaseURL, connect.WithGRPC())
	resp, err := c.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	assert.Equal(t, "rooms", resp.Msg.GetApp())
	assert.Equal(t, "preview", resp.Msg.GetEnv())
	assert.Equal(t, map[string]string{"greeting": "hi"}, resp.Msg.GetSettings(), "only declared settings")

	for _, v := range []uint32{2, 99} {
		_, err = c.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: v}))
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		require.ErrorContains(t, err, "E-RUN-002")
	}
}

func TestSecrets(t *testing.T) {
	c := runnerv1connect.NewSecretsServiceClient(serveRunner(t, newService()), socket.BaseURL, connect.WithGRPC())
	resp, err := c.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "api_key"}))
	require.NoError(t, err)
	assert.Equal(t, []byte("s3cret"), resp.Msg.GetValue())

	_, err = c.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "other"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "undeclared secrets are never handed out")
}

func TestFilesOnTheRunnerSocket(t *testing.T) {
	stat := func(svc *runnerService) error {
		c := runnerv1connect.NewFilesServiceClient(serveRunner(t, svc), socket.BaseURL, connect.WithGRPC())
		_, err := c.Stat(t.Context(), connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "."}))
		return err
	}
	assert.Equal(t, connect.CodeUnimplemented, connect.CodeOf(stat(newService())), "no file service, no handler")

	svc := newService()
	files, err := newFileService(t.TempDir(), []string{"docs"})
	require.NoError(t, err)
	t.Cleanup(files.close)
	svc.files = files
	require.NoError(t, stat(svc))
}

func TestRunnerRefusesOversizedMessages(t *testing.T) {
	svc := newService()
	tc := runnerv1connect.NewTelemetryServiceClient(serveRunner(t, svc), socket.BaseURL, connect.WithGRPC())
	big := &runnerv1.Span{Name: "a", Attributes: map[string]string{"k": strings.Repeat("x", runnerproto.MaxMessageBytes)}}
	_, err := tc.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{big}}))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Empty(t, svc.store.spans.All())
}

func TestExportAndReady(t *testing.T) {
	svc := newService()
	hc := serveRunner(t, svc)
	tc := runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	_, err := tc.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "a"}}, Dropped: 3}))
	require.NoError(t, err)
	assert.Len(t, svc.store.spans.All(), 1)
	assert.Equal(t, uint64(3), svc.dropped.Load())

	lc := runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	for range 2 {
		_, err := lc.Ready(t.Context(), connect.NewRequest(&runnerv1.ReadyRequest{}))
		require.NoError(t, err)
	}
	select {
	case <-svc.ready:
	default:
		t.Fatal("not ready")
	}
}

func TestSigner(t *testing.T) {
	s, err := NewSigner()
	require.NoError(t, err)
	token, err := s.Mint(devconfig.Persona{Name: "alice", Roles: []string{"hotel-ops"}}, "rooms", time.Now())
	require.NoError(t, err)
	c, err := viewer.Verify([]ed25519.PublicKey{s.Public()}, token, "rooms", time.Now())
	require.NoError(t, err)
	assert.Equal(t, "alice", c.Subject)
	assert.Equal(t, []string{"hotel-ops"}, c.Roles)
}

func TestRing(t *testing.T) {
	r := NewRing[int](3)
	r.Add(0)
	r.Add(1)
	assert.Equal(t, []int{0, 1}, r.All())
	for i := 2; i < 5; i++ {
		r.Add(i)
	}
	assert.Equal(t, []int{2, 3, 4}, r.All())
}

func TestCallOnTheRunnerSocket(t *testing.T) {
	call := func(svc *runnerService, method string) (*connect.Response[runnerv1.CallResponse], error) {
		c := runnerv1connect.NewRpcServiceClient(serveRunner(t, svc), socket.BaseURL, connect.WithGRPC())
		req := connect.NewRequest(&runnerv1.CallRequest{App: "billing", Method: method, Payload: []byte("7")})
		req.Header().Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		return c.Call(t.Context(), req)
	}
	_, err := call(newService(), "Ping")
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), "no router, no calls")
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, notRunning("billing").Message(), ce.Message(), "the same refusal as the router's")
	assert.Equal(t, runnerproto.OriginRunner, origin(err))

	callee := &fakeCallee{}
	r, _ := newTestRouter(t, callee)
	shop := newRunnerService(shopApp, devconfig.AppValues{}, devconfig.EnvDev, nil, nil, newStore("shop", io.Discard))
	shop.router = r
	resp, err := call(shop, "Ping")
	require.NoError(t, err)
	assert.Equal(t, []byte("Ping:7"), resp.Msg.GetPayload())
	assert.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", callee.last().traceparent)

	reports := newRunnerService(manifest.Manifest{App: "reports"}, devconfig.AppValues{}, devconfig.EnvDev, nil, nil, newStore("reports", io.Discard))
	reports.router = r
	_, err = call(reports, "Ping")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.ErrorContains(t, err, "E-RPC-008", "the caller is the app on the socket, whatever the request says")
	assert.Equal(t, runnerproto.OriginRunner, origin(err), "the runner's mark reaches the app")
}
