package lint_test

import (
	"archive/zip"
	"bytes"
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

	"aicoded.dev/framework/docs"
	"aicoded.dev/framework/lint"
)

// run runs lint on the app testdata/<name>, with the files given as generated and this checkout
// as the framework's, and returns its problems as "<pos>: <code>: <message>".
func run(t *testing.T, name string, generated ...string) []string {
	t.Helper()
	return runWith(t, name, lint.Config{Generated: generated, FrameworkDir: checkout(t)})
}

// runWith runs lint on the app testdata/<name> with cfg, as run does.
func runWith(t *testing.T, name string, cfg lint.Config) []string {
	t.Helper()
	return runIn(t, filepath.Join("testdata", name), cfg)
}

// runIn runs lint on the app in dir with cfg, as run does.
func runIn(t *testing.T, dir string, cfg lint.Config) []string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	r, err := lint.Run(t.Context(), dir, cfg)
	require.NoError(t, err)
	var out []string
	for _, p := range r.Problems {
		code := rule(t, p.Code)
		assert.Contains(t, append([]string{code.Fix}, code.Also...), p.Fix)
		out = append(out, p.Pos+": "+p.Code+": "+p.Msg)
	}
	return out
}

// checkout returns this framework checkout, which the fixtures replace the framework with.
func checkout(t *testing.T) string {
	dir, err := filepath.Abs("..")
	require.NoError(t, err)
	return dir
}

// only returns the problems of lines that have the code.
func only(code string, lines []string) []string {
	var out []string
	for _, l := range lines {
		if strings.Contains(l, ": "+code+": ") {
			out = append(out, l)
		}
	}
	return out
}

