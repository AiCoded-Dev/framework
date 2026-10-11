package dev

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/mysqlproxy"
	"aicoded.dev/framework/runnerproto/server"
)

// RunnerOptions configure NewRunner.
type RunnerOptions struct {
	// Dir is the folder where the runner makes runner.sock, and mysql.sock for an app that
	// declares sqldb. The app makes app.sock there.
	Dir string
	// FilesDir holds a folder for each file store the app declares, made when missing.
	FilesDir string
	// Values hold a value for each setting and secret the app declares.
	Values devconfig.AppValues
	// Env is the environment the app runs in: devconfig.EnvDev or devconfig.EnvPreview.
	Env devconfig.Env
	// Version is the version of the release that Hello reports.
	Version string
	// ViewerKey verifies the viewer tokens the app accepts.
	ViewerKey ed25519.PublicKey
	// Share makes runner.sock and mysql.sock usable by every user, and has Hello tell the app to
	// do the same with app.sock: for an app that runs as another user. Dir then decides who
	// reaches them.
	Share bool
	// MaxSpans and MaxSpanBytes bound the spans the runner keeps: the first MaxSpans spans of at
	// most MaxSpanBytes of text in all. Both must be positive.
	MaxSpans, MaxSpanBytes int
	// Database is where the proxy on mysql.sock logs in. It is set exactly when the app declares
	// sqldb.
	Database *mysqlproxy.Upstream
	// OnDatabaseError, if set, gets each reason the proxy could not open the app's database.
	// It may be called from several goroutines at once.
	OnDatabaseError func(error)
}

// Runner serves the runner protocol to one app that another program starts, through the runner
// server with the providers of aicoded dev. Its mail is caught and never sent, as under aicoded
// dev, it calls no other app, and it logs nothing.
type Runner struct {
	srv   *server.Server
	files *fileStore
	store *store
}

// NewRunner starts serving the runner protocol to the app of m as opts say. A fresh form key
// signs the app's form tokens.
func NewRunner(ctx context.Context, m manifest.Manifest, opts RunnerOptions) (*Runner, error) {
	if err := checkRunner(m, opts); err != nil {
		return nil, err
	}
	st := newStore(m.App, io.Discard)
	st.hideSecrets(opts.Values.Secrets)
	st.limitSpans(opts.MaxSpans, opts.MaxSpanBytes)
	files, err := newFileStore(opts.FilesDir, m.Stores())
	if err != nil {
		return nil, err
	}
	srv, err := server.New(ctx, server.Options{
		Dir:             opts.Dir,
		Manifest:        m,
		Env:             string(opts.Env),
		Version:         opts.Version,
		ViewerKeys:      []ed25519.PublicKey{opts.ViewerKey},
		CSRFKey:         newCSRFKey(),
		ShareAppSocket:  opts.Share,
		Database:        opts.Database,
		OnDatabaseError: opts.OnDatabaseError,
		Providers:       providers(files, st, opts.Values, nil),
	})
	if err != nil {
		files.close()
		return nil, err
	}
	return &Runner{srv: srv, files: files, store: st}, nil
}

// checkRunner refuses opts that do not fit the app of m.
func checkRunner(m manifest.Manifest, opts RunnerOptions) error {
	switch {
	case !opts.Env.Valid():
		return errors.New("dev: RunnerOptions.Env must be dev or preview")
	case len(opts.ViewerKey) != ed25519.PublicKeySize:
		return errors.New("dev: RunnerOptions.ViewerKey is not an Ed25519 public key")
	case opts.MaxSpans <= 0 || opts.MaxSpanBytes <= 0:
		return errors.New("dev: RunnerOptions.MaxSpans and MaxSpanBytes must be positive")
	case m.SQLDB() != (opts.Database != nil):
		return errors.New("dev: RunnerOptions.Database must be set exactly when the app declares sqldb")
	}
	if missing := missingValues(m, opts.Values); len(missing) > 0 {
		return fmt.Errorf("dev: no values for %s of app %s", strings.Join(missing, ", "), m.App)
	}
	return nil
}

// Ready is closed once the app reports that it accepts requests.
func (r *Runner) Ready() <-chan struct{} { return r.srv.Ready() }

// Spans returns the spans the runner kept, oldest first, and how many it did not keep: past its
// bounds, or dropped by the app or the runner server before they reached it.
func (r *Runner) Spans() ([]devapi.Span, uint64) {
	return r.store.spans.All(), r.store.droppedSpans() + r.store.exportDropped.Load()
}

// Close stops serving, which removes runner.sock and mysql.sock.
func (r *Runner) Close() {
	_ = r.srv.Close()
	r.files.close()
}
