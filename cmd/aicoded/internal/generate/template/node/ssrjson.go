package node

import "aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"

var _ Node = &SsrJSON{}

// SsrJSON renders a value as JSON in a <script type="application/json"> element with id Name,
// so page scripts can read it without inline code.
type SsrJSON struct {
	BaseNode
	Name  string
	Value Node
}

func (n *SsrJSON) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WritePrintString(`<script type="application/json" id="` + n.Name + `">`)
	buf.WriteStringLn(n.FilePos())
	buf.WriteString("if err := render.JSON(w, ")
	n.Value.WriteGoCode(buf)
	buf.WriteStringLn("); err != nil {\nreturn err\n}")
	buf.WritePrintString("</script>")
}

func (n *SsrJSON) CollectVarRefs(reactive map[string]bool) []string {
	return n.Value.CollectVarRefs(reactive)
}
