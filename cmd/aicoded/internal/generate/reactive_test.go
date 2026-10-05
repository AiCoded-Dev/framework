package generate

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

// liveCode returns the route_gen.go of an app whose only page is tpl, open to every viewer and
// with <ssr:assets/>.
func liveCode(t *testing.T, tpl string) string {
	t.Helper()
	return routeCode(t, `<ssr:access role="*"/><ssr:assets/>`+tpl)
}

// discoverErr returns the error of finding the routes of an app made of files.
func discoverErr(t *testing.T, files map[string]string) error {
	t.Helper()
	_, err := discover(newApp(t, files), noImages)
	return err
}

// body returns the declaration of the function or method whose signature starts with sig.
func body(t *testing.T, code, sig string) string {
	t.Helper()
	i := strings.Index(code, sig)
	require.GreaterOrEqual(t, i, 0, "no %s in\n%s", sig, code)
	end := strings.Index(code[i:], "\n}\n")
	require.GreaterOrEqual(t, end, 0)
	return code[i : i+end+3]
}

func TestBindingKey(t *testing.T) {
	sum := sha256.Sum256([]byte("a + b"))
	assert.Equal(t, "count", bindingKey([]string{"count"}, "count"))
	assert.Equal(t, hex.EncodeToString(sum[:8]), bindingKey([]string{"a", "b"}, "a + b"))
	assert.NotEqual(t, bindingKey([]string{"a", "b"}, "a + b"), bindingKey([]string{"a", "b"}, "a - b"))
	assert.Len(t, bindingKey([]string{"count"}, "count + 1"), 16, "a value that is not the bare variable has its own key")
	assert.Len(t, bindingKey([]string{"user"}, "user.Name"), 16)
}

func TestTextSite(t *testing.T) {
	code := liveCode(t, `<ssr:var name="count" type="int" reactive="true"/><p>{{ count }}</p>`)
	assert.Contains(t, code, `"<span data-ssr-bind=\"8a5edab2.count\">"`)
	assert.Contains(t, code, "if err := render.Text(w, count); err != nil {")
	assert.Contains(t, code, "func renderBlock_count(_ context.Context, data *RouteData) reactive.Binding {\n\treturn reactive.TextBinding(render.Format(data.Count))\n}")
	assert.Contains(t, code, `routeKey + ".count": renderBlock_count(ctx, &s.RouteData),`)
}

func TestCompositeSites(t *testing.T) {
	code := liveCode(t, `<ssr:var name="a" type="int" reactive="true"/><ssr:var name="b" type="int" reactive="true"/>
<p>{{ a + b }}</p><p>{{ a - b }}</p><p>{{ a * 2 }}</p><p>{{ a }}</p>`)
	assert.Equal(t, 4, strings.Count(code, "func renderBlock_"), "one per distinct value")
	assert.Equal(t, 2, strings.Count(body(t, code, "func (rs *ReactiveState) SetB("), "rs.conn.Enqueue("))
	assert.Equal(t, 4, strings.Count(body(t, code, "func (rs *ReactiveState) SetA("), "rs.conn.Enqueue("))
	sum := sha256.Sum256([]byte("a * 2"))
	fn := body(t, code, "func renderBlock_"+hex.EncodeToString(sum[:8])+"(")
	assert.Contains(t, fn, "a := data.A\n")
	assert.Contains(t, fn, "return reactive.TextBinding(render.Format(a * 2))")
}

func TestRawSite(t *testing.T) {
	code := liveCode(t, `<ssr:var name="note" type="HTML" reactive="true"/><div>{{$ note }}</div>`)
	assert.Contains(t, code, `"<span data-ssr-bind=\"8a5edab2.`)
	assert.NotContains(t, code, "renderBlock_note(", "the raw site does not share the key of a text site")
	assert.Regexp(t, `if err := render\.HTML\(w, note\); err != nil \{\n\s+return err\n\s+\}\n\s+return nil\n\s+\}\(&b\); err != nil \{`, code,
		"the value goes through render.HTML, which takes only web.SafeHTML")
}

