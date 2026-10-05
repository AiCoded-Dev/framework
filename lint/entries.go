package lint

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// entries are the fields through which an app hands the runner its entry points, by name and
// type, with the generated function that must fill each.
var entries = map[string]entry{
	"Handler": {typ: "net/http.Handler", fn: "NewHandler", call: "pages.NewHandler(...)"},
	"RPC":     {typ: "*" + Framework + "/rpc.Server", fn: "Server", call: "rpc.Server()"},
}

// entry is one field of entries.
type entry struct {
	typ  string // the field's type
	fn   string // the name of the generated function
	call string // the call as the docs write it
}

// runners are the functions of package app that take app.Options, by name, with the place of
// the options among their arguments.
var runners = map[string]int{"Main": 0, "Run": 1}

// entryPoints reports, with E-LINT-004, in hand-written files: a call of app.Main or app.Run
// whose options are not an app.Options composite literal written in the call, and any other use
// of app.Main or app.Run; and a Handler or RPC field of app.Options, or of any struct with a
// field of that name and type, that is set to anything but a direct call of the generated
// function: pages.NewHandler or rpc.Server. Such a field is set only in a composite literal,
// never by assignment or through a pointer. A field of that name whose type mentions a type
// parameter is refused wherever it is set, and so is a composite literal of a type parameter,
// since lint cannot tell whether either sets an entry point.
func (a *app) entryPoints(p *analysis.Pass) {
	for _, f := range p.Files {
		if a.isGenerated(p, f) {
			continue
		}
		runnerCalls(p, f)
		ast.Inspect(f, func(n ast.Node) bool {
			if lit, ok := n.(*ast.CompositeLit); ok {
				a.literal(p, lit)
			}
			for _, x := range targets(n) {
				a.setEntry(p, x)
			}
			return true
		})
	}
}

// runnerCalls reports, with E-LINT-004, a call in f of a function of runners whose options are
// not an app.Options composite literal written in the call, such as a conversion, a variable or
// a value that a function returns, and every other use of such a function, such as f := app.Main.
func runnerCalls(p *analysis.Pass, f *ast.File) {
	called := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, fn := runnerOf(p, ast.Unparen(call.Fun))
		if fn == nil {
			return true
		}
		called[id] = true
		i := runners[fn.Name()]
		if i < len(call.Args) && optionsLiteral(p, call.Args[i], fn.Signature().Params().At(i).Type()) {
			return true
		}
		pos := call.Pos()
		if i < len(call.Args) {
			pos = call.Args[i].Pos()
		}
		diagnose(p, pos, "E-LINT-004", "the options of app."+fn.Name()+" must be an app.Options literal written in the call, not a variable, a conversion or a value made elsewhere")
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && !called[id] {
			if _, fn := runnerOf(p, id); fn != nil {
				diagnose(p, id.Pos(), "E-LINT-004", "app."+fn.Name()+" is used as a value: call it directly, with an app.Options literal")
			}
		}
		return true
	})
}

// runnerOf returns the name in x, an identifier or a selector, and the function of runners it
// names, or nil.
func runnerOf(p *analysis.Pass, x ast.Expr) (*ast.Ident, *types.Func) {
	var id *ast.Ident
	switch x := x.(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id = x.Sel
	default:
		return nil, nil
	}
	fn, ok := p.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != Framework+"/app" || fn.Signature().Recv() != nil {
		return nil, nil
	}
	if _, ok := runners[fn.Name()]; !ok {
		return nil, nil
	}
	return id, fn
}

// optionsLiteral reports whether x is a composite literal of the type options.
func optionsLiteral(p *analysis.Pass, x ast.Expr, options types.Type) bool {
	lit, ok := ast.Unparen(x).(*ast.CompositeLit)
	return ok && types.Identical(p.TypesInfo.TypeOf(lit), options)
}

