package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/htmlutils"
)

var _ WithChildren = &HtmlElement{}

// HtmlAttribute is an attribute and its value as text and expressions.
type HtmlAttribute struct {
	Key    string
	Values []Node
}

// HtmlElement is an HTML element of the template.
type HtmlElement struct {
	BaseNode
	TagName    string
	Attributes []HtmlAttribute
	SelfClosed bool
	Children   []Node
	// BlockKey is set when an attribute value is live; the element is then wrapped in
	// <ssr-block data-ssr-bind="KEY"> and re-rendered whole.
	BlockKey string
}

func (n *HtmlElement) LastChild() Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

func (n *HtmlElement) PopChild() {
	if len(n.Children) > 0 {
		n.Children = n.Children[:len(n.Children)-1]
	}
}

func (n *HtmlElement) AddChildren(children ...Node) {
	n.Children = append(n.Children, children...)
}

func (n *HtmlElement) CollectVarRefs(reactive map[string]bool) []string {
	var sets [][]string
	for _, a := range n.Attributes {
		for _, v := range a.Values {
			sets = append(sets, v.CollectVarRefs(reactive))
		}
	}
	for _, c := range n.Children {
		sets = append(sets, c.CollectVarRefs(reactive))
	}
	return UnionRefs(sets...)
}

// CollectAttributeVarRefs returns the reactive variables the attribute values refer to,
// without the children.
func (n *HtmlElement) CollectAttributeVarRefs(reactive map[string]bool) []string {
	var sets [][]string
	for _, a := range n.Attributes {
		for _, v := range a.Values {
			sets = append(sets, v.CollectVarRefs(reactive))
		}
	}
	return UnionRefs(sets...)
}

func (n *HtmlElement) WriteGoCode(buf *gobuf.GoBuf) {
	if n.BlockKey != "" {
		buf.WritePrintString(openMarker("ssr-block", n.BlockKey))
	}
	n.writeInner(buf)
	if n.BlockKey != "" {
		buf.WritePrintString("</ssr-block>")
	}
}

// WriteInnerGoCode writes the element without the live-block wrapper.
func (n *HtmlElement) WriteInnerGoCode(buf *gobuf.GoBuf) {
	n.writeInner(buf)
}

func (n *HtmlElement) writeInner(buf *gobuf.GoBuf) {
	buf.WritePrintString("<" + n.TagName)
	writeAttributes(buf, n.Attributes, nil)
	if n.SelfClosed || htmlutils.VoidElements[n.TagName] {
		buf.WritePrintString(">")
		return
	}
	buf.WritePrintString(">")

	if htmlutils.LiteralElements[n.TagName] {
		for _, c := range n.Children {
			if tNode, ok := c.(*Text); ok {
				buf.WritePrintString(tNode.Text)
			} else {
				c.WriteGoCode(buf)
			}
		}
	} else {
		for _, c := range n.Children {
			c.WriteGoCode(buf)
		}
	}

	buf.WritePrintString("</" + n.TagName + ">")
}

func writeAttributes(buf *gobuf.GoBuf, attrs []HtmlAttribute, skip map[string]bool) {
	for _, a := range attrs {
		if skip[a.Key] {
			continue
		}
		buf.WritePrintString(" ")
		buf.WritePrintString(a.Key)
		if len(a.Values) > 0 {
			buf.WritePrintString(`="`)
			for _, value := range a.Values {
				value.WriteGoCode(buf)
			}
			buf.WritePrintString(`"`)
		}
	}
}
