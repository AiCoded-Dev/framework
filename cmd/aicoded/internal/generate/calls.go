package generate

import (
	"encoding/json"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
)

// callMethod returns the name of the data provider method that answers call c.
func callMethod(c template.Call) string { return "Call" + exported(c.Name) }

// writeCall writes the Call method of state, which reads the arguments of a page call and runs
// the call's method of the data provider. A name the page does not declare answers 404.
func writeCall(b *gobuf.GoBuf, calls []template.Call) {
	b.WriteStringLn("func (s *state) Call(ctx context.Context, r *web.Request, name string, args json.RawMessage) (any, error) {")
	b.WriteStringLn("switch name {")
	for _, c := range calls {
		b.WriteString("case ")
		b.WriteQuotedString(c.Name, ":\n")
		b.WriteStringLn("var in " + c.In)
		b.WriteStringLn("if err := web.DecodeArgs(args, &in); err != nil {")
		b.WriteStringLn("return nil, err")
		b.WriteStringLn("}")
		b.WriteStringLn("return s.dp." + callMethod(c) + "(ctx, r, in)")
	}
	b.WriteStringLn("}")
	b.WriteStringLn(`return nil, web.Error(http.StatusNotFound, "This call is not on this page.")`)
	b.WriteStringLn("}")
	b.WriteStringLn("")
}

// callsTS returns the Calls type of the route's reactive_gen.ts and adds the declarations of the
// types its calls send and receive to decls, by name.
func callsTS(r *Route, decls map[string]string) (string, error) {
	calls := r.Template.Calls()
	if len(calls) == 0 {
		return "export type Calls = Record<string, never>;\n", nil
	}
	pkg, err := loadPackage(r.Dir)
	if err != nil {
		return "", err
	}
	ts := &tsTypes{pkg: pkg, decls: decls, inline: map[string]bool{}}
	var b strings.Builder
	b.WriteString("export type Calls = {\n")
	for _, c := range calls {
		ts.warnings = nil
		entry := "  " + c.Name + ": { in: " + ts.source(c.In) + "; out: " + ts.source(c.Out) + " };\n"
		for _, w := range ts.warnings {
			b.WriteString("  // " + w + "\n")
		}
		b.WriteString(entry)
	}
	b.WriteString("};\n")
	return b.String(), nil
}

// goPackage holds the type declarations of a route's Go package and, by type, the methods
// through which a value writes or reads its own JSON or text: MarshalJSON, UnmarshalJSON,
// MarshalText and UnmarshalText.
type goPackage struct {
	types   map[string]*ast.TypeSpec
	files   map[string]*ast.File
	methods map[string]map[string]bool
}

// marshalers are the methods through which encoding/json lets a value write or read itself.
var marshalers = map[string]bool{"MarshalJSON": true, "UnmarshalJSON": true, "MarshalText": true, "UnmarshalText": true}

func (pkg *goPackage) addMethod(typ, method string) {
	if !marshalers[method] {
		return
	}
	if pkg.methods[typ] == nil {
		pkg.methods[typ] = map[string]bool{}
	}
	pkg.methods[typ][method] = true
}

// loadPackage reads the declarations of the Go files in dir that are neither tests nor
// generated.
func loadPackage(dir string) (*goPackage, error) {
	files, _, err := goFiles(dir)
	if err != nil {
		return nil, err
	}
	pkg := &goPackage{types: map[string]*ast.TypeSpec{}, files: map[string]*ast.File{}, methods: map[string]map[string]bool{}}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					pkg.types[ts.Name.Name] = ts
					pkg.files[ts.Name.Name] = f.file
					if it, ok := ts.Type.(*ast.InterfaceType); ok {
						for _, m := range it.Methods.List {
							for _, name := range m.Names {
								pkg.addMethod(ts.Name.Name, name.Name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv != nil && len(d.Recv.List) == 1 {
					pkg.addMethod(typeName(d.Recv.List[0].Type), d.Name.Name)
				}
			}
		}
	}
	return pkg, nil
}

// goFile is a parsed Go file of a route folder and its name.
type goFile struct {
	name string
	file *ast.File
}

// goFiles parses the Go files in dir that are neither tests nor generated, sorted by name. A file
// with syntax errors is read as far as it parses; the Go build reports the errors.
func goFiles(dir string) ([]goFile, *token.FileSet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	var out []goFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fset, filepath.Join(dir, name), nil, goparser.SkipObjectResolution)
		if f == nil {
			return nil, nil, err
		}
		out = append(out, goFile{name: name, file: f})
	}
	return out, fset, nil
}

