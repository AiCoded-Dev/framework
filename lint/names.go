package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// forGeneratedCode are the framework's functions and methods that only generated code, or the
// framework itself, calls (E-LINT-005), by path in the framework and name, a method as
// Type.Method. Each has the go doc line "Generated code only.".
var forGeneratedCode = []string{
	"rpc.Call", "rpc.DecodeError", "rpc.NewServer",
	"web.CheckNoGuard", "web.DecodeArgs", "web.LogError", "web.New", "web.Request.CSRFToken",
	"web/form.BaseFormValues.MarkValidated", "web/form.BaseFormValues.Prepare", "web/form.BaseFormValues.SetElements",
	"web/form.File.Process", "web/form.FileMultiple.Process", "web/form.Input.Process", "web/form.InputMultiple.Process",
	"web/form.IsMultipart", "web/form.Select.Process", "web/form.SelectMultiple.Process", "web/form.Textarea.Process",
	"web/reactive.Accept", "web/reactive.Decode", "web/reactive.HTMLBinding", "web/reactive.TextBinding",
}

// refused maps the members that app code may not use, by import path and name, a method as
// Type.Method, to the rule that refuses them.
var refused = func() map[string]string {
	m := map[string]string{
		"database/sql.Open":                "E-LINT-006",
		"database/sql.OpenDB":              "E-LINT-006",
		"database/sql.Register":            "E-LINT-006",
		"database/sql.DB.Driver":           "E-LINT-006",
		"database/sql.Conn.Raw":            "E-LINT-009",
		"database/sql.ColumnType.ScanType": "E-LINT-009",
		"log/slog.NewLogLogger":            "E-LINT-009",
		"log/slog.SetDefault":              "E-LINT-009",
		"log/slog.SetLogLoggerLevel":       "E-LINT-009",
	}
	for _, name := range forGeneratedCode {
		m[Framework+"/"+name] = "E-LINT-005"
	}
	return m
}()

// restricted are the packages whose package-level names app code may use only when they are
// listed here (E-LINT-009). A name that ends in * stands for every name with that prefix.
var restricted = map[string][]string{
	"database/sql": {
		"ColumnType", "Conn", "DB", "DBStats", "ErrConnDone", "ErrNoRows", "ErrTxDone", "IsolationLevel",
		"Level*", "Named", "NamedArg", "Null", "NullBool", "NullByte", "NullFloat64", "NullInt16", "NullInt32",
		"NullInt64", "NullString", "NullTime", "Out", "RawBytes", "Result", "Row", "Rows", "Scanner", "Stmt",
		"Tx", "TxOptions",
	},
	"net/http": {
		"CanonicalHeaderKey", "Cookie", "DetectContentType", "ErrNoCookie", "Handler", "Header", "Method*",
		"Request", "ResponseWriter", "SameSite*", "Status*", "TimeFormat",
	},
}

// Banned returns the code of the rule that refuses, in a hand-written file of an app that
// declares no modules, the import of the package at path when name is "", or else the use of
// its member name: a function, variable, constant or type, or a method or field written
// Type.Name, which is refused also when its type is. It returns "" when no rule refuses it.
// Banned("os", "") is "E-LINT-001", Banned("database/sql", "Open") is "E-LINT-006",
// Banned("net/http", "Request") is "" and Banned("net/http", "Client.Do") is "E-LINT-009".
//
// Banned answers for the packages of the standard library and the framework: a path whose first
// element has no dot names one of them, and any other path is E-LINT-002, a module that is not
// declared. Every answer holds in a generated file too, except E-LINT-005 and the import of a
// package that only generated files may import, which it may make. A change of a variable of
// the standard library or the framework is E-LINT-009 even where Banned allows its use.
func Banned(path, name string) string {
	if !allowed[path] && !stdOrFramework(path) {
		return "E-LINT-002"
	}
	if !allowed[path] {
		return "E-LINT-001"
	}
	if name == "" {
		return ""
	}
	if code := refused[path+"."+name]; code != "" {
		return code
	}
	typ, _, _ := strings.Cut(name, ".")
	if names, ok := restricted[path]; ok && !listed(names, typ) {
		return "E-LINT-009"
	}
	return ""
}

