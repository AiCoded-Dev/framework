package main

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
)

func TestPreviewStart(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	first, second := p.AddPublish("demo"), p.AddPublish("demo")

	code, out, errOut := aicoded("preview", first)
	require.Equal(t, 0, code, errOut)
	m := regexp.MustCompile(`^Preview of demo at https://demo\.acme\.preview\.aicoded\.cloud/ until (\S+Z)\.\n` +
		`Invite up to five people in the console\.\n$`).FindStringSubmatch(out)
	require.NotNil(t, m, out)
	expires, err := time.Parse(time.RFC3339, m[1])
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(336*time.Hour), expires, time.Minute)

	p.ResetPreviewData()
	code, out, errOut = aicoded("preview", second)
	require.Equal(t, 0, code, errOut)
	assert.Regexp(t, `^Preview of demo at https://demo\.acme\.preview\.aicoded\.cloud/ until \S+Z\.\n`+
		`Its data was reset: the preview had run schema changes that this publish does not continue\.\n`+
		`Invite up to five people in the console\.\n$`, out)
	running, _ := p.Preview("demo")
	assert.Equal(t, second, running)
}

func TestPreviewRefused(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	id := p.AddPublish("demo")

	p.Answers(platformtest.Failed(1, 10))
	code, out, errOut := aicoded("preview", id)
	assert.Equal(t, 1, code)
	assert.Empty(t, out)
	assert.Equal(t, "E-REL-029: the publish "+id+" did not pass its checks\n"+
		"  fix: preview a publish that passed its checks; publish again to get one\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-029\n", errOut)

	p.Answers(platformtest.Passed())
	p.RefusePreview(http.StatusConflict, "conflict", "E-REL-030", "the permission list declares the secret partner_api_key with no value set for the preview")
	code, _, errOut = aicoded("preview", id)
	assert.Equal(t, 1, code)
	assert.Equal(t, "E-REL-030: the permission list declares the secret partner_api_key with no value set for the preview\n"+
		"  fix: ask the app's owner or a security admin to set the names the message lists for the preview in the console\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-030\n", errOut)

	asked := p.Requests("/v1/previews/start")
	code, _, errOut = aicoded("preview", "pub_x")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, `"pub_x" is not the id of a publish`)
	assert.Equal(t, asked, p.Requests("/v1/previews/start"), "nothing was asked of a malformed id")
}

func TestPreviewEnd(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	code, _, errOut := aicoded("preview", p.AddPublish("demo"))
	require.Equal(t, 0, code, errOut)
	dir := demoDir(t)

	code, out, errOut := aicoded("preview", "end", dir)
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, "Preview of demo ended; its data is being deleted.\n", out)
	_, running := p.Preview("demo")
	assert.False(t, running)

	code, out, errOut = aicoded("preview", "end", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, out)
	assert.Equal(t, "no preview of demo is running\n", errOut)

	code, _, errOut = aicoded("preview", "end", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-MAN-006")
}
