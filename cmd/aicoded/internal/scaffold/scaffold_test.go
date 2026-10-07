package scaffold

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate"
	"aicoded.dev/framework/cmd/aicoded/internal/gotool"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
	"aicoded.dev/framework/manifest"
)

// offline skips the test without the go command, and keeps go mod tidy to the module cache.
func offline(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	t.Setenv("GOPROXY", "off")
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	require.NoError(t, err)
	return string(data)
}

func TestCreate(t *testing.T) {
	offline(t)
	checkout, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	parent := t.TempDir()
	var out bytes.Buffer
	dir, err := Create(t.Context(), parent, "notes", Framework{Dir: checkout}, &out)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(parent, "notes"), dir)
	assert.Equal(t, "aicoded init: created "+dir+"\n"+
		"aicoded init: notes/go.mod uses the framework checkout at "+checkout+
		"; the delivery pipeline will refuse a replace line, so require a released version before you publish\n", out.String())

	gomod := read(t, dir, "go.mod")
	assert.Contains(t, gomod, "module notes\n\ngo 1.25.0\n")
	assert.Contains(t, gomod, "require aicoded.dev/framework v0.0.0-00010101000000-000000000000\n")
	assert.Contains(t, gomod, "\nreplace aicoded.dev/framework => "+checkout+"\n")
	assert.FileExists(t, filepath.Join(dir, "go.sum"))
	assert.Equal(t, `package main

import (
	"aicoded.dev/framework/app"

	"notes/pages"
)

func main() {
	app.Main(app.Options{Handler: pages.NewHandler()})
}
`, read(t, dir, "main.go"))
	assert.Equal(t, `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<title>notes</title>
<ssr:assets/>
</head>
<body>
<h1>notes</h1>
</body>
</html>
`, read(t, dir, "pages/index.html"))
	assert.Equal(t, agents, read(t, dir, "AGENTS.md"))
	for _, rel := range []string{"pages/route_gen.go", "pages/handler_gen.go", "pages/dataprovider.go"} {
		assert.FileExists(t, filepath.Join(dir, rel))
	}
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	require.NoError(t, err)
	assert.Equal(t, "notes", m.App)
	assert.Equal(t, map[string]manifest.Access{"/": {Require: []string{"*"}}}, m.Access)

	problems, err := gotool.Build(t.Context(), dir)
	require.NoError(t, err)
	assert.Empty(t, problems)
	res, err := generate.Run(dir, generate.Options{Frozen: true})
	require.NoError(t, err)
	found, err := lint.Run(t.Context(), dir, lint.Config{Generated: res.Files, FrameworkDir: checkout})
	require.NoError(t, err)
	assert.Empty(t, found.Problems, "a new app passes lint, with the replace line to this checkout")
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp folder is left")
	st, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), st.Mode().Perm(), "the app folder is private, as os.MkdirTemp makes it")
}

func TestAgents(t *testing.T) {
	root, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "AGENTS.md"))
	require.NoError(t, err)
	assert.Equal(t, agents, string(root), "AGENTS.md at the repository root is a copy of this package's AGENTS.md")
	assert.LessOrEqual(t, strings.Count(agents, "\n"), 150)
	assert.NotContains(t, agents, "](", "AGENTS.md has no Markdown link")
	assert.NotRegexp(t, `[^\x00-\x7f]`, agents, "AGENTS.md is plain ASCII")
}

// packagePaths maps the package names of the calls AGENTS.md bans to their import paths.
var packagePaths = map[string]string{
	"http": "net/http",
	"rpc":  "aicoded.dev/framework/rpc",
	"sql":  "database/sql",
	"web":  "aicoded.dev/framework/web",
}

func TestAgentsBans(t *testing.T) {
	imports := map[string]string{}
	for _, path := range bans(t, "Never import ") {
		imports[path] = lint.Banned(path, "")
	}
	assert.Equal(t, map[string]string{
		"os": "E-LINT-001", "os/exec": "E-LINT-001", "net": "E-LINT-001", "syscall": "E-LINT-001",
		"reflect": "E-LINT-001", "unsafe": "E-LINT-001", "log": "E-LINT-001",
	}, imports, "the bullet names the imports in code spans before its first \": \", each refused by the rule given")
	calls := map[string]string{}
	for _, call := range bans(t, "Never call ") {
		pkg, name, _ := strings.Cut(call, ".")
		path, ok := packagePaths[pkg]
		require.True(t, ok, "add the import path of %s to packagePaths", pkg)
		calls[call] = lint.Banned(path, name)
	}
	assert.Equal(t, map[string]string{
		"sql.Open": "E-LINT-006", "sql.OpenDB": "E-LINT-006", "rpc.Call": "E-LINT-005", "web.New": "E-LINT-005",
		"http.Get": "E-LINT-009", "http.Post": "E-LINT-009", "http.ListenAndServe": "E-LINT-009", "http.NewServeMux": "E-LINT-009",
	}, calls, "the bullet names the calls in code spans before its first \": \", each refused by the rule given")
}