// typeName returns the name of the type e names, past a pointer, a package and type arguments,
// or "".
func typeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return typeName(e.X)
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.IndexExpr:
		return typeName(e.X)
	case *ast.IndexListExpr:
		return typeName(e.X)
	case *ast.ParenExpr:
		return typeName(e.X)
	}
	return ""
}

// tsOwn are the types reactive_gen.ts declares or uses itself.
var tsOwn = map[string]bool{
	"ReadVars": true, "WriteVars": true, "Calls": true, "CallError": true, "CallMap": true, "Route": true,
	"Record": true, "Promise": true,
}

// tsDeclarable reports whether reactive_gen.ts can declare a type named name: it starts with an
// upper-case letter, so it is no JavaScript keyword or TypeScript primitive, and the file does
// not use it itself.
func tsDeclarable(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r) && !tsOwn[name]
}

// tsTypes maps the Go types of a route package to the TypeScript types of their JSON, as
// encoding/json writes and reads it. It declares every named type it maps in decls, once, and
// collects in warnings what it could not map exactly.
type tsTypes struct {
	pkg      *goPackage
	decls    map[string]string
	warnings []string
	// inline holds the types being written in place, so a cycle among them ends.
	inline map[string]bool
}

func (t *tsTypes) warn(format string, args ...any) {
	if w := fmt.Sprintf(format, args...); !slices.Contains(t.warnings, w) {
		t.warnings = append(t.warnings, w)
	}
}

// source returns the TypeScript type of the Go type src, an in or out type of a call.
func (t *tsTypes) source(src string) string {
	e, err := goparser.ParseExpr(src)
	if err != nil {
		return "unknown"
	}
	return t.of(e, nil)
}

// of returns the TypeScript type of the Go type e, written in file f.
func (t *tsTypes) of(e ast.Expr, f *ast.File) string {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return t.of(e.X, f)
	case *ast.Ident:
		if t.pkg.types[e.Name] != nil {
			return t.named(e.Name)
		}
		return predeclaredTS(e.Name)
	case *ast.SelectorExpr:
		return foreignTS(e, f)
	case *ast.StarExpr:
		return nullable(t.of(e.X, f))
	case *ast.ArrayType:
		if e.Len == nil && t.basic(e.Elt, 0) == "uint8" {
			return "string | null"
		}
		elem := t.of(e.Elt, f)
		if strings.HasSuffix(elem, " | null") {
			elem = "(" + elem + ")"
		}
		if e.Len == nil {
			return elem + "[] | null"
		}
		return elem + "[]"
	case *ast.MapType:
		return "Record<string, " + t.of(e.Value, f) + "> | null"
	case *ast.StructType:
		if ts := t.marshaled(nil, e); ts != "" {
			return ts
		}
		return t.object(e, f, false)
	}
	return "unknown"
}

