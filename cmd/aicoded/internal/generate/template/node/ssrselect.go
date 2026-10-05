package node

import (
	"html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrSelect{}

// SsrSelect is an <ssr:select> form field; its options are set by the page's code.
type SsrSelect struct {
	BaseNode
	Name          string
	GoType        string
	FormFieldName string
	Attributes    []HtmlAttribute
	Multiple      bool
}

var selectReservedAttrs = map[string]bool{
	"name":   true,
	"value":  true,
	"gotype": true,
}

// CollectVarRefs returns [] — form fields are not reactive.
func (n *SsrSelect) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrSelect) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("{")
	buf.WriteStringLn(n.FilePos())
	buf.WriteStringLn("input := form." + n.FormFieldName)
	buf.WriteStringLn("_ = input")
	buf.WritePrintString("<select")
	buf.WritePrintString(` name="` + html.EscapeString(n.Name) + `"`)
	writeAttributes(buf, n.Attributes, selectReservedAttrs)
	buf.WritePrintString(">")

	if n.Multiple {
		buf.WriteStringLn("isSelected := func(v " + n.GoType + ") bool { _, exists := input.GetValue()[v]; return exists }")
	} else {
		buf.WriteStringLn("isSelected := func(v " + n.GoType + ") bool { return v == input.GetValue() }")
	}
	buf.WriteStringLn("for _, o := range input.GetOptions() {")
	buf.WriteStringLn("	if err := o.WriteHtml(w, isSelected); err != nil {")
	buf.WriteStringLn("		return err")
	buf.WriteStringLn("	}")
	buf.WriteStringLn("}")

	buf.WritePrintString("</select>")
	buf.WriteStringLn("}")
}
