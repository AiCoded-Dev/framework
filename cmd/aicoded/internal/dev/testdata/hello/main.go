// Command hello is a minimal app used by the dev runner's tests.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/config"
	"aicoded.dev/framework/secrets"
	"aicoded.dev/framework/telemetry"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", hello)
	mux.HandleFunc("GET /env", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Join(os.Environ(), "\n"))
	})
	mux.HandleFunc("GET /exit", func(http.ResponseWriter, *http.Request) { os.Exit(3) })
	mux.HandleFunc("GET /leak", leak)
	mux.HandleFunc("GET /hold", hold)
	app.Main(app.Options{Handler: mux})
}

// hold sends the start of its answer and keeps the request open until the client goes, so that
// the tests can stop the app while it serves a request.
func hold(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "held")
	_ = http.NewResponseController(w).Flush()
	<-r.Context().Done()
}

func hello(w http.ResponseWriter, r *http.Request) {
	ctx, span := telemetry.Start(r.Context(), "hello")
	defer span.End()
	greeting, err := config.String(ctx, "greeting")
	if err != nil {
		slog.ErrorContext(ctx, "read setting", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	token, err := secrets.Get(ctx, "token")
	if err != nil {
		slog.ErrorContext(ctx, "read secret", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "%s %s (token %d bytes)", greeting, auth.Viewer(ctx).Name, len(token.Reveal()))
}

// leak logs the secret token, and puts it on its span and in its error, so that the tests can
// check that aicoded dev hides it.
func leak(w http.ResponseWriter, r *http.Request) {
	ctx, span := telemetry.Start(r.Context(), "leak")
	defer span.End()
	token, err := secrets.Get(ctx, "token")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	v := token.Reveal()
	span.SetAttr("token", v)
	span.RecordError(errors.New("leaked " + v))
	slog.ErrorContext(ctx, "leaked "+v, "token", v, slog.Group("request", "headers", []string{"Bearer " + v}))
	fmt.Fprintln(os.Stderr, "plain "+v)
	fmt.Fprint(w, "leaked")
}