// named returns the TypeScript type of the type the route package declares as name. It declares
// the type in decls, or writes it in place when it is an alias or its name cannot be declared.
// A generic type is unknown, and so is a type that writes or reads its own JSON, directly or
// through a method an embedded type promotes, unless it writes and reads itself as text, which
// makes it a string.
func (t *tsTypes) named(name string) string {
	spec := t.pkg.types[name]
	if spec.TypeParams != nil {
		return "unknown"
	}
	if spec.Assign.IsValid() || !tsDeclarable(name) {
		if t.inline[name] {
			t.warn("cyclic type %q substituted with unknown in TypeScript interface", name)
			return "unknown"
		}
		if tsOwn[name] {
			t.warn("type %q is written in place, because reactive_gen.ts cannot declare that name", name)
		}
		t.inline[name] = true
		defer delete(t.inline, name)
		if spec.Assign.IsValid() {
			return t.of(spec.Type, t.pkg.files[name])
		}
	}
	st, _ := t.structOf(name)
	if ts := t.marshaled(t.pkg.methods[name], st); ts != "" {
		return ts
	}
	if !tsDeclarable(name) {
		return t.body(name, false)
	}
	if _, ok := t.decls[name]; !ok {
		t.decls[name] = ""
		t.decls[name] = "export type " + name + " = " + t.body(name, true) + ";"
	}
	return name
}

// body returns the TypeScript type of the defined type name: that of the type it is defined as,
// past other named types, since it does not take their methods.
func (t *tsTypes) body(name string, multiline bool) string {
	u, f := t.underlying(name, 0)
	if st, ok := u.(*ast.StructType); ok {
		return t.object(st, f, multiline)
	}
	return t.of(u, f)
}

// marshaled returns the TypeScript type of a value with the marshaling methods own, or of the
// struct st when st is not nil, if the value writes or reads its own JSON or text; "" otherwise.
// A struct takes the methods of the types it embeds, at any depth and whatever their tags. Its
// JSON is unknown when an embedded type comes from another package or is generic, because the
// generator cannot see its methods.
func (t *tsTypes) marshaled(own map[string]bool, st *ast.StructType) string {
	methods := maps.Clone(own)
	if methods == nil {
		methods = map[string]bool{}
	}
	if st != nil && !t.promoted(st, methods, map[string]bool{}) {
		return "unknown"
	}
	switch {
	case methods["MarshalJSON"] || methods["UnmarshalJSON"] || methods["MarshalText"] != methods["UnmarshalText"]:
		return "unknown"
	case methods["MarshalText"]:
		return "string"
	}
	return ""
}

// promoted adds to methods the marshaling methods the embedded types of st promote. It reports
// false when one of them comes from another package or is generic.
func (t *tsTypes) promoted(st *ast.StructType, methods, seen map[string]bool) bool {
	for _, fld := range st.Fields.List {
		if len(fld.Names) == 0 && !t.promotes(fld.Type, methods, seen) {
			return false
		}
	}
	return true
}

// promotes adds to methods the marshaling methods of the embedded type e and of the types it
// embeds. It reports false when e comes from another package or is generic.
func (t *tsTypes) promotes(e ast.Expr, methods, seen map[string]bool) bool {
	switch x := e.(type) {
	case *ast.StarExpr:
		return t.promotes(x.X, methods, seen)
	case *ast.ParenExpr:
		return t.promotes(x.X, methods, seen)
	case *ast.Ident:
		spec := t.pkg.types[x.Name]
		if spec == nil || seen[x.Name] {
			return true
		}
		seen[x.Name] = true
		switch {
		case spec.TypeParams != nil:
			return false
		case spec.Assign.IsValid():
			return t.promotes(spec.Type, methods, seen)
		}
		maps.Copy(methods, t.pkg.methods[x.Name])
		if st, _ := t.structOf(x.Name); st != nil {
			return t.promoted(st, methods, seen)
		}
		return true
	}
	return false
}

// basic returns the predeclared type e is, following the named types of the route package, such
// as "uint8" for byte or for type Level byte, or "" when e is not a predeclared type.
func (t *tsTypes) basic(e ast.Expr, depth int) string {
	if p, ok := e.(*ast.ParenExpr); ok {
		return t.basic(p.X, depth)
	}
	id, ok := e.(*ast.Ident)
	if !ok || depth > 16 {
		return ""
	}
	if spec := t.pkg.types[id.Name]; spec != nil {
		if spec.TypeParams != nil || len(t.pkg.methods[id.Name]) > 0 {
			return ""
		}
		return t.basic(spec.Type, depth+1)
	}
	switch id.Name {
	case "byte":
		return "uint8"
	case "rune":
		return "int32"
	}
	if predeclaredTS(id.Name) == "unknown" {
		return ""
	}
	return id.Name
}

