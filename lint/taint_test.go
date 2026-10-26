package lint_test

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/docs"
	"aicoded.dev/framework/lint"
)

func TestTaint(t *testing.T) {
	assert.Equal(t, marked(t, "taint", "leak", "E-LINT-008"), run(t, "taint", "pages/route_gen.go", "services/billing/client_gen.go"),
		"each line marked leak is found, and nothing else")
}

func TestMailNotes(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	r, err := lint.Run(t.Context(), filepath.Join("testdata", "taint"), lint.Config{
		Generated: []string{"pages/route_gen.go", "services/billing/client_gen.go"}, FrameworkDir: checkout(t),
	})
	require.NoError(t, err)
	page, err := fs.ReadFile(docs.FS, "errors/E-DATA-001.md")
	require.NoError(t, err)
	var notes []string
	for _, n := range r.Notes {
		assert.Contains(t, string(page), "\n**Fix:** "+n.Fix+"\n")
		notes = append(notes, n.Pos+": "+n.Code+": "+n.Msg)
	}
	assert.Equal(t, marked(t, "taint", "note", "E-DATA-001"), notes, "each send marked note is noted once, and nothing else")
}

// marked returns the findings of code that the comments "// <label>: <message>" at the ends of
// lines in the Go files of testdata/<name> expect, as run returns them.
func marked(t *testing.T, name, label, code string) []string {
	t.Helper()
	type mark struct {
		file string
		line int
		msg  string
	}
	var marks []mark
	dir := filepath.Join("testdata", name)
	err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil || !strings.HasSuffix(path, ".go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if _, msg, ok := strings.Cut(line, "// "+label+": "); ok {
				marks = append(marks, mark{filepath.ToSlash(rel), i + 1, msg})
			}
		}
		return nil
	})
	require.NoError(t, err)
	slices.SortFunc(marks, func(a, b mark) int { return cmp.Or(cmp.Compare(a.file, b.file), cmp.Compare(a.line, b.line)) })
	var out []string
	for _, m := range marks {
		out = append(out, fmt.Sprintf("%s:%d: %s: %s", m.file, m.line, code, m.msg))
	}
	return out
}
