package rpc

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/rpc/wire"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// serve serves Server() the way the app does after its middleware has checked a call token:
// to the calling app caller, as the viewer v.
func serve(t *testing.T, caller string, v identity.Identity) runnerv1connect.AppServiceClient {
	_, h := Server().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := runner.With(r.Context(), &runner.Session{App: "billing"})
		ctx = identity.With(identity.WithCaller(ctx, caller), v)
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)
	return runnerv1connect.NewAppServiceClient(srv.Client(), srv.URL)
}

func call(t *testing.T, c runnerv1connect.AppServiceClient, method string, payload []byte) ([]byte, error) {
	res, err := c.Serve(t.Context(), connect.NewRequest(&runnerv1.ServeRequest{Method: method, Payload: payload}))
	if err != nil {
		return nil, err
	}
	return res.Msg.Payload, nil
}

func TestServe(t *testing.T) {
	editor := identity.Identity{Subject: "u1", Roles: []string{"editor"}}
	in := GetIn{ID: 7}
	b, err := call(t, serve(t, "shop", editor), "Get", in.aicodedAppend(nil))
	require.NoError(t, err)
	var out GetOut
	require.NoError(t, wire.Decode(b, out.aicodedDecode))
	assert.Equal(t, GetOut{Number: "INV-7", Lines: []Line{{Amount: 9.5}}}, out)

	_, err = call(t, serve(t, "shop", editor), "Get", []byte{0xff})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "input that does not decode")
}

func TestServeChecksTheTable(t *testing.T) {
	for name, c := range map[string]struct {
		caller, method string
		viewer         identity.Identity
	}{
		"a caller Get does not list":   {"reports", "Get", identity.Identity{Subject: "u1", Roles: []string{"admin"}}},
		"a viewer without the role":    {"shop", "Get", identity.Identity{Subject: "u1"}},
		"no viewer, without apps=true": {"shop", "Get", identity.Identity{}},
		"a viewer, without role=":      {"shop", "Ping", identity.Identity{Subject: "u1", Roles: []string{"admin"}}},
	} {
		_, err := call(t, serve(t, c.caller, c.viewer), c.method, nil)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), name)
	}
	_, err := call(t, serve(t, "reports", identity.Identity{}), "Ping", nil)
	require.NoError(t, err, "no viewer, with apps=true")
}
