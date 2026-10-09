package generate

import (
	"flag"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// copyApp copies testdata/<name> into a temp dir and points its replace directive at this
// framework checkout.
func copyApp(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", name)
	dst := t.TempDir()
	require.NoError(t, os.CopyFS(dst, os.DirFS(src)))
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	gomod := filepath.Join(dst, "go.mod")
	data, err := os.ReadFile(gomod)
	require.NoError(t, err)
	data = regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`).
		ReplaceAll(data, []byte("replace aicoded.dev/framework => "+root))
	require.NoError(t, os.WriteFile(gomod, data, 0o600))
	return dst
}

// goVet builds and vets the generated app in dir.
func goVet(t *testing.T, dir string) {
	t.Helper()
	goTool(t, dir, "vet")
}

// goTool runs the go command on every package of the app in dir and returns its output.
func goTool(t *testing.T, dir, command string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", command, "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func golden(t *testing.T, got []byte, name string) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		require.NoError(t, os.WriteFile(p, got, 0o600))
	}
	want, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestGenerateCompiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))

	for _, p := range []string{"pages/route_gen.go", "pages/handler_gen.go", "pages/assets_gen.go",
		"pages/notes/route_gen.go", "pages/notes/n_id/route_gen.go", "pages/empty/route_gen.go", "pages/empty/dataprovider.go"} {
		st, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err, p)
		assert.Equal(t, fs.FileMode(0o644), st.Mode().Perm(), p)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "pages/notes/dataprovider.go"))
	require.NoError(t, err)
	orig, err := os.ReadFile("testdata/app/pages/notes/dataprovider.go")
	require.NoError(t, err)
	assert.Equal(t, orig, kept, "an existing data provider is never rewritten")

	goVet(t, dir)

	route, err := os.ReadFile(filepath.Join(dir, "pages/notes/n_id/route_gen.go"))
	require.NoError(t, err)
	golden(t, route, "notes_id_route_gen.go.golden")
	handler, err := os.ReadFile(filepath.Join(dir, "pages/handler_gen.go"))
	require.NoError(t, err)
	golden(t, handler, "handler_gen.go.golden")

	require.NoError(t, Generate(dir))
	again, err := os.ReadFile(filepath.Join(dir, "pages/notes/n_id/route_gen.go"))
	require.NoError(t, err)
	assert.Equal(t, route, again, "generation is deterministic")
}

// TestGeneratedPagesRun runs the fixture app's own tests against the generated code: a form
// is processed only when it is valid, a Guard the route does not declare stops the app, a live
// page stores only valid values of its client-writable variables, and a page call runs only
// with the arguments it declares.
func TestGeneratedPagesRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	out := goTool(t, dir, "test")
	assert.Contains(t, out, "ok  \texample.com/app/pages/notes/n_id", "./... includes the parameter folder")
	assert.Contains(t, out, "ok  \texample.com/app/pages/live")
	assert.Contains(t, out, "ok  \texample.com/app/pages/tools")
}

// TestStubsCompile removes the data providers of the fixture: the stubs written in their place
// implement every hook, so the app builds, and the stub Guard refuses every viewer.
func TestStubsCompile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	for _, p := range []string{"pages/notes/dataprovider.go", "pages/notes/n_id/dataprovider.go", "pages/live/dataprovider.go", "pages/tools/dataprovider.go"} {
		require.NoError(t, os.Remove(filepath.Join(dir, p)))
	}
	require.NoError(t, Generate(dir))
	goVet(t, dir)
	stub, err := os.ReadFile(filepath.Join(dir, "pages/notes/n_id/dataprovider.go"))
	require.NoError(t, err)
	assert.Contains(t, string(stub), "func (p *DP) Guard(ctx context.Context, r *web.Request) error {\n\treturn web.Forbidden()\n}")
}

// vetPages generates the fixture app with the extra templates in pages, by path, and vets it.
func vetPages(t *testing.T, pages map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := copyApp(t, "app")
	for p, tpl := range pages {
		write(t, dir, p, tpl)
	}
	require.NoError(t, Generate(dir))
	goVet(t, dir)
}

// A variable the template never reads, or only binds to an input, still builds.
func TestUnreadVarsCompile(t *testing.T) {
	vetPages(t, map[string]string{
		"pages/unread/index.html": `<ssr:access role="*"/><ssr:var name="unused" type="int"/><p>x</p>`,
		"pages/search/index.html": `<ssr:access role="*"/>
<ssr:var name="query" type="string" reactive="true" client-writable="true"/>
<input type="search" ssr:bind="query">`,
	})
}

// A variable may have a type of package web, such as the markup {{$ }} writes.
func TestWebVarTypesCompile(t *testing.T) {
	code := routeCode(t, `<ssr:access role="*"/><ssr:var name="intro" type="web.SafeHTML"/><div>{{$ intro }}</div>`)
	assert.Contains(t, code, "\tIntro web.SafeHTML\n")
	assert.Contains(t, code, "render.HTML(w, intro)")
	vetPages(t, map[string]string{
		"pages/intro/index.html": `<ssr:access role="*"/><ssr:var name="intro" type="web.SafeHTML"/><div>{{$ intro }}</div>`,
	})
}

func TestGenerateReportsAllDiagnostics(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/notes/index.html"), []byte(`<script>x()</script>`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages/empty/index.html"), []byte(`<p style="x"></p>`), 0o600))
	err := Generate(dir)
	var d Diagnostics
	require.ErrorAs(t, err, &d)
	assert.Len(t, d, 2)
	assert.Equal(t, "E-GEN-023", errs.Code(err), "sorted by position: pages/empty before pages/notes")
	_, statErr := os.Stat(filepath.Join(dir, "pages/handler_gen.go"))
	assert.ErrorIs(t, statErr, fs.ErrNotExist, "nothing is written when generation fails")
}

func TestGenerateRemovesStaleCode(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	require.NoError(t, os.Remove(filepath.Join(dir, "pages/empty/index.html")))
	require.NoError(t, Generate(dir))
	_, err := os.Stat(filepath.Join(dir, "pages/empty/route_gen.go"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestGenerateWithoutPages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o600))
	assert.NoError(t, Generate(dir))
}

func TestGenerateNeverWritesThroughLinks(t *testing.T) {
	dir := copyApp(t, "app")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0o600))
	for _, p := range []string{"pages/handler_gen.go", "pages/empty/dataprovider.go"} {
		if err := os.Symlink(outside, filepath.Join(dir, p)); err != nil {
			t.Skip("cannot create symbolic links:", err)
		}
	}

	require.NoError(t, Generate(dir))
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "keep", string(data))
	st, err := os.Lstat(filepath.Join(dir, "pages/handler_gen.go"))
	require.NoError(t, err)
	assert.True(t, st.Mode().IsRegular(), "the link is replaced by the generated file")
}

// built writes an app made of files and returns its generator after a successful build.
func built(t *testing.T, files map[string]string) *gen {
	t.Helper()
	a := newApp(t, files)
	g := newGen(a)
	routes, err := discover(a, g.assets.image)
	require.NoError(t, err)
	require.NoError(t, g.build(routes))
	return g
}

// doc wraps tpl in <!doctype html><html>…</html>, as a page needs.
func doc(tpl string) string { return "<!doctype html><html>" + tpl + "</html>" }

// routeCode returns the route_gen.go of an app whose only page is tpl in a document.
func routeCode(t *testing.T, tpl string) string {
	t.Helper()
	return string(built(t, map[string]string{"pages/index.html": doc(tpl)}).files["pages/route_gen.go"])
}

func TestRouteAccess(t *testing.T) {
	g := built(t, map[string]string{
		"pages/index.html":   doc(`<ssr:access role="*"/><ssr:content/>`),
		"pages/a/index.html": `<p>a</p>`,
		"pages/b/index.html": `<ssr:access role="x, y"/>`,
	})
	assert.Contains(t, string(g.files["pages/route_gen.go"]), `return web.Access{Roles: []string{"*"}} }`)
	assert.Contains(t, string(g.files["pages/a/route_gen.go"]), "return web.Access{} }")
	assert.Contains(t, string(g.files["pages/b/route_gen.go"]), `return web.Access{Roles: []string{"x", "y"}} }`)
}

func TestRouteGuard(t *testing.T) {
	code := routeCode(t, `<ssr:access role="*" guard="true"/>`)
	assert.Contains(t, code, "\tGuard(ctx context.Context, r *web.Request) error\n")
	assert.Contains(t, code, "func (s *state) Guard(ctx context.Context, r *web.Request) error { return s.dp.Guard(ctx, r) }")
	assert.NotContains(t, code, "CheckNoGuard")

	code = routeCode(t, `<ssr:access role="*"/>`)
	assert.NotContains(t, code, "\tGuard(")
	assert.Contains(t, code, "func (s *state) Guard(context.Context, *web.Request) error { return nil }")
	assert.Contains(t, code, "func NewRoute(dp RouteDataProvider) web.Route {\n\tweb.CheckNoGuard(dp, \"/\")\n")
}

func TestRouteDefault(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/><ssr:content default="/x"/>`), "pages/x/index.html": `<p>x</p>`})
	code := string(g.files["pages/route_gen.go"])
	assert.Contains(t, code, `func (s *state) DefaultRoute(context.Context, *web.Request) (string, error) { return "/x", nil }`)
	assert.NotContains(t, code, "\tDefaultRoute(")

	code = routeCode(t, `<ssr:access role="*"/><ssr:content/>`)
	assert.Contains(t, code, "\tDefaultRoute(ctx context.Context, r *web.Request) (string, error)\n")
	assert.Contains(t, code, "(string, error) {\n\treturn s.dp.DefaultRoute(ctx, r)\n}")

	code = routeCode(t, `<ssr:access role="*"/>`)
	assert.Contains(t, code, `func (s *state) DefaultRoute(context.Context, *web.Request) (string, error) { return "", nil }`)
}