// write writes data to the file rel in dir.
func write(t *testing.T, dir, rel, data string) {
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

// rule returns the rule with the code.
func rule(t *testing.T, code string) lint.Rule {
	for _, r := range lint.Rules() {
		if r.Code == code {
			return r
		}
	}
	t.Fatalf("no rule %s", code)
	return lint.Rule{}
}

func TestRules(t *testing.T) {
	rules := lint.Rules()
	require.NotEmpty(t, rules)
	for i, r := range rules {
		if i > 0 {
			assert.Less(t, rules[i-1].Code, r.Code, "sorted by code, each once")
		}
		assert.NotEmpty(t, r.Dont, r.Code)
		assert.NotEmpty(t, r.Do, r.Code)
		page, err := fs.ReadFile(docs.FS, "errors/"+r.Code+".md")
		require.NoError(t, err, "every rule has a page")
		first, _, _ := strings.Cut(string(page), "\n")
		assert.Equal(t, "# "+r.Code+": "+r.Title, first)
		assert.Contains(t, string(page), "\n**Fix:** "+r.Fix)
		for _, fix := range r.Also {
			assert.Contains(t, string(page), fix, "%s names each fix", r.Code)
		}
	}
}

func TestFixesNameTopics(t *testing.T) {
	topic := regexp.MustCompile(`\b(?:guides|tasks)/[\w.-]*[\w-]`)
	unknown := func(fix string) []string {
		var names []string
		for _, name := range topic.FindAllString(fix, -1) {
			if _, err := docs.Read(name); err != nil {
				names = append(names, name)
			}
		}
		return names
	}
	assert.Equal(t, []string{"tasks/List-by-role", "guides/sql_database", "tasks/list-by-role.md"},
		unknown("see tasks/List-by-role, guides/sql_database, tasks/list-by-role.md and tasks/list-by-role."))
	for _, r := range lint.Rules() {
		assert.Empty(t, unknown(r.Fix), "the fix of %s names topics that do not exist", r.Code)
	}
	assert.Contains(t, rule(t, "E-LINT-007").Fix, "as tasks/list-by-role shows", "the fix names the recipe for SQL chosen by the viewer's role")
}

func TestImports(t *testing.T) {
	unread := func(path string) string {
		return `E-LINT-001: app code may not import "` + path + `", a package of the app that the checks do not read: ` +
			`move it out of testdata, folders whose names start with _ or ., and folders that go.mod ignores`
	}
	assert.Equal(t, []string{
		`bad/bad.go:5: E-LINT-001: app code may not import "net"`,
		`bad/bad.go:6: E-LINT-001: app code may not import "os"`,
		`bad/bad.go:7: E-LINT-001: app code may not import "reflect"`,
		`bad/bad.go:9: E-LINT-001: app code may not import "aicoded.dev/framework/manifest"`,
		`bad/bad.go:10: E-LINT-001: app code may not import "aicoded.dev/framework/migrate"`,
		`bad/bad.go:11: E-LINT-001: app code may not import "aicoded.dev/framework/runnerproto"`,
		`bad/bad.go:12: E-LINT-001: app code may not import "aicoded.dev/framework/runnerproto/server"`,
		`bad/dot.go:3: E-LINT-001: app code may not import "path/filepath"`,
		`bad/own.go:4: ` + unread("imports/_hidden"),
		`bad/own.go:5: ` + unread("imports/skipped"),
		`bad/own.go:6: ` + unread("imports/testdata/old"),
		`go.mod:7: E-LINT-002: the app uses module google.golang.org/protobuf, which modules: in aicoded.yaml does not declare`,
		`render/copy_gen.go:5: E-LINT-001: only generated files may import "embed"`,
		`render/render.go:4: E-LINT-001: only generated files may import "aicoded.dev/framework/web/render"`,
		`render/route_gen.go:7: E-LINT-001: app code may not import "os"`,
		`skipped/skipped.go:1: E-LINT-010: skipped/skipped.go is in a folder that the ignore directive of go.mod names, so the checks do not read it`,
	}, run(t, "imports", "render/route_gen.go"), "a file is generated only when the generator lists it, a module the framework requires needs declaring when the app imports it, and a dot import of strings is allowed")
}

func TestFiles(t *testing.T) {
	assert.Equal(t, []string{
		`asm/asm_amd64.s:1: E-LINT-010: asm/asm_amd64.s is not Go source`,
		`cgo/cgo.go:1: E-LINT-010: cgo/cgo.go imports "C" (cgo)`,
		`pkg/deep/deep.go:1: E-LINT-010: pkg/deep/deep.go is in a folder that the ignore directive of go.mod names, so the checks do not read it`,
		`skipped/skipped.go:1: E-LINT-010: skipped/skipped.go is in a folder that the ignore directive of go.mod names, so the checks do not read it`,
		`tag/never.go:1: E-LINT-010: tag/never.go is left out of this build`,
		`tag/tag_plan9.go:1: E-LINT-010: tag/tag_plan9.go is left out of this build`,
	}, run(t, "files"), "tests, testdata and other modules are not read, and pkg/kept is read")
}

func TestEntries(t *testing.T) {
	const typeParam = "E-LINT-004: this composite literal builds a value of a type parameter, so lint cannot tell whether it sets Handler or RPC: use the concrete type"
	typeParamField := func(name string) string {
		return "E-LINT-004: " + name + " has a type with a type parameter, so lint cannot tell whether it is an entry point of app.Options: use the concrete type"
	}
	notLiteral := func(fn string) string {
		return "E-LINT-004: the options of app." + fn + " must be an app.Options literal written in the call, not a variable, a conversion or a value made elsewhere"
	}
	const handler = "E-LINT-004: Handler must be pages.NewHandler(...), the function aicoded generate writes"
	assert.Equal(t, []string{
		`bad/bad.go:29: E-LINT-004: set Handler only in the app.Options literal, to pages.NewHandler(...)`,
		`bad/bad.go:30: E-LINT-004: set RPC only in the app.Options literal, to rpc.Server()`,
		`bad/bad.go:32: ` + notLiteral("Main"),
		`bad/bad.go:32: ` + handler,
		`bad/bad.go:33: ` + notLiteral("Main"),
		`bad/bad.go:36: ` + handler,
		`bad/bad.go:37: E-LINT-004: RPC must be rpc.Server(), the function aicoded generate writes`,
		`bad/bad.go:43: ` + typeParam,
		`bad/bad.go:58: ` + notLiteral("Main"),
		`bad/bad.go:58: ` + handler,
		`bad/bad.go:67: ` + handler,
		`bad/bad.go:68: ` + handler,
		`bad/bad.go:69: ` + handler,
		`bad/bad.go:71: E-LINT-004: set Handler only in the app.Options literal, to pages.NewHandler(...)`,
		`bad/bad.go:77: ` + typeParam,
		`bad/bad.go:82: ` + notLiteral("Main"),
		`bad2/bad2.go:33: ` + typeParam,
		`bad2/bad2.go:38: ` + typeParam,
		`bad2/bad2.go:43: ` + typeParam,
		`bad2/bad2.go:48: ` + typeParam,
		`bad2/bad2.go:53: ` + notLiteral("Main"),
		`bad2/bad2.go:54: ` + notLiteral("Main"),
		`bad2/bad2.go:55: ` + notLiteral("Main"),
		`bad2/bad2.go:56: ` + notLiteral("Main"),
		`bad3/bad3.go:25: ` + typeParam,
		`bad3/bad3.go:30: ` + typeParam,
		`bad3/bad3.go:30: ` + handler,
		`bad3/bad3.go:35: ` + typeParam,
		`bad3/bad3.go:40: ` + typeParam,
		`bad3/bad3.go:40: ` + handler,
		`bad3/bad3.go:49: ` + typeParam,
		`bad3/bad3.go:54: ` + notLiteral("Main"),
		`bad3/bad3.go:55: ` + notLiteral("Main"),
		`bad3/bad3.go:56: ` + notLiteral("Main"),
		`bad3/bad3.go:57: ` + notLiteral("Main"),
		`bad3/bad3.go:58: ` + notLiteral("Main"),
		`bad3/bad3.go:70: ` + typeParamField("Handler"),
		`bad3/bad3.go:70: ` + typeParamField("RPC"),
		`bad3/bad3.go:75: ` + notLiteral("Main"),
		`bad4/bad4.go:23: ` + typeParamField("Handler"),
		`bad4/bad4.go:24: ` + typeParamField("RPC"),
		`bad4/bad4.go:31: ` + typeParamField("Handler"),
		`bad4/bad4.go:32: ` + typeParamField("RPC"),
		`bad4/bad4.go:38: ` + typeParamField("Handler"),
		`bad4/bad4.go:54: ` + typeParamField("Handler"),
		`bad4/bad4.go:55: ` + typeParamField("RPC"),
		`bad4/bad4.go:61: ` + notLiteral("Main"),
		`bad4/bad4.go:62: ` + notLiteral("Main"),
		`bad4/bad4.go:63: ` + notLiteral("Main"),
		`bad4/bad4.go:64: ` + notLiteral("Main"),
		`bad5/bad5.go:44: ` + notLiteral("Main"),
		`bad5/bad5.go:45: ` + notLiteral("Main"),
		`bad5/bad5.go:47: ` + notLiteral("Main"),
		`bad5/bad5.go:48: ` + notLiteral("Main"),
		`bad5/bad5.go:49: ` + notLiteral("Run"),
		`bad5/bad5.go:54: E-LINT-004: app.Main is used as a value: call it directly, with an app.Options literal`,
		`bad5/bad5.go:56: E-LINT-004: app.Run is used as a value: call it directly, with an app.Options literal`,
	}, run(t, "entries", "pages/handler_gen.go", "rpc/server_gen.go"), "main.go and Literal hand app.Main and app.Run a literal of the generated functions, and pair builds a slice of a type parameter")
}

func TestEntriesFromModule(t *testing.T) {
	app := proxyApp(t, true, "example.com/opt", map[string]string{
		"opt.go": `// Package opt builds options of the shape of app.Options.
package opt

import (
	"context"
	"net/http"
)

// Opts has the fields of app.Options.
type Opts[R any] struct {
	Handler http.Handler
	RPC     R
	OnStart func(context.Context) error
}

// Make returns options with h as the handler.
func Make[R any](h http.Handler) Opts[R] { return Opts[R]{Handler: h} }

// Plain returns options of an unnamed struct type with h as the handler.
func Plain[R any](h http.Handler) struct {
	Handler http.Handler
	RPC     R
	OnStart func(context.Context) error
} {
	return struct {
		Handler http.Handler
		RPC     R
		OnStart func(context.Context) error
	}{Handler: h}
}
`,
	}, `// Command app hands app.Main options that a declared module builds.
package main

import (
	"net/http"

	"example.com/opt"

	"aicoded.dev/framework/app"
	"aicoded.dev/framework/rpc"
)

// raw serves every request itself.
type raw struct{}

func (raw) ServeHTTP(http.ResponseWriter, *http.Request) {}

func main() {
	app.Main(app.Options(opt.Make[*rpc.Server](raw{})))
	app.Main(opt.Plain[*rpc.Server](raw{}))
}
`)
	notLiteral := "E-LINT-004: the options of app.Main must be an app.Options literal written in the call, not a variable, a conversion or a value made elsewhere"
	assert.Equal(t, []string{"main.go:19: " + notLiteral, "main.go:20: " + notLiteral},
		runIn(t, app, lint.Config{Modules: []string{"example.com/opt"}, FrameworkDir: checkout(t)}),
		"a declared module of plain Go hands app.Main a raw handler through a named or an unnamed struct")
}

func TestLoadErrorInModule(t *testing.T) {
	app := proxyApp(t, false, "example.com/broken", map[string]string{
		"broken.go": "// Package broken does not type-check.\npackage broken\n\n// N is not a number.\nvar N int = \"x\"\n",
	}, "package main\n\nimport \"example.com/broken\"\n\nfunc main() { _ = broken.N }\n")
	_, err := lint.Run(t.Context(), app, lint.Config{Modules: []string{"example.com/broken"}})
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "lint cannot load package example.com/broken: example.com/broken@v1.0.0/broken.go:5:13: "),
		"a file of the module is named module@version/path, not by its folder in the module cache: %s", err)
}

func TestImportsOutsideFramework(t *testing.T) {
	app := proxyApp(t, false, "aicoded.dev/framework/web", map[string]string{
		"web.go": "// Package web is not the framework's.\npackage web\n\n// Name names it.\nconst Name = \"web\"\n",
	}, "package main\n\nimport \"aicoded.dev/framework/web\"\n\nfunc main() { _ = web.Name }\n")
	assert.Equal(t, []string{
		`main.go:3: E-LINT-001: app code may not import "aicoded.dev/framework/web", which is not a package of the framework module`,
	}, runIn(t, app, lint.Config{Modules: []string{"aicoded.dev/framework/web"}}), "a module whose path starts with the framework's is not the framework")
}

func TestImportsBesideFramework(t *testing.T) {
	app := proxyApp(t, true, "aicoded.dev/frameworkx", map[string]string{
		"web/web.go":   "// Package web is not the framework's.\npackage web\n\nimport \"aicoded.dev/frameworkx/name\"\n\n// Name names it.\nconst Name = name.Name\n",
		"name/name.go": "// Package name is not the framework's.\npackage name\n\n// Name names it.\nconst Name = \"web\"\n",
	}, "package main\n\nimport (\n\t_ \"aicoded.dev/framework/web\"\n\t\"aicoded.dev/frameworkx/web\"\n)\n\nfunc main() { _ = web.Name }\n")
	cfg := lint.Config{FrameworkDir: checkout(t)}
	assert.Equal(t, []string{
		`go.mod:7: E-LINT-002: the app uses module aicoded.dev/frameworkx, which modules: in aicoded.yaml does not declare`,
	}, runIn(t, app, cfg), "aicoded.dev/framework/web is allowed, and aicoded.dev/frameworkx/web is a third-party package, not the framework's")
	cfg.Modules = []string{"aicoded.dev/frameworkx"}
	assert.Empty(t, runIn(t, app, cfg), "declared, aicoded.dev/frameworkx may import aicoded.dev/frameworkx/name, which is not the framework's")
}

