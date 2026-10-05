package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &HtmlRaw{}

// HtmlRaw is markup written as it is, such as the doctype.
type HtmlRaw struct {
	BaseNode
	Data string
}

func (n *HtmlRaw) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WritePrintString(n.Data)
}

func (n *HtmlRaw) CollectVarRefs(_ map[string]bool) []string { return []string{} }