// listed reports whether names lists name, itself or by a prefix ending in *.
func listed(names []string, name string) bool {
	return slices.ContainsFunc(names, func(n string) bool {
		prefix, wild := strings.CutSuffix(n, "*")
		return n == name || wild && strings.HasPrefix(name, prefix)
	})
}

// methodsOf returns the methods among members, as refused names them, that pkgs declare, by
// name, so that a call of an interface method with the same name and signature counts as a call
// of the method.
func methodsOf(pkgs []*packages.Package, members []string) map[string][]*types.Func {
	out := map[string][]*types.Func{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, name := range members {
			pkg, rest := splitMember(name)
			typ, method, ok := strings.Cut(rest, ".")
			if !ok || pkg != p.PkgPath || p.Types == nil {
				continue
			}
			tn, _ := p.Types.Scope().Lookup(typ).(*types.TypeName)
			if tn == nil {
				continue
			}
			obj, _, _ := types.LookupFieldOrMethod(tn.Type(), true, p.Types, method)
			if fn, ok := obj.(*types.Func); ok {
				out[method] = append(out[method], fn)
			}
		}
	})
	return out
}

// names reports every use of a member that refused lists (E-LINT-005, E-LINT-006 and
// E-LINT-009), also as a method of an interface with the same name and signature; every use of a
// name of a restricted package that it does not list, and of a method or field of package
// reflect, which app code cannot import but may get a value of (E-LINT-009); every use of an
// unexported name that a generated file declares (E-LINT-005); every change of a variable of the
// standard library or the framework, or of a value of their types through * (E-LINT-009); and
// every conversion of a pointer to one of their named types into another pointer type, and every
// conversion that pointerConversion refuses for a type parameter (E-LINT-009). It reads generated
// files too, which hold the code of the templates, but not for E-LINT-005: generated code makes
// those calls.
func (a *app) names(p *analysis.Pass) {
	for _, f := range p.Files {
		generated := a.isGenerated(p, f)
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				a.use(p, n, generated)
			case *ast.CallExpr:
				a.pointerConversion(p, n)
			}
			for _, x := range targets(n) {
				a.change(p, x)
			}
			return true
		})
	}
}

// pointerConversion reports, with E-LINT-009, a conversion from a pointer to a named type of the
// standard library or the framework into a different pointer type, such as (*logger)(slog.Default())
// for a local type logger = slog.Logger, since the new pointer would let app code write through it
// to a value it may not change; and a conversion where either side may be a pointer that lint
// cannot see: a pointer to a type parameter, or a type parameter that mayBePointer reports. A
// conversion to an interface type that is not a type parameter is allowed.
func (a *app) pointerConversion(p *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) != 1 || !p.TypesInfo.Types[call.Fun].IsType() {
		return
	}
	to, from := p.TypesInfo.Types[call.Fun].Type, p.TypesInfo.TypeOf(call.Args[0])
	if _, tp := types.Unalias(to).(*types.TypeParam); !tp && types.IsInterface(to) {
		return
	}
	if mayBePointer(to) || mayBePointer(from) {
		diagnose(p, call.Pos(), "E-LINT-009", "app code may not convert to or from a type parameter that could be a pointer, or a pointer to a type parameter: use the concrete types")
		return
	}
	toElem, fromElem := pointee(to), pointee(from)
	if toElem == nil || fromElem == nil || types.Identical(toElem, fromElem) {
		return
	}
	for _, elem := range []types.Type{fromElem, toElem} {
		if named, ok := a.restrictedPointee(elem); ok {
			diagnose(p, call.Pos(), "E-LINT-009", "app code may not convert a pointer to "+display(named)+" to another pointer type")
			return
		}
	}
}

