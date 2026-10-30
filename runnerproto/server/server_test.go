package server

import (
	"crypto/ed25519"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/mysqlproxy"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/socket"
)

func hello(t *testing.T, c clients) *runnerv1.HelloResponse {
	resp, err := c.life.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	return resp.Msg
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func mode(t *testing.T, path string) os.FileMode {
	st, err := os.Stat(path)
	require.NoError(t, err)
	return st.Mode().Perm()
}

// withDatabase makes opts serve an app that declares sqldb, whose database no server holds.
func withDatabase(opts *Options) {
	opts.Manifest.Data = append([]manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}}, opts.Manifest.Data...)
	opts.Database = &mysqlproxy.Upstream{Network: "unix", Address: filepath.Join(opts.Dir, "no-server.sock")}
}

func TestNewRefuses(t *testing.T) {
	for name, change := range map[string]func(*Options){
		"no folder":                 func(o *Options) { o.Dir = "" },
		"no app":                    func(o *Options) { o.Manifest.App = "" },
		"an unknown env":            func(o *Options) { o.Env = "prod" },
		"no env":                    func(o *Options) { o.Env = "" },
		"no viewer key":             func(o *Options) { o.ViewerKeys = nil },
		"a short viewer key":        func(o *Options) { o.ViewerKeys = append(o.ViewerKeys, ed25519.PublicKey("short")) },
		"no CSRF key":               func(o *Options) { o.CSRFKey = nil },
		"a short CSRF key":          func(o *Options) { o.CSRFKey = csrfKey[:MinCSRFKey-1] },
		"sqldb without a database":  func(o *Options) { withDatabase(o); o.Database = nil },
		"a database without sqldb":  func(o *Options) { withDatabase(o); o.Manifest.Data = notes.Data },
		"stores without Files":      func(o *Options) { o.Providers.Files = nil },
		"email without Mail":        func(o *Options) { o.Providers.Mail = nil },
		"secrets without Secrets":   func(o *Options) { o.Manifest.Settings = nil; o.Providers.Secrets = nil },
		"settings without Secrets":  func(o *Options) { o.Manifest.Secrets, o.Manifest.Egress, o.Providers.Secrets = nil, nil, nil },
		"egress without Outbound":   func(o *Options) { o.Providers.Outbound = nil },
		"Egress beyond the list":    func(o *Options) { o.Egress = []manifest.Egress{{Host: "evil.example"}} },
		"Egress with another key":   func(o *Options) { o.Egress = []manifest.Egress{{Host: partner.Host, Secret: "token", Header: "X-Key"}} },
		"a key not in secrets":      func(o *Options) { o.Manifest.Secrets = []string{"token"} },
		"a socket path that is bad": func(o *Options) { o.Dir = filepath.Join(o.Dir, "missing") },
	} {
		opts := options(t, newFakes())
		change(&opts)
		_, err := New(t.Context(), opts)
		require.Error(t, err, name)
		assert.NoFileExists(t, filepath.Join(opts.Dir, runnerproto.RunnerSocket), name)
	}
}

func TestNewWithoutProvidersTheAppDoesNotNeed(t *testing.T) {
	opts := options(t, newFakes())
	opts.Manifest, opts.Providers = manifest.Manifest{App: "bare"}, Providers{}
	start(t, opts)
	c := dial(opts.Dir)
	assert.Empty(t, hello(t, c).GetSettings())
	_, err := c.files.Stat(t.Context(), connect.NewRequest(&runnerv1.StatRequest{Store: "docs", Name: "."}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = c.mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	_, err = c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "token"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = c.tel.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "a"}}}))
	require.NoError(t, err, "spans without Telemetry are dropped")
	_, err = c.out.Do(t.Context(), connect.NewRequest(get("https://api.partner.example/")))
	assert.Equal(t, "E-OUT-001", coded(t, err).Code)
}

