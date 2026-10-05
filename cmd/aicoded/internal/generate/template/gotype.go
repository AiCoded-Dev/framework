package template

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
)

// goType returns t as Go source when it is a type the generated code can name: a type
// declared in the page's folder, a predeclared one or an exported type of package web, which
// the generated code imports, possibly with type arguments, or a pointer, slice, array, map or
// func of such types. Otherwise it returns what is wrong, because the type is written into
// generated code.
func goType(t string) (typ, problem string) {
	return typeSource(t, "; use a type of package web, such as web.SafeHTML, or declare a named type in a .go file next to the template", true)
}

// callGoType returns the in or out type t of an <ssr:call> as Go source, as goType does, but
// never a type of another package; t may also be struct{}: a call that takes or returns nothing.
func callGoType(t string) (typ, problem string) {
	if e, err := goparser.ParseExpr(t); err == nil {
		if s, ok := e.(*ast.StructType); ok && len(s.Fields.List) == 0 {
			return "struct{}", ""
		}
	}
	return typeSource(t, "; declare a named type in a .go file next to the template", false)
}

// typeSource returns t as Go source when it is a type the generated code can name, with web
// telling whether it may name exported types of package web. hint ends the message that says
// t names another package.
func typeSource(t, hint string, web bool) (typ, problem string) {
	e, err := goparser.ParseExpr(t)
	if err != nil || !isType(e) {
		return "", fmt.Sprintf("%q is not a type name or a pointer, slice, array, map or func of type names", t)
	}
	ast.Inspect(e, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || problem != "" {
			return problem == ""
		}
		switch pkg, _ := sel.X.(*ast.Ident); {
		case !web || pkg == nil || pkg.Name != "web":
			problem = fmt.Sprintf("%q names a type from another package, which the generated code cannot import%s", t, hint)
		case !sel.Sel.IsExported():
			problem = fmt.Sprintf("%q names web.%s, which package web does not export", t, sel.Sel.Name)
		}
		return false
	})
	if problem != "" {
		return "", problem
	}
	return types.ExprString(e), ""
}

// notJSON reports whether the Go type t is a func or a chan, or a pointer to one, which
// encoding/json cannot write or read.
func notJSON(t string) bool {
	e, err := goparser.ParseExpr(t)
	for err == nil {
		switch x := e.(type) {
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.FuncType, *ast.ChanType:
			return true
		default:
			return false
		}
	}
	return false
}

// usedName returns the first name that the Go type t refers to, leaving out the parameter names
// of func types, that names holds or that generatedName matches, or "".
func usedName(t string, names map[string]bool) string {
	e, err := goparser.ParseExpr(t)
	if err != nil {
		return ""
	}
	found := ""
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			ast.Inspect(n.Type, visit)
			return false
		case *ast.Ident:
			if found == "" && (names[n.Name] || generatedName(n.Name)) {
				found = n.Name
			}
		}
		return found == ""
	}
	ast.Inspect(e, visit)
	return found
}

func isType(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name != "_"
	case *ast.SelectorExpr:
		_, ok := e.X.(*ast.Ident)
		return ok
	case *ast.StarExpr:
		return isType(e.X)
	case *ast.ArrayType:
		if n, ok := e.Len.(*ast.BasicLit); e.Len != nil && (!ok || n.Kind != token.INT) {
			return false
		}
		return isType(e.Elt)
	case *ast.MapType:
		return isType(e.Key) && isType(e.Value)
	case *ast.IndexExpr:
		return isType(e.X) && isType(e.Index)
	case *ast.IndexListExpr:
		for _, i := range e.Indices {
			if !isType(i) {
				return false
			}
		}
		return isType(e.X)
	case *ast.FuncType:
		return e.TypeParams == nil && fieldTypes(e.Params) && fieldTypes(e.Results)
	}
	return false
}

// fieldTypes reports whether every parameter or result in fields has a type isType accepts,
// the last parameter possibly variadic.
func fieldTypes(fields *ast.FieldList) bool {
	if fields == nil {
		return true
	}
	for _, f := range fields.List {
		t := f.Type
		if v, ok := t.(*ast.Ellipsis); ok {
			t = v.Elt
		}
		if !isType(t) {
			return false
		}
	}
	return true
}
