package app

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/telemetry"
)

// tracing continues the runner's trace, or starts a new one when the header is missing or malformed.
func tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if sc, ok := tracectx.Parse(r.Header.Get("traceparent")); ok {
			ctx = tracectx.With(ctx, sc)
		}
		ctx, span := telemetry.Start(ctx, "HTTP "+r.Method)
		defer span.End()
		span.SetAttr("http.path", r.URL.Path)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// recoverer turns a panic into a plain 500 and logs it, with its value only under aicoded dev in
// environment `dev`; the app keeps serving.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			slog.ErrorContext(r.Context(), "panic serving request", "value", redact.ValueFor(r.Context(), v), "stack", string(debug.Stack()))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()
		next.ServeHTTP(w, r)
	})
}
