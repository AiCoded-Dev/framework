package rpc_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// fakeRunner answers calls with the payload prefixed by "out:", or with err, or waits for the
// call's context to end.
type fakeRunner struct {
	err   error
	block bool

	mu       sync.Mutex
	req      *runnerv1.CallRequest
	header   http.Header
	deadline time.Duration
	spans    []*runnerv1.Span
}

func (f *fakeRunner) Call(ctx context.Context, req *connect.Request[runnerv1.CallRequest]) (*connect.Response[runnerv1.CallResponse], error) {
	f.mu.Lock()
	f.req, f.header = req.Msg, req.Header().Clone()
	if d, ok := ctx.Deadline(); ok {
		f.deadline = time.Until(d)
	}
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&runnerv1.CallResponse{Payload: append([]byte("out:"), req.Msg.GetPayload()...)}), nil
}

// shop returns a context of the app shop whose runner is f.
func shop(t *testing.T, f *fakeRunner) context.Context {
	sock := filepath.Join(t.TempDir(), "runner.sock")
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewRpcServiceHandler(f))
	srv := socket.NewServer(mux)
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	c := runnerv1connect.NewRpcServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC())
	export := func(s *runnerv1.Span) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.spans = append(f.spans, s)
	}
	return runner.With(t.Context(), &runner.Session{App: "shop", Rpc: c, Export: export})
}

// spanError returns the error of the last span the app handed to f.
func (f *fakeRunner) spanError() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spans[len(f.spans)-1].GetError()
}

func TestCall(t *testing.T) {
	f := &fakeRunner{}
	sc, ok := tracectx.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.True(t, ok)
	ctx := tracectx.With(identity.WithToken(shop(t, f), "v1.viewer"), sc)

	out, err := rpc.Call(ctx, "billing", "GetInvoice", []byte("in"))
	require.NoError(t, err)
	assert.Equal(t, []byte("out:in"), out)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, "billing", f.req.GetApp())
	assert.Equal(t, "GetInvoice", f.req.GetMethod())
	assert.Equal(t, []byte("in"), f.req.GetPayload())
	assert.Equal(t, "v1.viewer", f.req.GetViewer())
	assert.True(t, strings.HasPrefix(f.header.Get("traceparent"), "00-4bf92f3577b34da6a3ce929d0e0e4736-"), "the call continues the trace")
	assert.InDelta(t, 10*time.Second, f.deadline, float64(time.Second), "a call with no deadline gets 10s")
}

func TestCallAsTheApp(t *testing.T) {
	f := &fakeRunner{}
	_, err := rpc.Call(shop(t, f), "billing", "Ping", nil)
	require.NoError(t, err)
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Empty(t, f.req.GetViewer())
}

// refusal returns the error the runner refuses a call with when marked, and the same text
// passed on from a called app otherwise.
func refusal(marked bool) error {
	err := connect.NewError(connect.CodePermissionDenied,
		errs.New("E-RPC-008", "shop does not call billing.GetInvoice", "run aicoded generate in shop"))
	if marked {
		err.Meta().Set(runnerproto.OriginHeader, runnerproto.OriginRunner)
	}
	return err
}

func TestCallErrors(t *testing.T) {
	f := &fakeRunner{err: refusal(true)}
	_, err := rpc.Call(shop(t, f), "billing", "GetInvoice", nil)
	assert.Equal(t, "E-RPC-008", errs.Code(err))
	assert.Equal(t, rpc.PermissionDenied, rpc.CodeOf(err))
	assert.Equal(t, "rpc permission_denied: E-RPC-008", redact.Error(err))
	assert.Equal(t, "rpc permission_denied: E-RPC-008", f.spanError())

	f = &fakeRunner{err: refusal(false)}
	_, err = rpc.Call(shop(t, f), "billing", "GetInvoice", nil)
	assert.Empty(t, errs.Code(err), "a called app cannot pose as the runner")
	assert.Equal(t, rpc.PermissionDenied, rpc.CodeOf(err))
	require.ErrorContains(t, err, "E-RPC-008", "its text stays as it is")
	assert.Equal(t, "rpc permission_denied", f.spanError(), "nor in the span")

	f = &fakeRunner{err: connect.NewError(connect.CodeNotFound, errors.New("no invoice 7"))}
	_, err = rpc.Call(shop(t, f), "billing", "GetInvoice", nil)
	require.EqualError(t, err, "no invoice 7")
	assert.Equal(t, rpc.NotFound, rpc.CodeOf(err))
	assert.Empty(t, errs.Code(err))
	assert.Equal(t, "rpc not_found", f.spanError())

	f = &fakeRunner{err: connect.NewError(connect.CodeDataLoss, errors.New("lost"))}
	_, err = rpc.Call(shop(t, f), "billing", "GetInvoice", nil)
	assert.Equal(t, rpc.Internal, rpc.CodeOf(err), "a code the caller may not see")

	ctx, cancel := context.WithTimeout(shop(t, &fakeRunner{block: true}), 50*time.Millisecond)
	defer cancel()
	_, err = rpc.Call(ctx, "billing", "GetInvoice", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	_, err = rpc.Call(context.Background(), "billing", "GetInvoice", nil)
	assert.Equal(t, "E-RUN-004", errs.Code(err))
}

func TestCallCutShort(t *testing.T) {
	for _, c := range []struct {
		code connect.Code
		is   error
		msg  string
	}{
		{connect.CodeDeadlineExceeded, context.DeadlineExceeded, "the call ran out of time"},
		{connect.CodeCanceled, context.Canceled, "the call was canceled"},
	} {
		_, err := rpc.Call(shop(t, &fakeRunner{err: connect.NewError(c.code, nil)}), "billing", "GetInvoice", nil)
		require.ErrorIs(t, err, c.is)
		require.EqualError(t, err, c.msg, "a call the runner cut short says so")
	}
}

func TestCallKeepsTheCallersDeadline(t *testing.T) {
	for _, d := range []time.Duration{2 * time.Second, 20 * time.Second} {
		f := &fakeRunner{}
		ctx, cancel := context.WithTimeout(shop(t, f), d)
		_, err := rpc.Call(ctx, "billing", "GetInvoice", nil)
		cancel()
		require.NoError(t, err)
		f.mu.Lock()
		assert.InDelta(t, d, f.deadline, float64(time.Second), "a deadline of %v is kept, not replaced by 10s", d)
		f.mu.Unlock()
	}
}
