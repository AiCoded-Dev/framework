package docs

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/lint"
)

const changelogMD = "# Changelog for agents\n\nChanges to the API.\n"

func TestTopicsOfATree(t *testing.T) {
	ts, err := topics(fstest.MapFS{
		"guides/forms.md":     {Data: []byte("# Forms\n\nA form posts\n`values`.\n\n## More\n")},
		"guides/overview.md":  {Data: []byte("# Overview \t\n\nHow an app works.\n")},
		"tasks/add-a-page.md": {Data: []byte("# Add a page\n\nSteps.\n")},
		"changelog-agents.md": {Data: []byte(changelogMD)},
	})
	require.NoError(t, err)
	assert.Equal(t, []Topic{
		{Path: "guides/forms", Title: "Forms", Summary: "A form posts `values`."},
		{Path: "guides/overview", Title: "Overview", Summary: "How an app works."},
		{Path: "tasks/add-a-page", Title: "Add a page", Summary: "Steps."},
		{Path: "changelog", Title: "Changelog for agents", Summary: "Changes to the API."},
	}, ts)
}

func TestTopicsRefuse(t *testing.T) {
	for name, tc := range map[string]struct{ file, text, want string }{
		"no title":                       {"guides/a.md", "Forms\n\nText.\n", "docs/guides/a.md:1: start the file with # <Title>"},
		"empty title":                    {"guides/a.md", "# \n\nText.\n", "docs/guides/a.md:1: start the file with # <Title>"},
		"no summary":                     {"guides/a.md", "# A\n", "docs/guides/a.md:3: follow the title with a blank line and a summary paragraph"},
		"no blank line":                  {"tasks/a.md", "# A\nText.\n", "docs/tasks/a.md:3: follow the title with a blank line and a summary paragraph"},
		"a heading":                      {"guides/a.md", "# A\n\n## B\n", "docs/guides/a.md:3: follow the title with a blank line and a summary paragraph"},
		"a list":                         {"guides/a.md", "# A\n\n- b\n", "docs/guides/a.md:3: follow the title with a blank line and a summary paragraph"},
		"a code block":                   {"guides/a.md", "# A\n\n```go\n```\n", "docs/guides/a.md:3: follow the title with a blank line and a summary paragraph"},
		"a heading after the summary":    {"guides/a.md", "# A\n\nText.\n## B\n", "docs/guides/a.md:4: end the summary paragraph with a blank line before this line"},
		"a list after the summary":       {"guides/a.md", "# A\n\nText:\n  - b\n", "docs/guides/a.md:4: end the summary paragraph with a blank line before this line"},
		"a code block after the summary": {"guides/a.md", "# A\n\nText\non two lines:\n```go\n```\n", "docs/guides/a.md:5: end the summary paragraph with a blank line before this line"},
		"a link":                         {"guides/a.md", "# A\n\nSee\n[forms](forms.md).\n", "docs/guides/a.md:3: move the link out of the summary, which howto and llms.txt show alone"},
		"a bad name":                     {"guides/Forms.md", "# A\n\nText.\n", "docs/guides/Forms.md: name the file in lowercase words joined by -"},
	} {
		_, err := topics(fstest.MapFS{
			tc.file:               {Data: []byte(tc.text)},
			"changelog-agents.md": {Data: []byte(changelogMD)},
		})
		require.Error(t, err, name)
		assert.Equal(t, tc.want, err.Error(), name)
	}
}

func TestTopics(t *testing.T) {
	ts, err := Topics()
	require.NoError(t, err)
	paths := make([]string, len(ts))
	for i, tp := range ts {
		paths[i] = tp.Path
	}
	assert.IsIncreasing(t, paths[:len(paths)-1])
	assert.Equal(t, "changelog", paths[len(paths)-1])
	assert.Contains(t, paths, "guides/overview")

	onDisk, err := topics(os.DirFS("."))
	require.NoError(t, err)
	assert.Equal(t, onDisk, ts, "FS embeds every guide and task")
}

func TestRead(t *testing.T) {
	text, err := Read("guides/overview")
	require.NoError(t, err)
	want, err := os.ReadFile("guides/overview.md")
	require.NoError(t, err)
	assert.Equal(t, string(want), text)

	text, err = Read("changelog")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(text, "# Changelog for AI assistants\n"), text)
}

