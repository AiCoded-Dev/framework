package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/describe"
	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/explain"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/docs"
)

func TestRun(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"version"}, &out, &errOut))
	assert.Equal(t, "aicoded devel\n", out.String())

	assert.Equal(t, 2, run(nil, &out, &errOut))
	assert.Equal(t, 2, run([]string{"nope"}, &out, &errOut))
	assert.Contains(t, errOut.String(), `unknown command "nope"`)
}

func TestRunDevWithoutManifest(t *testing.T) {
	testhome.Set(t)
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, run([]string{"dev", t.TempDir()}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-MAN-006")
}

func TestRunDevManual(t *testing.T) {
	testhome.Set(t)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "aicoded.yaml"), []byte("app: hello\n"), 0o600))
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, run([]string{"dev", "--manual", "hello", "--manual", "nope", root}, &out, &errOut))
	assert.Contains(t, errOut.String(), `E-DEV-014: no app named "nope"`, "every --manual reaches aicoded dev")

	errOut.Reset()
	assert.Equal(t, 2, run([]string{"dev", root, "--manual", "hello"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "aicoded dev: too many arguments", "--manual comes before the folder")
	assert.Equal(t, 2, run([]string{"dev", "--manual"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "flag needs an argument: -manual")
}

func TestRunDevPointsAtTheRunningOne(t *testing.T) {
	home := testhome.Set(t)
	root := filepath.Join(home, "work")
	require.NoError(t, os.MkdirAll(root, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "aicoded.yaml"), []byte("app: hello\n"), 0o600))
	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	testhome.DevYAML(t, home, fmt.Sprintf("workspaces:\n  %s:\n    port: %d\n", root, port))
	w, err := dev.Host(t.Context(), root, io.Discard, dev.Options{})
	require.NoError(t, err)
	t.Cleanup(w.Close)
	state, err := dev.StatePath(root)
	require.NoError(t, err)
	token, err := os.ReadFile(filepath.Join(state, "ui-token"))
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	assert.Equal(t, 1, run([]string{"dev", root}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-DEV-012: aicoded dev already runs for "+root)
	assert.True(t, strings.HasSuffix(errOut.String(), fmt.Sprintf("\naicoded dev: log in to the running one at http://localhost:%d/_aicoded/login?token=%s\n", port, token)), errOut.String())
}

func TestRunRPCAdd(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"rpc", "add"}, &out, &errOut))
	assert.Equal(t, 2, run([]string{"rpc", "remove", "billing"}, &out, &errOut))

	shop := filepath.Join(t.TempDir(), "shop")
	require.NoError(t, os.Mkdir(shop, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(shop, "aicoded.yaml"), []byte("app: shop\n"), 0o600))
	t.Chdir(shop)
	assert.Equal(t, 1, run([]string{"rpc", "add", "billing"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-RPC-007")
}

func TestRunGenerate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("internal/generate/testdata/app")))

	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"generate", dir}, &out, &errOut))
	assert.Empty(t, errOut.String())
	assert.FileExists(t, filepath.Join(dir, "pages", "handler_gen.go"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "pages", "empty", "index.html"), []byte(`<p style="x"></p>`), 0o600))
	assert.Equal(t, 1, run([]string{"generate", dir}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-GEN-")
}

func TestRunUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"version", "extra"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "aicoded version: too many arguments")
	assert.Equal(t, 2, run([]string{"dev", "-x"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "flag provided but not defined: -x")

	errOut.Reset()
	assert.Equal(t, 0, run([]string{"dev", "-h"}, &out, &errOut))
	assert.Contains(t, errOut.String(), usage)
}

func TestExplain(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"explain", "e-dev-001"}, &out, &errOut))
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "errors", "E-DEV-001.md"))
	require.NoError(t, err)
	assert.Equal(t, string(want), out.String())
	assert.Empty(t, errOut.String())
}

func TestExplainTopic(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"explain", "guides/overview"}, &out, &errOut))
	want, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", "overview.md"))
	require.NoError(t, err)
	assert.Equal(t, string(want), out.String())
	assert.Empty(t, errOut.String())
}

func TestExplainList(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"explain"}, &out, &errOut))
	topics, err := docs.Topics()
	require.NoError(t, err)
	entries, err := explain.List()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	assert.Len(t, lines, len(topics)+len(entries))
	assert.Equal(t, topics[0].Path+"  "+topics[0].Title, lines[0])
	assert.Contains(t, lines, "changelog  Changelog for AI assistants")
	assert.Equal(t, entries[0].Code+"  "+entries[0].Title, lines[len(topics)])
	assert.Contains(t, lines, "E-DEV-001  dev.yaml is readable by other users")
}

