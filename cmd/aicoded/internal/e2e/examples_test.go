package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/testmysql"
)

// teamAddress is people's setting team_address, where it mails the team.
const teamAddress = "team@acme.example"

// examplePersonas are the personas of examples/README.md.
var examplePersonas = []devconfig.Persona{
	{Name: "alice", Roles: []string{"staff"}},
	{Name: "bob", Roles: []string{"staff", "hr"}},
	{Name: "carol"},
	{Name: "dana", Roles: []string{"staff", "hotel-ops"}},
}

// exampleApps are the apps in examples/.
var exampleApps = []string{"contacts", "people", "room-maintenance"}

var (
	examplesOnce sync.Once
	examples     *site
	examplesErr  error
)

// startExamples runs examples/, exampleApps, once per test binary on the MySQL server of
// AICODED_TEST_MYSQL_DSN, after dropping what a killed run may have left there. Its site is
// people. It drops the database and the MySQL user ac-dev-<app> of each app, so the DSN must name
// a throwaway server, not the one the developer's own dev.yaml uses.
func startExamples(t *testing.T) *site {
	t.Helper()
	requireGo(t)
	dsn := testmysql.DSN(t)
	examplesOnce.Do(func() {
		dropDatabases(dsn, exampleApps...)
		examples, examplesErr = launch(filepath.Join("..", "..", "..", "..", "examples"), devconfig.Workspace{
			MySQL:    dsn,
			Personas: examplePersonas,
			Apps:     map[string]devconfig.AppValues{"people": {Settings: map[string]string{"team_address": teamAddress}}},
		})
		if examplesErr != nil {
			dropDatabases(dsn, exampleApps...)
			return
		}
		examples.onStop = func() { dropDatabases(dsn, exampleApps...) }
	})
	require.NoError(t, examplesErr)
	return examples
}

// newLogin returns a login no user has.
func newLogin() string { return "u" + strings.ToLower(rand.Text()[:12]) }

// pngData returns a PNG of one pixel, which the add form takes as a photo.
func pngData(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1))))
	return b.Bytes()
}

// addUser posts people's add form as bob: the user login, with name, bio and a photo named photo,
// aged 30, in the team IT, on site, on the day shift.
func addUser(t *testing.T, s *site, login, name, bio, photo string) (*response, string) {
	t.Helper()
	_, form := s.get(t, "bob", "/users/add")
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, kv := range [][2]string{
		{"_aicoded_form", field(t, form, "_aicoded_form")}, {"_aicoded_csrf", field(t, form, "_aicoded_csrf")},
		{"login", login}, {"name", name}, {"age", "30"}, {"team", "IT"}, {"remote", "false"}, {"shift", "1"}, {"bio", bio},
	} {
		require.NoError(t, w.WriteField(kv[0], kv[1]))
	}
	f, err := w.CreateFormFile("photo", photo)
	require.NoError(t, err)
	_, err = f.Write(pngData(t))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url("/users/add"), &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Origin", s.url(""))
	return s.do(t, "bob", req)
}

// mail returns the mail people sent with subject whose text holds text.
func (s *site) mail(t *testing.T, subject, text string) []devapi.Mail {
	t.Helper()
	sent, err := s.ws.Mail(t.Context(), s.app)
	require.NoError(t, err)
	var out []devapi.Mail
	for _, m := range sent {
		if m.Folder != "sent" || m.Subject != subject {
			continue
		}
		full, err := s.ws.MailGet(t.Context(), s.app, m.ID)
		require.NoError(t, err)
		if strings.Contains(full.Text, text) {
			out = append(out, full)
		}
	}
	return out
}

// socketCounter counts, in window.sockets, the live connections the pages of a tab open and how
// many of them are open.
const socketCounter = `window.sockets = {made: 0, open: 0};
const Native = window.WebSocket;
window.WebSocket = class extends Native {
  constructor(...args) {
    super(...args);
    window.sockets.made++;
    this.addEventListener("open", () => window.sockets.open++);
    this.addEventListener("close", () => window.sockets.open--);
  }
};`

// countSockets installs socketCounter in every page the tab opens from now on.
func countSockets() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(socketCounter).Do(ctx)
		return err
	})
}

// live is a script that reports whether the page's live connection is open.
const live = `window.sockets.open === 1`

