package generate

import (
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

// chain returns the routes that render the page at p, root first: every layout on the path to
// it and the route itself. A route on the path without <ssr:content/> is a gate and renders
// only as its own page. p must be a key of routes.
func chain(p string, routes map[string]*Route) []*Route {
	var out []*Route
	segs := segments(p)
	for i := range segs {
		if r := routes["/"+strings.Join(segs[:i], "/")]; r != nil && r.Template.GetContentNode() != nil {
			out = append(out, r)
		}
	}
	return append(out, routes[p])
}

// checkAssets refuses script and style tags that would never be written. <ssr:assets/> writes
// the tags of its own route and of the routes rendered inside it, so a route's tags need
// <ssr:assets/> in that route or in a layout above it: the routes of chain. The layouts above
// a route are the same on every page it renders on.
func (g *gen) checkAssets(routes []*Route) error {
	var diags Diagnostics
	for _, r := range routes {
		if len(g.assets.tags[r.Path]) == 0 ||
			slices.ContainsFunc(chain(r.Path, g.routes), func(x *Route) bool { return x.Template.HasAssets() }) {
			continue
		}
		diags = append(diags, diag(templatePos(r.Path), "E-GEN-050",
			"%s has scripts or styles that never load: neither it nor a layout above it has <ssr:assets/>", r.Path))
	}
	return diags.Err()
}

// checkDocuments refuses the pages a viewer can open that would reach the browser as a bare
// fragment.
func (g *gen) checkDocuments(routes []*Route) error {
	var diags Diagnostics
	for _, r := range routes {
		if e := withoutHTML(r, g.routes); e != nil {
			diags = append(diags, e)
		}
	}
	return diags.Err()
}

// withoutHTML refuses a page a viewer can open whose render chain has no <html> element. A
// layout with pages below it only redirects, so it is not checked on its own.
func withoutHTML(r *Route, routes map[string]*Route) *errs.Error {
	if redirects(r, routes) {
		return nil
	}
	for _, x := range chain(r.Path, routes) {
		found := false
		walk(x.Template.GetNodes(), func(n node.Node) {
			if e, ok := n.(*node.HtmlElement); ok && e.TagName == "html" {
				found = true
			}
		})
		if found {
			return nil
		}
	}
	return diag(templatePos(r.Path), "E-GEN-052", "%s renders without <html>: neither it nor a layout above it has an <html> element", r.Path)
}

// hasPagesBelow reports whether a route in routes lies below the route at p.
func hasPagesBelow(p string, routes map[string]*Route) bool {
	prefix := strings.TrimSuffix(p, "/") + "/"
	for q := range routes {
		if q != p && strings.HasPrefix(q, prefix) {
			return true
		}
	}
	return false
}
