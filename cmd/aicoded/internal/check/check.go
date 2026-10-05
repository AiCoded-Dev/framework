// Package check runs aicoded check on the apps of a workspace: for each app, the precheck of the
// lint package, generate, the check of the snapshots it holds, go build, go vet, the rules of the
// lint package and go test.
package check

import (
	"context"
	"errors"
	"path/filepath"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/cmd/aicoded/internal/manifest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
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
}

// Report is what aicoded check found.
type Report struct {
	Apps   []App    `json:"apps"`
	Frozen bool     `json:"frozen,omitempty"`
	Race   bool     `json:"race"`
	Notes  []string `json:"notes,omitempty"`
}

// App is what aicoded check found in one app.
type App struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
	// Changed lists the generated files generate wrote, or with Frozen the ones out of date.
	Changed  []string          `json:"changed,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Problems []problem.Problem `json:"problems,omitempty"`
	// Surface is the app's size, once lint has read its code.
	Surface *surface.Surface `json:"surface,omitempty"`
}

var (
	precheck = lint.Precheck
	goBuild  = gotool.Build
	goVet    = gotool.Vet
	runLint  = lint.Run
	goTest   = gotool.Test
)

// Run checks the apps under dir. Its error is one of the whole workspace: no app, a permission
// list that does not load, two apps with one name, an unknown Options.App, no go command, a
// package that lint cannot load once the app builds, or ctx ending. Everything else is a problem
// of an app, such as E-CHK-007 when lint cannot run with this go command. Lint's precheck of
// GOFLAGS and the vendor folder runs for every app before any other go command, since both
// decide what the go command builds and runs; -race is then probed in the first app that passed
// it. A step that fails skips the later steps of its app, and every app is checked.
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
	r := Report{Frozen: opts.Frozen}
	if probe != "" {
		race, why, err := gotool.Race(ctx, probe)
		if err != nil {
			why = err.Error()
		}
		r.Race = race
		if !race {
			r.Notes = append(r.Notes, "tests ran without -race: "+why+"; the delivery pipeline will run them with -race")
		}
	}
	names := workspace.Names(all)
	for _, a := range apps {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		if ps := refused[a.Name]; len(ps) > 0 {
			r.Apps = append(r.Apps, App{Name: a.Name, Dir: a.Dir, Problems: problem.FromErrs(a.Name, ps)})
			continue
		}
		app, notes, err := checkApp(ctx, a, names, opts, r.Race)
		if err != nil {
			return Report{}, err
		}
		r.Apps = append(r.Apps, app)
		r.Notes = append(r.Notes, notes...)
	}
	return r, nil
}

// checkApp checks the app a of the workspace names, which passed lint's precheck, and returns
// the notes of its held snapshots. Its error stops the whole check.
func checkApp(ctx context.Context, a workspace.App, names map[string]string, opts Options, race bool) (App, []string, error) {
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
	notes, err := generate.CheckHeld(a.Name, names)
	if err != nil {
		out.Problems = problem.From(a.Name, err)
		return out, nil, nil
	}
	steps := []func() ([]*errs.Error, error){
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
			}
			return r.Problems, err
		},
		func() ([]*errs.Error, error) { return goTest(ctx, a.Dir, race) },
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
	return out, notes, nil
}
