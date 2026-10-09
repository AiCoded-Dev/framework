package node

import (
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrCondition{}

// SsrConditionData is one ssr:if or ssr:else-if branch.
type SsrConditionData struct {
	BaseNode
	Condition Node
	Body      Node
}

// SsrCondition is a chain of elements with ssr:if, ssr:else-if and ssr:else.
type SsrCondition struct {
	BaseNode
	Conditions []SsrConditionData
	ElseBody   Node
	// BlockKey is set when a condition or a branch is live; the output is then wrapped in
	// <ssr-block data-ssr-bind="KEY"> and re-rendered whole.
	BlockKey string
}

func (n *SsrCondition) CollectVarRefs(reactive map[string]bool) []string {
	var sets [][]string
	for _, c := range n.Conditions {
		sets = append(sets, c.Condition.CollectVarRefs(reactive))
		sets = append(sets, c.Body.CollectVarRefs(reactive))
	}
	if n.ElseBody != nil {
		sets = append(sets, n.ElseBody.CollectVarRefs(reactive))
	}
	return UnionRefs(sets...)
}

func (n *SsrCondition) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	if n.BlockKey != "" {
		buf.WritePrintString(openMarker("ssr-block", n.BlockKey))
	}
	n.writeInner(buf)
	if n.BlockKey != "" {
		buf.WritePrintString("</ssr-block>")
	}
}

// WriteInnerGoCode writes the branches without the live-block wrapper.
func (n *SsrCondition) WriteInnerGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn(n.FilePos())
	n.writeInner(buf)
}

// writeInner writes each ssr:else-if as an if statement inside the else block of the one before,
// never as an else-if chain: staticcheck reports a chain that compares one value as QF1003, and
// a builder cannot change generated code.
func (n *SsrCondition) writeInner(buf *gobuf.GoBuf) {
	for i, c := range n.Conditions {
		if i > 0 {
			buf.WriteStringLn("} else {")
			buf.WriteStringLn(c.FilePos())
		}
		buf.WriteString("if ")
		c.Condition.WriteGoCode(buf)
		buf.WriteStringLn(" {")
		c.Body.WriteGoCode(buf)
	}

	if n.ElseBody != nil {
		buf.WriteStringLn(n.ElseBody.FilePos())
		buf.WriteStringLn("} else {")
		n.ElseBody.WriteGoCode(buf)
	}

	buf.WriteString(strings.Repeat("}\n", len(n.Conditions)))
}