func TestKeysAreNamespacedByRoute(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html":   doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="count" type="int" reactive="true"/><p>{{ count }}</p><ssr:content/>`),
		"pages/a/index.html": `<ssr:var name="count" type="int" reactive="true"/><p>{{ count }}</p>`,
	})
	assert.Contains(t, string(g.files["pages/route_gen.go"]), `data-ssr-bind=\"`+routeKey("/")+`.count\"`)
	assert.Contains(t, string(g.files["pages/a/route_gen.go"]), `data-ssr-bind=\"`+routeKey("/a")+`.count\"`)
	assert.NotEqual(t, routeKey("/"), routeKey("/a"))
}

func TestBlocks(t *testing.T) {
	for tpl, wrapper := range map[string]string{
		`<ssr:var name="flag" type="bool" reactive="true"/><div ssr:if="flag"><p>On</p></div>`:                                 "<ssr-block",
		`<ssr:var name="items" type="[]string" reactive="true"/><ul ssr:for="x in items"><li>{{ x }}</li></ul>`:                "<ssr-block",
		`<ssr:var name="c" type="string" reactive="true"/><p class="{{ c }}">x</p>`:                                            "<ssr-block",
		`<ssr:var name="items" type="[]string" reactive="true"/><table><tr ssr:for="x in items"><td>{{ x }}</td></tr></table>`: `"<tbody`,
	} {
		code := liveCode(t, tpl)
		assert.Contains(t, code, wrapper+` data-ssr-bind=\"8a5edab2.`, tpl)
		assert.NotContains(t, code, "<span data-ssr-bind", tpl)
		assert.Equal(t, 1, strings.Count(code, "func renderBlock_"), tpl)
		assert.Regexp(t, `func renderBlock_[0-9a-f]{16}\(ctx context\.Context, data \*RouteData\) reactive\.Binding \{`, code, tpl)
		assert.Regexp(t, `\}\(&b\); err != nil \{\n\s+web\.LogError\(ctx, "live block not rendered", err, "route", routeKey, "block", "[0-9a-f]{16}"\)\n`+
			`\s+return reactive\.HTMLBinding\(""\)\n\s+\}\n\s+return reactive\.HTMLBinding\(b\.String\(\)\)`, code, "a block that fails is blank and logged: %s", tpl)
	}
	assert.Contains(t, liveCode(t, `<ssr:var name="items" type="[]string" reactive="true"/><table><tr ssr:for="x in items"><td>{{ x }}</td></tr></table>`),
		`</tbody></table>`)

	code := liveCode(t, `<ssr:var name="flag" type="bool" reactive="true"/><ssr:var name="count" type="int" reactive="true"/>
<div ssr:if="flag"><ssr:json name="c" value="count"/></div>`)
	assert.Regexp(t, `count := data\.Count\n(.*\n)?\s+flag := data\.Flag\n`, body(t, code, "func renderBlock_"), "the block reads every variable it shows")
	assert.Equal(t, 1, strings.Count(body(t, code, "func (rs *ReactiveState) SetCount("), "Enqueue("))
}

func TestInnerSitesAreSuppressed(t *testing.T) {
	for _, tpl := range []string{
		`<ssr:var name="flag" type="bool" reactive="true"/><div ssr:if="flag">{{ flag }}</div>`,
		`<ssr:var name="c" type="string" reactive="true"/><p class="{{ c }}">{{ c }}</p>`,
		`<ssr:var name="items" type="[]string" reactive="true"/><ssr:var name="counter" type="int" reactive="true"/>
<ul ssr:for="x in items"><li>{{ counter }}</li></ul>`,
		`<ssr:var name="outer" type="bool" reactive="true"/><ssr:var name="inner" type="bool" reactive="true"/>
<div ssr:if="outer"><div ssr:if="inner"><p>Both</p></div></div>`,
	} {
		code := liveCode(t, tpl)
		assert.NotContains(t, code, "<span data-ssr-bind", tpl)
		assert.Equal(t, 1, strings.Count(code, "<ssr-block data-ssr-bind"), tpl)
		assert.Equal(t, 1, strings.Count(code, "func renderBlock_"), tpl)
	}

	code := liveCode(t, `<ssr:var name="outer" type="bool" reactive="true"/><ssr:var name="inner" type="bool" reactive="true"/>
<div ssr:if="outer"><div ssr:if="inner"><p>Both</p></div></div>`)
	enqueue := regexp.MustCompile(`rs\.conn\.Enqueue\(.*\n`)
	inner := enqueue.FindAllString(body(t, code, "func (rs *ReactiveState) SetInner("), -1)
	assert.Len(t, inner, 1)
	assert.Equal(t, inner, enqueue.FindAllString(body(t, code, "func (rs *ReactiveState) SetOuter("), -1), "both re-render the outer block")
}

