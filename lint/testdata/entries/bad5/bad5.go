// Package bad5 hands app.Main and app.Run options that are not a literal written in the call.
package bad5

import (
	"context"
	"net/http"

	"aicoded.dev/framework/app"
	frameworkrpc "aicoded.dev/framework/rpc"

	"entries/pages"
	"entries/rpc"
)

// options has the fields of app.Options.
type options struct {
	Handler http.Handler
	RPC     *frameworkrpc.Server
	OnStart func(context.Context) error
}

// unnamed returns options of an unnamed struct type, which app.Options accepts as it is.
func unnamed() struct {
	Handler http.Handler
	RPC     *frameworkrpc.Server
	OnStart func(context.Context) error
} {
	var o struct {
		Handler http.Handler
		RPC     *frameworkrpc.Server
		OnStart func(context.Context) error
	}
	return o
}

// built returns app options.
func built() app.Options { return app.Options{Handler: pages.NewHandler()} }

// both returns a context and app options, for app.Run.
func both() (context.Context, app.Options) { return context.Background(), built() }

// Passed hands app.Main and app.Run options in every way but a literal written in the call.
func Passed(o options) error {
	app.Main(app.Options(o))
	app.Main(unnamed())
	opts := app.Options{Handler: pages.NewHandler(), RPC: rpc.Server()}
	app.Main(opts)
	app.Main(built())
	return app.Run(both())
}

// Values uses app.Main and app.Run as values.
func Values(ctx context.Context) error {
	f := app.Main
	f(app.Options{Handler: pages.NewHandler()})
	run := []func(context.Context, app.Options) error{app.Run}
	return run[0](ctx, app.Options{Handler: pages.NewHandler()})
}

// Literal calls app.Main and app.Run with a literal written in the call, which passes.
func Literal(ctx context.Context) error {
	(app.Main)((app.Options{Handler: pages.NewHandler(), RPC: rpc.Server()}))
	return app.Run(ctx, app.Options{Handler: pages.NewHandler()})
}
