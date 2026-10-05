package gotool

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

func requireGo(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not in PATH")
	}
}

// module copies the module testdata/name into a temp folder and returns the folder.
func module(t *testing.T, name string) string {
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", name))))
	return dir
}

// listing returns every path under dir.
func listing(t *testing.T, dir string) []string {
	var paths []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		paths = append(paths, path)
		return err
	}))
	return paths
}

// brief is the part of a problem the tests compare.
type brief struct{ Code, Pos, Msg string }

func briefs(ps []*errs.Error) []brief {
	out := make([]brief, len(ps))
	for i, p := range ps {
		out[i] = brief{p.Code, p.Pos, p.Msg}
	}
	return out
}

func TestBuildOK(t *testing.T) {
	requireGo(t)
	dir := module(t, "ok")
	before := listing(t, dir)
	ps, err := Build(t.Context(), dir)
	require.NoError(t, err)
	assert.Empty(t, ps)
	assert.Equal(t, before, listing(t, dir), "no binary is written into the module")
}

func TestBuildWritesNothing(t *testing.T) {
	requireGo(t)
	dir := module(t, "ok")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "greet")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600))
	before := listing(t, dir)
	ps, err := Build(t.Context(), dir)
	require.NoError(t, err)
	assert.Empty(t, ps)
	assert.Equal(t, before, listing(t, dir), "go build of a single main package writes no binary either")
}

func TestBuildErrors(t *testing.T) {
	requireGo(t)
	ps, err := Build(t.Context(), module(t, "broken"))
	require.NoError(t, err)
	assert.Equal(t, []brief{
		{"E-CHK-002", "main.go:4", "undefined: undefinedMain"},
		{"E-CHK-002", "pages/x.go:6", "undefined: undefinedPage"},
	}, briefs(ps), "a package no main package imports is compiled too")
	assert.Equal(t, "fix the Go code at this line", ps[0].Fix)
}

func TestBuildBinary(t *testing.T) {
	requireGo(t)
	out := filepath.Join(t.TempDir(), "app")
	ps, err := BuildBinary(t.Context(), module(t, "ok"), out)
	require.NoError(t, err)
	assert.Empty(t, ps)
	printed, err := exec.CommandContext(t.Context(), out).Output()
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(printed))

	ps, err = BuildBinary(t.Context(), module(t, "broken"), out)
	require.NoError(t, err)
	assert.Equal(t, []brief{{"E-CHK-002", "main.go:4", "undefined: undefinedMain"}}, briefs(ps))
}

func TestVet(t *testing.T) {
	requireGo(t)
	ps, err := Vet(t.Context(), module(t, "vet"))
	require.NoError(t, err)
	assert.Equal(t, []brief{{"E-CHK-003", "main.go:6", `printf: fmt.Printf format %d has arg "text" of wrong type string`}}, briefs(ps))
	assert.Equal(t, "fix the code as the message says; go vet will run in the security checks too", ps[0].Fix)

	ps, err = Vet(t.Context(), module(t, "broken"))
	require.NoError(t, err)
	assert.Equal(t, []brief{
		{"E-CHK-002", "main.go:4", "undefined: undefinedMain"},
		{"E-CHK-002", "pages/x.go:6", "undefined: undefinedPage"},
	}, briefs(ps), "vet reports what does not compile as a build problem")

	ps, err = Vet(t.Context(), module(t, "ok"))
	require.NoError(t, err)
	assert.Empty(t, ps)
}

func TestTestFailures(t *testing.T) {
	requireGo(t)
	ps, err := Test(t.Context(), module(t, "failing"), false)
	require.NoError(t, err)
	assert.Equal(t, []brief{{"E-CHK-004", "calc/calc_test.go:12",
		"TestAddWrong failed: calc_test.go:12: adding\ncalc_test.go:14: Add(2, 2) = 4, want 5"}}, briefs(ps))
	assert.Equal(t, "fix the code or the test until go test passes", ps[0].Fix)

	ps, err = Test(t.Context(), module(t, "broken"), false)
	require.NoError(t, err)
	assert.Equal(t, []brief{
		{"E-CHK-002", "main.go:4", "undefined: undefinedMain"},
		{"E-CHK-002", "pages/x.go:6", "undefined: undefinedPage"},
	}, briefs(ps))
}

func TestTestRace(t *testing.T) {
	requireGo(t)
	dir := module(t, "ok")
	ok, why, err := Race(t.Context(), dir)
	require.NoError(t, err)
	if !ok {
		t.Skip("no -race here: " + why)
	}
	ps, err := Test(t.Context(), dir, true)
	require.NoError(t, err)
	assert.Empty(t, ps)
}

func TestRaceOffWithoutCgo(t *testing.T) {
	requireGo(t)
	t.Setenv("CGO_ENABLED", "0")
	ok, why, err := Race(t.Context(), module(t, "ok"))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "cgo is off", why)
}

func TestRaceWithoutCompiler(t *testing.T) {
	requireGo(t)
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("CC", "aicoded-no-such-cc -m64")
	ok, why, err := Race(t.Context(), module(t, "ok"))
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "no C compiler aicoded-no-such-cc on PATH", why)
}

func TestTidy(t *testing.T) {
	requireGo(t)
	dir := module(t, "ok")
	require.NoError(t, Tidy(t.Context(), dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module ok\n\nnot a directive\n"), 0o600))
	err := Tidy(t.Context(), dir)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-CHK-006", e.Code)
	assert.Contains(t, e.Msg, "unknown directive")
	assert.Equal(t, "check the network and go.mod, then run go mod tidy in "+dir, e.Fix)
}

func TestNoGo(t *testing.T) {
	t.Setenv("PATH", "")
	dir := t.TempDir()
	_, err := Build(t.Context(), dir)
	assert.Equal(t, "E-CHK-005", errs.Code(err))
	_, _, err = Race(t.Context(), dir)
	assert.Equal(t, "E-CHK-005", errs.Code(err))
	assert.Equal(t, "E-CHK-005", errs.Code(Tidy(t.Context(), dir)))
	assert.Equal(t, "E-CHK-005", errs.Code(Find()))
}

func TestStopped(t *testing.T) {
	requireGo(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := Build(ctx, module(t, "ok"))
	require.ErrorIs(t, err, context.Canceled)
}
