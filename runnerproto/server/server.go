package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"sync"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/mysqlproxy"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

// MinCSRFKey is the shortest key Options.CSRFKey may be, in bytes.
const MinCSRFKey = 32

// The environments an app may run in.
const (
	EnvDev        = "dev"
	EnvPreview    = "preview"
	EnvProduction = "production"
)

// Options configure New.
type Options struct {
	// Dir is the folder where the server makes runner.sock, and mysql.sock when the app declares
	// sqldb. The app makes app.sock there.
	Dir string
	// Manifest is the app's permission list.
	Manifest manifest.Manifest
	// Env is the environment the app runs in: EnvDev, EnvPreview or EnvProduction.
	Env string
	// Version is the version of the release that Hello reports.
	Version string
	// ViewerKeys verify the viewer tokens the app accepts. There must be at least one.
	ViewerKeys []ed25519.PublicKey
	// CSRFKey signs the app's form tokens: an app accepts the tokens of another instance of it
	// with the same key. It must be at least MinCSRFKey bytes.
	CSRFKey []byte
	// ShareAppSocket makes runner.sock and mysql.sock usable by every user, and has Hello tell
	// the app to do the same with app.sock: for an app that runs as another user. Dir then
	// decides who reaches them.
	ShareAppSocket bool
	// Adopted marks an app that is already running and said Hello to an earlier server. Ready is
	// closed at New; nothing else changes.
	Adopted bool
	// Database is where the proxy on mysql.sock logs in, until SetDatabase changes it. It is set
	// exactly when the app declares sqldb.
	Database *mysqlproxy.Upstream
	// OnDatabaseError, if set, gets each reason the proxy could not open the app's database.
	// It may be called from several goroutines at once.
	OnDatabaseError func(error)
	// OnReady, if set, is called at each Ready the app sends.
	OnReady func()
	// Providers do the work of the building blocks.
	Providers Providers
}

// Server serves the runner protocol to one app. It knows the app by the socket it listens on,
// never by anything in a request, and logs nothing, so that an app cannot write to its log.
type Server struct {
	m         manifest.Manifest
	env       string
	version   string
	csrfKey   []byte
	viewers   [][]byte
	share     bool
	providers Providers
	onReady   func()

	ready     chan struct{}
	readyOnce sync.Once

	http   *http.Server
	dbSock net.Listener
	db     *mysqlproxy.Proxy
}

