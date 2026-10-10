// Package l7runner is aicoded l7-runner, a hidden command that the delivery pipeline runs for
// the simulated attacks on one app:
//
//	aicoded l7-runner --app <tree> --runner-dir <dir> --control <socket path> --state <dir>
//	                  [--mysql-dsn-file <file> --mysql-database <name>]
//
// It serves the runner protocol to the app as aicoded dev does, in the environment preview, and
// builds and starts nothing: the pipeline starts the app, as another user, once the runner
// answers on its control socket. The control API, for the pipeline alone, reports the app's
// state, mints viewer tokens and returns the app's spans. Nothing listens on TCP, and nothing is
// logged but one line on stderr when the command fails or cannot open the app's database.
package l7runner

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/mysqlproxy"
	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/socket"
)

// Exit codes: stopped by a signal, failed while serving, and a flag, folder or file that is not
// valid.
const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

const (
	// maxSocketPath is the longest path of a Unix socket on Linux.
	maxSocketPath = 107
	maxDSNFile    = 4 << 10
	maxManifest   = 1 << 20
	secretBytes   = 16
	loginTimeout  = 10 * time.Second
)

// The bounds of the spans the runner keeps; it counts the spans past them as dropped.
var (
	maxSpans     = 100_000
	maxSpanBytes = 64 << 20
)

var databaseName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// config is a command line that Main checked.
type config struct {
	manifest  manifest.Manifest
	runnerDir string
	control   string
	state     string
	// database is where the proxy on mysql.sock logs in; nil for an app without sqldb.
	database *mysqlproxy.Upstream
}

// Main runs aicoded l7-runner with args, the arguments after the command, until ctx is done,
// and returns its exit code. It writes nothing but one line to stderr when it fails, and never
// a value the DSN file holds.
func Main(ctx context.Context, args []string, stderr io.Writer) int {
	cfg, err := load(args, os.Geteuid())
	if err != nil {
		fmt.Fprintln(stderr, "aicoded l7-runner: "+oneLine(err))
		return exitUsage
	}
	if err := serve(ctx, cfg, stderr); err != nil {
		fmt.Fprintln(stderr, "aicoded l7-runner: "+oneLine(err))
		return exitFail
	}
	return exitOK
}

// load parses args and checks the folders and files they name for the user uid.
func load(args []string, uid int) (*config, error) {
	f, err := parseFlags(args)
	if err != nil {
		return nil, err
	}
	if err := checkFolder("--runner-dir", f.runnerDir, uid, fs.ModeSticky|0o733); err != nil {
		return nil, err
	}
	if err := checkFolder("the folder of --control", filepath.Dir(f.control), uid, 0o700); err != nil {
		return nil, err
	}
	if err := checkFolder("--state", f.state, uid, 0o700); err != nil {
		return nil, err
	}
	if len(filepath.Join(f.runnerDir, runnerproto.RunnerSocket)) > maxSocketPath || len(f.control) > maxSocketPath {
		return nil, fmt.Errorf("the paths of --runner-dir and --control must leave a socket's path at most %d bytes long", maxSocketPath)
	}
	m, err := readManifest(f.app)
	if err != nil {
		return nil, err
	}
	cfg := &config{manifest: m, runnerDir: f.runnerDir, control: f.control, state: f.state}
	switch {
	case m.SQLDB() && f.dsnFile == "":
		return nil, errors.New("the app declares sqldb: give --mysql-dsn-file and --mysql-database")
	case !m.SQLDB() && f.dsnFile != "":
		return nil, errors.New("the app does not declare sqldb: leave out --mysql-dsn-file and --mysql-database")
	case f.dsnFile != "":
		dsn, err := readDSN(f.dsnFile, uid)
		if err != nil {
			return nil, err
		}
		cfg.database = &mysqlproxy.Upstream{Network: dsn.Net, Address: dsn.Addr, User: dsn.User, Password: dsn.Passwd, Database: f.database}
	}
	return cfg, nil
}

// flags are the values of the command line.
type flags struct {
	app, runnerDir, control, state, dsnFile, database string
}