func TestSnapshot(t *testing.T) {
	code := liveCode(t, `<ssr:var name="count" type="int" reactive="true"/><ssr:var name="flag" type="bool" reactive="true"/>
<ssr:var name="n" type="int" reactive="true" client-writable="true"/><ssr:var name="p" type="Profile" reactive="true" client-writable="true"/>
<p>{{ count }}</p><div ssr:if="flag"><p>Visible</p></div>`)
	snap := body(t, code, "func (s *state) Snapshot(ctx context.Context) map[string]reactive.Binding {")
	assert.Equal(t, 3, strings.Count(snap, "renderBlock_"), snap)
	assert.Regexp(t, `routeKey \+ "\.count":\s+renderBlock_count\(ctx, &s\.RouteData\),`, snap)
	assert.Regexp(t, `routeKey \+ "\.n":\s+renderBlock_n\(ctx, &s\.RouteData\),`, snap, "a client-writable scalar always has a binding")
	assert.NotContains(t, snap, `".p"`, "a variable the template does not show is not sent as text")
	assert.Contains(t, snap, "s.mu.Lock()\n\tdefer s.mu.Unlock()")
}

func TestSet(t *testing.T) {
	code := liveCode(t, `<ssr:var name="count" type="int" reactive="true"/><ssr:var name="flag" type="bool" reactive="true"/>
<p>{{ count }}</p><div ssr:if="flag"><p>{{ count }}</p></div>`)
	set := body(t, code, "func (rs *ReactiveState) SetCount(v int) {")
	assert.Regexp(t, `(?s)rs\.s\.mu\.Lock\(\)\n\s+defer rs\.s\.mu\.Unlock\(\)\n\s+rs\.s\.RouteData\.Count = v\n`+
		`\s+rs\.conn\.Enqueue\(routeKey\+"\.[0-9a-f]{16}", renderBlock_[0-9a-f]{16}\(rs\.ctx, &rs\.s\.RouteData\)\)\n`+
		`\s+rs\.conn\.Enqueue\(routeKey\+"\.count", renderBlock_count\(rs\.ctx, &rs\.s\.RouteData\)\)\n\}`, set, "sorted keys")
	assert.Equal(t, 1, strings.Count(body(t, code, "func (rs *ReactiveState) SetFlag(v bool) {"), "Enqueue("))
}

