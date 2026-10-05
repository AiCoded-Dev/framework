// Package rpcgen reads the rpc/ package of an app and writes the Go code of calls between apps:
// the codec and method table of the app that serves the functions, and the clients of the apps
// that call them.
package rpcgen

import (
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/rpcschema"
	"aicoded.dev/framework/internal/errs"
)

// Access says who may call a function: the calling apps, the role rule a viewer must meet,
// written as in the access section and empty without role=, and whether an app may call the
// function with no viewer.
type Access struct {
	Callers []string
	Require []string
	Apps    bool
}

// Func is an exported function of the rpc/ package: its name, the names of its input and output
// types, who may call it, and its position as file:line relative to the app.
type Func struct {
	Name, In, Out string
	Access        Access
	Pos           string
}

// Package is the rpc/ package of the app in Dir: its functions sorted by name, and the schema of
// the types they take and return. The schema is unnumbered, names no app, and holds only the
// types the functions reach.
type Package struct {
	Dir    string
	Funcs  []Func
	Schema rpcschema.Schema
}

// scalars are the field types calls carry as they are.
var scalars = map[string]bool{
	"bool": true, "int32": true, "int64": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "string": true,
}

// narrowInts are the integer types calls do not carry, because their size depends on the
// platform or proto3 has no such type.
var narrowInts = map[string]bool{
	"int": true, "uint": true, "int8": true, "int16": true, "uint8": true, "uint16": true,
	"byte": true, "rune": true, "uintptr": true,
}

// reserved are the package-level names that rpc/server_gen.go declares or imports.
var reserved = map[string]bool{"Server": true, "context": true, "frameworkrpc": true, "protowire": true, "wire": true}

// Parse reads the rpc/ package of the app in dir: every Go file that is neither a test nor
// generated. The bool is false when the app has no rpc/ folder. It reports every function and
// type that breaks the rules of calls, sorted by position, and a file that does not parse.
func Parse(dir string) (Package, bool, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Package{}, false, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), "rpc")
	if errors.Is(err, fs.ErrNotExist) {
		return Package{}, false, nil
	}
	if err != nil {
		return Package{}, false, err
	}
	p := &parser{
		fset:   token.NewFileSet(),
		types:  map[string]typeDecl{},
		walked: map[string]bool{},
		schema: rpcschema.Schema{Methods: map[string]rpcschema.Method{}, Types: map[string]rpcschema.Type{}},
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		src, err := root.ReadFile(filepath.Join("rpc", name))
		if err != nil {
			return Package{}, true, err
		}
		f, err := goparser.ParseFile(p.fset, "rpc/"+name, src, goparser.ParseComments|goparser.SkipObjectResolution)
		if err != nil {
			return Package{}, true, err
		}
		files = append(files, f)
	}
	var funcs []funcDecl
	for _, f := range files {
		funcs = append(funcs, p.declarations(f)...)
	}
	var out []Func
	for _, d := range funcs {
		fn, ok := p.function(d)
		if fn.In != "" {
			p.walk(fn.In)
			p.walk(fn.Out)
		}
		if ok {
			out = append(out, fn)
			p.schema.Methods[fn.Name] = rpcschema.Method{In: fn.In, Out: fn.Out}
		}
	}
	if len(p.problems) > 0 {
		slices.SortStableFunc(p.problems, func(a, b problem) int {
			return cmp.Or(cmp.Compare(a.at.Filename, b.at.Filename), cmp.Compare(a.at.Line, b.at.Line))
		})
		list := make([]error, len(p.problems))
		for i, pr := range p.problems {
			list[i] = pr.err
		}
		return Package{}, true, errors.Join(list...)
	}
	slices.SortFunc(out, func(a, b Func) int { return cmp.Compare(a.Name, b.Name) })
	return Package{Dir: dir, Funcs: out, Schema: p.schema}, true, nil
}

// parser collects the declarations of the rpc/ package and the problems it finds in them.
type parser struct {
	fset     *token.FileSet
	types    map[string]typeDecl
	walked   map[string]bool
	schema   rpcschema.Schema
	problems []problem
}

// typeDecl is a type declaration with the imports of its file.
type typeDecl struct {
	spec    *ast.TypeSpec
	imports map[string]string
}

// funcDecl is an exported top-level function with the imports of its file.
type funcDecl struct {
	decl    *ast.FuncDecl
	imports map[string]string
}

type problem struct {
	at  token.Position
	err *errs.Error
}

// pos returns pos as file:line relative to the app.
func (p *parser) pos(pos token.Pos) string {
	at := p.fset.Position(pos)
	return fmt.Sprintf("%s:%d", at.Filename, at.Line)
}

func (p *parser) fail(pos token.Pos, code, format string, args ...any) {
	p.problems = append(p.problems, problem{at: p.fset.Position(pos), err: diag(p.pos(pos), code, format, args...)})
}

