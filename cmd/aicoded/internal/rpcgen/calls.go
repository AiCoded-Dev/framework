package rpcgen

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
)

// skippedDirs are the folders Calls does not read, besides those whose names start with . or _.
var skippedDirs = map[string]bool{"testdata": true, "vendor": true, "node_modules": true}

// Calls returns, by app, the functions of the apps in held that the Go code of the app in dir
// refers to, sorted. held maps every app whose snapshot the app holds to that snapshot; the
// app's client is the package <module>/services/<package name>. Calls reads every regular Go
// file of the app that is neither a test nor generated, outside services/, other modules and the
// folders Go skips, and collects each selector <name>.<Func> where <name> is the client's name in
// that file and <Func> a function of its app. Dot imports are not followed. An app with no such
// selector has no entry.
//
// The scan is syntactic: in a file that imports a client, a selector on any other value with the
// client's name, such as a local variable that shadows it, counts too. That can only widen what
// the app declares it calls; the called app and the runner still check every call. A file with
// syntax errors is read as far as it parses; the Go build reports the errors.
func Calls(dir, module string, held map[string]rpcschema.Schema) (map[string][]string, error) {
	names, err := PackageNames(slices.Collect(maps.Keys(held)))
	if err != nil {
		return nil, err
	}
	byPath := map[string]string{}
	for app, name := range names {
		byPath[module+"/services/"+name] = app
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	fset := token.NewFileSet()
	found := map[string]map[string]bool{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p != ".":
			if name := d.Name(); p == "services" || name[0] == '.' || name[0] == '_' || skippedDirs[name] {
				return fs.SkipDir
			}
			if _, err := root.Stat(filepath.Join(filepath.FromSlash(p), "go.mod")); err == nil {
				return fs.SkipDir
			}
			return nil
		case !d.Type().IsRegular() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_gen.go"):
			return nil
		}
		src, err := root.ReadFile(filepath.FromSlash(p))
		if err != nil {
			return err
		}
		f, err := goparser.ParseFile(fset, p, src, goparser.SkipObjectResolution)
		if f == nil {
			return err
		}
		clients := map[string]string{}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			app, ok := byPath[path]
			if err != nil || !ok {
				continue
			}
			local := names[app]
			if imp.Name != nil {
				local = imp.Name.Name
			}
			clients[local] = app
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			app, ok := clients[x.Name]
			if _, method := held[app].Methods[sel.Sel.Name]; ok && method {
				if found[app] == nil {
					found[app] = map[string]bool{}
				}
				found[app][sel.Sel.Name] = true
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for app, funcs := range found {
		out[app] = slices.Sorted(maps.Keys(funcs))
	}
	return out, nil
}
