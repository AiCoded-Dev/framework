package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/exporter"
	"aicoded.dev/framework/internal/logging"
	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/viewerauth"
	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// Options configure an app. An app needs Handler, RPC or both.
type Options struct {
	// Handler serves the app's pages. Every request it sees carries a verified viewer.
	Handler http.Handler
	// RPC serves the functions of the app's rpc/ package to other apps: the rpc.Server() that
	// aicoded generate writes there.
	RPC *rpc.Server
	// OnStart, if set, runs after the handshake and before the app reports ready.
	// Its context carries the runner, so settings and secrets can be read.
	OnStart func(ctx context.Context) error
}

const (
	shutdownTimeout = 8 * time.Second
	spanQueue       = 4096
)

// Main runs the app until the process gets SIGINT or SIGTERM. On error it prints the error, its
// text under aicoded dev in environment `dev` and only its kinds and codes anywhere else, and
// exits with status 1.
func Main(opts Options) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	failed := runMain(ctx, opts, os.Stderr)
	stop()
	if failed {
		os.Exit(1)
	}
}

// runMain runs the app and prints the error it ends with to w, as Main does. It reports whether
// there was one.
func runMain(ctx context.Context, opts Options, w io.Writer) bool {
	base, err := run(ctx, opts)
	if err != nil {
		fmt.Fprintln(w, redact.For(base, err))
	}
	return err != nil
}

// Run runs the app until ctx is done, then lets in-flight requests finish.
func Run(ctx context.Context, opts Options) error {
	_, err := run(ctx, opts)
	return err
}

// run is Run. It also returns ctx with the runner's session once the handshake is done.
func run(ctx context.Context, opts Options) (context.Context, error) {
	if opts.Handler == nil && opts.RPC == nil {
		return ctx, errors.New("app: Options.Handler and Options.RPC are both nil")
	}
	dir := os.Getenv(runnerproto.EnvRunnerDir)
	c, err := connectRunner(ctx, dir)
	if err != nil {
		return ctx, err
	}
	slog.SetDefault(logging.New(os.Stdout))

	exp := exporter.New(c.telemetry, spanQueue)
	c.session.Export = exp.Add
	base := runner.With(context.WithoutCancel(ctx), c.session)
	expCtx, stopExport := context.WithCancel(base)
	exported := make(chan struct{})
	go func() {
		exp.Run(expCtx)
		close(exported)
	}()
	defer func() {
		stopExport()
		<-exported
	}()

	listen := socket.Listen
	if c.shareSocket {
		listen = socket.ListenShared
	}
	l, err := listen(ctx, filepath.Join(dir, runnerproto.AppSocket))
	if err != nil {
		return base, fmt.Errorf("app: listen: %w", err)
	}
	callPath, h := routes(opts)
	srv := socket.NewServer(tracing(recoverer(viewerauth.Middleware(c.keys, c.session.App, callPath, h))))
	srv.BaseContext = func(net.Listener) context.Context { return base }
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()

	if err := start(ctx, base, c, opts); err != nil {
		_ = srv.Close()
		return base, err
	}

	select {
	case <-ctx.Done():
	case err := <-served:
		return base, fmt.Errorf("app: serve: %w", err)
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return base, fmt.Errorf("app: shutdown: %w", err)
	}
	return base, nil
}

// routes returns the path of calls from other apps and a handler that serves them with
// opts.RPC and everything else with opts.Handler. A request for a missing one gets 404.
func routes(opts Options) (string, http.Handler) {
	callPath, calls := runnerv1connect.AppServiceServeProcedure, http.NotFoundHandler()
	if opts.RPC != nil {
		callPath, calls = opts.RPC.Handler()
	}
	pages := opts.Handler
	if pages == nil {
		pages = http.NotFoundHandler()
	}
	return callPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == callPath {
			calls.ServeHTTP(w, r)
			return
		}
		pages.ServeHTTP(w, r)
	})
}

// start runs OnStart and tells the runner the app is ready. A Ready call cut short because
// ctx is done is not an error: the app then shuts down gracefully.
func start(ctx, base context.Context, c *conn, opts Options) error {
	if opts.OnStart != nil {
		if err := opts.OnStart(base); err != nil {
			return fmt.Errorf("app: start: %w", err)
		}
	}
	if _, err := c.lifecycle.Ready(ctx, connect.NewRequest(&runnerv1.ReadyRequest{})); err != nil && ctx.Err() == nil {
		return fmt.Errorf("app: ready: %w", err)
	}
	return nil
}
