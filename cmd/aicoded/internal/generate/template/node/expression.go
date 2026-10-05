package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Expression{}

// Expression is a {{ }} value, written through the escaper Esc.
type Expression struct {
	BaseNode
	Value Node
	Esc   Esc
	// RCDATA is set for the text of <title> and <textarea>, which cannot hold a live value.
	RCDATA bool
	// Source is the trimmed text between {{ and }}.
	Source string
	// BindingKey is set for a live value; the value is then wrapped in <span data-ssr-bind="KEY">.
	BindingKey string
}

func (n *Expression) WriteGoCode(buf *gobuf.GoBuf) {
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
func (n *Expression) WriteInnerGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	n.writeValue(buf)
}

func (n *Expression) writeValue(buf *gobuf.GoBuf) {
	buf.WriteString("if err := " + n.Esc.Func() + "(w, ")
	n.Value.WriteGoCode(buf)
	buf.WriteStringLn("); err != nil {\nreturn err\n}")
}

func (n *Expression) CollectVarRefs(reactive map[string]bool) []string {
	return n.Value.CollectVarRefs(reactive)
}
