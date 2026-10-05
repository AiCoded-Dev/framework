package node

import (
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &Number{}

// Number is a number literal. A negative one is written in parentheses, so it stays one operand
// after an operator, as in a-(-1).
type Number struct {
	BaseNode
	Text string
}

func (n *Number) WriteGoCode(buf *gobuf.GoBuf) {
	if strings.HasPrefix(n.Text, "-") {
		buf.WriteString("(" + n.Text + ")")
		return
	}
	buf.WriteString(n.Text)
}

func (n *Number) CollectVarRefs(_ map[string]bool) []string { return []string{} }