func TestAgentsPackages(t *testing.T) {
	var paths []string
	for _, name := range bans(t, "App code imports the framework's packages ") {
		paths = append(paths, lint.Framework+"/"+name)
	}
	assert.Equal(t, lint.Blocks(), paths, "the bullet names, before its first \": \", the framework's packages that lint lets app code import")
}

var codeSpan = regexp.MustCompile("`([^`]+)`")

// bans returns the code spans of the bullet of AGENTS.md that starts with prefix, before the
// first ": " of the bullet.
func bans(t *testing.T, prefix string) []string {
	t.Helper()
	_, bullet, ok := strings.Cut(agents, "\n- "+prefix)
	require.True(t, ok, "AGENTS.md has a bullet that starts with %q", "- "+prefix)
	bullet, _, _ = strings.Cut(strings.ReplaceAll(bullet, "\n  ", " "), "\n")
	bullet, _, _ = strings.Cut(bullet, ": ")
	var out []string
	for _, m := range codeSpan.FindAllStringSubmatch(bullet, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestCreateRefuses(t *testing.T) {
	parent := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(parent, "taken"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "taken-file"), nil, 0o600))
	for _, name := range []string{"Bad_Name", "../x", "taken", "taken-file"} {
		_, err := Create(t.Context(), parent, name, Framework{Version: "v1.0.0"}, io.Discard)
		assert.Equal(t, "E-CLI-002", errs.Code(err), name)
	}
	_, err := Create(t.Context(), parent, "notes", Framework{}, io.Discard)
	assert.Equal(t, "E-CLI-003", errs.Code(err), "no framework to require")

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "nothing is created")
	assert.NoDirExists(t, filepath.Join(filepath.Dir(parent), "x"))
	inside, err := os.ReadDir(filepath.Join(parent, "taken"))
	require.NoError(t, err)
	assert.Empty(t, inside)
}

func TestCreateCleansUpOnFailure(t *testing.T) {
	offline(t)
	parent := t.TempDir()
	var out bytes.Buffer
	_, err := Create(t.Context(), parent, "notes", Framework{Version: "v9.9.9"}, &out)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-CHK-006", e.Code, "go mod tidy cannot find v9.9.9")
	assert.Equal(t, "check the network, then run aicoded init notes again", e.Fix)
	assert.Empty(t, out.String())
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	assert.Empty(t, entries, "the temp folder is removed")
}

func TestDetect(t *testing.T) {
	_, err := Detect("")
	assert.Equal(t, "E-CLI-003", errs.Code(err), "a test binary gets the framework through a replace line")

	fw, err := Detect("/x")
	require.NoError(t, err)
	assert.Equal(t, Framework{Dir: "/x"}, fw)
	fw, err = Detect("x")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(fw.Dir), fw.Dir)
}

func TestReleased(t *testing.T) {
	info := func(dep debug.Module) *debug.BuildInfo {
		return &debug.BuildInfo{Deps: []*debug.Module{{Path: "example.com/other", Version: "v1.0.0"}, &dep}}
	}
	assert.Equal(t, "v1.2.3", released(info(debug.Module{Path: "aicoded.dev/framework", Version: "v1.2.3"})))
	for name, dep := range map[string]debug.Module{
		"a checkout":     {Path: "aicoded.dev/framework", Version: "v0.0.0-00010101000000-000000000000"},
		"a replace line": {Path: "aicoded.dev/framework", Version: "v1.2.3", Replace: &debug.Module{Path: "../.."}},
		"a devel build":  {Path: "aicoded.dev/framework", Version: "(devel)"},
		"no framework":   {Path: "example.com/x", Version: "v1.2.3"},
	} {
		assert.Empty(t, released(info(dep)), name)
	}
	assert.Empty(t, released(nil))
}
