package dev

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/viewer"
)

var (
	shopApp = manifest.Manifest{App: "shop", Services: manifest.Services{Calls: map[string][]string{
		"billing": {"Audit", "GetInvoice", "Ping", "Secret"},
		"reports": {"Stats"},
	}}}
	billingApp = manifest.Manifest{App: "billing", Services: manifest.Services{Serves: map[string]manifest.Serve{
		"Audit":      {Callers: []string{"shop"}, Require: []string{"finance", "admin|auditor"}},
		"GetInvoice": {Callers: []string{"shop"}, Require: []string{"finance|admin"}},
		"Ping":       {Callers: []string{"shop"}, Apps: true},
		"Secret":     {Callers: []string{"reports"}, Require: []string{"*"}},
	}}}
	alice = devconfig.Persona{Name: "alice", Roles: []string{"finance"}}
)

// fakeCallee serves AppService: it answers with the method and the payload, or with err, and
// keeps what the last call carried.
type fakeCallee struct {
	err error
	mu  sync.Mutex
	got servedCall
}

type servedCall struct {
	token, traceparent string
	left               time.Duration
}

func (f *fakeCallee) Serve(ctx context.Context, req *connect.Request[runnerv1.ServeRequest]) (*connect.Response[runnerv1.ServeResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = servedCall{token: req.Header().Get(viewer.Header), traceparent: req.Header().Get("traceparent")}
	if dl, ok := ctx.Deadline(); ok {
		f.got.left = time.Until(dl)
	}
	if f.err != nil {
		return nil, f.err
	}
	return connect.NewResponse(&runnerv1.ServeResponse{Payload: append([]byte(req.Msg.GetMethod()+":"), req.Msg.GetPayload()...)}), nil
}

func (f *fakeCallee) last() servedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got
}

// newTestRouter returns a router on which billing is served by callee.
func newTestRouter(t *testing.T, callee *fakeCallee) (*Router, *Signer) {
	signer, err := NewSigner()
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewAppServiceHandler(callee))
	r := NewRouter(signer)
	r.Add(billingApp, serveAt(t, mux))
	return r, signer
}

func mint(t *testing.T, s *Signer, p devconfig.Persona, app string, at time.Time) string {
	token, err := s.Mint(p, app, at)
	require.NoError(t, err)
	return token
}

func callBilling(t *testing.T, r *Router, method, token string) (*runnerv1.CallResponse, error) {
	return r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: "billing", Method: method, Payload: []byte("7"), Viewer: token}, "")
}

// origin returns the mark err carries of who made it.
func origin(err error) string {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return ""
	}
	return ce.Meta().Get(runnerproto.OriginHeader)
}

