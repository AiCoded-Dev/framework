package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Indexed{}

// Indexed is an index expression, a[i].
type Indexed struct {
	BaseNode
	Expr  Node
	Index Node
}

func (n *Indexed) WriteGoCode(buf *gobuf.GoBuf) {
	n.Expr.WriteGoCode(buf)
	buf.WriteString("[")
	n.Index.WriteGoCode(buf)
	buf.WriteString("]")
}

func (n *Indexed) CollectVarRefs(reactive map[string]bool) []string {
	return UnionRefs(n.Expr.CollectVarRefs(reactive), n.Index.CollectVarRefs(reactive))
}