// proxyApp writes an app that imports the module mod, served at v1.0.0 with files from a local
// file GOPROXY, and main.go as its main package, and requires the framework from this checkout
// when framework is set. It points the go command at an empty module cache that the proxy and the
// module cache of this computer fill, runs go mod tidy, turns the network off and returns the
// app's folder.
func proxyApp(t *testing.T, framework bool, mod string, files map[string]string, main string) string {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	downloads := filepath.Join(goEnv(t, "GOMODCACHE"), "cache", "download")
	proxy := filepath.Join(t.TempDir(), "proxy")
	files["go.mod"] = "module " + mod + "\n\ngo 1.26.0\n"
	publishModule(t, proxy, mod, "v1.0.0", files)
	t.Setenv("GOFLAGS", "-modcacherw")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "modcache"))
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy)+",file://"+filepath.ToSlash(downloads))
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	app := t.TempDir()
	gomod := "module app\n\ngo 1.26.0\n"
	if framework {
		gomod += "\nrequire aicoded.dev/framework v0.0.0-00010101000000-000000000000\n\nreplace aicoded.dev/framework => " + checkout(t) + "\n"
	}
	write(t, app, "go.mod", gomod)
	write(t, app, "main.go", main)
	goRun(t, app, "mod", "tidy")
	t.Setenv("GOPROXY", "off")
	return app
}

