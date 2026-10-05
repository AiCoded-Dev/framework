package lint

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

// goCommand makes the go command report version v.
func goCommand(t *testing.T, built, v string) {
	b, g := builtWith, goVersion
	t.Cleanup(func() { builtWith, goVersion = b, g })
	builtWith = built
	goVersion = func(context.Context, string, []string) (string, error) { return v, nil }
}

func TestOlderThanGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := filepath.Join("testdata", "files")
	goCommand(t, "go1.25.8", "go1.26.8")
	_, err := Run(t.Context(), dir, Config{})
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, errs.New("E-CHK-007",
		"aicoded was built with go1.25.8, and the go command is go1.26.8, a newer Go: lint cannot read code for it",
		"install aicoded again with go1.26.8, the Go you build with, or set GOTOOLCHAIN=go1.25.8, the Go aicoded was built with"), e)

	goCommand(t, "go1.25.8", "go1.25.14")
	_, err = Run(t.Context(), dir, Config{})
	assert.NoError(t, err, "a newer patch release of the same Go")
}

func TestGoEnvJSON(t *testing.T) {
	printed := func(out string) (string, error) {
		return goEnvJSON(exec.CommandContext(t.Context(), "sh", "-c", `printf '%s' "$1"`, "sh", out, "GOFLAGS"), t.TempDir(), nil)
	}
	value, err := printed("{\n\t\"GOFLAGS\": \"-v\"\n}\n")
	require.NoError(t, err)
	assert.Equal(t, "-v", value)
	for _, out := range []string{"", "-v", `GOFLAGS="-v"`, `{}`, `{"GOFLAGS": null}`, `{"goflags": "-v"}`, `{"GOFLAGS": "-v"}{"GOFLAGS": "-x"}`, `["-v"]`} {
		_, err := printed(out)
		assert.Error(t, err, "go env -json printed %q", out)
	}
}
