package lint

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// hostPrefix starts the name of every cookie app code may set or read (E-LINT-012).
const hostPrefix = "__Host-"

// The messages of E-LINT-012.
const (
	cookieName    = "a cookie that app code sets must have a Name that is a constant starting with " + hostPrefix
	cookiePath    = `a cookie that app code sets must have Path: "/"`
	cookieSecure  = "a cookie that app code sets must have Secure: true"
	cookieDomain  = "a cookie that app code sets may not have a Domain"
	cookieZero    = `make a cookie with an http.Cookie literal that sets its Name, Path: "/" and Secure: true, and no Domain, not as an empty value`
	cookieAssign  = "set the Name, Path, Secure and Domain of a cookie only with = and a constant for each field"
	cookieAddress = "app code may not take the address of the Name, Path, Secure or Domain of a cookie"
	cookieHeader  = "the value of a Set-Cookie header must be the String() of an http.Cookie, or a constant " + hostPrefix + " cookie with Path=/, Secure and no Domain"
	cookieMap     = `set a cookie with w.Header().Add("Set-Cookie", c.String()), not by its key in a header map`
	cookieRead    = "app code reads only cookies whose name starts with " + hostPrefix + ", which no other app can set"
)

// The methods of net/http that set a header or read a cookie, each written Type.Method.
var (
	headerSetters = []string{"Header.Add", "Header.Set"}
	cookieReaders = []string{"Request.Cookie", "Request.CookiesNamed"}
)

// httpTypes are the types of net/http that the cookie rule reads: nil when no package of the
// build imports net/http, so that no code of the app can make a cookie or set a header.
type httpTypes struct {
	cookie *types.Struct
	header types.Type
}

// httpTypesOf returns the httpTypes of pkgs.
func httpTypesOf(pkgs []*packages.Package) *httpTypes {
	var out *httpTypes
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.PkgPath != "net/http" || p.Types == nil {
			return
		}
		cookie, _ := p.Types.Scope().Lookup("Cookie").(*types.TypeName)
		header, _ := p.Types.Scope().Lookup("Header").(*types.TypeName)
		if cookie != nil && header != nil {
			st, _ := cookie.Type().Underlying().(*types.Struct)
			out = &httpTypes{cookie: st, header: header.Type().Underlying()}
		}
	})
	return out
}

// cookies reports, with E-LINT-012, the cookies app code makes, sets or reads that are not
// __Host- cookies with Path=/, Secure and without Domain:
//   - a composite literal of http.Cookie, or of any type with its fields, whose Name is not a
//     constant starting with __Host-, whose Path is not the constant "/", whose Secure is not the
//     constant true, or which sets a Domain other than the constant "";
//   - a change of the Name, Path, Secure or Domain of a cookie to a value the literal could not
//     hold, or by anything but a plain assignment, and the address of one of these fields;
//   - an empty cookie made with new or a var declaration without a value;
//   - a call of Add or Set of http.Header whose key is the constant Set-Cookie, in any case, and
//     whose value is neither the String() of a cookie nor a constant __Host- cookie with Path=/,
//     Secure and no Domain;
//   - a Set-Cookie key written into a header map, by index or in a composite literal;
//   - a call of Cookie or CookiesNamed of http.Request with a constant name that does not start
//     with __Host-;
//   - Add and Set of http.Header, and Cookie and CookiesNamed of http.Request, used as values.
//
// http.SetCookie is E-LINT-009. A header key or a cookie name that is not a constant is not read.
func (a *app) cookies(p *analysis.Pass) {
	h := a.http
	if h == nil {
		return
	}
	for _, f := range p.Files {
		called := map[ast.Expr]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CompositeLit:
				h.literal(p, n)
			case *ast.AssignStmt:
				h.assign(p, n)
			case *ast.CallExpr:
				called[ast.Unparen(n.Fun)] = true
				h.call(p, n)
			case *ast.SelectorExpr:
				if !called[n] {
					methodValue(p, n)
				}
			case *ast.ValueSpec:
				h.zeroVar(p, n)
			}
			for _, x := range targets(n) {
				h.target(p, n, x)
			}
			return true
		})
	}
}

// isCookie reports whether st is the struct of http.Cookie.
func (h *httpTypes) isCookie(st *types.Struct) bool {
	return st != nil && types.Identical(st, h.cookie)
}

// isHeader reports whether t is a header map: a type whose underlying type is that of
// http.Header.
func (h *httpTypes) isHeader(t types.Type) bool {
	return t != nil && types.Identical(t.Underlying(), h.header)
}

