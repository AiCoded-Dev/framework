package l7runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/runnerproto/socket"
	"aicoded.dev/framework/runnerproto/viewer"
)

const (
	helloYAML = "app: hello\nsettings: [greeting]\nsecrets: [token]\naccess: {}\n"
	mailYAML  = "app: hello\nemail: {from: hello@acme.example, to_domains: [acme.example]}\naccess: {}\n"
	sqlYAML   = "app: notes\ndata:\n  - {source: sqldb, classes: [internal]}\naccess: {}\n"
)

// binDir holds the app the tests build, once per test binary.
var binDir string

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

var buildHello = sync.OnceValues(func() (string, error) {
	dir, err := os.MkdirTemp("", "l7-bin-")
	if err != nil {
		return "", err
	}
	binDir = dir
	src, err := filepath.Abs(filepath.Join("..", "dev", "testdata", "hello"))
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "hello")
	problems, err := gotool.BuildBinary(context.Background(), src, bin)
	if err == nil && len(problems) > 0 {
		err = problems[0]
	}
	return bin, err
})

// run is one run of the runner, in folders of its own.
type run struct {
	dir, runnerDir, control, state, app string
	stderr                              syncBuffer
	done                                chan struct{}
	code                                int
	cancel                              context.CancelFunc
}

// folders makes the folders of a run as the delivery pipeline does, with the permission list
// manifestYAML in the app's tree.
func folders(t *testing.T, manifestYAML string) *run {
	dir, err := os.MkdirTemp("", "l7-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r := &run{dir: dir, runnerDir: filepath.Join(dir, "run"), control: filepath.Join(dir, "ctl", "control.sock"),
		state: filepath.Join(dir, "state"), app: filepath.Join(dir, "app")}
	mkdir(t, r.runnerDir, fs.ModeSticky|0o733)
	mkdir(t, filepath.Dir(r.control), 0o700)
	mkdir(t, r.state, 0o700)
	mkdir(t, r.app, 0o700)
	require.NoError(t, os.WriteFile(filepath.Join(r.app, manifest.FileName), []byte(manifestYAML), 0o600))
	return r
}

func mkdir(t *testing.T, dir string, mode fs.FileMode) {
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, mode))
}

func (r *run) args(extra ...string) []string {
	return append([]string{"--app", r.app, "--runner-dir", r.runnerDir, "--control", r.control, "--state", r.state}, extra...)
}

// dsn writes dsn to a file of mode perm and returns the database flags that name it.
func (r *run) dsn(t *testing.T, dsn string, perm fs.FileMode, database string) []string {
	file := filepath.Join(r.dir, "l7.dsn")
	require.NoError(t, os.WriteFile(file, []byte(dsn+"\n"), 0o600))
	require.NoError(t, os.Chmod(file, perm))
	return []string{"--mysql-dsn-file", file, "--mysql-database", database}
}

// start runs the runner with args and waits until its control API answers.
func (r *run) start(t *testing.T, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel, r.done = cancel, make(chan struct{})
	go func() {
		r.code = Main(ctx, args, &r.stderr)
		close(r.done)
	}()
	t.Cleanup(func() { r.stop() })
	for {
		if _, err := r.get(t.Context(), "/v1/status"); err == nil {
			return
		}
		select {
		case <-r.done:
			t.Fatalf("the runner exited with %d: %s", r.code, r.stderr.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// stop stops the runner as SIGTERM does and returns its exit code.
func (r *run) stop() int {
	r.cancel()
	<-r.done
	return r.code
}

func (r *run) client() *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", r.control)
	}}}
}

func (r *run) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://control"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (r *run) status(t *testing.T) statusResponse {
	body, err := r.get(t.Context(), "/v1/status")
	require.NoError(t, err)
	var st statusResponse
	require.NoError(t, json.Unmarshal(body, &st))
	return st
}

