// Package gittest makes git repositories of apps for the tests of aicoded publish. Its git
// commands have constant arguments: messages and bundles go on standard input or in files.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// NewApp makes, in a new temp folder, the app name, which aicoded check passes without the
// framework, as a git repository with one commit, and returns the folder. It skips the test
// when git or go is not on PATH.
func NewApp(t testing.TB, name string) string {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not on PATH")
		}
	}
	dir := t.TempDir()
	Write(t, dir, map[string]string{
		"aicoded.yaml": "app: " + name + "\n# access is written by aicoded generate from <ssr:access>; do not edit it\naccess: {}\n",
		"go.mod":       "module " + name + "\n\ngo 1.25.0\n",
		"main.go":      "package main\n\nfunc main() {}\n",
	})
	Init(t, dir)
	Commit(t, dir, "Add "+name)
	return dir
}

// Init makes dir a git repository with no commit.
func Init(t testing.TB, dir string) {
	t.Helper()
	git(t, exec.CommandContext(t.Context(), "git", "init", "-q"), dir, "")
	git(t, exec.CommandContext(t.Context(), "git", "config", "user.name", "Ana"), dir, "")
	git(t, exec.CommandContext(t.Context(), "git", "config", "user.email", "ana@acme.example"), dir, "")
	git(t, exec.CommandContext(t.Context(), "git", "config", "commit.gpgsign", "false"), dir, "")
}

// Write writes files, by their paths relative to dir.
func Write(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Commit commits everything in the repository dir with message, and returns the commit.
func Commit(t testing.TB, dir, message string) string {
	t.Helper()
	git(t, exec.CommandContext(t.Context(), "git", "add", "-A"), dir, "")
	git(t, exec.CommandContext(t.Context(), "git", "commit", "-q", "--allow-empty", "-F", "-"), dir, message)
	return Head(t, dir)
}

// Amend replaces the last commit of dir with one of the message, and returns it.
func Amend(t testing.TB, dir, message string) string {
	t.Helper()
	git(t, exec.CommandContext(t.Context(), "git", "add", "-A"), dir, "")
	git(t, exec.CommandContext(t.Context(), "git", "commit", "-q", "--amend", "--allow-empty", "-F", "-"), dir, message)
	return Head(t, dir)
}

// Head returns the commit HEAD of dir.
func Head(t testing.TB, dir string) string {
	t.Helper()
	return strings.TrimSpace(git(t, exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD"), dir, ""))
}

// Verify checks each bundle in turn as the platform's worker does, in a new repository that
// holds the history of the bundles before it: git bundle verify must accept it, and its HEAD
// is then fetched. It returns the commit each bundle's HEAD names.
func Verify(t testing.TB, bundles ...[]byte) []string {
	t.Helper()
	dir := t.TempDir()
	git(t, exec.CommandContext(t.Context(), "git", "init", "-q"), dir, "")
	var heads []string
	for _, b := range bundles {
		if err := os.WriteFile(filepath.Join(dir, "x.bundle"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		git(t, exec.CommandContext(t.Context(), "git", "bundle", "verify", "-q", "x.bundle"), dir, "")
		git(t, exec.CommandContext(t.Context(), "git", "fetch", "-q", "x.bundle", "+HEAD:refs/heads/published"), dir, "")
		heads = append(heads, strings.TrimSpace(git(t, exec.CommandContext(t.Context(), "git", "rev-parse", "refs/heads/published"), dir, "")))
	}
	return heads
}

// git runs cmd in dir with in on its standard input and returns its output, failing the test
// when it fails.
func git(t testing.TB, cmd *exec.Cmd, dir, in string) string {
	t.Helper()
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(in)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, out)
	}
	return string(out)
}
