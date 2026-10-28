// Package check runs aicoded check on the apps of a workspace: for each app, the precheck of the
// lint package, generate, the check of the snapshots it holds, the check of its schema-change
// draft, go build, go vet, the rules of the lint package and go test, with a test database for an
// app that declares sqldb.
package check

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/draft"
	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/cmd/aicoded/internal/migrations"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
)

// Options says what aicoded check checks.
type Options struct {
	// Frozen writes nothing; every generated file that would change is an E-CHK-001 problem.
	Frozen bool
	// App limits the check to one app; "" checks every app. An unknown name is E-DEV-014.
	App string
	// FrameworkDir is the framework checkout this aicoded was installed from, or "" for a
	// released aicoded. Lint lets an app's go.mod replace the framework with this folder only
	// (E-LINT-011).
	FrameworkDir string
	// NoTests skips the tests and the probe of -race.
	NoTests bool
	// MySQL returns the admin DSN of the MySQL server in dev.yaml, or ""; nil is "". aicoded check
	// asks for it once, and only when the tests of an app that declares sqldb run. With a server,
	// each such app gets a new test database, which aicoded check makes as aicoded dev makes a dev
	// database and drops after the tests; the tests find the data user's DSN in AICODED_TEST_DB.
	// An error of MySQL, or a DSN that is not valid (E-DEV-007), stops the whole check.
	MySQL func() (string, error)
	// Platform gives the migrations each app published, applied to its test database before its
	// draft; with nil, they come from the app's cache.
	Platform migrations.Platform
}

// EnvTestDB is the variable that holds, for an app's tests, the DSN of the data user of its test
// database, or "" when it has none.
const EnvTestDB = "AICODED_TEST_DB"

// WithDatabase returns opts with the MySQL server that dev.yaml names for the workspace of dir, or
// of a folder above it, read only when an app needs it, and the platform that AICODED_PLATFORM
// names, which give each app that declares sqldb its test database. With NoTests, it returns opts
// as they are.
func WithDatabase(dir string, opts Options) (Options, error) {
	if opts.NoTests {
		return opts, nil
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return opts, err
	}
	opts.MySQL = func() (string, error) {
		path, err := devconfig.DefaultPath()
		if err != nil {
			return "", err
		}
		return devconfig.MySQL(path, root)
	}
	opts.Platform = dev.Platform()
	return opts, nil
}

// Report is what aicoded check found.
type Report struct {
	Apps   []App `json:"apps"`
	Frozen bool  `json:"frozen,omitempty"`
	// NoTests is set when the tests did not run.
	NoTests bool     `json:"no_tests,omitempty"`
	Race    bool     `json:"race"`
	Notes   []string `json:"notes,omitempty"`
}

