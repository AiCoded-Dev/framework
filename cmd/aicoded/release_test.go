package main

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/gittest"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
)

var previewLines = `Preview of demo at https://demo\.acme\.preview\.aicoded\.cloud/ until \S+Z\.\nInvite up to five people in the console\.\n`

const limitsLine = `The preview keeps its first end, reaches no outside service that production does not reach yet, and keeps its mail in the console\.\n`

func TestPublishHoldsForPreviewThenRelease(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")

	code, out, errOut := aicoded("publish", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)
	assert.NotContains(t, out, "E-REL-039", "the note for older aicoded is dropped: the held lines say it")
	assert.Regexp(t, `  tests     passed\n  partial checks: .*\n  change record 1\n  Pre-approved by app type internal-tool\.\n`+
		`  Held for preview, not live: its owner asks for its release with aicoded release `+id+`\.\npassed\n`+previewLines+limitsLine+
		`It is not live\. When the app's owner asks for its release: aicoded release `+id+`\.\n$`, out)
	require.NotNil(t, p.Uploads()[0].Release)
	assert.False(t, *p.Uploads()[0].Release)
	running, _ := p.Preview("demo")
	assert.Equal(t, id, running)

	due := &platformtest.Release{Tier: "R1", AppType: "internal-tool@builtin", Reasons: []platformtest.Problem{}, State: "due"}
	p.ReleaseAnswers(due, due, platformtest.Live("demo"))
	code, out, errOut = aicoded("release", id)
	require.Equal(t, 0, code, errOut)
	assert.Regexp(t, `^going live \.\.\.\npublish `+id+` of demo at [0-9a-f]{7}: Add demo\n(.*\n)*`+
		`  Pre-approved by app type internal-tool: live\.\n  Live at https://demo\.acme\.apps\.aicoded\.cloud/\npassed\n$`, out)
	assert.NotContains(t, out, "E-REL-039", "no note once its release was asked for")

	code, again, errOut := aicoded("release", id)
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, strings.TrimPrefix(out, "going live ...\n"), again, "a request made again shows the publish as it is")
}

func TestPublishPreviewThatDoesNotStart(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	p.RefusePreview(http.StatusConflict, "conflict", "E-REL-030", "the permission list declares the secret partner_api_key with no value set for the preview")

	code, out, errOut := aicoded("publish", dir)
	assert.Equal(t, 0, code, "the publish passed")
	id := publishID(t, out)
	assert.True(t, strings.HasSuffix(out, "  Held for preview, not live: its owner asks for its release with aicoded release "+id+".\npassed\n"+
		"The publish passed and is held; it is not live. Fix that and run aicoded preview "+id+
		". Its owner may also release it without a preview, only at their word: aicoded release "+id+".\n"), out)
	assert.Equal(t, "E-REL-030: the permission list declares the secret partner_api_key with no value set for the preview\n"+
		"  fix: ask the app's owner or a security admin to set the names the message lists for the preview in the console\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-030\n", errOut)
	_, running := p.Preview("demo")
	assert.False(t, running)

	code, out, errOut = aicoded("preview", id)
	require.Equal(t, 0, code, errOut)
	assert.Regexp(t, "^"+previewLines+"$", out, "started later")
}

func TestPublishNoWait(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	code, out, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)
	assert.Contains(t, out, "aicoded status "+id+" shows its outcome.\n"+
		"Once it passed, aicoded preview "+id+" starts its preview. It is not live. When the app's owner asks for its release: aicoded release "+id+".\n")
	assert.Zero(t, p.Requests("/v1/previews/start"))
}

func TestPublishStraightToProduction(t *testing.T) {
	waiting := &platformtest.Release{Tier: "R2", AppType: "internal-tool@builtin", State: "waiting",
		Reasons: []platformtest.Problem{{Code: "E-GATE-043", Pos: "aicoded.yaml:4", Message: "people outside the company may use it"}}}
	for _, tc := range []struct {
		release *platformtest.Release
		want    string
	}{
		{platformtest.Live("demo"), "  Pre-approved by app type internal-tool: live.\n  Live at https://demo.acme.apps.aicoded.cloud/\npassed\n"},
		{waiting, "  Needs approval:\n    E-GATE-043: people outside the company may use it\n  An approver must approve it before it goes live.\npassed\n"},
	} {
		p, aicoded := signedIn(t)
		dir := gittest.NewApp(t, "demo")
		p.Answers(platformtest.Publish{Status: "passed", Release: tc.release})
		code, out, errOut := aicoded("publish", "--release", dir)
		require.Equal(t, 0, code, errOut)
		assert.Contains(t, out, "It skips the preview: once it passed, it goes live by itself or waits for an approver.\n")
		assert.True(t, strings.HasSuffix(out, tc.want), out)
		assert.True(t, *p.Uploads()[0].Release)
		assert.Zero(t, p.Requests("/v1/previews/start"))
	}

	_, aicoded := signedIn(t)
	code, out, errOut := aicoded("publish", "--release", "--no-wait", gittest.NewApp(t, "demo"))
	require.Equal(t, 0, code, errOut)
	assert.Regexp(t, `It skips the preview: .*\naicoded status pub_\w+ shows its outcome\.\n$`, out)
}

