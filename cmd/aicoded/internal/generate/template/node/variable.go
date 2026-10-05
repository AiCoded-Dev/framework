package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Variable{}

// Variable is a name.
type Variable struct {
	BaseNode
	Name string
}

func (n *Variable) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteString(n.Name)
}

func (n *Variable) CollectVarRefs(reactive map[string]bool) []string {
	if reactive[n.Name] {
		return []string{n.Name}
	}
	return []string{}
}
