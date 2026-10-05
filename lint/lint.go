// Package lint holds the rules that aicoded check applies to an app's code after go vet, and that
// the delivery pipeline will apply too, such as which packages and names app code may use and
// which files an app may hold. Run loads the app's packages without their tests, type-checks
// them, and reports every finding as a coded problem with its position and fix. Nothing in the
// app can turn a finding off: comments such as //nolint have no effect.
//
// Rules returns every rule with its code and the sentences the docs show for it.
package lint

import (
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"aicoded.dev/framework/internal/errs"
)

// Config is what Run needs to know about the app besides its code.
type Config struct {
	// Generated lists the files aicoded generate owns in the app, relative to its folder and
	// slash-separated. Every other file is checked as hand-written, whatever its name or header.
	// E-LINT-001, E-LINT-006, E-LINT-007 and E-LINT-009 read generated files too, and a generated
	// file may also import the packages only generated files may import; E-LINT-004 and
	// E-LINT-005 read hand-written files only. A problem in a generated file is reported where
	// its //line comment points, such as the line of the template that the code was written from,
	// when that comment names a file in the app folder, and at the generated file otherwise.
	Generated []string
	// Modules lists the module paths of the modules: section of the app's permission list.
	Modules []string
	// FrameworkDir is the framework checkout that the aicoded running lint was installed from,
	// or "" for a released aicoded. The app's go.mod may replace the framework with this folder,
	// and with nothing else: every other replace directive is E-LINT-011.
	FrameworkDir string
}

// Result is what Run found.
type Result struct {
	// Problems are the findings, sorted by position. Each has a "file:line" position, relative
	// to the app folder and slash-separated; a file of a declared module is written
	// module@version/path, and a file outside the app folder by its absolute path. A GOFLAGS
	// finding has the position GOFLAGS, and a vendor folder with no vendor/modules.txt the
	// position vendor.
	Problems []*errs.Error
	// Writes are the tables that the app's constant SQL writes, with how, each once, sorted.
	Writes []Write
	// Stopped is true when what Precheck reports stopped Run before it loaded the app: Problems
	// then holds only those findings, and Writes is empty.
	Stopped bool
}

// Write is a table that SQL writes, and how.
type Write struct {
	Table string // as the SQL names it, without backquotes
	Op    string // INSERT, UPDATE, DELETE or REPLACE
}

// Run checks the app in dir. It loads every package of the app with cgo off, without tests, and
// applies every rule. Its error is E-CHK-007 when the go command is a newer Go than the one this
// program was built with; any other error says that a package could not be loaded or
// type-checked, or that ctx ended. What the rules find is in the Result.
func Run(ctx context.Context, dir string, cfg Config) (Result, error) {
	a, err := newApp(dir, cfg)
	if err != nil {
		return Result{}, err
	}
	dir = a.dir
	env := loadEnv()
	// GOFLAGS and a vendor folder decide which files the go command reads, so they are checked
	// before any other go command runs: a refused one makes the load untrustworthy, and the
	// findings stand alone.
	if a.preLoad(ctx, env) {
		return Result{Problems: a.problems(), Stopped: true}, nil
	}
	if err := checkGoVersion(ctx, dir, env); err != nil {
		return Result{}, err
	}
	work, err := goWork(ctx, dir, env)
	if err != nil {
		return Result{}, err
	}
	pkgs, err := packages.Load(&packages.Config{
		Mode:    packages.LoadAllSyntax | packages.NeedModule,
		Context: ctx,
		Dir:     dir,
		Env:     env,
	}, "./...")
	if err != nil {
		return Result{}, err
	}
	if err := loadError(pkgs); err != nil {
		return Result{}, err
	}
	for _, p := range pkgs {
		a.own[p.PkgPath] = true
	}
	packages.Visit(pkgs, nil, func(p *packages.Package) { a.module[p.PkgPath] = p.Module })
	a.methods = methodsOf(pkgs, slices.Sorted(maps.Keys(refused)))
	a.sqlMethods = methodsOf(pkgs, sqlMethods)
	a.reflection = methodsOf(pkgs, reflection)
	g, err := checker.Analyze(a.analyzers(), pkgs, nil)
	if err != nil {
		return Result{}, err
	}
	for act := range g.All() {
		if !act.IsRoot {
			continue
		}
		if act.Err != nil {
			return Result{}, fmt.Errorf("lint %s: %s: %w", act.Package.PkgPath, act.Analyzer.Name, act.Err)
		}
		for _, d := range act.Diagnostics {
			file, line, column := a.position(act.Package.Fset, d.Pos)
			a.report(file, line, column, d.Category, d.Message)
		}
	}
	if err := a.taint(ctx, pkgs); err != nil {
		return Result{}, err
	}
	if err := a.files(pkgs); err != nil {
		return Result{}, err
	}
	a.undeclared(pkgs)
	if err := a.replaces(cfg.FrameworkDir, work); err != nil {
		return Result{}, err
	}
	a.verifyModules(ctx, env)
	a.declaredModules(pkgs)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return Result{Problems: a.problems(), Writes: a.sortedWrites()}, nil
}

// Precheck reports, with E-LINT-011, what Run checks in the app in dir before it runs any other
// go command, since it decides which files and tools the go command uses: a GOFLAGS entry other
// than the few lint allows, in the environment or in what go env prints, and a vendor folder at
// the app root. aicoded check runs it before go build and go vet. Its error says that dir cannot
// be resolved.
func Precheck(ctx context.Context, dir string) ([]*errs.Error, error) {
	a, err := newApp(dir, Config{})
	if err != nil {
		return nil, err
	}
	a.preLoad(ctx, loadEnv())
	return a.problems(), nil
}