// underlying returns the type the route package defines name as, past the other named types of
// the package it is defined from, and the file of that type. Their methods do not carry over.
func (t *tsTypes) underlying(name string, depth int) (ast.Expr, *ast.File) {
	spec := t.pkg.types[name]
	if id, ok := spec.Type.(*ast.Ident); ok && depth < 16 {
		if next := t.pkg.types[id.Name]; next != nil && next.TypeParams == nil {
			return t.underlying(id.Name, depth+1)
		}
	}
	return spec.Type, t.pkg.files[name]
}

// structOf returns the struct type the route package defines name as, and its file, or nil.
func (t *tsTypes) structOf(name string) (*ast.StructType, *ast.File) {
	if spec := t.pkg.types[name]; spec == nil || spec.TypeParams != nil {
		return nil, nil
	}
	u, f := t.underlying(name, 0)
	st, _ := u.(*ast.StructType)
	return st, f
}

// predeclaredTS returns the TypeScript type of a predeclared Go type.
func predeclaredTS(name string) string {
	switch name {
	case "bool":
		return "boolean"
	case "string":
		return "string"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64",
		"uintptr", "byte", "rune", "float32", "float64":
		return "number"
	}
	return "unknown"
}

// foreignTS returns the TypeScript type of a type from another package: time.Time is written as
// an RFC 3339 string and time.Duration as a number; any other is unknown.
func foreignTS(e *ast.SelectorExpr, f *ast.File) string {
	x, ok := e.X.(*ast.Ident)
	if !ok || importPath(f, x.Name) != "time" {
		return "unknown"
	}
	switch e.Sel.Name {
	case "Time":
		return "string"
	case "Duration":
		return "number"
	}
	return "unknown"
}

// importPath returns the path of the package file f imports as name, or "".
func importPath(f *ast.File, name string) string {
	if f == nil {
		return ""
	}
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		local := p[strings.LastIndex(p, "/")+1:]
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == name {
			return p
		}
	}
	return ""
}

func nullable(ts string) string {
	if ts == "unknown" || strings.HasSuffix(ts, " | null") {
		return ts
	}
	return ts + " | null"
}

// jsonField is a field of a struct's JSON object: its name, the index path of the Go field,
// whether the name comes from a tag, and whether the field may be missing.
type jsonField struct {
	name     string
	index    []int
	tagged   bool
	optional bool
	quoted   bool
	typ      ast.Expr
	file     *ast.File
}

// object returns the TypeScript object type of the struct st of file f, on several lines when
// multiline is set.
func (t *tsTypes) object(st *ast.StructType, f *ast.File, multiline bool) string {
	var props []string
	fields := t.fields(st, f)
	for _, fld := range fields {
		opt := ""
		if fld.optional {
			opt = "?"
		}
		ts := t.of(fld.typ, fld.file)
		if fld.quoted {
			ts = "string"
			if _, ptr := fld.typ.(*ast.StarExpr); ptr {
				ts = "string | null"
			}
		}
		props = append(props, propName(fld.name)+opt+": "+ts)
	}
	switch {
	case len(props) == 0:
		return "Record<string, never>"
	case multiline:
		return "{\n  " + strings.Join(props, ";\n  ") + ";\n}"
	}
	return "{ " + strings.Join(props, "; ") + " }"
}

// embedded is a struct whose fields encoding/json looks at: the root struct or an embedded one.
type embedded struct {
	name     string
	st       *ast.StructType
	file     *ast.File
	index    []int
	optional bool
}

