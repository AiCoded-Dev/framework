package dev

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
)

const (
	readyTimeout  = 30 * time.Second
	stopTimeout   = 15 * time.Second
	maxSocketPath = 100
	csrfKeySize   = 32
	runDirPattern = "aicoded-"
)

// Services are what the local runner gives an app besides the runner protocol.
type Services struct {
	// StateDir is the workspace's private folder from StateDir, an absolute path; file stores
	// and built apps live under it.
	StateDir string
	// MySQL creates the apps' databases; nil when dev.yaml names no MySQL server.
	MySQL *MySQLAdmin
	// Router carries the app's calls to the other apps of the workspace; with nil, every call
	// fails with E-RPC-011.
	Router *Router
	// Env is the environment the app runs in, which also names its database: devconfig.EnvDev
	// when empty, or devconfig.EnvPreview.
	Env devconfig.Env
}

// env returns the environment of svc, devconfig.EnvDev when it names none.
func (svc Services) env() devconfig.Env { return cmp.Or(svc.Env, devconfig.EnvDev) }

// Instance is one app running under the local runner.
type Instance struct {
	Name string
	// Manifest is the app's permission list, with the sections aicoded generate wrote.
	Manifest manifest.Manifest

	served
	store  *store
	runDir string
	cmd    *exec.Cmd
	exited chan struct{}
	// byHand marks the runner of an app the developer runs by hand: it has no process, and its
	// run folder outlives it.
	byHand bool
}

// Start builds the app in dir into the bin folder of svc.StateDir and runs it as launch does,
// with a store of its own that prints to out. It refuses an svc.StateDir that is not an
// absolute path.
func Start(ctx context.Context, dir string, m manifest.Manifest, values devconfig.AppValues, svc Services, signer *Signer, out io.Writer) (*Instance, error) {
	if err := checkStart(m, values, svc); err != nil {
		return nil, err
	}
	bin := filepath.Join(svc.StateDir, "bin", m.App)
	if err := build(ctx, dir, bin); err != nil {
		return nil, err
	}
	st := newStore(m.App, &lockedWriter{w: out})
	st.hideSecrets(values.Secrets)
	return launch(ctx, dir, bin, m, values, svc, signer, st)
}

// checkStart refuses to start the app of m when svc.StateDir is not an absolute path, when
// svc.Env is not empty, dev or preview, when values lack a setting or secret m declares, or when
// m declares sqldb and svc has no MySQL.
func checkStart(m manifest.Manifest, values devconfig.AppValues, svc Services) error {
	if !filepath.IsAbs(svc.StateDir) {
		return errors.New("dev: Services.StateDir must be an absolute path")
	}
	if !svc.env().Valid() {
		return errors.New("dev: Services.Env must be empty, dev or preview")
	}
	if missing := missingValues(m, values); len(missing) > 0 {
		return errs.New("E-DEV-003", fmt.Sprintf("no local values for %s of app %s", strings.Join(missing, ", "), m.App),
			"add them under workspaces.<this directory>.apps."+m.App+" in dev.yaml")
	}
	if m.SQLDB() && svc.MySQL == nil {
		return errs.New("E-DEV-006", m.App+" declares sqldb, but dev.yaml names no MySQL server",
			"add mysql: user:password@unix(/var/run/mysqld/mysqld.sock)/ under workspaces.<this directory> in dev.yaml")
	}
	return nil
}

// build compiles the main package in dir into the file bin, in a folder of mode 0700. The Go
// compiler's problems come back as one generate.Diagnostics.
func build(ctx context.Context, dir, bin string) error {
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		return err
	}
	problems, err := gotool.BuildBinary(ctx, dir, bin)
	if err != nil {
		return err
	}
	return generate.Diagnostics(problems).Err()
}

// launch runs bin, the built app in dir, with only AICODED_RUNNER_DIR in its environment, and
// returns once the app reports ready. The app's output lines, spans and mail go to st.
func launch(ctx context.Context, dir, bin string, m manifest.Manifest, values devconfig.AppValues, svc Services, signer *Signer, st *store) (*Instance, error) {
	if err := checkStart(m, values, svc); err != nil {
		return nil, err
	}
	runDir, err := newRunDir()
	if err != nil {
		return nil, err
	}
	i := &Instance{Name: m.App, Manifest: m, store: st, runDir: runDir, exited: make(chan struct{})}
	if err := i.run(ctx, dir, bin, m, values, svc, signer); err != nil {
		i.Stop()
		return nil, err
	}
	return i, nil
}