// goEnv returns the value of the go environment variable name.
func goEnv(t *testing.T, name string) string {
	out, err := exec.CommandContext(t.Context(), "go", "env", name).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestNames(t *testing.T) {
	const typeParamConversion = "app code may not convert to or from a type parameter that could be a pointer, or a pointer to a type parameter: use the concrete types"
	const (
		makeClient = "app code may not make an http.Client of its own: use the *http.Client that outbound.Client() returns"
		holdClient = "app code may not hold an http.Client by value: hold the *http.Client that outbound.Client() returns"
	)
	assert.Equal(t, []string{
		`bad/bad.go:32: E-LINT-005: only generated code may call rpc.Call`,
		`bad/bad.go:33: E-LINT-005: only generated code may call web.Request.CSRFToken`,
		`bad/bad.go:34: E-LINT-005: only generated code may call form.Input.Process`,
		`bad/bad.go:35: E-LINT-005: only generated code may call reactive.HTMLBinding`,
		`bad/bad.go:36: E-LINT-006: app code may not use sql.Open`,
		`bad/bad.go:37: E-LINT-006: app code may not use sql.DB.Driver`,
		`bad/bad.go:39: E-LINT-006: app code may not use sql.DB.Driver`,
		`bad/bad.go:40: E-LINT-009: app code may not use http.Get`,
		`bad/bad.go:42: E-LINT-009: app code may not use sql.Conn.Raw`,
		`bad/bad.go:43: E-LINT-009: app code may not use slog.SetDefault`,
		`bad/bad.go:44: E-LINT-009: app code may not change rand.Reader`,
		`bad/bad.go:45: E-LINT-009: app code may not change time.Local`,
		`bad/bad.go:64: E-LINT-006: app code may not use sql.DB.Driver`,
		`bad/bad.go:68: E-LINT-009: app code may not change time.Local`,
		`bad/bad.go:69: E-LINT-009: app code may not change crc32.IEEETable`,
		`bad/bad.go:70: E-LINT-009: app code may not change time.Local`,
		`bad/bad.go:72: E-LINT-009: app code may not change a slog.Logger through a pointer`,
		`bad/bad.go:74: E-LINT-009: app code may not change a slog.Logger through a pointer`,
		`bad/bad.go:75: E-LINT-009: app code may not change a time.Location through a pointer`,
		`bad/bad.go:78: E-LINT-009: app code may not change a time.Location through a pointer`,
		`bad/bad.go:92: E-LINT-009: app code may not use sql.ColumnType.ScanType`,
		`bad/bad.go:95: E-LINT-009: app code may not use MethodByName of package reflect`,
		`bad/bad.go:96: E-LINT-009: app code may not use Func of package reflect`,
		`bad/bad.go:96: E-LINT-009: app code may not use Interface of package reflect`,
		`bad/bad.go:113: E-LINT-009: app code may not convert a pointer to slog.Logger to another pointer type`,
		`bad/bad.go:114: E-LINT-009: app code may not change a time.Time through a pointer`,
		`bad/bad.go:119: E-LINT-009: app code may not change a value through a pointer to a type parameter, which could point at a value of the standard library or the framework`,
		`bad/client.go:13: E-LINT-009: ` + holdClient,
		`bad/client.go:17: E-LINT-009: ` + holdClient,
		`bad/client.go:18: E-LINT-009: ` + holdClient,
		`bad/client.go:23: E-LINT-009: ` + makeClient,
		`bad/client.go:24: E-LINT-009: ` + makeClient,
		`bad/client.go:25: E-LINT-009: ` + makeClient,
		`bad/client.go:26: E-LINT-009: ` + holdClient,
		`bad/client.go:27: E-LINT-009: ` + makeClient,
		`bad/client.go:28: E-LINT-009: ` + makeClient,
		`bad/client.go:29: E-LINT-009: app code may not use http.Transport`,
		`bad/client.go:30: E-LINT-009: app code may not use http.Transport`,
		`bad/client.go:31: E-LINT-009: app code may not use http.DefaultClient`,
		`bad/client.go:32: E-LINT-009: app code may not use http.DefaultTransport`,
		`bad/client.go:33: E-LINT-009: app code may not use http.Get`,
		`bad/client.go:34: E-LINT-009: app code may not use http.Post`,
		`bad/client.go:35: E-LINT-009: app code may not use http.Head`,
		`bad/client.go:36: E-LINT-009: app code may not use http.PostForm`,
		`bad/client.go:38: E-LINT-009: app code may not use http.Client.Transport`,
		`bad/client.go:39: E-LINT-009: app code may not use http.Client.CheckRedirect`,
		`bad/client.go:40: E-LINT-009: app code may not use http.Client.Jar`,
		`bad/client.go:41: E-LINT-009: app code may not use http.Client.Timeout`,
		`bad/client.go:42: E-LINT-009: app code may not change a http.Client through a pointer`,
		`bad/client.go:43: E-LINT-009: app code may not use http.Client.Get`,
		`bad/client.go:44: E-LINT-009: app code may not turn a *http.Client into an interface value, through which other code could change the client`,
		`bad/client.go:45: E-LINT-009: app code may not turn a bad.holder into an interface value, through which other code could change the client it holds`,
		`bad/client.go:46: E-LINT-009: app code may not instantiate zeroOf with a type that holds an http.Client`,
		`bad/client.go:58: E-LINT-009: app code may not turn a *http.Client into an interface value in Reach, through which other code could change the client`,
		`bad/client.go:60: E-LINT-009: app code may not use http.Client.Transport`,
		`bad/client.go:61: E-LINT-009: app code may not use http.Client.Head`,
		`bad/client.go:63: E-LINT-009: app code may not turn a **http.Client into an interface value, through which other code could change the client it holds`,
		`bad/client.go:64: E-LINT-009: app code may not instantiate pass with a type that holds an http.Client`,
		`bad/client.go:66: E-LINT-009: app code may not turn a bad.wrapped into an interface value, through which other code could change the client it holds`,
		`bad2/bad2.go:10: E-LINT-009: ` + typeParamConversion,
		`bad2/bad2.go:13: E-LINT-009: ` + typeParamConversion,
		`bad3/bad3.go:16: E-LINT-009: ` + typeParamConversion,
		`bad3/bad3.go:20: E-LINT-009: ` + typeParamConversion,
		`bad3/bad3.go:24: E-LINT-009: app code may not change a value through a pointer to a type parameter, which could point at a value of the standard library or the framework`,
		`bad4/bad4.go:18: E-LINT-009: ` + typeParamConversion,
		`bad4/bad4.go:25: E-LINT-009: ` + typeParamConversion,
		`bad4/bad4.go:29: E-LINT-009: app code may not change a value through a pointer to a type parameter, which could point at a value of the standard library or the framework`,
		`pages/key.go:5: E-LINT-005: only generated code may use routeKey, which pages/route_gen.go declares`,
		`pages/key.go:5: E-LINT-005: only generated code may use state, which pages/route_gen.go declares`,
		`pages/key.go:5: E-LINT-005: only generated code may use conn, which pages/route_gen.go declares`,
	}, run(t, "names", "pages/route_gen.go"), "Keep, Count, Bypass, Sum, Show, Any and the generated file use only what they may")
}

func TestClientFix(t *testing.T) {
	const (
		outboundFix = "call outside services with outbound.Client(), which goes through the runner"
		asIsFix     = "use the client from outbound.Client() as it is: build requests with http.NewRequestWithContext and send them with its Do method"
	)
	r, err := lint.Run(t.Context(), filepath.Join("testdata", "names"), lint.Config{Generated: []string{"pages/route_gen.go"}, FrameworkDir: checkout(t)})
	require.NoError(t, err)
	fixes := map[string][]string{}
	for _, p := range r.Problems {
		if p.Fix == outboundFix || p.Fix == asIsFix {
			fixes[p.Fix] = append(fixes[p.Fix], p.Pos)
		} else {
			assert.False(t, strings.HasPrefix(p.Pos, "bad/client.go:"), p.Error())
		}
	}
	lines := func(file string, ns ...int) []string {
		var out []string
		for _, n := range ns {
			out = append(out, file+":"+strconv.Itoa(n))
		}
		return out
	}
	assert.Equal(t, append(lines("bad/bad.go", 40), lines("bad/client.go", 23, 24, 25, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36)...),
		fixes[outboundFix], "a client the app makes, and the client names of net/http")
	assert.Equal(t, lines("bad/client.go", 13, 17, 18, 26, 38, 39, 40, 41, 42, 43, 44, 45, 46, 58, 60, 61, 63, 64, 66),
		fixes[asIsFix], "a client held by value, and what is done to the one outbound.Client returns")
}

func TestClientInDeps(t *testing.T) {
	assert.Equal(t, []string{
		"pages/handler_gen.go:18: E-LINT-009: app code may not turn a *exposed.DP into an interface value, through which other code could change the client it holds",
	}, run(t, "clients", "pages/handler_gen.go", "pages/route_gen.go", "pages/exposed/route_gen.go"),
		"a client in an exported field of Deps that a page's DP reaches, not one in an unexported field")
}

func TestBanned(t *testing.T) {
	for _, c := range []struct{ path, name, code string }{
		{"os", "", "E-LINT-001"},
		{"os", "Exit", "E-LINT-001"},
		{"aicoded.dev/framework/web/render", "", "E-LINT-001"},
		{"github.com/go-sql-driver/mysql", "", "E-LINT-002"},
		{"aicoded.dev/frameworkx/web", "", "E-LINT-002"},
		{"net/http", "", ""},
		{"net/http", "Request", ""},
		{"net/http", "StatusOK", ""},
		{"net/http", "Get", "E-LINT-009"},
		{"net/http", "Request.Context", ""},
		{"database/sql", "Open", "E-LINT-006"},
		{"database/sql", "DB.Driver", "E-LINT-006"},
		{"database/sql", "DB.QueryContext", ""},
		{"database/sql", "Conn.Raw", "E-LINT-009"},
		{"database/sql", "ColumnType.ScanType", "E-LINT-009"},
		{"database/sql", "ColumnType.Name", ""},
		{"net/http", "Client", ""},
		{"net/http", "Client.Do", ""},
		{"net/http", "Client.Get", "E-LINT-009"},
		{"net/http", "Client.Transport", "E-LINT-009"},
		{"net/http", "NewRequestWithContext", ""},
		{"net/http", "NoBody", ""},
		{"net/http", "Response", ""},
		{"net/http", "DefaultClient", "E-LINT-009"},
		{"net/http", "DefaultTransport", "E-LINT-009"},
		{"net/http", "Transport", "E-LINT-009"},
		{"net/http", "Transport.RoundTrip", "E-LINT-009"},
		{"aicoded.dev/framework/outbound", "Client", ""},
		{"database/sql", "Named", ""},
		{"database/sql", "ErrConnDone", ""},
		{"database/sql", "Drivers", "E-LINT-009"},
		{"log/slog", "SetDefault", "E-LINT-009"},
		{"log/slog", "InfoContext", ""},
		{"aicoded.dev/framework/rpc", "Call", "E-LINT-005"},
		{"aicoded.dev/framework/web/form", "Input.Process", "E-LINT-005"},
		{"aicoded.dev/framework/web", "Forbidden", ""},
	} {
		assert.Equal(t, c.code, lint.Banned(c.path, c.name), "%s %s", c.path, c.name)
	}
}

func TestModules(t *testing.T) {
	replaced := []string{
		`go.mod:15: E-LINT-011: go.mod replaces example.com/caps with ./third/caps`,
		`go.mod:16: E-LINT-011: go.mod replaces example.com/fine with ./third/fine`,
		`go.mod:17: E-LINT-011: go.mod replaces example.com/link with ./third/link`,
		`go.mod:18: E-LINT-011: go.mod replaces example.com/testonly with ./third/testonly`,
		`go.mod:19: E-LINT-011: go.mod replaces example.com/words with ./third/words`,
	}
	assert.Equal(t, append([]string{
		`go.mod:6: E-LINT-002: the app uses module example.com/caps, which modules: in aicoded.yaml does not declare`,
		`go.mod:7: E-LINT-002: the app uses module example.com/fine, which modules: in aicoded.yaml does not declare`,
		`go.mod:8: E-LINT-002: the app uses module example.com/link, which modules: in aicoded.yaml does not declare`,
	}, replaced...), runWith(t, "modules", lint.Config{}), "only a test imports testonly, and only fine imports words")

	assert.Equal(t, append([]string{
		`example.com/caps@v0.1.0/run/broken.go:1: E-LINT-003: declared module example.com/caps holds broken.go, which the checks cannot read`,
		`example.com/caps@v0.1.0/run/call.go:17: E-LINT-003: declared module example.com/caps uses reflect.Value.Method`,
		`example.com/caps@v0.1.0/run/call.go:18: E-LINT-003: declared module example.com/caps uses reflect.Value.MethodByName`,
		`example.com/caps@v0.1.0/run/call.go:18: E-LINT-003: declared module example.com/caps uses reflect.Value.Call`,
		`example.com/caps@v0.1.0/run/call.go:22: E-LINT-003: declared module example.com/caps uses reflect.Value.CallSlice`,
		`example.com/caps@v0.1.0/run/call.go:25: E-LINT-003: declared module example.com/caps uses reflect.Value.Call`,
		`example.com/caps@v0.1.0/run/call.go:29: E-LINT-003: declared module example.com/caps uses reflect.Type.MethodByName`,
		`example.com/caps@v0.1.0/run/call.go:30: E-LINT-003: declared module example.com/caps uses reflect.Type.Method`,
		`example.com/caps@v0.1.0/run/call.go:38: E-LINT-003: declared module example.com/caps uses sql.ColumnType.ScanType`,
		`example.com/caps@v0.1.0/run/count.go:10: E-LINT-003: declared module example.com/caps runs SQL with sql.DB.QueryRowContext`,
		`example.com/caps@v0.1.0/run/count.go:19: E-LINT-003: declared module example.com/caps runs SQL with sql.DB.QueryContext`,
		`example.com/caps@v0.1.0/run/count.go:25: E-LINT-003: declared module example.com/caps calls Query, which has type parameters in its signature, so lint cannot tell whether it takes SQL`,
		`example.com/caps@v0.1.0/run/count.go:29: E-LINT-003: declared module example.com/caps uses sql.Open`,
		`example.com/caps@v0.1.0/run/count.go:33: E-LINT-003: declared module example.com/caps uses sql.DB.Driver`,
		`example.com/caps@v0.1.0/run/count.go:38: E-LINT-003: declared module example.com/caps uses sql.Conn.Raw`,
		`example.com/caps@v0.1.0/run/dial.go:4: E-LINT-003: declared module example.com/caps imports "crypto/tls"`,
		`example.com/caps@v0.1.0/run/dial.go:5: E-LINT-003: declared module example.com/caps imports "go/build"`,
		`example.com/caps@v0.1.0/run/dial.go:6: E-LINT-003: declared module example.com/caps imports "go/importer"`,
		`example.com/caps@v0.1.0/run/dial.go:8: E-LINT-003: declared module example.com/caps imports "log/syslog"`,
		`example.com/caps@v0.1.0/run/framework.go:5: E-LINT-003: declared module example.com/caps imports "aicoded.dev/framework/runnerproto"`,
		`example.com/caps@v0.1.0/run/logs.go:13: E-LINT-003: declared module example.com/caps uses slog.SetDefault`,
		`example.com/caps@v0.1.0/run/logs.go:14: E-LINT-003: declared module example.com/caps uses slog.SetLogLoggerLevel`,
		`example.com/caps@v0.1.0/run/logs.go:15: E-LINT-003: declared module example.com/caps uses slog.NewLogLogger`,
		`example.com/caps@v0.1.0/run/logs.go:16: E-LINT-003: declared module example.com/caps changes time.Local`,
		`example.com/caps@v0.1.0/run/logs.go:17: E-LINT-003: declared module example.com/caps changes time.Local`,
		`example.com/caps@v0.1.0/run/logs.go:18: E-LINT-003: declared module example.com/caps changes crc32.IEEETable`,
		`example.com/caps@v0.1.0/run/logs.go:19: E-LINT-003: declared module example.com/caps changes rand.Reader`,
		`example.com/caps@v0.1.0/run/logs.go:21: E-LINT-003: declared module example.com/caps changes time.Local`,
		`example.com/caps@v0.1.0/run/proc.go:4: E-LINT-003: declared module example.com/caps imports "net/http/httputil"`,
		`example.com/caps@v0.1.0/run/proc.go:6: E-LINT-003: declared module example.com/caps imports "net/rpc"`,
		`example.com/caps@v0.1.0/run/proc.go:12: E-LINT-003: declared module example.com/caps uses os.Process`,
		`example.com/caps@v0.1.0/run/proc.go:13: E-LINT-003: declared module example.com/caps uses os.StartProcess`,
		`example.com/caps@v0.1.0/run/proc.go:18: E-LINT-003: declared module example.com/caps uses os.FindProcess`,
		`example.com/caps@v0.1.0/run/proc.go:22: E-LINT-003: declared module example.com/caps uses os.Process.Kill`,
		`example.com/caps@v0.1.0/run/raw.go:13: E-LINT-003: declared module example.com/caps uses reflect.NewAt`,
		`example.com/caps@v0.1.0/run/raw.go:13: E-LINT-003: declared module example.com/caps uses reflect.Value.UnsafePointer`,
		`example.com/caps@v0.1.0/run/raw.go:14: E-LINT-003: declared module example.com/caps uses driver.Connector.Connect`,
		`example.com/caps@v0.1.0/run/raw.go:18: E-LINT-003: declared module example.com/caps uses driver.QueryerContext.QueryContext`,
		`example.com/caps@v0.1.0/run/raw.go:23: E-LINT-003: declared module example.com/caps uses reflect.Value.Pointer`,
		`example.com/caps@v0.1.0/run/raw.go:23: E-LINT-003: declared module example.com/caps uses reflect.Value.UnsafeAddr`,
		`example.com/caps@v0.1.0/run/raw.go:27: E-LINT-003: declared module example.com/caps uses driver.Driver.Open`,
		`example.com/caps@v0.1.0/run/raw.go:30: E-LINT-003: declared module example.com/caps uses driver.Conn.Prepare`,
		`example.com/caps@v0.1.0/run/raw.go:34: E-LINT-003: declared module example.com/caps uses driver.StmtQueryContext.QueryContext`,
		`example.com/caps@v0.1.0/run/run.go:6: E-LINT-003: declared module example.com/caps imports "os/exec"`,
		`example.com/caps@v0.1.0/run/run.go:15: E-LINT-003: declared module example.com/caps uses http.Get`,
		`example.com/caps@v0.1.0/run/run.go:18: E-LINT-003: declared module example.com/caps uses http.Client`,
		`example.com/caps@v0.1.0/run/run.go:18: E-LINT-003: declared module example.com/caps uses http.Client`,
		`example.com/caps@v0.1.0/run/run_plan9.go:3: E-LINT-003: declared module example.com/caps imports "syscall"`,
		`example.com/caps@v0.1.0/run/stub.s:1: E-LINT-003: declared module example.com/caps holds stub.s, which is not Go source`,
		`example.com/link@v0.1.0/clock/clock.go:4: E-LINT-003: declared module example.com/link imports "unsafe"`,
		`example.com/link@v0.1.0/clock/clock.go:6: E-LINT-003: declared module example.com/link uses //go:linkname`,
		`go.mod:12: E-LINT-002: the app uses module example.com/words, which modules: in aicoded.yaml does not declare`,
	}, replaced...), runWith(t, "modules", lint.Config{Modules: []string{"example.com/caps", "example.com/fine", "example.com/link"}}),
		"fine is plain Go and may be imported, but it imports words, which needs declaring too, and Keep only reads the variables of the standard library")
}

func TestReplace(t *testing.T) {
	link := filepath.Join(t.TempDir(), "framework")
	require.NoError(t, os.Symlink(checkout(t), link))
	for _, dir := range []string{checkout(t), link} {
		assert.Empty(t, only("E-LINT-011", runWith(t, "names", lint.Config{FrameworkDir: dir})), "the framework replaced with this checkout, also through %s", dir)
	}
	assert.Equal(t, []string{
		`go.mod:13: E-LINT-011: go.mod replaces aicoded.dev/framework with ../../.., and this aicoded was not installed from a framework checkout`,
	}, only("E-LINT-011", runWith(t, "names", lint.Config{})), "a released aicoded accepts no replace")

	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n\n"+
		"replace aicoded.dev/framework => ./framework\n\n"+
		"replace aicoded.dev/framework v0.1.0 => example.com/fork v0.1.0\n\n"+
		"replace aicoded.dev/framework v0.2.0 => "+link+"\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	write(t, app, "framework/go.mod", "module aicoded.dev/framework\n\ngo 1.26.0\n")
	other := []string{
		`go.mod:5: E-LINT-011: go.mod replaces aicoded.dev/framework with ./framework, which is not the framework checkout this aicoded was installed from`,
		`go.mod:7: E-LINT-011: go.mod replaces aicoded.dev/framework with example.com/fork v0.1.0, which is not the framework checkout this aicoded was installed from`,
	}
	assert.Equal(t, other, runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "the framework replaced with another folder or module")

	write(t, app, "go.work", "go 1.26.0\n\nuse .\n")
	assert.Equal(t, append(other,
		`go.work:1: E-LINT-011: the go command builds the app with the workspace file go.work, whose use and replace lines stand in for modules`,
	), runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "a workspace file")
}

func TestVendor(t *testing.T) {
	t.Setenv("GOFLAGS", "")
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	write(t, app, "vendor/modules.txt", "")
	assert.Equal(t, []string{"vendor/modules.txt:1: E-LINT-011: the go command builds the app from its vendor folder, which can hold a changed copy of any module"},
		only("E-LINT-011", runIn(t, app, lint.Config{FrameworkDir: checkout(t)})), "a vendor folder is refused at its manifest")

	require.NoError(t, os.Remove(filepath.Join(app, "vendor", "modules.txt")))
	write(t, app, "vendor/x.txt", "")
	assert.Equal(t, []string{"vendor: E-LINT-011: the go command builds the app from its vendor folder, which can hold a changed copy of any module"},
		only("E-LINT-011", runIn(t, app, lint.Config{FrameworkDir: checkout(t)})), "an inconsistent vendor folder is refused too, before loading")
}

func TestPrecheck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	t.Setenv("GOFLAGS", "")
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	ps, err := lint.Precheck(t.Context(), app)
	require.NoError(t, err)
	assert.Empty(t, ps)
	r, err := lint.Run(t.Context(), app, lint.Config{})
	require.NoError(t, err)
	assert.False(t, r.Stopped)

	write(t, app, "vendor/modules.txt", "")
	t.Setenv("GOFLAGS", "-toolexec=/bin/true")
	ps, err = lint.Precheck(t.Context(), app)
	require.NoError(t, err)
	var found []string
	for _, p := range ps {
		found = append(found, p.Pos+": "+p.Code)
	}
	assert.Equal(t, []string{"GOFLAGS: E-LINT-011", "vendor/modules.txt:1: E-LINT-011"}, found)
	r, err = lint.Run(t.Context(), app, lint.Config{})
	require.NoError(t, err)
	assert.True(t, r.Stopped)
	assert.Equal(t, ps, r.Problems, "Run stops with what Precheck finds")
}