// New starts serving the runner protocol on runner.sock in opts.Dir, and the app's database on
// mysql.sock there when the app declares sqldb. It refuses options that do not fit the app.
func New(ctx context.Context, opts Options) (*Server, error) {
	if err := check(opts); err != nil {
		return nil, err
	}
	s := &Server{
		m:         opts.Manifest,
		env:       opts.Env,
		version:   opts.Version,
		csrfKey:   bytes.Clone(opts.CSRFKey),
		share:     opts.ShareAppSocket,
		providers: opts.Providers,
		onReady:   opts.OnReady,
		ready:     make(chan struct{}),
	}
	for _, k := range opts.ViewerKeys {
		s.viewers = append(s.viewers, bytes.Clone(k))
	}
	if opts.Adopted {
		s.markReady()
	}
	err := s.listenRunner(ctx, opts.Dir)
	if err == nil && opts.Database != nil {
		err = s.listenDatabase(ctx, opts.Dir, *opts.Database, opts.OnDatabaseError)
	}
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// check refuses opts that do not fit the app of opts.Manifest.
func check(opts Options) error {
	m, p := opts.Manifest, opts.Providers
	switch {
	case opts.Dir == "":
		return errors.New("server: Options.Dir is empty")
	case m.App == "":
		return errors.New("server: Options.Manifest names no app")
	case opts.Env != EnvDev && opts.Env != EnvPreview && opts.Env != EnvProduction:
		return fmt.Errorf("server: Options.Env must be %s, %s or %s", EnvDev, EnvPreview, EnvProduction)
	case len(opts.ViewerKeys) == 0 || slices.ContainsFunc(opts.ViewerKeys, func(k ed25519.PublicKey) bool { return len(k) != ed25519.PublicKeySize }):
		return errors.New("server: Options.ViewerKeys must hold Ed25519 public keys, at least one")
	case len(opts.CSRFKey) < MinCSRFKey:
		return fmt.Errorf("server: Options.CSRFKey must be at least %d bytes", MinCSRFKey)
	case m.SQLDB() != (opts.Database != nil):
		return errors.New("server: Options.Database must be set exactly when the app declares sqldb")
	case len(m.Stores()) > 0 && p.Files == nil:
		return errors.New("server: the app declares file stores, and Providers.Files is nil")
	case m.Email != nil && p.Mail == nil:
		return errors.New("server: the app declares email, and Providers.Mail is nil")
	case len(m.Secrets)+len(m.Settings) > 0 && p.Secrets == nil:
		return errors.New("server: the app declares secrets or settings, and Providers.Secrets is nil")
	}
	return nil
}

func (s *Server) markReady() { s.readyOnce.Do(func() { close(s.ready) }) }

// Ready is closed once the app reports that it accepts requests, or at New for an adopted app.
func (s *Server) Ready() <-chan struct{} { return s.ready }

// SetDatabase makes the proxy on mysql.sock log in as up from the next connection the app opens
// on: sessions already open keep their login. It refuses a server that serves no database.
func (s *Server) SetDatabase(up mysqlproxy.Upstream) error {
	if s.db == nil {
		return errors.New("server: the app declares no sqldb, so the server serves no database")
	}
	s.db.SetUpstream(up)
	return nil
}

// Close stops serving, which removes runner.sock and mysql.sock.
func (s *Server) Close() error {
	var err error
	if s.http != nil {
		err = s.http.Close()
	}
	if s.dbSock != nil {
		if cerr := s.dbSock.Close(); cerr != nil && !errors.Is(cerr, net.ErrClosed) {
			err = errors.Join(err, cerr)
		}
	}
	if s.db != nil {
		s.db.Close()
	}
	return err
}

// listen listens on the Unix socket at path, which only this user can use, or every user when s
// shares its sockets.
func (s *Server) listen(ctx context.Context, path string) (net.Listener, error) {
	if s.share {
		return socket.ListenShared(ctx, path)
	}
	return socket.Listen(ctx, path)
}

func (s *Server) listenRunner(ctx context.Context, dir string) error {
	l, err := s.listen(ctx, filepath.Join(dir, runnerproto.RunnerSocket))
	if err != nil {
		return err
	}
	s.http = socket.NewServer(s.handler())
	s.http.ErrorLog = log.New(io.Discard, "", 0)
	go func() { _ = s.http.Serve(l) }()
	return nil
}

// listenDatabase serves the database on mysql.sock in dir through a proxy that logs in as up,
// and hands each reason it cannot to onErr, if set.
func (s *Server) listenDatabase(ctx context.Context, dir string, up mysqlproxy.Upstream, onErr func(error)) error {
	if onErr == nil {
		onErr = func(error) {}
	}
	l, err := s.listen(ctx, filepath.Join(dir, runnerproto.MySQLSocket))
	if err != nil {
		return err
	}
	s.dbSock = l
	s.db = mysqlproxy.New(up)
	s.db.OnError = onErr
	go serveDB(s.db, l, onErr)
	return nil
}

// serveDB serves p on l until l is closed. When p stops for any other reason, serveDB reports
// why and closes l, so the app's connections fail at once instead of waiting for an accept.
func serveDB(p *mysqlproxy.Proxy, l net.Listener, onErr func(error)) {
	if err := p.Serve(l); err != nil {
		onErr(err)
		_ = l.Close()
	}
}

func (s *Server) handler() http.Handler {
	limit := connect.WithReadMaxBytes(runnerproto.MaxMessageBytes)
	mux := http.NewServeMux()
	mux.Handle(runnerv1connect.NewLifecycleServiceHandler(lifecycleService{s}, limit))
	mux.Handle(runnerv1connect.NewSecretsServiceHandler(secretsService{s}, limit))
	mux.Handle(runnerv1connect.NewTelemetryServiceHandler(telemetryService{s}, limit))
	mux.Handle(runnerv1connect.NewRpcServiceHandler(rpcService{s}, limit))
	mux.Handle(runnerv1connect.NewFilesServiceHandler(&fileService{stores: s.m.Stores(), files: s.providers.Files}, limit))
	mux.Handle(runnerv1connect.NewMailServiceHandler(&mailService{email: s.m.Email, mail: s.providers.Mail}, limit))
	return mux
}

type lifecycleService struct{ s *Server }

func (l lifecycleService) Hello(_ context.Context, req *connect.Request[runnerv1.HelloRequest]) (*connect.Response[runnerv1.HelloResponse], error) {
	if v := req.Msg.GetProtocolVersion(); v != runnerproto.ProtocolVersion {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errs.New("E-RUN-002",
			fmt.Sprintf("the app speaks runner protocol %d, this runner speaks %d", v, runnerproto.ProtocolVersion),
			"build the app with the framework version that matches the runner: update aicoded.dev/framework in go.mod, or update aicoded"))
	}
	s := l.s
	return connect.NewResponse(&runnerv1.HelloResponse{
		App:           s.m.App,
		Env:           s.env,
		Version:       s.version,
		CsrfKey:       s.csrfKey,
		ViewerKeys:    s.viewers,
		Settings:      s.settings(),
		AppSocketOpen: s.share,
	}), nil
}

