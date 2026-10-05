// Package flows: values that hold a context without being one. fmt prints an exported field, an
// element or what a pointer points to through the context's String method, which prints its
// values, and an unexported field as an address. Every context of the app shares what they hold,
// so each line that reads one names every kind that any of them holds.
package flows

import (
	"context"
	"fmt"
	"log/slog"
)

// Holder holds a context in an exported field.
type Holder struct {
	Ctx context.Context
	N   int
}

// holder holds a context in an unexported field.
type holder struct{ ctx context.Context }

// Holders records values that hold the context ctx.
func Holders(ctx context.Context) {
	slog.Info("struct", "h", Holder{Ctx: ctx, N: 1})               // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("pointer", "h", &Holder{Ctx: ctx, N: 1})             // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	fmt.Println(Holder{Ctx: ctx})                                  // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches fmt.Println
	slog.Info("sprint", "h", fmt.Sprint(Holder{Ctx: ctx}))         // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("slice", "s", []context.Context{ctx})                // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("array", "a", [1]context.Context{ctx})               // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("map", "m", map[string]context.Context{"c": ctx})    // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("nested", "n", struct{ H Holder }{Holder{Ctx: ctx}}) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("unexported", "h", holder{ctx: ctx})                 // clean
}

// hinner holds a context in an exported field.
type hinner struct{ Ctx context.Context }

// hbox embeds an hinner, whose exported fields fmt prints.
type hbox struct{ hinner }

// hptr embeds a pointer to an hinner, which fmt prints as an address.
type hptr struct{ *hinner }

// HoldersEmbedded records values that hold the context ctx in an embedded field.
func HoldersEmbedded(ctx context.Context) {
	slog.Info("embedded", "h", hbox{hinner{Ctx: ctx}})          // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("embedded pointer", "h", hptr{&hinner{Ctx: ctx}}) // clean
}
