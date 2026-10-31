package outbound_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/outbound"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// fake is an OutboundService that records each request and answers with resp, or with err.
type fake struct {
	mu          sync.Mutex
	got         []*runnerv1.DoRequest
	traceparent string
	resp        *runnerv1.DoResponse
	err         error
}

func (f *fake) Do(_ context.Context, r *connect.Request[runnerv1.DoRequest]) (*connect.Response[runnerv1.DoResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, r.Msg)
	f.traceparent = r.Header().Get("traceparent")
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(f.resp), nil
}

func (f *fake) requests() []*runnerv1.DoRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

// fakeRunner returns a context whose session talks to f over a Unix socket.
func fakeRunner(t *testing.T, f *fake) context.Context {
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, runnerproto.RunnerSocket)
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewOutboundServiceHandler(f, connect.WithReadMaxBytes(runnerproto.MaxMessageBytes)))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	client := runnerv1connect.NewOutboundServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC())
	return runner.With(t.Context(), &runner.Session{Outbound: client})
}

func send(t *testing.T, ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	require.NoError(t, err)
	return outbound.Client().Do(req)
}

// sendFailing sends a request that the test expects to fail, and returns its error.
func sendFailing(t *testing.T, ctx context.Context, method, url string, body io.Reader) error {
	resp, err := send(t, ctx, method, url, body)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

func TestRoundTrip(t *testing.T) {
	f := &fake{resp: &runnerv1.DoResponse{Status: http.StatusCreated, Body: []byte("pong"), Headers: []*runnerv1.Header{
		{Name: "Content-Type", Value: "text/plain"}, {Name: "X-Echo", Value: "a"}, {Name: "X-Echo", Value: "b"},
	}}}
	sc := tracectx.SpanContext{TraceID: tracectx.NewTraceID(), SpanID: tracectx.NewSpanID(), Sampled: true}
	ctx := tracectx.With(fakeRunner(t, f), sc)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.partner.example/v1/items?q=1", strings.NewReader(`{"n":1}`))
	require.NoError(t, err)
	req.Header.Add("X-A", "1")
	req.Header.Add("X-A", "2")
	req.Header.Set("Content-Type", "application/json")

	resp, err := outbound.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "201 Created", resp.Status)
	assert.Equal(t, []string{"a", "b"}, resp.Header.Values("X-Echo"))
	assert.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
	assert.Equal(t, "pong", string(body))
	assert.Equal(t, int64(4), resp.ContentLength)
	got := f.requests()
	require.Len(t, got, 1)
	assert.Equal(t, http.MethodPost, got[0].GetMethod())
	assert.Equal(t, "https://api.partner.example/v1/items?q=1", got[0].GetUrl())
	assert.Equal(t, `{"n":1}`, string(got[0].GetBody()))
	var headers []string
	for _, h := range got[0].GetHeaders() {
		headers = append(headers, h.GetName()+": "+h.GetValue())
	}
	assert.Equal(t, []string{"Content-Type: application/json", "X-A: 1", "X-A: 2"}, headers)
	assert.Equal(t, sc.Traceparent(), f.traceparent, "the runner's span joins the request's trace")
}

func TestRedirectIsNotFollowed(t *testing.T) {
	f := &fake{resp: &runnerv1.DoResponse{Status: http.StatusFound, Headers: []*runnerv1.Header{{Name: "Location", Value: "https://other.example/"}}}}
	resp, err := send(t, fakeRunner(t, f), http.MethodGet, "https://api.partner.example/", http.NoBody)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://other.example/", resp.Header.Get("Location"))
	assert.Len(t, f.requests(), 1)
}

func TestClient(t *testing.T) {
	c := outbound.Client()
	assert.Equal(t, 30*time.Second, c.Timeout)
	require.ErrorIs(t, c.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	assert.NotSame(t, c, outbound.Client(), "each caller gets its own client")
}

func TestRefusal(t *testing.T) {
	refusal := errs.New("E-OUT-001", `"evil.example" is not in egress in aicoded.yaml`, "add the host to egress in aicoded.yaml; a host the app type does not allow needs approval")
	f := &fake{err: connect.NewError(connect.CodePermissionDenied, refusal)}
	err := sendFailing(t, fakeRunner(t, f), http.MethodGet, "https://evil.example/", http.NoBody)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, refusal, e)
}

func TestNoRunner(t *testing.T) {
	err := sendFailing(t, context.Background(), http.MethodGet, "https://api.partner.example/", http.NoBody)
	assert.Equal(t, "E-RUN-004", errs.Code(err))
}

func TestBodyOver10MiB(t *testing.T) {
	f := &fake{resp: &runnerv1.DoResponse{Status: http.StatusOK}}
	ctx := fakeRunner(t, f)
	for name, body := range map[string]io.Reader{
		"known length":   bytes.NewReader(make([]byte, runnerproto.MaxOutboundBody+1)),
		"unknown length": io.MultiReader(bytes.NewReader(make([]byte, runnerproto.MaxOutboundBody)), strings.NewReader("x")),
	} {
		err := sendFailing(t, ctx, http.MethodPost, "https://api.partner.example/", body)
		assert.Equal(t, "E-OUT-004", errs.Code(err), name)
	}
	assert.Empty(t, f.requests(), "refused before it is sent")

	resp, err := send(t, ctx, http.MethodPost, "https://api.partner.example/", bytes.NewReader(make([]byte, runnerproto.MaxOutboundBody)))
	require.NoError(t, err, "10 MiB is allowed")
	_ = resp.Body.Close()
	assert.Len(t, f.requests(), 1)
}

func TestOtherError(t *testing.T) {
	f := &fake{err: connect.NewError(connect.CodeUnavailable, errors.New("runner going away"))}
	err := sendFailing(t, fakeRunner(t, f), http.MethodGet, "https://api.partner.example/", http.NoBody)
	require.Error(t, err)
	assert.Empty(t, errs.Code(err))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}
