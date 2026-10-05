package billing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
	frameworkrpc "aicoded.dev/framework/rpc"
	"aicoded.dev/framework/rpcgentest/rpc"
	"aicoded.dev/framework/rpcgentest/services/billing"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// echo stands in for the runner: it hands every call to the server of billing.
type echo struct {
	billing runnerv1connect.AppServiceClient
}

func (e echo) Call(ctx context.Context, req *connect.Request[runnerv1.CallRequest]) (*connect.Response[runnerv1.CallResponse], error) {
	res, err := e.billing.Serve(ctx, connect.NewRequest(&runnerv1.ServeRequest{Method: req.Msg.Method, Payload: req.Msg.Payload}))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.CallResponse{Payload: res.Msg.Payload}), nil
}

// shop returns the context of a request the app shop serves for viewer. Its calls reach
// billing's generated server, which sees shop as the caller and the same viewer.
func shop(t *testing.T, viewer identity.Identity) context.Context {
	_, h := rpc.Server().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := runner.With(r.Context(), &runner.Session{App: "billing"})
		h.ServeHTTP(w, r.WithContext(identity.With(identity.WithCaller(ctx, "shop"), viewer)))
	}))
	t.Cleanup(srv.Close)
	s := &runner.Session{App: "shop", Rpc: echo{runnerv1connect.NewAppServiceClient(srv.Client(), srv.URL)}}
	return identity.With(runner.With(t.Context(), s), viewer)
}

func TestCall(t *testing.T) {
	ctx := shop(t, identity.Identity{Subject: "u1", Roles: []string{"admin"}})
	out, err := billing.Get(ctx, billing.GetIn{ID: 7})
	require.NoError(t, err)
	assert.Equal(t, billing.GetOut{Number: "INV-7", Lines: []billing.Line{{Amount: 9.5}}}, out)

	_, err = billing.Get(ctx, billing.GetIn{})
	assert.Equal(t, frameworkrpc.Internal, frameworkrpc.CodeOf(err), "an uncoded error of billing")
}

func TestCallAsTheApp(t *testing.T) {
	ctx := shop(t, identity.Identity{})
	_, err := billing.Ping(ctx, billing.PingIn{})
	require.NoError(t, err)
	_, err = billing.Get(ctx, billing.GetIn{ID: 7})
	assert.Equal(t, frameworkrpc.PermissionDenied, frameworkrpc.CodeOf(err))
}
