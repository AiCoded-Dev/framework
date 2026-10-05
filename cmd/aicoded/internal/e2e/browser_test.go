package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findChrome() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

type cspKey struct{}

// violations are the Content-Security-Policy violations a tab has seen.
type violations struct {
	mu   sync.Mutex
	seen []string
}

// chromeRequired reads the value of AICODED_E2E_BROWSER: "required" makes the browser tests fail
// without Chrome, and "" lets them skip. Any other value, such as a typo in CI, is an error,
// because it would turn the browser tests into skips unnoticed.
func chromeRequired(v string) (bool, error) {
	switch v {
	case "":
		return false, nil
	case "required":
		return true, nil
	}
	return false, fmt.Errorf("AICODED_E2E_BROWSER=%q: set it to required or leave it unset", v)
}

func TestChromeRequired(t *testing.T) {
	for v, want := range map[string]bool{"": false, "required": true} {
		got, err := chromeRequired(v)
		require.NoError(t, err, v)
		assert.Equal(t, want, got, v)
	}
	for _, v := range []string{"require", "REQUIRED", "1", "true", " required"} {
		_, err := chromeRequired(v)
		assert.Error(t, err, v)
	}
}

// browser returns a headless Chrome tab that records the Content-Security-Policy violations of
// every page it opens.
func browser(t *testing.T) context.Context {
	t.Helper()
	required, err := chromeRequired(os.Getenv("AICODED_E2E_BROWSER"))
	require.NoError(t, err)
	path := findChrome()
	if path == "" {
		if required {
			t.Fatal("Chrome is required but was not found")
		}
		t.Skip("Chrome not found")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path))
	actx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(cancelAlloc)
	ctx, cancelTab := chromedp.NewContext(actx)
	t.Cleanup(cancelTab)
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	t.Cleanup(cancel)
	v := &violations{}
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventBindingCalled); ok && e.Name == "reportCSP" {
			v.mu.Lock()
			v.seen = append(v.seen, e.Payload)
			v.mu.Unlock()
		}
	})
	require.NoError(t, chromedp.Run(ctx, runtime.AddBinding("reportCSP"), chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(
			`document.addEventListener("securitypolicyviolation", e => reportCSP(e.violatedDirective + " " + e.blockedURI));`).Do(ctx)
		return err
	})))
	return context.WithValue(ctx, cspKey{}, v)
}

func switchPersona(t *testing.T, ctx context.Context, s *site, name string) {
	t.Helper()
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/_aicoded/persona")),
		chromedp.Click(fmt.Sprintf(`//button[normalize-space()=%q]`, name), chromedp.BySearch),
		chromedp.WaitVisible("#viewer"),
	))
}

// cspViolations returns what the tab has recorded once the current page has handled its events.
func cspViolations(t *testing.T, ctx context.Context) []string {
	t.Helper()
	require.NoError(t, chromedp.Run(ctx, chromedp.Evaluate(`new Promise(r => setTimeout(r))`, nil, awaitPromise)))
	v := ctx.Value(cspKey{}).(*violations)
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.Clone(v.seen)
}

func awaitPromise(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }

func noViolations(t *testing.T, ctx context.Context) {
	t.Helper()
	assert.Empty(t, cspViolations(t, ctx))
}

// Chrome enforces the site's policy, and the recorder that noViolations reads sees it do so.
func TestBrowserBlocksInlineScript(t *testing.T) {
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "viewer")
	var title string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Evaluate(`const s = document.createElement("script");
s.textContent = "document.title = 'pwned'";
document.head.append(s);`, nil),
		chromedp.Title(&title),
	))
	assert.Equal(t, "Site", title)
	assert.Equal(t, []string{"script-src-elem inline"}, cspViolations(t, ctx))
}

// The dev gateway's persona cookie works in Chrome over http on *.localhost.
func TestBrowserPersonaSwitch(t *testing.T) {
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "editor")
	var who string
	require.NoError(t, chromedp.Run(ctx, chromedp.Text("#viewer", &who)))
	assert.Equal(t, "editor", who)
	noViolations(t, ctx)
}

