package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &StructField{}

// StructField is a selector, a.B.
type StructField struct {
	BaseNode
	Expr      Node
	FieldName string
}

func (n *StructField) WriteGoCode(buf *gobuf.GoBuf) {
	n.Expr.WriteGoCode(buf)
	buf.WriteString(".")
	buf.WriteString(n.FieldName)
}

// CollectVarRefs returns the variables of the expression the field is read from.
func (n *StructField) CollectVarRefs(reactive map[string]bool) []string {
	return n.Expr.CollectVarRefs(reactive)
}
