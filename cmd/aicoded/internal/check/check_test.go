package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/surface"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
)

func requireGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

func write(t *testing.T, path, data string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func read(t *testing.T, path string) string {
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// checkout returns this framework checkout, which the apps of these tests replace the framework
// with.
func checkout(t *testing.T) string {
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	return dir
}

// tempRoot returns a temp folder for apps, with the symbolic links in its path resolved.
func tempRoot(t *testing.T) string {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return root
}

// hello copies the hello app of the dev runner's tests into root/name, as copyApp does.
func hello(t *testing.T, root, name string) string {
	return copyApp(t, filepath.Join("..", "dev", "testdata", "hello"), root, name)
}

// pagesApp copies the one-page app testdata/shop into root/name, as copyApp does.
func pagesApp(t *testing.T, root, name string) string {
	return copyApp(t, filepath.Join("testdata", "shop"), root, name)
}

// copyApp copies the app in the folder src into root/name under the name name, with its replace
// directive pointing at this checkout, and returns its folder.
func copyApp(t *testing.T, src, root, name string) string {
	dir := filepath.Join(root, name)
	require.NoError(t, os.CopyFS(dir, os.DirFS(src)))
	gomod := filepath.Join(dir, "go.mod")
	write(t, gomod, strings.Replace(read(t, gomod), "=> ../../../../../..", "=> "+checkout(t), 1))
	yaml := filepath.Join(dir, "aicoded.yaml")
	write(t, yaml, regexp.MustCompile(`(?m)^app: .*$`).ReplaceAllLiteralString(read(t, yaml), "app: "+name))
	return dir
}

// calc writes an app named name into root/name, without the framework, whose one test fails
// when fails is set, and returns its folder.
func calc(t *testing.T, root, name string, fails bool) string {
	dir := filepath.Join(root, name)
	sum := "4"
	if fails {
		sum = "5"
	}
	write(t, filepath.Join(dir, "aicoded.yaml"), "app: "+name+"\n"+
		"# access is written by aicoded generate from <ssr:access>; do not edit it\naccess: {}\n")
	write(t, filepath.Join(dir, "go.mod"), "module "+name+"\n\ngo 1.25.0\n")
	write(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	write(t, filepath.Join(dir, "calc", "calc.go"), "// Package calc adds.\npackage calc\n\n// Add returns a plus b.\nfunc Add(a, b int) int { return a + b }\n")
	write(t, filepath.Join(dir, "calc", "calc_test.go"), `package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 2) != `+sum+` {
		t.Error("wrong sum")
	}
}
`)
	return dir
}

// record makes the steps of a check, and the probe of -race, note the app folder they run in.
func record(t *testing.T) *[]string {
	var ran []string
	pre, probe, build, vet, analyze, test := precheck, probeRace, goBuild, goVet, runLint, goTest
	t.Cleanup(func() { precheck, probeRace, goBuild, goVet, runLint, goTest = pre, probe, build, vet, analyze, test })
	precheck = func(ctx context.Context, dir string) ([]*errs.Error, error) {
		ran = append(ran, "precheck "+filepath.Base(dir))
		return pre(ctx, dir)
	}
	probeRace = func(ctx context.Context, dir string) (bool, string, error) {
		ran = append(ran, "race "+filepath.Base(dir))
		return probe(ctx, dir)
	}
	goBuild = func(ctx context.Context, dir string) ([]*errs.Error, error) {
		ran = append(ran, "build "+filepath.Base(dir))
		return build(ctx, dir)
	}
	goVet = func(ctx context.Context, dir string) ([]*errs.Error, error) {
		ran = append(ran, "vet "+filepath.Base(dir))
		return vet(ctx, dir)
	}
	runLint = func(ctx context.Context, dir string, cfg lint.Config) (lint.Result, error) {
		ran = append(ran, "lint "+filepath.Base(dir))
		return analyze(ctx, dir, cfg)
	}
	goTest = func(ctx context.Context, dir string, race bool) ([]*errs.Error, error) {
		ran = append(ran, "test "+filepath.Base(dir))
		return test(ctx, dir, race)
	}
	return &ran
}

// brief is the part of a problem the tests compare.
type brief struct{ App, Code, Pos string }

func briefs(ps []problem.Problem) []brief {
	var out []brief
	for _, p := range ps {
		out = append(out, brief{p.App, p.Code, p.Pos})
	}
	return out
}

func TestCheckOK(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := pagesApp(t, root, "shop")
	stale := filepath.Join(dir, "services", "billing", "client_gen.go")
	write(t, stale, "package billing\n")
	ran := record(t)

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	assert.True(t, r.OK())
	assert.Equal(t, []App{{Name: "shop", Dir: dir, Changed: []string{"services/billing/client_gen.go"},
		Surface: &surface.Surface{Size: 1, EntryPoints: 1}}}, r.Apps, "one page")
	assert.Equal(t, []string{"precheck shop", "race shop", "build shop", "vet shop", "lint shop", "test shop"}, *ran)
	assert.NoFileExists(t, stale)
}

func TestCheckLint(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	hello(t, root, "hello")
	ran := record(t)

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	assert.Equal(t, []brief{
		{"hello", "E-LINT-001", "main.go:9"},
		{"hello", "E-LINT-009", "main.go:20"},
		{"hello", "E-LINT-004", "main.go:28"},
		{"hello", "E-LINT-009", "main.go:35"},
		{"hello", "E-LINT-009", "main.go:45"},
		{"hello", "E-LINT-009", "main.go:51"},
		{"hello", "E-LINT-009", "main.go:64"},
		{"hello", "E-LINT-008", "main.go:68"},
		{"hello", "E-LINT-008", "main.go:70"},
	}, briefs(r.Apps[0].Problems), "os, a raw mux as the handler, the parts of net/http that serve, and the secret it leaks")
	assert.Equal(t, `app code may not import "os"`, r.Apps[0].Problems[0].Message)
	assert.Equal(t, "app code may not use http.NewServeMux", r.Apps[0].Problems[1].Message)
	assert.Equal(t, "a secret reaches telemetry.Span.SetAttr", r.Apps[0].Problems[7].Message)
	assert.Equal(t, []string{"precheck hello", "race hello", "build hello", "vet hello", "lint hello"}, *ran, "the tests do not run")
}

func TestCheckLintTemplate(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := pagesApp(t, root, "shop")
	write(t, filepath.Join(dir, "pages", "store.go"), "package pages\n\nimport \"database/sql\"\n\n// Store holds the database.\ntype Store struct{ DB *sql.DB }\n")
	write(t, filepath.Join(dir, "pages", "index.html"), `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<ssr:var name="store" type="Store"/>
<ssr:var name="q" type="string"/>
<title>Shop</title>
</head>
<body>
<p>{{ store.DB.QueryRow('SELECT ' + q).Err() }}</p>
<p>{{ store.DB.Driver() }}</p>
</body>
</html>
`)

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	require.Equal(t, []brief{
		{"shop", "E-LINT-007", "pages/index.html:11"},
		{"shop", "E-LINT-006", "pages/index.html:12"},
	}, briefs(r.Apps[0].Problems), "the code of the template's expressions, at their lines")
	assert.Equal(t, "the SQL passed to sql.DB.QueryRow is not a constant", r.Apps[0].Problems[0].Message)
}

func TestCheckLintTemplateLogs(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := pagesApp(t, root, "shop")
	write(t, filepath.Join(dir, "pages", "find", "logger.go"), "package find\n\nimport \"log/slog\"\n\n// Logger is the logger of the page.\ntype Logger = slog.Logger\n")
	write(t, filepath.Join(dir, "pages", "find", "index.html"), `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<ssr:var name="log" type="*Logger"/>
<title>Find</title>
</head>
<body>
<ssr:form name="find">
<ssr:input name="q" type="text"/>
<p>{{ log.With('q', form.Q.GetValue()) }}</p>
</ssr:form>
</body>
</html>
`)

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	require.Equal(t, []brief{{"shop", "E-LINT-008", "pages/find/index.html:12"}}, briefs(r.Apps[0].Problems),
		"the template's expression logs a form value")
	assert.Equal(t, "a form value reaches slog.Logger.With", r.Apps[0].Problems[0].Message)
}

func TestCheckLintCannotRun(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	pagesApp(t, root, "shop")
	analyze := runLint
	t.Cleanup(func() { runLint = analyze })
	runLint = func(context.Context, string, lint.Config) (lint.Result, error) {
		return lint.Result{}, errs.New("E-CHK-007", "aicoded was built with go1.25.8, and the go command is go1.26.8", "fix")
	}
	ran := record(t)

	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.False(t, r.OK(), "an app that lint cannot read does not pass")
	assert.Equal(t, []brief{{"shop", "E-CHK-007", ""}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, []string{"precheck shop", "race shop", "build shop", "vet shop", "lint shop"}, *ran, "the tests do not run")
}

func TestCheckLintLoadError(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	pagesApp(t, root, "shop")
	analyze := runLint
	t.Cleanup(func() { runLint = analyze })
	runLint = func(context.Context, string, lint.Config) (lint.Result, error) {
		return lint.Result{}, errors.New("lint cannot load package shop/pages: pages/x.go:3:9: undefined: y")
	}
	ran := record(t)

	_, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.EqualError(t, err, "lint cannot load package shop/pages: pages/x.go:3:9: undefined: y", "an uncoded lint error stops the whole check")
	assert.Equal(t, []string{"precheck shop", "race shop", "build shop", "vet shop", "lint shop"}, *ran)
}

func TestCheckLintGetsModules(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	yaml := filepath.Join(pagesApp(t, root, "shop"), "aicoded.yaml")
	write(t, yaml, strings.Replace(read(t, yaml), "app: shop\n", "app: shop\nmodules: [golang.org/x/text]\n", 1))
	analyze := runLint
	t.Cleanup(func() { runLint = analyze })
	var got lint.Config
	runLint = func(_ context.Context, _ string, cfg lint.Config) (lint.Result, error) {
		got = cfg
		return lint.Result{}, nil
	}

	r, err := Run(t.Context(), root, Options{FrameworkDir: "/fw"})
	require.NoError(t, err)
	assert.True(t, r.OK())
	assert.Equal(t, []string{"golang.org/x/text"}, got.Modules, "the modules: of the permission list")
	assert.Contains(t, got.Generated, "pages/route_gen.go")
	assert.Equal(t, "/fw", got.FrameworkDir)
}

func TestCheckReplace(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	pagesApp(t, root, "shop")
	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"shop", "E-LINT-011", "go.mod:13"}}, briefs(r.Apps[0].Problems), "an aicoded not installed from a checkout accepts no replace")

	r, err = Run(t.Context(), root, Options{FrameworkDir: t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"shop", "E-LINT-011", "go.mod:13"}}, briefs(r.Apps[0].Problems), "nor one to a checkout other than its own")
}