// The home page renders the template showcase on the server, and its script reads the data
// the page passes to it.
func TestExampleHome(t *testing.T) {
	s := startExamples(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "carol")
	var title string
	var usersLink bool
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/home")),
		chromedp.Title(&title),
		chromedp.Evaluate(`document.querySelector("#nav-users") !== null`, &usersLink),
		chromedp.WaitVisible(`nav a.active[href="/home"]`),
	))
	assert.Equal(t, "People", title)
	assert.False(t, usersLink, "the Users link shows only to staff")
	for sel, want := range map[string]string{
		"#viewer":      "carol",
		"#intro":       "You are signed in as carol.",
		"#price":       "9.99",
		"#quantity":    "3",
		"#total":       "29.97",
		"#age-check":   "an adult",
		"#status":      "Active",
		"#fruits":      "Apple\nBanana\nCherry",
		"#langs":       "#0: Go\n#1: TypeScript\n#2: Rust",
		"#script-note": "A script read 3 languages from the page.",
	} {
		var got string
		require.NoError(t, chromedp.Run(ctx, chromedp.Text(sel, &got)))
		assert.Equal(t, want, got, sel)
	}
	noViolations(t, ctx)
}

// visitors is a script that reports whether the home page shows n open home pages, in a badge
// of the class tone.
func visitors(n int, tone string) string {
	return fmt.Sprintf(`document.querySelector("#visitors").textContent === "%d" && `+
		`document.querySelector("#visitors").classList.contains(%q)`, n, tone)
}

// The number of open home pages and its badge's class change live as a second home page opens
// and closes.
func TestExampleVisitorsOnline(t *testing.T) {
	s := startExamples(t)
	first := browser(t)
	switchPersona(t, first, s, "alice")
	require.NoError(t, chromedp.Run(first, chromedp.Navigate(s.url("/home")), until(visitors(1, "alone"))))

	second, closeSecond := chromedp.NewContext(first)
	t.Cleanup(closeSecond)
	require.NoError(t, chromedp.Run(second, chromedp.Navigate(s.url("/home")), until(visitors(2, "few"))))
	require.NoError(t, chromedp.Run(first, until(visitors(2, "few"))))

	closeSecond()
	require.NoError(t, chromedp.Run(first, until(visitors(1, "alone"))))
	noViolations(t, first)
}

// The display name goes to the server as the viewer types and comes back in the greeting. A
// name over 50 characters shows the server's error, and the greeting keeps the last good name.
func TestExampleDisplayName(t *testing.T) {
	s := startExamples(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "carol")
	var greeting string
	require.NoError(t, chromedp.Run(ctx,
		countSockets(),
		chromedp.Navigate(s.url("/home")),
		until(live),
		chromedp.SendKeys("#display-name", "Ana"),
		until(textIs("#greeting", "Hi, Ana!")),
		chromedp.Evaluate(`const input = document.querySelector("#display-name");
input.value = "x".repeat(51);
input.dispatchEvent(new Event("input", {bubbles: true}));`, nil),
		until(textIs("#display-name-error", "Use at most 50 characters.")),
		chromedp.Text("#greeting", &greeting),
	))
	assert.Equal(t, "Hi, Ana!", greeting)
	noViolations(t, ctx)
}

// The contact form shows the server's errors for empty fields, then mails the team once for a
// message sent twice.
func TestExampleContactForm(t *testing.T) {
	s := startExamples(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "carol")
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/contact")),
		chromedp.Evaluate(`document.querySelector("form").noValidate = true`, nil),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#contact-message-error"),
	))
	for _, sel := range []string{"#contact-name-error", "#contact-email-error", "#contact-message-error"} {
		var got string
		require.NoError(t, chromedp.Run(ctx, chromedp.Text(sel, &got)))
		assert.Equal(t, "This field is required", got, sel)
	}

	message := "Please call me back about " + rand.Text()
	send := chromedp.Tasks{
		chromedp.Navigate(s.url("/contact")),
		chromedp.SendKeys("#contact-name", "Carol"),
		chromedp.SendKeys("#contact-email", "carol@example.com"),
		chromedp.SetValue("#contact-topic", "Billing"),
		chromedp.SendKeys("#contact-message", message),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#sent"),
	}
	var location string
	require.NoError(t, chromedp.Run(ctx, send, chromedp.Location(&location), send))
	assert.Equal(t, s.url("/contact?sent=1"), location)
	mail := s.mail(t, "Contact form: Billing", message)
	require.Len(t, mail, 1, "the second send has the first one's idempotency key")
	assert.Equal(t, []string{teamAddress}, mail[0].To)
	assert.Equal(t, "people@acme.example", mail[0].From)
	noViolations(t, ctx)
}