func TestVariableTypes(t *testing.T) {
	for typ, ts := range map[string]string{
		"User":           "user: User;",
		"[]string":       "user: string[] | null;",
		"map[string]int": "user: Record<string, number> | null;",
		"*int":           "user: number | null;",
		"[]*int":         "user: (number | null)[] | null;",
	} {
		g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="user" type="` + typ + `" reactive="true"/><p>{{ user }}</p>`)})
		assert.Contains(t, string(g.files["pages/route_gen.go"]), "func (rs *ReactiveState) SetUser(v "+typ+") {", typ)
		assert.Contains(t, string(g.files["pages/reactive_gen.ts"]), "  "+ts+"\n", typ)
	}
}

func TestHandleWrite(t *testing.T) {
	code := liveCode(t, `<ssr:var name="n" type="int" reactive="true" client-writable="true"/>
<ssr:var name="tags" type="[]string" reactive="true" client-writable="true"/><ssr:var name="shown" type="int" reactive="true"/>`)
	hw := body(t, code, "func (s *state) HandleWrite(")
	assert.Contains(t, hw, "case \"n\":\n\t\tv, err := reactive.Decode[int](msg.Value)")
	assert.Contains(t, hw, "case \"tags\":\n\t\tv, err := reactive.Decode[[]string](msg.Value)")
	assert.NotContains(t, hw, `"shown"`, "a variable without client-writable cannot be written")
	assert.Regexp(t, `(?s)if v, err = s\.dp\.ValidateN\(ctx, r, v\); err != nil \{\n\s+s\.refuseWrite\(ctx, conn, msg, "n", err\)\n\s+return\n\s+\}\n`+
		`\s+\(&ReactiveState\{ctx: ctx, s: s, conn: conn\}\)\.SetN\(v\)\n\s+conn\.Ack\(ctx, msg\)`, hw, "the value is stored only after it is valid")
	assert.Contains(t, hw, "default:\n\t\tconn.Reject(ctx, msg, \"unknown variable\", reactive.CodeValidation)")
	assert.Contains(t, code, "\tValidateN(ctx context.Context, r *web.Request, val int) (int, error)\n")
	assert.Contains(t, code, "\tValidateTags(ctx context.Context, r *web.Request, val []string) ([]string, error)\n")
	assert.NotContains(t, code, "ValidateShown")
	assert.NotContains(t, code, "err.Error()", "an error's own text never reaches the page")
	assert.Contains(t, code, `func (s *state) refuseWrite(ctx context.Context, conn *reactive.Conn, msg reactive.WriteMsg, name string, err error) {
	var he *web.HTTPError
	if errors.As(err, &he) {
		conn.Reject(ctx, msg, he.Message, reactive.CodeValidation)
		return
	}
	web.LogError(ctx, "live value refused", err, "route", routeKey, "var", name)
	conn.Reject(ctx, msg, "The value was not accepted.", reactive.CodeValidation)
}`)

	code = liveCode(t, `<ssr:var name="n" type="int" reactive="true"/>`)
	assert.Contains(t, body(t, code, "func (s *state) HandleWrite("), "{\n\tconn.Reject(ctx, msg, \"unknown variable\", reactive.CodeValidation)\n}")
	assert.NotContains(t, code, "refuseWrite")
}

func TestBoundInput(t *testing.T) {
	code := liveCode(t, `<ssr:var name="q" type="string" reactive="true" client-writable="true"/><input ssr:bind="q" type="text">{{ q }}`)
	assert.Contains(t, code, `"<input type=\"text\" data-ssr-bind=\"8a5edab2.q\" data-ssr-write>"`)
	assert.NotContains(t, code, "ssr:bind")
}

func TestNonLiveRoute(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:var name="title" type="string"/><h1>{{ title }}</h1>`)})
	code := string(g.files["pages/route_gen.go"])
	for _, s := range []string{"ReactiveState", "reactive", "sync", "Snapshot", "Subscribe", "data-ssr-bind"} {
		assert.NotContains(t, code, s)
	}
	assert.NotContains(t, g.files, "pages/reactive_gen.ts")
	assert.False(t, g.live("/"))
}

