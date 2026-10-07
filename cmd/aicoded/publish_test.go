package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/gittest"
	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
)

// signedIn starts a fake platform, signs in to it in a home of its own, makes aicoded publish
// ask for the outcome every 10 ms, and returns the platform and a function that runs aicoded.
func signedIn(t *testing.T) (*platformtest.Platform, func(args ...string) (int, string, string)) {
	p, aicoded := fakePlatform(t)
	p.SignIn(t)
	every := pollEvery
	pollEvery = 10 * time.Millisecond
	t.Cleanup(func() { pollEvery = every })
	return p, aicoded
}

var published = regexp.MustCompile(`Published demo at [0-9a-f]{7} as (pub_[a-z2-7]{26})\.\n`)

// publishID returns the id of the publish that out reports.
func publishID(t *testing.T, out string) string {
	t.Helper()
	m := published.FindStringSubmatch(out)
	require.NotNil(t, m, out)
	return m[1]
}

func TestRunPublishSendsTheHistorySinceTheBase(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	first := gittest.Head(t, dir)

	code, out, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)
	assert.Equal(t, "Created the app demo, which you own.\nPublished demo at "+first[:7]+" as "+id+".\n"+
		"aicoded status "+id+" shows its outcome.\n", out)
	uploads := p.Uploads()
	require.Len(t, uploads, 1)
	assert.Equal(t, platformtest.Upload{App: "demo", SHA: first, Summary: "Add demo"}, platformtest.Upload{App: uploads[0].App, SHA: uploads[0].SHA, Summary: uploads[0].Summary})
	assert.True(t, bytes.HasPrefix(uploads[0].Bundle, []byte("# v2 git bundle\n"+first+" HEAD\n\nPACK")), "a full bundle")

	gittest.Write(t, dir, map[string]string{"notes.txt": "notes\n"})
	second := gittest.Commit(t, dir, "Add notes")
	code, out, errOut = aicoded("publish", "-m", "Notes\tfor\nthe team\x1b[31m", dir)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "Published demo at "+second[:7]+" as ")
	assert.NotContains(t, out, "Created the app")
	uploads = p.Uploads()
	require.Len(t, uploads, 2)
	assert.True(t, bytes.HasPrefix(uploads[1].Bundle, []byte("# v2 git bundle\n-"+first+" ")), "only the history since the base")
	assert.Equal(t, "Notes for the team[31m", uploads[1].Summary, "control characters are removed")
	assert.Equal(t, []string{first, second}, gittest.Verify(t, uploads[0].Bundle, uploads[1].Bundle))
	assert.Equal(t, 1, p.TokenRequests(), "the sign-in from before the app existed is refreshed once, for its scopes")
}

func TestRunPublishRewrittenHistory(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	code, _, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)

	amended := gittest.Amend(t, dir, "Add demo, again")
	code, _, errOut = aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	uploads := p.Uploads()
	require.Len(t, uploads, 2)
	assert.True(t, bytes.HasPrefix(uploads[1].Bundle, []byte("# v2 git bundle\n"+amended+" HEAD\n\nPACK")), "a full bundle")
	assert.Equal(t, []string{amended}, gittest.Verify(t, uploads[1].Bundle), "on its own")
}

func TestRunPublishWaits(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	sha := gittest.Head(t, dir)
	p.Answers(platformtest.Publish{Status: "queued"}, platformtest.Publish{Status: "running"}, platformtest.Passed())

	code, out, errOut := aicoded("publish", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)
	assert.Contains(t, out, "\nqueued ...\nrunning ...\npublish "+id+" of demo at "+sha[:7]+": Add demo\n"+
		"  checkout  passed\n  modules   passed\n  check     passed\n  tests     passed\n"+
		"  partial checks: only aicoded check and the tests run; the full security checks come later\n"+
		"  change record 1\npassed\n")
}

func TestRunPublishFails(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	p.Answers(platformtest.Publish{Status: "failed", Checks: "partial", Steps: platformtest.Steps("passed", "passed", "failed", "skipped"),
		Problems: []platformtest.Problem{{Code: "E-GEN-001", Pos: "pages/index.html:3", Message: "route has no access rule\x1b[2J\nsecond line",
			Fix: "add <ssr:access role=\"...\"/>", Docs: "https://evil.example/"}}, Record: 7})

	code, out, errOut := aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, errOut)
	assert.Contains(t, out, "  check     failed\n  tests     skipped\n"+
		"  demo: pages/index.html:3: E-GEN-001: route has no access rule?[2J\n  second line\n"+
		"    fix: add <ssr:access role=\"...\"/>\n    docs: https://aicoded.dev/docs/errors/E-GEN-001\n")
	assert.Contains(t, out, "  change record 7\nfailed: 1 problem\n")
}