// launchByHand serves the runner of the app of m in runDir, for a process the developer starts
// by hand as often as they like. The instance has no process, and Stop leaves runDir in place.
func launchByHand(ctx context.Context, runDir string, m manifest.Manifest, values devconfig.AppValues, svc Services, signer *Signer, st *store) (*Instance, error) {
	if err := checkStart(m, values, svc); err != nil {
		return nil, err
	}
	i := &Instance{Name: m.App, Manifest: m, store: st, runDir: runDir, exited: make(chan struct{}), byHand: true}
	if err := i.serve(ctx, m, values, svc, signer); err != nil {
		i.Stop()
		return nil, err
	}
	return i, nil
}

func (i *Instance) run(ctx context.Context, dir, bin string, m manifest.Manifest, values devconfig.AppValues, svc Services, signer *Signer) error {
	if err := i.serve(ctx, m, values, svc, signer); err != nil {
		return err
	}

	// Stop ends the app with SIGTERM; the context only bounds the start-up.
	i.cmd = exec.CommandContext(context.WithoutCancel(ctx), bin)
	i.cmd.Dir = dir
	i.cmd.Env = []string{runnerproto.EnvRunnerDir + "=" + i.runDir}
	i.cmd.SysProcAttr = appSysProcAttr()
	i.cmd.Stdout, i.cmd.Stderr = i.store.writer(sourceApp), i.store.writer(sourceApp)
	if err := i.cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = i.cmd.Wait()
		close(i.exited)
	}()

	select {
	case <-i.svc.ready:
		return nil
	case <-i.exited:
		return errs.New("E-DEV-004", i.Name+" exited before it was ready", "read the app's log lines above; main must call app.Main")
	case <-time.After(readyTimeout):
		return errs.New("E-DEV-004", i.Name+" did not report ready within 30s", "read the app's log lines above; main must call app.Main")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// serve serves the runner protocol on runner.sock in the run folder, and the database on
// mysql.sock when m declares sqldb.
func (i *Instance) serve(ctx context.Context, m manifest.Manifest, values devconfig.AppValues, svc Services, signer *Signer) error {
	csrfKey := make([]byte, csrfKeySize)
	_, _ = rand.Read(csrfKey)
	i.svc = newRunnerService(m, values, svc.env(), csrfKey, [][]byte{signer.Public()}, i.store)
	i.svc.byHand = i.byHand
	i.svc.router = svc.Router
	if err := i.listenRunner(ctx, i.runDir, filepath.Join(svc.StateDir, "files", m.App), m, false); err != nil {
		return err
	}
	if m.SQLDB() {
		return i.serveDatabase(ctx, m.App, svc.MySQL, svc.env())
	}
	return nil
}

// serveDatabase prepares the database of app in env and serves it on mysql.sock through a proxy
// that logs in as the app's own user.
func (i *Instance) serveDatabase(ctx context.Context, app string, admin *MySQLAdmin, env devconfig.Env) error {
	up, err := admin.Prepare(ctx, app, string(env))
	if err != nil {
		return err
	}
	return i.listenDatabase(ctx, i.runDir, up, false, func(err error) {
		i.store.add(sourceRunner, "ERROR", "database: "+err.Error(), true)
	})
}

// Stop sends SIGTERM, waits for the app to exit (killing it after 15s) and removes its sockets.
// For an app run by hand, it stops only the runner, keeps the run folder and closes Exited.
func (i *Instance) Stop() {
	if i.cmd != nil && i.cmd.Process != nil {
		_ = i.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-i.exited:
		case <-time.After(stopTimeout):
			_ = i.cmd.Process.Kill()
			<-i.exited
		}
	}
	i.close()
	if i.byHand {
		close(i.exited)
		return
	}
	_ = os.RemoveAll(i.runDir)
}

// AppSocket returns the path of the app's HTTP socket.
func (i *Instance) AppSocket() string { return filepath.Join(i.runDir, runnerproto.AppSocket) }

// Exited is closed when the app process ends, or, for an app run by hand, when Stop stops its
// runner.
func (i *Instance) Exited() <-chan struct{} { return i.exited }

func missingValues(m manifest.Manifest, v devconfig.AppValues) []string {
	var missing []string
	for _, n := range m.Settings {
		if _, ok := v.Settings[n]; !ok {
			missing = append(missing, "setting "+n)
		}
	}
	for _, n := range m.Secrets {
		if _, ok := v.Secrets[n]; !ok {
			missing = append(missing, "secret "+n)
		}
	}
	return missing
}

// newRunDir creates the private directory for an app's sockets.
func newRunDir() (string, error) {
	env, base := "XDG_RUNTIME_DIR", os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		env, base = "TMPDIR", os.TempDir()
	}
	// os.MkdirTemp appends at most 10 random digits to the pattern.
	if len(filepath.Join(base, runDirPattern+"0123456789", runnerproto.RunnerSocket)) > maxSocketPath {
		return "", errs.New("E-DEV-005", "the socket directory "+base+" is too long for a Unix socket path",
			"set "+env+" to a short directory such as /tmp")
	}
	return os.MkdirTemp(base, runDirPattern)
}
