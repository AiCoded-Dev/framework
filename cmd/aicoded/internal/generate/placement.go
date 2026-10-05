package generate

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

// placement refuses <ssr:assets/>, <ssr:content/> and <html> inside ssr:if, ssr:else-if, ssr:else
// or ssr:for: the page's scripts and styles, the pages below it, or its document would show only
// sometimes.
func placement(r *Route) []*errs.Error {
	var out []*errs.Error
	file := templateFile(r.Path)
	var visit func(ns []node.Node, inside bool)
	visit = func(ns []node.Node, inside bool) {
		for _, n := range ns {
			switch v := n.(type) {
			case *node.SsrAssets:
				if inside {
					out = append(out, diag(at(file, v.Line), "E-GEN-051", "<ssr:assets/> is inside ssr:if or ssr:for, so the page's scripts and styles would load only sometimes"))
				}
			case *node.SsrContent:
				if inside {
					out = append(out, diag(at(file, v.Line), "E-GEN-051", "<ssr:content/> is inside ssr:if or ssr:for, so the pages below would show only sometimes"))
				}
			case *node.HtmlElement:
				if inside && v.TagName == "html" {
					out = append(out, diag(at(file, v.Line), "E-GEN-051", "<html> is inside ssr:if or ssr:for, so the page would sometimes reach the browser without a document"))
				}
			case *node.SsrCondition, *node.Loop:
				visit(children(n), true)
				continue
			}
			visit(children(n), inside)
		}
	}
	visit(r.Template.GetNodes(), false)
	return out
}
