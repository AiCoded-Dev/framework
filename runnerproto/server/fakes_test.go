package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

var (
	acme    = &manifest.Email{From: "ops@acme.example", ToDomains: []string{"acme.example"}}
	partner = manifest.Egress{Host: "api.partner.example", Secret: "partner_api_key", Header: "Authorization", Prefix: "Bearer "}
	notes   = manifest.Manifest{App: "notes", Settings: []string{"greeting"}, Secrets: []string{"partner_api_key", "token"}, Email: acme,
		Data:   []manifest.Data{{Source: "filestore:docs", Classes: []string{"internal"}}},
		Egress: []manifest.Egress{partner, {Host: "status.partner.example"}, {Host: "xn--bcher-kva.example"}}}
	csrfKey = bytes.Repeat([]byte{7}, MinCSRFKey)
)

// fakes are the providers of a test server, each keeping what reached it.
type fakes struct {
	files     *fakeFiles
	mail      *fakeMail
	secrets   *fakeSecrets
	telemetry *fakeTelemetry
	outbound  *fakeOutbound
}

func newFakes() *fakes {
	return &fakes{
		files: &fakeFiles{entries: map[string]*fakeEntry{}},
		mail:  &fakeMail{},
		secrets: &fakeSecrets{values: map[string]string{"token": "s3cret", "other": "x", "partner_api_key": partnerKey},
			settings: map[string]string{"greeting": "hi", "extra": "x"}},
		telemetry: &fakeTelemetry{},
		outbound:  &fakeOutbound{},
	}
}

func (f *fakes) providers() Providers {
	return Providers{Files: f.files, Mail: f.mail, Secrets: f.secrets, Telemetry: f.telemetry, Outbound: f.outbound}
}

// shortDir returns a new folder whose socket paths fit a Unix socket address.
func shortDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "rs-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// options returns the options of a server for notes in a folder of its own.
func options(t *testing.T, f *fakes) Options {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return Options{Dir: shortDir(t), Manifest: notes, Env: EnvProduction, Version: "r-1",
		ViewerKeys: []ed25519.PublicKey{pub}, CSRFKey: csrfKey, Providers: f.providers()}
}

// start serves opts until the test ends.
func start(t *testing.T, opts Options) *Server {
	s, err := New(t.Context(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// clients talk to a server as its app does.
type clients struct {
	life    runnerv1connect.LifecycleServiceClient
	secrets runnerv1connect.SecretsServiceClient
	tel     runnerv1connect.TelemetryServiceClient
	rpc     runnerv1connect.RpcServiceClient
	files   runnerv1connect.FilesServiceClient
	mail    runnerv1connect.MailServiceClient
	out     runnerv1connect.OutboundServiceClient
}

func dial(dir string) clients {
	hc := socket.Client(filepath.Join(dir, runnerproto.RunnerSocket))
	o := connect.WithGRPC()
	return clients{
		life:    runnerv1connect.NewLifecycleServiceClient(hc, socket.BaseURL, o),
		secrets: runnerv1connect.NewSecretsServiceClient(hc, socket.BaseURL, o),
		tel:     runnerv1connect.NewTelemetryServiceClient(hc, socket.BaseURL, o),
		rpc:     runnerv1connect.NewRpcServiceClient(hc, socket.BaseURL, o),
		files:   runnerv1connect.NewFilesServiceClient(hc, socket.BaseURL, o),
		mail:    runnerv1connect.NewMailServiceClient(hc, socket.BaseURL, o),
		out:     runnerv1connect.NewOutboundServiceClient(hc, socket.BaseURL, o),
	}
}

// serve starts a server for notes on fresh fakes and returns its clients and the fakes.
func serve(t *testing.T) (clients, *fakes) {
	f := newFakes()
	opts := options(t, f)
	start(t, opts)
	return dial(opts.Dir), f
}

// recorder keeps the calls that reached a fake, and fails each with err when set.
type recorder struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (r *recorder) record(call ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, strings.Join(call, " "))
	return r.err
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recorder) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// fakeFiles keeps files and folders in memory, by store and name.
type fakeFiles struct {
	recorder
	entries map[string]*fakeEntry
}

type fakeEntry struct {
	dir  bool
	data []byte
}

func key(store, name string) string { return store + ":" + name }

func (f *fakeFiles) get(store, name string) (*fakeEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "." {
		return &fakeEntry{dir: true}, nil
	}
	e := f.entries[key(store, name)]
	if e == nil {
		return nil, fs.ErrNotExist
	}
	return e, nil
}

func (f *fakeFiles) put(store, name string, e *fakeEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key(store, name)] = e
}

func info(name string, e *fakeEntry) FileInfo {
	return FileInfo{Name: path.Base(name), Size: int64(len(e.data)), Dir: e.dir, ModTime: time.Unix(1, 0)}
}

func (f *fakeFiles) Stat(_ context.Context, store, name string) (FileInfo, error) {
	if err := f.record("stat", store, name); err != nil {
		return FileInfo{}, err
	}
	e, err := f.get(store, name)
	if err != nil {
		return FileInfo{}, err
	}
	return info(name, e), nil
}

func (f *fakeFiles) ReadDir(_ context.Context, store, name string) ([]FileInfo, error) {
	if err := f.record("readdir", store, name); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []FileInfo
	for k, e := range f.entries {
		if p, ok := strings.CutPrefix(k, store+":"); ok && path.Dir(p) == name {
			out = append(out, info(p, e))
		}
	}
	return out, nil
}

func (f *fakeFiles) Read(_ context.Context, store, name string) (io.ReadCloser, error) {
	if err := f.record("read", store, name); err != nil {
		return nil, err
	}
	e, err := f.get(store, name)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(e.data)), nil
}