func TestReadRefuses(t *testing.T) {
	for _, path := range []string{
		"../go.mod", "guides/../../x", "guides/overview.md", "errors/E-DEV-001", "errors/README", "guides/nope",
		"changelog-agents", "", "GUIDES/OVERVIEW", "guides/overview/", "guides/overview\x00",
	} {
		_, err := Read(path)
		assert.Equal(t, "E-CLI-001", errs.Code(err), path)
	}
}

func TestErrorPages(t *testing.T) {
	pages, err := fs.Glob(FS, "errors/E-*.md")
	require.NoError(t, err)
	require.NotEmpty(t, pages)
	for _, name := range pages {
		data, err := fs.ReadFile(FS, name)
		require.NoError(t, err)
		code := strings.TrimSuffix(strings.TrimPrefix(name, "errors/"), ".md")
		assert.True(t, strings.HasPrefix(string(data), "# "+code+": "), "%s starts with # %s: <title>", name, code)
		assert.Contains(t, string(data), "\n**Fix:** ", "%s has a Fix line", name)
	}
}

var update = flag.Bool("update", false, "rewrite the code blocks of the docs and llms.txt")

func TestDocsAreCurrent(t *testing.T) {
	problems, err := checkDocs("..", *update)
	require.NoError(t, err)
	for _, p := range problems {
		t.Error(p)
	}
}

// tree writes files, by their paths with /, under a new temp folder and returns the folder.
func tree(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
	}
	return root
}

func read(t *testing.T, root, name string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	require.NoError(t, err)
	return string(data)
}

const notesGo = `package notes

import "fmt"

// Note is one note.
type Note struct {
	Title string
}

type (
	// ID names a note.
	ID  int64
	tag string
)

// Default is the note a new page shows.
var Default = Note{Title: "new"}

// Print prints n.
func Print(n Note) { fmt.Println(n.Title) }

// DP provides the data.
type DP struct{}

// Data fills the note.
func (p *DP) Data(n *Note) error {
	n.Title = "x"
	return nil
}
`

const printGo = "// Print prints n.\nfunc Print(n Note) { fmt.Println(n.Title) }\n"

// doc returns a doc whose line 5 is marker, followed by a Go code block of code.
func doc(marker, code string) string {
	return "# Doc\n\nText.\n\n" + marker + "\n```go\n" + code + "```\n\nMore.\n"
}

func TestCodeBlocks(t *testing.T) {
	for marker, code := range map[string]string{
		"<!-- code: notes/notes.go -->":         notesGo,
		"<!-- code: notes/notes.go Note -->":    "// Note is one note.\ntype Note struct {\n\tTitle string\n}\n",
		"<!-- code: notes/notes.go ID -->":      "\t// ID names a note.\n\tID  int64\n",
		"<!-- code: notes/notes.go Default -->": "// Default is the note a new page shows.\nvar Default = Note{Title: \"new\"}\n",
		"<!-- code: notes/notes.go Print -->":   printGo,
		"<!-- code: notes/notes.go DP.Data -->": "// Data fills the note.\nfunc (p *DP) Data(n *Note) error {\n\tn.Title = \"x\"\n\treturn nil\n}\n",
	} {
		root := tree(t, map[string]string{"notes/notes.go": notesGo, "docs/a.md": doc(marker, code)})
		problems, err := checkCode(root, "docs/a.md", false)
		require.NoError(t, err)
		assert.Empty(t, problems, marker)
	}
}

func TestCodeBlockUpdate(t *testing.T) {
	root := tree(t, map[string]string{"notes/notes.go": notesGo, "docs/a.md": doc("<!-- code: notes/notes.go Print -->", "func Print() {}\n")})
	problems, err := checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/a.md:5: the code block differs from notes/notes.go Print; run make docs and commit the result"}, problems)

	problems, err = checkCode(root, "docs/a.md", true)
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, doc("<!-- code: notes/notes.go Print -->", printGo), read(t, root, "docs/a.md"))
}

