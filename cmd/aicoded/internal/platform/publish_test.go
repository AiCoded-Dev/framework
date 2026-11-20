package platform_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

var sha = strings.Repeat("ab", 20)

// signedIn returns a client of a new fake platform, signed in to it in a home of its own.
func signedIn(t *testing.T) (*platformtest.Platform, *platform.Client) {
	testhome.Set(t)
	p := platformtest.New(t)
	p.SignIn(t)
	return p, platform.New(p.URL, &bytes.Buffer{})
}

// upload is a publish of the commit sha of demo, whose bundle has only a header.
func upload(sha string) platform.Upload {
	return platform.Upload{App: "demo", SHA: sha, Summary: "First", Bundle: []byte("# v2 git bundle\n" + sha + " HEAD\n\nPACK")}
}

func TestCreatePublishRefused(t *testing.T) {
	for _, tc := range []struct {
		status          int
		errorCode, code string
		want            string
	}{
		{http.StatusForbidden, "access_denied", "", "E-PUB-005: refused as the test asked"},
		{http.StatusForbidden, "access_denied", "E-PUB-006", "E-PUB-006: refused as the test asked"},
		{http.StatusConflict, "conflict", "E-PUB-006", "E-PUB-006: refused as the test asked"},
		{http.StatusConflict, "conflict", "E-PUB-007", "E-PUB-007: refused as the test asked"},
		{http.StatusTooManyRequests, "too_many_requests", "E-PUB-008", "E-PUB-008: refused as the test asked"},
		{http.StatusRequestEntityTooLarge, "too_large", "", "E-PUB-009: refused as the test asked"},
		{http.StatusBadRequest, "invalid_request", "E-PUB-010", "E-PUB-010: refused as the test asked"},
		{http.StatusConflict, "base_changed", "E-PUB-010", platform.ErrBaseChanged.Error()},
		{http.StatusServiceUnavailable, "temporarily_unavailable", "", "the platform answered 503: temporarily_unavailable: refused as the test asked"},
		{http.StatusConflict, "conflict", "E-PUB-999", "E-PUB-999: refused as the test asked\n  fix: read the docs page below"},
		{http.StatusConflict, "conflict", "E-NOPE-001", "the platform answered 409: conflict: refused as the test asked"},
		{http.StatusConflict, "conflict", "E-MIG-001", "E-MIG-001: refused as the test asked\n  fix: keep the draft to 64 KiB of UTF-8"},
		{http.StatusBadRequest, "invalid_request", "E-MIG-002", "E-MIG-002: refused as the test asked\n" +
			"  fix: write only CREATE TABLE, CREATE INDEX and ALTER TABLE … ADD statements on the app's own tables, or the destructive forms of E-MIG-003, " +
			"give a new NOT NULL column a DEFAULT, and change data from the app's code\n" +
			"  docs: https://aicoded.dev/docs/errors/E-MIG-002"},
		{http.StatusBadRequest, "invalid_request", "E-MIG-999", "the platform answered 400: invalid_request: refused as the test asked"},
	} {
		p, c := signedIn(t)
		p.RefuseUpload(tc.status, tc.errorCode, tc.code)
		_, err := c.CreatePublish(t.Context(), upload(sha))
		require.Error(t, err, tc.want)
		assert.True(t, strings.HasPrefix(err.Error(), tc.want), "%s\n%s", tc.want, err)
		if tc.errorCode == "base_changed" {
			assert.ErrorIs(t, err, platform.ErrBaseChanged)
		}
	}
}

func TestCreatePublishRefreshesForAScope(t *testing.T) {
	p, c := signedIn(t)
	p.RefuseUpload(http.StatusForbidden, "insufficient_scope", "E-PUB-005")
	created, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	assert.True(t, created.CreatedApp)
	assert.Equal(t, 1, p.TokenRequests(), "refreshed once")
	assert.Equal(t, 2, p.Requests("/v1/publishes/create"), "and sent again")
}

