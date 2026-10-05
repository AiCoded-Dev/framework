package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &RawExpression{}

// RawExpression is a {{$ }} value. Its value must be a web.SafeHTML.
type RawExpression struct {
	BaseNode
	Value Node
	// Source is the trimmed text between {{$ and }}.
	Source string
	// BindingKey is set for a live value; the value is then wrapped in <span data-ssr-bind="KEY">.
	BindingKey string
}

func (n *RawExpression) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	if n.BindingKey != "" {
		buf.WritePrintString(openMarker("span", n.BindingKey))
	}
	n.writeValue(buf)
	if n.BindingKey != "" {
		buf.WritePrintString("</span>")
	}
}

// WriteInnerGoCode writes the value without the live-value wrapper.
func (n *RawExpression) WriteInnerGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	n.writeValue(buf)
}

func (n *RawExpression) writeValue(buf *gobuf.GoBuf) {
	buf.WriteString("if err := render.HTML(w, ")
	n.Value.WriteGoCode(buf)
	buf.WriteStringLn("); err != nil {\nreturn err\n}")
}

func (n *RawExpression) CollectVarRefs(reactive map[string]bool) []string {
	return n.Value.CollectVarRefs(reactive)
}