func TestLiveHooks(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/>
<ssr:var name="n" type="int" reactive="true" client-writable="true"/><ssr:var name="m" type="int" reactive="true"/>`)})
	code := string(g.files["pages/route_gen.go"])
	assert.Contains(t, code, "\t// Subscribe runs while the page is open and must return when ctx is done.\n"+
		"\tSubscribe(ctx context.Context, r *web.Request, state *ReactiveState) error\n")
	assert.Contains(t, code, "func (s *state) Subscribe(ctx context.Context, r *web.Request, conn *reactive.Conn) error {\n"+
		"\treturn s.dp.Subscribe(ctx, r, &ReactiveState{ctx: ctx, s: s, conn: conn})\n}")
	assert.Contains(t, code, "type state struct {\n\tweb.Frame\n\tmu        sync.Mutex\n")
	assert.True(t, g.live("/"))

	stub := string(g.stubs["pages/dataprovider.go"])
	assert.Contains(t, stub, "import (\n\t\"context\"\n\t\"errors\"\n")
	assert.Contains(t, stub, "// Subscribe sends new live values with state.Set… while the page is open. It must return\n"+
		"// when ctx is done; until it returns, the viewer's live connection stays taken.\n"+
		"func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {\n\treturn nil\n}")
	assert.Contains(t, stub, "// ValidateN refuses every value the page writes to n until it checks the value and returns the one to store.\n"+
		"func (p *DP) ValidateN(ctx context.Context, r *web.Request, val int) (int, error) {\n"+
		"\treturn val, errors.New(\"this value cannot be changed yet\")\n}")
	assert.NotContains(t, stub, "ValidateM")
}

func TestLiveRouteCompiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/live/index.ts"), []byte("// mine\n"), 0o600))
	require.NoError(t, Generate(dir))
	goVet(t, dir)
	mine, err := os.ReadFile(filepath.Join(dir, "pages/live/index.ts"))
	require.NoError(t, err)
	assert.Equal(t, "// mine\n", string(mine), "the page's own script is never touched")

	code, err := os.ReadFile(filepath.Join(dir, "pages/live/route_gen.go"))
	require.NoError(t, err)
	golden(t, code, "live_route_gen.go.golden")
	for _, want := range []string{
		`reactive.TextBinding(render.Format(data.Count))`,
		`reactive.HTMLBinding(`,
		`func (s *state) Snapshot(ctx context.Context) map[string]reactive.Binding`,
		`reactive.Decode[int](msg.Value)`,
		`s.dp.ValidateCount(ctx, r, v)`,
		`data-ssr-bind=\"`,
		`data-ssr-write`,
	} {
		assert.Contains(t, string(code), want)
	}
	assert.NotContains(t, string(code), "ssr:bind")

	ts, err := os.ReadFile(filepath.Join(dir, "pages/live/reactive_gen.ts"))
	require.NoError(t, err)
	assert.Contains(t, string(ts), "count: number;")
	assert.Contains(t, string(ts), "items: string[] | null;")
	assert.Contains(t, string(ts), `.aicodedReactive.route<ReadVars, WriteVars, Calls>("`)
	_, err = os.Stat(filepath.Join(dir, "pages/notes/reactive_gen.ts"))
	assert.ErrorIs(t, err, fs.ErrNotExist, "only live routes get the client")
}

// TestLiveSitesRun builds live sites that read variables named like the locals of the
// generated code, a {{$ }} value and a value in a block, and runs a block whose value cannot
// be written: it is sent blank and logged.
func TestLiveSitesRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/empty/types.go"), []byte(
		"package empty\n\nimport \"aicoded.dev/framework/web\"\n\n// HTML is markup shown as it is.\ntype HTML = web.SafeHTML\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/empty/index.html"), []byte(`<ssr:access role="*"/>
<ssr:var name="data" type="int" reactive="true" client-writable="true"/>
<ssr:var name="b" type="int" reactive="true"/>
<ssr:var name="err" type="string" reactive="true"/>
<ssr:var name="note" type="HTML" reactive="true"/>
<ssr:var name="flag" type="bool" reactive="true"/>
<ssr:var name="ratio" type="float64" reactive="true"/>
<p>{{ data + b }}</p>
<div ssr:if="flag"><p>{{ data }} {{ err }}</p><ssr:json name="d" value="data"/></div>
<div ssr:if="ratio > 0"><ssr:json name="r" value="ratio"/></div>
<div>{{$ note }}</div>
<input ssr:bind="data" type="number">`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/empty/block_test.go"), []byte(blockTest), 0o600))
	require.NoError(t, Generate(dir))
	goVet(t, dir)
	assert.Contains(t, goTool(t, dir, "test"), "ok  \texample.com/app/pages/empty")
}