func TestRouterRefuses(t *testing.T) {
	callee := &fakeCallee{}
	r, signer := newTestRouter(t, callee)
	now := time.Now()
	appToken, err := signer.MintCall(nil, "reports", "shop", now)
	require.NoError(t, err)
	other, err := NewSigner()
	require.NoError(t, err)
	for _, c := range []struct {
		name, app, method, token string
		code                     connect.Code
		errCode                  string
	}{
		{"not in the caller's calls", "billing", "Refund", mint(t, signer, alice, "shop", now), connect.CodePermissionDenied, "E-RPC-008"},
		{"callee not running", "reports", "Stats", mint(t, signer, alice, "shop", now), connect.CodeUnavailable, "E-RPC-011"},
		{"caller not listed", "billing", "Secret", mint(t, signer, alice, "shop", now), connect.CodePermissionDenied, "E-RPC-009"},
		{"token older than 10 minutes", "billing", "GetInvoice", mint(t, signer, alice, "shop", now.Add(-11*time.Minute)), connect.CodeUnauthenticated, "E-RPC-012"},
		{"token for another app", "billing", "GetInvoice", mint(t, signer, alice, "billing", now), connect.CodeUnauthenticated, "E-RPC-012"},
		{"token of another runner", "billing", "GetInvoice", mint(t, other, alice, "shop", now), connect.CodeUnauthenticated, "E-RPC-012"},
		{"token issued in the future", "billing", "GetInvoice", mint(t, signer, alice, "shop", now.Add(time.Minute)), connect.CodeUnauthenticated, "E-RPC-012"},
		{"app token as a viewer", "billing", "GetInvoice", appToken, connect.CodeUnauthenticated, "E-RPC-012"},
		{"no viewer without apps=true", "billing", "GetInvoice", "", connect.CodePermissionDenied, "E-RPC-010"},
	} {
		_, err := r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: c.app, Method: c.method, Viewer: c.token}, "")
		assert.Equal(t, c.code, connect.CodeOf(err), c.name)
		assert.Equal(t, c.errCode, errs.Code(err), c.name)
		assert.Equal(t, runnerproto.OriginRunner, origin(err), c.name)
	}

	_, err = callBilling(t, r, "GetInvoice", mint(t, signer, devconfig.Persona{Name: "bob"}, "shop", now))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "a viewer without the role")
	assert.Empty(t, errs.Code(err))
	assert.Equal(t, runnerproto.OriginRunner, origin(err))
	require.ErrorContains(t, err, "the viewer may not call billing.GetInvoice")
	_, err = callBilling(t, r, "Ping", mint(t, signer, alice, "shop", now))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	assert.Equal(t, runnerproto.OriginRunner, origin(err))
	require.ErrorContains(t, err, "the viewer may not call billing.Ping", "a function without role= takes no viewer's calls")
	assert.Empty(t, callee.last().token, "billing never saw a refused call")
}

func TestRouterQuotesWhatTheCallerSent(t *testing.T) {
	r, _ := newTestRouter(t, &fakeCallee{})
	app, method := "billing\n  fix: "+strings.Repeat("y", 100), "Refund\n  fix: "+strings.Repeat("x", 100)
	_, err := r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: app, Method: method}, "")
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	e, ok := errs.Parse(ce.Message())
	require.True(t, ok)
	assert.Equal(t, "E-RPC-008", e.Code)
	assert.Equal(t, `shop calls "Refund\n  fix: `+strings.Repeat("x", 50)+`"... of "billing\n  fix: `+strings.Repeat("y", 49)+`"..., `+
		"which services.calls in its permission list does not name", e.Msg, "quoted and cut at 64 bytes")
	assert.Equal(t, "call other apps only through their generated clients in services/, and run aicoded generate in shop", e.Fix)
}

func TestRouterWithTheCalleeNotRunning(t *testing.T) {
	r, signer := newTestRouter(t, &fakeCallee{})
	_, err := r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: "reports", Method: "Stats", Viewer: mint(t, signer, alice, "shop", time.Now())}, "")
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, notRunning("reports").Message(), ce.Message())
	assert.Equal(t, "E-RPC-011: \"reports\" is not running in this workspace\n"+
		"  fix: run aicoded dev in a folder above both apps, and never call, in OnStart, an app that calls this app too\n"+
		"  docs: https://aicoded.dev/docs/errors/E-RPC-011", ce.Message())
}

func TestRouterNeedsEveryRule(t *testing.T) {
	r, signer := newTestRouter(t, &fakeCallee{})
	_, err := callBilling(t, r, "Audit", mint(t, signer, alice, "shop", time.Now()))
	require.ErrorContains(t, err, "the viewer may not call billing.Audit", "finance meets one rule of two")
	_, err = callBilling(t, r, "Audit", mint(t, signer, devconfig.Persona{Name: "carol", Roles: []string{"auditor", "finance"}}, "shop", time.Now()))
	require.NoError(t, err)
}