// A form posts and redirects in Chrome, and the note page's script calls the server over the
// live connection.
func TestBrowserFormAndCall(t *testing.T) {
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "editor")
	var title string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/notes")),
		chromedp.SendKeys(`input[name="title"]`, "From Chrome"),
		chromedp.SetValue(`select[name="priority"]`, "2"),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#title"),
		chromedp.Text("#title", &title),
		chromedp.Click("#star"),
		chromedp.Poll(`document.querySelector("#star-result").textContent === "starred From Chrome"`, nil),
	))
	assert.Equal(t, "From Chrome", title)
	noViolations(t, ctx)
}

// A call that the page's guard refuses rejects in the page script with its status.
func TestBrowserCallRefused(t *testing.T) {
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "editor")
	note := addNote(t, s, "editor", "Given from Chrome")
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url(note)),
		chromedp.Click("#star"),
		chromedp.Poll(`document.querySelector("#star-result").textContent === "starred Given from Chrome"`, nil),
	))

	_, body := s.get(t, "editor", note)
	resp, _ := s.post(t, "editor", note, url.Values{
		"_aicoded_form": {routeKey("/notes/n_id") + ".give"},
		"_aicoded_csrf": {field(t, body, "_aicoded_csrf")},
		"to":            {"editor2"},
	})
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	require.NoError(t, chromedp.Run(ctx,
		chromedp.Click("#star"),
		chromedp.Poll(`document.querySelector("#star-result").textContent === "failed 404"`, nil),
	))
	noViolations(t, ctx)
}

// Defect N1: a live value never becomes markup in the browser. A bound input shows the
// server's value once the page is live.
func TestBrowserLiveValuesStayText(t *testing.T) {
	const probe = `<img src=x onerror="document.title='pwned'">`
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "admin")
	var first, input, message, title string
	var elements int
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/admin/live")),
		chromedp.WaitVisible("#echo-list li"),
		chromedp.Text("#message", &first),
		chromedp.Value("#message-input", &input),
		chromedp.Evaluate(`document.querySelector("#message-input").value = ""`, nil),
		chromedp.SendKeys("#message-input", "<b>bold</b>"),
		chromedp.Poll(`document.querySelector("#message").textContent === "<b>bold</b>"`, nil),
		chromedp.Poll(`document.querySelector("#echo-list li").textContent === "<b>bold</b>"`, nil),
		chromedp.Text("#message", &message),
		chromedp.Evaluate(`document.querySelectorAll("#message img, #message b, #echo-list img, #echo-list b").length`, &elements),
		chromedp.Title(&title),
	))
	assert.Equal(t, probe, first)
	assert.Equal(t, probe, input)
	assert.Equal(t, "<b>bold</b>", message)
	assert.Zero(t, elements)
	assert.Equal(t, "Site", title)
	noViolations(t, ctx)
}

// One bound element or page listener that throws does not keep the others from getting their
// values: a file input throws when a script sets its value.
func TestBrowserInitSurvivesThrows(t *testing.T) {
	const probe = `<img src=x onerror="document.title='pwned'">`
	s := startSite(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "admin")
	var message, seen string
	require.NoError(t, chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(`document.addEventListener("DOMContentLoaded", () => {
  document.querySelector("#message-input").type = "file";
  document.querySelector("#message span").textContent = "stale";
  const live = aicodedReactive.route("` + routeKey("/admin/live") + `");
  live.on("message", () => { throw new Error("listener failed"); });
  live.on("message", (v) => { document.body.dataset.seen = v; });
});`).Do(ctx)
			return err
		}),
		chromedp.Navigate(s.url("/admin/live")),
		chromedp.WaitVisible("#echo-list li"),
		chromedp.Text("#message", &message),
		chromedp.Evaluate(`document.body.dataset.seen ?? ""`, &seen),
	))
	assert.Equal(t, probe, message)
	assert.Equal(t, probe, seen)
	noViolations(t, ctx)
}
