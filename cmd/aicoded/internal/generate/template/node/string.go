package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &String{}

// String is a string literal. Text is the literal as Go source.
type String struct {
	BaseNode
	Text string
}

func (n *String) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteString(n.Text)
}

func (n *String) CollectVarRefs(_ map[string]bool) []string { return []string{} }