func TestCheckFrozenDrift(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := filepath.Join(root, "shop")
	write(t, filepath.Join(dir, "aicoded.yaml"), "app: shop\n")
	write(t, filepath.Join(dir, "go.mod"), "module shop\n\ngo 1.25.0\n")
	write(t, filepath.Join(dir, "pages", "index.html"), `<!doctype html><html><head><ssr:access role="*"/><title>Shop</title></head><body></body></html>`)
	require.NoError(t, generate.Generate(dir))
	route := filepath.Join(dir, "pages", "route_gen.go")
	edited := read(t, route) + "// edited\n"
	write(t, route, edited)
	ran := record(t)

	r, err := Run(t.Context(), root, Options{Frozen: true})
	require.NoError(t, err)
	assert.False(t, r.OK())
	require.Len(t, r.Apps, 1)
	assert.Equal(t, []string{"pages/route_gen.go"}, r.Apps[0].Changed)
	assert.Equal(t, []brief{{"shop", "E-CHK-001", "pages/route_gen.go:1"}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, "pages/route_gen.go is out of date", r.Apps[0].Problems[0].Message)
	assert.Equal(t, []string{"precheck shop", "race shop"}, *ran, "nothing is built after generate fails")
	assert.Equal(t, edited, read(t, route), "a frozen check writes nothing")
}

func TestCheckStopsAfterBuildFailure(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := calc(t, root, "calc", true)
	write(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {\n\tundefined()\n}\n")
	ran := record(t)

	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"calc", "E-CHK-002", "main.go:4"}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, []string{"precheck calc", "race calc", "build calc"}, *ran, "vet and the tests do not run")
}

