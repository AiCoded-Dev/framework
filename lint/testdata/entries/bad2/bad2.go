// Package bad2 builds options through type parameters whose constraints embed other interfaces.
package bad2

import (
	"context"
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

// ptrTo lists the pointer to options.
type ptrTo interface{ *options }

// viaPtr embeds ptrTo, so its type set is still {*options}.
type viaPtr interface{ ptrTo }

// valTo lists options.
type valTo interface{ options }

// viaVal embeds valTo.
type viaVal interface{ valTo }

// fillPtr returns options through a type parameter whose constraint embeds a pointer constraint.
func fillPtr[T viaPtr](h http.Handler) []T {
	return []T{{h, nil, nil}}
}

// fillVal returns options through a type parameter whose constraint embeds another interface.
func fillVal[T viaVal](h http.Handler) T {
	return T{h, nil, nil}
}

// fillInline returns options through an inline embedded interface.
func fillInline[T interface{ interface{ options } }](h http.Handler) T {
	return T{h, nil, nil}
}

// fillUnion returns options through a union whose first term is an interface.
func fillUnion[T interface{ valTo | options }](h http.Handler) T {
	return T{h, nil, nil}
}

// Converted hands app.Main options built through the constraints.
func Converted(h http.Handler) {
	app.Main(app.Options(*fillPtr[*options](h)[0]))
	app.Main(app.Options(fillVal[options](h)))
	app.Main(app.Options(fillInline[options](h)))
	app.Main(app.Options(fillUnion[options](h)))
}
