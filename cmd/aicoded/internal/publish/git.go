package publish

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/internal/errs"
)

// The oldest git that aicoded publish works with: git bundle create reads its revisions from
// standard input since git 2.31.
const minMajor, minMinor = 2, 31

var (
	gitVersion = regexp.MustCompile(`^git version ([0-9]+)\.([0-9]+)`)
	commitSHA  = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// errTooLong is the error of run when git prints more than its caller takes.
var errTooLong = errors.New("git printed more than aicoded takes")

const fixGit = "install git 2.31 or later, and put it on PATH"

// run runs cmd, a git command whose arguments are constants, in the folder dir, with in on its
// standard input: revisions go there, never in the arguments. git runs without the GIT_
// variables of the environment, which could point it at another repository or other history.
// run returns what git printed, at most limit bytes: when git prints more, run stops it and
// returns errTooLong with the first limit bytes. A git that is not on PATH is E-PUB-012.
func run(cmd *exec.Cmd, dir, in string, limit int) ([]byte, error) {
	cmd.Dir = dir
	cmd.Env = environ()
	cmd.Stdin = strings.NewReader(in)
	stderr := &capped{limit: 4096}
	cmd.Stderr = stderr
	// A child of git, such as git pack-objects, may outlive it with the pipes open.
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errs.New("E-PUB-012", "git is not on PATH", fixGit)
		}
		return nil, err
	}
	out, err := io.ReadAll(io.LimitReader(stdout, int64(limit)+1))
	if err == nil && len(out) > limit {
		_ = cmd.Process.Kill()
		_ = stdout.Close()
		_ = cmd.Wait()
		return out[:limit], errTooLong
	}
	if werr := cmd.Wait(); err == nil && werr != nil {
		line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
		err = &failure{args: cmd.Args, err: werr, line: line}
	}
	return out, err
}

// failure is the error of a git command that failed.
type failure struct {
	args []string
	err  error
	// line is the first line git printed on its standard error.
	line string
}

func (f *failure) Error() string {
	return fmt.Sprintf("%s: %v: %s", strings.Join(f.args, " "), f.err, f.line)
}

func (f *failure) Unwrap() error { return f.err }

// environ returns the environment of git: this one without its GIT_ variables, with no prompt,
// no optional lock and no replacement of objects.
func environ() []string {
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}

// capped keeps the first limit bytes written to it.
type capped struct {
	bytes.Buffer
	limit int
}

func (c *capped) Write(p []byte) (int, error) {
	c.Buffer.Write(p[:min(len(p), max(c.limit-c.Len(), 0))])
	return len(p), nil
}

// checkGit refuses, with E-PUB-012, a git that is missing or older than 2.31.
func checkGit(ctx context.Context) error {
	out, err := run(exec.CommandContext(ctx, "git", "--version"), "", "", 256)
	if err != nil {
		return err
	}
	m := gitVersion.FindSubmatch(out)
	if m == nil {
		return nil
	}
	major, _ := strconv.Atoi(string(m[1]))
	minor, _ := strconv.Atoi(string(m[2]))
	if major < minMajor || major == minMajor && minor < minMinor {
		return errs.New("E-PUB-012", fmt.Sprintf("git %d.%d is too old: aicoded publish needs git 2.31 or later", major, minor), fixGit)
	}
	return nil
}

// head refuses, with E-PUB-001, a dir that is not the top level of a git repository whose HEAD
// is a commit with its whole history, and returns that commit.
func head(ctx context.Context, dir string) (string, error) {
	const fix = "make the app's folder a git repository of its own, with git init and git commit in it"
	out, err := run(exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel"), dir, "", 4096)
	var f *failure
	switch {
	case errors.As(err, &f):
		return "", errs.New("E-PUB-001", dir+" is not in the working tree of a git repository, as git says: "+f.line, fix)
	case err != nil:
		return "", err
	}
	top, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
	if err != nil {
		return "", err
	}
	here, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if top != here {
		return "", errs.New("E-PUB-001", fmt.Sprintf("%s is a folder inside the git repository %s, whose whole history a publish would send", dir, top), fix)
	}
	out, err = run(exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "HEAD^{commit}"), dir, "", 256)
	sha := strings.TrimSuffix(string(out), "\n")
	switch {
	case errors.As(err, &f):
		return "", errs.New("E-PUB-001", "the git repository "+dir+" has no commit yet", "commit the app with git add -A and git commit")
	case err != nil:
		return "", err
	case !commitSHA.MatchString(sha):
		return "", errs.New("E-PUB-001", "the git repository "+dir+" names its commits with SHA-256, which the platform does not read yet",
			"make the app's folder a git repository with git init --object-format=sha1 and commit it")
	}
	out, err = run(exec.CommandContext(ctx, "git", "rev-parse", "--is-shallow-repository"), dir, "", 256)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(out)) == "true" {
		return "", errs.New("E-PUB-001", "the git repository "+dir+" is a shallow clone, which holds only part of its history",
			"fetch the rest of the history with git fetch --unshallow")
	}
	return sha, nil
}