func TestRunPublishRefusedAndError(t *testing.T) {
	for status, code := range map[string]string{"refused": "E-PUB-014", "error": "E-PUB-011"} {
		p, aicoded := signedIn(t)
		dir := gittest.NewApp(t, "demo")
		p.Answers(platformtest.Publish{Status: status, Reason: "the bundle does not verify"})
		exit, out, errOut := aicoded("publish", dir)
		assert.Equal(t, 1, exit, status)
		assert.Contains(t, out, "\n"+status+": the bundle does not verify\n", status)
		assert.Contains(t, errOut, code+": ", status)
		assert.Contains(t, errOut, ": the bundle does not verify\n", status)
	}
}

func TestRunPublishStopsWaiting(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	p.Answers(platformtest.Publish{Status: "queued"})
	limit := pollFor
	pollFor = 50 * time.Millisecond
	t.Cleanup(func() { pollFor = limit })

	code, out, errOut := aicoded("publish", dir)
	assert.Equal(t, 1, code)
	id := publishID(t, out)
	assert.Equal(t, "aicoded publish: no outcome after 50ms\nthe publish goes on: aicoded status "+id+"\n", errOut)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	assert.Equal(t, 1, wait(ctx, platform.New(p.URL, &stdout), id, &stdout, &stderr), "as after Ctrl-C")
	assert.Equal(t, "the publish goes on: aicoded status "+id+"\n", stderr.String())

	p.Close()
	pollFor = time.Minute
	code, _, errOut = aicoded("status", id)
	assert.Equal(t, 1, code)
	stderr.Reset()
	assert.Equal(t, 1, wait(t.Context(), platform.New(p.URL, &stdout), id, &stdout, &stderr))
	assert.Equal(t, strings.TrimSuffix(errOut, "\n")+"\nthe publish goes on: aicoded status "+id+"\n", stderr.String(),
		"the fifth failure in a row ends the wait")
}

func TestRunPublishBaseChanged(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	first := gittest.Head(t, dir)
	code, _, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	second := gittest.Commit(t, dir, "Second")
	code, _, errOut = aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)

	third := gittest.Commit(t, dir, "Third")
	p.StaleBase("demo", first)
	code, _, errOut = aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, 4, p.Requests("/v1/publishes/create"), "sent again once")
	uploads := p.Uploads()
	require.Len(t, uploads, 3)
	assert.True(t, bytes.HasPrefix(uploads[2].Bundle, []byte("# v2 git bundle\n-"+second+" ")), "from the new base")
	assert.Equal(t, []string{first, second, third}, gittest.Verify(t, uploads[0].Bundle, uploads[1].Bundle, uploads[2].Bundle))

	gittest.Commit(t, dir, "Fourth")
	p.StaleBase("demo", first, first)
	code, _, errOut = aicoded("publish", "--no-wait", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-010: ")
	assert.Equal(t, 6, p.Requests("/v1/publishes/create"), "sent again only once")
}

func TestRunPublishRefusesTheRepository(t *testing.T) {
	p, aicoded := signedIn(t)
	app := map[string]string{"aicoded.yaml": "app: demo\n", "go.mod": "module demo\n\ngo 1.25.0\n", "main.go": "package main\n\nfunc main() {}\n"}

	outside := t.TempDir()
	gittest.Write(t, outside, app)
	repo := t.TempDir()
	gittest.Init(t, repo)
	gittest.Write(t, repo, map[string]string{"secret.txt": "not the app's\n"})
	gittest.Write(t, repo+"/demo", app)
	gittest.Commit(t, repo, "An app in a folder")
	empty := t.TempDir()
	gittest.Write(t, empty, app)
	gittest.Init(t, empty)
	changed, untracked := gittest.NewApp(t, "demo"), gittest.NewApp(t, "demo")
	gittest.Write(t, changed, map[string]string{"main.go": "package main\n\nfunc main() { println() }\n"})
	gittest.Write(t, untracked, map[string]string{"deps/notes.txt": "notes\n"})

	for dir, want := range map[string]string{
		outside:         "E-PUB-001: " + outside + " is not in the working tree of a git repository, as git says: fatal: not a git repository",
		repo + "/demo":  "E-PUB-001: " + repo + "/demo is a folder inside the git repository " + repo + ", whose whole history a publish would send\n",
		empty:           "E-PUB-001: the git repository " + empty + " has no commit yet\n",
		changed:         "E-PUB-002: the working tree of " + changed + " has changes that are not committed: M main.go\n",
		untracked:       "E-PUB-002: the working tree of " + untracked + " has changes that are not committed: ?? deps/notes.txt\n",
		t.TempDir():     "E-MAN-006: ",
		changed + "/no": "E-MAN-006: ",
	} {
		code, _, errOut := aicoded("publish", dir)
		assert.Equal(t, 1, code, dir)
		assert.Contains(t, errOut, want, dir)
	}
	assert.Zero(t, p.Requests("/v1/apps/get"), "nothing reached the platform")
}