func (r *run) spans(t *testing.T) spansResponse {
	body, err := r.get(t.Context(), "/v1/spans")
	require.NoError(t, err)
	var out spansResponse
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

// token asks the control API for a token of kind for subject.
func (r *run) token(t *testing.T, subject, kind string) string {
	body, _ := json.Marshal(tokenRequest{Subject: subject, Roles: []string{}, Kind: kind})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://control/v1/token", bytes.NewReader(body))
	require.NoError(t, err)
	resp, err := r.client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out tokenResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out.Token
}

// startApp starts the app the tests build, as the delivery pipeline does: with only
// AICODED_RUNNER_DIR in its environment.
func (r *run) startApp(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	bin, err := buildHello()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = []string{runnerproto.EnvRunnerDir + "=" + r.runnerDir}
	cmd.Dir = r.dir
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
}

// fetch sends GET path straight to app.sock with token, or none when it is empty.
func (r *run) fetch(t *testing.T, path, token string) (int, string) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://app"+path, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set(viewer.Header, token)
	}
	resp, err := socket.Client(filepath.Join(r.runnerDir, runnerproto.AppSocket)).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func mode(t *testing.T, path string) fs.FileMode {
	st, err := os.Stat(path)
	require.NoError(t, err)
	return st.Mode().Perm()
}

func TestRunsTheApp(t *testing.T) {
	r := folders(t, helloYAML)
	r.start(t, r.args())
	assert.Equal(t, statusResponse{Protocol: 1, App: "hello", State: "waiting"}, r.status(t))
	assert.Equal(t, fs.FileMode(0o666), mode(t, filepath.Join(r.runnerDir, runnerproto.RunnerSocket)))
	assert.Equal(t, fs.FileMode(0o600), mode(t, r.control))
	assert.NoFileExists(t, filepath.Join(r.runnerDir, runnerproto.MySQLSocket), "no mysql.sock without sqldb")

	r.startApp(t)
	require.Eventually(t, func() bool { return r.status(t).State == "ready" }, 30*time.Second, 20*time.Millisecond)
	assert.Equal(t, fs.FileMode(0o666), mode(t, filepath.Join(r.runnerDir, runnerproto.AppSocket)))

	code, body := r.fetch(t, "/?q=l7secret", r.token(t, "l7-a", "valid"))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "l7-greeting l7-a (token 32 bytes)", body, "settings are l7-<name>, secrets 32 characters")
	for _, kind := range []string{"expired", "audience"} {
		code, _ := r.fetch(t, "/", r.token(t, "l7-a", kind))
		assert.Equal(t, http.StatusUnauthorized, code, kind)
	}
	code, _ = r.fetch(t, "/", "")
	assert.Equal(t, http.StatusUnauthorized, code, "no token")

	var spans spansResponse
	require.Eventually(t, func() bool {
		spans = r.spans(t)
		return slices.ContainsFunc(spans.Spans, func(s span) bool { return s.Name == "hello" })
	}, 10*time.Second, 50*time.Millisecond)
	assert.Zero(t, spans.Dropped)
	i := slices.IndexFunc(spans.Spans, func(s span) bool { return s.Name == "HTTP GET" && s.Attributes["http.path"] == "/" })
	require.GreaterOrEqual(t, i, 0, "the request's span")
	assert.Regexp(t, `^[0-9a-f]{32}$`, spans.Spans[i].TraceID)
	assert.Equal(t, []event{}, spans.Spans[i].Events)
	assert.Empty(t, spans.Spans[i].Status)

	assert.Equal(t, 0, r.stop(), "SIGTERM stops it")
	assert.Empty(t, r.stderr.String(), "nothing is logged: no token, no query string")
	assert.NoFileExists(t, r.control)
	assert.NoFileExists(t, filepath.Join(r.runnerDir, runnerproto.RunnerSocket))
}

// hello calls Hello on runner.sock as an app does.
func (r *run) hello(t *testing.T) *runnerv1.HelloResponse {
	c := runnerv1connect.NewLifecycleServiceClient(socket.Client(filepath.Join(r.runnerDir, runnerproto.RunnerSocket)), socket.BaseURL, connect.WithGRPC())
	resp, err := c.Hello(t.Context(), connect.NewRequest(&runnerv1.HelloRequest{ProtocolVersion: runnerproto.ProtocolVersion}))
	require.NoError(t, err)
	return resp.Msg
}

func (r *run) secret(t *testing.T, name string) string {
	c := runnerv1connect.NewSecretsServiceClient(socket.Client(filepath.Join(r.runnerDir, runnerproto.RunnerSocket)), socket.BaseURL, connect.WithGRPC())
	resp, err := c.GetSecret(t.Context(), connect.NewRequest(&runnerv1.GetSecretRequest{Name: name}))
	require.NoError(t, err)
	return string(resp.Msg.GetValue())
}

