package dev

import (
	"crypto/ed25519"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
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
	"aicoded.dev/framework/runnerproto/server"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

var (
	rooms       = manifest.Manifest{App: "rooms", Settings: []string{"greeting"}, Secrets: []string{"api_key"}}
	roomsValues = devconfig.AppValues{Settings: map[string]string{"greeting": "hi", "extra": "x"}, Secrets: map[string]string{"api_key": "s3cret", "other": "y"}}
)

// serveRooms serves rooms as aicoded dev does and returns a client of runner.sock, the store of
// its spans and mail, and the server.
func serveRooms(t *testing.T) (*http.Client, *store, *server.Server) {
	st := newStore("rooms", io.Discard)
	hc, srv, _ := serveDev(t, rooms, roomsValues, st, nil)
	return hc, st, srv
}

func TestHello(t *testing.T) {
	hc, _, _ := serveRooms(t)
	c := runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, connect.WithGRPC())
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
	hc, _, _ := serveRooms(t)
	c := runnerv1connect.NewSecretsServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	resp, err := c.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "api_key"}))
	require.NoError(t, err)
	assert.Equal(t, []byte("s3cret"), resp.Msg.GetValue())

	_, err = c.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "other"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "undeclared secrets are never handed out")
}

func TestFilesOnTheRunnerSocket(t *testing.T) {
	stat := func(hc *http.Client) error {
		c := runnerv1connect.NewFilesServiceClient(hc, socket.BaseURL, connect.WithGRPC())
		_, err := c.Stat(t.Context(), connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "."}))
		return err
	}
	hc, _, _ := serveRooms(t)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(stat(hc)), "a store the app does not declare")

	m := rooms
	m.Data = []manifest.Data{{Source: "filestore:docs", Classes: []string{"internal"}}}
	hc, _, _ = serveDev(t, m, roomsValues, newStore("rooms", io.Discard), nil)
	require.NoError(t, stat(hc))
}

func TestRunnerRefusesOversizedMessages(t *testing.T) {
	hc, st, _ := serveRooms(t)
	tc := runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	big := &runnerv1.Span{Name: "a", Attributes: map[string]string{"k": strings.Repeat("x", runnerproto.MaxMessageBytes)}}
	_, err := tc.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{big}}))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	assert.Empty(t, st.spans.All())
}

func TestExportAndReady(t *testing.T) {
	hc, st, srv := serveRooms(t)
	tc := runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	_, err := tc.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "a"}}, Dropped: 3}))
	require.NoError(t, err)
	assert.Len(t, st.spans.All(), 1)
	assert.Equal(t, uint64(3), st.exportDropped.Load())

	lc := runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	for range 2 {
		_, err := lc.Ready(t.Context(), connect.NewRequest(&runnerv1.ReadyRequest{}))
		require.NoError(t, err)
	}
	select {
	case <-srv.Ready():
	default:
		t.Fatal("not ready")
	}
}

func TestDevValuesHandOutOnlyDeclaredNames(t *testing.T) {
	v := devValues{m: rooms, v: roomsValues}
	got, err := v.Secret(t.Context(), "api_key")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", got)
	_, err = v.Secret(t.Context(), "other")
	require.Error(t, err, "a secret the app does not declare")
	assert.Equal(t, map[string]string{"greeting": "hi"}, v.Settings())
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
	call := func(m manifest.Manifest, r *server.Router) (*connect.Response[runnerv1.CallResponse], error) {
		hc, _, _ := serveDev(t, m, devconfig.AppValues{}, newStore(m.App, io.Discard), r)
		c := runnerv1connect.NewRpcServiceClient(hc, socket.BaseURL, connect.WithGRPC())
		req := connect.NewRequest(&runnerv1.CallRequest{App: "billing", Method: "Ping", Payload: []byte("7")})
		req.Header().Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		return c.Call(t.Context(), req)
	}
	callee := &fakeCallee{}
	r, signer := newTestRouter(t, callee)
	_, err := call(shopApp, nil)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err), "no router, no calls")
	assert.Equal(t, runnerproto.OriginRunner, origin(err))
	_, routerErr := server.NewRouter(signer).Call(t.Context(), shopApp, &runnerv1.CallRequest{App: "billing", Method: "Ping"}, "")
	var ce, router *connect.Error
	require.ErrorAs(t, err, &ce)
	require.ErrorAs(t, routerErr, &router)
	assert.Equal(t, router.Message(), ce.Message(), "the same refusal as the router's")

	resp, err := call(shopApp, r)
	require.NoError(t, err)
	assert.Equal(t, []byte("Ping:7"), resp.Msg.GetPayload())
	assert.Equal(t, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", callee.last().traceparent)

	_, err = call(manifest.Manifest{App: "reports"}, r)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.ErrorContains(t, err, "E-RPC-008", "the caller is the app on the socket, whatever the request says")
	assert.Equal(t, runnerproto.OriginRunner, origin(err), "the runner's mark reaches the app")
}

func TestOutboundDialsOnlyCheckedAddresses(t *testing.T) {
	var reached atomic.Bool
	opts := fakeService(t, "10.0.0.7", func(http.ResponseWriter, *http.Request) { reached.Store(true) })
	m := manifest.Manifest{App: "rooms", Egress: []manifest.Egress{{Host: "api.example.com"}}}
	hc, _, _ := serveDevEgress(t, m, devconfig.AppValues{}, newStore("rooms", io.Discard), nil, opts)
	c := runnerv1connect.NewOutboundServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	_, err := c.Do(t.Context(), connect.NewRequest(&runnerv1.DoRequest{Method: http.MethodGet, Url: "https://api.example.com/"}))
	require.ErrorContains(t, err, "E-OUT-003", "a host that resolves to a private address")
	assert.False(t, reached.Load())
}
