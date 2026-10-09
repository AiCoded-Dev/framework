// Package gotool runs the go command on the developer's module. Every go subprocess of aicoded
// goes through it. It turns what go build, go vet and go test print into coded problems whose
// positions are relative to the module's folder.
package gotool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"aicoded.dev/framework/internal/errs"
)

// Build compiles every package of the module in dir and writes nothing into dir.
func Build(ctx context.Context, dir string) ([]*errs.Error, error) {
	r, err := run(ctx, dir, "build", "-json", "-o", os.DevNull, "./...")
	if err != nil {
		return nil, err
	}
	return buildProblems(r), nil
}

// BuildBinary compiles the main package in dir into the file out.
func BuildBinary(ctx context.Context, dir, out string) ([]*errs.Error, error) {
	out, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	r, err := run(ctx, dir, "build", "-json", "-o", out, ".")
	if err != nil {
		return nil, err
	}
	return buildProblems(r), nil
}

// Vet runs go vet on every package of the module in dir. It turns off -v and -x, which GOFLAGS
// may set, so that the go command prints nothing but what vetProblems reads.
func Vet(ctx context.Context, dir string) ([]*errs.Error, error) {
	r, err := run(ctx, dir, "vet", "-json", "-v=false", "-x=false", "./...")
	if err != nil {
		return nil, err
	}
	return vetProblems(r), nil
}

// Test runs the tests of every package of the module in dir, with the race detector when race
// is set.
func Test(ctx context.Context, dir string, race bool) ([]*errs.Error, error) {
	args := []string{"test", "-json", "./..."}
	if race {
		args = []string{"test", "-json", "-race", "./..."}
	}
	r, err := run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return testProblems(r, modulePath(r.dir)), nil
}

// Race reports whether go test -race can run for the module in dir, and why not.
func Race(ctx context.Context, dir string) (ok bool, why string, err error) {
	r, err := run(ctx, dir, "env", "-json", "CGO_ENABLED", "CC")
	if err != nil {
		return false, "", err
	}
	if r.failed {
		return false, "", fmt.Errorf("go env failed: %s", limit(string(r.stderr)))
	}
	var env struct {
		CGOEnabled string `json:"CGO_ENABLED"`
		CC         string `json:"CC"`
	}
	if err := json.Unmarshal(r.stdout, &env); err != nil {
		return false, "", fmt.Errorf("go env: %w", err)
	}
	if env.CGOEnabled != "1" {
		return false, "cgo is off", nil
	}
	cc := strings.Fields(env.CC)
	if len(cc) == 0 {
		return false, "no C compiler is set", nil
	}
	if _, err := exec.LookPath(cc[0]); err != nil {
		return false, "no C compiler " + cc[0] + " on PATH", nil
	}
	return true, "", nil
}

// Tidy runs go mod tidy in dir. It fails with E-CHK-006 when go mod tidy does.
func Tidy(ctx context.Context, dir string) error {
	r, err := run(ctx, dir, "mod", "tidy")
	if err != nil {
		return err
	}
	if r.failed {
		return errs.New("E-CHK-006", "go mod tidy failed: "+limit(string(r.stderr)),
			"check the network and go.mod, then run go mod tidy in "+r.dir)
	}
	return nil
}

// Find fails with E-CHK-005 when the go command is not on PATH.
func Find() error {
	if _, err := exec.LookPath("go"); err != nil {
		return noGo()
	}
	return nil
}

// noGo is the error of a go command that is not on PATH.
func noGo() error {
	return errs.New("E-CHK-005", "the go command is not on PATH", "install Go 1.25 or newer and put go on PATH")
}

// result is what one run of the go command printed.
type result struct {
	cmd            string // "go <subcommand>"
	dir            string // the absolute folder it ran in
	stdout, stderr []byte
	failed         bool // it exited with a status other than 0
}

// run runs go with args in dir. An exit status other than 0 is not an error; the error is a
// failure to run go at all.
func run(ctx context.Context, dir string, args ...string) (result, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return result{}, err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 5 * time.Second
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, exec.ErrDot):
		return result{}, noGo()
	case ctx.Err() != nil:
		return result{}, ctx.Err()
	case errors.As(err, &exit):
	default:
		return result{}, err
	}
	return result{cmd: "go " + args[0], dir: dir, stdout: stdout.Bytes(), stderr: stderr.Bytes(), failed: err != nil}, nil
}

// modulePath returns the module path in the go.mod of dir, or "" when there is none.
func modulePath(dir string) string {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer root.Close()
	data, err := root.ReadFile("go.mod")
	if err != nil {
		return ""
	}
	return parseModule(data)
}
