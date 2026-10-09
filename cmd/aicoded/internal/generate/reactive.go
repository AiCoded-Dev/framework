package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

const reactivePkg = "aicoded.dev/framework/web/reactive"

// bindScalarTypes are the types an input with ssr:bind can write: the page sends the input's
// text, and reactive.Decode reads text only into these.
var bindScalarTypes = map[string]bool{
	"string": true, "bool": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"byte": true, "rune": true, "float32": true, "float64": true,
}

// bindingKey returns the local key of a live value: the name of the variable when the value is
// that variable alone, and otherwise the first 16 hex digits of the sha256 of its source.
func bindingKey(refs []string, src string) string {
	if len(refs) == 1 && src == refs[0] {
		return refs[0]
	}
	sum := sha256.Sum256([]byte(src))
	return hex.EncodeToString(sum[:8])
}

// livePage is the live part of a route: its live variables and the parts of the page that show
// them, by local key.
type livePage struct {
	vars     []template.Variable
	writable []template.Variable
	// keys holds the variables every key reads.
	keys map[string][]string
	// sites holds the node of every key the page shows. A client-writable variable the page
	// does not show has a key and no node.
	sites map[string]node.Node
}

// reactiveVars returns the variables declared with reactive="true".
func reactiveVars(vars []template.Variable) []template.Variable {
	var out []template.Variable
	for _, v := range vars {
		if v.Reactive {
			out = append(out, v)
		}
	}
	return out
}

// writableVars returns the variables the page may write: reactive="true" and
// client-writable="true".
func writableVars(vars []template.Variable) []template.Variable {
	var out []template.Variable
	for _, v := range vars {
		if v.Reactive && v.ClientWritable {
			out = append(out, v)
		}
	}
	return out
}

// checkLive checks the live parts of the route's template and marks them for generation; it
// sets the route's live field when the route has live variables. routes are the routes of the
// app, which name the page of a variable that ssr:bind cannot reach.
func checkLive(r *Route, routes map[string]*Route) Diagnostics {
	t := r.Template
	vars := t.GetVariables()
	var diags Diagnostics
	for _, ref := range t.GetSsrBindRefs() {
		if d := checkBind(ref, vars, r.Path, routes); d != nil {
			diags = append(diags, d)
		}
	}
	lv := reactiveVars(vars)
	if len(lv) == 0 {
		return diags
	}
	names := map[string]bool{}
	for _, v := range lv {
		names[v.Name] = true
	}
	file := templateFile(r.Path)
	walk(t.GetNodes(), func(n node.Node) {
		switch v := n.(type) {
		case *node.HtmlElement:
			for _, c := range v.Children {
				if e, ok := c.(*node.Expression); ok && e.RCDATA && len(e.CollectVarRefs(names)) > 0 {
					diags = append(diags, diag(at(file, e.Line), "E-GEN-034", "a live value inside <%s>", v.TagName))
				}
			}
		case *node.SsrForm:
			if line, refs := liveInForm(v, names); len(refs) > 0 {
				diags = append(diags, diag(at(file, line), "E-GEN-048",
					"<ssr:form name=%q> shows the live value %s, which the form cannot update", v.Name, strings.Join(refs, ", ")))
			}
		}
	})

	rk := routeKey(r.Path)
	bindings := t.AnnotateBindings(names, bindingKey, rk)
	sites := t.CollectReactiveNodes()
	for _, key := range slices.Sorted(maps.Keys(sites)) {
		block, line := blockLine(sites[key])
		if !block {
			continue
		}
		walk(children(sites[key]), func(n node.Node) {
			if tag, inner := stateNode(n); tag != "" {
				diags = append(diags, diag(at(file, inner), "E-GEN-046",
					"%s is inside a part of the page that changes with a live value (line %d)", tag, line))
			}
		})
	}
	t.MarkBoundElements(rk)

	out := &livePage{vars: lv, writable: writableVars(vars), keys: map[string][]string{}, sites: map[string]node.Node{}}
	for key, refs := range bindings {
		local := strings.TrimPrefix(key, rk+".")
		out.keys[local] = refs
		out.sites[local] = sites[key]
	}
	for _, v := range out.writable {
		if bindScalarTypes[v.Type] {
			out.keys[v.Name] = []string{v.Name}
		}
	}
	r.live = out
	return diags
}