func TestGoflags(t *testing.T) {
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	for _, fl := range []string{
		"-overlay=/x/overlay.json",
		"--overlay=/x/overlay.json",
		"'-overlay=/x/overlay.json'",
		`"-overlay=/x/overlay.json"`,
		"'-v=true'-overlay=/x/overlay.json",
		"'-v''-overlay=/x/overlay.json'",
		"'-buildvcs=auto'-overlay=/x/overlay.json",
		`"-trimpath=true"-overlay=/x/overlay.json`,
		"'' -v",
		"-v\\ -x",
		"-v\v-overlay=/x/overlay.json",
		"-v\f",
		"-v\x1b",
		"-overlay /x/overlay.json",
		"-v\t-overlay=/x/overlay.json",
		"-v\n-overlay=/x/overlay.json",
		"-modfile=evil.mod",
		"--modfile=evil.mod",
		"--mod=mod",
		"-mod=vendor",
		"-mod=",
		"-buildvcs",
		"-v=true",
		"-toolexec=/bin/true",
		"-ldflags=-s",
		"-gcflags=-m",
		"-tags=x",
	} {
		t.Setenv("GOFLAGS", fl)
		found := runIn(t, app, lint.Config{FrameworkDir: checkout(t)})
		require.NotEmpty(t, found, "GOFLAGS=%q is refused", fl)
		for _, l := range found {
			assert.True(t, strings.HasPrefix(l, "GOFLAGS: E-LINT-011: the GOFLAGS environment variable "), "GOFLAGS=%q is refused before any go command runs: %s", fl, l)
		}
	}
	t.Setenv("GOFLAGS", "-overlay /x/overlay.json")
	assert.Equal(t, []string{
		"GOFLAGS: E-LINT-011: the GOFLAGS environment variable sets " + "-overlay, which lint does not allow; " + allowedGoflags,
		"GOFLAGS: E-LINT-011: the GOFLAGS environment variable sets /x/overlay.json, which lint does not allow; " + allowedGoflags,
	}, runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "each entry outside the list")
	t.Setenv("GOFLAGS", "'-v=true'-overlay=/x/overlay.json")
	assert.Equal(t, []string{
		"GOFLAGS: E-LINT-011: the GOFLAGS environment variable holds a quote, a backslash or a control character, which lint does not allow; " + allowedGoflags,
	}, runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "a quote")

	for _, fl := range []string{"-mod=mod -trimpath -v -x -modcacherw -buildvcs=false", " -mod=readonly\t-buildvcs=true\n-buildvcs=auto\r\n"} {
		t.Setenv("GOFLAGS", fl)
		assert.Empty(t, runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "GOFLAGS=%q holds only the entries the build itself may use", fl)
	}
}

