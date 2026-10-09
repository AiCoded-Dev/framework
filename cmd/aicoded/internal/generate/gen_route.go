package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

// header starts every generated Go file.
const header = gobuf.Header

// routeFile is the name of the Go file of a page's route.
const routeFile = "route_gen.go"

// Import paths of the framework packages generated code uses.
const (
	webPkg    = "aicoded.dev/framework/web"
	formPkg   = "aicoded.dev/framework/web/form"
	renderPkg = "aicoded.dev/framework/web/render"
)

// routeKey returns the key of the route at p: the first 8 hex digits of sha256(p).
func routeKey(p string) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:4])
}

// route returns the route_gen.go of r: its data types, its data provider interface and the
// phases of the request as web.RouteState.
func (g *gen) route(r *Route) ([]byte, error) {
	t := r.Template
	forms := t.GetForms()
	for _, f := range forms {
		bindForm(f)
	}
	access := t.Access()
	guard := access != nil && access.Guard
	content := t.GetContentNode()
	hasForms := len(forms) > 0
	calls := t.Calls()
	lv := r.live

	b := gobuf.New()
	b.WriteString(header)
	b.WriteStringLn("package " + path.Base(folder(r.Path)))
	b.WriteStringLn("")
	b.WriteStringLn("import (")
	writable := lv != nil && len(lv.writable) > 0
	b.WriteQuotedString("context", "\n")
	if len(calls) > 0 {
		b.WriteQuotedString("encoding/json", "\n")
	}
	if writable {
		b.WriteQuotedString("errors", "\n")
	}
	b.WriteQuotedString("io", "\n")
	if hasForms || len(calls) > 0 {
		b.WriteQuotedString("net/http", "\n")
	}
	if lv != nil && lv.hasHTML() {
		b.WriteQuotedString("strings", "\n")
	}
	if lv != nil {
		b.WriteQuotedString("sync", "\n")
	}
	b.WriteStringLn("")
	b.WriteQuotedString(webPkg, "\n")
	if hasForms {
		b.WriteQuotedString(formPkg, "\n")
	}
	if lv != nil {
		b.WriteQuotedString(reactivePkg, "\n")
	}
	if writesValues(t.GetNodes()...) || lv != nil && lv.hasText() {
		b.WriteQuotedString(renderPkg, "\n")
	}
	b.WriteStringLn(")")
	b.WriteStringLn("")
	b.WriteString("const routeKey = ")
	b.WriteQuotedString(routeKey(r.Path), "\n\n")

	b.WriteStringLn("// RouteData holds the variables the template declares with <ssr:var>.")
	if vars := t.GetVariables(); len(vars) == 0 {
		b.WriteStringLn("type RouteData struct{}")
	} else {
		b.WriteStringLn("type RouteData struct {")
		for _, v := range vars {
			b.WriteStringLn(exported(v.Name) + " " + v.Type)
		}
		b.WriteStringLn("}")
	}
	b.WriteStringLn("")

	for _, f := range forms {
		b.WriteStringLn("// " + valuesType(f) + " holds the fields of the form " + f.Name + ".")
		b.WriteStringLn("type " + valuesType(f) + " struct {")
		b.WriteStringLn("form.BaseFormValues")
		for _, e := range f.Elements {
			b.WriteStringLn(exported(e.Name) + " " + fieldType(e))
		}
		b.WriteStringLn("}")
		b.WriteStringLn("")
	}

	b.WriteStringLn("// RouteDataProvider is what this page's DP implements.")
	b.WriteStringLn("type RouteDataProvider interface {")
	for _, h := range hooks(t) {
		if h.note != "" {
			b.WriteStringLn("// " + h.note)
		}
		b.WriteStringLn(h.sig)
	}
	b.WriteStringLn("}")
	b.WriteStringLn("")

	b.WriteStringLn("type route struct{ dp RouteDataProvider }")
	b.WriteStringLn("")
	b.WriteStringLn("// NewRoute returns the page for this folder.")
	if guard {
		b.WriteStringLn("func NewRoute(dp RouteDataProvider) web.Route { return &route{dp: dp} }")
	} else {
		b.WriteStringLn("func NewRoute(dp RouteDataProvider) web.Route {")
		b.WriteString("web.CheckNoGuard(dp, ")
		b.WriteQuotedString(r.Path, ")\n")
		b.WriteStringLn("return &route{dp: dp}")
		b.WriteStringLn("}")
	}
	b.WriteStringLn("")

	b.WriteString("func (rt *route) Access() web.Access { return web.Access{")
	if access != nil {
		b.WriteString("Roles: []string{" + quoteAll(access.Roles) + "}")
	}
	b.WriteStringLn("} }")
	b.WriteStringLn("")
	b.WriteStringLn("func (rt *route) Layout() bool { return " + strconv.FormatBool(content != nil) + " }")
	b.WriteStringLn("")

	b.WriteStringLn("func (rt *route) NewState() web.RouteState {")
	b.WriteStringLn("s := &state{dp: rt.dp}")
	b.WriteStringLn("s.SetAssets([]string{" + quoteAll(g.assetTags(r.Path)) + "})")
	for _, f := range forms {
		fields := make([]string, len(f.Elements))
		for i, e := range f.Elements {
			fields[i] = "&s." + formField(f) + "." + exported(e.Name)
		}
		b.WriteStringLn("s." + formField(f) + ".SetElements([]form.Element{" + strings.Join(fields, ", ") + "})")
	}
	b.WriteStringLn("return s")
	b.WriteStringLn("}")
	b.WriteStringLn("")

	b.WriteStringLn("type state struct {")
	b.WriteStringLn("web.Frame")
	if lv != nil {
		b.WriteStringLn("mu sync.Mutex")
	}
	b.WriteStringLn("dp RouteDataProvider")
	b.WriteStringLn("RouteData RouteData")
	for _, f := range forms {
		b.WriteStringLn(formField(f) + " " + valuesType(f))
	}
	b.WriteStringLn("}")
	b.WriteStringLn("")

	if guard {
		b.WriteStringLn("func (s *state) Guard(ctx context.Context, r *web.Request) error { return s.dp.Guard(ctx, r) }")
	} else {
		b.WriteStringLn("func (s *state) Guard(context.Context, *web.Request) error { return nil }")
	}
	b.WriteStringLn("")
	writeInitForms(b, forms)
	b.WriteStringLn("")
	writeSubmitForm(b, forms)
	b.WriteStringLn("")

	b.WriteStringLn("func (s *state) Data(ctx context.Context, r *web.Request, w web.ResponseWriter) error {")
	b.WriteStringLn("return s.dp.Data(ctx, r, w, &s.RouteData)")
	b.WriteStringLn("}")
	b.WriteStringLn("")

	switch {
	case content == nil:
		b.WriteStringLn(`func (s *state) DefaultRoute(context.Context, *web.Request) (string, error) { return "", nil }`)
	case content.Default == "":
		b.WriteStringLn("func (s *state) DefaultRoute(ctx context.Context, r *web.Request) (string, error) { return s.dp.DefaultRoute(ctx, r) }")
	default:
		b.WriteString("func (s *state) DefaultRoute(context.Context, *web.Request) (string, error) { return ")
		b.WriteQuotedString(content.Default, ", nil }\n")
	}
	b.WriteStringLn("")
	if g.live(r.Path) {
		b.WriteStringLn("func (s *state) RouteKey() string { return routeKey }")
		b.WriteStringLn("")
	}
	if len(calls) > 0 {
		writeCall(b, calls)
	}
	if lv != nil {
		writeLive(b, lv)
	}

	b.WriteStringLn("func (s *state) Write(w io.Writer) error {")
	for _, v := range t.GetVariables() {
		b.WriteStringLn(v.FilePos())
		b.WriteStringLn(v.Name + " := s.RouteData." + exported(v.Name))
		b.WriteStringLn("_ = " + v.Name)
	}
	t.WriteGoCode(b)
	b.WriteLineReset(routeFile)
	b.WriteStringLn("return nil")
	b.WriteStringLn("}")
	b.WriteStringLn("")
	if lv != nil {
		writeRenderBlocks(b, lv, t.GetVariables())
	}
	b.WriteConsts()

	code, err := b.Formatted()
	if err != nil {
		return nil, fmt.Errorf("generate %s: %w", folder(r.Path), err)
	}
	return code, nil
}