func TestCodeBlockInAList(t *testing.T) {
	list := "# Doc\n\nText.\n\n1. Print it:\n\n   <!-- code: notes/notes.go DP.Data -->\n   ```go\n   old\n   ```\n2. Done.\n"
	root := tree(t, map[string]string{"notes/notes.go": notesGo, "docs/a.md": list})
	problems, err := checkCode(root, "docs/a.md", true)
	require.NoError(t, err)
	assert.Empty(t, problems)
	want := "# Doc\n\nText.\n\n1. Print it:\n\n   <!-- code: notes/notes.go DP.Data -->\n   ```go\n" +
		"   // Data fills the note.\n   func (p *DP) Data(n *Note) error {\n   \tn.Title = \"x\"\n   \treturn nil\n   }\n   ```\n2. Done.\n"
	assert.Equal(t, want, read(t, root, "docs/a.md"))
	problems, err = checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Empty(t, problems)
}

func TestCodeBlockProblems(t *testing.T) {
	for text, want := range map[string]string{
		doc("<!-- code: notes/nope.go -->", ""):                           "docs/a.md:5: notes/nope.go does not exist; fix the path in the marker",
		doc("<!-- code: ../x.go -->", ""):                                 "docs/a.md:5: ../x.go is not a path in the repo; write it relative to the repo root",
		doc("<!-- code: notes/notes.go Nope -->", ""):                     "docs/a.md:5: notes/notes.go has no declaration Nope; fix the name in the marker",
		doc("<!-- code: notes/notes.go Note.Data -->", ""):                "docs/a.md:5: notes/notes.go has no declaration Note.Data; fix the name in the marker",
		doc("<!-- code: notes/fence.md Note -->", ""):                     "docs/a.md:5: notes/fence.md is not a Go file, so the marker cannot name a declaration in it",
		doc("<!-- code: notes/fence.md -->", ""):                          "docs/a.md:5: notes/fence.md has a line of backticks that would close the block; open the block with a longer fence",
		"# Doc\n\nText.\n\n<!-- code: notes/notes.go -->\n\n```go\n```\n": "docs/a.md:5: follow the marker directly with a fenced code block",
		"# Doc\n\nText.\n\n<!-- code: notes/notes.go -->\n```go\nx\n":     "docs/a.md:6: close the code block",
		"# Doc\n\nText.\n\n<!-- code:notes/notes.go-->\n":                 "docs/a.md:5: write the marker as <!-- code: <path> [<name>] --> on a line of its own",
		"# Doc\n\nText.\n\n<!-- rules -->\n| x |\n":                       "docs/a.md:5: close the table with <!-- end rules -->",
		"# Doc\n\nText.\n\n<!-- end rules -->\n":                          "docs/a.md:5: <!-- end rules --> closes no table; open the table with <!-- rules --> on a line of its own",
		"# Doc\n\nText.\n\nSee <!-- rules -->\n":                          "docs/a.md:5: write <!-- rules --> and <!-- end rules --> each on a line of its own, around the table",
	} {
		root := tree(t, map[string]string{"notes/notes.go": notesGo, "notes/fence.md": "```go\n```\n", "docs/a.md": text})
		problems, err := checkCode(root, "docs/a.md", true)
		require.NoError(t, err)
		assert.Equal(t, []string{want}, problems, text)
		assert.Equal(t, text, read(t, root, "docs/a.md"), "nothing is rewritten")
	}
}

func TestCodeBlockFences(t *testing.T) {
	text := "# Doc\n\nText.\n\n````md\n<!-- code: notes/nope.go -->\n<!-- rules -->\n```go\n```\n````\n\n" +
		"<!-- code: notes/fence.md -->\n````md\n```go\n```\n````\n"
	root := tree(t, map[string]string{"notes/fence.md": "```go\n```\n", "docs/a.md": text})
	problems, err := checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Empty(t, problems, "a marker inside a code block is text, and a longer fence holds backticks")
}

func TestCodeBlockMarkerInACodeSpan(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc\n\nText.\n\nWrite `<!-- code: <path> [<name>] -->` above a block.\n"})
	problems, err := checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Empty(t, problems, "a marker in a code span is text")
}

