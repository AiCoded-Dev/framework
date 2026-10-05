// Package runner carries what the runner handed this app process, through contexts.
package runner

import (
	"context"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
)

// Session is what the runner handed the process at start-up.
type Session struct {
	App, Env, Version string
	Settings          map[string]string
	CSRFKey           []byte
	// Dir is the runner's folder of sockets.
	Dir     string
	Secrets runnerv1connect.SecretsServiceClient
	Files   runnerv1connect.FilesServiceClient
	Mail    runnerv1connect.MailServiceClient
	Rpc     runnerv1connect.RpcServiceClient
	Export  func(*runnerv1.Span) // never blocks
}

// ErrNoRunner is returned by building blocks called with a context that does not come from the app.
var ErrNoRunner = errs.New("E-RUN-004", "this context does not carry the app's runner",
	"pass the context of a request, or the one app.Options.OnStart receives")

type ctxKey struct{}

// With returns a copy of ctx carrying s.
func With(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// From returns the session carried by ctx, or nil.
func From(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}