// blockTest checks in the generated app that a block whose JSON cannot be written is sent
// blank, and that the log names the route, the key and the kind of the error but not the values.
const blockTest = `package empty

import (
	"log/slog"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/web/reactive"
)

func TestFailedBlockIsBlankAndLogged(t *testing.T) {
	var logs strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(prev)

	s := NewRoute(NewDP(nil)).NewState().(*state)
	s.RouteData.Ratio = math.Inf(1)
	s.RouteData.Data = 4242
	snap := s.Snapshot(t.Context())

	m := regexp.MustCompile("msg=\"live block not rendered\" route=" + routeKey + " block=([0-9a-f]{16}) err=\\*json\\.UnsupportedValueError\n").FindStringSubmatch(logs.String())
	require.NotNil(t, m, logs.String())
	assert.Equal(t, reactive.HTMLBinding(""), snap[routeKey+"."+m[1]])
	assert.Equal(t, 1, strings.Count(logs.String(), "level="), "only the failed block is logged")
	assert.NotContains(t, logs.String(), "4242")
	assert.NotContains(t, logs.String(), "+Inf", "with no runner, the error's text is not logged")
}
`

func TestLiveTS(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/>
<ssr:var name="count" type="int" reactive="true" client-writable="true"/><ssr:var name="p" type="Profile" reactive="true"/>
<ssr:var name="plain" type="int"/>`)})
	ts := string(g.files["pages/reactive_gen.ts"])
	assert.True(t, strings.HasPrefix(ts, "// Code generated by aicoded. DO NOT EDIT.\n// Live variables and calls of the page /.\n\n"))
	assert.Contains(t, ts, "export interface Profile {\n  [key: string]: unknown;\n}\n\n")
	assert.Contains(t, ts, "export type ReadVars = {\n  count: number;\n  p: Profile;\n};\n\nexport type WriteVars = {\n  count: number;\n};\n")
	assert.NotContains(t, ts, "plain")
	assert.True(t, strings.HasSuffix(ts, `.aicodedReactive.route<ReadVars, WriteVars, Calls>("8a5edab2");`+"\n"))

	ts = string(built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:assets/><ssr:var name="m" type="map[bool]string" reactive="true"/>`)}).
		files["pages/reactive_gen.ts"])
	assert.Contains(t, ts, "export type ReadVars = {\n  // map key type \"bool\" is not a string or an integer")
	assert.Contains(t, ts, "  m: Record<string, unknown> | null;\n")
	assert.Contains(t, ts, "export type WriteVars = {};\n")
}

func TestReactiveClientIsRemoved(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	require.FileExists(t, filepath.Join(dir, "pages/live/reactive_gen.ts"))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/live/index.html"), []byte(`<ssr:access role="*"/><p>still</p>`), 0o600))
	require.NoError(t, Generate(dir))
	assert.NoFileExists(t, filepath.Join(dir, "pages/live/reactive_gen.ts"))
}

func TestLiveValueInTitleIsRefused(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/live/index.html"), []byte(
		`<ssr:access role="*"/><ssr:var name="n" type="int" reactive="true"/><title>{{ n }}</title>`), 0o600))
	assert.Equal(t, "E-GEN-034", errs.Code(Generate(dir)))
}