func (f *fakeFiles) Write(_ context.Context, store, name string, data io.Reader) (FileInfo, error) {
	if err := f.record("write", store, name); err != nil {
		return FileInfo{}, err
	}
	if dir, err := f.get(store, path.Dir(name)); err != nil || !dir.dir {
		return FileInfo{}, fs.ErrNotExist
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return FileInfo{}, err
	}
	e := &fakeEntry{data: b}
	f.put(store, name, e)
	return info(name, e), nil
}

func (f *fakeFiles) Mkdir(_ context.Context, store, name string) error {
	if err := f.record("mkdir", store, name); err != nil {
		return err
	}
	if e, err := f.get(store, name); err == nil && !e.dir {
		return fs.ErrExist
	}
	f.put(store, name, &fakeEntry{dir: true})
	return nil
}

func (f *fakeFiles) Remove(_ context.Context, store, name string) error {
	if err := f.record("remove", store, name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.entries[key(store, name)] == nil {
		return syscall.ENOENT
	}
	delete(f.entries, key(store, name))
	return nil
}

func (f *fakeFiles) Rename(_ context.Context, store, oldName, newName string) error {
	if err := f.record("rename", store, oldName, newName); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.entries[key(store, oldName)]
	if e == nil {
		return fs.ErrNotExist
	}
	delete(f.entries, key(store, oldName))
	f.entries[key(store, newName)] = e
	return nil
}

// fakeMail keeps the mail it is asked to send and answers mailbox calls with a fixed message.
type fakeMail struct {
	recorder
	sent []*runnerv1.SendRequest
	page int // how many messages List returns
}

func (f *fakeMail) Send(_ context.Context, from string, msg *runnerv1.SendRequest) (string, error) {
	if err := f.record("send", from); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return "m-1", nil
}

func (f *fakeMail) List(_ context.Context, folder string, offset, limit int) ([]*runnerv1.MessageSummary, int, error) {
	if err := f.record("list", folder, strconv.Itoa(offset), strconv.Itoa(limit)); err != nil {
		return nil, 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*runnerv1.MessageSummary, f.page)
	for i := range out {
		out[i] = &runnerv1.MessageSummary{Id: strconv.Itoa(i), Folder: folder}
	}
	return out, 1 << 40, nil
}

func (f *fakeMail) Get(_ context.Context, id string) (*runnerv1.GetResponse, error) {
	if err := f.record("get", id); err != nil {
		return nil, err
	}
	return &runnerv1.GetResponse{Summary: &runnerv1.MessageSummary{Id: id}, Text: "hello"}, nil
}

func (f *fakeMail) Delete(_ context.Context, id string) error { return f.record("delete", id) }

func (f *fakeMail) Move(_ context.Context, id, folder string) error {
	return f.record("move", id, folder)
}

type fakeSecrets struct {
	recorder
	values, settings map[string]string
}

func (f *fakeSecrets) Secret(_ context.Context, name string) (string, error) {
	if err := f.record("secret", name); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[name], nil
}

func (f *fakeSecrets) set(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[name] = value
}

func (f *fakeSecrets) Settings() map[string]string { return f.settings }

type fakeTelemetry struct {
	mu      sync.Mutex
	spans   []*runnerv1.Span
	dropped uint64
}

func (f *fakeTelemetry) Export(_ context.Context, spans []*runnerv1.Span, dropped uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spans = append(f.spans, spans...)
	f.dropped += dropped
}

func (f *fakeTelemetry) got() ([]*runnerv1.Span, uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spans, f.dropped
}

// testMinter signs viewer and call tokens with a key of its own, as a runner does.
type testMinter struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func newMinter(t *testing.T) *testMinter {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &testMinter{pub: pub, priv: priv}
}

func (m *testMinter) Public() ed25519.PublicKey { return m.pub }

func (m *testMinter) MintCall(v *viewer.Claims, caller, callee string, now time.Time) (string, error) {
	c := viewer.Claims{Audience: callee, Caller: caller, IssuedAt: now.Unix(), Expires: now.Add(viewer.MaxLifetime).Unix()}
	if v == nil {
		c.App = true
	} else {
		c.Subject, c.Name, c.Groups, c.Roles = v.Subject, v.Name, v.Groups, v.Roles
	}
	return viewer.Sign(m.priv, c)
}

// mint returns a token of the viewer subject with roles for app, issued at.
func (m *testMinter) mint(t *testing.T, subject string, roles []string, app string, at time.Time) string {
	token, err := viewer.Sign(m.priv, viewer.Claims{Subject: subject, Name: subject, Roles: roles, Audience: app,
		IssuedAt: at.Unix(), Expires: at.Add(viewer.MaxLifetime).Unix()})
	require.NoError(t, err)
	return token
}