// pointee returns the type that t, a pointer type, points to, or nil when t is not a pointer.
func pointee(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		return ptr.Elem()
	}
	return nil
}

// restrictedPointee returns the member name of elem when it is a named type of the standard
// library or the framework, for a pointer message; ok is false otherwise.
func (a *app) restrictedPointee(elem types.Type) (name string, ok bool) {
	if elem == nil {
		return "", false
	}
	named, is := types.Unalias(elem).(*types.Named)
	if !is || named.Obj().Pkg() == nil {
		return "", false
	}
	if path := named.Obj().Pkg().Path(); !a.own[path] && stdOrFramework(path) {
		return path + "." + named.Obj().Name(), true
	}
	return "", false
}

// mayBePointer reports whether t is a pointer to a type parameter, or a type parameter whose
// type set holds a pointer type or cannot be listed.
func mayBePointer(t types.Type) bool {
	if ofTypeParam(pointee(t)) {
		return true
	}
	tp, ok := types.Unalias(t).(*types.TypeParam)
	if !ok {
		return false
	}
	terms, listed := typeTerms(tp.Constraint())
	return !listed || slices.ContainsFunc(terms, func(term types.Type) bool { return pointee(term) != nil })
}

// typeTerms returns the types that the constraint t lists, walking embedded interfaces and unions.
// An element that lists none, such as an interface of methods only, adds nothing; listed is false
// when no element lists a type, so the type set holds every type that has the methods.
func typeTerms(t types.Type) (terms []types.Type, listed bool) {
	iface, ok := t.Underlying().(*types.Interface)
	if !ok {
		return []types.Type{t}, true
	}
	for i := range iface.NumEmbeddeds() {
		var element []types.Type
		ok := true
		if u, isUnion := iface.EmbeddedType(i).(*types.Union); isUnion {
			for j := 0; ok && j < u.Len(); j++ {
				var ts []types.Type
				ts, ok = typeTerms(u.Term(j).Type())
				element = append(element, ts...)
			}
		} else {
			element, ok = typeTerms(iface.EmbeddedType(i))
		}
		if ok {
			terms, listed = append(terms, element...), true
		}
	}
	return terms, listed
}

// use reports the use of id when a rule refuses it. In a generated file, E-LINT-005 does not
// apply.
func (a *app) use(p *analysis.Pass, id *ast.Ident, generated bool) {
	obj := p.TypesInfo.Uses[id]
	if obj == nil || obj.Pkg() == nil {
		return
	}
	if name := a.refusedAs(obj); name != "" {
		if code := refused[name]; code != "E-LINT-005" || !generated {
			a.refuse(p, id, code, name)
		}
		return
	}
	pkg := obj.Pkg().Path()
	if names, ok := restricted[pkg]; ok && obj.Parent() == obj.Pkg().Scope() && !listed(names, obj.Name()) {
		diagnose(p, id.Pos(), "E-LINT-009", "app code may not use "+display(pkg+"."+obj.Name()))
		return
	}
	if pkg == "reflect" {
		diagnose(p, id.Pos(), "E-LINT-009", "app code may not use "+obj.Name()+" of package reflect")
		return
	}
	if !generated && a.own[pkg] && !obj.Exported() {
		if file := a.rel(p.Fset.PositionFor(obj.Pos(), false).Filename); a.generated[file] {
			diagnose(p, id.Pos(), "E-LINT-005", "only generated code may use "+obj.Name()+", which "+file+" declares")
		}
	}
}

// refusedAs returns the member of refused that obj is, or that obj, a method of an interface,
// stands for. It returns "" for any other obj.
func (a *app) refusedAs(obj types.Object) string {
	name := member(obj)
	if refused[name] != "" {
		return name
	}
	if m := standsFor(obj, a.methods); m != nil {
		return member(m)
	}
	return ""
}

