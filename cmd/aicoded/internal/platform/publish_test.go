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
		Record: -1})

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
	assert.True(t, got.Done())
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
