// Package bad4 builds options through a generic struct whose entry fields are set by assignment
// inside a generic function, so no composite literal builds or holds a value of a type parameter.
package bad4

import (
	"context"
	"net/http"

	"aicoded.dev/framework/app"
	frameworkrpc "aicoded.dev/framework/rpc"
)

// generic has the fields of app.Options, with type parameters as their types.
type generic[H, R any] struct {
	Handler H
	RPC     R
	OnStart func(context.Context) error
}

// assign sets the fields of a zero generic by assignment.
func assign[H, R any](h H, r R) generic[H, R] {
	var g generic[H, R]
	g.Handler = h
	g.RPC = r
	return g
}

// assignPtr sets the fields through a pointer that new returns.
func assignPtr[H, R any](h H, r R) *generic[H, R] {
	g := new(generic[H, R])
	g.Handler = h
	g.RPC = r
	return g
}

// assignRange sets Handler by range.
func assignRange[H, R any](hs []H) (g generic[H, R]) {
	for _, g.Handler = range hs {
	}
	return g
}

// assignAnon sets the fields of an anonymous struct by assignment.
func assignAnon[H, R any](h H, r R) struct {
	Handler H
	RPC     R
	OnStart func(context.Context) error
} {
	var g struct {
		Handler H
		RPC     R
		OnStart func(context.Context) error
	}
	g.Handler = h
	g.RPC = r
	return g
}

// Assigned hands app.Main options built through assign, assignPtr, assignRange and assignAnon.
func Assigned(h http.Handler) {
	app.Main(app.Options(assign[http.Handler, *frameworkrpc.Server](h, nil)))
	app.Main(app.Options(*assignPtr[http.Handler, *frameworkrpc.Server](h, nil)))
	app.Main(app.Options(assignRange[http.Handler, *frameworkrpc.Server]([]http.Handler{h})))
	app.Main(assignAnon[http.Handler, *frameworkrpc.Server](h, nil))
}