// fields returns the JSON fields of the struct st of file f in the order and with the names
// encoding/json gives them. The embedded types of st are all of the route package, as marshaled
// checks first.
func (t *tsTypes) fields(st *ast.StructType, f *ast.File) []jsonField {
	var all []jsonField
	visited := map[string]bool{}
	next := []embedded{{st: st, file: f}}
	nextCount := map[string]int{}
	for len(next) > 0 {
		current, count := next, nextCount
		next, nextCount = nil, map[string]int{}
		for _, s := range current {
			if s.name != "" {
				if visited[s.name] {
					continue
				}
				visited[s.name] = true
			}
			i := -1
			for _, fld := range s.st.Fields.List {
				names := fld.Names
				if len(names) == 0 {
					names = []*ast.Ident{nil}
				}
				for _, id := range names {
					i++
					index := append(slices.Clone(s.index), i)
					typ, ptr := fld.Type, false
					if star, ok := typ.(*ast.StarExpr); ok {
						typ, ptr = star.X, true
					}
					goName := typeName(typ)
					var inner *ast.StructType
					var innerFile *ast.File
					if id == nil {
						if x, ok := typ.(*ast.Ident); ok {
							inner, innerFile = t.structOf(x.Name)
						}
						if !ast.IsExported(goName) && inner == nil {
							continue
						}
					} else if goName = id.Name; !ast.IsExported(goName) {
						continue
					}
					tag := ""
					if fld.Tag != nil {
						raw, _ := strconv.Unquote(fld.Tag.Value)
						tag = reflect.StructTag(raw).Get("json")
					}
					if tag == "-" {
						continue
					}
					name, opts, _ := strings.Cut(tag, ",")
					if !validTag(name) {
						name = ""
					}
					if id == nil && name == "" && inner != nil {
						nextCount[goName]++
						if nextCount[goName] == 1 {
							next = append(next, embedded{name: goName, st: inner, file: innerFile, index: index, optional: s.optional || ptr})
						}
						continue
					}
					jf := jsonField{name: name, index: index, tagged: name != "", typ: fld.Type, file: s.file,
						optional: s.optional || hasOpt(opts, "omitempty") || hasOpt(opts, "omitzero"),
						quoted:   hasOpt(opts, "string") && t.basic(typ, 0) != ""}
					if jf.name == "" {
						jf.name = goName
					}
					all = append(all, jf)
					if count[s.name] > 1 {
						all = append(all, jf)
					}
				}
			}
		}
	}
	return dominant(all)
}

// dominant keeps, of the fields that share a JSON name, the one encoding/json uses: the least
// deep one, and of those the tagged one. Two that tie are both left out. The result is in the
// order of the Go fields.
func dominant(fields []jsonField) []jsonField {
	slices.SortStableFunc(fields, func(a, b jsonField) int {
		switch {
		case a.name != b.name:
			return strings.Compare(a.name, b.name)
		case len(a.index) != len(b.index):
			return len(a.index) - len(b.index)
		case a.tagged != b.tagged:
			if a.tagged {
				return -1
			}
			return 1
		}
		return slices.Compare(a.index, b.index)
	})
	var out []jsonField
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].name == fields[i].name {
			j++
		}
		if j-i == 1 || len(fields[i].index) != len(fields[i+1].index) || fields[i].tagged != fields[i+1].tagged {
			out = append(out, fields[i])
		}
		i = j
	}
	slices.SortFunc(out, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	return out
}

func hasOpt(opts, want string) bool {
	return slices.Contains(strings.Split(opts, ","), want)
}

// validTag reports whether encoding/json takes name from a json tag as the field's name.
func validTag(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) && !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

// propName returns name as a TypeScript property name, quoted unless it is an identifier.
func propName(name string) string {
	ident := true
	for i, c := range name {
		letter := c == '_' || c == '$' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			ident = false
		}
	}
	if ident && name != "" {
		return name
	}
	q, _ := json.Marshal(name)
	return string(q)
}
