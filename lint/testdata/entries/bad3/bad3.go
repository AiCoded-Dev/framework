// Package bad3 builds options through pointers to type parameters, and through a constraint that
// mixes a type term with a method.
package bad3

import (
	"context"
	"fmt"
	"net/http"

	"aicoded.dev/framework/app"
	frameworkrpc "aicoded.dev/framework/rpc"
)

// options has the fields of app.Options.
type options struct {
	Handler http.Handler
	RPC     *frameworkrpc.Server
	OnStart func(context.Context) error
}

func (options) String() string { return "options" }

// ptrToTP builds options as the elided &T{} of a []*T, T a type parameter.
func ptrToTP[T options](h http.Handler) []*T {
	return []*T{{h, nil, nil}}
}

// ptrToTPKeyed is ptrToTP with keyed fields.
func ptrToTPKeyed[T options](h http.Handler) []*T {
	return []*T{{Handler: h}}
}

// tpOfPtrToTP builds options through PT, whose core type is *T, a pointer to a type parameter.
func tpOfPtrToTP[T options, PT interface{ *T }](h http.Handler) []PT {
	return []PT{{h, nil, nil}}
}

// tpOfPtrToTPKeyed is tpOfPtrToTP with a tilde term and keyed fields.
func tpOfPtrToTPKeyed[T options, PT interface{ ~*T }](h http.Handler) map[string]PT {
	return map[string]PT{"a": {Handler: h}}
}

// withMethods builds options through a constraint with a type term and a method; go/types gives
// T the core type options.
func withMethods[T interface {
	options
	fmt.Stringer
}](h http.Handler) T {
	return T{h, nil, nil}
}

// Converted hands app.Main options built through the pointers.
func Converted(h http.Handler) {
	app.Main(app.Options(*ptrToTP[options](h)[0]))
	app.Main(app.Options(*ptrToTPKeyed[options](h)[0]))
	app.Main(app.Options(*tpOfPtrToTP[options, *options](h)[0]))
	app.Main(app.Options(*tpOfPtrToTPKeyed[options, *options](h)["a"]))
	app.Main(app.Options(withMethods[options](h)))
}

// generic has the fields of app.Options, with type parameters as their types.
type generic[H, R any] struct {
	Handler H
	RPC     R
	OnStart func(context.Context) error
}

// viaFields builds options through generic, with a value of a type parameter in each field.
func viaFields[H, R any](h H, r R) generic[H, R] {
	return generic[H, R]{h, r, nil}
}

// Generic hands app.Main options built through viaFields.
func Generic(h http.Handler) {
	app.Main(app.Options(viaFields[http.Handler, *frameworkrpc.Server](h, nil)))
}

// pair builds a slice that holds values of a type parameter, which stays allowed.
func pair[T any](a, b T) []T {
	return []T{a, b}
}