func TestRunPublishRefusesBeforeSending(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	code, _, errOut := aicoded("publish", "--app", "other", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, `E-PUB-003: --app "other" is not the app in `+dir+`, which aicoded.yaml names "demo"`)

	broken := gittest.NewApp(t, "demo")
	gittest.Write(t, broken, map[string]string{"main.go": "package main\n\nfunc main() { x }\n"})
	gittest.Commit(t, broken, "Break it")
	code, out, errOut := aicoded("publish", broken)
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "FAIL demo\n  demo: main.go:3: E-CHK-002: ")
	assert.Contains(t, errOut, "E-PUB-004: aicoded check --frozen --no-tests found problems in demo, so nothing was sent\n")

	replaced := gittest.NewApp(t, "demo")
	gittest.Write(t, replaced, map[string]string{"go.mod": "module demo\n\ngo 1.25.0\n\nreplace example.com/x => ../x\n"})
	gittest.Commit(t, replaced, "Replace a module")
	code, out, errOut = aicoded("publish", replaced)
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "E-LINT-011: ", "no replace line, whichever aicoded publishes")
	assert.Contains(t, errOut, "E-PUB-004: ")

	big := gittest.NewApp(t, "demo")
	data := make([]byte, 33<<20)
	_, _ = rand.Read(data)
	gittest.Write(t, big, map[string]string{"data/big.bin": string(data)})
	gittest.Commit(t, big, "Add a large file")
	code, _, errOut = aicoded("publish", big)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-009: the history of demo to send is larger than 32 MiB\n")
	assert.Zero(t, p.Requests("/v1/publishes/create"), "nothing was sent")
}

func TestRunPublishRefusals(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("theirs", "bo@acme.example")
	dir := gittest.NewApp(t, "theirs")
	code, _, errOut := aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-005: the app theirs belongs to another builder\n")
	assert.Equal(t, 1, p.TokenRequests(), "refused after one refresh")

	dir = gittest.NewApp(t, "demo")
	code, out, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	code, _, errOut = aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-007: demo already published the commit ")
	code, _, errOut = aicoded("status", publishID(t, out))
	require.Equal(t, 0, code, errOut)
	p.Archive("demo")
	gittest.Commit(t, dir, "After the archive")
	code, _, errOut = aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-006: the app demo is archived on the platform\n")

	p.NewAppLimit(1)
	code, _, errOut = aicoded("publish", gittest.NewApp(t, "another"))
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-008: ")
	assert.Len(t, p.Uploads(), 1, "only the first publish was taken")
}

func TestRunPublishSignedOut(t *testing.T) {
	_, aicoded := fakePlatform(t)
	code, _, errOut := aicoded("publish", gittest.NewApp(t, "demo"))
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-CLI-004: not signed in to the platform at ")
	code, _, errOut = aicoded("status", "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-CLI-004: ")
}

func TestRunStatus(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	code, out, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)
	p.Answers(platformtest.Publish{Status: "running", Steps: []platformtest.Step{{Name: "checkout", Outcome: "passed"}}})

	code, out, errOut = aicoded("status", id)
	assert.Equal(t, 0, code, errOut)
	assert.Equal(t, "publish "+id+" of demo at "+gittest.Head(t, dir)[:7]+": Add demo\n"+
		"  checkout  passed\nrunning\n", out)

	code, out, errOut = aicoded("status", "--json", id)
	assert.Equal(t, 0, code, errOut)
	var got platform.Publish
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "running", got.Status)

	code, _, errOut = aicoded("status", "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-013: ")
}

func TestRunStatusUsage(t *testing.T) {
	p, aicoded := signedIn(t)
	for _, args := range [][]string{
		{"status"}, {"status", "pub_1"}, {"status", "PUB_AAAAAAAAAAAAAAAAAAAAAAAAAA"}, {"status", "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa", "x"},
		{"status", "../pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"}, {"publish", "a", "b"}, {"publish", "--nope"},
	} {
		code, _, _ := aicoded(args...)
		assert.Equal(t, 2, code, args)
	}
	_, _, errOut := aicoded("status", "pub_1")
	assert.Contains(t, errOut, `aicoded status: "pub_1" is not the id of a publish`)
	assert.Zero(t, p.Requests("/v1/publishes/get"))
}