func TestExplainUnknown(t *testing.T) {
	for _, topic := range []string{"E-NOPE-001", "guides/nope"} {
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, run([]string{"explain", topic}, &out, &errOut))
		assert.Contains(t, errOut.String(), "E-CLI-001", topic)
		assert.Empty(t, out.String(), topic)
	}
}

func TestExplainUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"explain", "E-DEV-001", "E-DEV-002"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "aicoded explain: too many arguments")
	assert.Empty(t, out.String())
}

func TestRunGenerateWarnings(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS("internal/generate/testdata/app")))
	script := filepath.Join(dir, "pages", "notes", "index.ts")
	data, err := os.ReadFile(script)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(script, append(data, "console.log({a: 1, a: 2});\n"...), 0o600))

	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run([]string{"generate", dir}, &out, &errOut), "a warning does not fail the run")
	assert.Equal(t, "aicoded generate: warning: pages/notes/index.ts:6: Duplicate key \"a\" in object literal\n", errOut.String())
}

// testApp writes an app named calc into a temp folder, without the framework, with the test
// test, and returns the folder.
func testApp(t *testing.T, test string) string {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"aicoded.yaml": "app: calc\n# access is written by aicoded generate from <ssr:access>; do not edit it\naccess: {}\n",
		"go.mod":       "module calc\n\ngo 1.25.0\n",
		"main.go":      "package main\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCalc(t *testing.T) {\n\t" + test + "\n}\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600))
	}
	return dir
}

func TestRunCheck(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"check", "a", "b"}, &out, &errOut))
	assert.Equal(t, 2, run([]string{"check", "--nope"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "(--frozen, --no-tests, --json, --app <name>)")
	assert.Equal(t, 1, run([]string{"check", t.TempDir()}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-MAN-006")

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	out.Reset()
	assert.Equal(t, 1, run([]string{"check", "--json", testApp(t, `t.Error("fails")`)}, &out, &errOut))
	var report check.Report
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	require.Len(t, report.Apps, 1)
	require.Len(t, report.Apps[0].Problems, 1)
	assert.Equal(t, "E-CHK-004", report.Apps[0].Problems[0].Code)

	out.Reset()
	assert.Equal(t, 0, run([]string{"check", "--no-tests", "--json", testApp(t, `t.Error("fails")`)}, &out, &errOut))
	report = check.Report{}
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))
	assert.True(t, report.NoTests)
	assert.Empty(t, report.Apps[0].Problems, "the tests did not run")

	out.Reset()
	assert.Equal(t, 0, run([]string{"check", "--app", "calc", testApp(t, "")}, &out, &errOut))
	assert.Contains(t, out.String(), "ok  calc\n")
	assert.Contains(t, out.String(), "aicoded check: ok\n")
}

func TestRunDescribe(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"describe", "a", "b"}, &out, &errOut))
	dir := testApp(t, "")
	assert.Equal(t, 1, run([]string{"describe", "--app", "ledger", dir}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-DEV-014")

	out.Reset()
	assert.Equal(t, 0, run([]string{"describe", "--json", dir}, &out, &errOut))
	var summary describe.Summary
	require.NoError(t, json.Unmarshal(out.Bytes(), &summary))
	require.Len(t, summary.Apps, 1)
	assert.Equal(t, "calc", summary.Apps[0].Name)

	out.Reset()
	assert.Equal(t, 0, run([]string{"describe", dir}, &out, &errOut))
	assert.True(t, strings.HasPrefix(out.String(), "app calc ("), out.String())
}

func TestRunInit(t *testing.T) {
	checkout, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	var out, errOut bytes.Buffer
	assert.Equal(t, 2, run([]string{"init"}, &out, &errOut))
	t.Chdir(t.TempDir())
	assert.Equal(t, 1, run([]string{"init", "notes"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-CLI-003", "a test binary knows no framework version")
	assert.NoDirExists(t, "notes")

	old := frameworkDir
	t.Cleanup(func() { frameworkDir = old })
	frameworkDir = checkout
	assert.Equal(t, 1, run([]string{"init", "Bad_Name"}, &out, &errOut))
	assert.Contains(t, errOut.String(), "E-CLI-002")

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	t.Setenv("GOPROXY", "off")
	out.Reset()
	assert.Equal(t, 0, run([]string{"init", "notes"}, &out, &errOut), errOut.String())
	assert.Contains(t, out.String(), "aicoded init: created ")
	assert.FileExists(t, filepath.Join("notes", "aicoded.yaml"))
}
