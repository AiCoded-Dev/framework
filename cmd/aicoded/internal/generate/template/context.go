package template

import (
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

var urlAttrs = map[string]bool{
	"action": true, "archive": true, "background": true, "cite": true, "classid": true, "codebase": true,
	"data": true, "formaction": true, "href": true, "icon": true, "longdesc": true, "manifest": true,
	"poster": true, "profile": true, "src": true, "usemap": true, "xmlns": true,
}

// Elements whose attributes never take values: they load code or change how the page is read.
var fixedAttrElements = map[string]bool{
	"script": true, "base": true, "meta": true, "link": true, "object": true, "embed": true, "applet": true,
	"animate": true, "set": true, "animatemotion": true, "animatetransform": true,
}

var fixedAttrs = map[string]bool{"srcdoc": true, "srcset": true, "imagesrcset": true, "http-equiv": true}

// animationElements set an attribute of their parent, such as href, to their own values.
var animationElements = map[string]bool{"animate": true, "set": true, "animatemotion": true, "animatetransform": true}

// rawTextElements have text the browser does not decode, so no escaper is safe inside them.
var rawTextElements = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true, "xmp": true,
}

// rcdataElements have text the browser decodes but never parses as markup.
var rcdataElements = map[string]bool{"title": true, "textarea": true}

// rawTags are the elements whose text the tokenizer reads up to their end tag, even after a
// start tag that ends with "/>".
var rawTags = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true,
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
}

const urlSpace = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f" +
	"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x20"

const htmlSpace = " \t\n\f\r"

func localName(name string) string {
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func isURLAttr(name string) bool {
	n := strings.TrimPrefix(localName(name), "data-")
	return urlAttrs[n] || strings.Contains(n, "src") || strings.Contains(n, "uri") || strings.Contains(n, "url")
}

// dropTabs removes tabs and newlines, which browsers drop inside URLs.
func dropTabs(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

// badScheme reports whether a URL starting with scheme: runs code.
func badScheme(scheme string) bool {
	s := strings.ToLower(dropTabs(strings.Trim(scheme, urlSpace)))
	return s == "javascript" || s == "vbscript"
}

// scriptURL reports whether the literal URL v runs code.
func scriptURL(v string) bool {
	v = strings.TrimLeft(v, urlSpace)
	i := strings.IndexAny(v, ":/?#")
	return i >= 0 && v[i] == ':' && badScheme(v[:i])
}

// attr parses the value of attribute a of an element that renders as <tag>. It refuses inline
// code and unsafe attribute names, and sets the escaper of every value.
func (p *parser) attr(tag string, a tagAttr) ([]node.Node, error) {
	name := localName(a.name)
	switch {
	case strings.Contains(a.name, "{{") || strings.Contains(a.name, "}}"):
		return nil, p.fail("E-GEN-025", "<%s> has a value in the attribute name %s", tag, a.name)
	case strings.HasPrefix(name, "on"):
		return nil, p.fail("E-GEN-022", "<%s> has the event handler %s", tag, a.name)
	case name == "style":
		return nil, p.fail("E-GEN-023", "<%s> has a style attribute", tag)
	case !a.quoted && strings.Contains(a.value, "{{"):
		return nil, p.fail("E-GEN-024", "the %s attribute of <%s> is not quoted", a.name, tag)
	}
	values, err := p.attrValue(a.value)
	if err != nil {
		return nil, err
	}
	var exprs []*node.Expression
	for _, v := range values {
		switch x := v.(type) {
		case *node.Expression:
			exprs = append(exprs, x)
		case *node.RawExpression:
			return nil, p.fail("E-GEN-029", "{{$ }} in the %s attribute of <%s>", a.name, tag)
		}
	}
	url := isURLAttr(a.name)
	if len(exprs) == 0 {
		if name == "srcdoc" {
			return nil, p.fail("E-GEN-020", "<%s> has a srcdoc attribute, which holds a whole page inline", tag)
		}
		if url && scriptURL(literal(values)) || animationElements[tag] && anyScriptURL(literal(values)) {
			return nil, p.fail("E-GEN-028", "the %s attribute of <%s> runs a script", a.name, tag)
		}
		return values, nil
	}
	switch {
	case fixedAttrElements[tag] || fixedAttrs[a.name]:
		return nil, p.fail("E-GEN-027", "the %s attribute of <%s> takes a value", a.name, tag)
	case url:
		return values, p.urlContexts(tag, a.name, values)
	}
	for _, e := range exprs {
		e.Esc = node.EscAttr
	}
	return values, nil
}

// anyScriptURL reports whether one of the ;-separated values of an animation runs code.
func anyScriptURL(v string) bool {
	for item := range strings.SplitSeq(v, ";") {
		if scriptURL(item) {
			return true
		}
	}
	return false
}

func literal(values []node.Node) string {
	var b strings.Builder
	for _, v := range values {
		if t, ok := v.(*node.Text); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// urlContexts picks the escaper of each expression in a URL attribute. A value may be the whole
// attribute; otherwise the literal text before the first value must settle the scheme. A value
// right after a lone leading "/" gets an escaper that cannot make the URL start with "//".
func (p *parser) urlContexts(tag, attr string, values []node.Node) error {
	var prefix strings.Builder
	first := -1
	for i, v := range values {
		if t, ok := v.(*node.Text); ok {
			prefix.WriteString(t.Text)
			continue
		}
		first = i
		break
	}
	lead := strings.TrimLeft(prefix.String(), urlSpace)
	if lead == "" {
		rest := values[first+1:]
		if len(rest) == 0 || len(rest) == 1 && isURLSpace(rest[0]) {
			values[first].(*node.Expression).Esc = node.EscURL
			return nil
		}
		return p.fail("E-GEN-038", "the scheme of the %s attribute of <%s> depends on a value", attr, tag)
	}
	i := strings.IndexAny(lead, ":/?#")
	if i < 0 {
		return p.fail("E-GEN-038", "the scheme of the %s attribute of <%s> depends on a value", attr, tag)
	}
	if lead[i] == ':' && badScheme(lead[:i]) {
		return p.fail("E-GEN-028", "the %s attribute of <%s> runs a script", attr, tag)
	}
	query := false
	for _, v := range values {
		switch x := v.(type) {
		case *node.Text:
			query = query || strings.ContainsAny(x.Text, "?#")
		case *node.Expression:
			x.Esc = node.EscURLPart
			if query {
				x.Esc = node.EscURLQuery
			}
		}
	}
	if dropTabs(lead) == "/" {
		values[first].(*node.Expression).Esc = node.EscURLPathStart
	}
	return nil
}

func isURLSpace(n node.Node) bool {
	t, ok := n.(*node.Text)
	return ok && strings.Trim(t.Text, urlSpace) == ""
}
