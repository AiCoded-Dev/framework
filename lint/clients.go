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

// clientNames are the package-level names of net/http that make or reach an HTTP client of the
// app's own. Their findings get outboundFix.
var clientNames = []string{
	"DefaultClient", "DefaultTransport", "Get", "Head", "NewFileTransport", "NewFileTransportFS",
	"Post", "PostForm", "ProxyFromEnvironment", "ProxyURL", "ReadResponse", "RoundTripper", "Transport",
}

// The messages for an http.Client that app code makes, and for one it holds by value.
const (
	makeClient = "app code may not make an http.Client of its own: use the *http.Client that outbound.Client() returns"
	holdClient = "app code may not hold an http.Client by value: hold the *http.Client that outbound.Client() returns"
)

// clientName reports whether name, as refused names a member, is one of clientNames.
func clientName(name string) bool {
	pkg, rest := splitMember(name)
	return pkg == "net/http" && slices.Contains(clientNames, rest)
}

// clientMember reports whether name, as refused names a member, is a field or method of
// http.Client. Their findings get asIsFix.
func clientMember(name string) bool {
	pkg, rest := splitMember(name)
	return pkg == "net/http" && strings.HasPrefix(rest, "Client.")
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
// reflection could change: through pointers, elements, map keys and exported fields. It does not
// look into functions or interfaces, whose values reflection outside the app cannot reach into
// without the reflect package, which app code cannot import.
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

// clientUses records, for a file, the names that a * stands right before, as in *http.Client,
// and the names that a composite literal or new makes a value of, as in http.Client{} and
// new(http.Client).
type clientUses struct{ starred, made map[*ast.Ident]bool }

func clientUsesOf(f *ast.File) clientUses {
	u := clientUses{starred: map[*ast.Ident]bool{}, made: map[*ast.Ident]bool{}}
	name := func(x ast.Expr) *ast.Ident {
		x = ast.Unparen(x)
		if sel, ok := x.(*ast.SelectorExpr); ok {
			x = sel.Sel
		}
		id, _ := x.(*ast.Ident)
		return id
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.StarExpr:
			if id := name(n.X); id != nil {
				u.starred[id] = true
			}
		case *ast.CompositeLit:
			if id := name(n.Type); id != nil {
				u.made[id] = true
			}
		case *ast.CallExpr:
			if fn, ok := ast.Unparen(n.Fun).(*ast.Ident); ok && fn.Name == "new" && len(n.Args) == 1 {
				if id := name(n.Args[0]); id != nil {
					u.made[id] = true
				}
			}
		}
		return true
	})
	return u
}

// clients reports, with E-LINT-009, the HTTP clients that app code makes or holds for itself in
// n: a use of the type http.Client that no * stands before, as in http.Client{},
// new(http.Client) or a variable, field or type of type http.Client; a composite literal of
// http.Client whose type is left out, as in []*http.Client{{}}; and an instance of a generic
// function or type with a type argument that holds an http.Client, which could make one, as
// new(T) does.
func (a *app) clients(p *analysis.Pass, n ast.Node, uses clientUses) {
	switch n := n.(type) {
	case *ast.Ident:
		if tn, ok := p.TypesInfo.Uses[n].(*types.TypeName); ok && isClient(tn.Type()) && !uses.starred[n] {
			if uses.made[n] {
				diagnoseFix(p, n.Pos(), "E-LINT-009", makeClient, outboundFix)
			} else {
				diagnoseFix(p, n.Pos(), "E-LINT-009", holdClient, asIsFix)
			}
		}
		if inst, ok := p.TypesInfo.Instances[n]; ok {
			for t := range inst.TypeArgs.Types() {
				if holdsClient(t) {
					diagnoseFix(p, n.Pos(), "E-LINT-009", "app code may not instantiate "+n.Name+" with a type that holds an http.Client", asIsFix)
					break
				}
			}
		}
	case *ast.CompositeLit:
		if t := p.TypesInfo.TypeOf(n); n.Type == nil && (isClient(t) || isClient(pointee(t))) {
			diagnoseFix(p, n.Lbrace, "E-LINT-009", makeClient, outboundFix)
		}
	}
}

// clientInterfaces reports, with E-LINT-009, every conversion in the app's code of a value that
// holdsClient into an interface value, once per position: of conversions that share one, such
// as two in one call, it reports the first. Code that takes an interface value, such as
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
				if seen[pos] {
					continue
				}
				seen[pos] = true
				msg := "app code may not turn a " + types.TypeString(mi.X.Type(), qualifier) + " into an interface value"
				if !exact {
					msg += " in " + mi.Parent().Name()
				}
				if isClient(mi.X.Type()) || isClient(pointee(mi.X.Type())) {
					msg += ", through which other code could change the client"
				} else {
					msg += ", through which other code could change the client it holds"
				}
				file, line, column := a.position(prog.Fset, pos)
				a.found = append(a.found, finding{file: file, line: line, column: column, code: "E-LINT-009", fix: asIsFix, message: msg})
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