// Each page admits only the roles its access rule names, and only hr is offered the add form.
func TestExampleAccess(t *testing.T) {
	s := startExamples(t)
	for _, c := range []struct {
		persona, path string
		status        int
	}{
		{"carol", "/home", http.StatusOK},
		{"carol", "/contact", http.StatusOK},
		{"carol", "/users", http.StatusForbidden},
		{"carol", "/users/alice/info", http.StatusForbidden},
		{"alice", "/users", http.StatusFound},
		{"alice", "/users/johndoe123/info", http.StatusOK},
		{"alice", "/users/add", http.StatusForbidden},
		{"bob", "/users/add", http.StatusOK},
	} {
		resp, _ := s.get(t, c.persona, c.path)
		assert.Equal(t, c.status, resp.StatusCode, "%+v", c)
	}
	_, list := s.get(t, "alice", "/users/johndoe123/info")
	assert.Contains(t, list, `<a class="active" href="/users/johndoe123">John Doe</a>`)
	assert.NotContains(t, list, `id="add-user"`)
	_, list = s.get(t, "bob", "/users/johndoe123/info")
	assert.Contains(t, list, `id="add-user"`)
}

// bob adds a user with a photo in Chrome. The card shows the photo, an open users page counts the
// new user live, the team gets one mail, and the user's contacts tab is 404, since contacts has
// none for them.
func TestExampleAddUser(t *testing.T) {
	s := startExamples(t)
	list := browser(t)
	switchPersona(t, list, s, "bob")
	var count string
	require.NoError(t, chromedp.Run(list, chromedp.Navigate(s.url("/users")), chromedp.Text("#user-count", &count)))
	n, err := strconv.Atoi(count)
	require.NoError(t, err)

	data := pngData(t)
	photo := filepath.Join(t.TempDir(), "ana.png")
	require.NoError(t, os.WriteFile(photo, data, 0o600))
	login := newLogin()
	form, closeForm := chromedp.NewContext(list)
	t.Cleanup(closeForm)
	var location string
	require.NoError(t, chromedp.Run(form,
		chromedp.Navigate(s.url("/users/add")),
		chromedp.SendKeys("#login", login),
		chromedp.SetValue("#name", "Ana Lima"),
		chromedp.SendKeys("#age", "41"),
		chromedp.SetValue("#team", "IT"),
		chromedp.Evaluate(`for (const o of document.querySelector("#languages").options) o.selected = o.value === "English" || o.value === "Portuguese";`, nil),
		chromedp.Click("#remote"),
		chromedp.Click("#night"),
		chromedp.SendKeys("#bio", "Ana looks after the hotel's network."),
		chromedp.SetUploadFiles("#photo", []string{photo}),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#user-name"),
		chromedp.Location(&location),
	))
	assert.Equal(t, s.url("/users/"+login+"/info"), location)
	for sel, want := range map[string]string{
		"#user-name":  "Ana Lima",
		"#avatar":     "AL",
		"#user-login": login,
		"#user-age":   "41",
		"li.photo":    fmt.Sprintf("Photo: ana.png, %d bytes", len(data)),
		"#team":       "IT",
		"#languages":  "English, Portuguese",
		"#works":      "remotely",
		"#shift":      "night",
		"#bio":        "Ana looks after the hotel's network.",
	} {
		var got string
		require.NoError(t, chromedp.Run(form, chromedp.Text(sel, &got)))
		assert.Equal(t, want, got, sel)
	}

	require.NoError(t, chromedp.Run(list, until(textIs("#user-count", strconv.Itoa(n+1)))))
	mail := s.mail(t, "User "+login+" added", "")
	require.Len(t, mail, 1)
	assert.Equal(t, []string{teamAddress}, mail[0].To)
	resp, _ := s.get(t, "bob", "/users/"+login+"/contacts")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "contacts answers NotFound for a login it has no contacts of")
	noViolations(t, list)
}