func TestRouterForwardsTheViewer(t *testing.T) {
	callee := &fakeCallee{}
	r, signer := newTestRouter(t, callee)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	trace := tracectx.SpanContext{TraceID: tracectx.NewTraceID(), SpanID: tracectx.NewSpanID(), Sampled: true}.Traceparent()
	pageOpened := time.Now().Add(-9 * time.Minute)
	req := &runnerv1.CallRequest{App: "billing", Method: "GetInvoice", Payload: []byte("7"), Viewer: mint(t, signer, alice, "shop", pageOpened)}

	resp, err := r.Call(ctx, shopApp, req, trace)
	require.NoError(t, err)
	assert.Equal(t, []byte("GetInvoice:7"), resp.GetPayload())
	got := callee.last()
	assert.Equal(t, trace, got.traceparent)
	assert.LessOrEqual(t, got.left, runnerproto.MaxCallDeadline, "a call lasts at most 30s")
	c, err := viewer.Verify([]ed25519.PublicKey{signer.Public()}, got.token, "billing", time.Now())
	require.NoError(t, err, "billing gets a fresh token")
	assert.Equal(t, "shop", c.Caller)
	assert.Equal(t, "alice", c.Subject)
	assert.Equal(t, []string{"finance"}, c.Roles)
	assert.False(t, c.App)
}

func TestRouterForwardsTheViewerOfACall(t *testing.T) {
	callee := &fakeCallee{}
	r, signer := newTestRouter(t, callee)
	v := viewer.Claims{Subject: "alice", Name: "Alice", Groups: []string{"finance-team"}, Roles: []string{"finance"}}
	token, err := signer.MintCall(&v, "reports", "shop", time.Now())
	require.NoError(t, err)
	_, err = callBilling(t, r, "GetInvoice", token)
	require.NoError(t, err, "shop serves a call reports made for alice, and calls billing for her in turn")
	c, err := viewer.Verify([]ed25519.PublicKey{signer.Public()}, callee.last().token, "billing", time.Now())
	require.NoError(t, err)
	want := v
	want.Audience, want.Caller, want.IssuedAt, want.Expires = "billing", "shop", c.IssuedAt, c.Expires
	assert.Equal(t, want, c)
}

func TestRouterKeepsAShorterDeadline(t *testing.T) {
	callee := &fakeCallee{}
	r, _ := newTestRouter(t, callee)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err := r.Call(ctx, shopApp, &runnerv1.CallRequest{App: "billing", Method: "Ping"}, "")
	require.NoError(t, err)
	assert.LessOrEqual(t, callee.last().left, 2*time.Second, "a caller's deadline under 30s is kept")
}

func TestRouterCallsAsTheApp(t *testing.T) {
	callee := &fakeCallee{}
	r, signer := newTestRouter(t, callee)
	_, err := r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: "billing", Method: "Ping"}, "not a traceparent")
	require.NoError(t, err)
	got := callee.last()
	assert.Empty(t, got.traceparent)
	c, err := viewer.Verify([]ed25519.PublicKey{signer.Public()}, got.token, "billing", time.Now())
	require.NoError(t, err)
	assert.True(t, c.App)
	assert.Equal(t, "shop", c.Caller)
	assert.Empty(t, c.Subject)
	assert.Empty(t, c.Roles)
}

