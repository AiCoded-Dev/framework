package dev

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

const (
	// maxErrorMessage bounds the message of a refusal passed on from the called app.
	maxErrorMessage = 1 << 10
	// maxQuote bounds the part of a string an app sent that an error message repeats.
	maxQuote = 64
)

// Router carries calls between the apps of a workspace. It checks every call against the
// permission lists of both apps and hands the called app a fresh token for the viewer, or for
// the calling app when there is no viewer.
type Router struct {
	signer *Signer
	mu     sync.RWMutex
	apps   map[string]*callee
}

// callee is a running app that takes calls.
type callee struct {
	m      manifest.Manifest
	hc     *http.Client
	client runnerv1connect.AppServiceClient
}

// NewRouter returns a router that signs call tokens with signer.
func NewRouter(signer *Signer) *Router {
	return &Router{signer: signer, apps: map[string]*callee{}}
}

// Add lets the other apps call the app of m, which listens on appSocket. It closes the
// connections to the instance of the app it replaces.
func (r *Router) Add(m manifest.Manifest, appSocket string) {
	hc := socket.Client(appSocket)
	c := runnerv1connect.NewAppServiceClient(hc, socket.BaseURL,
		connect.WithGRPC(), connect.WithReadMaxBytes(runnerproto.MaxMessageBytes))
	r.put(m.App, &callee{m: m, hc: hc, client: c})
}

// Remove takes app out of the workspace's calls: calls to it fail with E-RPC-011.
func (r *Router) Remove(app string) { r.put(app, nil) }

// put routes the calls to app to c, or to no instance when c is nil, and closes the idle
// connections to the instance routed before.
func (r *Router) put(app string, c *callee) {
	r.mu.Lock()
	old := r.apps[app]
	if c == nil {
		delete(r.apps, app)
	} else {
		r.apps[app] = c
	}
	r.mu.Unlock()
	if old != nil {
		old.hc.CloseIdleConnections()
	}
}

// Call checks req, a call made by the app of caller, and forwards it to the called app with
// traceparent. A refusal is a connect error marked as the runner's, whose message is the text
// of a coded error. An error of the called app passes on only with a code a function may
// return, and never with the runner's mark.
func (r *Router) Call(ctx context.Context, caller manifest.Manifest, req *runnerv1.CallRequest, traceparent string) (*runnerv1.CallResponse, error) {
	app, fn := req.GetApp(), req.GetMethod()
	if !slices.Contains(caller.Services.Calls[app], fn) {
		return nil, refuse(connect.CodePermissionDenied, errs.New("E-RPC-008",
			fmt.Sprintf("%s calls %s of %s, which services.calls in its permission list does not name", caller.App, quote(fn), quote(app)),
			"call other apps only through their generated clients in services/, and run aicoded generate in "+caller.App))
	}
	r.mu.RLock()
	to, ok := r.apps[app]
	r.mu.RUnlock()
	if !ok {
		return nil, notRunning(app)
	}
	serve, ok := to.m.Services.Serves[fn]
	if !ok || !slices.Contains(serve.Callers, caller.App) {
		return nil, refuse(connect.CodePermissionDenied, errs.New("E-RPC-009",
			fmt.Sprintf("%s does not serve %s to %s", app, fn, caller.App),
			"add "+caller.App+" to caller= in the //ssr:access line of "+fn+" in the rpc/ folder of "+app))
	}
	now := time.Now()
	var v *viewer.Claims
	if req.GetViewer() != "" {
		c, err := viewer.VerifyIssued([]ed25519.PublicKey{r.signer.Public()}, req.GetViewer(), caller.App, now, runnerproto.MaxForwardedTokenAge)
		if err == nil && c.App {
			err = errors.New("it is an app's token, not a viewer's")
		}
		if err != nil {
			return nil, refuse(connect.CodeUnauthenticated, errs.New("E-RPC-012",
				fmt.Sprintf("%s forwarded a viewer token the runner does not accept: %v", caller.App, err),
				"call with the context of the request being served, not one kept from an earlier request"))
		}
		if !allowed(serve.Require, c.Roles) {
			return nil, refuse(connect.CodePermissionDenied, fmt.Errorf("the viewer may not call %s.%s", app, fn))
		}
		v = &c
	} else if !serve.Apps {
		return nil, refuse(connect.CodePermissionDenied, errs.New("E-RPC-010",
			fmt.Sprintf("%s calls %s.%s with no viewer, and %s does not take calls from apps on their own", caller.App, app, fn, fn),
			"add apps=true to the //ssr:access line of "+fn+" in "+app+", or call it while serving a viewer's request"))
	}
	token, err := r.signer.MintCall(v, caller.App, app, now)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	ctx, cancel := context.WithTimeout(ctx, runnerproto.MaxCallDeadline)
	defer cancel()
	out := connect.NewRequest(&runnerv1.ServeRequest{Method: fn, Payload: req.GetPayload()})
	out.Header().Set(viewer.Header, token)
	if _, ok := tracectx.Parse(traceparent); ok {
		out.Header().Set("traceparent", traceparent)
	}
	resp, err := to.client.Serve(ctx, out)
	if err != nil {
		return nil, passOn(err)
	}
	return &runnerv1.CallResponse{Payload: resp.Msg.GetPayload()}, nil
}

// notRunning returns the refusal of a call to app, which is not running in this workspace.
func notRunning(app string) *connect.Error {
	return refuse(connect.CodeUnavailable, errs.New("E-RPC-011",
		quote(app)+" is not running in this workspace",
		"run aicoded dev in a folder above both apps, and never call, in OnStart, an app that calls this app too"))
}

// quote quotes s, a string an app sent, for an error message, cut at maxQuote bytes.
func quote(s string) string {
	if len(s) > maxQuote {
		return strconv.Quote(cut(s, maxQuote)) + "..."
	}
	return strconv.Quote(s)
}

// refuse returns the runner's own refusal of a call, marked as the runner's.
func refuse(code connect.Code, err error) *connect.Error {
	ce := connect.NewError(code, err)
	ce.Meta().Set(runnerproto.OriginHeader, runnerproto.OriginRunner)
	return ce
}

// allowed reports whether a viewer with roles may call a function that requires require. A rule
// is roles joined by "|", or "*" for any viewer. Every rule must hold, and a function with no
// rule takes no viewer's calls.
func allowed(require, roles []string) bool {
	if len(require) == 0 {
		return false
	}
	for _, rule := range require {
		if !slices.ContainsFunc(strings.Split(rule, "|"), func(r string) bool { return r == "*" || slices.Contains(roles, r) }) {
			return false
		}
	}
	return true
}

// passOn turns an error of the called app into the caller's, with its code and message only. A
// code a function may return keeps its message, cut at 1 KiB. A call cut short keeps its code
// with a fixed message in place of its own. Anything else, including a failure to reach the app,
// becomes Internal "internal error".
func passOn(err error) error {
	code := connect.CodeOf(err)
	var ce *connect.Error
	if errors.As(err, &ce) && connect.IsWireError(ce) {
		switch code {
		case connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeAlreadyExists,
			connect.CodePermissionDenied, connect.CodeFailedPrecondition, connect.CodeUnavailable:
			return connect.NewError(code, errors.New(cut(ce.Message(), maxErrorMessage)))
		}
	}
	switch code {
	case connect.CodeCanceled:
		return connect.NewError(code, errors.New("the call was canceled"))
	case connect.CodeDeadlineExceeded:
		return connect.NewError(code, errors.New("the call ran out of time"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

// cut returns s cut to at most n bytes, never inside a UTF-8 sequence.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
