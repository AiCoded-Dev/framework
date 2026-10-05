package rpc_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/viewerauth"
	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/rpc/wire"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

var (
	editor = viewer.Claims{Subject: "alice", Roles: []string{"editor"}, Caller: "shop"}
	nobody = viewer.Claims{Subject: "bob", Caller: "shop"}
	asShop = viewer.Claims{Caller: "shop", App: true}
)

// billing serves methods the way an app does: behind the real middleware, as the app billing.
type billing struct {
	client runnerv1connect.AppServiceClient
	key    ed25519.PrivateKey
}

func serveBilling(t *testing.T, methods ...rpc.Method) *billing {
	return serveBillingIn(t, "", methods...)
}

// serveBillingIn is serveBilling in the runner's environment env.
func serveBillingIn(t *testing.T, env string, methods ...rpc.Method) *billing {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	path, h := rpc.NewServer(methods...).Handler()
	sock := filepath.Join(t.TempDir(), "app.sock")
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)
	srv := socket.NewServer(viewerauth.Middleware([]ed25519.PublicKey{pub}, "billing", path, h))
	srv.BaseContext = func(net.Listener) context.Context {
		return runner.With(context.Background(), &runner.Session{App: "billing", Env: env})
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return &billing{client: runnerv1connect.NewAppServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC()), key: priv}
}

