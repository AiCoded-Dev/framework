package node

import (
	"html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrTextarea{}

// SsrTextarea is an <ssr:textarea> form field.
type SsrTextarea struct {
	BaseNode
	Name          string
	FormFieldName string
	Attributes    []HtmlAttribute
}

var textareaReservedAttrs = map[string]bool{
	"name":  true,
	"value": true,
}

// CollectVarRefs returns [] — form fields are not reactive.
func (n *SsrTextarea) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrTextarea) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("{")
	buf.WriteStringLn(n.FilePos())
	buf.WriteStringLn("textarea := form." + n.FormFieldName)
	buf.WriteStringLn("_ = textarea")
	buf.WritePrintString("<textarea")
	buf.WritePrintString(` name="` + html.EscapeString(n.Name) + `"`)
	writeAttributes(buf, n.Attributes, textareaReservedAttrs)
	buf.WritePrintString(">")
	buf.WriteStringLn("if err := render.Text(w, textarea.GetValue()); err != nil {\nreturn err\n}")
	buf.WritePrintString("</textarea>")
	buf.WriteStringLn("}")
}
