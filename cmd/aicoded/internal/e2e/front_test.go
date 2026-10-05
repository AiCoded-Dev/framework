package e2e

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/fake"
)

// frontToken is the access token of the dev UI that serveFront serves.
var frontToken = strings.Repeat("ab", 32)

// scriptedMail is a mail's HTML part that is green, unless its script or its image's error
// handler runs and turns it red.
const scriptedMail = `<body style="margin:0;background:#00ff00"><script>document.body.style.background = "#ff0000"</script>` +
	`<img src="x" onerror="document.body.style.background = '#ff0000'"></body>`

// serveFront serves the dev UI over a fake workspace at http://localhost:<port>, and returns that
// address. At /frame it also serves a page of the same origin and policy as the pages, which
// frames shop's mail sent-1 as the mail page does, and next to it the same HTML, without a
// policy, in a frame that may run scripts.
func serveFront(t *testing.T) string {
	b := &fake.Backend{Data: fake.Data{
		Status: devapi.Status{Apps: []devapi.App{{Name: "shop", State: devapi.Running}}},
		Mail:   map[string][]devapi.Mail{"shop": {{MailSummary: devapi.MailSummary{ID: "sent-1"}, HTML: scriptedMail}}},
	}}
	front := devui.New(b, frontToken, "/opt/aicoded", io.Discard)
	l, err := listenLocal()
	require.NoError(t, err)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/frame":
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; frame-ancestors 'none'")
			fmt.Fprint(w, `<!doctype html><title>frame</title>`+
				`<iframe id="mail" sandbox src="/_aicoded/mail/shop/sent-1"></iframe>`+
				`<iframe id="control" sandbox="allow-scripts" src="/control"></iframe>`)
		case "/control":
			fmt.Fprint(w, scriptedMail)
		default:
			front.ServeHTTP(w, r)
		}
	})}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return fmt.Sprintf("http://localhost:%d", l.Addr().(*net.TCPAddr).Port)
}

// centre returns the colour in the middle of the element sel, or of the page when sel is "",
// as #rrggbb.
func centre(t *testing.T, ctx context.Context, sel string) string {
	t.Helper()
	var shot []byte
	if sel == "" {
		require.NoError(t, chromedp.Run(ctx, chromedp.CaptureScreenshot(&shot)))
	} else {
		require.NoError(t, chromedp.Run(ctx, chromedp.Screenshot(sel, &shot)))
	}
	img, err := png.Decode(bytes.NewReader(shot))
	require.NoError(t, err)
	r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// The login cookie of the dev UI works in Chrome over http on localhost, and the login's
// redirects carry it to the apps page.
func TestBrowserDevUILogin(t *testing.T) {
	base := serveFront(t)
	ctx := browser(t)
	var location, title string
	var cookies []*network.Cookie
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(base+devui.LoginPath+"?token="+frontToken),
		chromedp.Location(&location),
		chromedp.Title(&title),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{base + "/"}).Do(ctx)
			return err
		}),
	))
	assert.Equal(t, base+"/apps", location)
	assert.Equal(t, "aicoded dev", title)
	require.Len(t, cookies, 1)
	assert.Equal(t, "__Host-aicoded_dev_ui", cookies[0].Name)
	assert.True(t, cookies[0].HTTPOnly)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, network.CookieSameSiteStrict, cookies[0].SameSite)
	noViolations(t, ctx)
}

// A caught mail's HTML shows in a sandboxed frame of the dev UI, and its scripts do not run.
func TestBrowserDevUIMailFrame(t *testing.T) {
	base := serveFront(t)
	ctx := browser(t)
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(base+devui.LoginPath+"?token="+frontToken),
		chromedp.Navigate(base+"/frame"),
	))
	assert.Equal(t, "#00ff00", centre(t, ctx, "#mail"), "the frame shows the mail, and its scripts did not run")
	assert.Equal(t, "#ff0000", centre(t, ctx, "#control"), "the same HTML runs its scripts where it may")

	require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(base+"/_aicoded/mail/shop/sent-1")))
	assert.Equal(t, "#00ff00", centre(t, ctx, ""), "opened by itself, the policy alone keeps its scripts from running")
}
