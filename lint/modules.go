package lint

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// capabilities are the packages that a package of a declared module may not import (E-LINT-003),
// besides the framework's and every net/... package that netAllowed does not list. Each runs
// programs, connects or listens, makes system calls, loads code or reaches memory unchecked:
// crypto/tls and log/syslog connect through net, and go/build and go/importer run the go
// command and the cgo tool.
var capabilities = []string{
	"C", "crypto/tls", "go/build", "go/importer", "log/syslog", "net", "os/exec", "plugin", "syscall", "unsafe",
}

// netAllowed are the only net/... packages that a package of a declared module may import.
var netAllowed = []string{"net/http", "net/mail", "net/netip", "net/url"}

// processes are the names of package os that start or reach other processes, which a package of
// a declared module may not use (E-LINT-003). A method of os.Process counts as os.Process.
var processes = []string{"os.FindProcess", "os.Process", "os.StartProcess"}

// reflection are the names of package reflect that call a function, reach a method by its name or
// number, or hand out a pointer that unsafe can turn into any typed value; a package of a declared
// module may not use them (E-LINT-003), also through an interface with the same name and signature.
var reflection = []string{
	"reflect.NewAt",
	"reflect.Type.Method", "reflect.Type.MethodByName",
	"reflect.Value.Call", "reflect.Value.CallSlice", "reflect.Value.Method", "reflect.Value.MethodByName",
	"reflect.Value.Pointer", "reflect.Value.UnsafeAddr", "reflect.Value.UnsafePointer",
}

// driverIfaces are the interfaces of package database/sql/driver whose methods open connections,
// prepare or run statements, or begin transactions; a package of a declared module may not call
// them (E-LINT-003), so it cannot reach the driver that database/sql hides. The package stays
// importable, because a module may implement driver.Valuer.
var driverIfaces = []string{
	"Conn", "ConnBeginTx", "ConnPrepareContext", "Connector", "Driver", "DriverContext",
	"Execer", "ExecerContext", "Pinger", "Queryer", "QueryerContext", "SessionResetter",
	"Stmt", "StmtExecContext", "StmtQueryContext", "Tx", "Validator",
}

// driverMethod reports whether name, as member names it, is a method of a driverIfaces interface.
func driverMethod(name string) bool {
	rest, ok := strings.CutPrefix(name, "database/sql/driver.")
	if !ok {
		return false
	}
	typ, _, ok := strings.Cut(rest, ".")
	return ok && slices.Contains(driverIfaces, typ)
}

// thirdParty reports whether the package at path belongs to a module other than the app's own,
// the framework and the standard library.
func (a *app) thirdParty(path string) bool {
	m := a.module[path]
	return m != nil && !m.Main && m.Path != Framework
}

// isDeclared reports whether p belongs to a module that the app declares in modules:.
func (a *app) isDeclared(p *packages.Package) bool {
	return p.Module != nil && !p.Module.Main && a.declared[p.Module.Path]
}

// undeclared reports, with E-LINT-002 at its require line in go.mod, every module that is not
// declared although a package of the app, or of a declared module, imports a package of it. The
// framework and the modules only the framework imports need no declaration, nor does a package
// that only generated files may import, imported by the app.
func (a *app) undeclared(pkgs []*packages.Package) {
	missing := map[string]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		own := a.own[p.PkgPath]
		if !own && !a.isDeclared(p) {
			return
		}
		for _, q := range p.Imports {
			if a.thirdParty(q.PkgPath) && !a.declared[q.Module.Path] && (!own || !forGenerated[q.PkgPath]) {
				missing[q.Module.Path] = true
			}
		}
	})
	if len(missing) == 0 {
		return
	}
	lines := a.requireLines()
	for m := range missing {
		a.report("go.mod", max(lines[m], 1), 0, "E-LINT-002",
			"the app uses module "+m+", which modules: in aicoded.yaml does not declare")
	}
}

// requireLines returns the line of each module's require in the app's go.mod.
func (a *app) requireLines() map[string]int {
	lines := map[string]int{}
	data, err := os.ReadFile(filepath.Join(a.dir, "go.mod"))
	if err != nil {
		return lines
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return lines
	}
	for _, r := range f.Require {
		lines[r.Mod.Path] = r.Syntax.Start.Line
	}
	return lines
}

// preLoad reports, with E-LINT-011, the two things that decide which files the go command reads
// before any package is loaded: a GOFLAGS value with an entry other than the few lint allows, and
// a vendor folder at the app root, which the go command builds in place of the required modules.
// It returns whether it reported anything, so the caller can stop before an untrustworthy load.
func (a *app) preLoad(ctx context.Context, env []string) bool {
	before := len(a.found)
	a.goflags(ctx, env)
	if fi, err := os.Stat(filepath.Join(a.dir, "vendor")); err == nil && fi.IsDir() {
		pos, line := "vendor", 0
		if _, err := os.Stat(filepath.Join(a.dir, "vendor", "modules.txt")); err == nil {
			pos, line = "vendor/modules.txt", 1
		}
		a.report(pos, line, 0, "E-LINT-011", "the go command builds the app from its vendor folder, which can hold a changed copy of any module")
	}
	return len(a.found) > before
}