func TestCheckNamesFrameworkPackage(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	dir := pagesApp(t, root, "shop")
	write(t, filepath.Join(dir, "pages", "fields.go"), "package pages\n\nimport \"aicoded.dev/framework/form\"\n\nvar _ form.Textarea\n")

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	require.Equal(t, []brief{{"shop", "E-CHK-002", "pages/fields.go:3"}}, briefs(r.Apps[0].Problems), "the import's line")
	p := r.Apps[0].Problems[0]
	assert.Contains(t, p.Message, "\nthe framework has no package aicoded.dev/framework/form; its form package is aicoded.dev/framework/web/form")
	assert.Equal(t, `import "aicoded.dev/framework/web/form" instead`, p.Fix)
}

func TestCheckTestFailure(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	calc(t, root, "calc", true)
	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"calc", "E-CHK-004", "calc/calc_test.go:7"}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, "TestAdd failed: calc_test.go:7: wrong sum", r.Apps[0].Problems[0].Message)
}

func TestCheckOneApp(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	calc(t, root, "adder", true)
	summer := calc(t, root, "summer", false)

	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	require.Len(t, r.Apps, 2, "every app is checked")
	assert.Equal(t, []brief{{"adder", "E-CHK-004", "calc/calc_test.go:7"}}, briefs(r.Apps[0].Problems))
	assert.Empty(t, r.Apps[1].Problems)

	r, err = Run(t.Context(), root, Options{App: "summer"})
	require.NoError(t, err)
	assert.Equal(t, []App{{Name: "summer", Dir: summer, Surface: &surface.Surface{}}}, r.Apps)

	_, err = Run(t.Context(), root, Options{App: "ledger"})
	assert.Equal(t, "E-DEV-014", errs.Code(err))
}

