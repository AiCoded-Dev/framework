package e2e

import (
	"crypto/rand"
	"fmt"
	"html"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
)

// deskSecret is the value of desk's secret token in dev.yaml.
const deskSecret = "desk-s3cr3t-value"

// deskDev is testdata/desk running under aicoded dev, and the dev UI's login link it printed.
type deskDev struct {
	ws    *dev.Workspace
	out   *output
	base  string // the dev UI: http://localhost:<port>
	login string
	token string
	port  int
	hc    *http.Client
}

// openDesk copies testdata/desk and opens aicoded dev on it, with desk's secret in dev.yaml,
// printing the login link, and running the apps of manual by hand.
func openDesk(t *testing.T, manual ...string) *deskDev {
	t.Helper()
	requireGo(t)
	root, err := copyApp("desk")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	d := &deskDev{out: &output{}}
	d.ws, err = dev.Open(t.Context(), dev.Config{Root: root, State: t.TempDir(), Out: d.out, Login: true, Manual: manual,
		Dev: devconfig.Workspace{Personas: personas, Apps: map[string]devconfig.AppValues{"desk": {Secrets: map[string]string{"token": deskSecret}}}}})
	require.NoError(t, err)
	t.Cleanup(d.ws.Close)
	st, err := d.ws.Status(t.Context())
	require.NoError(t, err)
	require.Empty(t, st.Apps[0].Problems)
	d.port, err = portOf(st.Gateway)
	require.NoError(t, err)
	d.base = fmt.Sprintf("http://localhost:%d", d.port)
	m := regexp.MustCompile(`aicoded dev: the dev UI is at (\S+/_aicoded/login\?token=(\w+))\n`).FindStringSubmatch(d.out.String())
	require.NotNil(t, m, d.out.String())
	d.login, d.token = m[1], m[2]
	d.hc = newClient(d.port)
	return d
}

// app returns desk's status.
func (d *deskDev) app(t *testing.T) devapi.App {
	t.Helper()
	st, err := d.ws.Status(t.Context())
	require.NoError(t, err)
	return st.Apps[0]
}

// visit gets path of desk as the viewer persona, and returns the status and the body; it
// reports an error instead of failing the test, so it may run inside Eventually.
func (d *deskDev) visit(t *testing.T, path string) (int, string, error) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://desk.localhost:%d%s", d.port, path), nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Cookie", "aicoded_dev_persona=viewer")
	resp, err := d.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

// call gets path of desk, which must answer 200.
func (d *deskDev) call(t *testing.T, path string) {
	t.Helper()
	status, body, err := d.visit(t, path)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)
}

// traffic has desk log a line in a span and send a mail, each holding its secret, and returns
// the mail's id and the line's trace once the dev UI can show both.
func (d *deskDev) traffic(t *testing.T) (mail, trace string) {
	t.Helper()
	msg := "traffic " + rand.Text()
	d.call(t, "/say?msg="+url.QueryEscape(msg))
	d.call(t, "/mail")
	require.Eventually(t, func() bool {
		entries, err := d.ws.Logs(t.Context(), devapi.LogQuery{App: "desk", Contains: msg})
		if err != nil || len(entries) == 0 {
			return false
		}
		trace = entries[0].TraceID
		tr, err := d.ws.Trace(t.Context(), trace)
		return err == nil && slices.ContainsFunc(tr.Spans, func(s devapi.Span) bool { return s.Name == "say" })
	}, 10*time.Second, 50*time.Millisecond)
	sent, err := d.ws.Mail(t.Context(), "desk")
	require.NoError(t, err)
	require.NotEmpty(t, sent)
	return sent[0].ID, trace
}

// logIn opens the login link over HTTP and returns the cookie of the dev UI.
func (d *deskDev) logIn(t *testing.T) *http.Cookie {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.login, nil)
	require.NoError(t, err)
	resp, err := d.hc.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Len(t, resp.Cookies(), 1)
	return resp.Cookies()[0]
}

// localRef is a link or a frame to a path of the dev UI itself.
var localRef = regexp.MustCompile(`(?:href|src)="(/[^/"][^"]*)"`)

// crawl gets, with cookie, every path of the dev UI that a link or a frame leads to from
// /apps, and returns the bodies by path. Every one must answer 200.
func (d *deskDev) crawl(t *testing.T, cookie *http.Cookie) map[string]string {
	t.Helper()
	pages := map[string]string{}
	next := []string{"/apps"}
	for len(next) > 0 {
		path := next[0]
		next = next[1:]
		if _, ok := pages[path]; ok {
			continue
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.base+path, nil)
		require.NoError(t, err)
		req.AddCookie(cookie)
		resp, err := d.hc.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode, path)
		pages[path] = string(body)
		for _, m := range localRef.FindAllStringSubmatch(string(body), -1) {
			next = append(next, html.UnescapeString(m[1]))
		}
	}
	return pages
}

// uiPages are the paths of every page of the dev UI, for desk's mail and trace.
func uiPages(mail, trace string) []string {
	return []string{"/apps", "/apps/desk", "/logs", "/traces", "/traces/" + trace, "/mail", "/mail/desk", "/mail/desk/" + mail, "/personas", "/mcp"}
}

// until waits until the script expr is true. It polls on a timer, which Chrome also runs in a
// tab in the background.
func until(expr string) chromedp.Action {
	return chromedp.Poll(expr, nil, chromedp.WithPollingInterval(50*time.Millisecond))
}

