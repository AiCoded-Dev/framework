package generate

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

// assetsCode is pages/assets_gen.go for an app without assets.
const assetsCode = header + "package pages\n\nimport \"io/fs\"\n\nvar assets fs.FS\n"

// embedCode is pages/assets_gen.go for an app with assets: it embeds pages/assets_gen.
const embedCode = header + `package pages

import (
	"embed"
	"io/fs"
)

//go:embed all:assets_gen
var assetsDir embed.FS

var assets, _ = fs.Sub(assetsDir, "assets_gen")
`

// handler returns pages/handler_gen.go, which serves every route with web.New.
func (g *gen) handler(routes []*Route) ([]byte, error) {
	aliases := routeAliases(routes)
	var imports []string
	if g.app.Deps {
		imports = append(imports, strconv.Quote(g.depsPath()))
	}
	for _, r := range routes {
		if r.Path != "/" {
			imports = append(imports, aliases[r.Path]+" "+strconv.Quote(path.Join(g.app.Module, folder(r.Path))))
		}
	}
	param, arg := "", ""
	if g.app.Deps {
		param, arg = "d *deps.Deps", "d"
	}

	b := gobuf.New()
	b.WriteString(header)
	b.WriteStringLn("package pages")
	b.WriteStringLn("")
	b.WriteStringLn("import (")
	b.WriteQuotedString("net/http", "\n\n")
	b.WriteQuotedString(webPkg, "\n")
	if len(imports) > 0 {
		b.WriteStringLn("")
		for _, imp := range imports {
			b.WriteStringLn(imp)
		}
	}
	b.WriteStringLn(")")
	b.WriteStringLn("")
	b.WriteStringLn("// NewHandler returns the handler for the app's pages.")
	b.WriteStringLn("func NewHandler(" + param + ") http.Handler {")
	b.WriteStringLn("return web.New(map[string]web.Route{")
	for _, r := range routes {
		pkg := ""
		if r.Path != "/" {
			pkg = aliases[r.Path] + "."
		}
		b.WriteQuotedString(r.Path, ": "+pkg+"NewRoute("+pkg+"NewDP("+arg+")),\n")
	}
	b.WriteStringLn("}, web.Options{Assets: assets})")
	b.WriteStringLn("}")

	code, err := b.Formatted()
	if err != nil {
		return nil, fmt.Errorf("generate pages/handler_gen.go: %w", err)
	}
	return code, nil
}

// routeAliases returns the import names of the route packages: route followed by the route's
// folders, each with its first letter in upper case, and a number when that name is taken.
func routeAliases(routes []*Route) map[string]string {
	used := map[string]bool{"routeKey": true} // declared by the route_gen.go of pages/
	out := map[string]string{}
	for _, r := range routes {
		if r.Path == "/" {
			continue
		}
		base := "route"
		for _, seg := range segments(r.Path) {
			base += exported(seg)
		}
		alias := base
		for n := 2; used[alias]; n++ {
			alias = base + strconv.Itoa(n)
		}
		used[alias] = true
		out[r.Path] = alias
	}
	return out
}

// stub returns the dataprovider.go of a route folder that has none. It implements every hook,
// so the app builds, and fails closed.
func (g *gen) stub(r *Route) ([]byte, error) {
	b := gobuf.New()
	b.WriteStringLn("package " + path.Base(folder(r.Path)))
	b.WriteStringLn("")
	b.WriteStringLn("import (")
	b.WriteQuotedString("context", "\n")
	if len(writableVars(r.Template.GetVariables())) > 0 {
		b.WriteQuotedString("errors", "\n")
	}
	b.WriteStringLn("")
	b.WriteQuotedString(webPkg, "\n")
	if g.app.Deps {
		b.WriteStringLn("")
		b.WriteQuotedString(g.depsPath(), "\n")
	}
	b.WriteStringLn(")")
	b.WriteStringLn("")
	b.WriteStringLn("var _ RouteDataProvider = &DP{}")
	b.WriteStringLn("")
	b.WriteStringLn("// DP provides the data of this page.")
	if g.app.Deps {
		b.WriteStringLn("type DP struct {")
		b.WriteStringLn("d *deps.Deps")
		b.WriteStringLn("}")
		b.WriteStringLn("")
		b.WriteStringLn("// NewDP returns the page's data provider.")
		b.WriteStringLn("func NewDP(d *deps.Deps) *DP { return &DP{d: d} }")
	} else {
		b.WriteStringLn("type DP struct{}")
		b.WriteStringLn("")
		b.WriteStringLn("// NewDP returns the page's data provider.")
		b.WriteStringLn("func NewDP() *DP { return &DP{} }")
	}
	for _, h := range hooks(r.Template) {
		b.WriteStringLn("")
		for line := range strings.SplitSeq(h.doc, "\n") {
			b.WriteStringLn("// " + line)
		}
		b.WriteStringLn("func (p *DP) " + h.sig + " {")
		b.WriteStringLn(h.stub)
		b.WriteStringLn("}")
	}

	code, err := b.Formatted()
	if err != nil {
		return nil, fmt.Errorf("generate %s/dataprovider.go: %w", folder(r.Path), err)
	}
	return code, nil
}

// depsPath returns the import path of the app's deps package.
func (g *gen) depsPath() string { return path.Join(g.app.Module, "deps") }