func TestHelloAndSecrets(t *testing.T) {
	var secrets []string
	var keys [][]byte
	for range 2 {
		r := folders(t, helloYAML)
		r.start(t, r.args())
		h := r.hello(t)
		assert.Equal(t, "hello", h.GetApp())
		assert.Equal(t, "preview", h.GetEnv())
		assert.Equal(t, frameworkVersion(), h.GetVersion())
		assert.True(t, h.GetAppSocketOpen())
		assert.Len(t, h.GetCsrfKey(), 32)
		require.Len(t, h.GetViewerKeys(), 1)
		keys = append(keys, h.GetCsrfKey(), h.GetViewerKeys()[0])
		assert.Equal(t, map[string]string{"greeting": "l7-greeting"}, h.GetSettings())
		secrets = append(secrets, r.secret(t, "token"))
	}
	assert.Regexp(t, `^[0-9a-f]{32}$`, secrets[0])
	assert.NotEqual(t, secrets[0], secrets[1], "secrets are new each run")
	assert.NotEqual(t, keys[0], keys[2], "the form key is new each run")
	assert.NotEqual(t, keys[1], keys[3], "the viewer key is new each run")
}

func TestRefusesWhatAnotherUserOwns(t *testing.T) {
	r := folders(t, sqlYAML)
	args := r.dsn(t, "l7user:S3cretPassw0rd@tcp(127.0.0.1:3306)/", 0o600, "l7_db")
	other := os.Geteuid() + 1
	_, err := load(r.args(args...), other)
	require.ErrorContains(t, err, "--runner-dir must be a folder of this user's own")
	require.ErrorContains(t, checkFolder("--state", r.state, other, 0o700), "--state must be a folder of this user's own")
	require.ErrorContains(t, checkFolder("--control", filepath.Dir(r.control), other, 0o700), "must be a folder of this user's own")
	_, err = readDSN(args[1], other)
	require.ErrorContains(t, err, "--mysql-dsn-file must be a regular file of this user's own")
	_, err = load(r.args(args...), os.Geteuid())
	require.NoError(t, err, "the same folders and file pass for their owner")
}

func TestDroppedSpans(t *testing.T) {
	defer func(n int) { maxSpans = n }(maxSpans)
	maxSpans = 2
	r := folders(t, helloYAML)
	r.start(t, r.args())
	c := runnerv1connect.NewTelemetryServiceClient(socket.Client(filepath.Join(r.runnerDir, runnerproto.RunnerSocket)), socket.BaseURL, connect.WithGRPC())
	_, err := c.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: []*runnerv1.Span{{Name: "a"}, {Name: "b"}, {Name: "c"}}, Dropped: 1}))
	require.NoError(t, err)
	got := r.spans(t)
	assert.Len(t, got.Spans, 2)
	assert.Equal(t, uint64(2), got.Dropped, "one past the bound and one the app dropped")
}

func TestMailIsCaught(t *testing.T) {
	r := folders(t, mailYAML)
	r.start(t, r.args())
	c := runnerv1connect.NewMailServiceClient(socket.Client(filepath.Join(r.runnerDir, runnerproto.RunnerSocket)), socket.BaseURL, connect.WithGRPC())
	send := func(to string) error {
		_, err := c.Send(t.Context(), connect.NewRequest(&runnerv1.SendRequest{IdempotencyKey: to, To: []*runnerv1.Address{{Address: to}}, Subject: "Hi", Text: "Hello"}))
		return err
	}
	require.NoError(t, send("ana@acme.example"))
	err := send("ana@other.example")
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	require.ErrorContains(t, err, "E-MAIL-003")
}