// checkBind refuses an ssr:bind whose variable this template does not declare with
// reactive="true" client-writable="true", or whose type an input cannot write.
func checkBind(ref template.SsrBindRef, vars []template.Variable, routePath string, routes map[string]*Route) *errs.Error {
	pos := at(ref.File, ref.Line)
	if ref.VarName == "" {
		return diag(pos, "E-GEN-017", "ssr:bind is empty; name a variable of this template")
	}
	declares := func(v template.Variable) bool { return v.Name == ref.VarName }
	i := slices.IndexFunc(vars, declares)
	if i < 0 {
		for _, p := range slices.Sorted(maps.Keys(routes)) {
			if p != routePath && slices.ContainsFunc(routes[p].Template.GetVariables(), declares) {
				return diag(pos, "E-GEN-017", "ssr:bind=%q: %s is declared in the page %s; bind only variables of this template", ref.VarName, ref.VarName, p)
			}
		}
		return diag(pos, "E-GEN-017", "ssr:bind=%q: this template declares no variable %s", ref.VarName, ref.VarName)
	}
	v := vars[i]
	switch {
	case !v.Reactive || !v.ClientWritable:
		return diag(pos, "E-GEN-017", `ssr:bind=%q: %s is not declared with reactive="true" client-writable="true"`, v.Name, v.Name)
	case !bindScalarTypes[v.Type]:
		return diag(pos, "E-GEN-019", "ssr:bind=%q: %s has the type %s; only a string, bool or number can be bound", v.Name, v.Name, v.Type)
	}
	return nil
}

// liveInForm returns the live variables that form f shows, in its attributes, its text, its
// conditions, loops and <ssr:json>, and the attributes of its fields, with the line of the
// first place that shows one. A form is written from the page's state, so none of them would
// change.
func liveInForm(f *node.SsrForm, live map[string]bool) (int, []string) {
	line := 0
	var sets [][]string
	add := func(l int, refs []string) {
		if len(refs) > 0 && line == 0 {
			line = l
		}
		sets = append(sets, refs)
	}
	add(f.Line, attrRefs(f.Attributes, live))
	walk(f.Children, func(n node.Node) {
		switch v := n.(type) {
		case *node.Expression:
			add(v.Line, v.CollectVarRefs(live))
		case *node.RawExpression:
			add(v.Line, v.CollectVarRefs(live))
		case *node.SsrJSON:
			add(v.Line, v.CollectVarRefs(live))
		case *node.HtmlElement:
			add(v.Line, v.CollectAttributeVarRefs(live))
		case *node.Loop:
			add(v.Line, v.Array.CollectVarRefs(live))
		case *node.SsrCondition:
			for _, c := range v.Conditions {
				add(c.Line, c.Condition.CollectVarRefs(live))
			}
		case *node.SsrInput:
			add(v.Line, attrRefs(v.Attributes, live))
		case *node.SsrSelect:
			add(v.Line, attrRefs(v.Attributes, live))
		case *node.SsrTextarea:
			add(v.Line, attrRefs(v.Attributes, live))
		}
	})
	refs := node.UnionRefs(sets...)
	slices.Sort(refs)
	return line, refs
}

// attrRefs returns the variables of live that the attribute values read.
func attrRefs(attrs []node.HtmlAttribute, live map[string]bool) []string {
	var sets [][]string
	for _, a := range attrs {
		for _, v := range a.Values {
			sets = append(sets, v.CollectVarRefs(live))
		}
	}
	return node.UnionRefs(sets...)
}

// blockLine reports whether n is a live block, which re-renders whole, and its line.
func blockLine(n node.Node) (bool, int) {
	switch v := n.(type) {
	case *node.SsrCondition:
		return true, v.Line
	case *node.Loop:
		return true, v.Line
	case *node.HtmlElement:
		return true, v.Line
	}
	return false, 0
}

// stateNode returns the tag and line of a node that writes from the page's state rather than
// from its variables, so a live block cannot re-render it: a form, <ssr:content/> or
// <ssr:assets/>. It returns "" for any other node.
func stateNode(n node.Node) (string, int) {
	switch v := n.(type) {
	case *node.SsrForm:
		return "<ssr:form>", v.Line
	case *node.SsrContent:
		return "<ssr:content/>", v.Line
	case *node.SsrAssets:
		return "<ssr:assets/>", v.Line
	}
	return "", 0
}

// children returns the nodes directly inside n.
func children(n node.Node) []node.Node {
	switch v := n.(type) {
	case *node.HtmlElement:
		return v.Children
	case *node.Content:
		return v.Children
	case *node.SsrForm:
		return v.Children
	case *node.Loop:
		return v.Children
	case *node.SsrCondition:
		var out []node.Node
		for _, c := range v.Conditions {
			out = append(out, c.Body)
		}
		if v.ElseBody != nil {
			out = append(out, v.ElseBody)
		}
		return out
	}
	return nil
}

// walk calls fn for every node of nodes and of their children, parents first.
func walk(nodes []node.Node, fn func(node.Node)) {
	for _, n := range nodes {
		fn(n)
		walk(children(n), fn)
	}
}