func TestRulesTable(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc\n\nText.\n\n<!-- rules -->\n<!-- end rules -->\n"})
	problems, err := checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/a.md:5: the table differs from lint's rules; run make docs and commit the result"}, problems)

	problems, err = checkCode(root, "docs/a.md", true)
	require.NoError(t, err)
	assert.Empty(t, problems)
	rules := lint.Rules()
	lines := strings.Split(read(t, root, "docs/a.md"), "\n")
	require.Len(t, lines, 9+len(rules))
	assert.Equal(t, []string{"<!-- rules -->", "| Code | Rule | Don't | Do |", "|---|---|---|---|",
		"| [E-LINT-001](../errors/E-LINT-001.md) | " + rules[0].Title + " | " + rules[0].Dont + " | " + rules[0].Do + " |"}, lines[4:8])
	assert.Equal(t, []string{"<!-- end rules -->", ""}, lines[len(lines)-2:])
	problems, err = checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Empty(t, problems)
}

func TestRulesTableRefusesMarkdown(t *testing.T) {
	const want = "E-LINT-005 in lint/rules.go holds a character that Markdown reads or a line break; write its title, Dont and Do on one line, without < > | * ` [ ] or \\"
	for _, do := range []string{"Call services/<app>/.", "Call services/\nof the app.", "Call services/\r."} {
		_, p := rulesTable([]lint.Rule{{Code: "E-LINT-005", Title: "for generated code only", Dont: "Call rpc.Call.", Do: do}})
		assert.Equal(t, want, p, do)
	}
}

func TestRulesMarkerInACodeSpan(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc\n\nText.\n\nWrite `<!-- rules -->` and `<!-- end rules -->` around the table.\n"})
	problems, err := checkCode(root, "docs/a.md", false)
	require.NoError(t, err)
	assert.Empty(t, problems, "a rules marker in a code span is text")
}

func TestLinks(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/guides/a.md": "# A\n\nText.\n\n" +
			"[b](b.md), [errors](../errors/README.md), [the examples](../../examples) and ![logo](../logo.png \"Logo\").\n" +
			"[web](https://example.com/x.md), [mail](mailto:a@b.example) and [up](#a-section).\n" +
			"`[code](nope.md)` and ``a `[code](nope.md)` span``.\n" +
			"```md\n[fenced](nope.md)\n```\n" +
			"[missing](c.md)\n[anchor](b.md#part)\n[root](/docs/guides/b.md)\n[out](../../../x.md)\n",
		"docs/guides/b.md":      "# B\n\nText.\n",
		"docs/errors/README.md": "# E\n\nText.\n",
		"docs/logo.png":         "png",
		"examples/README.md":    "# Examples\n\nText.\n",
	})
	problems, err := checkLinks(root, "docs/guides/a.md")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"docs/guides/a.md:11: the link c.md names no file; fix the link or create the file",
		"docs/guides/a.md:12: the link b.md#part points into another file, which is not checked; link to the file",
		"docs/guides/a.md:13: the link /docs/guides/b.md starts at /; make it relative to this file",
		"docs/guides/a.md:14: the link ../../../x.md points outside the repo",
	}, problems)
}

// dropped is what checkHTML reports for the raw HTML text on line of docs/a.md.
func dropped(line int, text string) string {
	return fmt.Sprintf("docs/a.md:%d: Markdown may read %s as HTML and drop it; put the text in a code span", line, text)
}

func TestRawHTML(t *testing.T) {
	for text, want := range map[string][]string{
		"Use <b>bold</b>.\n":         {dropped(5, "<b>"), dropped(5, "</b>")},
		"Name it x_<os>.go.\n":       {dropped(5, "<os>")},
		"<!-- a note -->\n":          {dropped(5, "<!--")},
		"<?xml version=\"1\"?>\n":    {dropped(5, "<?xml")},
		"A `span\nover` and <br>\n":  {dropped(6, "<br>")},
		"A `span\n\nand <os>`\n":     {dropped(7, "<os>")},
		"- one `item\n- and <os>`\n": {dropped(6, "<os>")},
		"## Head `x\nline <os>`\n":   {dropped(6, "<os>")},
		"A `span\n> quote <os>`\n":   {dropped(6, "<os>")},
		"A ``span\n<div> here``\n":   {dropped(6, "<div>")},
		"`x\n<= 3 `<a>`\n":           {dropped(6, "<a>")},
		"#1 `x\n`<a>`\n":             {dropped(6, "<a>")},
	} {
		root := tree(t, map[string]string{"docs/a.md": "# Doc\n\nText.\n\n" + text})
		problems, err := checkHTML(root, "docs/a.md")
		require.NoError(t, err)
		assert.Equal(t, want, problems, text)
	}
}