// standsFor returns the method of methods that obj, a method of an interface, stands for: one
// with its name and signature, where a type parameter in obj's signature stands for any type.
// It returns nil for any other obj.
func standsFor(obj types.Object, methods map[string][]*types.Func) *types.Func {
	fn := interfaceMethod(obj)
	if fn == nil {
		return nil
	}
	for _, m := range methods[fn.Name()] {
		if matches(fn.Type(), m.Type()) {
			return m
		}
	}
	return nil
}

// interfaceMethod returns obj when it is a method of an interface or of a type parameter, or
// else nil.
func interfaceMethod(obj types.Object) *types.Func {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Signature().Recv() == nil || !types.IsInterface(fn.Signature().Recv().Type()) {
		return nil
	}
	return fn
}

// matches reports whether the types x and y are identical once every type parameter in x
// stands for any type.
func matches(x, y types.Type) bool {
	x, y = types.Unalias(x), types.Unalias(y)
	switch x := x.(type) {
	case *types.TypeParam:
		return true
	case *types.Pointer:
		y, ok := y.(*types.Pointer)
		return ok && matches(x.Elem(), y.Elem())
	case *types.Slice:
		y, ok := y.(*types.Slice)
		return ok && matches(x.Elem(), y.Elem())
	case *types.Signature:
		y, ok := y.(*types.Signature)
		return ok && x.Variadic() == y.Variadic() && matchesAll(x.Params(), y.Params()) && matchesAll(x.Results(), y.Results())
	}
	return types.Identical(x, y)
}

// matchesAll reports whether the tuples x and y have as many types, and each of x matches the
// one of y at its place.
func matchesAll(x, y *types.Tuple) bool {
	if x.Len() != y.Len() {
		return false
	}
	for i := range x.Len() {
		if !matches(x.At(i).Type(), y.At(i).Type()) {
			return false
		}
	}
	return true
}

// hasTypeParam reports whether t mentions a type parameter.
func hasTypeParam(t types.Type) bool {
	switch t := t.(type) {
	case *types.TypeParam:
		return true
	case *types.Alias:
		return hasTypeParam(types.Unalias(t)) || anyTypeParam(t.TypeArgs())
	case *types.Named:
		return anyTypeParam(t.TypeArgs())
	case *types.Pointer:
		return hasTypeParam(t.Elem())
	case *types.Slice:
		return hasTypeParam(t.Elem())
	case *types.Array:
		return hasTypeParam(t.Elem())
	case *types.Chan:
		return hasTypeParam(t.Elem())
	case *types.Map:
		return hasTypeParam(t.Key()) || hasTypeParam(t.Elem())
	case *types.Signature:
		return hasTypeParam(t.Params()) || hasTypeParam(t.Results())
	case *types.Tuple:
		for i := range t.Len() {
			if hasTypeParam(t.At(i).Type()) {
				return true
			}
		}
	case *types.Struct:
		for i := range t.NumFields() {
			if hasTypeParam(t.Field(i).Type()) {
				return true
			}
		}
	case *types.Interface:
		for i := range t.NumEmbeddeds() {
			if hasTypeParam(t.EmbeddedType(i)) {
				return true
			}
		}
		for i := range t.NumExplicitMethods() {
			if hasTypeParam(t.ExplicitMethod(i).Type()) {
				return true
			}
		}
	case *types.Union:
		for i := range t.Len() {
			if hasTypeParam(t.Term(i).Type()) {
				return true
			}
		}
	}
	return false
}

// anyTypeParam reports whether a type of list mentions a type parameter.
func anyTypeParam(list *types.TypeList) bool {
	for i := range list.Len() {
		if hasTypeParam(list.At(i)) {
			return true
		}
	}
	return false
}

// refuse reports the use at id of the member name, which the rule code refuses.
func (a *app) refuse(p *analysis.Pass, id *ast.Ident, code, name string) {
	if code == "E-LINT-005" {
		diagnose(p, id.Pos(), code, "only generated code may call "+display(name))
	} else {
		diagnose(p, id.Pos(), code, "app code may not use "+display(name))
	}
}