// literal checks lit when it builds a cookie or a header map.
func (h *httpTypes) literal(p *analysis.Pass, lit *ast.CompositeLit) {
	t := p.TypesInfo.TypeOf(lit)
	if h.isHeader(t) {
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok && setCookieKey(p, kv.Key) {
				diagnose(p, kv.Key.Pos(), "E-LINT-012", cookieMap)
			}
		}
		return
	}
	st := structOf(t)
	if !h.isCookie(st) {
		return
	}
	fields := map[string]ast.Expr{}
	for i, elt := range lit.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok {
				fields[key.Name] = kv.Value
			}
		} else if i < st.NumFields() {
			fields[st.Field(i).Name()] = elt
		}
	}
	for _, field := range []string{"Name", "Path", "Secure", "Domain"} {
		value := fields[field]
		if msg := cookieField(p, field, value); msg != "" {
			pos := lit.Pos()
			if value != nil {
				pos = value.Pos()
			}
			diagnose(p, pos, "E-LINT-012", msg)
		}
	}
}

// cookieField returns the message for value, the value of the field of a cookie, when the rule
// refuses it, or "". A nil value is a field the literal leaves out.
func cookieField(p *analysis.Pass, field string, value ast.Expr) string {
	var c constant.Value
	if value != nil {
		c = p.TypesInfo.Types[value].Value
	}
	str := func() (string, bool) {
		if c == nil || c.Kind() != constant.String {
			return "", false
		}
		return constant.StringVal(c), true
	}
	switch field {
	case "Name":
		if s, ok := str(); !ok || !strings.HasPrefix(s, hostPrefix) {
			return cookieName
		}
	case "Path":
		if s, ok := str(); !ok || s != "/" {
			return cookiePath
		}
	case "Secure":
		if c == nil || c.Kind() != constant.Bool || !constant.BoolVal(c) {
			return cookieSecure
		}
	case "Domain":
		if s, ok := str(); value != nil && (!ok || s != "") {
			return cookieDomain
		}
	}
	return ""
}

// cookieFieldOf returns the name of the field of a cookie that x, a selector, names when it is
// Name, Path, Secure or Domain, or "".
func (h *httpTypes) cookieFieldOf(p *analysis.Pass, x ast.Expr) string {
	sel, ok := ast.Unparen(x).(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	s := p.TypesInfo.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return ""
	}
	switch name := sel.Sel.Name; name {
	case "Name", "Path", "Secure", "Domain":
		if h.isCookie(owner(s)) {
			return name
		}
	}
	return ""
}

// owner returns the struct that holds the field that s selects, through embedded fields.
func owner(s *types.Selection) *types.Struct {
	t := s.Recv()
	path := s.Index()
	for i, j := range path {
		st := structOf(t)
		if st == nil || i == len(path)-1 {
			return st
		}
		t = st.Field(j).Type()
	}
	return nil
}

// assign checks each field of a cookie that the plain assignment n sets to a value of its own.
func (h *httpTypes) assign(p *analysis.Pass, n *ast.AssignStmt) {
	if n.Tok != token.ASSIGN || len(n.Lhs) != len(n.Rhs) {
		return
	}
	for i, x := range n.Lhs {
		if field := h.cookieFieldOf(p, x); field != "" {
			if msg := cookieField(p, field, n.Rhs[i]); msg != "" {
				diagnose(p, n.Rhs[i].Pos(), "E-LINT-012", msg)
			}
		}
	}
}

// target reports x, a target that targets returns of n, when it is a field of a cookie that n
// changes otherwise than by a plain assignment that assign checks, or a Set-Cookie key of a
// header map.
func (h *httpTypes) target(p *analysis.Pass, n ast.Node, x ast.Expr) {
	if h.headerKey(p, x) {
		diagnose(p, x.Pos(), "E-LINT-012", cookieMap)
		return
	}
	if h.cookieFieldOf(p, x) == "" {
		return
	}
	switch n := n.(type) {
	case *ast.AssignStmt:
		if n.Tok != token.ASSIGN || len(n.Lhs) != len(n.Rhs) {
			diagnose(p, x.Pos(), "E-LINT-012", cookieAssign)
		}
	case *ast.UnaryExpr:
		diagnose(p, x.Pos(), "E-LINT-012", cookieAddress)
	default:
		diagnose(p, x.Pos(), "E-LINT-012", cookieAssign)
	}
}

