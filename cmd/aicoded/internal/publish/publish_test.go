package publish

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/gittest"
	"aicoded.dev/framework/internal/errs"
)

// git runs git with args in dir.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestBundleAfterAMergeOfOlderHistory(t *testing.T) {
	dir := gittest.NewApp(t, "demo")
	git(t, dir, "branch", "side")
	base := gittest.Commit(t, dir, "On main")
	git(t, dir, "checkout", "-q", "side")
	gittest.Commit(t, dir, "On the side")
	git(t, dir, "checkout", "-q", "-")
	git(t, dir, "merge", "-q", "--no-edit", "side")
	head := gittest.Head(t, dir)

	b, err := makeBundle(t.Context(), dir, "demo", head, base)
	require.NoError(t, err)
	prerequisites, refs, ok := header(b)
	require.True(t, ok)
	assert.Empty(t, prerequisites, "the history since base needs two prerequisites, so all of it is sent")
	assert.Equal(t, []string{head}, refs)

	gittest.Commit(t, dir, "After the merge")
	b, err = makeBundle(t.Context(), dir, "demo", gittest.Head(t, dir), head)
	require.NoError(t, err)
	prerequisites, _, _ = header(b)
	assert.Equal(t, []string{head}, prerequisites, "the history since the merge needs only the merge")
}

func TestBundleOfAMovedHead(t *testing.T) {
	dir := gittest.NewApp(t, "demo")
	old := gittest.Head(t, dir)
	gittest.Commit(t, dir, "Meanwhile")
	_, err := makeBundle(t.Context(), dir, "demo", old, "")
	require.EqualError(t, err, "HEAD changed while aicoded publish ran: run it again once you are done committing")
}

func TestBundleOfABaseThisRepositoryLacks(t *testing.T) {
	dir := gittest.NewApp(t, "demo")
	b, err := makeBundle(t.Context(), dir, "demo", gittest.Head(t, dir), strings.Repeat("ab", 20))
	require.NoError(t, err)
	prerequisites, _, _ := header(b)
	assert.Empty(t, prerequisites)
}

func TestGitIgnoresTheGitVariables(t *testing.T) {
	dir, other := gittest.NewApp(t, "demo"), gittest.NewApp(t, "other")
	want := gittest.Head(t, dir)
	gittest.Write(t, other, map[string]string{"notes.txt": "notes\n"})
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	sha, err := head(t.Context(), dir)
	require.NoError(t, err)
	assert.Equal(t, want, sha)
	assert.NoError(t, clean(t.Context(), dir), "the changes are in the other repository")
}

func TestCleanRunsNoCommandOfTheRepository(t *testing.T) {
	dir := gittest.NewApp(t, "demo")
	hook := filepath.Join(t.TempDir(), "hook")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+hook+".ran\n"), 0o700))
	git(t, dir, "config", "core.fsmonitor", hook)
	require.NoError(t, clean(t.Context(), dir))
	assert.NoFileExists(t, hook+".ran", "git status runs no fsmonitor hook")
}

func TestGitMissingOrTooOld(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	err := checkGit(t.Context())
	assert.Equal(t, "E-PUB-012", errs.Code(err))
	require.ErrorContains(t, err, "git is not on PATH")

	for version, ok := range map[string]bool{"2.30.9": false, "1.9.5": false, "2.31.0": true, "2.43.0 (Apple Git-115)": true, "3.0.0": true} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho 'git version "+version+"'\n"), 0o700))
		err := checkGit(t.Context())
		if ok {
			assert.NoError(t, err, version)
		} else {
			assert.Equal(t, "E-PUB-012", errs.Code(err), version)
			assert.ErrorContains(t, err, " is too old: aicoded publish needs git 2.31 or later", version)
		}
	}
}

func TestPrepareTakesTheSummary(t *testing.T) {
	dir := gittest.NewApp(t, "demo")
	cm, err := Prepare(t.Context(), dir, Options{App: "demo"})
	require.NoError(t, err)
	assert.Equal(t, Commit{Dir: dir, App: "demo", SHA: gittest.Head(t, dir), Summary: "Add demo"}, cm, "the commit's subject")

	cm, err = Prepare(t.Context(), dir, Options{Summary: "\x1b\u202e"})
	require.NoError(t, err)
	assert.Equal(t, "Add demo", cm.Summary, "nothing is left of the summary")
}

func TestSummarize(t *testing.T) {
	assert.Equal(t, "Fix the list of rooms", summarize("  Fix the\tlist\nof rooms\r\n"))
	assert.Equal(t, "Bidi text", summarize("Bidi\u202e text\x00\x07"))
	assert.Equal(t, strings.Repeat("é", 1000), summarize(strings.Repeat("é", 1200)))
	assert.Equal(t, strings.Repeat("\U0001F680", 975), summarize(strings.Repeat("\U0001F680", 1000)), "3900 bytes")
	assert.Equal(t, strings.Repeat(`"\`, 500), summarize(strings.Repeat(`"\`, 600)), "2000 bytes of JSON")
}

func TestHeader(t *testing.T) {
	sha := strings.Repeat("ab", 20)
	prerequisites, refs, ok := header([]byte("# v2 git bundle\n-" + sha + " a subject\n" + sha + " HEAD\n\nPACK"))
	assert.True(t, ok)
	assert.Equal(t, []string{sha}, prerequisites)
	assert.Equal(t, []string{sha}, refs)
	for _, b := range []string{"# v3 git bundle\n" + sha + " HEAD\n\nPACK", "# v2 git bundle\n" + sha + " HEAD\n", ""} {
		_, _, ok := header([]byte(b))
		assert.False(t, ok, b)
	}
}
