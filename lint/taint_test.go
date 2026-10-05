package lint_test

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaint(t *testing.T) {
	assert.Equal(t, marked(t, "taint"), run(t, "taint", "pages/route_gen.go", "services/billing/client_gen.go"),
		"each line marked leak is found, and nothing else")
}

// marked returns the findings that the comments "// leak: <message>" at the ends of lines in
// the Go files of testdata/<name> expect, as run returns them.
func marked(t *testing.T, name string) []string {
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
			if _, msg, ok := strings.Cut(line, "// leak: "); ok {
				marks = append(marks, mark{filepath.ToSlash(rel), i + 1, msg})
			}
		}
		return nil
	})
	require.NoError(t, err)
	slices.SortFunc(marks, func(a, b mark) int { return cmp.Or(cmp.Compare(a.file, b.file), cmp.Compare(a.line, b.line)) })
	var out []string
	for _, m := range marks {
		out = append(out, fmt.Sprintf("%s:%d: E-LINT-008: %s", m.file, m.line, m.msg))
	}
	return out
}
