package node

import (
	"golang.org/x/net/html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Text{}

// Text is literal text, written HTML-escaped.
type Text struct {
	BaseNode
	Text string
}

func (n *Text) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WritePrintString(html.EscapeString(n.Text))
}

func (n *Text) CollectVarRefs(_ map[string]bool) []string { return []string{} }
