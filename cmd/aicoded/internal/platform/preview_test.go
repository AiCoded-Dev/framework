package platform_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

func TestPreviewStart(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	first, second := p.AddPublish("demo"), p.AddPublish("demo")

	pv, err := c.PreviewStart(t.Context(), first)
	require.NoError(t, err)
	assert.Equal(t, "demo", pv.App)
	assert.Equal(t, "demo.acme.preview.aicoded.cloud", pv.Host)
	assert.Equal(t, "https://demo.acme.preview.aicoded.cloud/", pv.URL)
	assert.Equal(t, first, pv.Publish)
	assert.Equal(t, 336*time.Hour, pv.Expires.Sub(pv.Started))
	assert.False(t, pv.Reset)

	p.ResetPreviewData()
	next, err := c.PreviewStart(t.Context(), second)
	require.NoError(t, err)
	assert.Equal(t, platform.Preview{App: "demo", Host: pv.Host, URL: pv.URL, Publish: second, Started: pv.Started, Expires: pv.Expires,
		Reset: true}, next, "a new version keeps the times")
	running, ok := p.Preview("demo")
	assert.True(t, ok)
	assert.Equal(t, second, running)
}

func TestPreviewStartRefused(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.AddApp("payroll", "bo@acme.example")
	demo, payroll := p.AddPublish("demo"), p.AddPublish("payroll")

	_, err := c.PreviewStart(t.Context(), "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Equal(t, "E-PUB-013", errs.Code(err))
	_, err = c.PreviewStart(t.Context(), payroll)
	assert.Equal(t, "E-PUB-005", errs.Code(err))

	p.RefusePreview(http.StatusConflict, "conflict", "E-REL-030", "the permission list declares the setting report_day with no value set for the preview")
	_, err = c.PreviewStart(t.Context(), demo)
	assert.Equal(t, "E-REL-030: the permission list declares the setting report_day with no value set for the preview\n"+
		"  fix: ask the app's owner or a security admin to set the names the message lists for the preview in the console\n"+
		"  docs: https://aicoded.dev/docs/errors/E-REL-030", err.Error())

	p.Answers(platformtest.Failed(1, 10))
	_, err = c.PreviewStart(t.Context(), demo)
	assert.Equal(t, "E-REL-029", errs.Code(err))
	assert.Contains(t, err.Error(), "fix: preview a publish that passed its checks; publish again to get one")
	_, ok := p.Preview("demo")
	assert.False(t, ok)
}

// An answer that is not what the platform gives is refused, so that nothing of it is printed.
func TestPreviewStartChecksTheAnswer(t *testing.T) {
	const id = "pub_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	good := map[string]any{"app": "demo", "host": "demo.acme.preview.aicoded.cloud", "url": "https://demo.acme.preview.aicoded.cloud/",
		"publish": id, "started": "2026-10-30T10:00:00Z", "expires": "2026-11-13T10:00:00Z", "reset": false}
	for name, edit := range map[string]func(map[string]any){
		"another publish": func(a map[string]any) { a["publish"] = "pub_bbbbbbbbbbbbbbbbbbbbbbbbbb" },
		"not an app":      func(a map[string]any) { a["app"] = "Demo\x1b[2J" },
		"not a host":      func(a map[string]any) { a["host"] = "demo.acme\x1b[2J" },
		"http":            func(a map[string]any) { a["url"] = "http://demo.acme.preview.aicoded.cloud/" },
		"another host":    func(a map[string]any) { a["url"] = "https://evil.example/" },
		"a user":          func(a map[string]any) { a["url"] = "https://x@demo.acme.preview.aicoded.cloud/" },
		"no expiry":       func(a map[string]any) { delete(a, "expires") },
	} {
		answer := map[string]any{}
		for k, v := range good {
			answer[k] = v
		}
		edit(answer)
		c := answering(t, http.StatusCreated, answer)
		_, err := c.PreviewStart(t.Context(), id)
		require.EqualError(t, err, "the platform's answer about the preview of "+id+" is not valid", name)
	}
	_, err := answering(t, http.StatusCreated, good).PreviewStart(t.Context(), id)
	assert.NoError(t, err)
}

func TestPreviewEnd(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.AddApp("payroll", "bo@acme.example")
	_, err := c.PreviewStart(t.Context(), p.AddPublish("demo"))
	require.NoError(t, err)

	require.NoError(t, c.PreviewEnd(t.Context(), "demo"))
	_, ok := p.Preview("demo")
	assert.False(t, ok)
	require.EqualError(t, c.PreviewEnd(t.Context(), "demo"), "no preview of demo is running")
	assert.Equal(t, "E-PUB-019", errs.Code(c.PreviewEnd(t.Context(), "nope")))
	assert.Equal(t, "E-PUB-005", errs.Code(c.PreviewEnd(t.Context(), "payroll")))
}

// answering returns a client, signed in, of a platform that answers every request with status
// and the JSON of body.
func answering(t *testing.T, status int, body any) *platform.Client {
	testhome.Set(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, platform.Save(srv.URL, platform.Credentials{Org: "acme", AccessToken: "a", RefreshToken: "r", Expires: time.Now().Add(time.Hour)}))
	return platform.New(srv.URL, nil)
}