// rowWith is a script that reports whether a row of the one table in the element sel holds text.
func rowWith(sel, text string) string {
	return fmt.Sprintf(`document.querySelectorAll("%s table").length === 1 && `+
		`[...document.querySelectorAll("%s tbody tr")].some(r => r.textContent.includes(%q))`, sel, sel, text)
}

// textIs is a script that reports whether the element sel holds text.
func textIs(sel, text string) string {
	return fmt.Sprintf(`document.querySelector(%q).textContent === %q`, sel, text)
}

func TestDevUI(t *testing.T) {
	d := openDesk(t)

	t.Run("login", func(t *testing.T) {
		ctx := browser(t)
		var location, apps string
		require.NoError(t, chromedp.Run(ctx,
			chromedp.Navigate(d.login),
			chromedp.Location(&location),
			chromedp.Text("#apps tbody", &apps),
		))
		assert.Equal(t, d.base+"/apps", location)
		assert.Contains(t, apps, "desk")
		assert.Contains(t, apps, "running")
		noViolations(t, ctx)
	})

	t.Run("no CSP violations", func(t *testing.T) {
		mail, trace := d.traffic(t)
		ctx := browser(t)
		require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(d.login)))
		for _, path := range uiPages(mail, trace) {
			var title string
			require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(d.base+path), chromedp.Title(&title)))
			assert.Equal(t, "aicoded dev", title, path)
			assert.Empty(t, cspViolations(t, ctx), path)
		}
	})

	t.Run("live log line", func(t *testing.T) {
		ctx := browser(t)
		msg := "live " + rand.Text()
		require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(d.login), chromedp.Navigate(d.base+"/logs?app=desk")))
		d.call(t, "/say?msg="+url.QueryEscape(msg))
		require.NoError(t, chromedp.Run(ctx, until(rowWith("#entries", msg))))
		noViolations(t, ctx)
	})

	t.Run("scripted mail", func(t *testing.T) {
		ctx := browser(t)
		var before int
		require.NoError(t, chromedp.Run(ctx,
			chromedp.Navigate(d.login),
			chromedp.Navigate(d.base+"/mail/desk"),
			chromedp.Evaluate(`document.querySelectorAll("#messages tbody tr").length`, &before),
		))
		d.call(t, "/mail")
		require.NoError(t, chromedp.Run(ctx, until(fmt.Sprintf(
			`document.querySelectorAll("#messages table").length === 1 && document.querySelectorAll("#messages tbody tr").length > %d`, before))))
		_, err := chromedp.RunResponse(ctx, chromedp.Click("#messages tbody tr:first-child a"))
		require.NoError(t, err)
		var text string
		require.NoError(t, chromedp.Run(ctx, chromedp.Text("#text", &text)))
		assert.Equal(t, "The token is [secret token]", text)
		assert.Equal(t, "#00ff00", centre(t, ctx, "iframe[sandbox]"), "the frame shows the mail, and neither its script nor its image's handler ran")
		noViolations(t, ctx)
	})

	t.Run("stop and start", func(t *testing.T) {
		list := browser(t)
		require.NoError(t, chromedp.Run(list, chromedp.Navigate(d.login)))
		detail, cancel := chromedp.NewContext(list)
		t.Cleanup(cancel)
		require.NoError(t, chromedp.Run(detail, chromedp.Navigate(d.base+"/apps/desk")))
		listed := "#apps tbody tr td:nth-child(2)"

		_, err := chromedp.RunResponse(detail, chromedp.Click(`//button[normalize-space()="Stop"]`, chromedp.BySearch))
		require.NoError(t, err)
		require.NoError(t, chromedp.Run(list, until(textIs(listed, "stopped"))))
		require.NoError(t, chromedp.Run(detail, until(textIs("#state", "stopped"))))
		status, body, err := d.visit(t, "/")
		require.NoError(t, err)
		assert.Equal(t, http.StatusServiceUnavailable, status)
		assert.Contains(t, body, fmt.Sprintf(`desk is stopped. Start it on the dev UI at <a href="%s/apps/desk">`, d.base))

		_, err = chromedp.RunResponse(detail, chromedp.Click(`//button[normalize-space()="Start"]`, chromedp.BySearch))
		require.NoError(t, err)
		require.NoError(t, chromedp.Run(detail, until(textIs("#state", "running"))))
		require.NoError(t, chromedp.Run(list, until(textIs(listed, "running"))))
		d.call(t, "/")
		noViolations(t, list)
	})

	t.Run("no secrets", func(t *testing.T) {
		mail, trace := d.traffic(t)
		pages := d.crawl(t, d.logIn(t))
		for path, body := range pages {
			assert.NotContains(t, body, deskSecret, path)
			assert.NotContains(t, body, d.token, path)
		}
		assert.Subset(t, slices.Collect(maps.Keys(pages)), append(uiPages(mail, trace), "/_aicoded/mail/desk/"+mail))
		assert.Contains(t, pages["/logs"], "[secret token]")
		assert.Contains(t, pages["/traces/"+trace], "token=[secret token]")
		assert.Contains(t, pages["/mail/desk/"+mail], "The token is [secret token]")
		assert.Contains(t, pages["/_aicoded/mail/desk/"+mail], "<!-- [secret token] -->")
	})
}