func TestRawHTMLInCode(t *testing.T) {
	root := tree(t, map[string]string{"docs/a.md": "# Doc\n\nText.\n\n" +
		"A code span `_<os>`, one ``over\ntwo <lines>`` and a < b.\n\n" +
		"```html\n<p>\n```\n\n<!-- code: notes/notes.go -->\n```go\n```\n\n<!-- rules -->\n<!-- end rules -->\n"})
	problems, err := checkHTML(root, "docs/a.md")
	require.NoError(t, err)
	assert.Empty(t, problems)
}

func TestDocsRefuseRawHTML(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/a.md":                "# Doc\n\nText.\n\nName it x_<os>.go.\n",
		"docs/changelog-agents.md": changelogMD,
		"docs/errors/README.md":    "# Error catalogue\n\nEvery error has a code.\n",
	})
	problems, err := checkLLMs(root, true)
	require.NoError(t, err)
	require.Empty(t, problems)
	problems, err = checkDocs(root, false)
	require.NoError(t, err)
	assert.Equal(t, []string{dropped(5, "<os>")}, problems)
}

func TestLLMs(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/guides/overview.md":   "# Overview\n\nHow an app\nworks.\n",
		"docs/guides/forms.md":      "# Forms\n\nPosting `values`.\n",
		"docs/tasks/add-a-page.md":  "# Add a page\n\nSteps.\n",
		"docs/changelog-agents.md":  changelogMD,
		"docs/errors/README.md":     "# Error catalogue\n\nEvery error has a code.\n\n| Area |\n",
		"examples/README.md":        "# Examples\n\nTwo apps.\n",
		"examples/people/README.md": "# People\n\nA directory.\n",
	})
	text, err := llms(root)
	require.NoError(t, err)
	assert.Equal(t, "# aicoded framework\n\n> "+llmsSummary+"\n\n"+llmsIntro+"\n"+`
## Guides

- [Forms](docs/guides/forms.md): Posting `+"`values`"+`.
- [Overview](docs/guides/overview.md): How an app works.

## Tasks

- [Add a page](docs/tasks/add-a-page.md): Steps.

## Errors

- [Error catalogue](docs/errors/README.md): Every error has a code.

## Optional

- [Changelog for agents](docs/changelog-agents.md): Changes to the API.
- [Examples](examples/README.md): Two apps.
- [People](examples/people/README.md): A directory.
`, text)

	problems, err := checkLLMs(root, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"llms.txt is out of date; run make docs and commit the result"}, problems)
	problems, err = checkLLMs(root, true)
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, text, read(t, root, "llms.txt"))

	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guides", "forms.md"), []byte("# Forms\n\nNew summary.\n"), 0o644))
	problems, err = checkLLMs(root, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"llms.txt is out of date; run make docs and commit the result"}, problems)
}

func TestLLMsLeavesOutEmptySections(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/guides/overview.md":  "# Overview\n\nHow an app works.\n",
		"docs/changelog-agents.md": changelogMD,
		"docs/errors/README.md":    "# Error catalogue\n\nEvery error has a code.\n",
	})
	text, err := llms(root)
	require.NoError(t, err)
	assert.NotContains(t, text, "## Tasks")
	assert.Contains(t, text, "## Optional\n\n- [Changelog for agents](docs/changelog-agents.md): Changes to the API.\n")
}

func TestLLMsNamesABadSummary(t *testing.T) {
	root := tree(t, map[string]string{
		"docs/guides/overview.md":  "# Overview\n\n- a list\n",
		"docs/changelog-agents.md": changelogMD,
		"docs/errors/README.md":    "# Error catalogue\n\nEvery error has a code.\n",
	})
	problems, err := checkLLMs(root, true)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/guides/overview.md:3: follow the title with a blank line and a summary paragraph"}, problems)
	assert.NoFileExists(t, filepath.Join(root, "llms.txt"))
}