func TestHello(t *testing.T) {
	for _, env := range []string{EnvDev, EnvPreview, EnvProduction} {
		f := newFakes()
		opts := options(t, f)
		opts.Env = env
		key := append([]byte(nil), csrfKey...)
		opts.CSRFKey = key
		start(t, opts)
		key[0] ^= 0xff
		h := hello(t, dial(opts.Dir))
		assert.Equal(t, "notes", h.GetApp())
		assert.Equal(t, env, h.GetEnv())
		assert.Equal(t, "r-1", h.GetVersion())
		assert.Equal(t, csrfKey, h.GetCsrfKey(), "the key as it was at New")
		assert.Equal(t, [][]byte{opts.ViewerKeys[0]}, h.GetViewerKeys())
		assert.Equal(t, map[string]string{"greeting": "hi"}, h.GetSettings(), "only declared settings")
		assert.False(t, h.GetAppSocketOpen())
	}

}

func TestHelloVersions(t *testing.T) {
	sayHello := func(c clients, v uint32) error {
		_, err := c.life.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: v}))
		return err
	}
	bare, _ := serveWith(t, func(o *Options) { o.Manifest.Egress = nil })
	for _, v := range []uint32{runnerproto.MinProtocolVersion, runnerproto.ProtocolVersion} {
		require.NoError(t, sayHello(bare, v), v)
	}
	for _, v := range []uint32{runnerproto.MinProtocolVersion - 1, runnerproto.ProtocolVersion + 1, 99} {
		err := sayHello(bare, v)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
		assert.Equal(t, "E-RUN-002", coded(t, err).Code, v)
	}

	c, _ := serve(t)
	require.NoError(t, sayHello(c, 4))
	err := sayHello(c, 3)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Equal(t, "E-RUN-014", coded(t, err).Code, "protocol 3 has no outbound")
	assert.Equal(t, "update aicoded.dev/framework to v0.10.0 or newer", coded(t, err).Fix)

	preview, _ := serveWith(t, func(o *Options) { o.Env = EnvPreview; o.Egress = []manifest.Egress{} })
	assert.Equal(t, "E-RUN-014", coded(t, sayHello(preview, 3)).Code, "by the permission list, whatever the preview reaches")
}

func TestReady(t *testing.T) {
	f := newFakes()
	opts := options(t, f)
	readies := make(chan struct{}, 2)
	opts.OnReady = func() { readies <- struct{}{} }
	s := start(t, opts)
	assert.False(t, isClosed(s.Ready()), "ready before the app said so")
	c := dial(opts.Dir)
	for range 2 {
		_, err := c.life.Ready(t.Context(), connect.NewRequest(&runnerv1.ReadyRequest{}))
		require.NoError(t, err)
	}
	assert.True(t, isClosed(s.Ready()))
	assert.Len(t, readies, 2, "OnReady hears every Ready")
}

func TestAdoptedClosesReadyAtNew(t *testing.T) {
	opts := options(t, newFakes())
	opts.Adopted = true
	readies := make(chan struct{}, 1)
	opts.OnReady = func() { readies <- struct{}{} }
	s := start(t, opts)
	assert.True(t, isClosed(s.Ready()))
	assert.Empty(t, readies, "OnReady hears only the Ready the app sends")

	_, err := dial(opts.Dir).life.Ready(t.Context(), connect.NewRequest(&runnerv1.ReadyRequest{}))
	require.NoError(t, err)
	assert.Len(t, readies, 1)
}

func TestSockets(t *testing.T) {
	opts := options(t, newFakes())
	withDatabase(&opts)
	failed := make(chan error, 1)
	opts.OnDatabaseError = func(err error) { failed <- err }
	s := start(t, opts)
	sock, db := filepath.Join(opts.Dir, runnerproto.RunnerSocket), filepath.Join(opts.Dir, runnerproto.MySQLSocket)
	assert.Equal(t, os.FileMode(0o600), mode(t, sock))
	assert.Equal(t, os.FileMode(0o600), mode(t, db))

	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "unix", db)
	require.NoError(t, err)
	defer conn.Close()
	select {
	case err := <-failed:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the database proxy's failure was not reported")
	}

	require.NoError(t, s.Close())
	assert.NoFileExists(t, sock)
	assert.NoFileExists(t, db)
}

