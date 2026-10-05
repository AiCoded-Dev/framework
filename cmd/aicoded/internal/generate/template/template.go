// Package template parses page templates (index.html) into nodes that write Go code.
package template

//go:generate go run golang.org/x/tools/cmd/goyacc@v0.34.0 -o texty.go -v /dev/null text.y

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/htmlutils"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

var (
	reWhitespace = regexp.MustCompile(`\s+`)
	// reName is the shape of form and field names.
	reName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	// fieldTags are the ssr: tags of form fields.
	fieldTags = map[string]bool{"input": true, "textarea": true, "select": true}
	// reJSONName is the shape of an <ssr:json> name.
	reJSONName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	// wrappers are the ssr: attributes that put their element in a loop or a condition.
	wrappers = map[string]bool{"for": true, "if": true, "else-if": true, "else": true}
	// bindable are the elements ssr:bind may bind.
	bindable = map[string]bool{"input": true, "select": true, "textarea": true}
	// roleName is the shape of a role in <ssr:access>.
	roleName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	// callName is the shape of an <ssr:call> name.
	callName = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,62}$`)
)

// Access is a template's <ssr:access> rule: the viewer needs one of Roles, "*" admits every
// viewer, and Guard declares the route's Guard method. Line is the line of the tag.
type Access struct {
	Roles []string
	Guard bool
	Line  int
}

// Call is an <ssr:call>: a function of the page that its script may call. In and Out are the
// Go types of its argument and result. Line is the line of the tag.
type Call struct {
	Name, In, Out string
	Line          int
}

// Template is a parsed page template.
type Template struct {
	nodes       []node.Node
	variables   []Variable
	forms       []*Form
	contentNode *node.SsrContent
	ssrBindRefs []SsrBindRef
	access      *Access
	calls       []Call
	hasAssets   bool
}

// Variable is an <ssr:var> declaration.
type Variable struct {
	File           string
	Line           int
	Name           string
	Type           string
	Reactive       bool
	ClientWritable bool
}

// SsrBindRef is an ssr:bind attribute on a native input. File is the display path.
type SsrBindRef struct {
	File    string
	Line    int
	VarName string
	Element *node.HtmlElement
}

// Form is an <ssr:form> and its fields.
type Form struct {
	Name     string
	Node     *node.SsrForm
	Elements []*FormElement
}

// FormElement is a form field; radio and checkbox inputs sharing a name are one field.
type FormElement struct {
	Name       string
	Nodes      []node.Node
	Type       FormElementType
	IsRequired bool
	IsMultiple bool
	GoType     string
}

// FormElementType is the kind of a form field.
type FormElementType uint8

// Form field kinds.
const (
	FormElementUnknown FormElementType = iota
	FormElementInput
	FormElementInputFile
	FormElementTextarea
	FormElementSelect
)

// formElementGoTypes maps the gotypes of form fields to their bit size.
var formElementGoTypes = map[string]int{
	"string":  0,
	"bool":    0,
	"int":     64,
	"int8":    8,
	"int16":   16,
	"int32":   32,
	"int64":   64,
	"uint":    64,
	"uint8":   8,
	"uint16":  16,
	"uint32":  32,
	"uint64":  64,
	"float32": 32,
	"float64": 64,
}

// FilePos returns the //line directive of the declaration.
func (v *Variable) FilePos() string {
	return fmt.Sprintf("//line %s:%d", v.File, v.Line)
}

// Parse reads the template at path. display is the path shown in error positions, relative to
// the app root, such as pages/notes/index.html; node positions keep the base name of path.
// image maps the src of an <img> to its served URL; nil keeps src as it is. Parse returns nil
// and no error when the file does not exist.
func Parse(path, display string, image func(src string) (string, error)) (*Template, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not open template file: %w", err)
	}
	defer f.Close()

	if image == nil {
		image = func(src string) (string, error) { return src, nil }
	}
	p := &parser{
		file:      filepath.Base(path),
		display:   display,
		image:     image,
		tok:       html.NewTokenizer(f),
		line:      1,
		t:         &Template{},
		jsonLines: map[string]int{},
		wrappers:  map[*node.HtmlElement]string{},
	}
	return p.parse()
}

type parser struct {
	file    string
	display string
	image   func(src string) (string, error)
	tok     *html.Tokenizer
	line    int
	stack   NodesStack
	t       *Template
	form    *Form
	// jsonLines holds the line of every <ssr:json> by name.
	jsonLines map[string]int
	// formAttrs is set while the attributes of an <ssr:form> are read.
	formAttrs bool
	// wrappers holds the loop or condition directive of every open element that has one.
	wrappers map[*node.HtmlElement]string
}

// tagAttr is an attribute of the current tag.
type tagAttr struct {
	rawAttr
	value string
}

func (p *parser) parse() (*Template, error) {
	root := &node.HtmlElement{}
	p.stack = NodesStack{root}
	for {
		tokenType := p.tok.Next()
		raw := p.tok.Raw()
		lines := bytes.Count(raw, []byte{'\n'})
		var err error
		switch tokenType {
		case html.ErrorToken:
			if !errors.Is(p.tok.Err(), io.EOF) {
				return nil, fmt.Errorf("could not read template file: %w", p.tok.Err())
			}
			if len(raw) > 0 {
				return nil, p.fail("E-GEN-001", "the tag %s is never closed", preview(raw))
			}
			if t, ok := root.LastChild().(*node.Text); ok && t.Text == " " {
				root.PopChild()
			}
			p.t.nodes = root.Children
			return p.t, nil
		case html.TextToken:
			err = p.text()
		case html.StartTagToken, html.SelfClosingTagToken:
			err = p.startTag(scanAttrs(raw), tokenType == html.SelfClosingTagToken)
		case html.EndTagToken:
			err = p.endTag()
		case html.DoctypeToken:
			p.stack.Top().AddChildren(&node.HtmlRaw{BaseNode: p.pos(), Data: string(raw)})
		}
		if err != nil {
			return nil, err
		}
		p.line += lines
	}
}

func (p *parser) pos() node.BaseNode { return node.BaseNode{File: p.file, Line: p.line} }

// startTag handles a start tag whose attributes, as the source spells them, are scanned.
func (p *parser) startTag(scanned []rawAttr, selfClosing bool) error {
	name, hasAttrs := p.tok.TagName()
	tag := string(name)
	if strings.Contains(tag, "{{") || strings.Contains(tag, "}}") {
		return p.fail("E-GEN-025", "a value in the tag name <%s>", tag)
	}
	attrs, err := p.attrs(tag, scanned, hasAttrs)
	if err != nil {
		return err
	}
	if t, ok := strings.CutPrefix(tag, "ssr:"); ok {
		return p.ssrTag(t, attrs, selfClosing)
	}
	return p.element(tag, attrs, selfClosing)
}

// attrs reads the attributes of the current tag and takes from scanned whether each value was
// quoted. A tag the tokenizer and the scan read differently is refused.
func (p *parser) attrs(tag string, scanned []rawAttr, hasAttrs bool) ([]tagAttr, error) {
	seen := make(map[string]bool, len(scanned))
	for _, a := range scanned {
		if seen[a.name] {
			return nil, p.fail("E-GEN-001", "<%s> has the attribute %s twice", tag, a.name)
		}
		if strings.HasPrefix(a.name, "data-ssr-") {
			return nil, p.fail("E-GEN-042", "<%s> has the attribute %s, which only the generated code may write", tag, a.name)
		}
		seen[a.name] = true
	}
	var attrs []tagAttr
	for more := hasAttrs; more; {
		var k, v []byte
		k, v, more = p.tok.TagAttr()
		attrs = append(attrs, tagAttr{rawAttr: rawAttr{name: string(k)}, value: string(v)})
	}
	if len(attrs) != len(scanned) {
		return nil, p.fail("E-GEN-001", "cannot read the attributes of <%s>", tag)
	}
	for i := range attrs {
		if attrs[i].name != scanned[i].name {
			return nil, p.fail("E-GEN-001", "cannot read the attributes of <%s>", tag)
		}
		attrs[i].quoted = scanned[i].quoted
	}
	return attrs, nil
}

// fail returns a coded error at the current line.
func (p *parser) fail(code, format string, args ...any) *errs.Error {
	return errAt(p.display, p.line, code, fixes[code], format, args...)
}

// failAt returns a coded error at line.
func (p *parser) failAt(line int, code, format string, args ...any) *errs.Error {
	return errAt(p.display, line, code, fixes[code], format, args...)
}

func preview(raw []byte) string {
	s, _, _ := strings.Cut(string(raw), "\n")
	if len(s) > 40 {
		s = s[:40] + "…"
	}
	return s
}

// text handles text by the element it is in: a <script> takes no body, raw-text elements take no
// values and no markup, and <title> and <textarea> take no {{$ }}. Outside <pre>, <textarea>
// and raw-text elements, a run of whitespace becomes one space, as the browser shows it;
// whitespace at the start of the template, or right after text that ends in a space, adds
// nothing.
func (p *parser) text() error {
	text := string(p.tok.Text())
	tag := p.stack.topTag()
	preserve := p.stack.isWhitespacePreserved()
	if !preserve && strings.Trim(text, htmlSpace) == "" {
		if p.showsSpace() {
			p.stack.Top().AddChildren(&node.Text{BaseNode: p.pos(), Text: " "})
		}
		return nil
	}
	if tag == "script" {
		if strings.Trim(text, htmlSpace) != "" {
			return p.fail("E-GEN-020", "<script> has a body")
		}
		p.stack.Top().AddChildren(&node.Text{BaseNode: p.pos(), Text: text})
		return nil
	}
	nodes, err := p.parseText(text, false)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		switch v := n.(type) {
		case *node.Expression:
			if rawTextElements[tag] {
				return p.failAt(v.Line, "E-GEN-026", "a value inside <%s>", tag)
			}
			v.RCDATA = rcdataElements[tag]
		case *node.RawExpression:
			if rawTextElements[tag] {
				return p.failAt(v.Line, "E-GEN-026", "a value inside <%s>", tag)
			}
			if rcdataElements[tag] {
				return p.failAt(v.Line, "E-GEN-029", "{{$ }} inside <%s>", tag)
			}
		}
	}
	if rawTextElements[tag] && strings.Contains(text, "<") {
		return p.fail("E-GEN-001", "markup inside <%s>; it may hold only plain text", tag)
	}
	if !preserve {
		for _, n := range nodes {
			if t, ok := n.(*node.Text); ok {
				t.Text = reWhitespace.ReplaceAllString(t.Text, " ")
			}
		}
	}
	p.stack.Top().AddChildren(nodes...)
	return nil
}

// showsSpace reports whether whitespace at this point of the template shows as a space: it does
// unless it starts the template or follows text that ends in a space.
func (p *parser) showsSpace() bool {
	switch last := p.stack.Top().LastChild().(type) {
	case nil:
		return len(p.stack) > 1
	case *node.Text:
		return !strings.HasSuffix(last.Text, " ")
	}
	return true
}

// attrValue parses an attribute value.
func (p *parser) attrValue(value string) ([]node.Node, error) {
	if value == "" {
		return nil, nil
	}
	return p.parseText(value, false)
}

func (p *parser) ssrTag(tag string, attrs []tagAttr, selfClosing bool) error {
	switch tag {
	case "access":
		return p.accessTag(attrs)
	case "call":
		return p.callTag(attrs)
	}
	byName := map[string]string{}
	var attributes []node.HtmlAttribute
	rendered := tag == "form" || fieldTags[tag]
	p.formAttrs = tag == "form"
	for _, a := range attrs {
		if strings.HasPrefix(a.name, "ssr:") && (a.name != "ssr:bind" || !fieldTags[tag]) {
			return p.fail("E-GEN-005", "%s does not work on <ssr:%s>; put it on an element around the tag", a.name, tag)
		}
		byName[a.name] = a.value
		var values []node.Node
		var err error
		if rendered {
			values, err = p.attr(tag, a)
		} else {
			values, err = p.attrValue(a.value)
		}
		if err != nil {
			return err
		}
		attributes = append(attributes, node.HtmlAttribute{Key: a.name, Values: values})
	}
	p.formAttrs = false

	switch tag {
	case "var":
		switch name := byName["name"]; {
		case name == "":
			return p.fail("E-GEN-002", "<ssr:var> has no name")
		case !token.IsIdentifier(name):
			return p.fail("E-GEN-042", "<ssr:var name=%q>: the name is not a Go identifier", name)
		case isReserved(name):
			return p.fail("E-GEN-042", "<ssr:var name=%q>: %s is a name the generated code uses", name, name)
		}
		if byName["type"] == "" {
			return p.fail("E-GEN-003", "<ssr:var name=%q> has no type", byName["name"])
		}
		typ, problem := goType(byName["type"])
		if problem != "" {
			return p.fail("E-GEN-045", "<ssr:var name=%q>: %s", byName["name"], problem)
		}
		if used := usedName(typ, varLocals); used != "" {
			return p.fail("E-GEN-042", "<ssr:var name=%q> type: %s is a name the generated code uses", byName["name"], used)
		}
		p.t.variables = append(p.t.variables, Variable{
			File:           p.file,
			Line:           p.line,
			Name:           byName["name"],
			Type:           typ,
			Reactive:       byName["reactive"] == "true",
			ClientWritable: byName["client-writable"] == "true",
		})
	case "content":
		if p.t.contentNode != nil {
			return p.fail("E-GEN-039", "a second <ssr:content/>; the first is at line %d", p.t.contentNode.Line)
		}
		p.t.contentNode = &node.SsrContent{BaseNode: p.pos(), Default: byName["default"]}
		p.stack.Top().AddChildren(p.t.contentNode)
	case "assets":
		p.t.hasAssets = true
		p.stack.Top().AddChildren(&node.SsrAssets{BaseNode: p.pos()})
	case "form":
		return p.formTag(byName, attributes, selfClosing)
	case "input", "textarea", "select":
		return p.fieldTag(tag, byName, attributes)
	case "json":
		return p.jsonTag(attrs)
	default:
		return p.fail("E-GEN-004", "unknown tag <ssr:%s>; the ssr: tags are access, var, call, content, assets, form, input, select, textarea and json", tag)
	}
	return nil
}

// nesting returns what makes the current position conditional or part of a form: an element
// with ssr:if, ssr:else-if, ssr:else or ssr:for, or <ssr:form>. It returns "" at the top level.
func (p *parser) nesting() string {
	for _, n := range p.stack {
		if el, ok := n.(*node.HtmlElement); ok && p.wrappers[el] != "" {
			return "an element with ssr:" + p.wrappers[el]
		}
	}
	if p.form != nil {
		return "<ssr:form>"
	}
	return ""
}

// accessTag reads the template's one <ssr:access role="a,b" guard="true"/>. It renders nothing.
func (p *parser) accessTag(attrs []tagAttr) error {
	if in := p.nesting(); in != "" {
		return p.fail("E-GEN-031", "<ssr:access> is inside %s; the rule always applies, so it must not be nested", in)
	}
	if p.t.access != nil {
		return p.fail("E-GEN-031", "a second <ssr:access>; the first is at line %d", p.t.access.Line)
	}
	a := &Access{Line: p.line}
	role := ""
	for _, at := range attrs {
		switch at.name {
		case "role":
			role = at.value
		case "guard":
			if at.value != "true" {
				return p.fail("E-GEN-031", `<ssr:access> has guard=%q; write guard="true" or leave it out`, at.value)
			}
			a.Guard = true
		default:
			return p.fail("E-GEN-031", "<ssr:access> has the attribute %s", at.name)
		}
	}
	if strings.TrimSpace(role) == "" {
		return p.fail("E-GEN-031", "<ssr:access> has no role")
	}
	for r := range strings.SplitSeq(role, ",") {
		r = strings.TrimSpace(r)
		if r != "*" && !roleName.MatchString(r) {
			return p.fail("E-GEN-031", "role %q is not a lower-case name or *", r)
		}
		a.Roles = append(a.Roles, r)
	}
	if len(a.Roles) > 1 && slices.Contains(a.Roles, "*") {
		return p.fail("E-GEN-031", `role %q: "*" admits every viewer, so it stands alone`, role)
	}
	p.t.access = a
	return nil
}

// callTag reads an <ssr:call name="…" in="…" out="…"/>. It renders nothing.
func (p *parser) callTag(attrs []tagAttr) error {
	c := Call{Line: p.line}
	for _, a := range attrs {
		switch a.name {
		case "name":
			c.Name = a.value
		case "in":
			c.In = a.value
		case "out":
			c.Out = a.value
		default:
			return p.fail("E-GEN-040", "<ssr:call> has the attribute %s", a.name)
		}
	}
	switch {
	case c.Name == "":
		return p.fail("E-GEN-040", "<ssr:call> has no name")
	case !callName.MatchString(c.Name):
		return p.fail("E-GEN-040", "<ssr:call> name %q is not a lower-case letter followed by letters and digits", c.Name)
	}
	if in := p.nesting(); in != "" {
		return p.fail("E-GEN-040", "<ssr:call name=%q> is inside %s; the page can always make the call, so it must not be nested", c.Name, in)
	}
	for _, other := range p.t.calls {
		if other.Name == c.Name {
			return p.fail("E-GEN-040", "a second <ssr:call name=%q>; the first is at line %d", c.Name, other.Line)
		}
	}
	var err error
	if c.In, err = p.callType(c.Name, "in", c.In); err != nil {
		return err
	}
	if c.Out, err = p.callType(c.Name, "out", c.Out); err != nil {
		return err
	}
	p.t.calls = append(p.t.calls, c)
	return nil
}

// callType checks the in or out type of the call named name and returns it as Go source.
func (p *parser) callType(name, attr, typ string) (string, error) {
	if strings.TrimSpace(typ) == "" {
		return "", p.fail("E-GEN-040", "<ssr:call name=%q> has no %s type", name, attr)
	}
	if notJSON(typ) {
		return "", p.fail("E-GEN-040", "<ssr:call name=%q> %s: %q cannot be sent as JSON", name, attr, typ)
	}
	t, problem := callGoType(typ)
	if problem != "" {
		return "", p.fail("E-GEN-045", "<ssr:call name=%q> %s: %s", name, attr, problem)
	}
	if used := usedName(t, callLocals); used != "" {
		return "", p.fail("E-GEN-042", "<ssr:call name=%q> %s: %s is a name the generated code uses", name, attr, used)
	}
	return t, nil
}

// jsonTag adds an <ssr:json name="…" value="…"/>.
func (p *parser) jsonTag(attrs []tagAttr) error {
	var name, value string
	for _, a := range attrs {
		switch a.name {
		case "name":
			name = a.value
		case "value":
			value = a.value
		default:
			return p.fail("E-GEN-033", "<ssr:json> has the attribute %s", a.name)
		}
	}
	switch {
	case !reJSONName.MatchString(name):
		return p.fail("E-GEN-033", "<ssr:json> name %q is not lower-case letters, digits and -, starting with a letter", name)
	case strings.TrimSpace(value) == "":
		return p.fail("E-GEN-033", "<ssr:json name=%q> has no value", name)
	case strings.Contains(value, "{{"):
		return p.fail("E-GEN-033", "<ssr:json name=%q>: write the value without {{ }}", name)
	case p.jsonLines[name] > 0:
		return p.fail("E-GEN-033", "a second <ssr:json name=%q>; the first is at line %d", name, p.jsonLines[name])
	}
	p.jsonLines[name] = p.line
	v, err := p.expr("value", value)
	if err != nil {
		return err
	}
	p.stack.Top().AddChildren(&node.SsrJSON{BaseNode: p.pos(), Name: name, Value: v})
	return nil
}

func (p *parser) formTag(byName map[string]string, attributes []node.HtmlAttribute, selfClosing bool) error {
	if p.form != nil {
		return p.fail("E-GEN-009", "<ssr:form> inside the <ssr:form> of line %d", p.form.Node.Line)
	}
	name := byName["name"]
	switch {
	case name == "":
		return p.fail("E-GEN-010", "<ssr:form> has no name")
	case !reName.MatchString(name):
		return p.fail("E-GEN-010", "form name %q is not made of letters, digits and _, starting with a letter", name)
	}
	for _, f := range p.t.forms {
		if strings.EqualFold(f.Name, name) {
			return p.fail("E-GEN-010", "the form of line %d is already named %q", f.Node.Line, f.Name)
		}
	}
	if enc := byName["enctype"]; enc != "" && enc != node.FormEncUrlEncoded && enc != node.FormEncTypeMultipart {
		return p.fail("E-GEN-010", "<ssr:form name=%q> has the enctype %q", name, enc)
	}

	formNode := &node.SsrForm{
		BaseNode:   p.pos(),
		Name:       name,
		EncType:    byName["enctype"],
		Attributes: attributes,
	}
	p.t.forms = append(p.t.forms, &Form{Name: name, Node: formNode})
	p.stack.Top().AddChildren(formNode)
	if !selfClosing {
		p.form = p.t.forms[len(p.t.forms)-1]
		p.stack.Push(formNode)
	}
	return nil
}

func (p *parser) fieldTag(tag string, byName map[string]string, attributes []node.HtmlAttribute) error {
	if p.form == nil {
		return p.fail("E-GEN-008", "<ssr:%s> is outside an <ssr:form>", tag)
	}
	if _, ok := byName["ssr:bind"]; ok {
		return p.fail("E-GEN-018", "ssr:bind on <ssr:%s>; form fields cannot be bound to live values", tag)
	}
	name := byName["name"]
	if !reName.MatchString(name) {
		return p.fail("E-GEN-014", "field name %q is not made of letters, digits and _, starting with a letter", name)
	}
	_, isRequired := byName["required"]
	_, isMultiple := byName["multiple"]
	goType := byName["gotype"]
	if goType == "" {
		goType = "string"
	}
	if _, ok := formElementGoTypes[goType]; !ok {
		return p.fail("E-GEN-012", "unknown gotype %q on <ssr:%s name=%q>", goType, tag, name)
	}

	var (
		n        node.Node
		nodeType FormElementType
	)
	switch tag {
	case "input":
		input := &node.SsrInput{
			BaseNode:   p.pos(),
			Name:       name,
			Type:       byName["type"],
			Value:      byName["value"],
			Attributes: attributes,
		}
		nodeType = FormElementInput
		switch input.Type {
		case "file":
			if p.form.Node.EncType == "" {
				p.form.Node.EncType = node.FormEncTypeMultipart
			}
			if p.form.Node.EncType != node.FormEncTypeMultipart {
				return p.fail("E-GEN-011", "<ssr:input name=%q type=\"file\"> in a form with the enctype %q", name, p.form.Node.EncType)
			}
			nodeType = FormElementInputFile
		case "radio", "checkbox":
			lit, ok := goLiteral(goType, input.Value)
			if !ok {
				return p.fail("E-GEN-012", "value %q is not a %s", input.Value, goType)
			}
			input.GoValue = lit
		}
		n = input
	case "textarea":
		n = &node.SsrTextarea{BaseNode: p.pos(), Name: name, Attributes: attributes}
		nodeType = FormElementTextarea
	case "select":
		n = &node.SsrSelect{BaseNode: p.pos(), Name: name, GoType: goType, Attributes: attributes, Multiple: isMultiple}
		nodeType = FormElementSelect
	}

	if err := p.addField(name, n, nodeType, goType, isRequired, isMultiple); err != nil {
		return err
	}
	p.stack.Top().AddChildren(n)
	return nil
}

// addField adds a field to the open form. Radio and checkbox inputs of one type may share
// a name; they become one field.
func (p *parser) addField(name string, n node.Node, nodeType FormElementType, goType string, isRequired, isMultiple bool) error {
	var existing *FormElement
	for _, e := range p.form.Elements {
		if e.Name == name {
			existing = e
			break
		}
	}
	if existing == nil {
		p.form.Elements = append(p.form.Elements, &FormElement{
			Name:       name,
			Nodes:      []node.Node{n},
			Type:       nodeType,
			IsRequired: isRequired,
			GoType:     goType,
			IsMultiple: isMultiple,
		})
		return nil
	}

	input, _ := n.(*node.SsrInput)
	first, _ := existing.Nodes[0].(*node.SsrInput)
	if input == nil || first == nil || input.Type != first.Type || (input.Type != "checkbox" && input.Type != "radio") {
		return p.fail("E-GEN-014", "form %q has more than one field named %q", p.form.Name, name)
	}
	if goType != existing.GoType {
		return p.fail("E-GEN-013", "inputs named %q have the gotypes %s and %s", name, existing.GoType, goType)
	}
	if input.Type == "checkbox" {
		existing.IsMultiple = true
		for _, e := range existing.Nodes {
			e.(*node.SsrInput).Multiple = true
		}
		input.Multiple = true
	}
	existing.Nodes = append(existing.Nodes, n)
	return nil
}

// goLiteral returns the fixed value of a radio or checkbox input as a Go literal of goType,
// in the form the form fields parse it, and false when it is not a value of that type.
func goLiteral(goType, value string) (string, bool) {
	bits := formElementGoTypes[goType]
	switch goType {
	case "string":
		return strconv.Quote(value), true
	case "bool":
		b, err := strconv.ParseBool(value)
		return strconv.FormatBool(b), err == nil
	case "int", "int8", "int16", "int32", "int64":
		i, err := strconv.ParseInt(value, 10, bits)
		return strconv.FormatInt(i, 10), err == nil
	case "uint", "uint8", "uint16", "uint32", "uint64":
		u, err := strconv.ParseUint(value, 10, bits)
		return strconv.FormatUint(u, 10), err == nil
	case "float32", "float64":
		f, err := strconv.ParseFloat(value, bits)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		return strconv.FormatFloat(f, 'g', -1, bits), true
	}
	return "", false
}

func (p *parser) element(tag string, attrs []tagAttr, selfClosing bool) error {
	switch {
	case tag == "style":
		return p.fail("E-GEN-021", "a <style> element")
	case selfClosing && rawTags[tag]:
		return p.fail("E-GEN-001", "<%[1]s/> does not end the element in HTML; write <%[1]s …></%[1]s>", tag)
	}
	n := &node.HtmlElement{
		BaseNode:   p.pos(),
		TagName:    tag,
		SelfClosed: selfClosing,
	}
	wrapped, bound := false, false
	wrapper := ""
	for _, a := range attrs {
		if name, ok := strings.CutPrefix(a.name, "ssr:"); ok {
			bound = bound || name == "bind"
			if wrappers[name] {
				if err := p.oneWrapper(wrapper, name); err != nil {
					return err
				}
				wrapper = name
			}
			w, err := p.directive(n, name, a.value)
			if err != nil {
				return err
			}
			wrapped = wrapped || w
			continue
		}
		if a.name == "src" && tag == "img" {
			src, err := p.image(a.value)
			if err != nil {
				return p.imageErr(err)
			}
			a.value = src
		}
		values, err := p.attr(tag, a)
		if err != nil {
			return err
		}
		n.Attributes = append(n.Attributes, node.HtmlAttribute{Key: a.name, Values: values})
	}
	if bound && tag == "input" {
		if err := p.checkBoundInput(n); err != nil {
			return err
		}
	}

	if !wrapped {
		p.stack.Top().AddChildren(n)
	}
	if !selfClosing && !htmlutils.VoidElements[tag] {
		if wrapper != "" {
			p.wrappers[n] = wrapper
		}
		p.stack.Push(n)
	}
	return nil
}

// checkBoundInput refuses ssr:bind on an <input> whose value a script cannot set: a file input,
// or one whose type is a value and so may be file.
func (p *parser) checkBoundInput(n *node.HtmlElement) error {
	for _, a := range n.Attributes {
		if a.Key != "type" {
			continue
		}
		for _, v := range a.Values {
			if _, ok := v.(*node.Text); !ok {
				return p.fail("E-GEN-018", "ssr:bind on an <input> whose type is a value; write a fixed type")
			}
		}
		if strings.EqualFold(strings.Trim(literal(a.Values), htmlSpace), "file") {
			return p.fail("E-GEN-018", `ssr:bind on <input type="file">; a script cannot set the file an input holds`)
		}
	}
	return nil
}

// oneWrapper refuses a second loop or condition on one element, which would add it twice.
func (p *parser) oneWrapper(first, second string) error {
	switch {
	case first == "":
		return nil
	case first == "for":
		return p.fail("E-GEN-007", "ssr:for cannot share an element with ssr:%s; wrap one of them in another element", second)
	case second == "for":
		return p.fail("E-GEN-007", "ssr:for cannot share an element with ssr:%s; wrap one of them in another element", first)
	}
	return p.fail("E-GEN-006", "ssr:%s and ssr:%s cannot share an element", first, second)
}

// directive applies an ssr: attribute of element n. It reports whether n was added to the
// page inside a loop or a condition.
func (p *parser) directive(n *node.HtmlElement, name, value string) (bool, error) {
	switch name {
	case "bind":
		if !bindable[n.TagName] {
			return false, p.fail("E-GEN-018", "ssr:bind on <%s>; only a native <input>, <select> or <textarea> can be bound", n.TagName)
		}
		p.t.ssrBindRefs = append(p.t.ssrBindRefs, SsrBindRef{File: p.display, Line: p.line, VarName: value, Element: n})
		return false, nil
	case "for":
		nodes, err := p.parseText(value, true)
		if err != nil {
			return false, err
		}
		loop, ok := nodes[0].(*node.Loop)
		if !ok || len(nodes) != 1 {
			return false, p.fail("E-GEN-007", "ssr:for=%q is not a loop", value)
		}
		loop.Children = []node.Node{n}
		p.stack.Top().AddChildren(loop)
		return true, nil
	case "if":
		cond, err := p.expr("ssr:"+name, value)
		if err != nil {
			return false, err
		}
		p.stack.Top().AddChildren(&node.SsrCondition{
			BaseNode:   p.pos(),
			Conditions: []node.SsrConditionData{{BaseNode: p.pos(), Condition: cond, Body: n}},
		})
		return true, nil
	case "else", "else-if":
		chain := p.openCondition()
		if chain == nil {
			return false, p.fail("E-GEN-006", "ssr:%s does not follow an element with ssr:if or ssr:else-if", name)
		}
		if name == "else" {
			chain.ElseBody = n
			return true, nil
		}
		cond, err := p.expr("ssr:"+name, value)
		if err != nil {
			return false, err
		}
		chain.Conditions = append(chain.Conditions, node.SsrConditionData{BaseNode: p.pos(), Condition: cond, Body: n})
		return true, nil
	}
	return false, p.fail("E-GEN-005", "unknown attribute ssr:%s", name)
}

// expr parses the expression in the attribute attr: ssr:if, ssr:else-if or the value of
// <ssr:json>.
func (p *parser) expr(attr, value string) (node.Node, error) {
	nodes, err := p.parseText(value, true)
	if err != nil {
		return nil, err
	}
	if _, loop := nodes[0].(*node.Loop); loop || len(nodes) != 1 {
		return nil, p.fail("E-GEN-015", "%s=%q is not an expression", attr, value)
	}
	return nodes[0], nil
}

// openCondition returns the condition chain an ssr:else or ssr:else-if continues: the last
// child of the open element, past blank text, when it is a chain without ssr:else.
func (p *parser) openCondition() *node.SsrCondition {
	top := p.stack.Top()
	for {
		t, ok := top.LastChild().(*node.Text)
		if !ok || strings.TrimSpace(t.Text) != "" {
			break
		}
		top.PopChild()
	}
	chain, ok := top.LastChild().(*node.SsrCondition)
	if !ok || chain.ElseBody != nil {
		return nil
	}
	return chain
}

func (p *parser) endTag() error {
	name, _ := p.tok.TagName()
	tag := string(name)
	if strings.HasPrefix(tag, "ssr:") && tag != "ssr:form" {
		return nil
	}
	for i := len(p.stack) - 1; i > 0; i-- {
		if tagOf(p.stack[i]) != tag {
			continue
		}
		for _, n := range p.stack[i:] {
			if _, ok := n.(*node.SsrForm); ok {
				p.form = nil
			}
		}
		p.stack = p.stack[:i]
		return nil
	}
	return p.fail("E-GEN-001", "</%s> closes no open element", tag)
}

func tagOf(n node.WithChildren) string {
	switch n := n.(type) {
	case *node.HtmlElement:
		return n.TagName
	case *node.SsrForm:
		return "ssr:form"
	}
	return ""
}

// imageErr places an error of the image resolver at the <img> tag.
func (p *parser) imageErr(err error) error {
	var e *errs.Error
	if !errors.As(err, &e) {
		return fmt.Errorf("%s:%d: %w", p.display, p.line, err)
	}
	if e.Pos != "" {
		return e
	}
	placed := *e
	placed.Pos = fmt.Sprintf("%s:%d", p.display, p.line)
	return &placed
}

// parseText parses text at the current line and refuses reserved names in it.
func (p *parser) parseText(text string, insideExpr bool) ([]node.Node, error) {
	nodes, err := parseText(text, p.file, p.display, p.line, insideExpr)
	if err != nil {
		return nil, err
	}
	if err := p.checkNames(nodes...); err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetVariables returns the <ssr:var> declarations sorted by name.
func (t *Template) GetVariables() []Variable {
	sort.Slice(t.variables, func(i, j int) bool {
		return t.variables[i].Name < t.variables[j].Name
	})
	return t.variables
}

// GetForms returns the forms sorted by name.
func (t *Template) GetForms() []*Form {
	sort.Slice(t.forms, func(i, j int) bool {
		return t.forms[i].Name < t.forms[j].Name
	})
	return t.forms
}

// Calls returns the template's <ssr:call> declarations sorted by name.
func (t *Template) Calls() []Call {
	return slices.SortedFunc(slices.Values(t.calls), func(a, b Call) int { return strings.Compare(a.Name, b.Name) })
}

// Access returns the template's <ssr:access> rule, or nil when it has none.
func (t *Template) Access() *Access { return t.access }

// GetContentNode returns the <ssr:content/> node, or nil.
func (t *Template) GetContentNode() *node.SsrContent { return t.contentNode }

// HasAssets reports whether the template has <ssr:assets/>.
func (t *Template) HasAssets() bool { return t.hasAssets }

// GetSsrBindRefs returns the ssr:bind attributes of native inputs.
func (t *Template) GetSsrBindRefs() []SsrBindRef { return t.ssrBindRefs }

// MarkBoundElements gives every input with ssr:bind the markers the browser runtime reads:
// data-ssr-bind="<routeKey>.<variable>" and data-ssr-write.
func (t *Template) MarkBoundElements(routeKey string) {
	for _, ref := range t.ssrBindRefs {
		ref.Element.Attributes = append(ref.Element.Attributes,
			node.HtmlAttribute{Key: "data-ssr-bind", Values: []node.Node{&node.Text{Text: namespacedKey(routeKey, ref.VarName)}}},
			node.HtmlAttribute{Key: "data-ssr-write"})
	}
}

// GetNodes returns the top-level nodes.
func (t *Template) GetNodes() []node.Node { return t.nodes }

// CollectAllVarRefs returns the reactive variables the template refers to anywhere.
func (t *Template) CollectAllVarRefs(reactive map[string]bool) []string {
	sets := make([][]string, len(t.nodes))
	for i, n := range t.nodes {
		sets[i] = n.CollectVarRefs(reactive)
	}
	return node.UnionRefs(sets...)
}

// WriteGoCode writes the Go code that renders the template.
func (t *Template) WriteGoCode(buf *gobuf.GoBuf) {
	for _, n := range t.nodes {
		n.WriteGoCode(buf)
	}
}

// CollectReactiveNodes returns every live site by its binding or block key. It needs
// AnnotateBindings to have run.
func (t *Template) CollectReactiveNodes() map[string]node.Node {
	result := make(map[string]node.Node)
	collectReactiveNodesIn(t.nodes, result)
	return result
}

func collectReactiveNodesIn(nodes []node.Node, result map[string]node.Node) {
	for _, n := range nodes {
		collectReactiveNode(n, result)
	}
}

func collectReactiveNode(n node.Node, result map[string]node.Node) {
	switch v := n.(type) {
	case *node.Expression:
		if v.BindingKey != "" {
			result[v.BindingKey] = v
		}
	case *node.RawExpression:
		if v.BindingKey != "" {
			result[v.BindingKey] = v
		}
	case *node.SsrCondition:
		if v.BlockKey != "" {
			result[v.BlockKey] = v
		}
		for _, c := range v.Conditions {
			collectReactiveNode(c.Body, result)
		}
		if v.ElseBody != nil {
			collectReactiveNode(v.ElseBody, result)
		}
	case *node.Loop:
		if v.BlockKey != "" {
			result[v.BlockKey] = v
		}
		collectReactiveNodesIn(v.Children, result)
	case *node.HtmlElement:
		if v.BlockKey != "" {
			result[v.BlockKey] = v
		}
		collectReactiveNodesIn(v.Children, result)
	case *node.Content:
		collectReactiveNodesIn(v.Children, result)
	}
}

// blockNodeKey returns the 16-hex key of a live block: a hash of file, line, node type and
// the block's ordinal in the template, so blocks on one line get different keys.
func blockNodeKey(file string, line int, nodeType string, ordinal int) string {
	input := file + ":" + strconv.Itoa(line) + ":" + nodeType + ":" + strconv.Itoa(ordinal)
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum[:8]) // 8 bytes = 16 hex chars
}

// AnnotateBindings marks the live sites of the template: a {{ }} or {{$ }} that refers to a
// reactive variable gets a BindingKey; a condition, a loop or an element with a live attribute
// gets a BlockKey. A site inside a live block is not marked, because the block re-renders it.
// keyer returns the local key of a value from the variables it refers to and its source; the
// source of a {{$ }} value starts with "$", so it never shares the key of a {{ }} value.
// Keys are prefixed with routeKeyPrefix and a dot unless the prefix is empty. AnnotateBindings
// returns the variables of every key.
func (t *Template) AnnotateBindings(reactive map[string]bool, keyer func(refs []string, exprSrc string) string, routeKeyPrefix string) map[string][]string {
	bindings := make(map[string][]string)
	counter := 0
	annotateBindingsNodes(t.nodes, reactive, keyer, routeKeyPrefix, bindings, false, &counter)
	return bindings
}

func annotateBindingsNodes(nodes []node.Node, reactive map[string]bool, keyer func([]string, string) string, routeKeyPrefix string, bindings map[string][]string, insideBlock bool, counter *int) {
	for _, n := range nodes {
		annotateBindingsNode(n, reactive, keyer, routeKeyPrefix, bindings, insideBlock, counter)
	}
}

func namespacedKey(routeKeyPrefix, localKey string) string {
	if routeKeyPrefix == "" {
		return localKey
	}
	return routeKeyPrefix + "." + localKey
}

func annotateBindingsNode(n node.Node, reactive map[string]bool, keyer func([]string, string) string, routeKeyPrefix string, bindings map[string][]string, insideBlock bool, counter *int) {
	switch v := n.(type) {
	case *node.Expression:
		if insideBlock {
			return
		}
		if refs := v.CollectVarRefs(reactive); len(refs) > 0 {
			nsKey := namespacedKey(routeKeyPrefix, keyer(refs, v.Source))
			v.BindingKey = nsKey
			bindings[nsKey] = refs
		}
	case *node.RawExpression:
		if insideBlock {
			return
		}
		if refs := v.CollectVarRefs(reactive); len(refs) > 0 {
			nsKey := namespacedKey(routeKeyPrefix, keyer(refs, "$"+v.Source))
			v.BindingKey = nsKey
			bindings[nsKey] = refs
		}
	case *node.HtmlElement:
		// A live attribute cannot be patched alone, so the whole element re-renders.
		if !insideBlock {
			if attrRefs := v.CollectAttributeVarRefs(reactive); len(attrRefs) > 0 {
				*counter++
				nsKey := namespacedKey(routeKeyPrefix, blockNodeKey(v.File, v.Line, "HtmlElement", *counter))
				v.BlockKey = nsKey
				bindings[nsKey] = v.CollectVarRefs(reactive)
				annotateBindingsNodes(v.Children, reactive, keyer, routeKeyPrefix, bindings, true, counter)
				return
			}
		}
		annotateBindingsNodes(v.Children, reactive, keyer, routeKeyPrefix, bindings, insideBlock, counter)
	case *node.Content:
		annotateBindingsNodes(v.Children, reactive, keyer, routeKeyPrefix, bindings, insideBlock, counter)
	case *node.SsrCondition:
		inner := insideBlock
		if refs := v.CollectVarRefs(reactive); len(refs) > 0 && !insideBlock {
			*counter++
			nsKey := namespacedKey(routeKeyPrefix, blockNodeKey(v.File, v.Line, "SsrCondition", *counter))
			v.BlockKey = nsKey
			bindings[nsKey] = refs
			inner = true
		}
		for _, c := range v.Conditions {
			annotateBindingsNode(c.Body, reactive, keyer, routeKeyPrefix, bindings, inner, counter)
		}
		if v.ElseBody != nil {
			annotateBindingsNode(v.ElseBody, reactive, keyer, routeKeyPrefix, bindings, inner, counter)
		}
	case *node.Loop:
		inner := insideBlock
		if refs := v.CollectVarRefs(reactive); len(refs) > 0 && !insideBlock {
			*counter++
			nsKey := namespacedKey(routeKeyPrefix, blockNodeKey(v.File, v.Line, "Loop", *counter))
			v.BlockKey = nsKey
			bindings[nsKey] = refs
			inner = true
		}
		annotateBindingsNodes(v.Children, reactive, keyer, routeKeyPrefix, bindings, inner, counter)
	}
}

// parseText parses text with {{ }} values, or an expression or a loop when insideExpr is set.
// file names node positions and display error positions; line is the line text starts on.
func parseText(text, file, display string, line int, insideExpr bool) ([]node.Node, error) {
	lexer := &exprLex{text: text, file: file, display: display, curLine: line, insideExpr: insideExpr}
	yyParse(lexer)
	if lexer.err != nil {
		return nil, lexer.err
	}
	idx := 0
	assignExprSources(lexer.result.Children, lexer.exprSources, &idx)
	return lexer.result.Children, nil
}

// assignExprSources gives every value its source, in the order the lexer read them.
func assignExprSources(nodes []node.Node, sources []string, idx *int) {
	for _, n := range nodes {
		switch v := n.(type) {
		case *node.Expression:
			if *idx < len(sources) {
				v.Source = sources[*idx]
				*idx++
			}
		case *node.RawExpression:
			if *idx < len(sources) {
				v.Source = sources[*idx]
				*idx++
			}
		case *node.Content:
			assignExprSources(v.Children, sources, idx)
		case *node.HtmlElement:
			assignExprSources(v.Children, sources, idx)
		}
	}
}