// newApp returns a run over the app in dir with cfg, its folder made absolute with its symbolic
// links resolved.
func newApp(dir string, cfg Config) (*app, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return nil, err
	}
	return &app{dir: dir, generated: setOf(cfg.Generated), own: map[string]bool{}, declared: setOf(cfg.Modules),
		module: map[string]*packages.Module{}, writes: map[Write]bool{}}, nil
}

// loadEnv returns the environment lint runs the go command with: this process's, with cgo off.
func loadEnv() []string {
	return append(os.Environ(), "CGO_ENABLED=0")
}

// loadError returns the first error of a package that pkgs need, or nil. A path in the folder of
// a module other than the app's is written module@version/path, the same on every machine.
func loadError(pkgs []*packages.Package) error {
	var err error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if err != nil || len(p.Errors) == 0 {
			return
		}
		msg := p.Errors[0].Error()
		if m := p.Module; m != nil && !m.Main && m.Dir != "" {
			msg = strings.ReplaceAll(msg, m.Dir+string(filepath.Separator), m.Path+"@"+m.Version+"/")
		}
		err = fmt.Errorf("lint cannot load package %s: %s", p.PkgPath, msg)
	})
	return err
}

// app is one run of Run over an app.
type app struct {
	// dir is the app folder: absolute, with its symbolic links resolved.
	dir string
	// generated holds the files of Config.Generated.
	generated map[string]bool
	// own holds the import paths of the app's packages, the ones Run checks.
	own map[string]bool
	// declared holds the modules of Config.Modules.
	declared map[string]bool
	// module holds the module of every loaded package by import path, nil for the standard
	// library.
	module map[string]*packages.Module
	// methods holds the refused methods of the loaded packages, by name.
	methods map[string][]*types.Func
	// sqlMethods holds the methods of sqlMethods in the loaded packages, by name.
	sqlMethods map[string][]*types.Func
	// reflection holds the methods of reflection in the loaded packages, by name.
	reflection map[string][]*types.Func
	// mu guards writes, which the analyzers fill in parallel.
	mu sync.Mutex
	// writes holds the writes of the app's constant SQL.
	writes map[Write]bool
	// found holds what the rules found.
	found []finding
}

// finding is one problem a rule found.
type finding struct {
	file          string
	line, column  int
	code, message string
}

// analyzers returns the analyzers of the rules that read the app's type-checked Go files. Each
// reports a finding as a diagnostic whose Category is the rule's code.
func (a *app) analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		a.analyzer("imports", a.imports),
		a.analyzer("entries", a.entryPoints),
		a.analyzer("names", a.names),
		a.analyzer("sql", a.constantSQL),
	}
}

// analyzer returns an analyzer that runs check on each package of the app.
func (a *app) analyzer(name string, check func(*analysis.Pass)) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: name,
		Doc:  "aicoded lint rule " + name,
		Run: func(p *analysis.Pass) (any, error) {
			check(p)
			return nil, nil
		},
	}
}

// diagnose reports a finding of the rule code at pos.
func diagnose(p *analysis.Pass, pos token.Pos, code, message string) {
	p.Report(analysis.Diagnostic{Pos: pos, Category: code, Message: message})
}

// isGenerated reports whether f is a file of Config.Generated. It goes by the file read, not by
// the position a //line comment gives.
func (a *app) isGenerated(p *analysis.Pass, f *ast.File) bool {
	return a.generated[a.rel(p.Fset.File(f.FileStart).Name())]
}

// position returns where a finding at pos is reported: its file, relative to the app folder, its
// line and its column. In a generated file it is the position that the //line comment before pos
// gives, such as the line of the template that the code was written from, when that file lies in
// the app folder. In every other file, //line comments change nothing.
func (a *app) position(fset *token.FileSet, pos token.Pos) (string, int, int) {
	p := fset.PositionFor(pos, false)
	file := a.rel(p.Filename)
	if !a.generated[file] {
		return file, p.Line, p.Column
	}
	q := fset.PositionFor(pos, true)
	if r, err := filepath.Rel(a.dir, q.Filename); err == nil && filepath.IsLocal(r) {
		return filepath.ToSlash(r), q.Line, q.Column
	}
	return file, p.Line, p.Column
}

// report records a finding at line and column of file, a path relative to the app folder.
func (a *app) report(file string, line, column int, code, message string) {
	a.found = append(a.found, finding{file: file, line: line, column: column, code: code, message: message})
}

// rel returns name, an absolute path, relative to the app folder and slash-separated, or name
// itself when it lies outside the folder.
func (a *app) rel(name string) string {
	if r, err := filepath.Rel(a.dir, name); err == nil && filepath.IsLocal(r) {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(name)
}

// problems returns the findings as coded errors, sorted by position.
func (a *app) problems() []*errs.Error {
	slices.SortFunc(a.found, func(x, y finding) int {
		return cmp.Or(cmp.Compare(x.file, y.file), cmp.Compare(x.line, y.line), cmp.Compare(x.column, y.column),
			cmp.Compare(x.code, y.code), cmp.Compare(x.message, y.message))
	})
	var out []*errs.Error
	for _, f := range a.found {
		pos := f.file
		if f.line > 0 {
			pos += ":" + strconv.Itoa(f.line)
		}
		out = append(out, errs.At(pos, f.code, f.message, fix(f.code)))
	}
	return out
}