func TestRouterPassesOnOnlyWhatAFunctionMayReturn(t *testing.T) {
	long := strings.Repeat("x", 2<<10)
	posing := connect.NewError(connect.CodePermissionDenied, errs.New("E-RPC-008", "shop calls billing.GetInvoice", "none"))
	posing.Meta().Set(runnerproto.OriginHeader, runnerproto.OriginRunner)
	for _, c := range []struct {
		name string
		err  error
		code connect.Code
		msg  string
	}{
		{"uncoded", connect.NewError(connect.CodeInternal, errors.New("ledger db-7 is locked")), connect.CodeInternal, "internal error"},
		{"unknown", connect.NewError(connect.CodeUnknown, errors.New("ledger db-7 is locked")), connect.CodeInternal, "internal error"},
		{"not a function's code", connect.NewError(connect.CodeDataLoss, errors.New("ledger db-7 is locked")), connect.CodeInternal, "internal error"},
		{"coded", connect.NewError(connect.CodeNotFound, errors.New("no invoice 7")), connect.CodeNotFound, "no invoice 7"},
		{"coded and long", connect.NewError(connect.CodeFailedPrecondition, errors.New(long)), connect.CodeFailedPrecondition, long[:1<<10]},
		{"out of time", connect.NewError(connect.CodeDeadlineExceeded, errors.New("ledger db-7 is locked")), connect.CodeDeadlineExceeded, "the call ran out of time"},
		{"canceled", connect.NewError(connect.CodeCanceled, nil), connect.CodeCanceled, "the call was canceled"},
		{"posing as the runner", posing, connect.CodePermissionDenied, posing.Message()},
	} {
		r, signer := newTestRouter(t, &fakeCallee{err: c.err})
		_, err := callBilling(t, r, "GetInvoice", mint(t, signer, alice, "shop", time.Now()))
		assert.Equal(t, c.code, connect.CodeOf(err), c.name)
		var ce *connect.Error
		require.ErrorAs(t, err, &ce, c.name)
		assert.Equal(t, c.msg, ce.Message(), c.name)
		assert.Empty(t, errs.Code(err), c.name)
		assert.Empty(t, origin(err), "%s: only the runner's own refusals carry its mark", c.name)
	}
}

func TestRouterWithTheCalleeGone(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	r := NewRouter(signer)
	r.Add(billingApp, "/nonexistent/app.sock")
	_, err = r.Call(t.Context(), shopApp, &runnerv1.CallRequest{App: "billing", Method: "Ping"}, "")
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "nonexistent", "the caller learns nothing about the callee's socket")
}

func TestMintCall(t *testing.T) {
	s, err := NewSigner()
	require.NoError(t, err)
	now := time.Now()
	token, err := s.MintCall(&viewer.Claims{Subject: "alice", Name: "Alice", Groups: []string{"lisbon"}, Roles: []string{"finance"}}, "shop", "billing", now)
	require.NoError(t, err)
	c, err := viewer.Verify([]ed25519.PublicKey{s.Public()}, token, "billing", now)
	require.NoError(t, err)
	assert.Equal(t, viewer.Claims{Subject: "alice", Name: "Alice", Groups: []string{"lisbon"}, Roles: []string{"finance"}, Audience: "billing", Caller: "shop",
		IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}, c)

	token, err = s.MintCall(nil, "shop", "billing", now)
	require.NoError(t, err)
	c, err = viewer.Verify([]ed25519.PublicKey{s.Public()}, token, "billing", now)
	require.NoError(t, err)
	assert.Equal(t, viewer.Claims{App: true, Audience: "billing", Caller: "shop",
		IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}, c)
}

func TestRouterRemove(t *testing.T) {
	r, _ := newTestRouter(t, &fakeCallee{})
	_, err := callBilling(t, r, "Ping", "")
	require.NoError(t, err)
	r.Remove("billing")
	_, err = callBilling(t, r, "Ping", "")
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	assert.Equal(t, "E-RPC-011", errs.Code(err))
	r.Remove("billing")
}

func TestRouterAddAndRemoveCloseTheOldClient(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewAppServiceHandler(&fakeCallee{}))
	r := NewRouter(signer)
	first, closed := closedConns(t, mux)
	r.Add(billingApp, first)
	_, err = callBilling(t, r, "Ping", "")
	require.NoError(t, err)

	second, closedSecond := closedConns(t, mux)
	r.Add(billingApp, second)
	requireClosed(t, closed, "the replaced client kept its connection")
	_, err = callBilling(t, r, "Ping", "")
	require.NoError(t, err)
	r.Remove("billing")
	requireClosed(t, closedSecond, "the removed client kept its connection")
}