// literal checks the entry fields that the composite literal lit sets: a keyed field in any
// literal, and a field by its place in a literal of a struct type. A literal whose type is a type
// parameter or a pointer to one is refused outright.
func (a *app) literal(p *analysis.Pass, lit *ast.CompositeLit) {
	t := p.TypesInfo.TypeOf(lit)
	st := structOf(t)
	for i, elt := range lit.Elts {
		var field *types.Var
		value := elt
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			key, _ := kv.Key.(*ast.Ident)
			field, _ = p.TypesInfo.Uses[key].(*types.Var)
			value = kv.Value
		} else if st != nil && i < st.NumFields() {
			field = st.Field(i)
		}
		if entryOfTypeParam(field) {
			diagnose(p, value.Pos(), "E-LINT-004", typeParamEntry(field))
			continue
		}
		e, ok := entryOf(field)
		if ok && !a.generatedCall(p, value, e.fn) {
			diagnose(p, value.Pos(), "E-LINT-004", field.Name()+" must be "+e.call+", the function aicoded generate writes")
		}
	}
	if ofTypeParam(t) {
		diagnose(p, lit.Pos(), "E-LINT-004", "this composite literal builds a value of a type parameter, so lint cannot tell whether it sets Handler or RPC: use the concrete type")
	}
}

// ofTypeParam reports whether t is a type parameter or a pointer to one.
func ofTypeParam(t types.Type) bool {
	if t == nil {
		return false
	}
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	_, ok := types.Unalias(t).(*types.TypeParam)
	return ok
}

// setEntry reports x, a target that targets returns, when it is an entry field, or a field with
// an entry's name whose type mentions a type parameter.
func (a *app) setEntry(p *analysis.Pass, x ast.Expr) {
	sel, ok := ast.Unparen(x).(*ast.SelectorExpr)
	if !ok {
		return
	}
	field, _ := p.TypesInfo.Uses[sel.Sel].(*types.Var)
	if entryOfTypeParam(field) {
		diagnose(p, sel.Sel.Pos(), "E-LINT-004", typeParamEntry(field))
		return
	}
	if e, ok := entryOf(field); ok {
		diagnose(p, sel.Sel.Pos(), "E-LINT-004", "set "+field.Name()+" only in the app.Options literal, to "+e.call)
	}
}

// entryOfTypeParam reports whether field has the name of an entry and a type that mentions a
// type parameter, which an instantiation could make the entry's type.
func entryOfTypeParam(field *types.Var) bool {
	if field == nil || !field.IsField() {
		return false
	}
	_, ok := entries[field.Name()]
	return ok && hasTypeParam(field.Type())
}

// typeParamEntry is the message for a field that entryOfTypeParam reports.
func typeParamEntry(field *types.Var) string {
	return field.Name() + " has a type with a type parameter, so lint cannot tell whether it is an entry point of app.Options: use the concrete type"
}

// entryOf returns the entry that field is, whatever name its type is written under.
func entryOf(field *types.Var) (entry, bool) {
	if field == nil || !field.IsField() {
		return entry{}, false
	}
	e, ok := entries[field.Name()]
	return e, ok && types.TypeString(types.Unalias(field.Type()), nil) == e.typ
}

// generatedCall reports whether x is a direct call of the function named fn that aicoded generate
// wrote in a package of the app.
func (a *app) generatedCall(p *analysis.Pass, x ast.Expr, fn string) bool {
	call, ok := ast.Unparen(x).(*ast.CallExpr)
	if !ok {
		return false
	}
	var id *ast.Ident
	switch f := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return false
	}
	obj, ok := p.TypesInfo.Uses[id].(*types.Func)
	if !ok || obj.Name() != fn || obj.Signature().Recv() != nil || obj.Pkg() == nil || !a.own[obj.Pkg().Path()] {
		return false
	}
	return a.generated[a.rel(p.Fset.PositionFor(obj.Pos(), false).Filename)]
}

// structOf returns the struct type that a composite literal of type t builds, or nil: the
// struct t is, or the struct t points to, such as each {...} in []*T{{...}}.
func structOf(t types.Type) *types.Struct {
	if t == nil {
		return nil
	}
	u := t.Underlying()
	if ptr, ok := u.(*types.Pointer); ok {
		u = ptr.Elem().Underlying()
	}
	st, _ := u.(*types.Struct)
	return st
}