func TestPublishFlagsOfOldAndNew(t *testing.T) {
	p, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	for _, flag := range []string{"--preview", "-preview", "--preview=true"} {
		code, _, errOut := aicoded("publish", flag, dir)
		assert.Equal(t, 2, code, flag)
		assert.Contains(t, errOut, "aicoded publish: --preview is gone: every publish is held for its owner's preview now; --release skips it\n")
	}
	assert.False(t, previewFlag([]string{"-m", "--preview", dir}), "a summary is a value, not a flag")
	assert.False(t, previewFlag([]string{"--app=demo", dir, "--preview"}), "nothing after the folder is a flag")

	p.Old()
	code, _, errOut := aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-CLI-010: the platform at "+p.URL+" is older than this aicoded: it does not hold publishes for their owner's preview\n")
	assert.Empty(t, p.Uploads(), "a platform that would not hold it takes nothing")

	code, _, errOut = aicoded("release", "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, errOut, "flags of aicoded release:\n  -no-wait\n")
}

func TestPublishSameCommitWhileHeld(t *testing.T) {
	_, aicoded := signedIn(t)
	dir := gittest.NewApp(t, "demo")
	code, out, errOut := aicoded("publish", "--no-wait", dir)
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)

	code, _, errOut = aicoded("publish", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-007: this commit is already published to the app as publish "+id+
		", which is held for its owner's preview\n  fix: preview it with aicoded preview "+id+", and release it only when the owner asks: aicoded release "+id+"\n")
}

func TestPublishReplacedACard(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	older := p.AddPublish("demo")
	p.Answers(platformtest.Publish{Status: "passed", Release: &platformtest.Release{Tier: "R1", AppType: "internal-tool@builtin",
		Reasons: []platformtest.Problem{}, Replaced: older, ReplacedState: "waiting"}})
	code, out, errOut := aicoded("publish", "--no-wait", gittest.NewApp(t, "demo"))
	require.Equal(t, 0, code, errOut)
	id := publishID(t, out)

	code, out, errOut = aicoded("status", id)
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "  This publish replaced "+older+", which was waiting for approval; its approval card is gone. "+
		"Ask for this one's release to get a new card.\n")
}

func TestReleaseOutcomes(t *testing.T) {
	waiting := &platformtest.Release{Tier: "R2", AppType: "internal-tool@builtin", State: "waiting",
		Reasons: []platformtest.Problem{{Code: "E-GATE-043", Pos: "aicoded.yaml:4", Message: "people outside the company may use it"}}}
	refused := &platformtest.Release{Tier: "R3", AppType: "internal-tool@builtin", State: "refused", Reasons: []platformtest.Problem{},
		Reason: "E-REL-021: the source sqldb holds hr data"}
	due := &platformtest.Release{Tier: "R1", AppType: "internal-tool@builtin", Reasons: []platformtest.Problem{}, State: "due"}
	for _, tc := range []struct {
		release *platformtest.Release
		args    []string
		code    int
		want    string
	}{
		{waiting, nil, 0, "  Needs approval:\n    E-GATE-043: people outside the company may use it\n  An approver must approve it before it goes live.\npassed\n"},
		{refused, nil, 1, "  Not allowed: E-REL-021: the source sqldb holds hr data.\npassed\n"},
		{due, []string{"--no-wait"}, 0, "  Pre-approved by app type internal-tool: going live.\npassed\n"},
	} {
		p, aicoded := signedIn(t)
		p.AddApp("demo", platformtest.Email)
		id := p.AddPublish("demo")
		p.Hold(id)
		p.ReleaseAnswers(tc.release)
		code, out, errOut := aicoded(append(append([]string{"release"}, tc.args...), id)...)
		assert.Equal(t, tc.code, code, errOut)
		assert.Regexp(t, "^publish "+id+" of demo at [0-9a-f]{7}: Change\n", out)
		assert.Contains(t, out, tc.want)
	}
}

func TestReleaseRefused(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	older := p.AddPublish("demo")
	p.Hold(older)
	newer := p.AddPublish("demo")

	code, out, errOut := aicoded("release", newer)
	assert.Equal(t, 1, code)
	assert.Empty(t, out)
	assert.Equal(t, "E-REL-037: publish "+newer+" is not held for its owner's preview: it was published with --release, so its release was asked for at once\n"+
		"  fix: release only a held publish that passed its checks: aicoded status shows it as held\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-037\n", errOut)

	code, _, errOut = aicoded("release", older)
	assert.Equal(t, 1, code)
	assert.Equal(t, "E-REL-038: publish "+older+" was superseded by a newer publish of the app, and only the app's newest publish is released\n"+
		"  fix: release the app's newest publish\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-038\n", errOut)

	asked := p.Requests("/v1/publishes/release")
	code, _, errOut = aicoded("release", "pub_x")
	assert.Equal(t, 2, code)
	assert.Regexp(t, regexp.QuoteMeta(`aicoded release: "pub_x" is not the id of a publish`), errOut)
	assert.Equal(t, asked, p.Requests("/v1/publishes/release"), "nothing was asked of a malformed id")
}