// replaces reports, with E-LINT-011, a replace directive of the app's go.mod (except the framework
// replaced with the checkout frameworkDir this aicoded was installed from) and the workspace file
// work, both of which build the app from code other than the modules its go.mod requires.
func (a *app) replaces(frameworkDir, work string) error {
	data, err := os.ReadFile(filepath.Join(a.dir, "go.mod"))
	if err != nil {
		return err
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return err
	}
	checkout := realDir(frameworkDir)
	for _, r := range f.Replace {
		msg := "go.mod replaces " + r.Old.Path + " with " + r.New.Path
		if r.New.Version != "" {
			msg += " " + r.New.Version
		}
		if r.Old.Path == Framework {
			dir := r.New.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(a.dir, dir)
			}
			switch {
			case frameworkDir == "":
				msg += ", and this aicoded was not installed from a framework checkout"
			case checkout == "":
				msg += ", and the framework checkout this aicoded was installed from, " + frameworkDir + ", cannot be found"
			case r.New.Version != "" || realDir(dir) != checkout:
				msg += ", which is not the framework checkout this aicoded was installed from"
			default:
				continue
			}
		}
		a.report("go.mod", r.Syntax.Start.Line, 0, "E-LINT-011", msg)
	}
	if work != "" {
		a.report(a.rel(work), 1, 0, "E-LINT-011", "the go command builds the app with the workspace file "+a.rel(work)+", whose use and replace lines stand in for modules")
	}
	return nil
}

// realDir returns the folder dir as an absolute path with its symbolic links resolved, or "" when
// dir is "" or cannot be resolved.
func realDir(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ""
	}
	return resolved
}

// declaredModules reports, with E-LINT-003, what a package of a declared module may not hold:
// an import of a capability, of a net/... package outside netAllowed or of the framework, a
// //go:linkname directive, a source file that is not Go, a name of net/http that app code may not
// use, a name of processes or reflection, a way to open a database or reach its driver, a name
// that E-LINT-006 or E-LINT-009 refuses in app code, such as slog.SetDefault, a change of a
// package-level variable of the standard library or the framework, and SQL it runs, or may run
// through a method that lint cannot tell from one that takes SQL. It reads the files the build
// leaves out too, for their imports and directives.
func (a *app) declaredModules(pkgs []*packages.Package) {
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if a.isDeclared(p) {
			a.declaredPackage(p)
		}
	})
}

// declaredPackage checks p, a package of a declared module.
func (a *app) declaredPackage(p *packages.Package) {
	mod := p.Module.Path
	files := slices.Clone(p.Syntax)
	fset := p.Fset
	for _, name := range slices.Concat(p.OtherFiles, p.IgnoredFiles) {
		switch {
		case strings.HasSuffix(name, "_test.go"):
		case filepath.Ext(name) == ".go":
			f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
			if err != nil {
				a.report(a.inModule(p, name), 1, 0, "E-LINT-003", "declared module "+mod+" holds "+filepath.Base(name)+", which the checks cannot read")
				continue
			}
			files = append(files, f)
		default:
			a.report(a.inModule(p, name), 1, 0, "E-LINT-003", "declared module "+mod+" holds "+filepath.Base(name)+", which is not Go source")
		}
	}
	at := func(pos token.Pos, msg string) {
		position := fset.PositionFor(pos, false)
		a.report(a.inModule(p, position.Filename), position.Line, position.Column, "E-LINT-003", "declared module "+mod+" "+msg)
	}
	for _, f := range files {
		for _, spec := range f.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if capability(path) {
				at(spec.Pos(), "imports "+strconv.Quote(path))
			}
		}
		for _, group := range f.Comments {
			for _, c := range group.List {
				if strings.HasPrefix(c.Text, "//go:linkname ") {
					at(c.Pos(), "uses //go:linkname")
				}
			}
		}
	}
	for _, f := range p.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			for _, x := range targets(n) {
				if id, v := a.changedVar(p.TypesInfo, x, nil); v != nil {
					at(id.Pos(), "changes "+display(v.Pkg().Path()+"."+v.Name()))
				}
			}
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			obj := p.TypesInfo.Uses[id]
			if obj == nil || obj.Pkg() == nil {
				return true
			}
			if name := a.refusedAs(obj); refused[name] == "E-LINT-006" || refused[name] == "E-LINT-009" {
				at(id.Pos(), "uses "+display(name))
			} else if m := a.sqlMethod(obj); m != "" {
				at(id.Pos(), "runs SQL with "+display(m))
			} else if a.unmatchedSQL(obj) {
				at(id.Pos(), "calls "+id.Name+", which has type parameters in its signature, so lint cannot tell whether it takes SQL")
			} else if obj.Pkg().Path() == "net/http" && obj.Parent() == obj.Pkg().Scope() && !listed(restricted["net/http"], obj.Name()) {
				at(id.Pos(), "uses "+display("net/http."+obj.Name()))
			} else if name := member(obj); process(name) || slices.Contains(reflection, name) || driverMethod(name) {
				at(id.Pos(), "uses "+display(name))
			} else if m := standsFor(obj, a.reflection); m != nil {
				at(id.Pos(), "uses "+display(member(m)))
			}
			return true
		})
	}
}

// capability reports whether a package of a declared module may not import the package at path.
func capability(path string) bool {
	if strings.HasPrefix(path, "net/") {
		return !slices.Contains(netAllowed, path)
	}
	return slices.Contains(capabilities, path) || path == Framework || strings.HasPrefix(path, Framework+"/")
}

// process reports whether name, as member names it, is one of processes or a method of
// os.Process.
func process(name string) bool {
	return slices.Contains(processes, name) || strings.HasPrefix(name, "os.Process.")
}

// inModule names file, a file of p, as module@version/path, the same on every machine. A file
// that is not in the module's folder is named as rel names it.
func (a *app) inModule(p *packages.Package, file string) string {
	rel, err := filepath.Rel(p.Module.Dir, file)
	if p.Module.Dir == "" || err != nil || !filepath.IsLocal(rel) {
		return a.rel(file)
	}
	return p.Module.Path + "@" + p.Module.Version + "/" + filepath.ToSlash(rel)
}
