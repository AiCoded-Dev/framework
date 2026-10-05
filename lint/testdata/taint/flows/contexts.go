// Package flows: the routes into and out of contexts. Every context of the app shares what they
// hold, so each line that reads one, here and in flows.go, names every kind that any of them
// holds. Each route puts a kind of its own, so a route that is lost changes every message.
package flows

import (
	"context"
	"errors"
	"log/slog"

	"aicoded.dev/framework/filestore"
	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/web"
)

// MyCtx is an interface type of the app with the underlying type of context.Context.
type MyCtx context.Context

// EmbCtx is an interface type of the app that embeds context.Context and adds nothing.
type EmbCtx interface{ context.Context }

// headerCtx is a context of the app's own type that holds a request header.
type headerCtx struct {
	context.Context
	header string
}

// Value returns the header, whatever the key.
func (c *headerCtx) Value(any) any { return c.header }

// fileKey is a context key that prints itself.
type fileKey string

// String returns the key.
func (k fileKey) String() string { return string(k) }

// ContextRoutes carries a request header into contexts through the app's own interface types of
// context and into a context's memory after it became one, a mail through a cancel function of
// any type, and a stored file as a key.
func ContextRoutes(ctx context.Context, r *web.Request, store *filestore.Store) error {
	h := r.Header.Get("X-Team")
	showContext(MyCtx(&headerCtx{ctx, h}))
	showContext(EmbCtx(&headerCtx{ctx, h}))
	var m MyCtx = ctx
	slog.Info("MyCtx", "ctx", m) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	var e EmbCtx = ctx
	slog.Info("EmbCtx", "ctx", e)                          // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Info("own type", "ctx", &headerCtx{Context: ctx}) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	c := context.Context(&headerCtx{Context: ctx})
	c.(*headerCtx).header = h
	slog.Info("asserted", "v", c.Value(nil)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	c = &headerCtx{Context: ctx}
	setHeader(c, h)
	slog.Info("callee", "v", c.Value(nil)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	c = &headerCtx{Context: ctx}
	if p, ok := c.(*headerCtx); ok {
		p.header = h
	}
	slog.Info("comma-ok", "v", c.Value(nil)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	mail, err := mailer.Get(ctx, "m1")
	if err != nil {
		return err
	}
	cctx, cancel := context.WithCancelCause(ctx)
	var cf func(error) = cancel
	cf(errors.New("bounced " + mail.Subject))
	slog.Error("cause", "err", context.Cause(cctx)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Error
	data, err := store.ReadFile("team.txt")
	if err != nil {
		return err
	}
	kctx := context.WithValue(ctx, fileKey(data), true)
	slog.Info("key", "ctx", kctx) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	return nil
}

// showContext records what c holds.
func showContext(c context.Context) {
	slog.Info("value", "v", c.Value(nil)) // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Info
	slog.Error("err", "err", c.Err())     // leak: a URL parameter, the request, the viewer's identity, a mail from the inbox or a stored file reaches slog.Error
}

// setHeader writes h into c, a context of type *headerCtx.
func setHeader(c context.Context, h string) { c.(*headerCtx).header = h }