func TestRouteForms(t *testing.T) {
	code := routeCode(t, `<ssr:access role="*"/>`)
	assert.Contains(t, code, "func (s *state) InitForms(context.Context, *web.Request, web.ResponseWriter) error { return nil }")
	assert.Contains(t, code, "SubmitForm(context.Context, *web.Request, web.ResponseWriter, string) (bool, error) {\n\treturn false, nil\n}")

	code = routeCode(t, `<ssr:access role="*"/>
<ssr:form name="up"><ssr:input name="doc" type="file" required/></ssr:form>
<ssr:form name="note"><ssr:textarea name="text"/></ssr:form>`)
	assert.Contains(t, code, "if !form.IsMultipart(r.Request) {")
	assert.Contains(t, code, `s.FormUp.Doc.Process(r.Request, "doc", true, true)`)
	assert.Contains(t, code, "if form.IsMultipart(r.Request) {")
	assert.Contains(t, code, `s.FormNote.Text.Process(r.Request, "text", false, false)`)
	assert.Regexp(t, `(?s)s\.FormNote\.MarkValidated\(\)\n\s+if s\.FormNote\.HasError\(\) \{\n\s+return true, nil\n\s+\}\n\s+return true, s\.dp\.ProcessNote\(`, code,
		"Process runs only after the fields are valid")
}