func TestRefuses(t *testing.T) {
	const dsn = "l7user:S3cretPassw0rd@tcp(127.0.0.1:3306)/"
	for name, c := range map[string]struct {
		yaml, want string
		args       func(*testing.T, *run) []string
	}{
		"a runner folder of mode 0700": {helloYAML, "--runner-dir must be a folder of this user's own, of mode 01733", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Chmod(r.runnerDir, 0o700))
			return r.args()
		}},
		"a runner folder without the sticky bit": {helloYAML, "--runner-dir must be", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Chmod(r.runnerDir, 0o733))
			return r.args()
		}},
		"a runner folder open to all": {helloYAML, "--runner-dir must be", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Chmod(r.runnerDir, fs.ModeSticky|0o777))
			return r.args()
		}},
		"a runner folder through a symbolic link": {helloYAML, "--runner-dir must be", func(t *testing.T, r *run) []string {
			link := filepath.Join(r.dir, "link")
			require.NoError(t, os.Symlink(r.runnerDir, link))
			r.runnerDir = link
			return r.args()
		}},
		"a control folder that is not 0700": {helloYAML, "the folder of --control must be a folder of this user's own, of mode 0700", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Chmod(filepath.Dir(r.control), 0o750))
			return r.args()
		}},
		"a state folder that is not 0700": {helloYAML, "--state must be a folder of this user's own, of mode 0700", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Chmod(r.state, 0o755))
			return r.args()
		}},
		"a runner folder too deep for a socket": {helloYAML, "at most 107 bytes long", func(t *testing.T, r *run) []string {
			r.runnerDir = filepath.Join(r.dir, strings.Repeat("d", maxSocketPath))
			mkdir(t, r.runnerDir, fs.ModeSticky|0o733)
			return r.args()
		}},
		"database flags for an app without sqldb": {helloYAML, "the app does not declare sqldb", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn, 0o600, "l7_db")...)
		}},
		"sqldb without database flags": {sqlYAML, "the app declares sqldb", func(_ *testing.T, r *run) []string { return r.args() }},
		"a DSN that names a database": {sqlYAML, "names a database", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn+"l7_db", 0o600, "l7_db")...)
		}},
		"a DSN file with group bits": {sqlYAML, "no permission for group or others", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn, 0o640, "l7_db")...)
		}},
		"a DSN file that is a symbolic link": {sqlYAML, "is a symbolic link", func(t *testing.T, r *run) []string {
			args := r.dsn(t, dsn, 0o600, "l7_db")
			link := filepath.Join(r.dir, "link.dsn")
			require.NoError(t, os.Symlink(args[1], link))
			args[1] = link
			return r.args(args...)
		}},
		"a DSN file over 4 KiB": {sqlYAML, "larger than 4 KiB", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn+"?"+strings.Repeat("x", maxDSNFile), 0o600, "l7_db")...)
		}},
		"a DSN that asks for TLS": {sqlYAML, "asks for TLS", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn+"?tls=true", 0o600, "l7_db")...)
		}},
		"a DSN over a named pipe": {sqlYAML, "over a Unix socket or TCP", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, "l7user:S3cretPassw0rd@pipe(x)/", 0o600, "l7_db")...)
		}},
		"no DSN": {sqlYAML, "does not hold a valid MySQL DSN", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, "l7user S3cretPassw0rd", 0o600, "l7_db")...)
		}},
		"a database name that is not one": {sqlYAML, "--mysql-database must be", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn, 0o600, "l7_db;")...)
		}},
		"the DSN file without the database": {sqlYAML, "go together", func(t *testing.T, r *run) []string {
			return r.args(r.dsn(t, dsn, 0o600, "l7_db")[:2]...)
		}},
		"a missing flag":  {helloYAML, "--state is required", func(_ *testing.T, r *run) []string { return r.args()[:6] }},
		"an unknown flag": {helloYAML, "flag provided but not defined: -nope", func(_ *testing.T, r *run) []string { return r.args("--nope", "x") }},
		"a relative path": {helloYAML, "--app must be an absolute path", func(_ *testing.T, r *run) []string { return append(r.args(), "--app", "app") }},
		"an argument":     {helloYAML, "no arguments", func(_ *testing.T, r *run) []string { return r.args(dsn) }},
		"no permission list": {helloYAML, "aicoded.yaml cannot be opened", func(t *testing.T, r *run) []string {
			require.NoError(t, os.Remove(filepath.Join(r.app, manifest.FileName)))
			return r.args()
		}},
		"a permission list that is not valid": {"app: Hello\n", "E-MAN-004", func(_ *testing.T, r *run) []string { return r.args() }},
	} {
		t.Run(name, func(t *testing.T) {
			r := folders(t, c.yaml)
			var stderr bytes.Buffer
			assert.Equal(t, exitUsage, Main(t.Context(), c.args(t, r), &stderr))
			out := stderr.String()
			assert.Equal(t, 1, strings.Count(out, "\n"), out)
			assert.True(t, strings.HasPrefix(out, "aicoded l7-runner: "), out)
			assert.Contains(t, out, c.want)
			for _, secret := range []string{"S3cretPassw0rd", "l7user"} {
				assert.NotContains(t, out, secret, "the DSN never reaches a message")
			}
			assert.NoFileExists(t, r.control, "nothing listens")
			assert.NoFileExists(t, filepath.Join(r.runnerDir, runnerproto.RunnerSocket), "nothing listens")
		})
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}