// hook is a method of a route's RouteDataProvider: its name and signature, a note on it in the
// interface, and the go doc and return statement of its stub in dataprovider.go.
type hook struct {
	sig, note, doc, stub string
}

// hooks returns the methods of the RouteDataProvider of template t. The stubs fail closed: a
// stub Guard refuses every viewer, a stub DefaultRoute names no page, which answers 404, a stub
// Validate refuses every value and a stub Call refuses every call.
func hooks(t *template.Template) []hook {
	var hs []hook
	if a := t.Access(); a != nil && a.Guard {
		hs = append(hs, hook{sig: "Guard(ctx context.Context, r *web.Request) error",
			doc: "Guard refuses every viewer until it checks the viewer's access to what this page shows.", stub: "return web.Forbidden()"})
	}
	hs = append(hs, hook{sig: "Data(ctx context.Context, r *web.Request, w web.ResponseWriter, data *RouteData) error",
		doc: "Data fills the variables the template declares.", stub: "return nil"})
	if c := t.GetContentNode(); c != nil && c.Default == "" {
		hs = append(hs, hook{sig: "DefaultRoute(ctx context.Context, r *web.Request) (string, error)",
			doc: "DefaultRoute names the page below this one that opening this page shows; \"\" answers 404.", stub: `return "", nil`})
	}
	for _, f := range t.GetForms() {
		values := "(ctx context.Context, r *web.Request, w web.ResponseWriter, data *" + valuesType(f) + ") error"
		hs = append(hs,
			hook{sig: "Init" + exported(f.Name) + values,
				doc: "Init" + exported(f.Name) + " sets the starting values and options of the form " + f.Name + ". It runs every time\n" +
					"the page shows or receives the form, on a GET too, so it must not change data:\n" +
					"Process" + exported(f.Name) + " does that.",
				stub: "return nil"},
			hook{sig: "Process" + exported(f.Name) + values, doc: "Process" + exported(f.Name) + " handles the form " + f.Name + " once every field is valid.", stub: "return nil"})
	}
	vars := t.GetVariables()
	if len(reactiveVars(vars)) > 0 {
		hs = append(hs, hook{sig: "Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error",
			note: "Subscribe runs while the page is open and must return when ctx is done.",
			doc: "Subscribe sends new live values with state.Set… while the page is open. It must return\n" +
				"when ctx is done; until it returns, the viewer's live connection stays taken.",
			stub: "return nil"})
	}
	for _, v := range writableVars(vars) {
		name := "Validate" + exported(v.Name)
		hs = append(hs, hook{sig: name + "(ctx context.Context, r *web.Request, val " + v.Type + ") (" + v.Type + ", error)",
			doc:  name + " refuses every value the page writes to " + v.Name + " until it checks the value and returns the one to store.",
			stub: `return val, errors.New("this value cannot be changed yet")`})
	}
	for _, c := range t.Calls() {
		name := callMethod(c)
		hs = append(hs, hook{sig: name + "(ctx context.Context, r *web.Request, in " + c.In + ") (" + c.Out + ", error)",
			doc: name + " answers the page's call " + c.Name + ". It refuses every call until it checks that the viewer\n" +
				"may make it and what the page sent.",
			stub: "var out " + c.Out + "\nreturn out, web.Forbidden()"})
	}
	return hs
}

