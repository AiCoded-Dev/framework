package lint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// foreign are the extensions of the source files, other than Go, that the go command builds into
// a package.
var foreign = []string{
	".c", ".cc", ".cpp", ".cxx", ".f", ".F", ".f90", ".for", ".h", ".hh", ".hpp", ".hxx", ".m",
	".s", ".S", ".swig", ".swigcxx", ".sx", ".syso",
}

// files reports, with E-LINT-010, every file below the app folder that the checks cannot read:
// a source file in another language, and a Go file, other than a test, that none of the loaded
// packages compiles, such as one in a folder that the ignore directives of go.mod name. It walks
// the folders the go command reads for ./..., but for the ignore directives, so it skips
// testdata, the folders whose names start with . or _, and other modules.
func (a *app) files(pkgs []*packages.Package) error {
	compiled := map[string]bool{}
	for _, p := range pkgs {
		for _, f := range p.CompiledGoFiles {
			compiled[a.rel(f)] = true
		}
	}
	ignores := a.ignores()
	return filepath.WalkDir(a.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path == a.dir {
				return nil
			}
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Lstat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		rel := a.rel(path)
		switch {
		case strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || strings.HasSuffix(name, "_test.go"):
		case slices.Contains(foreign, filepath.Ext(name)):
			a.report(rel, 1, 0, "E-LINT-010", rel+" is not Go source")
		case filepath.Ext(name) == ".go" && !compiled[rel]:
			switch {
			case ignored(rel, ignores):
				a.report(rel, 1, 0, "E-LINT-010", rel+" is in a folder that the ignore directive of go.mod names, so the checks do not read it")
			case cgo(path):
				a.report(rel, 1, 0, "E-LINT-010", rel+` imports "C" (cgo)`)
			default:
				a.report(rel, 1, 0, "E-LINT-010", rel+" is left out of this build")
			}
		}
		return nil
	})
}

// ignores returns the paths of the ignore directives of the app's go.mod.
func (a *app) ignores() []string {
	data, err := os.ReadFile(filepath.Join(a.dir, "go.mod"))
	if err != nil {
		return nil
	}
	f, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return nil
	}
	var out []string
	for _, i := range f.Ignore {
		out = append(out, i.Path)
	}
	return out
}

// ignored reports whether the file rel, slash-separated and relative to the app folder, lies in
// a folder that an ignore directive names: one relative to the module root when the directive
// starts with ./, and one at any depth with those path elements otherwise.
func ignored(rel string, ignores []string) bool {
	dir := path.Dir(rel)
	for _, ig := range ignores {
		ig = strings.Trim(ig, "/")
		if root, ok := strings.CutPrefix(ig, "./"); ok {
			if dir == root || strings.HasPrefix(dir, root+"/") {
				return true
			}
		} else if inner := "/" + dir + "/"; strings.Contains(inner, "/"+ig+"/") {
			return true
		}
	}
	return false
}

// cgo reports whether the Go file at path imports "C".
func cgo(path string) bool {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(f.Imports, func(s *ast.ImportSpec) bool { return s.Path.Value == `"C"` })
}