func TestAppRefreshesForItsScopes(t *testing.T) {
	p, c := signedIn(t)
	_, found, err := c.App(t.Context(), "demo")
	require.NoError(t, err)
	assert.False(t, found)
	_, err = c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	assert.Zero(t, p.TokenRequests())

	app, found, err := c.App(t.Context(), "demo")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, sha, app.Base)
	require.NotNil(t, app.Last)
	assert.Equal(t, sha, app.Last.SHA)
	assert.Equal(t, 1, p.TokenRequests(), "the token from before the app existed is refreshed once")

	p.AddApp("theirs", "bo@acme.example")
	_, _, err = c.App(t.Context(), "theirs")
	assert.Equal(t, "E-PUB-005", errs.Code(err))
	assert.Equal(t, 2, p.TokenRequests(), "refused after one more refresh")

	p.Archive("demo")
	p.SignIn(t)
	_, _, err = c.App(t.Context(), "demo")
	assert.Equal(t, "E-PUB-006", errs.Code(err), "an archived app drops out of the scopes")
	require.ErrorContains(t, err, "the app is archived and takes no more publishes")
	assert.Equal(t, 3, p.TokenRequests(), "refused after one more refresh")
	_, err = c.Publish(t.Context(), app.Last.ID)
	assert.Equal(t, "E-PUB-006", errs.Code(err))
}

func TestRefreshKeepsANewerToken(t *testing.T) {
	p, c := signedIn(t)
	tok, err := c.Token(t.Context())
	require.NoError(t, err)
	got, err := c.Refresh(t.Context(), "aicoded_at_older")
	require.NoError(t, err)
	assert.Equal(t, tok, got, "another command refreshed the sign-in already")
	assert.Zero(t, p.TokenRequests())

	got, err = c.Refresh(t.Context(), tok)
	require.NoError(t, err)
	assert.NotEqual(t, tok, got)
	assert.Equal(t, 1, p.TokenRequests())
}

func TestBuilderAPIRefreshesAfterA401(t *testing.T) {
	p, c := signedIn(t)
	p.RejectTokens(1)
	_, found, err := c.App(t.Context(), "demo")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, 1, p.TokenRequests(), "refreshed once")
	assert.Equal(t, 2, p.Requests("/v1/apps/get"), "and asked again")

	p.RejectTokens(2)
	_, _, err = c.App(t.Context(), "demo")
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "the platform at "+p.URL+" no longer accepts your sign-in")
	assert.Equal(t, 2, p.TokenRequests(), "refreshed once more, and no more")
}

func TestBuilderAPISignedOut(t *testing.T) {
	p, c := signedIn(t)
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_revoked",
		RefreshToken: "aicoded_rt_revoked", Expires: time.Now().Add(time.Hour)}))
	_, _, err := c.App(t.Context(), "demo")
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "your sign-in to the platform at "+p.URL+" has ended: the refresh token is not valid")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr, "the refresh refused for good ends the sign-in")
}

func TestPublishIsCleaned(t *testing.T) {
	p, c := signedIn(t)
	created, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	p.Answers(platformtest.Publish{Status: "error", Reason: "out of\x1b[2J time", Checks: "partial",
		Steps: []platformtest.Step{{Name: "tests\u202e", Outcome: "error"}},
		Problems: []platformtest.Problem{
			{Code: "E-GATE-012", Pos: "go.mod:1\r", Message: "first\x07\nsecond", Fix: "try again", Docs: "https://evil.example/"},
			{Code: "rm -rf", Message: "no code"},
		},
		Notes:  []platformtest.Problem{{Code: "E-GATE-025", Pos: "main.go:3\x1b", Message: "a directive\u202e", Fix: "remove it", Docs: "https://evil.example/"}},
		Record: -1,
		Release: &platformtest.Release{Tier: "R2\x1b", AppType: "internal-tool@builtin\u202e", State: "refused\r", Reason: "no\x07", Live: true,
			Reasons: []platformtest.Problem{{Code: "E-GATE-043", Pos: "aicoded.yaml:4\x1b", Message: "outside\u202e people", Docs: "https://evil.example/"}}},
		Declined: &platformtest.Declined{By: "ann@acme.example\x1b[2J", Reason: "not\u202e this\nway", At: "2026-10-29T10:00:00Z"}})

	got, err := c.Publish(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "demo", got.App)
	assert.Equal(t, "out of?[2J time", got.Reason)
	assert.Equal(t, []platform.Step{{Name: "tests?", Outcome: "error"}}, got.Steps)
	assert.Equal(t, []problem.Problem{
		{Code: "E-GATE-012", Pos: "go.mod:1?", Message: "first?\nsecond", Fix: "try again", Docs: "https://aicoded.dev/docs/errors/E-GATE-012"},
		{Message: "no code"},
	}, got.Problems)
	assert.Equal(t, []problem.Problem{
		{Code: "E-GATE-025", Pos: "main.go:3?", Message: "a directive?", Fix: "remove it", Docs: "https://aicoded.dev/docs/errors/E-GATE-025"},
	}, got.Notes, "notes are cleaned as problems are")
	assert.Zero(t, got.Record)
	assert.Equal(t, &platform.Release{Tier: "R2?", AppType: "internal-tool@builtin?", State: "refused?", Reason: "no?", Live: true,
		Reasons: []problem.Problem{{Code: "E-GATE-043", Pos: "aicoded.yaml:4?", Message: "outside? people", Docs: "https://aicoded.dev/docs/errors/E-GATE-043"}}},
		got.Release, "the decision is cleaned as the rest")
	assert.Equal(t, &platform.Declined{By: "ann@acme.example?[2J", Reason: "not? this\nway", At: time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)},
		got.Declined, "the decline is cleaned as the rest")
	assert.True(t, got.Done())
}