// parseFlags parses args. Its errors name a flag, never a value.
func parseFlags(args []string) (flags, error) {
	var f flags
	fs := flag.NewFlagSet("aicoded l7-runner", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.app, "app", "", "")
	fs.StringVar(&f.runnerDir, "runner-dir", "", "")
	fs.StringVar(&f.control, "control", "", "")
	fs.StringVar(&f.state, "state", "", "")
	fs.StringVar(&f.dsnFile, "mysql-dsn-file", "", "")
	fs.StringVar(&f.database, "mysql-database", "", "")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, errors.New("it takes no arguments besides its flags")
	}
	paths := []struct {
		name     string
		value    *string
		required bool
	}{
		{"--app", &f.app, true}, {"--runner-dir", &f.runnerDir, true}, {"--control", &f.control, true},
		{"--state", &f.state, true}, {"--mysql-dsn-file", &f.dsnFile, false},
	}
	for _, p := range paths {
		switch {
		case *p.value == "" && p.required:
			return f, errors.New(p.name + " is required")
		case *p.value != "" && !filepath.IsAbs(*p.value):
			return f, errors.New(p.name + " must be an absolute path")
		case *p.value != "":
			*p.value = filepath.Clean(*p.value)
		}
	}
	if (f.dsnFile == "") != (f.database == "") {
		return f, errors.New("--mysql-dsn-file and --mysql-database go together")
	}
	if f.database != "" && !databaseName.MatchString(f.database) {
		return f, errors.New("--mysql-database must be 1 to 64 letters, digits, '_' and '-'")
	}
	return f, nil
}

// checkFolder checks that dir, which the flag name names, is a folder of the user uid's, not a
// symbolic link, of exactly the mode perm.
func checkFolder(name, dir string, uid int, perm fs.FileMode) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("%s cannot be read: %w", name, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || fi.Mode() != fs.ModeDir|perm || int64(st.Uid) != int64(uid) {
		return fmt.Errorf("%s must be a folder of this user's own, of mode %s", name, octal(perm))
	}
	return nil
}

// octal writes perm as chmod takes it, such as 0700 or 01733.
func octal(perm fs.FileMode) string {
	bits := uint32(perm.Perm())
	if perm&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return fmt.Sprintf("%#04o", bits)
}

// readManifest reads the permission list in the app's tree, never through a symbolic link that
// leaves it.
func readManifest(app string) (manifest.Manifest, error) {
	root, err := os.OpenRoot(app)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("--app cannot be opened: %w", err)
	}
	defer root.Close()
	path := filepath.Join(app, manifest.FileName)
	f, err := root.OpenFile(manifest.FileName, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("%s cannot be opened", path)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return manifest.Manifest{}, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxManifest+1))
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("%s cannot be read", path)
	}
	if len(data) > maxManifest {
		return manifest.Manifest{}, fmt.Errorf("%s is larger than 1 MiB", path)
	}
	return manifest.Parse(path, data)
}

// readDSN reads the MySQL DSN in the file name as the platform reads a token: a regular file of
// the user uid's, not a symbolic link, with no permission for group or others, of at most 4 KiB.
// Its errors never quote the file's content.
func readDSN(name string, uid int) (*mysql.Config, error) {
	f, err := os.OpenFile(filepath.Clean(name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.New("--mysql-dsn-file cannot be opened, or is a symbolic link")
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, errors.New("--mysql-dsn-file cannot be read")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.Mode().IsRegular() || !ok || int64(st.Uid) != int64(uid) || fi.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("--mysql-dsn-file must be a regular file of this user's own, with no permission for group or others")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDSNFile+1))
	if err != nil {
		return nil, errors.New("--mysql-dsn-file cannot be read")
	}
	if len(data) > maxDSNFile {
		return nil, errors.New("--mysql-dsn-file is larger than 4 KiB")
	}
	cfg, err := mysql.ParseDSN(strings.TrimSpace(string(data)))
	switch {
	case err != nil:
		return nil, errors.New("--mysql-dsn-file does not hold a valid MySQL DSN")
	case cfg.DBName != "":
		return nil, errors.New("the DSN in --mysql-dsn-file names a database: leave it out, the runner uses --mysql-database")
	case cfg.User == "":
		return nil, errors.New("the DSN in --mysql-dsn-file names no user")
	case cfg.Net != "unix" && cfg.Net != "tcp" && cfg.Net != "tcp4" && cfg.Net != "tcp6":
		return nil, errors.New("the DSN in --mysql-dsn-file must reach the server over a Unix socket or TCP")
	case cfg.TLSConfig != "" && cfg.TLSConfig != "false":
		return nil, errors.New("the DSN in --mysql-dsn-file asks for TLS, which the runner's database proxy does not speak")
	}
	return cfg, nil
}