func TestCheckRaceNote(t *testing.T) {
	requireGo(t)
	t.Setenv("CGO_ENABLED", "0")
	root := tempRoot(t)
	calc(t, root, "calc", false)
	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err)
	assert.False(t, r.Race)
	assert.Equal(t, []string{"tests ran without -race: cgo is off; the delivery pipeline will run them with -race"}, r.Notes)
}

func TestCheckNoTests(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	calc(t, root, "calc", true)
	ran := record(t)

	r, err := Run(t.Context(), root, Options{NoTests: true})
	require.NoError(t, err)
	assert.True(t, r.OK(), "the failing test does not run")
	assert.Equal(t, []string{"precheck calc", "build calc", "vet calc", "lint calc"}, *ran, "neither -race is probed nor the tests run")
	assert.False(t, r.Race)
	assert.Empty(t, r.Notes)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"no_tests":true`)
}

func TestCheckPrecheckFirst(t *testing.T) {
	requireGo(t)
	t.Setenv("GOTOOLCHAIN", "auto")
	t.Setenv("GOPROXY", "off")
	root := tempRoot(t)
	adder := calc(t, root, "adder", false)
	write(t, filepath.Join(adder, "go.mod"), "module adder\n\ngo 9.99\n")
	calc(t, root, "summer", false)
	ran := record(t)

	r, err := Run(t.Context(), root, Options{})
	require.NoError(t, err, "a first app whose go env fails does not stop the check")
	assert.Equal(t, []brief{{"adder", "E-LINT-011", "GOFLAGS"}}, briefs(r.Apps[0].Problems))
	assert.Regexp(t, "^lint cannot read GOFLAGS with go env: go: [^\n]+$", r.Apps[0].Problems[0].Message, "the first line of what go env printed")
	assert.Empty(t, r.Apps[1].Problems)
	assert.Equal(t, []string{"precheck adder", "precheck summer", "race summer", "build summer", "vet summer", "lint summer", "test summer"}, *ran,
		"no other go command runs for an app that the precheck refuses, and -race is probed in summer")
	for _, n := range r.Notes {
		assert.NotContains(t, n, "go env failed")
	}
}

func TestCheckPrecheckBeforeBuild(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	pagesApp(t, root, "shop")
	ran := record(t)
	marker := filepath.Join(t.TempDir(), "ran")
	tool := filepath.Join(t.TempDir(), "tool")
	write(t, tool, "#!/bin/sh\ntouch "+marker+"\nexec \"$@\"\n")
	require.NoError(t, os.Chmod(tool, 0o700))
	t.Setenv("GOFLAGS", "-toolexec="+tool)

	r, err := Run(t.Context(), root, Options{FrameworkDir: checkout(t)})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"shop", "E-LINT-011", "GOFLAGS"}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, []string{"precheck shop"}, *ran)
	assert.Empty(t, r.Notes, "-race is not probed when no app passes the precheck")
	assert.NoFileExists(t, marker, "the tool that GOFLAGS names never runs")
}

const (
	invoiceSnapshot = `{
  "app": "billing",
  "methods": {
    "GetInvoice": {"in": "InvoiceID", "out": "Invoice"}
  },
  "types": {
    "Invoice": {"fields": {"ID": {"number": 1, "type": "int64"}, "Total": {"number": 2, "type": "int64"}}},
    "InvoiceID": {"fields": {"ID": {"number": 1, "type": "int64"}}}
  }
}
`
	retiredTotal = `{
  "app": "billing",
  "methods": {"GetInvoice": {"in": "InvoiceID", "out": "Invoice"}},
  "types": {
    "Invoice": {"fields": {"ID": {"number": 1, "type": "int64"}}, "retired": [2]},
    "InvoiceID": {"fields": {"ID": {"number": 1, "type": "int64"}}}
  }
}
`
	ledgerSnapshot = `{"app": "ledger", "methods": {"Post": {"in": "Entry", "out": "Entry"}}, "types": {"Entry": {"fields": {"ID": {"number": 1, "type": "int64"}}}}}`
)

func TestCheckHeldSnapshots(t *testing.T) {
	requireGo(t)
	root := tempRoot(t)
	shop := pagesApp(t, root, "shop")
	calc(t, root, "billing", false)
	write(t, filepath.Join(shop, ".aicoded", "services", "ledger.json"), ledgerSnapshot)

	r, err := Run(t.Context(), root, Options{App: "shop", FrameworkDir: checkout(t)})
	require.NoError(t, err)
	assert.True(t, r.OK())
	assert.Contains(t, r.Notes, "shop holds a snapshot of ledger, which is not in this workspace")

	held := filepath.Join(shop, ".aicoded", "services", "billing.json")
	write(t, held, invoiceSnapshot)
	write(t, filepath.Join(root, "billing", ".aicoded", "rpc.json"), retiredTotal)
	ran := record(t)
	r, err = Run(t.Context(), root, Options{App: "shop", FrameworkDir: checkout(t)})
	require.NoError(t, err)
	assert.Equal(t, []brief{{"shop", "E-RPC-013", held + ":4"}}, briefs(r.Apps[0].Problems))
	assert.Equal(t, []string{"precheck shop", "race shop"}, *ran, "nothing is built with a broken held snapshot")
}

func TestCheckRefusesWorkspace(t *testing.T) {
	_, err := Run(t.Context(), t.TempDir(), Options{})
	assert.Equal(t, "E-MAN-006", errs.Code(err))
}

func TestWriteText(t *testing.T) {
	fail := problem.From("shop", errs.At("main.go:4", "E-CHK-002", "undefined: x", "fix the Go code at this line"))
	r := Report{
		Apps: []App{
			{Name: "billing", Dir: "/w/billing", Changed: []string{"rpc/server_gen.go"}, Warnings: []string{"pages/x.ts:3: duplicate key"},
				Surface: &surface.Surface{Size: 2, EntryPoints: 2, Effects: 1}},
			{Name: "shop", Dir: "/w/shop", Problems: append(fail, fail...)},
		},
		Notes: []string{"tests ran without -race: cgo is off; the delivery pipeline will run them with -race"},
	}
	var out bytes.Buffer
	require.NoError(t, r.WriteText(&out))
	assert.Equal(t, `ok  billing
  wrote rpc/server_gen.go
  warning: pages/x.ts:3: duplicate key
  surface 2 (2 entry points, 1 effect)
FAIL shop
  shop: main.go:4: E-CHK-002: undefined: x
    fix: fix the Go code at this line
    docs: https://aicoded.dev/docs/errors/E-CHK-002
  shop: main.go:4: E-CHK-002: undefined: x
    fix: fix the Go code at this line
    docs: https://aicoded.dev/docs/errors/E-CHK-002
tests ran without -race: cgo is off; the delivery pipeline will run them with -race
aicoded check: 2 problems in 1 app
`, out.String())

	r = Report{Frozen: true, Apps: []App{{Name: "billing", Changed: []string{"rpc/server_gen.go"}}}}
	out.Reset()
	require.NoError(t, r.WriteText(&out))
	assert.Equal(t, "ok  billing\n  out of date rpc/server_gen.go\naicoded check: ok\n", out.String())

	r = Report{NoTests: true, Apps: []App{{Name: "billing"}}}
	out.Reset()
	require.NoError(t, r.WriteText(&out))
	assert.Equal(t, "ok  billing\naicoded check: ok\ntests did not run (--no-tests)\n", out.String())
}