func at(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) }

// html reports whether the value of key is markup the page's escapers write, not text.
func (lv *livePage) html(key string) bool {
	switch lv.sites[key].(type) {
	case nil, *node.Expression:
		return false
	}
	return true
}

// hasHTML and hasText report whether the value of any key is markup, or text.
func (lv *livePage) hasHTML() bool {
	return slices.ContainsFunc(slices.Collect(maps.Keys(lv.keys)), lv.html)
}

func (lv *livePage) hasText() bool {
	return slices.ContainsFunc(slices.Collect(maps.Keys(lv.keys)), func(k string) bool { return !lv.html(k) })
}

// writeLive writes the web.Reactive methods of state, and ReactiveState with a Set method for
// every live variable.
func writeLive(b *gobuf.GoBuf, lv *livePage) {
	keys := slices.Sorted(maps.Keys(lv.keys))

	b.WriteStringLn("func (s *state) Snapshot(ctx context.Context) map[string]reactive.Binding {")
	b.WriteStringLn("s.mu.Lock()")
	b.WriteStringLn("defer s.mu.Unlock()")
	b.WriteStringLn("return map[string]reactive.Binding{")
	for _, k := range keys {
		b.WriteString("routeKey + ")
		b.WriteQuotedString("."+k, ": renderBlock_"+k+"(ctx, &s.RouteData),\n")
	}
	b.WriteStringLn("}")
	b.WriteStringLn("}")
	b.WriteStringLn("")
	b.WriteStringLn("func (s *state) Subscribe(ctx context.Context, r *web.Request, conn *reactive.Conn) error {")
	b.WriteStringLn("return s.dp.Subscribe(ctx, r, &ReactiveState{ctx: ctx, s: s, conn: conn})")
	b.WriteStringLn("}")
	b.WriteStringLn("")
	writeHandleWrite(b, lv.writable)
	b.WriteStringLn("")

	b.WriteStringLn("// ReactiveState sends new values of the page's live variables to one open page. Its Set")
	b.WriteStringLn("// methods may be called from any goroutine.")
	b.WriteStringLn("type ReactiveState struct {")
	b.WriteStringLn("ctx context.Context")
	b.WriteStringLn("s *state")
	b.WriteStringLn("conn *reactive.Conn")
	b.WriteStringLn("}")
	for _, v := range lv.vars {
		b.WriteStringLn("")
		b.WriteStringLn("// Set" + exported(v.Name) + " stores v as " + v.Name + " and sends the parts of the page that show it.")
		b.WriteStringLn("func (rs *ReactiveState) Set" + exported(v.Name) + "(v " + v.Type + ") {")
		b.WriteStringLn("rs.s.mu.Lock()")
		b.WriteStringLn("defer rs.s.mu.Unlock()")
		b.WriteStringLn("rs.s.RouteData." + exported(v.Name) + " = v")
		for _, k := range keys {
			if slices.Contains(lv.keys[k], v.Name) {
				b.WriteString("rs.conn.Enqueue(routeKey+")
				b.WriteQuotedString("."+k, ", renderBlock_"+k+"(rs.ctx, &rs.s.RouteData))\n")
			}
		}
		b.WriteStringLn("}")
	}
	b.WriteStringLn("")
}

// writeHandleWrite writes HandleWrite, which decodes, validates and stores a value the page
// wrote to a client-writable variable, and refuses any other variable. A route with
// client-writable variables also gets refuseWrite.
func writeHandleWrite(b *gobuf.GoBuf, writable []template.Variable) {
	b.WriteStringLn("func (s *state) HandleWrite(ctx context.Context, r *web.Request, conn *reactive.Conn, msg reactive.WriteMsg) {")
	const unknown = `conn.Reject(ctx, msg, "unknown variable", reactive.CodeValidation)`
	if len(writable) == 0 {
		b.WriteStringLn(unknown)
		b.WriteStringLn("}")
		return
	}
	b.WriteStringLn("switch msg.Var {")
	for _, v := range writable {
		b.WriteString("case ")
		b.WriteQuotedString(v.Name, ":\n")
		b.WriteStringLn("v, err := reactive.Decode[" + v.Type + "](msg.Value)")
		b.WriteStringLn("if err != nil {")
		b.WriteStringLn(`conn.Reject(ctx, msg, "The value has the wrong type.", reactive.CodeDecode)`)
		b.WriteStringLn("return")
		b.WriteStringLn("}")
		b.WriteStringLn("if v, err = s.dp.Validate" + exported(v.Name) + "(ctx, r, v); err != nil {")
		b.WriteString("s.refuseWrite(ctx, conn, msg, ")
		b.WriteQuotedString(v.Name, ", err)\n")
		b.WriteStringLn("return")
		b.WriteStringLn("}")
		b.WriteStringLn("(&ReactiveState{ctx: ctx, s: s, conn: conn}).Set" + exported(v.Name) + "(v)")
		b.WriteStringLn("conn.Ack(ctx, msg)")
	}
	b.WriteStringLn("default:")
	b.WriteStringLn(unknown)
	b.WriteStringLn("}")
	b.WriteStringLn("}")
	b.WriteStringLn("")
	b.WriteStringLn("// refuseWrite answers a write that a Validate hook refused. The page shows the message of a")
	b.WriteStringLn("// web.Error; any other error is logged and the page gets a general message.")
	b.WriteStringLn("func (s *state) refuseWrite(ctx context.Context, conn *reactive.Conn, msg reactive.WriteMsg, name string, err error) {")
	b.WriteStringLn("var he *web.HTTPError")
	b.WriteStringLn("if errors.As(err, &he) {")
	b.WriteStringLn("conn.Reject(ctx, msg, he.Message, reactive.CodeValidation)")
	b.WriteStringLn("return")
	b.WriteStringLn("}")
	b.WriteStringLn(`web.LogError(ctx, "live value refused", err, "route", routeKey, "var", name)`)
	b.WriteStringLn(`conn.Reject(ctx, msg, "The value was not accepted.", reactive.CodeValidation)`)
	b.WriteStringLn("}")
}