// serve serves the app of cfg until ctx is done.
func serve(ctx context.Context, cfg *config, stderr io.Writer) error {
	if cfg.database != nil {
		if err := checkLogin(ctx, *cfg.database); err != nil {
			return err
		}
	}
	signer, err := dev.NewSigner()
	if err != nil {
		return err
	}
	var once sync.Once
	r, err := dev.NewRunner(ctx, cfg.manifest, dev.RunnerOptions{
		Dir:          cfg.runnerDir,
		FilesDir:     filepath.Join(cfg.state, "files"),
		Values:       values(cfg.manifest),
		Env:          devconfig.EnvPreview,
		Version:      frameworkVersion(),
		ViewerKey:    signer.Public(),
		Share:        true,
		MaxSpans:     maxSpans,
		MaxSpanBytes: maxSpanBytes,
		Database:     cfg.database,
		OnDatabaseError: func(error) {
			once.Do(func() { fmt.Fprintln(stderr, "aicoded l7-runner: the runner could not open the app's database") })
		},
	})
	if err != nil {
		return err
	}
	defer r.Close()
	l, err := socket.Listen(ctx, cfg.control)
	if err != nil {
		return err
	}
	srv := newControlServer(newControl(cfg.manifest, signer, r))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(l) }()
	select {
	case <-ctx.Done():
		_ = srv.Close()
		return nil
	case err := <-served:
		return err
	}
}

// checkLogin logs in to up's database as the proxy will, so that a database the runner cannot
// open fails the run before the app starts. Its error names the server's error number only.
func checkLogin(ctx context.Context, up mysqlproxy.Upstream) error {
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = up.Network, up.Address, up.User, up.Password, up.Database
	cfg.Timeout = loginTimeout
	cfg.Logger = log.New(io.Discard, "", 0)
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return errors.New("the DSN in --mysql-dsn-file cannot be used")
	}
	db := sql.OpenDB(conn)
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return errors.New("cannot open --mysql-database with the account of --mysql-dsn-file: " + reason(err))
	}
	return nil
}

// reason describes a failed login without the server's text, which names the account.
func reason(err error) string {
	var me *mysql.MySQLError
	var ne *net.OpError
	switch {
	case errors.As(err, &me):
		return fmt.Sprintf("MySQL error %d", me.Number)
	case errors.As(err, &ne):
		return "the server cannot be reached"
	}
	return "the server did not answer"
}

// values are the app's settings, each l7-<name>, and its secrets, each 32 random hex digits.
func values(m manifest.Manifest) devconfig.AppValues {
	v := devconfig.AppValues{Settings: map[string]string{}, Secrets: map[string]string{}}
	for _, n := range m.Settings {
		v.Settings[n] = "l7-" + n
	}
	for _, n := range m.Secrets {
		b := make([]byte, secretBytes)
		_, _ = rand.Read(b)
		v.Secrets[n] = hex.EncodeToString(b)
	}
	return v
}

// frameworkVersion returns the version of the framework this aicoded was built with.
func frameworkVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == lint.Framework {
				return d.Version
			}
		}
	}
	return "(devel)"
}

// oneLine joins the lines of err's text.
func oneLine(err error) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(err.Error(), "\n", "; ")), " ")
}