// allowedGoflags is how a GOFLAGS finding names the entries lint allows.
const allowedGoflags = "only -mod=mod, -mod=readonly, -modcacherw, -trimpath, -buildvcs=true, -buildvcs=false, -buildvcs=auto, -v and -x are allowed, separated by spaces"

func TestGoflagsGoEnv(t *testing.T) {
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	goenv := filepath.Join(t.TempDir(), "env")
	t.Setenv("GOENV", goenv)
	t.Setenv("GOFLAGS", "")
	const prefix = "GOFLAGS: E-LINT-011: "
	for _, c := range []struct{ goflags, want string }{
		{"-v -overlay=/x/overlay.json", "the GOFLAGS that go env prints sets -overlay=/x/overlay.json, which lint does not allow; " + allowedGoflags},
		{"-changed -overlay=/x/overlay.json", "the GOFLAGS that go env prints sets -changed, which lint does not allow; " + allowedGoflags},
		{"-u -overlay=/x/overlay.json", "lint cannot read GOFLAGS with go env: go: cannot use -json with -u"},
		{"-w -overlay=/x/overlay.json", "lint cannot read GOFLAGS with go env: go: cannot use -json with -w"},
		{"-C=/tmp -overlay=/x/overlay.json", `lint cannot read GOFLAGS with go env: go: invalid value "/tmp" for flag -C (from $GOFLAGS)`},
		{"''", "lint cannot read GOFLAGS with go env: "},
	} {
		data := "GOFLAGS=" + c.goflags + "\n"
		require.NoError(t, os.WriteFile(goenv, []byte(data), 0o600))
		found := runIn(t, app, lint.Config{FrameworkDir: checkout(t)})
		require.NotEmpty(t, found, "GOFLAGS=%s in the go env file is refused", c.goflags)
		assert.True(t, strings.HasPrefix(found[0], prefix+c.want), "GOFLAGS=%s: %s", c.goflags, found[0])
		assert.NotContains(t, found[0], "\n", "the finding quotes the first line of what the go command printed")
		after, err := os.ReadFile(goenv)
		require.NoError(t, err)
		assert.Equal(t, data, string(after), "lint never changes the go env file")
	}

	require.NoError(t, os.WriteFile(goenv, nil, 0o600))
	t.Setenv("GOTOOLCHAIN", "bogus")
	assert.Equal(t, []string{
		prefix + `lint cannot read GOFLAGS with go env: go: invalid GOTOOLCHAIN "bogus"; ` +
			"when the go command itself cannot run, fix what it reports, such as the go line of go.mod or GOTOOLCHAIN",
	}, runIn(t, app, lint.Config{FrameworkDir: checkout(t)}), "a failed go env, with what the go command printed")
}

