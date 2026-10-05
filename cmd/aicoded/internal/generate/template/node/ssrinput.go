package node

import (
	"html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ Node = &SsrInput{}

// SsrInput is an <ssr:input> form field.
type SsrInput struct {
	BaseNode
	Name          string
	FormFieldName string
	Type          string
	// Value is the fixed value of a radio or checkbox input; GoValue is the same value as a
	// Go literal of the field's gotype.
	Value      string
	GoValue    string
	Multiple   bool
	Attributes []HtmlAttribute
}

var inputReservedAttrs = map[string]bool{
	"name":   true,
	"type":   true,
	"value":  true,
	"gotype": true,
}

// CollectVarRefs returns [] — form fields are not reactive.
func (n *SsrInput) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrInput) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("{")
	buf.WriteStringLn(n.FilePos())
	buf.WriteStringLn("input := form." + n.FormFieldName)
	buf.WriteStringLn("_ = input")
	buf.WritePrintString("<input")

	buf.WritePrintString(` name="` + html.EscapeString(n.Name) + `"`)
	buf.WritePrintString(` type="` + html.EscapeString(n.Type) + `"`)
	if n.Type != "file" && n.Type != "checkbox" && n.Type != "radio" {
		buf.WriteStringLn("if input.IsNotNull() {")
		buf.WritePrintString(` value="`)
		buf.WriteStringLn("v := any(input.GetValue())")
		// An invalid value is shown as it was sent.
		buf.WriteStringLn("if input.HasError() {")
		buf.WriteStringLn("v = input.GetFormValue()")
		buf.WriteStringLn("}")
		buf.WriteStringLn("if err := render.Attr(w, v); err != nil {\nreturn err\n}")
		buf.WritePrintString(`"`)
		buf.WriteStringLn("}")
	}
	if n.Type == "radio" || n.Type == "checkbox" {
		buf.WritePrintString(` value="` + html.EscapeString(n.Value) + `"`)

		if n.Multiple {
			buf.WriteStringLn("if _, exists := input.GetValue()[" + n.GoValue + "]; exists {")
		} else {
			buf.WriteStringLn("if input.GetValue() == " + n.GoValue + " {")
		}
		buf.WritePrintString(" checked")
		buf.WriteStringLn("}")
	}

	writeAttributes(buf, n.Attributes, inputReservedAttrs)
	buf.WritePrintString(">")
	buf.WriteStringLn("}")
}
