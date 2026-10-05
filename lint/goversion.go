package lint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/version"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"aicoded.dev/framework/internal/errs"
)

var (
	// builtWith is the Go version this program was built with.
	builtWith = runtime.Version()
	// goVersion returns the Go version of the go command in dir, as go env GOVERSION prints it.
	goVersion = func(ctx context.Context, dir string, env []string) (string, error) {
		return goEnv(exec.CommandContext(ctx, "go", "env", "GOVERSION"), dir, env)
	}
)

// goEnv runs cmd, go env with the name of one variable, in dir with env and returns the value it
// prints. Its error holds the first line of what the go command wrote to stderr, as oneLine
// gives it.
func goEnv(cmd *exec.Cmd, dir string, env []string) (string, error) {
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(bytes.TrimSpace(exit.Stderr)) > 0 {
			err = errors.New(oneLine(string(exit.Stderr)))
		}
		return "", fmt.Errorf("lint cannot read %s with go env: %w", cmd.Args[len(cmd.Args)-1], err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// goEnvJSON runs cmd, go env -json with the name of one variable, as goEnv does, and returns the
// variable's value. The output must be one JSON object that holds the variable as a string, so a
// go env flag that GOFLAGS itself sets, such as -u or -w, fails instead of changing what it prints.
func goEnvJSON(cmd *exec.Cmd, dir string, env []string) (string, error) {
	name := cmd.Args[len(cmd.Args)-1]
	out, err := goEnv(cmd, dir, env)
	if err != nil {
		return "", err
	}
	var vars map[string]any
	dec := json.NewDecoder(strings.NewReader(out))
	if err := dec.Decode(&vars); err == nil && dec.Decode(&struct{}{}) == io.EOF {
		if value, ok := vars[name].(string); ok {
			return value, nil
		}
	}
	return "", fmt.Errorf("lint cannot read %s with go env: go env -json printed %q, not one JSON object with %s as a string", name, out, name)
}

// goWork returns the workspace file that the go command in dir reads, as go env GOWORK prints
// it, or "" when it reads none.
func goWork(ctx context.Context, dir string, env []string) (string, error) {
	work, err := goEnv(exec.CommandContext(ctx, "go", "env", "GOWORK"), dir, env)
	if err != nil || work == "off" {
		return "", err
	}
	return work, nil
}

// goflagsAllowed are the only GOFLAGS entries lint accepts, each written exactly so.
var goflagsAllowed = []string{
	"-mod=mod", "-mod=readonly", "-modcacherw", "-trimpath", "-buildvcs=true", "-buildvcs=false", "-buildvcs=auto", "-v", "-x",
}

// goflags reports, with E-LINT-011, a GOFLAGS that holds anything but entries of goflagsAllowed
// separated by spaces, tabs and line breaks: first the GOFLAGS of env, read before any go command
// runs, and then, when that one passes, the one go env -json prints, which go env -w can set. A
// go env that fails, or prints anything but one JSON object with a GOFLAGS string, is reported
// with what the go command printed and the fix for a go command that cannot run. It returns
// whether it reported anything.
func (a *app) goflags(ctx context.Context, env []string) bool {
	var value string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "GOFLAGS="); ok {
			value = v
		}
	}
	if a.goflagsValue(value, "the GOFLAGS environment variable") {
		return true
	}
	value, err := goEnvJSON(exec.CommandContext(ctx, "go", "env", "-json", "GOFLAGS"), a.dir, env)
	if err != nil {
		a.report("GOFLAGS", 0, 0, "E-LINT-011", err.Error()+"; when the go command itself cannot run, fix what it reports, such as the go line of go.mod or GOTOOLCHAIN")
		return true
	}
	return a.goflagsValue(value, "the GOFLAGS that go env prints")
}