func TestGoflagsToolchainNotAvailable(t *testing.T) {
	bin := t.TempDir()
	write(t, bin, "go", "#!/bin/sh\n"+
		"echo 'go: downloading go1.26.0 (linux/amd64)' >&2\n"+
		"echo 'go: download go1.26.0 for linux/amd64: toolchain not available' >&2\n"+
		"exit 1\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "go"), 0o700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOFLAGS", "")
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	assert.Equal(t, []string{
		"GOFLAGS: E-LINT-011: lint cannot read GOFLAGS with go env: go: download go1.26.0 for linux/amd64: toolchain not available (and 1 more line); " +
			"when the go command itself cannot run, fix what it reports, such as the go line of go.mod or GOTOOLCHAIN",
	}, runIn(t, app, lint.Config{}), "the finding quotes the line that says why, not the download notice")
	assert.True(t, strings.HasSuffix(rule(t, "E-LINT-011").Fix, "when the go command itself cannot run, fix what it reports, such as the go line of go.mod or GOTOOLCHAIN"))
}

func TestEditedCache(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	home := t.TempDir()
	cache := filepath.Join(home, "modcache")
	proxy := filepath.Join(home, "proxy")
	publishModule(t, proxy, "example.com/dep", "v1.0.0", map[string]string{
		"go.mod":    "module example.com/dep\n\ngo 1.26.0\n",
		"dep.go":    "// Package dep is a plain module.\npackage dep\n\n// N is a number.\nconst N = 1\n",
		"README.md": "dep\n",
	})
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOMODCACHE", cache)
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy))
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GODEBUG", "goindex=0")
	t.Cleanup(func() {
		_ = filepath.WalkDir(cache, func(p string, _ os.DirEntry, _ error) error { _ = os.Chmod(p, 0o700); return nil })
	})
	app := t.TempDir()
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n\nrequire example.com/dep v1.0.0\n")
	write(t, app, "main.go", "package main\n\nimport \"example.com/dep\"\n\nfunc main() { _ = dep.N }\n")
	goRun(t, app, "mod", "download")
	goRun(t, app, "mod", "tidy")
	t.Setenv("GOPROXY", "off")

	cfg := lint.Config{Modules: []string{"example.com/dep"}}
	assert.Empty(t, only("E-LINT-011", runIn(t, app, cfg)), "an unmodified module cache is clean, offline")

	dep := filepath.Join(cache, "example.com", "dep@v1.0.0")
	require.NoError(t, os.Chmod(dep, 0o755))
	require.NoError(t, os.Chmod(filepath.Join(dep, "dep.go"), 0o644))
	write(t, dep, "dep.go", "// Package dep is a plain module.\npackage dep\n\n// N is a number.\nconst N = 2\n")
	verified := func(want string) {
		t.Helper()
		assert.Equal(t, []string{"go.mod:1: E-LINT-011: go mod verify does not report all modules verified, so the build could use changed code in the module cache: " + want},
			only("E-LINT-011", runIn(t, app, cfg)), "go mod verify fails, offline, and the module cache is named $GOMODCACHE")
	}
	verified("example.com/dep v1.0.0: dir has been modified ($GOMODCACHE/example.com/dep@v1.0.0)")

	if os.Geteuid() != 0 {
		require.NoError(t, os.Chmod(filepath.Join(dep, "README.md"), 0))
		verified("example.com/dep v1.0.0: open $GOMODCACHE/example.com/dep@v1.0.0/README.md: permission denied")
	}
}

// publishModule writes module mod at version into a local file GOPROXY under proxy.
func publishModule(t *testing.T, proxy, mod, version string, files map[string]string) {
	dir := filepath.Join(proxy, filepath.FromSlash(mod), "@v")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, version+".info"), []byte(`{"Version":"`+version+`","Time":"2026-01-01T00:00:00Z"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, version+".mod"), []byte(files["go.mod"]), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "list"), []byte(version+"\n"), 0o644))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(mod + "@" + version + "/" + name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(filepath.Join(dir, version+".zip"), buf.Bytes(), 0o644))
}

// goRun runs the go command in dir with the process environment, failing the test on error.
func goRun(t *testing.T, dir string, args ...string) {
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "go %s: %s", strings.Join(args, " "), out)
}

func TestWorkAbove(t *testing.T) {
	parent := t.TempDir()
	app := filepath.Join(parent, "app")
	write(t, app, "go.mod", "module app\n\ngo 1.26.0\n")
	write(t, app, "main.go", "package main\n\nfunc main() {}\n")
	write(t, parent, "go.work", "go 1.26.0\n\nuse ./app\n")
	lines := only("E-LINT-011", runIn(t, app, lint.Config{FrameworkDir: checkout(t)}))
	require.Len(t, lines, 1, "a go.work above the app is found and refused")
	assert.Equal(t, filepath.Join(parent, "go.work")+":1: E-LINT-011: the go command builds the app with the workspace file "+filepath.Join(parent, "go.work")+", whose use and replace lines stand in for modules", lines[0])
}

func TestSQL(t *testing.T) {
	assert.Equal(t, []string{
		`main.go:26: E-LINT-007: the SQL passed to sql.DB.ExecContext is not a constant`,
		`main.go:27: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:28: E-LINT-007: the SQL passed to sql.Tx.Prepare is not a constant`,
		`main.go:29: E-LINT-007: sql.DB.QueryContext is used as a value: call it directly, with constant SQL`,
		`main.go:46: E-LINT-007: the SQL passed to sql.DB.ExecContext is not a constant`,
		`main.go:47: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:50: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:51: E-LINT-007: the SQL passed to sql.DB.QueryContext comes from a call with several results, so it is not a constant`,
		`main.go:52: E-LINT-007: the SQL passed to sql.DB.QueryContext comes from a call with several results, so it is not a constant`,
		`main.go:56: E-LINT-007: the SQL passed to sql.DB.ExecContext holds an executable comment, which runs as SQL that lint does not read`,
		`main.go:57: E-LINT-007: the SQL passed to sql.DB.ExecContext holds an executable comment, which runs as SQL that lint does not read`,
		`main.go:69: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:75: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:78: E-LINT-007: the SQL passed to sql.DB.QueryContext is not a constant`,
		`main.go:83: E-LINT-007: Query has type parameters in its signature, so lint cannot tell whether it takes SQL: give the method another name`,
		`main.go:97: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, CREATE, which app code may not make`,
		`main.go:98: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, ALTER, which app code may not make`,
		`main.go:99: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, CREATE, which app code may not make`,
		`main.go:100: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, CREATE, which app code may not make`,
		`main.go:101: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, DROP, which app code may not make`,
		`main.go:102: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, DROP, which app code may not make`,
		`main.go:104: E-LINT-007: the SQL passed to sql.DB.ExecContext is not one of the statement kinds lint allows: SELECT, INSERT, UPDATE, DELETE, REPLACE and WITH`,
		`main.go:105: E-LINT-007: the SQL passed to sql.DB.ExecContext is not one of the statement kinds lint allows: SELECT, INSERT, UPDATE, DELETE, REPLACE and WITH`,
		`main.go:106: E-LINT-007: the SQL passed to sql.DB.ExecContext is not one of the statement kinds lint allows: SELECT, INSERT, UPDATE, DELETE, REPLACE and WITH`,
		`main.go:107: E-LINT-007: the SQL passed to sql.DB.ExecContext is not one of the statement kinds lint allows: SELECT, INSERT, UPDATE, DELETE, REPLACE and WITH`,
		`main.go:108: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, TRUNCATE, which app code may not make`,
		`main.go:109: E-LINT-007: the SQL passed to sql.DB.ExecContext is not one of the statement kinds lint allows: SELECT, INSERT, UPDATE, DELETE, REPLACE and WITH`,
		`main.go:115: E-LINT-007: the SQL passed to sql.DB.ExecContext holds more than one statement: pass each statement in a call of its own`,
		`main.go:116: E-LINT-007: the SQL passed to sql.DB.ExecContext holds more than one statement: pass each statement in a call of its own`,
		`main.go:117: E-LINT-007: the SQL passed to sql.DB.QueryRowContext reads or writes a file of the database server with INTO OUTFILE, INTO DUMPFILE or LOAD_FILE`,
		`main.go:118: E-LINT-007: the SQL passed to sql.DB.QueryRowContext reads or writes a file of the database server with INTO OUTFILE, INTO DUMPFILE or LOAD_FILE`,
		`main.go:119: E-LINT-007: the SQL passed to sql.DB.QueryRowContext reads or writes a file of the database server with INTO OUTFILE, INTO DUMPFILE or LOAD_FILE`,
		`main.go:127: E-LINT-013: the SQL passed to sql.Tx.ExecContext is a schema change, CREATE, which app code may not make`,
		`main.go:128: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, RENAME, which app code may not make`,
		`main.go:129: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, DROP, which app code may not make`,
		`main.go:130: E-LINT-013: the SQL passed to sql.DB.Exec is a schema change, CREATE, which app code may not make`,
		`main.go:131: E-LINT-013: the SQL passed to sql.DB.ExecContext is a schema change, ALTER, which app code may not make`,
		`main.go:138: E-LINT-013: the SQL passed to sql.DB.QueryRowContext names aicoded_migrations, the platform's journal of migrations, which app code may not read or write`,
		`main.go:139: E-LINT-013: the SQL passed to sql.Tx.ExecContext names aicoded_migrations, the platform's journal of migrations, which app code may not read or write`,
		`main.go:140: E-LINT-013: the SQL passed to sql.DB.ExecContext names aicoded_migrations, the platform's journal of migrations, which app code may not read or write`,
		`main.go:141: E-LINT-013: the SQL passed to sql.DB.ExecContext names aicoded_migrations, the platform's journal of migrations, which app code may not read or write`,
	}, run(t, "sql"), "a constant expression is a constant, in every form a method that takes SQL is called, an allowed statement kind, no schema change, not the journal, one statement and no file of the server")

	r, err := lint.Run(t.Context(), filepath.Join("testdata", "sql"), lint.Config{})
	require.NoError(t, err)
	for _, p := range r.Problems {
		if strings.Contains(p.Msg, "journal") {
			assert.Equal(t, "do not read or write aicoded_migrations; schema changes go in .aicoded/migration.sql", p.Fix, p.Pos)
		} else {
			assert.Equal(t, rule(t, p.Code).Fix, p.Fix, p.Pos)
		}
	}
	assert.Equal(t, []lint.Write{
		{Table: "aicoded_migrations_log", Op: "DELETE"},
		{Table: "app.tags", Op: "REPLACE"},
		{Table: "drafts", Op: "DELETE"},
		{Table: "notes", Op: "DELETE"},
		{Table: "notes", Op: "INSERT"},
		{Table: "notes", Op: "UPDATE"},
	}, r.Writes, "the writes of the constant SQL")
}

