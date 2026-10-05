package node

import (
	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
)

var _ WithChildren = &SsrForm{}

// SsrForm is an <ssr:form>. It posts to its own page with the form ID and the CSRF token in
// hidden fields.
type SsrForm struct {
	BaseNode
	Name           string
	DPCtxFieldName string
	EncType        string
	Attributes     []HtmlAttribute
	Children       []Node
}

var formReservedAttrs = map[string]bool{
	"name":    true,
	"enctype": true,
}

// Form encodings.
const (
	FormEncTypeMultipart = "multipart/form-data"
	FormEncUrlEncoded    = "application/x-www-form-urlencoded"
)

// CollectVarRefs returns [] — form fields are not reactive.
func (n *SsrForm) CollectVarRefs(_ map[string]bool) []string { return []string{} }

func (n *SsrForm) AddChildren(children ...Node) {
	n.Children = append(n.Children, children...)
}

func (n *SsrForm) LastChild() Node {
	if len(n.Children) == 0 {
		return nil
	}
	return n.Children[len(n.Children)-1]
}

func (n *SsrForm) PopChild() {
	if len(n.Children) > 0 {
		n.Children = n.Children[:len(n.Children)-1]
	}
}

func (n *SsrForm) WriteGoCode(buf *gobuf.GoBuf) {
	buf.WriteStringLn("{")
	buf.WriteStringLn("form := s." + n.DPCtxFieldName)
	buf.WriteStringLn("_ = form")
	buf.WritePrintString(`<form method="post"`)
	if n.EncType == "" {
		n.EncType = FormEncUrlEncoded
	}
	buf.WritePrintString(` enctype="` + n.EncType + `"`)
	writeAttributes(buf, n.Attributes, formReservedAttrs)
	buf.WritePrintString(">")

	buf.WritePrintString(`<input type="hidden" name="_aicoded_form" value="`)
	buf.WriteStringLn("if err := render.Attr(w, form.ID()); err != nil {\nreturn err\n}")
	buf.WritePrintString(`"><input type="hidden" name="_aicoded_csrf" value="`)
	buf.WriteStringLn("if err := render.Attr(w, form.Token()); err != nil {\nreturn err\n}")
	buf.WritePrintString(`">`)

	for _, c := range n.Children {
		c.WriteGoCode(buf)
	}

	buf.WritePrintString("</form>")
	buf.WriteStringLn("}")
}