// targets returns the expressions that n changes, or lets change: the left side of an
// assignment, the operand of ++ or --, the key and value that a range statement assigns, and the
// operand of &.
func targets(n ast.Node) []ast.Expr {
	switch n := n.(type) {
	case *ast.AssignStmt:
		return n.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{n.X}
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN {
			return slices.DeleteFunc([]ast.Expr{n.Key, n.Value}, func(x ast.Expr) bool { return x == nil })
		}
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			return []ast.Expr{n.X}
		}
	}
	return nil
}

// change reports x, a target that targets returns, when it is, or lies in, a variable of the
// standard library or the framework, or when it is reached through * and its pointee is a named
// type of the standard library or the framework, or a type parameter, which could point at one.
func (a *app) change(p *analysis.Pass, x ast.Expr) {
	id, v := a.changedVar(p.TypesInfo, x, func(e *ast.StarExpr) bool {
		et := types.Unalias(p.TypesInfo.TypeOf(e))
		if _, ok := et.(*types.TypeParam); ok {
			diagnose(p, e.Pos(), "E-LINT-009", "app code may not change a value through a pointer to a type parameter, which could point at a value of the standard library or the framework")
			return false
		}
		if named, ok := a.restrictedPointee(et); ok {
			diagnose(p, e.Pos(), "E-LINT-009", "app code may not change a "+display(named)+" through a pointer")
			return false
		}
		return true
	})
	if v != nil {
		diagnose(p, id.Pos(), "E-LINT-009", "app code may not change "+display(v.Pkg().Path()+"."+v.Name()))
	}
}

// changedVar walks x, a target that targets returns, through parentheses, indexes, fields and *
// down to the name it changes, and returns that name and its variable when it is a package-level
// variable of the standard library or the framework, or nil. At each *, it first calls star, when
// star is not nil, and stops when star returns false.
func (a *app) changedVar(info *types.Info, x ast.Expr, star func(*ast.StarExpr) bool) (*ast.Ident, *types.Var) {
	for {
		switch e := x.(type) {
		case *ast.ParenExpr:
			x = e.X
		case *ast.StarExpr:
			if star != nil && !star(e) {
				return nil, nil
			}
			x = e.X
		case *ast.IndexExpr:
			x = e.X
		case *ast.IndexListExpr:
			x = e.X
		case *ast.SelectorExpr:
			if v, ok := info.Uses[e.Sel].(*types.Var); ok && !v.IsField() {
				x = e.Sel
			} else {
				x = e.X
			}
		case *ast.Ident:
			v, ok := info.Uses[e].(*types.Var)
			if ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope() && !a.own[v.Pkg().Path()] && stdOrFramework(v.Pkg().Path()) {
				return e, v
			}
			return nil, nil
		default:
			return nil, nil
		}
	}
}

// member returns how refused names obj: its import path and name, a method as Type.Method. It
// returns "" for a method of an unnamed type and for a name that is not at package level.
func member(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Signature().Recv() == nil {
		if obj.Parent() != obj.Pkg().Scope() {
			return ""
		}
		return obj.Pkg().Path() + "." + obj.Name()
	}
	fn = fn.Origin()
	t := fn.Signature().Recv().Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	return fn.Pkg().Path() + "." + named.Obj().Name() + "." + fn.Name()
}

// display returns a member as app code writes it: the package's name, a dot and the name.
func display(name string) string {
	pkg, rest := splitMember(name)
	return path.Base(pkg) + "." + rest
}

// splitMember splits a member into its import path and its name.
func splitMember(name string) (pkg, rest string) {
	slash := strings.LastIndex(name, "/") + 1
	dot := strings.Index(name[slash:], ".")
	return name[:slash+dot], name[slash+dot+1:]
}

// stdOrFramework reports whether the package at path is in the standard library or the framework.
func stdOrFramework(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".") || path == Framework || strings.HasPrefix(path, Framework+"/")
}