func TestTemplate(t *testing.T) {
	assert.Equal(t, []string{
		`pages/index.html:11: E-LINT-007: the SQL passed to sql.DB.QueryRow is not a constant`,
		`pages/index.html:13: E-LINT-006: app code may not use sql.DB.Driver`,
		`pages/index.html:14: E-LINT-009: app code may not use http.Get`,
		`pages/index.html:16: E-LINT-009: app code may not change time.Local`,
		`pages/index.html:17: E-LINT-006: app code may not use sql.DB.Driver`,
		`pages/route_gen.go:17: E-LINT-009: app code may not use http.DefaultClient`,
		`pages/route_gen.go:65: E-LINT-009: app code may not use http.DefaultTransport`,
		`pages/store.go:14: E-LINT-007: the SQL passed to sql.DB.QueryRow is not a constant`,
	}, run(t, "template", "pages/route_gen.go"), "generated code at the lines its //line comments name in the app folder, a hand-written file at its own")

	r, err := lint.Run(t.Context(), filepath.Join("testdata", "template"), lint.Config{Generated: []string{"pages/route_gen.go"}})
	require.NoError(t, err)
	assert.Equal(t, []lint.Write{{Table: "carts", Op: "DELETE"}}, r.Writes, "the writes of a template's constant SQL")
}

func TestCookies(t *testing.T) {
	const (
		name   = "E-LINT-012: a cookie that app code sets must have a Name that is a constant starting with __Host-"
		path   = `E-LINT-012: a cookie that app code sets must have Path: "/"`
		secure = "E-LINT-012: a cookie that app code sets must have Secure: true"
		domain = "E-LINT-012: a cookie that app code sets may not have a Domain"
		zero   = `E-LINT-012: make a cookie with an http.Cookie literal that sets its Name, Path: "/" and Secure: true, and no Domain, not as an empty value`
		header = "E-LINT-012: the value of a Set-Cookie header must be the String() of an http.Cookie, or a constant __Host- cookie with Path=/, Secure and no Domain"
		byKey  = `E-LINT-012: set a cookie with w.Header().Add("Set-Cookie", c.String()), not by its key in a header map`
		read   = "E-LINT-012: app code reads only cookies whose name starts with __Host-, which no other app can set"
		value  = " is used as a value: call it directly, so that lint reads the cookie it sets or reads"
	)
	assert.Equal(t, []string{
		"bad/bad.go:19: " + name,
		"bad/bad.go:20: " + secure,
		"bad/bad.go:21: " + domain,
		"bad/bad.go:23: " + name,
		"bad/bad.go:23: " + secure,
		"bad/bad.go:24: " + name,
		"bad/bad.go:25: " + name,
		"bad/bad.go:27: " + name,
		"bad/bad.go:28: " + secure,
		"bad/bad.go:29: " + domain,
		"bad/bad.go:30: E-LINT-012: set the Name, Path, Secure and Domain of a cookie only with = and a constant for each field",
		"bad/bad.go:31: E-LINT-012: app code may not take the address of the Name, Path, Secure or Domain of a cookie",
		"bad/bad.go:32: " + zero,
		"bad/bad.go:33: " + zero,
		"bad/bad.go:35: " + header,
		"bad/bad.go:36: " + header,
		"bad/bad.go:37: " + header,
		"bad/bad.go:38: " + byKey,
		"bad/bad.go:39: " + byKey,
		"bad/bad.go:40: " + header,
		"bad/bad.go:41: E-LINT-009: app code may not use http.SetCookie",
		"bad/bad.go:43: " + secure,
		"bad/bad.go:44: " + name,
		"bad/bad.go:50: " + path,
		"bad/bad.go:51: " + path,
		"bad/bad.go:52: " + path,
		"bad/bad.go:54: " + path,
		"bad/bad.go:55: " + header,
		"bad/bad.go:60: " + read,
		"bad/bad.go:61: " + read,
		"bad/bad.go:62: " + read,
		"bad/bad.go:63: E-LINT-012: http.Request.Cookie" + value,
		"bad/bad.go:65: E-LINT-012: http.Header.Add" + value,
		"bad/bad.go:67: E-LINT-012: http.Header.Set" + value,
	}, run(t, "cookies"), "good sets __Host- cookies with Path=/, Secure and no Domain, deletes one with MaxAge -1, reads __Host- cookies and one by a name lint cannot read, and sets other headers, also with a key lint cannot read")
}