func TestRouteImports(t *testing.T) {
	const web, form, render = `"aicoded.dev/framework/web"`, `"aicoded.dev/framework/web/form"`, `"aicoded.dev/framework/web/render"`
	const reactive = `"aicoded.dev/framework/web/reactive"`
	for tpl, want := range map[string][]string{
		`<p>text</p>`: {`"context"`, `"io"`, web},
		`<ssr:var name="x" type="int"/><p>{{ x }}</p>`:                          {`"context"`, `"io"`, web, render},
		`<ssr:var name="x" type="bool"/><p ssr:if="x ? true : false">a</p>`:     {`"context"`, `"io"`, web, render},
		`<ssr:form name="f"></ssr:form>`:                                        {`"context"`, `"io"`, `"net/http"`, web, form, render},
		`<ssr:var name="x" type="bool" reactive="true"/><p ssr:if="x">a</p>`:    {`"context"`, `"io"`, `"strings"`, `"sync"`, web, reactive},
		`<ssr:var name="x" type="int" reactive="true" client-writable="true"/>`: {`"context"`, `"errors"`, `"io"`, `"sync"`, web, reactive, render},
		`<ssr:var name="x" type="int" reactive="true"/><p>{{ x }}</p>`:          {`"context"`, `"io"`, `"sync"`, web, reactive, render},
		`<ssr:call name="a" in="int" out="int"/>`:                               {`"context"`, `"encoding/json"`, `"io"`, `"net/http"`, web},
	} {
		code := routeCode(t, `<ssr:access role="*"/><ssr:assets/>`+tpl)
		f, err := parser.ParseFile(token.NewFileSet(), "route_gen.go", code, parser.ImportsOnly)
		require.NoError(t, err, tpl)
		var got []string
		for _, imp := range f.Imports {
			got = append(got, imp.Path.Value)
		}
		assert.Equal(t, want, got, tpl)
	}
}

