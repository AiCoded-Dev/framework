// Package cantbebuilt shows what cannot be built. Each test makes one attempt at a mistake
// behind a well-known kind of leak from AI-built apps, and shows the code that refuses it; a
// control without the mistake shows that nothing else refuses it. docs/cant-be-built.md explains
// them.
package cantbebuilt

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
)

// checkout returns this framework checkout, which the test apps replace the framework with.
func checkout(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	return dir
}

// copyApp copies the app testdata/<name> into a temp folder, with the replace line of its go.mod
// pointing at this checkout, and returns the folder.
func copyApp(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", name))))
	gomod := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(gomod)
	require.NoError(t, err)
	replace := regexp.MustCompile(`(?m)^replace aicoded.dev/framework => .*$`)
	require.Regexp(t, replace, string(data))
	data = replace.ReplaceAll(data, []byte("replace aicoded.dev/framework => "+checkout(t)))
	require.NoError(t, os.WriteFile(gomod, data, 0o600))
	return dir
}

// refusal is what the builder sees of a problem: its code, where it is and what it says.
type refusal struct{ Code, Pos, Message string }

// runCheck runs aicoded check --frozen on the app in dir, which writes nothing and requires the
// generated files to be current, and returns the problems it found.
func runCheck(t *testing.T, dir string) []refusal {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	r, err := check.Run(t.Context(), dir, check.Options{Frozen: true, FrameworkDir: checkout(t)})
	require.NoError(t, err)
	require.Len(t, r.Apps, 1)
	return refusals(r.Apps[0].Problems)
}

func refusals(ps []problem.Problem) []refusal {
	var out []refusal
	for _, p := range ps {
		out = append(out, refusal{p.Code, p.Pos, p.Message})
	}
	return out
}