// The contacts tab shows what the contacts app serves: a viewer's own contacts, and anyone's to
// hr. alice may not read bob's, nor those of a login people does not know, which is 404 to bob.
func TestExampleContacts(t *testing.T) {
	s := startExamples(t)
	for _, c := range []struct {
		persona, login string
		phones, emails string
	}{
		{"alice", "alice", "<li>+3510000001</li>", "<li>alice@example.com</li>"},
		{"bob", "alice", "<li>+3510000001</li>", "<li>alice@example.com</li>"},
		{"bob", "bob", "<li>+3510000002</li>", "<li>bob@example.com</li>"},
		{"bob", "johndoe123", "<li>+1234567890</li><li>+0987654321</li>", "<li>johndoe@example.com</li><li>john.d@example.com</li>"},
	} {
		resp, body := s.get(t, c.persona, "/users/"+c.login+"/contacts")
		require.Equal(t, http.StatusOK, resp.StatusCode, "%+v", c)
		assert.Contains(t, body, `<ul id="phones">`+c.phones+`</ul>`, "%+v", c)
		assert.Contains(t, body, `<ul id="emails">`+c.emails+`</ul>`, "%+v", c)
	}
	resp, _ := s.get(t, "alice", "/users/bob/contacts")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = s.get(t, "alice", "/users/nosuchuser/contacts")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a refused viewer cannot tell whether a login exists")
	resp, _ = s.get(t, "bob", "/users/nosuchuser/contacts")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// One live connection carries the live values of every page of the address: on a user's info
// page, the users layout's userCount and the info page's lastSeen.
func TestExampleLiveValuesShareAConnection(t *testing.T) {
	s := startExamples(t)
	ctx := browser(t)
	switchPersona(t, ctx, s, "bob")
	var count, seen string
	require.NoError(t, chromedp.Run(ctx,
		countSockets(),
		chromedp.Navigate(s.url("/users/alice/info")),
		until(live),
		chromedp.Text("#user-count", &count),
		chromedp.Text("#last-seen", &seen),
	))
	n, err := strconv.Atoi(count)
	require.NoError(t, err)

	time.Sleep(time.Second) // lastSeen shows whole seconds
	s.get(t, "alice", "/home")
	resp, _ := addUser(t, s, newLogin(), "Rui Costa", "Rui runs the night shift.", "rui.png")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	var made int
	require.NoError(t, chromedp.Run(ctx,
		until(fmt.Sprintf(`document.querySelector("#last-seen").textContent !== %q`, seen)),
		until(textIs("#user-count", strconv.Itoa(n+1))),
		chromedp.Text("#last-seen", &seen),
		chromedp.Evaluate(`window.sockets.made`, &made),
	))
	assert.Regexp(t, `^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d UTC$`, seen)
	assert.Equal(t, 1, made)
	noViolations(t, ctx)
}

// No page of people, the error pages included, breaks its Content-Security-Policy in Chrome.
func TestExampleCSP(t *testing.T) {
	s := startExamples(t)
	ctx := browser(t)
	for _, v := range []struct {
		persona string
		paths   []string
	}{
		{"bob", []string{"/home", "/contact", "/contact?sent=1", "/users/johndoe123/info", "/users/bob/contacts", "/users/add", "/users/nosuchuser"}},
		{"alice", []string{"/users/bob/contacts"}},
	} {
		switchPersona(t, ctx, s, v.persona)
		for _, path := range v.paths {
			require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(s.url(path))))
			assert.Empty(t, cspViolations(t, ctx), path)
		}
	}
}

// writeLive writes value to the client-writable live value name of the page at path over c.
func writeLive(t *testing.T, c *websocket.Conn, path, name, value string) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"t": "write", "routeKey": routeKey(path), "var": name, "value": value})
	require.NoError(t, err)
	require.NoError(t, c.Write(t.Context(), websocket.MessageText, b))
}

