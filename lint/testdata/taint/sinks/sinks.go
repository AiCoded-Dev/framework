// Package sinks records a URL parameter in every way that reaches a log, standard output or a
// span.
package sinks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"aicoded.dev/framework/telemetry"
	"aicoded.dev/framework/web"
)

// printf prints through a variable.
var printf = fmt.Printf

// logger is what an app may log through instead of *slog.Logger.
type logger interface {
	Info(msg string, args ...any)
}

// Record records the URL parameter login.
func Record(ctx context.Context, r *web.Request) {
	login := r.URLParam("login")
	slog.Debug(login)                                                        // leak: a URL parameter reaches slog.Debug
	slog.DebugContext(ctx, "login", "login", login)                          // leak: a URL parameter reaches slog.DebugContext
	slog.Info("login", "login", login)                                       // leak: a URL parameter reaches slog.Info
	slog.InfoContext(ctx, "login", "login", login)                           // leak: a URL parameter reaches slog.InfoContext
	slog.Warn("login", "login", login)                                       // leak: a URL parameter reaches slog.Warn
	slog.WarnContext(ctx, "login", "login", login)                           // leak: a URL parameter reaches slog.WarnContext
	slog.Error("login", "login", login)                                      // leak: a URL parameter reaches slog.Error
	slog.ErrorContext(ctx, "login", "login", login)                          // leak: a URL parameter reaches slog.ErrorContext
	slog.Log(ctx, slog.LevelInfo, "login", "login", login)                   // leak: a URL parameter reaches slog.Log
	slog.LogAttrs(ctx, slog.LevelInfo, "login", slog.String("login", login)) // leak: a URL parameter reaches slog.LogAttrs
	l := slog.With("login", login)                                           // leak: a URL parameter reaches slog.With
	l.Info("seen")                                                           // clean
	slog.Default().Info("login", "login", login)                             // leak: a URL parameter reaches slog.Logger.Info
	_ = slog.Default().WithGroup(login)                                      // leak: a URL parameter reaches slog.Logger.WithGroup
	h := slog.Default().Handler()
	_ = h.WithAttrs([]slog.Attr{slog.String("login", login)}) // leak: a URL parameter reaches slog.Handler.WithAttrs
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "seen", 0)
	rec.Add("login", login)
	_ = h.Handle(ctx, rec) // leak: a URL parameter reaches slog.Handler.Handle
	var lg logger = slog.Default()
	lg.Info("login", "login", login) // leak: a URL parameter reaches slog.Logger.Info
	info := slog.Default().Info
	info("login", "login", login)                  // leak: a URL parameter reaches slog.Logger.Info
	fmt.Print(login)                               // leak: a URL parameter reaches fmt.Print
	fmt.Printf("%s\n", login)                      // leak: a URL parameter reaches fmt.Printf
	fmt.Println(login)                             // leak: a URL parameter reaches fmt.Println
	_, _ = printf("%s\n", login)                   // leak: a URL parameter reaches fmt.Printf
	print(login)                                   // leak: a URL parameter reaches print
	println(login)                                 // leak: a URL parameter reaches println
	_, span := telemetry.Start(ctx, "user "+login) // leak: a URL parameter reaches telemetry.Start
	span.SetAttr("login", login)                   // leak: a URL parameter reaches telemetry.Span.SetAttr
	span.SetAttr(login, "seen")                    // leak: a URL parameter reaches telemetry.Span.SetAttr
	span.End()
}

// appLogger is a logger of the app's own type, with the methods of *slog.Logger.
type appLogger struct{ *slog.Logger }

// RecordMore records the URL parameter login through a handler's group, a promoted method and a
// method expression.
func RecordMore(r *web.Request) {
	login := r.URLParam("login")
	_ = slog.Default().Handler().WithGroup(login)                // leak: a URL parameter reaches slog.Handler.WithGroup
	appLogger{slog.Default()}.Warn("login", "login", login)      // leak: a URL parameter reaches slog.Logger.Warn
	(*slog.Logger).Info(slog.Default(), "login", "login", login) // leak: a URL parameter reaches slog.Logger.Info
}

// RecordThroughField records the URL parameter login through a function value in a field, which
// may be slog.Info or a logger's Info as Record takes it as a value.
func RecordThroughField(r *web.Request) {
	s := struct{ f func(string, ...any) }{f: slog.Info}
	s.f("login", "login", r.URLParam("login")) // leak: a URL parameter reaches slog.Info or slog.Logger.Info
}