func TestSetDatabase(t *testing.T) {
	opts := options(t, newFakes())
	withDatabase(&opts)
	s := start(t, opts)
	upstream := filepath.Join(shortDir(t), "db.sock")
	l, err := socket.Listen(t.Context(), upstream)
	require.NoError(t, err)
	defer l.Close()
	require.NoError(t, s.SetDatabase(mysqlproxy.Upstream{Network: "unix", Address: upstream}))

	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "unix", filepath.Join(opts.Dir, runnerproto.MySQLSocket))
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, l.(*net.UnixListener).SetDeadline(time.Now().Add(5*time.Second)))
	up, err := l.Accept()
	require.NoError(t, err, "the next connection dials the new upstream")
	_ = up.Close()
}

func TestSetDatabaseRefusesAServerWithoutOne(t *testing.T) {
	s := start(t, options(t, newFakes()))
	require.Error(t, s.SetDatabase(mysqlproxy.Upstream{Network: "unix", Address: "/elsewhere.sock"}))
}

// failingAccept is a listener whose Accept fails as it does when the runner is out of files.
type failingAccept struct{ net.Listener }

func (failingAccept) Accept() (net.Conn, error) { return nil, syscall.EMFILE }

func TestServeDBClosesTheSocketWhenItFails(t *testing.T) {
	sock := filepath.Join(shortDir(t), runnerproto.MySQLSocket)
	l, err := socket.Listen(t.Context(), sock)
	require.NoError(t, err)

	var logged []error
	serveDB(mysqlproxy.New(mysqlproxy.Upstream{}), failingAccept{l}, func(err error) { logged = append(logged, err) })
	assert.Equal(t, []error{syscall.EMFILE}, logged)
	var d net.Dialer
	_, err = d.DialContext(t.Context(), "unix", sock)
	require.Error(t, err, "the app's connections fail at once")
}

func TestSharedSockets(t *testing.T) {
	opts := options(t, newFakes())
	withDatabase(&opts)
	opts.ShareAppSocket = true
	start(t, opts)
	assert.Equal(t, os.FileMode(0o666), mode(t, filepath.Join(opts.Dir, runnerproto.RunnerSocket)))
	assert.Equal(t, os.FileMode(0o666), mode(t, filepath.Join(opts.Dir, runnerproto.MySQLSocket)))
	assert.True(t, hello(t, dial(opts.Dir)).GetAppSocketOpen())
}

func TestSecrets(t *testing.T) {
	c, f := serve(t)
	resp, err := c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "token"}))
	require.NoError(t, err)
	assert.Equal(t, []byte("s3cret"), resp.Msg.GetValue())

	_, err = c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "other"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "undeclared secrets are never handed out")
	assert.Equal(t, []string{"secret token"}, f.secrets.seen(), "nor asked for")

	f.secrets.fail(errors.New("vault at 10.0.0.7 refused token hvs.abc"))
	_, err = c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "token"}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "10.0.0.7", "the provider's own text never reaches the app")
	f.secrets.fail(connect.NewError(connect.CodeNotFound, errors.New("secret \"token\" has no value")))
	_, err = c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "token"}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "a connect error passes as it is")
	require.ErrorContains(t, err, "has no value")
}

func TestSecretsRefusesABoundKey(t *testing.T) {
	for name, change := range map[string]func(*Options){
		"production":                 func(*Options) {},
		"a preview without the host": func(o *Options) { o.Env = EnvPreview; o.Egress = []manifest.Egress{} },
	} {
		c, f := serveWith(t, change)
		_, err := c.secrets.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: "partner_api_key"}))
		e := coded(t, err)
		assert.Equal(t, "E-RUN-015", e.Code, name)
		assert.Equal(t, "the runner adds it to every request to api.partner.example: call the service with outbound.Client() and do not read the secret", e.Fix, name)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), name)
		assert.NotContains(t, err.Error(), partnerKey, name)
		assert.Empty(t, f.secrets.seen(), "the key is never read for the app")
	}
}

func TestOversizedMessagesAreRefused(t *testing.T) {
	c, f := serve(t)
	big := &runnerv1.Span{Name: "a", Attributes: map[string]string{"k": string(make([]byte, runnerproto.MaxMessageBytes))}}
	_, err := c.tel.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{big}}))
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
	spans, _ := f.telemetry.got()
	assert.Empty(t, spans)
}