func TestLiveChecks(t *testing.T) {
	for tpl, want := range map[string]string{
		`<ssr:var name="c" type="int" reactive="true"/><input ssr:bind="c">`:        `E-GEN-017 ssr:bind="c": c is not declared with reactive="true" client-writable="true"`,
		`<ssr:var name="c" type="int" client-writable="true"/><input ssr:bind="c">`: `E-GEN-017 ssr:bind="c": c is not declared with reactive="true" client-writable="true"`,
		`<input ssr:bind="c">`: `E-GEN-017 ssr:bind="c": this template declares no variable c`,
		`<input ssr:bind="">`:  `E-GEN-017 ssr:bind is empty; name a variable of this template`,
		`<ssr:var name="p" type="Profile" reactive="true" client-writable="true"/><input ssr:bind="p">`:                              `E-GEN-019 ssr:bind="p": p has the type Profile; only a string, bool or number can be bound`,
		`<ssr:var name="p" type="[]string" reactive="true" client-writable="true"/><input ssr:bind="p">`:                             "E-GEN-019",
		`<ssr:var name="p" type="UserID" reactive="true" client-writable="true"/><input ssr:bind="p">`:                               "E-GEN-019",
		`<ssr:var name="p" type="complex64" reactive="true" client-writable="true"/><input ssr:bind="p">`:                            "E-GEN-019",
		`<ssr:var name="p" type="uintptr" reactive="true" client-writable="true"/><input ssr:bind="p">`:                              "E-GEN-019",
		`<ssr:var name="n" type="int" reactive="true"/><title>{{ n }}</title>`:                                                       `E-GEN-034 a live value inside <title>`,
		`<ssr:var name="n" type="int" reactive="true"/><textarea>{{ n + 1 }}</textarea>`:                                             `E-GEN-034 a live value inside <textarea>`,
		`<ssr:var name="n" type="bool" reactive="true"/><div ssr:if="n"><textarea>{{ n }}</textarea></div>`:                          "E-GEN-034",
		`<ssr:var name="n" type="bool" reactive="true"/><div ssr:if="n"><ssr:content/></div>`:                                        `E-GEN-046 <ssr:content/> is inside a part of the page that changes with a live value (line 1)`,
		`<ssr:var name="n" type="string" reactive="true"/><body class="{{ n }}"><ssr:assets/></body>`:                                "E-GEN-046",
		`<ssr:var name="n" type="[]int" reactive="true"/><div ssr:for="x in n"><ssr:form name="f"></ssr:form></div>`:                 "E-GEN-046",
		`<ssr:var name="n" type="bool" reactive="true"/><ssr:form name="f"><p>{{ n }}</p></ssr:form>`:                                `E-GEN-048 <ssr:form name="f"> shows the live value n`,
		`<ssr:var name="n" type="string" reactive="true"/><ssr:form name="f"><ssr:input name="t" placeholder="{{ n }}"/></ssr:form>`: "E-GEN-048",
		`<ssr:var name="n" type="int" reactive="true"/><ssr:form name="f" action="/x/{{ n }}"></ssr:form>`:                           "E-GEN-048",
		`<ssr:var name="n" type="bool" reactive="true"/><ssr:form name="f"><div><p ssr:if="n">on</p></div></ssr:form>`:               "E-GEN-048",
		`<ssr:var name="n" type="[]int" reactive="true"/><ssr:form name="f"><p ssr:for="x in n">{{ x }}</p></ssr:form>`:              "E-GEN-048",
	} {
		err := discoverErr(t, map[string]string{"pages/index.html": `<ssr:access role="*"/>` + tpl})
		var e *errs.Error
		require.ErrorAs(t, err, &e, tpl)
		assert.Equal(t, "pages/index.html:1", e.Pos, tpl)
		assert.True(t, strings.HasPrefix(e.Code+" "+e.Msg, want), "%s\n%s %s", tpl, e.Code, e.Msg)
	}
	for _, typ := range []string{"string", "bool", "int", "int8", "uint64", "byte", "rune", "float32", "float64"} {
		tpl := `<ssr:access role="*"/><ssr:var name="v" type="` + typ + `" reactive="true" client-writable="true"/><input ssr:bind="v">`
		require.NoError(t, discoverErr(t, map[string]string{"pages/index.html": tpl}), typ)
	}
	for _, tpl := range []string{
		`<ssr:var name="n" type="int"/><title>{{ n }}</title>`,
		`<ssr:var name="n" type="bool" reactive="true"/><div ssr:if="n"><ssr:json name="n" value="n"/></div>`,
		`<ssr:var name="m" type="int"/><ssr:var name="n" type="int" reactive="true"/><ssr:form name="f"><p>{{ m }}</p></ssr:form>`,
		`<ssr:var name="q" type="string" reactive="true" client-writable="true"/><ssr:form name="f"><input ssr:bind="q"></ssr:form>`,
	} {
		assert.NoError(t, discoverErr(t, map[string]string{"pages/index.html": `<ssr:access role="*"/>` + tpl}), tpl)
	}
}

