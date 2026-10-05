// Package bad sets the entry points of an app in every way lint refuses.
package bad

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

// NewHandler is not the generated function.
func NewHandler() http.Handler { return pages.NewHandler() }

// Options returns app options with handlers that are not generated, and hands app.Main options
// converted from a struct of the same fields.
func Options(h http.Handler) []app.Options {
	o := app.Options{Handler: pages.NewHandler(), RPC: rpc.Server()}
	o.Handler = h
	p := &o.RPC
	*p = nil
	app.Main(app.Options(options{h, rpc.Server(), nil}))
	app.Main(app.Options(fill[options](h)))
	return []app.Options{
		o,
		{Handler: NewHandler()},
		{Handler: (pages.NewHandler()), RPC: nil},
	}
}

// fill returns options of type T with h as the handler.
func fill[T options](h http.Handler) T {
	return T{h, nil, nil}
}

// handler is http.Handler under another name.
type handler = http.Handler

// aliased has the fields of app.Options, with Handler's type written as handler.
type aliased struct {
	Handler handler
	RPC     *frameworkrpc.Server
	OnStart func(context.Context) error
}

// Aliased hands app.Main options converted from aliased.
func Aliased(h http.Handler) {
	app.Main(app.Options(aliased{h, rpc.Server(), nil}))
}

// optionsPtr is a pointer to options under another name.
type optionsPtr = *options

// Pointers builds options through pointers, with the literal types left out, and sets a handler
// by range.
func Pointers(h http.Handler, hs []http.Handler) {
	_ = []*options{{h, rpc.Server(), nil}}
	_ = map[string]*options{"a": {h, rpc.Server(), nil}}
	_ = []optionsPtr{{h, rpc.Server(), nil}}
	var o app.Options
	for _, o.Handler = range hs {
	}
}

// fillPtr returns options through a type parameter constrained to a pointer to options.
func fillPtr[T *options](h http.Handler) []T {
	return []T{{h, nil, nil}}
}

// Converted hands app.Main options built by fillPtr.
func Converted(h http.Handler) {
	app.Main(app.Options(*fillPtr[*options](h)[0]))
}