// kinds names the declarations of each keyword in messages.
var kinds = map[token.Token]string{token.CONST: "a const", token.IMPORT: "an import", token.TYPE: "a type", token.VAR: "a var"}

// declarations records the types of f and returns its exported top-level functions. It refuses
// a package not named rpc, a type that hides a predeclared Go type, the names rpc/server_gen.go
// uses, methods named like the generated ones, and //ssr:access lines anywhere but on an
// exported function.
func (p *parser) declarations(f *ast.File) []funcDecl {
	if f.Name.Name != "rpc" {
		p.fail(f.Name.Pos(), "E-RPC-003", "the package in rpc/ is named %s; name it rpc", f.Name.Name)
	}
	imps := imports(f)
	var out []funcDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			p.inert(d.Doc, kinds[d.Tok])
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					p.inert(s.Doc, kinds[d.Tok])
					if types.Universe.Lookup(s.Name.Name) != nil {
						p.fail(s.Name.Pos(), "E-RPC-003", "the type %s hides Go's own %s; rename it", s.Name.Name, s.Name.Name)
					}
					p.reserve(s.Name)
					p.types[s.Name.Name] = typeDecl{spec: s, imports: imps}
				case *ast.ValueSpec:
					p.inert(s.Doc, kinds[d.Tok])
					for _, n := range s.Names {
						p.reserve(n)
					}
				}
			}
		case *ast.FuncDecl:
			switch {
			case d.Recv != nil:
				p.inert(d.Doc, "a method")
				if n := d.Name.Name; n == "aicodedAppend" || n == "aicodedDecode" {
					p.fail(d.Name.Pos(), "E-RPC-003", "the method %s is written by aicoded generate; rename it", n)
				}
			case !d.Name.IsExported():
				p.inert(d.Doc, "an unexported function")
				p.reserve(d.Name)
			case !p.reserve(d.Name):
				out = append(out, funcDecl{decl: d, imports: imps})
			}
		}
	}
	return out
}

// inert refuses the //ssr:access lines of doc, which have no effect on what, and the lines of
// doc written // ssr:access.
func (p *parser) inert(doc *ast.CommentGroup, what string) {
	if doc == nil {
		return
	}
	for _, c := range doc.List {
		switch {
		case isDirective(c.Text):
			p.fail(c.Pos(), "E-RPC-002", "an //ssr:access line has no effect on %s; put it on an exported function, or remove it", what)
		case isSpaced(c.Text):
			p.spaced(c)
		}
	}
}

// spaced refuses c, a line written // ssr:access.
func (p *parser) spaced(c *ast.Comment) {
	p.fail(c.Pos(), "E-RPC-002", "this line is not read as //ssr:access because of the space after //; write //ssr:access without a space")
}

// reserve reports a package-level name that rpc/server_gen.go uses, and whether it was one.
func (p *parser) reserve(name *ast.Ident) bool {
	if !reserved[name.Name] {
		return false
	}
	p.fail(name.Pos(), "E-RPC-003", "the name %s is used by rpc/server_gen.go; rename it", name.Name)
	return true
}

// imports maps the local name of every import of f to its path.
func imports(f *ast.File) map[string]string {
	m := map[string]string{}
	for _, s := range f.Imports {
		p, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if s.Name != nil {
			name = s.Name.Name
		}
		m[name] = p
	}
	return m
}

// function returns the function d as a Func, and whether its //ssr:access line and its shape are
// right. In and Out are empty when the shape is wrong.
func (p *parser) function(d funcDecl) (Func, bool) {
	name := d.decl.Name.Name
	fn := Func{Name: name, Pos: p.pos(d.decl.Pos())}
	var lines []*ast.Comment
	spaced := false
	if d.decl.Doc != nil {
		for _, c := range d.decl.Doc.List {
			switch {
			case isDirective(c.Text):
				lines = append(lines, c)
			case isSpaced(c.Text):
				p.spaced(c)
				spaced = true
			}
		}
	}
	ok := len(lines) == 1 && !spaced
	switch len(lines) {
	case 0:
		if !spaced {
			p.fail(d.decl.Pos(), "E-RPC-001", "%s has no //ssr:access line, so no app may call it", name)
		}
	case 1:
		a, err := parseAccess(lines[0].Text)
		if err != nil {
			p.fail(lines[0].Pos(), "E-RPC-002", "%s: %v", name, err)
			ok = false
		}
		fn.Access = a
	default:
		p.fail(lines[1].Pos(), "E-RPC-002", "%s has more than one //ssr:access line", name)
	}
	t := d.decl.Type
	params, results := flatten(t.Params), flatten(t.Results)
	if t.TypeParams == nil && len(params) == 2 && len(results) == 2 && isContext(params[0], d.imports) && isIdent(results[1], "error") {
		fn.In, fn.Out = p.structName(params[1]), p.structName(results[0])
	}
	if fn.In == "" || fn.Out == "" {
		p.fail(d.decl.Pos(), "E-RPC-003", "%s must be func %s(ctx context.Context, in In) (Out, error), where In and Out are exported struct types of package rpc", name, name)
		return Func{}, false
	}
	return fn, ok
}

