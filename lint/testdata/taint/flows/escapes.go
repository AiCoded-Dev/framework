// Package flows: a closure that escapes into the framework, which no code of the app calls.
package flows

import (
	"context"
	"log/slog"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/web"
)

// Escapes returns options whose OnStart records the URL parameter login.
func Escapes(r *web.Request) app.Options {
	login := r.URLParam("login")
	return app.Options{OnStart: func(context.Context) error {
		slog.Info("started", "login", login) // leak: a URL parameter reaches slog.Info
		return nil
	}}
}