// call calls method as the claims say, with a token the runner would mint.
func (b *billing) call(t *testing.T, c viewer.Claims, method string, in []byte) ([]byte, error) {
	now := time.Now()
	c.Audience, c.IssuedAt, c.Expires = "billing", now.Unix(), now.Add(time.Minute).Unix()
	token, err := viewer.Sign(b.key, c)
	require.NoError(t, err)
	req := connect.NewRequest(&runnerv1.ServeRequest{Method: method, Payload: in})
	req.Header().Set(viewer.Header, token)
	resp, err := b.client.Serve(t.Context(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetPayload(), nil
}

func echo(_ context.Context, in []byte) ([]byte, error) { return in, nil }

// method returns a method shop may call as a viewer with any role.
func method(name string, call func(context.Context, []byte) ([]byte, error)) rpc.Method {
	return rpc.Method{Name: name, Callers: []string{"shop"}, Require: []string{"*"}, Call: call}
}

func TestServeChecksTheCall(t *testing.T) {
	b := serveBilling(t,
		rpc.Method{Name: "GetInvoice", Callers: []string{"reports", "shop"}, Require: []string{"admin|editor"}, Call: echo},
		rpc.Method{Name: "Refund", Callers: []string{"reports"}, Require: []string{"*"}, Call: echo},
		rpc.Method{Name: "Ping", Callers: []string{"shop"}, Apps: true, Call: echo},
		rpc.Method{Name: "Audit", Callers: []string{"shop"}, Require: []string{"editor", "admin|auditor"}, Call: echo},
	)
	for name, c := range map[string]struct {
		claims viewer.Claims
		method string
		code   connect.Code
		text   string
	}{
		"unknown function":          {editor, "Nope", connect.CodeNotFound, "no such function"},
		"caller not listed":         {editor, "Refund", connect.CodePermissionDenied, "E-RPC-009"},
		"viewer without the role":   {nobody, "GetInvoice", connect.CodePermissionDenied, "the viewer may not call billing.GetInvoice"},
		"app call without apps":     {asShop, "GetInvoice", connect.CodePermissionDenied, "E-RPC-010"},
		"viewer call without role=": {editor, "Ping", connect.CodePermissionDenied, "the viewer may not call billing.Ping"},
		"viewer meeting one rule":   {editor, "Audit", connect.CodePermissionDenied, "the viewer may not call billing.Audit"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := b.call(t, c.claims, c.method, nil)
			assert.Equal(t, c.code, connect.CodeOf(err))
			require.ErrorContains(t, err, c.text)
		})
	}

	out, err := b.call(t, editor, "GetInvoice", []byte("in"))
	require.NoError(t, err)
	assert.Equal(t, []byte("in"), out)
	_, err = b.call(t, viewer.Claims{Subject: "carol", Roles: []string{"auditor", "editor"}, Caller: "shop"}, "Audit", nil)
	require.NoError(t, err, "a viewer meeting every rule")
}

func TestServeAnAppCall(t *testing.T) {
	b := serveBilling(t, rpc.Method{Name: "Ping", Callers: []string{"shop"}, Apps: true,
		Call: func(ctx context.Context, _ []byte) ([]byte, error) {
			v := auth.Viewer(ctx)
			return []byte(rpc.Caller(ctx) + "|" + v.Subject + "|" + v.Name), nil
		}})
	out, err := b.call(t, asShop, "Ping", nil)
	require.NoError(t, err)
	assert.Equal(t, "shop||", string(out), "the caller is known and there is no viewer")
}

func TestServeMapsErrors(t *testing.T) {
	logs := captureLogs(t)

	b := serveBilling(t,
		method("Fail", func(context.Context, []byte) ([]byte, error) { return nil, errors.New("password hunter2 refused") }),
		method("Missing", func(context.Context, []byte) ([]byte, error) { return nil, rpc.Error(rpc.NotFound, "no invoice 7") }),
		method("Hidden", func(context.Context, []byte) ([]byte, error) { return nil, rpc.Error(rpc.Internal, "hunter2") }),
		method("Panic", func(context.Context, []byte) ([]byte, error) { panic("hunter2") }),
		method("Decode", func(_ context.Context, in []byte) ([]byte, error) {
			if err := wire.Decode(in, func(protowire.Number, wire.Value) error { return nil }); err != nil {
				return nil, rpc.DecodeError(err)
			}
			return in, nil
		}),
	)
	for name, c := range map[string]struct {
		method string
		code   connect.Code
		msg    string
	}{
		"uncoded error":   {"Fail", connect.CodeInternal, "internal error"},
		"coded error":     {"Missing", connect.CodeNotFound, "no invoice 7"},
		"internal code":   {"Hidden", connect.CodeInternal, "internal error"},
		"panic":           {"Panic", connect.CodeInternal, "internal error"},
		"malformed input": {"Decode", connect.CodeInvalidArgument, "the call's input could not be decoded"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := b.call(t, editor, c.method, []byte{0x08})
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, c.code, ce.Code())
			assert.Equal(t, c.msg, ce.Message())
			assert.NotContains(t, err.Error(), "hunter2")
		})
	}
	assert.Contains(t, logs.String(), `msg="call failed" function=Fail caller=shop error=*errors.errorString`,
		"the callee logs the kind of the error it hides")
	assert.Contains(t, logs.String(), `msg="panic in function" function=Panic caller=shop value=string stack=`)
	assert.NotContains(t, logs.String(), "hunter2", "outside aicoded dev")
}

func TestServeLogsTextInDev(t *testing.T) {
	logs := captureLogs(t)
	b := serveBillingIn(t, "dev",
		method("Fail", func(context.Context, []byte) ([]byte, error) { return nil, errors.New("password hunter2 refused") }),
		method("Panic", func(context.Context, []byte) ([]byte, error) { panic("hunter2") }),
	)
	for _, name := range []string{"Fail", "Panic"} {
		_, err := b.call(t, editor, name, nil)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	}
	assert.Contains(t, logs.String(), `msg="call failed" function=Fail caller=shop error="password hunter2 refused"`)
	assert.Contains(t, logs.String(), `msg="panic in function" function=Panic caller=shop value=hunter2 stack=`)
	assert.Equal(t, 2, strings.Count(logs.String(), "level=ERROR"), "a panic is logged once")
}

func captureLogs(t *testing.T) *lockedBuffer {
	logs := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return logs
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