// settings returns the value of each setting the app declares.
func (s *Server) settings() map[string]string {
	out := make(map[string]string, len(s.m.Settings))
	var values map[string]string
	if s.providers.Secrets != nil {
		values = s.providers.Secrets.Settings()
	}
	for _, n := range s.m.Settings {
		out[n] = values[n]
	}
	return out
}

func (l lifecycleService) Ready(context.Context, *connect.Request[runnerv1.ReadyRequest]) (*connect.Response[runnerv1.ReadyResponse], error) {
	s := l.s
	s.markReady()
	if s.onReady != nil {
		s.onReady()
	}
	return connect.NewResponse(&runnerv1.ReadyResponse{}), nil
}

type secretsService struct{ s *Server }

func (ss secretsService) GetSecret(ctx context.Context, req *connect.Request[runnerv1.GetSecretRequest]) (*connect.Response[runnerv1.GetSecretResponse], error) {
	name := req.Msg.GetName()
	if !slices.Contains(ss.s.m.Secrets, name) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret %s is not declared", quote(name)))
	}
	v, err := ss.s.providers.Secrets.Secret(ctx, name)
	if err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&runnerv1.GetSecretResponse{Value: []byte(v)}), nil
}

type telemetryService struct{ s *Server }

func (t telemetryService) Export(ctx context.Context, req *connect.Request[runnerv1.ExportRequest]) (*connect.Response[runnerv1.ExportResponse], error) {
	spans, dropped := boundSpans(req.Msg.GetSpans())
	if tel := t.s.providers.Telemetry; tel != nil {
		tel.Export(ctx, spans, addSaturating(req.Msg.GetDropped(), dropped))
	}
	return connect.NewResponse(&runnerv1.ExportResponse{}), nil
}

type rpcService struct{ s *Server }

// Call carries a call of the app to another app. It refuses a call that services.calls in the
// app's permission list does not name before Calls sees it.
func (r rpcService) Call(ctx context.Context, req *connect.Request[runnerv1.CallRequest]) (*connect.Response[runnerv1.CallResponse], error) {
	if err := callDeclared(r.s.m, req.Msg.GetApp(), req.Msg.GetMethod()); err != nil {
		return nil, err
	}
	calls := r.s.providers.Calls
	if calls == nil {
		return nil, notRunning(req.Msg.GetApp())
	}
	resp, err := calls.Call(ctx, r.s.m, req.Msg, req.Header().Get("traceparent"))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// providerError returns err, an error of a provider, as the app sees it: a *connect.Error as it
// is, and anything else as an internal error that says nothing of its cause.
func providerError(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
