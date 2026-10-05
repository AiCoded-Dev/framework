package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func TestAccessMap(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/index.html":             `<!doctype html><html><head><ssr:access role="*"/><ssr:assets/></head><body><ssr:content default="notes"/></body></html>`,
		"pages/notes/index.html":       `<ssr:access role="editor"/><ssr:call name="count" in="int" out="int"/><p>list</p>`,
		"pages/notes/n_id/index.html":  `<ssr:access role="*" guard="true"/><ssr:call name="star" in="int" out="int"/><p>note</p>`,
		"pages/admin/index.html":       `<ssr:access role="owner,admin"/><ssr:content/>`,
		"pages/admin/s_tab/index.html": `<ssr:access role="admin"/><p>tab</p>`,
	}), noImages)
	require.NoError(t, err)
	assert.Equal(t, map[string]accessEntry{
		"/notes":       {Require: []string{"editor"}, Calls: []string{"count"}},
		"/notes/{id}":  {Require: []string{"editor"}, Guard: true, Calls: []string{"star"}},
		"/admin/{tab}": {Require: []string{"admin|owner", "admin"}},
	}, accessMap(rs), "layouts with pages below are left out; the note inherits the gate's rule and not its calls")
}

func TestAccessMapGuardAndRepeats(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/a/index.html":   `<ssr:access role="x" guard="true"/><p>a</p>`,
		"pages/a/b/index.html": `<ssr:access role="x,x"/><p>b</p>`,
	}), noImages)
	require.NoError(t, err)
	assert.Equal(t, map[string]accessEntry{
		"/a":   {Require: []string{"x"}, Guard: true},
		"/a/b": {Require: []string{"x"}, Guard: true},
	}, accessMap(rs), "a gate's guard runs for the pages below it; a rule is listed once")
}

func TestAccessSectionIsSpliced(t *testing.T) {
	section, err := accessSection(map[string]accessEntry{
		"/":           {Require: []string{"*"}},
		"/notes/{id}": {Require: []string{"editor"}, Guard: true, Calls: []string{"star"}},
	})
	require.NoError(t, err)
	// If yaml.v3 spaces this differently, fix the expectation, never the marker line.
	assert.Equal(t, `# access is written by aicoded generate from <ssr:access>; do not edit it
access:
  "/":
    require: ["*"]
  "/notes/{id}":
    require: ["editor"]
    guard: true
    calls: ["star"]
`, string(section))

	before := "# notes app\napp: notes # the name\naccess:\n  \"/old\":\n    require: [\"x\"]\n\n# secrets below\nsecrets: [api_key]\n"
	got, err := withAccess([]byte(before), section)
	require.NoError(t, err)
	assert.Equal(t, "# notes app\napp: notes # the name\n"+string(section)+"\n# secrets below\nsecrets: [api_key]\n", string(got))

	again, err := withAccess(got, section)
	require.NoError(t, err)
	assert.Equal(t, string(got), string(again), "a second run changes nothing")

	empty, err := accessSection(map[string]accessEntry{})
	require.NoError(t, err)
	assert.Equal(t, accessMarker+"\naccess: {}\n", string(empty))
	changed, err := withAccess(got, empty)
	require.NoError(t, err)
	assert.Equal(t, "# notes app\napp: notes # the name\n"+string(empty)+"\n# secrets below\nsecrets: [api_key]\n", string(changed))

	appended, err := withAccess([]byte("app: notes\nsecrets: [api_key]"), section)
	require.NoError(t, err)
	assert.Equal(t, "app: notes\nsecrets: [api_key]\n"+string(section), string(appended))
}

func TestWithAccessRefuses(t *testing.T) {
	section, err := accessSection(map[string]accessEntry{"/": {Require: []string{"*"}}})
	require.NoError(t, err)
	const oneDoc = "one YAML document"
	for name, c := range map[string]struct{ data, pos, fix string }{
		"invalid YAML":                   {"app: [x\n", "aicoded.yaml:1", "fix the YAML syntax"},
		"a list":                         {"- app\n", "aicoded.yaml:1", oneDoc},
		"flow style":                     {"{app: x, access: {}}\n", "aicoded.yaml:1", oneDoc},
		"indented keys":                  {"  app: x\n  access: {}\n  secrets: [a]\n", "aicoded.yaml:2", oneDoc},
		"two documents":                  {"app: x\n---\napp: y\n", "aicoded.yaml:2", "remove the --- line"},
		"access twice":                   {"access: {}\napp: x\naccess: {}\n", "aicoded.yaml:3", oneDoc},
		"access continued at column one": {"access: [a,\nb]\napp: x\n", "aicoded.yaml:1", oneDoc},
		"lines broken by CR":             {"app: x\rowner: y\raccess: {}\r", "aicoded.yaml:3", oneDoc},
		"lines broken by NEL":            {"app: x\u0085access: {}\n", "aicoded.yaml:2", oneDoc},
	} {
		_, err := withAccess([]byte(c.data), section)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, name) {
			assert.Equal(t, "E-MAN-003", e.Code, name)
			assert.Equal(t, c.pos, e.Pos, name)
			assert.Contains(t, e.Fix, c.fix, name)
		}
	}
}

func TestGenerateWritesAccess(t *testing.T) {
	dir := copyApp(t, "app")
	manifest := filepath.Join(dir, "aicoded.yaml")
	before, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.NoError(t, Generate(dir))
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	assert.Contains(t, string(after), string(before), "everything else stays as it was")
	assert.Contains(t, string(after), "\naccess:\n")

	info, err := os.Stat(manifest)
	require.NoError(t, err)
	require.NoError(t, Generate(dir))
	again, err := os.Stat(manifest)
	require.NoError(t, err)
	assert.Equal(t, info.ModTime(), again.ModTime(), "an unchanged section is not rewritten")
}

func TestGenerateKeepsModules(t *testing.T) {
	dir := copyApp(t, "app")
	manifest := filepath.Join(dir, "aicoded.yaml")
	modules := "# third-party modules, by hand\nmodules:\n  - golang.org/x/text # collation\n  - github.com/google/uuid\n"
	require.NoError(t, os.WriteFile(manifest, []byte("app: app\n"+modules+"access: {}\n"), 0o600))
	require.NoError(t, Generate(dir))
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	assert.Contains(t, string(after), "app: app\n"+modules+accessMarker+"\naccess:\n", "modules: stays byte for byte")
}

func TestGenerateWithoutManifest(t *testing.T) {
	dir := copyApp(t, "app")
	require.NoError(t, os.Remove(filepath.Join(dir, "aicoded.yaml")))
	require.NoError(t, Generate(dir))
	assert.NoFileExists(t, filepath.Join(dir, "aicoded.yaml"))
}

func TestGenerateRefusesManifestItCannotSplice(t *testing.T) {
	dir := copyApp(t, "app")
	manifest := filepath.Join(dir, "aicoded.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte("{app: app}\n"), 0o600))
	assert.Equal(t, "E-MAN-003", errs.Code(Generate(dir)))
	assert.NoFileExists(t, filepath.Join(dir, "pages", "handler_gen.go"), "nothing is written")
	data, err := os.ReadFile(manifest)
	require.NoError(t, err)
	assert.Equal(t, "{app: app}\n", string(data))
}
