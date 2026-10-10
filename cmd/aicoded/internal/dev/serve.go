package dev

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/mysqlproxy"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/socket"
)

// served is what a runner serves one app in its run folder: the runner protocol on runner.sock,
// and the database on mysql.sock when the app declares sqldb.
type served struct {
	svc    *runnerService
	server *http.Server
	dbSock net.Listener
	db     *mysqlproxy.Proxy
	// share makes the sockets usable by every user, not only by this one.
	share bool
	// quiet servers log nothing, so that an app cannot write to the runner's stderr.
	quiet bool
}

// listenRunner serves svc on runner.sock in dir, with the file stores of m in filesDir and the
// mail rules of m.
func (s *served) listenRunner(ctx context.Context, dir, filesDir string, m manifest.Manifest) error {
	files, err := newFileService(filesDir, m.Stores())
	if err != nil {
		return err
	}
	s.svc.files = files
	s.svc.store.mail.setEmail(m.Email)
	s.svc.mail = s.svc.store.mail
	l, err := s.listen(ctx, filepath.Join(dir, runnerproto.RunnerSocket))
	if err != nil {
		return err
	}
	s.server = socket.NewServer(s.svc.handler())
	if s.quiet {
		s.server.ErrorLog = log.New(io.Discard, "", 0)
	}
	go func() { _ = s.server.Serve(l) }()
	return nil
}

// listenDatabase serves the database on mysql.sock in dir through a proxy that logs in as up,
// and hands each reason it cannot to logErr.
func (s *served) listenDatabase(ctx context.Context, dir string, up mysqlproxy.Upstream, logErr func(error)) error {
	l, err := s.listen(ctx, filepath.Join(dir, runnerproto.MySQLSocket))
	if err != nil {
		return err
	}
	s.dbSock = l
	s.db = mysqlproxy.New(up)
	s.db.OnError = logErr
	go serveDB(s.db, l, logErr)
	return nil
}

// serveDB serves p on l until l is closed. When p stops for any other reason, serveDB logs why
// and closes l, so the app's connections fail at once instead of waiting for an accept.
func serveDB(p *mysqlproxy.Proxy, l net.Listener, logErr func(error)) {
	if err := p.Serve(l); err != nil {
		logErr(err)
		_ = l.Close()
	}
}

// close stops serving and closes the file stores.
func (s *served) close() {
	if s.server != nil {
		_ = s.server.Close()
	}
	if s.dbSock != nil {
		_ = s.dbSock.Close()
	}
	if s.db != nil {
		s.db.Close()
	}
	if s.svc != nil && s.svc.files != nil {
		s.svc.files.close()
	}
}

// listen listens on the Unix socket at path, which only this user can use, or every user when s
// shares its sockets.
func (s *served) listen(ctx context.Context, path string) (net.Listener, error) {
	if s.share {
		return socket.ListenShared(ctx, path)
	}
	return socket.Listen(ctx, path)
}

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

// Runner serves the runner protocol to one app that another program starts. Its mail is caught
// and never sent, as under aicoded dev, it calls no other app, and its servers log nothing.
type Runner struct {
	served
	store *store
}

// NewRunner starts serving the runner protocol to the app of m as opts say. A fresh form key
// signs the app's form tokens.
func NewRunner(ctx context.Context, m manifest.Manifest, opts RunnerOptions) (*Runner, error) {
	if err := checkRunner(m, opts); err != nil {
		return nil, err
	}
	csrfKey := make([]byte, csrfKeySize)
	_, _ = rand.Read(csrfKey)
	st := newStore(m.App, io.Discard)
	st.hideSecrets(opts.Values.Secrets)
	st.limitSpans(opts.MaxSpans, opts.MaxSpanBytes)
	r := &Runner{store: st}
	r.share, r.quiet = opts.Share, true
	r.svc = newRunnerService(m, opts.Values, opts.Env, csrfKey, [][]byte{opts.ViewerKey}, st)
	r.svc.version, r.svc.shareAppSocket = opts.Version, opts.Share
	err := r.listenRunner(ctx, opts.Dir, opts.FilesDir, m)
	if err == nil && opts.Database != nil {
		logErr := opts.OnDatabaseError
		if logErr == nil {
			logErr = func(error) {}
		}
		err = r.listenDatabase(ctx, opts.Dir, *opts.Database, logErr)
	}
	if err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
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
func (r *Runner) Ready() <-chan struct{} { return r.svc.ready }

// Spans returns the spans the runner kept, oldest first, and how many it did not keep: past its
// bounds, or dropped by the app before they reached it.
func (r *Runner) Spans() ([]devapi.Span, uint64) {
	return r.store.spans.All(), r.store.droppedSpans() + r.svc.dropped.Load()
}

// Close stops serving, which removes runner.sock and mysql.sock.
func (r *Runner) Close() { r.close() }
