package dev

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

func getApp(t *testing.T, inst *Instance, signer *Signer, path string) string {
	token, err := signer.Mint(devconfig.Persona{Name: "viewer"}, inst.Name, time.Now())
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://app"+path, nil)
	require.NoError(t, err)
	req.Header.Set(viewer.Header, token)
	resp, err := socket.Client(inst.AppSocket()).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestStartRunsTheApp(t *testing.T) {
	requireGo(t)
	t.Setenv("AICODED_TEST_LEAK", "must not reach the app")
	signer, err := NewSigner()
	require.NoError(t, err)
	var out syncBuffer

	state := t.TempDir()
	m := helloManifest(t)
	m.Data = []manifest.Data{{Source: "filestore:docs", Classes: []string{"internal"}}}
	m.Email = &manifest.Email{From: "hello@acme.example", ToDomains: []string{"acme.example"}}
	inst, err := Start(t.Context(), helloDir(t), m, helloValues, Services{StateDir: state}, signer, &out)
	require.NoError(t, err)
	assert.Equal(t, m, inst.Manifest)
	st, err := os.Stat(inst.runDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), st.Mode().Perm())
	assert.DirExists(t, filepath.Join(state, "files", "hello", "docs"), "file stores live under the state folder")
	assert.NoFileExists(t, filepath.Join(inst.runDir, runnerproto.MySQLSocket), "no mysql.sock without sqldb")
	assert.Equal(t, appSysProcAttr(), inst.cmd.SysProcAttr, "the app gets SIGTERM when aicoded dev dies")
	assert.FileExists(t, filepath.Join(state, "bin", "hello"), "the app is built into the state folder")

	assert.Equal(t, "hi viewer (token 3 bytes)", getApp(t, inst, signer, "/"))
	assert.Equal(t, runnerproto.EnvRunnerDir+"="+inst.runDir, getApp(t, inst, signer, "/env"), "the app gets exactly one variable")

	id := inst.store.mail.deliver(&runnerv1.GetResponse{Text: "hi"})
	mail := runnerv1connect.NewMailServiceClient(socket.Client(filepath.Join(inst.runDir, runnerproto.RunnerSocket)), socket.BaseURL, connect.WithGRPC())
	got, err := mail.Get(t.Context(), connect.NewRequest(&runnerv1.GetRequest{Id: id}))
	require.NoError(t, err, "the mailbox is served on the runner socket")
	assert.Equal(t, "hi", got.Msg.GetText())
	assert.Empty(t, inst.store.mail.sentMail())

	inst.Stop()
	_, err = os.Stat(inst.runDir)
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.True(t, slices.ContainsFunc(inst.store.spans.All(), func(s devapi.Span) bool { return s.Name == "hello" && s.App == "hello" }))
}

func TestStartNeedsLocalValues(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	_, err = Start(t.Context(), helloDir(t), helloManifest(t), devconfig.AppValues{}, Services{StateDir: t.TempDir()}, signer, io.Discard)
	assert.Equal(t, "E-DEV-003", errs.Code(err))
	require.ErrorContains(t, err, "setting greeting, secret token")
}

func TestStartNeedsAnAbsoluteStateDir(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	for _, dir := range []string{"", "state"} {
		_, err := Start(t.Context(), helloDir(t), helloManifest(t), devconfig.AppValues{}, Services{StateDir: dir}, signer, io.Discard)
		require.ErrorContains(t, err, "StateDir", "%q", dir)
	}
}

func TestStartNeedsAKnownEnv(t *testing.T) {
	signer, err := NewSigner()
	require.NoError(t, err)
	_, err = Start(t.Context(), helloDir(t), helloManifest(t), helloValues, Services{StateDir: t.TempDir(), Env: "prod"}, signer, io.Discard)
	require.ErrorContains(t, err, "Services.Env")
}

func TestRunDirTooLong(t *testing.T) {
	base := filepath.Join(t.TempDir(), strings.Repeat("d", maxSocketPath))
	require.NoError(t, os.Mkdir(base, 0o700))
	t.Setenv("XDG_RUNTIME_DIR", base)
	_, err := newRunDir()
	assert.Equal(t, "E-DEV-005", errs.Code(err))
	require.ErrorContains(t, err, "set XDG_RUNTIME_DIR")

	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", base)
	_, err = newRunDir()
	require.ErrorContains(t, err, "set TMPDIR")
	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStartCallsOutsideServices(t *testing.T) {
	requireGo(t)
	signer, err := NewSigner()
	require.NoError(t, err)
	got := make(chan http.Header, 1)
	opts := fakeService(t, "93.184.216.34", func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
		_, _ = w.Write([]byte("hello from the service"))
	})
	m := helloManifest(t)
	m.Secrets = append(m.Secrets, "partner_key")
	m.Egress = []manifest.Egress{{Host: "api.example.com", Secret: "partner_key", Header: "Authorization", Prefix: "Bearer "}}
	values := devconfig.AppValues{Settings: helloValues.Settings, Secrets: map[string]string{"token": "abc", "partner_key": "p4rtner-key"}}
	var out syncBuffer
	inst, err := Start(t.Context(), helloDir(t), m, values, Services{StateDir: t.TempDir(), egress: opts}, signer, &out)
	require.NoError(t, err)

	assert.Equal(t, "200 hello from the service", getApp(t, inst, signer, "/call"))
	assert.Equal(t, "Bearer p4rtner-key", (<-got).Get("Authorization"), "the runner adds the bound key")
	assert.Contains(t, getApp(t, inst, signer, "/bound"), "E-RUN-015", "the app never reads a bound key")

	inst.Stop()
	i := slices.IndexFunc(inst.store.spans.All(), func(s devapi.Span) bool { return s.Name == "outbound" })
	require.GreaterOrEqual(t, i, 0, "the request's span is kept")
	sp := inst.store.spans.All()[i]
	assert.Equal(t, "hello", sp.App)
	assert.Equal(t, "api.example.com", sp.Attrs["server.address"])
	assert.Equal(t, "200", sp.Attrs["http.response.status_code"])
	assert.NotContains(t, fmt.Sprint(inst.store.spans.All(), inst.store.logs.All()), "p4rtner-key")
	assert.NotContains(t, out.String(), "p4rtner-key")
}
