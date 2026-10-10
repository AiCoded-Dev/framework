package dev

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/mysqlproxy"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
)

var notesManifest = manifest.Manifest{App: "notes", Settings: []string{"greeting"}, Secrets: []string{"token"}, Email: acme,
	Data: []manifest.Data{{Source: "sqldb", Classes: []string{"internal"}}, {Source: "filestore:docs", Classes: []string{"internal"}}}}

// runnerOptions returns options for a runner of notesManifest in a short folder of its own.
func runnerOptions(t *testing.T, share bool) RunnerOptions {
	dir, err := os.MkdirTemp("", "aicoded-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return RunnerOptions{Dir: dir, FilesDir: filepath.Join(dir, "files"), Values: helloValues, Env: devconfig.EnvPreview,
		Version: "v1.2.3", ViewerKey: pub, Share: share, MaxSpans: 10, MaxSpanBytes: 1000,
		Database: &mysqlproxy.Upstream{Network: "unix", Address: filepath.Join(dir, "no-server.sock")}}
}

func mode(t *testing.T, path string) os.FileMode {
	st, err := os.Stat(path)
	require.NoError(t, err)
	return st.Mode().Perm()
}

func TestRunnerServesOneApp(t *testing.T) {
	opts := runnerOptions(t, true)
	failed := make(chan error, 1)
	opts.OnDatabaseError = func(err error) { failed <- err }
	r, err := NewRunner(t.Context(), notesManifest, opts)
	require.NoError(t, err)
	sock := filepath.Join(opts.Dir, runnerproto.RunnerSocket)
	assert.Equal(t, os.FileMode(0o666), mode(t, sock))
	assert.Equal(t, os.FileMode(0o666), mode(t, filepath.Join(opts.Dir, runnerproto.MySQLSocket)))
	assert.DirExists(t, filepath.Join(opts.FilesDir, "docs"))

	hc := socket.Client(sock)
	lc := runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	hello, err := lc.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	assert.Equal(t, "notes", hello.Msg.GetApp())
	assert.Equal(t, "preview", hello.Msg.GetEnv())
	assert.Equal(t, "v1.2.3", hello.Msg.GetVersion())
	assert.True(t, hello.Msg.GetAppSocketOpen())
	assert.Equal(t, [][]byte{opts.ViewerKey}, hello.Msg.GetViewerKeys())
	assert.Len(t, hello.Msg.GetCsrfKey(), csrfKeySize)

	mail := runnerv1connect.NewMailServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	_, err = mail.Send(t.Context(), connect.NewRequest(msg("ana@acme.example")))
	require.NoError(t, err)
	assert.Len(t, r.store.mail.sentMail(), 1, "mail is caught and kept")

	tel := runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, connect.WithGRPC())
	_, err = tel.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "hello"}}, Dropped: 2}))
	require.NoError(t, err)
	spans, dropped := r.Spans()
	require.Len(t, spans, 1)
	assert.Equal(t, "hello", spans[0].Name)
	assert.Equal(t, uint64(2), dropped, "spans the app dropped count too")

	select {
	case <-r.Ready():
		t.Fatal("ready before the app said so")
	default:
	}
	_, err = lc.Ready(t.Context(), connect.NewRequest(&runnerv1.ReadyRequest{}))
	require.NoError(t, err)
	<-r.Ready()

	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "unix", filepath.Join(opts.Dir, runnerproto.MySQLSocket))
	require.NoError(t, err)
	defer conn.Close()
	select {
	case err := <-failed:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the database proxy's failure was not reported")
	}

	r.Close()
	assert.NoFileExists(t, sock)
	assert.NoFileExists(t, filepath.Join(opts.Dir, runnerproto.MySQLSocket))
}

func TestRunnerKeepsItsSocketsPrivateUnlessShared(t *testing.T) {
	opts := runnerOptions(t, false)
	r, err := NewRunner(t.Context(), notesManifest, opts)
	require.NoError(t, err)
	t.Cleanup(r.Close)
	sock := filepath.Join(opts.Dir, runnerproto.RunnerSocket)
	assert.Equal(t, os.FileMode(0o600), mode(t, sock))
	assert.Equal(t, os.FileMode(0o600), mode(t, filepath.Join(opts.Dir, runnerproto.MySQLSocket)))
	lc := runnerv1connect.NewLifecycleServiceClient(socket.Client(sock), socket.BaseURL, connect.WithGRPC())
	hello, err := lc.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	assert.False(t, hello.Msg.GetAppSocketOpen())
}

func TestNewRunnerRefuses(t *testing.T) {
	for name, change := range map[string]func(*manifest.Manifest, *RunnerOptions){
		"an unknown env":           func(_ *manifest.Manifest, o *RunnerOptions) { o.Env = "prod" },
		"no viewer key":            func(_ *manifest.Manifest, o *RunnerOptions) { o.ViewerKey = nil },
		"no span bound":            func(_ *manifest.Manifest, o *RunnerOptions) { o.MaxSpans = 0 },
		"no span byte bound":       func(_ *manifest.Manifest, o *RunnerOptions) { o.MaxSpanBytes = 0 },
		"sqldb without a database": func(_ *manifest.Manifest, o *RunnerOptions) { o.Database = nil },
		"a database without sqldb": func(m *manifest.Manifest, _ *RunnerOptions) { m.Data = m.Data[1:] },
		"a missing secret": func(_ *manifest.Manifest, o *RunnerOptions) {
			o.Values = devconfig.AppValues{Settings: helloValues.Settings}
		},
	} {
		m := notesManifest
		m.Data = append([]manifest.Data(nil), m.Data...)
		opts := runnerOptions(t, true)
		change(&m, &opts)
		_, err := NewRunner(t.Context(), m, opts)
		require.Error(t, err, name)
		assert.NoFileExists(t, filepath.Join(opts.Dir, runnerproto.RunnerSocket), name)
	}
}