// App is what aicoded check found in one app.
type App struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
	// Changed lists the generated files generate wrote, or with Frozen the ones out of date.
	Changed  []string          `json:"changed,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Problems []problem.Problem `json:"problems,omitempty"`
	// Notes are what lint reports without failing the app, such as E-DATA-001, once lint has
	// read its code.
	Notes []problem.Problem `json:"notes,omitempty"`
	// Surface is the app's size, once lint has read its code.
	Surface *surface.Surface `json:"surface,omitempty"`
}

var (
	precheck  = lint.Precheck
	probeRace = gotool.Race
	goBuild   = gotool.Build
	goVet     = gotool.Vet
	runLint   = lint.Run
	goTest    = gotool.Test
)

// Run checks the apps under dir. Its error is one of the whole workspace: no app, a permission
// list that does not load, two apps with one name, an unknown Options.App, no go command, a
// package that lint cannot load once the app builds, or ctx ending. Everything else is a problem
// of an app, such as E-CHK-007 when lint cannot run with this go command. Lint's precheck of
// GOFLAGS and the vendor folder runs for every app before any other go command, since both
// decide what the go command builds and runs; -race is then probed in the first app that passed
// it, unless Options.NoTests skips the tests. A step that fails skips the later steps of its app,
// and every app is checked.
func Run(ctx context.Context, dir string, opts Options) (Report, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Report{}, err
	}
	all, err := workspace.Apps(root)
	if err != nil {
		return Report{}, err
	}
	apps, err := workspace.Select(all, opts.App)
	if err != nil {
		return Report{}, err
	}
	if err := gotool.Find(); err != nil {
		return Report{}, err
	}
	refused := map[string][]*errs.Error{}
	probe := ""
	for _, a := range apps {
		ps, err := precheck(ctx, a.Dir)
		if err != nil {
			return Report{}, err
		}
		if len(ps) > 0 {
			refused[a.Name] = ps
		} else if probe == "" {
			probe = a.Dir
		}
	}
	r := Report{Frozen: opts.Frozen, NoTests: opts.NoTests}
	if probe != "" && !opts.NoTests {
		race, why, err := probeRace(ctx, probe)
		if err != nil {
			why = err.Error()
		}
		r.Race = race
		if !race {
			r.Notes = append(r.Notes, "tests ran without -race: "+why+"; the delivery pipeline will run them with -race")
		}
	}
	names := workspace.Names(all)
	admin := sync.OnceValues(func() (*dev.MySQLAdmin, error) {
		if opts.MySQL == nil {
			return nil, nil
		}
		dsn, err := opts.MySQL()
		if err != nil || dsn == "" {
			return nil, err
		}
		return dev.NewMySQLAdmin(dsn)
	})
	for _, a := range apps {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		if ps := refused[a.Name]; len(ps) > 0 {
			r.Apps = append(r.Apps, App{Name: a.Name, Dir: a.Dir, Problems: problem.FromErrs(a.Name, ps)})
			continue
		}
		app, held, err := checkApp(ctx, a, names, opts, r.Race, admin)
		if err != nil {
			return Report{}, err
		}
		r.Apps = append(r.Apps, app)
		r.Notes = append(r.Notes, held...)
	}
	return r, nil
}

// checkApp checks the app a of the workspace names, which passed lint's precheck, and returns
// the notes of its held snapshots. admin gives the MySQL server of the test databases, or nil.
// Its error stops the whole check.
func checkApp(ctx context.Context, a workspace.App, names map[string]string, opts Options, race bool,
	admin func() (*dev.MySQLAdmin, error)) (App, []string, error) {
	out := App{Name: a.Name, Dir: a.Dir}
	res, err := generate.Run(a.Dir, generate.Options{Workspace: names, Frozen: opts.Frozen})
	out.Changed, out.Warnings = res.Changed, res.Warnings
	if err != nil {
		out.Problems = problem.From(a.Name, err)
		return out, nil, nil
	}
	m, err := manifest.Load(filepath.Join(a.Dir, manifest.FileName))
	if err != nil {
		out.Problems = problem.From(a.Name, err)
		return out, nil, nil
	}
	held, err := generate.CheckHeld(a.Name, names)
	if err != nil {
		out.Problems = problem.From(a.Name, err)
		return out, nil, nil
	}
	steps := []func() ([]*errs.Error, error){
		func() ([]*errs.Error, error) {
			_, notes, err := draft.Check(a.Dir, m)
			out.Notes = append(out.Notes, problem.FromErrs(a.Name, notes)...)
			return codedErrs(err)
		},
		func() ([]*errs.Error, error) { return goBuild(ctx, a.Dir) },
		func() ([]*errs.Error, error) { return goVet(ctx, a.Dir) },
		func() ([]*errs.Error, error) {
			r, err := runLint(ctx, a.Dir, lint.Config{Generated: res.Files, Modules: m.Modules, FrameworkDir: opts.FrameworkDir})
			var coded *errs.Error
			if errors.As(err, &coded) {
				return []*errs.Error{coded}, nil
			}
			if err == nil {
				s := surface.Of(m, r.Writes)
				out.Surface = &s
				out.Notes = append(out.Notes, problem.FromErrs(a.Name, r.Notes)...)
			}
			return r.Problems, err
		},
	}
	if !opts.NoTests {
		steps = append(steps, func() ([]*errs.Error, error) {
			var server *dev.MySQLAdmin
			if m.SQLDB() {
				got, err := admin()
				if err != nil {
					return nil, err
				}
				server = got
			}
			env, drop, err := testDatabase(ctx, a.Dir, m, server, opts.Platform, &out)
			if coded := (*errs.Error)(nil); errors.As(err, &coded) {
				return []*errs.Error{coded}, nil
			}
			if err != nil {
				return nil, err
			}
			defer drop()
			return goTest(ctx, a.Dir, race, []string{EnvTestDB + "=" + env})
		})
	}
	for _, step := range steps {
		ps, err := step()
		if err != nil {
			return App{}, nil, err
		}
		if len(ps) > 0 {
			out.Problems = problem.FromErrs(a.Name, ps)
			break
		}
	}
	return out, held, nil
}

// testDatabase makes, on the server of admin, the test database of the app in dir whose
// permission list is m, when it declares sqldb and admin is not nil, with the steps aicoded dev
// would apply with the migrations of p, and returns the DSN of its data user and a function that
// drops it. A warning about the published steps goes to out's warnings. With no test database,
// the DSN is "".
func testDatabase(ctx context.Context, dir string, m manifest.Manifest, admin *dev.MySQLAdmin, p migrations.Platform, out *App) (string, func(), error) {
	if !m.SQLDB() || admin == nil {
		return "", func() {}, nil
	}
	steps, warning, err := migrations.Steps(ctx, dir, m, p)
	if err != nil {
		return "", nil, err
	}
	if warning != "" {
		out.Warnings = append(out.Warnings, warning)
	}
	env := "test-" + strings.ToLower(rand.Text()[:8])
	drop := func() { _ = admin.Drop(context.WithoutCancel(ctx), m.App, env) }
	users, err := admin.Migrate(ctx, m.App, env, steps)
	if err != nil {
		drop()
		return "", nil, err
	}
	cfg := mysql.NewConfig()
	cfg.Net, cfg.Addr, cfg.User, cfg.Passwd, cfg.DBName = users.Data.Network, users.Data.Address, users.Data.User, users.Data.Password, users.Data.Database
	return cfg.FormatDSN(), drop, nil
}

// codedErrs returns the coded errors that err is or joins, or err itself when one of them has no
// code.
func codedErrs(err error) ([]*errs.Error, error) {
	if err == nil {
		return nil, nil
	}
	all := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		all = joined.Unwrap()
	}
	var out []*errs.Error
	for _, e := range all {
		var coded *errs.Error
		if !errors.As(e, &coded) {
			return nil, err
		}
		out = append(out, coded)
	}
	return out, nil
}