// goflagsValue reports, with E-LINT-011, a quote, a backslash or a control character other than
// a tab or line break in value, the GOFLAGS that source names, or else each entry of value outside
// goflagsAllowed. It returns whether it reported anything.
func (a *app) goflagsValue(value, source string) bool {
	const allowed = "only -mod=mod, -mod=readonly, -modcacherw, -trimpath, -buildvcs=true, -buildvcs=false, -buildvcs=auto, -v and -x are allowed, separated by spaces"
	if strings.ContainsFunc(value, func(r rune) bool {
		return r == '\'' || r == '"' || r == '\\' || unicode.IsControl(r) && !isGoflagsSpace(r)
	}) {
		a.report("GOFLAGS", 0, 0, "E-LINT-011", source+" holds a quote, a backslash or a control character, which lint does not allow; "+allowed)
		return true
	}
	before := len(a.found)
	for _, entry := range strings.FieldsFunc(value, isGoflagsSpace) {
		if !slices.Contains(goflagsAllowed, entry) {
			a.report("GOFLAGS", 0, 0, "E-LINT-011", source+" sets "+entry+", which lint does not allow; "+allowed)
		}
	}
	return len(a.found) > before
}

// isGoflagsSpace reports whether r separates GOFLAGS entries.
func isGoflagsSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// verifyModules reports, with E-LINT-011 and the first line of the go command's output, a run of
// go mod verify that does not end with exit status 0 and the line "all modules verified": a module
// in the module cache may have changed since it was downloaded, or its files may not be readable.
// The module cache folder in the output is written $GOMODCACHE.
func (a *app) verifyModules(ctx context.Context, env []string) {
	cmd := exec.CommandContext(ctx, "go", "mod", "verify")
	cmd.Dir, cmd.Env = a.dir, env
	out, err := cmd.CombinedOutput()
	if err == nil && slices.Contains(strings.Split(string(out), "\n"), "all modules verified") {
		return
	}
	text := string(out)
	if strings.TrimSpace(text) == "" && err != nil {
		text = err.Error()
	}
	if cache, err := goEnv(exec.CommandContext(ctx, "go", "env", "GOMODCACHE"), a.dir, env); err == nil && cache != "" {
		text = strings.ReplaceAll(text, filepath.Clean(cache)+string(filepath.Separator), "$GOMODCACHE"+string(filepath.Separator))
	}
	a.report("go.mod", 1, 0, "E-LINT-011", "go mod verify does not report all modules verified, so the build could use changed code in the module cache: "+oneLine(text))
}

// oneLine returns out, what a go command printed, as a finding quotes it: its first line that is
// not a "go: downloading" notice, or else its last line, and how many more lines it printed, such
// as the rest of a panic's trace.
func oneLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return !strings.HasPrefix(strings.TrimSpace(l), "go: downloading ") })
	if i < 0 {
		i = len(lines) - 1
	}
	line := strings.TrimSpace(lines[i])
	switch n := len(lines) - 1; n {
	case 0:
		return line
	case 1:
		return line + " (and 1 more line)"
	default:
		return fmt.Sprintf("%s (and %d more lines)", line, n)
	}
}

// checkGoVersion refuses, with E-CHK-007, a go command in dir whose Go is newer than the one this
// program was built with: go/types cannot read the code, the standard library included, of a
// newer Go. A newer patch release of the same Go passes.
func checkGoVersion(ctx context.Context, dir string, env []string) error {
	command, err := goVersion(ctx, dir, env)
	if err != nil {
		return err
	}
	built, _, _ := strings.Cut(builtWith, " ")
	lang := version.Lang(built)
	if lang == "" || version.Compare(lang, version.Lang(command)) >= 0 {
		return nil
	}
	return errs.New("E-CHK-007",
		fmt.Sprintf("aicoded was built with %s, and the go command is %s, a newer Go: lint cannot read code for it", built, command),
		fmt.Sprintf("install aicoded again with %s, the Go you build with, or set GOTOOLCHAIN=%s, the Go aicoded was built with", command, built))
}
