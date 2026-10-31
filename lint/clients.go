package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// outboundNames are the names of net/http that app code may use to hold and use the client that
// outbound.Client returns. A declared module may not use them.
var outboundNames = []string{"Client", "NewRequest", "NewRequestWithContext", "NoBody", "Response"}

// clientNames are the names of net/http, besides the members of Client, that make or reach an
// HTTP client. Their findings get outboundFix.
var clientNames = []string{
	"Client", "DefaultClient", "DefaultTransport", "Get", "Head", "NewFileTransport", "NewFileTransportFS",
	"Post", "PostForm", "ProxyFromEnvironment", "ProxyURL", "ReadResponse", "RoundTripper", "Transport",
}

// ownClient is the message for an http.Client that app code makes or holds by value.
const ownClient = "app code may not make or hold an http.Client of its own: use the *http.Client that outbound.Client() returns"

// clientName reports whether name, as refused names a member, makes or reaches an HTTP client.
func clientName(name string) bool {
	pkg, rest := splitMember(name)
	return pkg == "net/http" && (slices.Contains(clientNames, rest) || strings.HasPrefix(rest, "Client."))
}

// clientField returns the member name of obj when it is a field of http.Client, or "".
func clientField(obj types.Object) string {
	v, ok := obj.(*types.Var)
	if !ok || !v.IsField() || v.Pkg() == nil || v.Pkg().Path() != "net/http" {
		return ""
	}
	tn, _ := v.Pkg().Scope().Lookup("Client").(*types.TypeName)
	if tn == nil {
		return ""
	}
	st, ok := tn.Type().Underlying().(*types.Struct)
	for i := 0; ok && i < st.NumFields(); i++ {
		if st.Field(i) == v {
			return "net/http.Client." + v.Name()
		}
	}
	return ""
}

// isClient reports whether t is http.Client.
func isClient(t types.Type) bool {
	if t == nil {
		return false
	}
	named, ok := types.Unalias(t).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "net/http" && named.Obj().Name() == "Client"
}

// holdsClient reports whether a value of type t is, points to or holds an http.Client that
// reflection could change: through pointers, elements, map keys and exported fields.
func holdsClient(t types.Type) bool {
	return reaches(t, map[types.Type]bool{})
}

func reaches(t types.Type, seen map[types.Type]bool) bool {
	t = types.Unalias(t)
	if isClient(t) {
		return true
	}
	if seen[t] {
		return false
	}
	seen[t] = true
	switch t := t.(type) {
	case *types.Named:
		return reaches(t.Underlying(), seen)
	case *types.Pointer:
		return reaches(t.Elem(), seen)
	case *types.Slice:
		return reaches(t.Elem(), seen)
	case *types.Array:
		return reaches(t.Elem(), seen)
	case *types.Chan:
		return reaches(t.Elem(), seen)
	case *types.Map:
		return reaches(t.Key(), seen) || reaches(t.Elem(), seen)
	case *types.Tuple:
		for i := range t.Len() {
			if reaches(t.At(i).Type(), seen) {
				return true
			}
		}
	case *types.Struct:
		for i := range t.NumFields() {
			if t.Field(i).Exported() && reaches(t.Field(i).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// starred returns the names in f that a * stands right before, as in *http.Client.
func starred(f *ast.File) map[*ast.Ident]bool {
	out := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if s, ok := n.(*ast.StarExpr); ok {
			x := ast.Unparen(s.X)
			if sel, ok := x.(*ast.SelectorExpr); ok {
				x = sel.Sel
			}
			if id, ok := x.(*ast.Ident); ok {
				out[id] = true
			}
		}
		return true
	})
	return out
}

// clients reports, with E-LINT-009, the HTTP clients that app code makes for itself in n: a use
// of the type http.Client that no * stands before, as in http.Client{}, new(http.Client) or a
// variable, field or type of type http.Client; a composite literal of http.Client whose type is
// left out, as in []*http.Client{{}}; and an instance of a generic function or type with a type
// argument that holds an http.Client, which could make one, as new(T) does.
func (a *app) clients(p *analysis.Pass, n ast.Node, star map[*ast.Ident]bool) {
	switch n := n.(type) {
	case *ast.Ident:
		if tn, ok := p.TypesInfo.Uses[n].(*types.TypeName); ok && isClient(tn.Type()) && !star[n] {
			diagnoseFix(p, n.Pos(), "E-LINT-009", ownClient, outboundFix)
		}
		if inst, ok := p.TypesInfo.Instances[n]; ok {
			for t := range inst.TypeArgs.Types() {
				if holdsClient(t) {
					diagnoseFix(p, n.Pos(), "E-LINT-009", "app code may not instantiate "+n.Name+" with a type that holds an http.Client", outboundFix)
					break
				}
			}
		}
	case *ast.CompositeLit:
		if t := p.TypesInfo.TypeOf(n); n.Type == nil && (isClient(t) || isClient(pointee(t))) {
			diagnoseFix(p, n.Lbrace, "E-LINT-009", ownClient, outboundFix)
		}
	}
}

// clientInterfaces reports, with E-LINT-009, every conversion in the app's code of a value that
// holdsClient into an interface value. Code that takes an interface value, such as
// json.Unmarshal, could change the client through it, such as set its Transport to nil, which
// sends through the default transport and not the runner.
func (a *app) clientInterfaces(prog *ssa.Program) {
	qualifier := func(p *types.Package) string { return p.Name() }
	seen := map[token.Pos]bool{}
	for fn := range ssautil.AllFunctions(prog) {
		if pkg := packageOf(fn); pkg == nil || !a.own[pkg.Pkg.Path()] {
			continue
		}
		for _, b := range fn.Blocks {
			for _, instr := range b.Instrs {
				mi, ok := instr.(*ssa.MakeInterface)
				if !ok || !holdsClient(mi.X.Type()) {
					continue
				}
				pos, exact := conversionPos(mi)
				msg := "app code may not turn a " + types.TypeString(mi.X.Type(), qualifier) + " into an interface value"
				if !exact {
					msg += " in " + mi.Parent().Name()
				}
				if isClient(mi.X.Type()) || isClient(pointee(mi.X.Type())) {
					msg += ", through which other code could change the client"
				} else {
					msg += ", through which other code could change the client it holds"
				}
				if seen[pos] {
					continue
				}
				seen[pos] = true
				file, line, column := a.position(prog.Fset, pos)
				a.found = append(a.found, finding{file: file, line: line, column: column, code: "E-LINT-009", fix: outboundFix, message: msg})
			}
		}
	}
}

// packageOf returns the package whose source fn comes from: its own, or that of the generic
// function it instantiates or of the function it is declared in.
func packageOf(fn *ssa.Function) *ssa.Package {
	for f := fn; f != nil; f = f.Parent() {
		if o := f.Origin(); o != nil {
			f = o
		}
		if f.Pkg != nil {
			return f.Pkg
		}
	}
	return nil
}

// conversionPos returns where the conversion mi is written: its own position, or else that of
// the instruction that uses the converted value. When neither is known, as for a value assigned
// to a local variable of an interface type, it returns the position of mi's function and false.
func conversionPos(mi *ssa.MakeInterface) (token.Pos, bool) {
	if mi.Pos().IsValid() {
		return mi.Pos(), true
	}
	for _, r := range *mi.Referrers() {
		if r.Pos().IsValid() {
			return r.Pos(), true
		}
	}
	return mi.Parent().Pos(), false
}