// flatten returns the type of every parameter or result in l, one per name.
func flatten(l *ast.FieldList) []ast.Expr {
	if l == nil {
		return nil
	}
	var out []ast.Expr
	for _, f := range l.List {
		for range max(len(f.Names), 1) {
			out = append(out, f.Type)
		}
	}
	return out
}

func isContext(e ast.Expr, imps map[string]string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && imps[x.Name] == "context" && sel.Sel.Name == "Context"
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// structName returns the name e refers to when it is an exported struct type declared in the
// package without type parameters, or "".
func (p *parser) structName(e ast.Expr) string {
	id, ok := e.(*ast.Ident)
	if !ok || !id.IsExported() {
		return ""
	}
	td, ok := p.types[id.Name]
	if !ok || td.spec.Assign.IsValid() || td.spec.TypeParams != nil {
		return ""
	}
	if _, ok := td.spec.Type.(*ast.StructType); !ok {
		return ""
	}
	return id.Name
}

// walk adds the struct type name and every struct its fields reach to the schema.
func (p *parser) walk(name string) {
	if p.walked[name] {
		return
	}
	p.walked[name] = true
	td := p.types[name]
	t := rpcschema.Type{Fields: map[string]rpcschema.Field{}}
	var reached []string
	for _, field := range td.spec.Type.(*ast.StructType).Fields.List {
		if len(field.Names) == 0 {
			p.fail(field.Pos(), "E-RPC-004", "%s embeds %s; give the field a name", name, types.ExprString(field.Type))
			continue
		}
		typ, why := p.fieldType(field.Type, td.imports)
		for _, n := range field.Names {
			switch {
			case !n.IsExported():
				p.fail(n.Pos(), "E-RPC-004", "%s.%s is unexported; export it or move it out of the type", name, n.Name)
			case why != "":
				p.fail(n.Pos(), "E-RPC-004", "%s.%s: %s", name, n.Name, why)
			default:
				t.Fields[n.Name] = rpcschema.Field{Type: typ}
			}
		}
		if why == "" {
			if s := strings.TrimLeft(typ, "[]*"); !scalars[s] && s != "bytes" && s != "time" {
				reached = append(reached, s)
			}
		}
	}
	p.schema.Types[name] = t
	for _, r := range reached {
		p.walk(r)
	}
}

// fieldType returns the snapshot type of a field of type e, declared in a file with the imports
// imps, or what makes it a type calls do not carry.
func (p *parser) fieldType(e ast.Expr, imps map[string]string) (string, string) {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return p.fieldType(e.X, imps)
	case *ast.Ident:
		switch {
		case scalars[e.Name]:
			return e.Name, ""
		case narrowInts[e.Name]:
			return "", fmt.Sprintf("%s is not allowed; use int32, int64, uint32 or uint64", e.Name)
		case p.structName(e) != "":
			return e.Name, ""
		}
		return "", fmt.Sprintf("%s is neither an allowed scalar nor an exported struct type of package rpc", e.Name)
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && imps[x.Name] == "time" && e.Sel.Name == "Time" {
			return "time", ""
		}
		return "", fmt.Sprintf("%s is a type of another package; only time.Time is allowed", types.ExprString(e))
	case *ast.ArrayType:
		if e.Len != nil {
			return "", "arrays are not allowed; use a slice"
		}
		if id, ok := e.Elt.(*ast.Ident); ok && (id.Name == "byte" || id.Name == "uint8") {
			return "bytes", ""
		}
		elem, why := p.fieldType(e.Elt, imps)
		switch {
		case why != "":
			return "", why
		case strings.HasPrefix(elem, "*"):
			return "", "a slice of pointers is not allowed; use a slice of values"
		case strings.HasPrefix(elem, "[]"):
			return "", "a slice of slices is not allowed, except [][]byte; put the inner slice in a struct"
		}
		return "[]" + elem, ""
	case *ast.StarExpr:
		elem, why := p.fieldType(e.X, imps)
		switch {
		case why != "":
			return "", why
		case elem == "bytes" || strings.HasPrefix(elem, "[]"):
			return "", "a pointer to a slice is not allowed; a nil slice already means none"
		case strings.HasPrefix(elem, "*"):
			return "", "a pointer to a pointer is not allowed"
		}
		return "*" + elem, ""
	case *ast.MapType:
		return "", "maps are not allowed; use a slice of structs"
	}
	return "", fmt.Sprintf("%s is not allowed", types.ExprString(e))
}
