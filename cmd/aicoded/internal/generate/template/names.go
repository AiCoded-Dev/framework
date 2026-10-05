package template

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

// reserved are the names that the generated code declares or imports where a template's code
// runs. A template that named them could write to the page past the escapers.
var reserved = map[string]bool{
	"w": true, "s": true, "render": true, "web": true, "form": true, "reactive": true,
	"io": true, "context": true, "http": true, "json": true, "strings": true, "sync": true,
	"errors": true, "slog": true,
}

// generated are the package-level names that the generated files of a page's package declare,
// besides the names generatedName matches by their form. Only the generated code uses them.
var generated = map[string]bool{
	"NewHandler": true, "NewRoute": true, "ReactiveState": true, "RouteData": true, "RouteDataProvider": true,
	"assets": true, "assetsDir": true, "route": true, "routeKey": true, "state": true,
}

// generatedName reports whether name is a package-level name of the generated files of a page's
// package: one of generated, a name starting with _html or renderBlock_, or the Form<Name>Values
// type of a form.
func generatedName(name string) bool {
	if generated[name] || strings.HasPrefix(name, "_html") || strings.HasPrefix(name, "renderBlock_") {
		return true
	}
	rest, form := strings.CutPrefix(name, "Form")
	rest, values := strings.CutSuffix(rest, "Values")
	r, _ := utf8.DecodeRuneInString(rest)
	return form && values && unicode.IsUpper(r)
}

func isReserved(name string) bool { return reserved[name] || generatedName(name) }

// callLocals are the names the generated Call method and the data provider stub of a call
// declare where they name the call's in and out types.
var callLocals = map[string]bool{"s": true, "p": true, "r": true, "ctx": true, "in": true, "name": true, "args": true}

// varLocals are the names the generated HandleWrite declares where it names the type of an
// <ssr:var>.
var varLocals = map[string]bool{"s": true, "r": true, "ctx": true, "conn": true, "msg": true}

// checkNames refuses a reserved name in the expressions of nodes. Inside <ssr:form>, including
// its own attributes, form is the form's values and may be used.
func (p *parser) checkNames(nodes ...node.Node) error {
	for _, n := range nodes {
		if err := p.checkName(n); err != nil {
			return err
		}
	}
	return nil
}

func (p *parser) checkName(n node.Node) error {
	switch v := n.(type) {
	case *node.Text, *node.String, *node.Number:
		return nil
	case *node.Variable:
		if isReserved(v.Name) && (v.Name != "form" || p.form == nil && !p.formAttrs) {
			return p.reservedName(v.Line, v.Name)
		}
		return nil
	case *node.Loop:
		for _, name := range []string{v.Index, v.Variable} {
			if isReserved(name) {
				return p.reservedName(v.Line, name)
			}
		}
		return p.checkNames(v.Array)
	case *node.Content:
		return p.checkNames(v.Children...)
	case *node.Expression:
		return p.checkNames(v.Value)
	case *node.RawExpression:
		return p.checkNames(v.Value)
	case *node.Parentheses:
		return p.checkNames(v.Value)
	case *node.StructField:
		return p.checkNames(v.Expr)
	case *node.Operator:
		if v.Left != nil {
			if err := p.checkNames(v.Left); err != nil {
				return err
			}
		}
		return p.checkNames(v.Right)
	case *node.Indexed:
		return p.checkNames(v.Expr, v.Index)
	case *node.Function:
		if err := p.checkNames(v.Expr); err != nil {
			return err
		}
		return p.checkNames(v.Arguments.Values...)
	case *node.TernaryIf:
		return p.checkNames(v.Cond, v.T, v.F)
	}
	panic(fmt.Sprintf("template: checkName: unexpected node %T", n))
}

func (p *parser) reservedName(line int, name string) error {
	return errAt(p.display, line, "E-GEN-042", fixes["E-GEN-042"], "%s is a name the generated code uses", name)
}