// writeInitForms writes InitForms, which prepares every form of the page and runs its Init hook.
func writeInitForms(b *gobuf.GoBuf, forms []*template.Form) {
	if len(forms) == 0 {
		b.WriteStringLn("func (s *state) InitForms(context.Context, *web.Request, web.ResponseWriter) error { return nil }")
		return
	}
	b.WriteStringLn("func (s *state) InitForms(ctx context.Context, r *web.Request, w web.ResponseWriter) error {")
	for _, f := range forms {
		field := "s." + formField(f)
		b.WriteString(field + ".Prepare(routeKey+")
		b.WriteQuotedString("."+f.Name, ", r.CSRFToken())\n")
		b.WriteStringLn("if err := s.dp.Init" + exported(f.Name) + "(ctx, r, w, &" + field + "); err != nil {")
		b.WriteStringLn("return err")
		b.WriteStringLn("}")
	}
	b.WriteStringLn("return nil")
	b.WriteStringLn("}")
}

// writeSubmitForm writes SubmitForm, which reads the posted form into its fields and runs its
// Process hook only when every field is valid.
func writeSubmitForm(b *gobuf.GoBuf, forms []*template.Form) {
	if len(forms) == 0 {
		b.WriteStringLn("func (s *state) SubmitForm(context.Context, *web.Request, web.ResponseWriter, string) (bool, error) { return false, nil }")
		return
	}
	b.WriteStringLn("func (s *state) SubmitForm(ctx context.Context, r *web.Request, w web.ResponseWriter, id string) (bool, error) {")
	b.WriteStringLn("switch id {")
	for _, f := range forms {
		field := "s." + formField(f)
		multipart := f.Node.EncType == node.FormEncTypeMultipart
		b.WriteString("case routeKey + ")
		b.WriteQuotedString("."+f.Name, ":\n")
		if multipart {
			b.WriteStringLn("if !form.IsMultipart(r.Request) {")
		} else {
			b.WriteStringLn("if form.IsMultipart(r.Request) {")
		}
		b.WriteStringLn(`return true, web.Error(http.StatusBadRequest, "The form was sent in the wrong format.")`)
		b.WriteStringLn("}")
		for _, e := range f.Elements {
			b.WriteString(field + "." + exported(e.Name) + ".Process(r.Request, ")
			b.WriteQuotedString(e.Name, ", "+strconv.FormatBool(multipart)+", "+strconv.FormatBool(e.IsRequired)+")\n")
		}
		b.WriteStringLn(field + ".MarkValidated()")
		b.WriteStringLn("if " + field + ".HasError() {")
		b.WriteStringLn("return true, nil")
		b.WriteStringLn("}")
		b.WriteStringLn("return true, s.dp.Process" + exported(f.Name) + "(ctx, r, w, &" + field + ")")
	}
	b.WriteStringLn("}")
	b.WriteStringLn("return false, nil")
	b.WriteStringLn("}")
}

