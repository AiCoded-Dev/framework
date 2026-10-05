package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Loop{}

// Loop is an element with ssr:for.
type Loop struct {
	BaseNode
	Index    string
	Variable string
	Array    Node
	Children []Node
	// BlockKey is set when the list or the body is live; the whole loop output is then
	// wrapped and re-rendered.
	BlockKey string
}

func (n *Loop) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	openTag, closeTag := n.wrapperTags()
	if openTag != "" {
		buf.WritePrintString(openTag)
	}
	n.writeInner(buf)
	if closeTag != "" {
		buf.WritePrintString(closeTag)
	}
}

// wrapperTags returns the wrapper of a live loop: <tbody> when the body is a <tr>, because the
// HTML parser moves an <ssr-block> out of a table, and <ssr-block> otherwise.
func (n *Loop) wrapperTags() (string, string) {
	if n.BlockKey == "" {
		return "", ""
	}
	if n.firstChildIsTR() {
		return openMarker("tbody", n.BlockKey), "</tbody>"
	}
	return openMarker("ssr-block", n.BlockKey), "</ssr-block>"
}

func (n *Loop) firstChildIsTR() bool {
	for _, c := range n.Children {
		el, ok := c.(*HtmlElement)
		if !ok {
			continue
		}
		return el.TagName == "tr"
	}
	return false
}

// WriteInnerGoCode writes the loop without the live-block wrapper.
func (n *Loop) WriteInnerGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	n.writeInner(buf)
}

func (n *Loop) writeInner(buf *gobuf.GoBuf) {
	buf.WriteString("for ")
	if n.Index != "" {
		buf.WriteString(n.Index)
	} else {
		buf.WriteString("_")
	}
	buf.WriteString(",")
	buf.WriteString(n.Variable)
	buf.WriteString(":=range ")
	n.Array.WriteGoCode(buf)
	buf.WriteStringLn("{")
	for _, child := range n.Children {
		child.WriteGoCode(buf)
	}
	buf.WriteStringLn("}")
}

// CollectVarRefs returns the reactive variables of the list and of the body.
func (n *Loop) CollectVarRefs(reactive map[string]bool) []string {
	sets := [][]string{n.Array.CollectVarRefs(reactive)}
	for _, child := range n.Children {
		sets = append(sets, child.CollectVarRefs(reactive))
	}
	return UnionRefs(sets...)
}
