package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// Method is one function an app serves, as the generated table describes it.
type Method struct {
	Name string
	// Callers are the apps that may call the function.
	Callers []string
	// Require holds the rules a viewer must meet, each roles joined by "|", or "*" for any
	// viewer. With no rules, no viewer may call the function.
	Require []string
	// Apps allows calls an app makes as itself, with no viewer.
	Apps bool
	// Call decodes the input, runs the function and encodes its output.
	Call func(ctx context.Context, in []byte) ([]byte, error)
}

var (
	errDecode   = errors.New("rpc: the call's input could not be decoded")
	errPanicked = errors.New("rpc: the function panicked")
)

// DecodeError marks err as a failure to decode a call's input. Generated code uses it; the
// caller gets InvalidArgument.
//
// Generated code only.
func DecodeError(err error) error {
	return fmt.Errorf("%w: %w", errDecode, err)
}

// Server serves an app's functions to the runner.
type Server struct {
	methods map[string]Method
}

// NewServer returns a server for methods. aicoded generate writes the call of it, in the app's
// rpc.Server function.
//
// Generated code only.
func NewServer(methods ...Method) *Server {
	s := &Server{methods: make(map[string]Method, len(methods))}
	for _, m := range methods {
		s.methods[m.Name] = m
	}
	return s
}

// Handler returns the path the runner sends calls to and the handler to serve there.
func (s *Server) Handler() (path string, h http.Handler) {
	_, h = runnerv1connect.NewAppServiceHandler(service{s}, connect.WithReadMaxBytes(runnerproto.MaxMessageBytes))
	return runnerv1connect.AppServiceServeProcedure, h
}

type service struct{ s *Server }

func (v service) Serve(ctx context.Context, req *connect.Request[runnerv1.ServeRequest]) (*connect.Response[runnerv1.ServeResponse], error) {
	out, err := v.s.serve(ctx, req.Msg.GetMethod(), req.Msg.GetPayload())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ServeResponse{Payload: out}), nil
}

func (s *Server) serve(ctx context.Context, name string, in []byte) ([]byte, error) {
	m, ok := s.methods[name]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("no such function"))
	}
	app, caller := appName(ctx), identity.Caller(ctx)
	if !slices.Contains(m.Callers, caller) {
		return nil, connect.NewError(connect.CodePermissionDenied, errs.New("E-RPC-009",
			fmt.Sprintf("%s does not serve %s to %s", app, name, caller),
			fmt.Sprintf("add %s to caller= on the //ssr:access line of %s in %s's rpc/ package", caller, name, app)))
	}
	if v := identity.From(ctx); v.Authenticated() {
		if !allowed(m.Require, v) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("the viewer may not call %s.%s", app, name))
		}
	} else if !m.Apps {
		return nil, connect.NewError(connect.CodePermissionDenied, errs.New("E-RPC-010",
			fmt.Sprintf("%s called %s.%s with no viewer, and the function needs one", caller, app, name),
			fmt.Sprintf("call it with the context of a viewer's request, or add apps=true to the //ssr:access line of %s in %s's rpc/ package", name, app)))
	}
	out, err := run(ctx, m, in)
	if err != nil {
		return nil, answer(ctx, name, err)
	}
	return out, nil
}

// allowed reports whether v meets every rule of require. A rule holds when it has "*" or one of
// the viewer's roles.
func allowed(require []string, v identity.Identity) bool {
	if len(require) == 0 {
		return false
	}
	for _, rule := range require {
		if !slices.ContainsFunc(strings.Split(rule, "|"), func(r string) bool { return r == "*" || v.HasRole(r) }) {
			return false
		}
	}
	return true
}

// run calls m. A panic in it is logged, with its value only under aicoded dev in environment
// `dev`, and becomes errPanicked.
func run(ctx context.Context, m Method, in []byte) (out []byte, err error) {
	defer func() {
		if v := recover(); v != nil {
			slog.ErrorContext(ctx, "panic in function", "function", m.Name, "caller", identity.Caller(ctx),
				"value", redact.ValueFor(ctx, v), "stack", string(debug.Stack()))
			err = errPanicked
		}
	}()
	return m.Call(ctx, in)
}

// answer turns a function's error into what the caller may see. Only an error from Error with
// one of the six caller-facing codes keeps its message; any other error is logged here, with its
// text only under aicoded dev in environment `dev`, and reaches the caller as "internal error".
func answer(ctx context.Context, name string, err error) error {
	if cerr := ctx.Err(); cerr != nil && errors.Is(err, cerr) {
		code := connect.CodeCanceled
		if errors.Is(cerr, context.DeadlineExceeded) {
			code = connect.CodeDeadlineExceeded
		}
		return connect.NewError(code, cerr)
	}
	if errors.Is(err, errDecode) {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("the call's input could not be decoded"))
	}
	var se *statusError
	if errors.As(err, &se) && se.code != Internal {
		return connect.NewError(connectCodes[se.code], errors.New(se.msg))
	}
	if !errors.Is(err, errPanicked) {
		slog.ErrorContext(ctx, "call failed", "function", name, "caller", identity.Caller(ctx), "error", redact.For(ctx, err))
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func appName(ctx context.Context) string {
	if s := runner.From(ctx); s != nil {
		return s.App
	}
	return ""
}