// clean refuses, with E-PUB-002, a working tree with changes that are not committed, untracked
// files included.
func clean(ctx context.Context, dir string) error {
	out, err := run(exec.CommandContext(ctx, "git", "-c", "core.fsmonitor=", "status", "--porcelain=v1", "--untracked-files=all"), dir, "", 64<<10)
	if err != nil && !errors.Is(err, errTooLong) {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	shown := lines[:min(len(lines), 5)]
	for i, l := range shown {
		shown[i] = strings.TrimSpace(l)
	}
	msg := "the working tree of " + dir + " has changes that are not committed: " + strings.Join(shown, ", ")
	if len(lines) > len(shown) || errors.Is(err, errTooLong) {
		msg += " and more"
	}
	return errs.New("E-PUB-002", msg, "commit them, or remove them, then run aicoded publish again")
}

// subject returns the subject of the commit HEAD.
func subject(ctx context.Context, dir string) (string, error) {
	out, err := run(exec.CommandContext(ctx, "git", "log", "-1", "--no-show-signature", "--format=%s"), dir, "", 64<<10)
	if errors.Is(err, errTooLong) {
		err = nil
	}
	return string(out), err
}

// descends reports whether HEAD descends from the commit base, which this repository holds.
func descends(ctx context.Context, dir, base string) bool {
	out, err := run(exec.CommandContext(ctx, "git", "rev-list", "--max-count=1", "--stdin"), dir, base+"\n^HEAD\n", 256)
	return err == nil && len(out) == 0
}

// makeBundle returns a git bundle of HEAD, which must still be the commit sha, of the app.
// When HEAD descends from base, the bundle holds the history since base, which is its only
// prerequisite; otherwise it holds all of it. A merge of history older than base would need
// more prerequisites than the platform takes, so that bundle holds all of it too. A bundle over
// 32 MiB is E-PUB-009.
func makeBundle(ctx context.Context, dir, app, sha, base string) ([]byte, error) {
	if base != "" && descends(ctx, dir, base) {
		b, prerequisites, err := bundle(ctx, dir, app, sha, "HEAD\n^"+base+"\n")
		if err != nil || slices.Equal(prerequisites, []string{base}) {
			return b, err
		}
	}
	b, prerequisites, err := bundle(ctx, dir, app, sha, "HEAD\n")
	if err == nil && len(prerequisites) > 0 {
		return nil, fmt.Errorf("git bundle create wrote a bundle of all the history of %s that needs other commits", dir)
	}
	return b, err
}

// bundle returns the bundle of the revisions revs and its prerequisites.
func bundle(ctx context.Context, dir, app, sha, revs string) ([]byte, []string, error) {
	b, err := run(exec.CommandContext(ctx, "git", "bundle", "create", "-", "--stdin"), dir, revs, platform.MaxBundle)
	if errors.Is(err, errTooLong) {
		return nil, nil, errs.New("E-PUB-009", "the history of "+app+" to send is larger than 32 MiB",
			"remove large files from the repository and its history, or publish from a new repository that holds only the app")
	}
	if err != nil {
		return nil, nil, err
	}
	prerequisites, refs, ok := header(b)
	switch {
	case !ok:
		return nil, nil, errors.New("git bundle create wrote a header that aicoded cannot read")
	case len(refs) != 1 || refs[0] != sha:
		return nil, nil, errors.New("HEAD changed while aicoded publish ran: run it again once you are done committing")
	}
	return b, prerequisites, nil
}

// header returns the prerequisites and the commits of the refs of a v2 git bundle.
func header(b []byte) (prerequisites, refs []string, ok bool) {
	text, _, ok := bytes.Cut(b, []byte("\n\n"))
	lines := strings.Split(string(text), "\n")
	if !ok || lines[0] != "# v2 git bundle" {
		return nil, nil, false
	}
	for _, l := range lines[1:] {
		if p, ok := strings.CutPrefix(l, "-"); ok {
			sha, _, _ := strings.Cut(p, " ")
			prerequisites = append(prerequisites, sha)
		} else {
			sha, _, _ := strings.Cut(l, " ")
			refs = append(refs, sha)
		}
	}
	return prerequisites, refs, true
}