// innerWriter is a live block or a {{$ }} value: it writes its code without its marker.
type innerWriter interface {
	WriteInnerGoCode(buf *gobuf.GoBuf)
}

// writeRenderBlocks writes renderBlock_<key> for every key: the current value of the key,
// written by the same code, and so the same escapers, as the page. A block whose code fails is
// sent blank, and the failure is logged with the route and the key.
func writeRenderBlocks(b *gobuf.GoBuf, lv *livePage, vars []template.Variable) {
	for _, k := range slices.Sorted(maps.Keys(lv.keys)) {
		b.WriteStringLn("")
		ctx := "_"
		if lv.html(k) {
			ctx = "ctx"
		}
		b.WriteStringLn("func renderBlock_" + k + "(" + ctx + " context.Context, data *RouteData) reactive.Binding {")
		switch site := lv.sites[k].(type) {
		case nil:
			b.WriteStringLn("return reactive.TextBinding(render.Format(data." + exported(k) + "))")
		case *node.Expression:
			if refs := lv.keys[k]; len(refs) == 1 && refs[0] == k {
				b.WriteStringLn("return reactive.TextBinding(render.Format(data." + exported(k) + "))")
				break
			}
			b.WriteStringLn("{")
			writeAliases(b, site, vars)
			b.WriteStringLn(site.FilePos())
			b.WriteString("return reactive.TextBinding(render.Format(")
			site.Value.WriteGoCode(b)
			b.WriteStringLn("))")
			b.WriteLineReset(routeFile)
			b.WriteStringLn("}")
		case innerWriter:
			b.WriteStringLn("var b strings.Builder")
			b.WriteStringLn("if err := func(w io.Writer) error {")
			writeAliases(b, lv.sites[k], vars)
			site.WriteInnerGoCode(b)
			b.WriteLineReset(routeFile)
			b.WriteStringLn("return nil")
			b.WriteStringLn("}(&b); err != nil {")
			b.WriteString(`web.LogError(ctx, "live block not rendered", err, "route", routeKey, "block", `)
			b.WriteQuotedString(k, ")\n")
			b.WriteStringLn(`return reactive.HTMLBinding("")`)
			b.WriteStringLn("}")
			b.WriteStringLn("return reactive.HTMLBinding(b.String())")
		default:
			panic(fmt.Sprintf("generate: live site %T", site))
		}
		b.WriteStringLn("}")
	}
	b.WriteStringLn("")
}

// writeAliases declares a local for every variable of the template that n reads, as Write
// does. A variable named data comes last, because until then data is the RouteData, and the
// locals go in a block of their own, where they may shadow data.
func writeAliases(b *gobuf.GoBuf, n node.Node, vars []template.Variable) {
	all := map[string]bool{}
	for _, v := range vars {
		all[v.Name] = true
	}
	refs := n.CollectVarRefs(all)
	slices.SortFunc(refs, func(x, y string) int {
		switch {
		case x == y:
			return 0
		case x == "data":
			return 1
		case y == "data":
			return -1
		}
		return strings.Compare(x, y)
	})
	for _, name := range refs {
		v := vars[slices.IndexFunc(vars, func(v template.Variable) bool { return v.Name == name })]
		b.WriteStringLn(v.FilePos())
		b.WriteStringLn(v.Name + " := data." + exported(v.Name))
	}
}