// bindForm names the state fields the nodes of form f write from.
func bindForm(f *template.Form) {
	f.Node.DPCtxFieldName = formField(f)
	for _, e := range f.Elements {
		for _, n := range e.Nodes {
			switch n := n.(type) {
			case *node.SsrInput:
				n.FormFieldName = exported(e.Name)
			case *node.SsrSelect:
				n.FormFieldName = exported(e.Name)
			case *node.SsrTextarea:
				n.FormFieldName = exported(e.Name)
			}
		}
	}
}

// fieldType returns the web/form type of a form field.
func fieldType(e *template.FormElement) string {
	switch e.Type {
	case template.FormElementInput:
		if e.IsMultiple {
			return "form.InputMultiple[" + e.GoType + "]"
		}
		return "form.Input[" + e.GoType + "]"
	case template.FormElementInputFile:
		if e.IsMultiple {
			return "form.FileMultiple"
		}
		return "form.File"
	case template.FormElementTextarea:
		return "form.Textarea"
	case template.FormElementSelect:
		if e.IsMultiple {
			return "form.SelectMultiple[" + e.GoType + "]"
		}
		return "form.Select[" + e.GoType + "]"
	}
	panic(fmt.Sprintf("generate: form field %q has no type", e.Name))
}

// formField returns the name of the state field that holds form f.
func formField(f *template.Form) string { return "Form" + exported(f.Name) }

// valuesType returns the name of the type that holds the fields of form f.
func valuesType(f *template.Form) string { return formField(f) + "Values" }

// exported returns name with its first letter in upper case.
func exported(name string) string {
	r, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(r)) + name[size:]
}

// quoteAll returns ss as Go string literals separated by commas.
func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}

// writesValues reports whether the nodes write a value through web/render.
func writesValues(nodes ...node.Node) bool {
	for _, n := range nodes {
		if writesValue(n) {
			return true
		}
	}
	return false
}

func writesValue(n node.Node) bool {
	switch v := n.(type) {
	case *node.Expression, *node.RawExpression, *node.TernaryIf, *node.SsrJSON, *node.SsrForm, *node.SsrTextarea:
		return true
	case *node.Text, *node.HtmlRaw, *node.SsrContent, *node.SsrAssets, *node.Variable, *node.String, *node.Number:
		return false
	case *node.SsrInput:
		return v.Type != "file" && v.Type != "checkbox" && v.Type != "radio" || attrsWriteValues(v.Attributes)
	case *node.SsrSelect:
		return attrsWriteValues(v.Attributes)
	case *node.HtmlElement:
		return attrsWriteValues(v.Attributes) || writesValues(v.Children...)
	case *node.Content:
		return writesValues(v.Children...)
	case *node.Loop:
		return writesValue(v.Array) || writesValues(v.Children...)
	case *node.SsrCondition:
		for _, c := range v.Conditions {
			if writesValues(c.Condition, c.Body) {
				return true
			}
		}
		return v.ElseBody != nil && writesValue(v.ElseBody)
	case *node.Parentheses:
		return writesValue(v.Value)
	case *node.StructField:
		return writesValue(v.Expr)
	case *node.Operator:
		return v.Left != nil && writesValue(v.Left) || writesValue(v.Right)
	case *node.Indexed:
		return writesValues(v.Expr, v.Index)
	case *node.Function:
		return writesValue(v.Expr) || writesValues(v.Arguments.Values...)
	}
	panic(fmt.Sprintf("generate: writesValue: unexpected node %T", n))
}

func attrsWriteValues(attrs []node.HtmlAttribute) bool {
	for _, a := range attrs {
		if writesValues(a.Values...) {
			return true
		}
	}
	return false
}
