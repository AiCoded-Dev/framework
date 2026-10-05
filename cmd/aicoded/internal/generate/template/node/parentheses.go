package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Parentheses{}

// Parentheses is an expression in parentheses.
type Parentheses struct {
	BaseNode
	Value Node
}

func (n *Parentheses) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteString("(")
	n.Value.WriteGoCode(buf)
	buf.WriteString(")")
}

func (n *Parentheses) CollectVarRefs(reactive map[string]bool) []string {
	return n.Value.CollectVarRefs(reactive)
}