func TestPublishLiveAddress(t *testing.T) {
	p, c := signedIn(t)
	u := upload(sha)
	u.Release = true
	created, err := c.CreatePublish(t.Context(), u)
	require.NoError(t, err)
	live := platformtest.Passed()
	live.Release = platformtest.Live("demo")
	p.Answers(live)
	got, err := c.Publish(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://demo.acme.apps.aicoded.cloud/", got.Release.URL)

	live.Release.Live = false
	p.Answers(live)
	_, err = c.Publish(t.Context(), created.ID)
	require.EqualError(t, err, "the platform's answer about the publish "+created.ID+" is not valid", "an address of a release that is not live")

	live.Release.Live, live.Release.URL = true, ""
	p.Answers(live)
	got, err = c.Publish(t.Context(), created.ID)
	require.NoError(t, err, "an older platform sends no address")
	assert.Empty(t, got.Release.URL)

	for _, url := range []string{
		"http://demo.acme.apps.aicoded.cloud/",
		"https://ana:pw@demo.acme.apps.aicoded.cloud/",
		"https://demo.acme.apps.aicoded.cloud:8443/",
		"https://demo.acme.apps.aicoded.cloud",
		"https://demo.acme.apps.aicoded.cloud/admin",
		"https://demo.acme.apps.aicoded.cloud/?next=evil.example",
		"https://demo.acme.apps.aicoded.cloud/#x",
		"https://localhost/",
		"https://Demo.acme.apps.aicoded.cloud/",
		"https://demo.acme.apps.aicoded.cloud\x1b[2J/",
		"javascript:alert(1)//",
		"https://" + strings.Repeat("a.", 127) + "cloud/",
	} {
		live.Release.URL = url
		p.Answers(live)
		_, err = c.Publish(t.Context(), created.ID)
		require.EqualError(t, err, "the platform's answer about the publish "+created.ID+" is not valid", url)
	}
}

func TestPublishOfManyProblems(t *testing.T) {
	p, c := signedIn(t)
	created, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	p.Answers(platformtest.Failed(400, 4000))
	got, err := c.Publish(t.Context(), created.ID)
	require.NoError(t, err, "an answer of 1.6 MB")
	assert.Len(t, got.Problems, 400)
	assert.Equal(t, []problem.Problem{}, got.Notes, "an answer without notes, as an older platform sends")

	p.Answers(platformtest.Failed(2200, 4000))
	_, err = c.Publish(t.Context(), created.ID)
	require.EqualError(t, err, "the platform's answer is larger than 8 MiB, more than aicoded reads")
}

func TestPublishRefused(t *testing.T) {
	p, c := signedIn(t)
	_, err := c.Publish(t.Context(), "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Equal(t, "E-PUB-013", errs.Code(err))

	created, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	p.Answers(platformtest.Publish{Status: "approved"})
	_, err = c.Publish(t.Context(), created.ID)
	require.EqualError(t, err, "the platform's answer about the publish "+created.ID+" is not valid")
}

func TestCreatePublishSendsRelease(t *testing.T) {
	p, c := signedIn(t)
	_, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	straight := upload(strings.Repeat("cd", 20))
	straight.Release = true
	_, err = c.CreatePublish(t.Context(), straight)
	require.NoError(t, err)

	uploads := p.Uploads()
	require.Len(t, uploads, 2)
	require.NotNil(t, uploads[0].Release, "the key is always sent")
	assert.False(t, *uploads[0].Release)
	require.NotNil(t, uploads[1].Release)
	assert.True(t, *uploads[1].Release)
}

func TestCreatePublishOnAnOldPlatform(t *testing.T) {
	p, c := signedIn(t)
	p.Old()
	_, err := c.CreatePublish(t.Context(), upload(sha))
	assert.Equal(t, "E-CLI-010: the platform at "+p.URL+" is older than this aicoded: it does not hold publishes for their owner's preview\n"+
		"  fix: use the aicoded version the platform's docs name, or wait for the platform update\n"+
		"  docs: https://aicoded.dev/docs/errors/E-CLI-010", err.Error())
	assert.Empty(t, p.Uploads(), "nothing is published where it would not be held")
}

func TestPublishReplaced(t *testing.T) {
	p, c := signedIn(t)
	created, err := c.CreatePublish(t.Context(), upload(sha))
	require.NoError(t, err)
	older := p.AddPublish("demo")
	p.Answers(platformtest.Publish{Status: "passed", Release: &platformtest.Release{Tier: "R2", State: "waiting", Replaced: older,
		ReplacedState: "due"}})
	got, err := c.Publish(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{older, "held", "due"}, []string{got.Release.Replaced, got.Release.State, got.Release.ReplacedState})

	for _, bad := range []platformtest.Release{
		{Replaced: "pub_x\x1b[2J", ReplacedState: "waiting"},
		{Replaced: older, ReplacedState: "waiting\x1b"},
		{Replaced: older},
		{ReplacedState: "due"},
	} {
		bad.Tier, bad.State = "R2", "waiting"
		p.Answers(platformtest.Publish{Status: "passed", Release: &bad})
		_, err = c.Publish(t.Context(), created.ID)
		require.EqualError(t, err, "the platform's answer about the publish "+created.ID+" is not valid", "%+v", bad)
	}
}

// The note E-REL-039 is for clients that do not know the held state: it is dropped only from a
// publish whose state says held.
func TestHeldNote(t *testing.T) {
	const id = "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	note := map[string]any{"code": "E-REL-039", "message": "held for its owner's preview", "fix": "aicoded release " + id}
	for state, notes := range map[string]int{"held": 0, "due": 1} {
		c := answering(t, http.StatusOK, map[string]any{"id": id, "sha": sha, "status": "passed", "notes": []any{note},
			"release": map[string]any{"tier": "R1", "state": state}})
		got, err := c.Publish(t.Context(), id)
		require.NoError(t, err)
		assert.Len(t, got.Notes, notes, state)
	}
}

func TestRelease(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	id := p.AddPublish("demo")
	p.Hold(id)

	got, err := c.Publish(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, &platform.Release{Tier: "R1", AppType: "internal-tool@builtin", Reasons: []problem.Problem{}, State: "held"}, got.Release)
	assert.Empty(t, got.Notes, "the note E-REL-039 for older aicoded is dropped: the state says held")

	got, err = c.Release(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "https://demo.acme.apps.aicoded.cloud/", got.Release.URL)

	again, err := c.Release(t.Context(), id)
	require.NoError(t, err, "a request made again")
	assert.Equal(t, got, again)

	plain := p.AddPublish("demo")
	_, err = c.Release(t.Context(), plain)
	assert.Equal(t, "E-REL-037: publish "+plain+" is not held for its owner's preview: it was published with --release, so its release was asked for at once\n"+
		"  fix: release only a held publish that passed its checks: aicoded status shows it as held\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-037", err.Error())
}

func TestReleaseRefused(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.AddApp("payroll", "bo@acme.example")
	older, payroll := p.AddPublish("demo"), p.AddPublish("payroll")
	p.Hold(older)
	p.AddPublish("demo")

	for id, code := range map[string]string{older: "E-REL-038", payroll: "E-PUB-005", "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa": "E-PUB-013"} {
		_, err := c.Release(t.Context(), id)
		assert.Equal(t, code, errs.Code(err), id)
	}
	_, err := c.Release(t.Context(), older)
	assert.Contains(t, err.Error(), "fix: release the app's newest publish")

	p.Archive("demo")
	_, err = c.Release(t.Context(), older)
	assert.Equal(t, "E-PUB-006", errs.Code(err))
}

func TestReleaseChecksTheAnswer(t *testing.T) {
	const id = "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err := answering(t, http.StatusOK, map[string]any{"id": "pub_bbbbbbbbbbbbbbbbbbbbbbbbbb", "sha": sha, "status": "passed"}).Release(t.Context(), id)
	require.EqualError(t, err, "the platform's answer about the publish "+id+" is not valid")
	_, err = answering(t, http.StatusAccepted, map[string]any{"id": id, "sha": sha, "status": "passed"}).Release(t.Context(), id)
	require.EqualError(t, err, "the platform answered 202")
	_, err = answering(t, http.StatusNotFound, map[string]any{"error": "no_route"}).Release(t.Context(), id)
	require.EqualError(t, err, "the platform answered 404: no_route: ", "a 404 of no route is not an unknown publish")
	_, err = answering(t, http.StatusNotFound, map[string]any{"error": "not_found"}).Release(t.Context(), id)
	assert.Equal(t, "E-PUB-013", errs.Code(err))
}