// spansSince returns the spans of both apps of every trace that started after since.
func (s *site) spansSince(ctx context.Context, since time.Time) ([]devapi.Span, error) {
	traces, err := s.ws.Traces(ctx, devapi.TraceQuery{Limit: 200})
	if err != nil {
		return nil, err
	}
	if len(traces) == 200 && traces[len(traces)-1].Start.After(since) {
		return nil, fmt.Errorf("more than 200 traces since %s", since)
	}
	var spans []devapi.Span
	for _, sum := range traces {
		if sum.Start.Before(since) {
			continue
		}
		tr, err := s.ws.Trace(ctx, sum.TraceID)
		if err != nil {
			return nil, err
		}
		spans = append(spans, tr.Spans...)
	}
	return spans, nil
}

// Defect 8: nothing a viewer types reaches a log or a span. A marker goes into every text field
// of every form of people and into the live display name, on posts the app takes and on posts
// it refuses. It shows in no line either app printed, no log entry and no span of either app.
// Every refusal here is an answer the app chose, such as a field's error or a 404, and nothing
// fails on the server, where the framework would log the error's text under aicoded dev.
func TestExampleFormValuesStayOutOfLogs(t *testing.T) {
	s := startExamples(t)
	since := time.Now()
	marker := "pii" + strings.ToLower(rand.Text()[:13])

	c, _, err := s.dialLive(t, "carol", "/home/__ws")
	require.NoError(t, err)
	next(t, c, func(f frame) bool { return f.T == "init" })
	writeLive(t, c, "/home", "displayName", marker)
	next(t, c, func(f frame) bool { return f.T == "ack" })
	writeLive(t, c, "/home", "displayName", marker+strings.Repeat("x", 50))
	next(t, c, func(f frame) bool { return f.T == "err" })
	require.NoError(t, c.CloseNow(), "the connection's span ends when it closes")

	_, form := s.get(t, "carol", "/contact")
	contact := func(email string) (*response, string) {
		return s.post(t, "carol", "/contact", url.Values{
			"_aicoded_form": {field(t, form, "_aicoded_form")},
			"_aicoded_csrf": {field(t, form, "_aicoded_csrf")},
			"name":          {marker},
			"email":         {email},
			"topic":         {"Billing"},
			"message":       {marker},
		})
	}
	resp, body := contact(marker)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Enter an email address.", "the app refused the address")
	resp, _ = contact(marker + "@example.com")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Len(t, s.mail(t, "Contact form: Billing", marker), 1, "the message was sent")

	resp, body = addUser(t, s, marker+"!", marker, marker, marker+".png")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Use 1 to 32 lowercase letters and digits.", "the app refused the login")
	login := newLogin()
	resp, _ = addUser(t, s, login, marker, marker, marker+".png")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	_, body = s.get(t, "bob", "/users/"+login+"/info")
	require.Contains(t, body, marker, "the user was added with the marker")
	tab := "/users/" + login + "/contacts"
	resp, _ = s.get(t, "bob", tab)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	// The dev runner appends log entries and printed lines as it reads an app's pipe, while the
	// request is served; the request's span arrives later, when it ends. So once the spans of the
	// live connection and of the last request are in, so is everything the apps logged.
	require.Eventually(t, func() bool {
		spans, err := s.spansSince(t.Context(), since)
		return err == nil &&
			slices.ContainsFunc(spans, func(sp devapi.Span) bool { return sp.App == "contacts" }) &&
			slices.ContainsFunc(spans, func(sp devapi.Span) bool { return sp.Attrs["http.path"] == "/home/__ws" }) &&
			slices.ContainsFunc(spans, func(sp devapi.Span) bool { return sp.Attrs["http.path"] == tab })
	}, 10*time.Second, 100*time.Millisecond, "the spans of both apps arrive")
	spans, err := s.spansSince(t.Context(), since)
	require.NoError(t, err)
	for _, sp := range spans {
		b, err := json.Marshal(sp)
		require.NoError(t, err)
		assert.NotContains(t, string(b), marker, "a span of %s", sp.App)
	}
	entries, err := s.ws.Logs(t.Context(), devapi.LogQuery{Limit: 1000})
	require.NoError(t, err)
	require.True(t, len(entries) < 1000 || entries[0].Time.Before(since), "every log entry since the test began was read")
	for _, e := range entries {
		b, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(b), marker, "a log entry of %s", e.App)
	}
	for line := range strings.Lines(s.out.String()) {
		assert.NotContains(t, line, marker)
	}
}