// headerKey reports whether x indexes a header map, directly or through further indexes, with the
// constant key Set-Cookie.
func (h *httpTypes) headerKey(p *analysis.Pass, x ast.Expr) bool {
	for {
		ix, ok := ast.Unparen(x).(*ast.IndexExpr)
		if !ok {
			return false
		}
		if h.isHeader(p.TypesInfo.TypeOf(ix.X)) && setCookieKey(p, ix.Index) {
			return true
		}
		x = ix.X
	}
}

// setCookieKey reports whether x is a constant that names the Set-Cookie header.
func setCookieKey(p *analysis.Pass, x ast.Expr) bool {
	c := p.TypesInfo.Types[x].Value
	return c != nil && c.Kind() == constant.String && strings.EqualFold(constant.StringVal(c), "Set-Cookie")
}

// call checks new of a cookie, a call of Add or Set of http.Header with the key Set-Cookie, and a
// call of Cookie or CookiesNamed of http.Request.
func (h *httpTypes) call(p *analysis.Pass, call *ast.CallExpr) {
	if id, ok := ast.Unparen(call.Fun).(*ast.Ident); ok && len(call.Args) == 1 {
		if b, ok := p.TypesInfo.Uses[id].(*types.Builtin); ok && b.Name() == "new" && h.isCookie(structOf(p.TypesInfo.TypeOf(call.Args[0]))) {
			diagnose(p, call.Pos(), "E-LINT-012", cookieZero)
		}
		return
	}
	sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return
	}
	args := call.Args
	if s := p.TypesInfo.Selections[sel]; s != nil && s.Kind() == types.MethodExpr && len(args) > 0 {
		args = args[1:]
	}
	switch {
	case httpMethod(p, sel, headerSetters...):
		if len(args) == 2 && setCookieKey(p, args[0]) && !h.cookieValue(p, args[1]) {
			diagnose(p, args[1].Pos(), "E-LINT-012", cookieHeader)
		}
	case httpMethod(p, sel, cookieReaders...):
		if len(args) != 1 {
			return
		}
		if c := p.TypesInfo.Types[args[0]].Value; c != nil && c.Kind() == constant.String && !strings.HasPrefix(constant.StringVal(c), hostPrefix) {
			diagnose(p, args[0].Pos(), "E-LINT-012", cookieRead)
		}
	}
}

// methodValue reports sel, which is not called, when it is a method of headerSetters or
// cookieReaders: such a value could set a header or read a cookie that lint cannot see.
func methodValue(p *analysis.Pass, sel *ast.SelectorExpr) {
	if httpMethod(p, sel, slices.Concat(headerSetters, cookieReaders)...) {
		fn := p.TypesInfo.Uses[sel.Sel].(*types.Func)
		diagnose(p, sel.Pos(), "E-LINT-012", display(member(fn))+" is used as a value: call it directly, so that lint reads the cookie it sets or reads")
	}
}

// cookieValue reports whether x may be the value of a Set-Cookie header: a call of the String
// method of http.Cookie, whose cookie the rule checks where it is made, or a constant that is a
// __Host- cookie with Path=/, Secure and no Domain.
func (h *httpTypes) cookieValue(p *analysis.Pass, x ast.Expr) bool {
	if call, ok := ast.Unparen(x).(*ast.CallExpr); ok {
		sel, ok := ast.Unparen(call.Fun).(*ast.SelectorExpr)
		return ok && httpMethod(p, sel, "Cookie.String")
	}
	c := p.TypesInfo.Types[x].Value
	if c == nil || c.Kind() != constant.String {
		return false
	}
	cookie, err := http.ParseSetCookie(constant.StringVal(c))
	return err == nil && strings.HasPrefix(cookie.Name, hostPrefix) && cookie.Path == "/" && cookie.Secure && cookie.Domain == ""
}

// httpMethod reports whether sel names one of the methods of net/http, each written Type.Method.
func httpMethod(p *analysis.Pass, sel *ast.SelectorExpr, methods ...string) bool {
	fn, ok := p.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "net/http" {
		return false
	}
	return slices.Contains(methods, strings.TrimPrefix(member(fn), "net/http."))
}

// zeroVar reports a var declaration of a cookie without a value.
func (h *httpTypes) zeroVar(p *analysis.Pass, spec *ast.ValueSpec) {
	if spec.Type == nil || len(spec.Values) > 0 {
		return
	}
	t := p.TypesInfo.TypeOf(spec.Type)
	if t == nil {
		return
	}
	if st, ok := t.Underlying().(*types.Struct); ok && h.isCookie(st) {
		diagnose(p, spec.Pos(), "E-LINT-012", cookieZero)
	}
}