// After the code that //line directives map to the page, a //line directive maps route_gen.go
// back to its own lines: the end of Write and of each live block, and the constants, so the go
// command and the checks never point past the page's end.
func TestRouteLinesAfterThePage(t *testing.T) {
	code := routeCode(t, `<ssr:access role="*"/><ssr:assets/>
<ssr:var name="n" type="int" reactive="true"/><ssr:var name="items" type="[]string" reactive="true"/>
<p>{{ n + 1 }}</p>
<ul ssr:if="len(items) > 0"><li ssr:for="it in items">{{ it }}</li></ul>
<p>end</p>`)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "route_gen.go", code, parser.ParseComments)
	require.NoError(t, err)
	at := func(p token.Position) string { return p.Filename + ":" + strconv.Itoa(p.Line) }
	file := fset.File(f.Pos())
	page, resets := false, 0
	for i, line := range strings.Split(strings.TrimSuffix(code, "\n"), "\n") {
		n := i + 1
		switch {
		case strings.HasPrefix(line, "//line index.html:"):
			page = true
		case strings.HasPrefix(line, "//line "):
			assert.Equal(t, "//line route_gen.go:"+strconv.Itoa(n+1), line)
			page, resets = false, resets+1
		case !page:
			assert.Equal(t, "route_gen.go:"+strconv.Itoa(n), at(fset.Position(file.LineStart(n))))
		}
	}
	assert.Equal(t, 3, resets, "in Write and in the two live blocks with page code")
	for _, d := range f.Decls {
		for _, pos := range []token.Pos{d.Pos(), d.End() - 1} {
			assert.Equal(t, at(fset.PositionFor(pos, false)), at(fset.Position(pos)))
		}
	}
}

func TestRouteAliases(t *testing.T) {
	var routes []*Route
	for _, p := range []string{"/", "/ab/c", "/abC", "/key", "/notes/n_id"} {
		routes = append(routes, &Route{Path: p})
	}
	assert.Equal(t, map[string]string{
		"/ab/c":       "routeAbC",
		"/abC":        "routeAbC2",
		"/key":        "routeKey2",
		"/notes/n_id": "routeNotesN_id",
	}, routeAliases(routes))
}

func TestStubHooks(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*" guard="true"/><ssr:content/>
<ssr:form name="note"><ssr:input name="title"/></ssr:form>`)})
	stub := string(g.stubs["pages/dataprovider.go"])
	for _, want := range []string{
		"func (p *DP) Guard(ctx context.Context, r *web.Request) error {\n\treturn web.Forbidden()\n}",
		"func (p *DP) Data(ctx context.Context, r *web.Request, w web.ResponseWriter, data *RouteData) error {\n\treturn nil\n}",
		"func (p *DP) DefaultRoute(ctx context.Context, r *web.Request) (string, error) {\n\treturn \"\", nil\n}",
		"// InitNote sets the starting values and options of the form note. It runs every time\n" +
			"// the page shows or receives the form, on a GET too, so it must not change data:\n" +
			"// ProcessNote does that.\n" +
			"func (p *DP) InitNote(ctx context.Context, r *web.Request, w web.ResponseWriter, data *FormNoteValues) error {\n\treturn nil\n}",
		"func (p *DP) ProcessNote(ctx context.Context, r *web.Request, w web.ResponseWriter, data *FormNoteValues) error {\n\treturn nil\n}",
	} {
		assert.Contains(t, stub, want)
	}
}

func TestStubWithoutDeps(t *testing.T) {
	g := built(t, map[string]string{"pages/index.html": doc(`<ssr:access role="*"/>`)})
	stub := string(g.stubs["pages/dataprovider.go"])
	assert.Contains(t, stub, "type DP struct{}\n")
	assert.Contains(t, stub, "func NewDP() *DP { return &DP{} }")
	assert.NotContains(t, stub, "deps")
	assert.Contains(t, string(g.files["pages/handler_gen.go"]), `"/": NewRoute(NewDP()),`)
	assert.Contains(t, string(g.files["pages/handler_gen.go"]), "func NewHandler() http.Handler {")
}

func TestRouteKey(t *testing.T) {
	assert.Contains(t, routeCode(t, `<ssr:access role="*"/>`), `const routeKey = "8a5edab2"`, `sha256("/") starts with 8a5edab2`)
}

func TestLayoutFlag(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, Generate(dir))
	for p, want := range map[string]bool{"pages": true, "pages/notes": true, "pages/notes/n_id": false, "pages/tools": false} {
		code, err := os.ReadFile(filepath.Join(dir, p, "route_gen.go"))
		require.NoError(t, err)
		assert.Contains(t, string(code), "func (rt *route) Layout() bool { return "+strconv.FormatBool(want)+" }", p)
	}
}