func TestBindOnlyLocalVariables(t *testing.T) {
	err := discoverErr(t, map[string]string{
		"pages/index.html":        `<ssr:access role="*"/><ssr:var name="visits" type="int" reactive="true" client-writable="true"/><ssr:content/>`,
		"pages/detail/index.html": `<input ssr:bind="visits">`,
	})
	require.EqualError(t, err, `pages/detail/index.html:1: E-GEN-017: ssr:bind="visits": visits is declared in the page /; bind only variables of this template`+
		"\n  fix: "+`declare the variable in this template with reactive="true" client-writable="true"`+
		"\n  docs: https://aicoded.dev/docs/errors/E-GEN-017")

	assert.NoError(t, discoverErr(t, map[string]string{
		"pages/index.html":        `<ssr:access role="*"/><ssr:var name="visits" type="int" reactive="true" client-writable="true"/><ssr:content/>`,
		"pages/detail/index.html": `<ssr:var name="score" type="int" reactive="true" client-writable="true"/><input ssr:bind="score" type="number">`,
	}))
}

func TestRouteKeysAreUnique(t *testing.T) {
	require.Equal(t, routeKey("/p39801"), routeKey("/p82371"))
	err := discoverErr(t, map[string]string{
		"pages/index.html":        `<ssr:access role="*"/><ssr:content/>`,
		"pages/p39801/index.html": `<p>a</p>`,
		"pages/p82371/index.html": `<p>b</p>`,
	})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-GEN-047", e.Code)
	assert.Equal(t, "pages/p82371/index.html:1", e.Pos)
	assert.Equal(t, "/p39801 and /p82371 have the same route key 6ed3be29", e.Msg)
}

func TestGoTypeToTSType(t *testing.T) {
	for goType, want := range map[string]string{
		"string":            "string",
		"bool":              "boolean",
		"int":               "number",
		"uint8":             "number",
		"uintptr":           "number",
		"rune":              "number",
		"float64":           "number",
		"complex128":        "unknown",
		"*int":              "number | null",
		"[]string":          "string[] | null",
		"[][]int":           "(number[] | null)[] | null",
		"[]byte":            "string | null",
		"*[]uint8":          "string | null",
		"map[string]int":    "Record<string, number> | null",
		"map[int]string":    "Record<string, string> | null",
		"map[float64]int":   "Record<string, unknown> | null",
		"map[bool]string":   "Record<string, unknown> | null",
		"[]*int":            "(number | null)[] | null",
		"[][]*bool":         "((boolean | null)[] | null)[] | null",
		"*[]*int":           "(number | null)[] | null",
		"[]*Profile":        "(Profile | null)[] | null",
		"map[string]*int":   "Record<string, number | null> | null",
		"map[string][]*int": "Record<string, (number | null)[] | null> | null",
		"[]map[string]*int": "(Record<string, number | null> | null)[] | null",
		"time.Time":         "string",
		"Profile":           "Profile",
		"[]Profile":         "Profile[] | null",
		"pkg.Type":          "unknown",
		"func(string) bool": "unknown",
		"[3]int":            "unknown",
		"Route":             "unknown",
		"any":               "unknown",
		"class":             "unknown",
	} {
		registry := map[string]string{}
		got, _ := goTypeToTSType(goType, registry, map[string]bool{})
		assert.Equal(t, want, got, goType)
	}

	registry := map[string]string{}
	_, warning := goTypeToTSType("map[bool]string", registry, map[string]bool{})
	assert.Equal(t, `map key type "bool" is not a string or an integer. Falling back to Record<string, unknown>.`, warning)
	_, warning = goTypeToTSType("Node", registry, map[string]bool{"Node": true})
	assert.Equal(t, `cyclic type "Node" substituted with unknown in TypeScript interface`, warning)
	_, warning = goTypeToTSType("[]Profile", registry, map[string]bool{})
	assert.Empty(t, warning)
	assert.Equal(t, map[string]string{"Profile": "// Profile: opaque struct type. Add a module augmentation for field-level typing.\n" +
		"export interface Profile {\n  [key: string]: unknown;\n}"}, registry)
}
