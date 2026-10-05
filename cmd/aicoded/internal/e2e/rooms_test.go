package e2e

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
)

// startRooms returns the site of room-maintenance, which startExamples runs with the others.
func startRooms(t *testing.T) *site {
	t.Helper()
	s := *startExamples(t)
	s.app, s.onStop = "room-maintenance", nil
	return &s
}

// report posts room-maintenance's report form as persona and returns the answer.
func report(t *testing.T, s *site, persona, room, title, details string) (*response, string) {
	t.Helper()
	return postForm(t, s, persona, "/tickets/report", "/tickets/report", "report",
		url.Values{"room": {room}, "title": {title}, "details": {details}})
}

// newTicket reports a problem in room 101 as persona and returns the path of its page.
func newTicket(t *testing.T, s *site, persona, title string) string {
	t.Helper()
	resp, _ := report(t, s, persona, "101", title, "Seen this morning.")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	path := resp.Header.Get("Location")
	require.Regexp(t, `^/tickets/\d+$`, path)
	return path
}

// postForm posts the form name of the page at path as persona, with a form token the persona got
// on the report page, so that only the page's own checks can refuse it.
func postForm(t *testing.T, s *site, persona, path, page, name string, values url.Values) (*response, string) {
	t.Helper()
	_, form := s.get(t, persona, "/tickets/report")
	values.Set("_aicoded_form", routeKey(page)+"."+name)
	values.Set("_aicoded_csrf", field(t, form, "_aicoded_csrf"))
	return s.post(t, persona, path, values)
}

// editTicket posts the edit form of the ticket at path as persona.
func editTicket(t *testing.T, s *site, persona, path, title, details string) (*response, string) {
	t.Helper()
	return postForm(t, s, persona, path+"/edit", "/tickets/n_id/edit", "edit", url.Values{"title": {title}, "details": {details}})
}

// setStatus posts the status form of the ticket at path as persona.
func setStatus(t *testing.T, s *site, persona, path, status string) (*response, string) {
	t.Helper()
	return postForm(t, s, persona, path+"/status", "/tickets/n_id/status", "status", url.Values{"status": {status}})
}

// The IDOR case: an employee can neither open nor change another's ticket, by any page or form,
// and cannot tell which tickets exist. Only the author edits an open ticket, only hotel-ops change
// its status, and a viewer without staff opens nothing. The pages' own checks refuse every post,
// never the form token.
func TestRoomsOwnership(t *testing.T) {
	s := startRooms(t)
	const badToken = "form refused: missing or invalid form token"
	badTokens := s.logged(t, badToken)
	mine := newTicket(t, s, "alice", "Leaking tap")
	theirs := newTicket(t, s, "bob", "Broken lamp")
	missing := theirs + "000"

	for _, c := range []struct {
		persona, path string
		status        int
	}{
		{"alice", theirs, http.StatusForbidden},
		{"alice", theirs + "/edit", http.StatusForbidden},
		{"alice", theirs + "/status", http.StatusForbidden},
		{"alice", missing, http.StatusForbidden},
		{"alice", mine, http.StatusOK},
		{"alice", mine + "/edit", http.StatusOK},
		{"alice", mine + "/status", http.StatusForbidden},
		{"dana", theirs, http.StatusOK},
		{"dana", theirs + "/edit", http.StatusForbidden},
		{"dana", theirs + "/status", http.StatusOK},
		{"dana", missing, http.StatusNotFound},
		{"carol", "/", http.StatusForbidden},
		{"carol", "/tickets", http.StatusForbidden},
		{"carol", "/tickets/report", http.StatusForbidden},
		{"carol", mine, http.StatusForbidden},
	} {
		resp, _ := s.get(t, c.persona, c.path)
		assert.Equal(t, c.status, resp.StatusCode, "%+v", c)
	}

	_, forbidden := s.get(t, "alice", theirs)
	_, gone := s.get(t, "alice", missing)
	assert.Equal(t, forbidden, gone, "alice cannot tell another's ticket from a missing one")

	resp, _ := editTicket(t, s, "alice", theirs, "Mine now", "alice did not report it.")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = setStatus(t, s, "alice", theirs, "done")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = setStatus(t, s, "alice", mine, "done")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "an author does not change the status of their own ticket")
	resp, _ = editTicket(t, s, "dana", theirs, "Changed by hotel-ops", "dana did not report it.")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	_, page := s.get(t, "bob", theirs)
	assert.Contains(t, page, `<h1 id="ticket-title">Broken lamp</h1>`)
	assert.Contains(t, page, `<dd id="ticket-status" class="status open">Open</dd>`)
	_, page = s.get(t, "dana", theirs)
	assert.Contains(t, page, `id="set-status"`)

	_, list := s.get(t, "alice", "/tickets")
	assert.Contains(t, list, `href="`+mine+`"`)
	assert.NotContains(t, list, `href="`+theirs+`"`)
	_, list = s.get(t, "dana", "/tickets")
	assert.Contains(t, list, `href="`+mine+`"`)
	assert.Contains(t, list, `href="`+theirs+`"`)

	resp, _ = editTicket(t, s, "alice", mine, "Leaking hot tap", "It drips all night.")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, mine, resp.Header.Get("Location"))
	resp, _ = setStatus(t, s, "dana", mine, "in_progress")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	_, page = s.get(t, "alice", mine)
	assert.Contains(t, page, `<h1 id="ticket-title">Leaking hot tap</h1>`)
	assert.Contains(t, page, `<dd id="ticket-status" class="status in_progress">In progress</dd>`)
	assert.NotContains(t, page, `id="edit"`)
	assert.NotContains(t, page, `id="set-status"`)
	resp, _ = s.get(t, "alice", mine+"/edit")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a ticket in progress is no longer edited")
	resp, _ = editTicket(t, s, "alice", mine, "Too late", "It is in progress.")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	_, status, err := s.dialLive(t, "carol", "/tickets/__ws")
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, status, "the list's live connection checks the same rule")

	// Once the line of this forged token is in, so is every line logged before it.
	resp, _ = s.post(t, "alice", "/tickets/report", url.Values{
		"_aicoded_form": {routeKey("/tickets/report") + ".report"},
		"_aicoded_csrf": {"forged"},
	})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	s.waitLogged(t, badTokens+1, badToken)
}

// rowStatus is a script that reports whether the list shows the ticket at path with status.
func rowStatus(path, status string) string {
	return fmt.Sprintf(`document.querySelector('#ticket-%s .status')?.textContent === %q`, strings.TrimPrefix(path, "/tickets/"), status)
}

// alice reports a problem in Chrome. dana, of hotel-ops, sees it in her list and changes its
// status, and both her list and alice's, open in another browser, show the new status live. A
// ticket bob reports shows in dana's list, and never in alice's, which is sent again when hers
// changes.
func TestRoomsLiveStatus(t *testing.T) {
	s := startRooms(t)
	alice := browser(t)
	switchPersona(t, alice, s, "alice")
	var location string
	require.NoError(t, chromedp.Run(alice,
		chromedp.Navigate(s.url("/tickets/report")),
		chromedp.SetValue("#room", "305"),
		chromedp.SendKeys("#title", "No hot water"),
		chromedp.SendKeys("#details", "The shower runs cold."),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#ticket-title"),
		chromedp.Location(&location),
	))
	path := strings.TrimPrefix(location, s.url(""))
	require.Regexp(t, `^/tickets/\d+$`, path)
	var room string
	require.NoError(t, chromedp.Run(alice,
		chromedp.Text("#ticket-room", &room),
		countSockets(),
		chromedp.Navigate(s.url("/tickets")),
		until(live),
		until(rowStatus(path, "Open")),
	))
	assert.Equal(t, "305", room)

	dana := browser(t)
	switchPersona(t, dana, s, "dana")
	require.NoError(t, chromedp.Run(dana,
		countSockets(),
		chromedp.Navigate(s.url("/tickets")),
		until(live),
		until(rowStatus(path, "Open")),
	))
	form, closeForm := chromedp.NewContext(dana)
	t.Cleanup(closeForm)
	var status string
	require.NoError(t, chromedp.Run(form,
		chromedp.Navigate(s.url(path+"/status")),
		chromedp.SetValue("#status", "in_progress"),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#ticket-status"),
		chromedp.Text("#ticket-status", &status),
	))
	assert.Equal(t, "In progress", status)
	require.NoError(t, chromedp.Run(dana, until(rowStatus(path, "In progress"))))
	require.NoError(t, chromedp.Run(alice, until(rowStatus(path, "In progress"))))

	other := newTicket(t, s, "bob", "Flickering light")
	require.NoError(t, chromedp.Run(dana, until(rowStatus(other, "Open"))))
	resp, _ := setStatus(t, s, "dana", path, "done")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	var shown bool
	require.NoError(t, chromedp.Run(alice,
		until(rowStatus(path, "Done")),
		chromedp.Evaluate(fmt.Sprintf(`document.querySelector('#ticket-%s') !== null`, strings.TrimPrefix(other, "/tickets/")), &shown),
	))
	assert.False(t, shown, "alice's list is sent again with her own tickets only")
	noViolations(t, alice)
	noViolations(t, dana)
}

// An employee's live list wakes only when one of their own tickets changes, so it never tells them
// when another's ticket changed. Once alice's and dana's lists follow their tickets, which their
// first patches show, bob edits his ticket and dana changes its status. dana's list shows each
// change, so each was told to the lists before alice edits hers: the first patch alice gets after
// that is her own change.
func TestRoomsLiveListFollowsOwnTickets(t *testing.T) {
	s := startRooms(t)
	mine := newTicket(t, s, "alice", "Dripping shower")
	theirs := newTicket(t, s, "bob", "Noisy fan")
	patch := func(f frame) bool { return f.T == "patch" }
	follow := func(persona string) *websocket.Conn {
		c, _, err := s.dialLive(t, persona, "/tickets/__ws")
		require.NoError(t, err)
		next(t, c, func(f frame) bool { return f.T == "init" })
		return c
	}
	alice := follow("alice")
	require.Contains(t, ticketRow(next(t, alice, patch).text(), mine), "Dripping shower", "the list follows alice's tickets")
	dana := follow("dana")
	require.Contains(t, ticketRow(next(t, dana, patch).text(), theirs), "Noisy fan", "the list follows every ticket")
	shows := func(want string) func(frame) bool {
		return func(f frame) bool { return patch(f) && strings.Contains(ticketRow(f.text(), theirs), want) }
	}

	resp, _ := editTicket(t, s, "bob", theirs, "Noisy fan in the bathroom", "It hums all night.")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	next(t, dana, shows("Noisy fan in the bathroom"))
	resp, _ = setStatus(t, s, "dana", theirs, "in_progress")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	next(t, dana, shows("In progress"))
	title := "Dripping shower " + strings.ToLower(rand.Text()[:8])
	resp, _ = editTicket(t, s, "alice", mine, title, "It drips all night.")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Contains(t, next(t, alice, patch).text(), title, "bob's ticket did not wake alice's list")
}

// ticketRow returns the row of the ticket at path in a list's markup, or "".
func ticketRow(html, path string) string {
	id := strings.TrimPrefix(path, "/tickets/")
	return regexp.MustCompile(`(?s)<tr id="ticket-` + id + `">.*?</tr>`).FindString(html)
}

// Defect 8: nothing a viewer types reaches a log or a span. A marker goes into every field of
// every form of room-maintenance, on posts the app takes and on posts it refuses. It shows in
// no line any app printed, no log entry and no span.
func TestRoomsFormValuesStayOutOfLogs(t *testing.T) {
	s := startRooms(t)
	since := time.Now()
	marker := "pii" + strings.ToLower(rand.Text()[:13])

	resp, body := report(t, s, "alice", marker, marker, marker)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Invalid syntax", "the app refused the room")
	resp, body = report(t, s, "alice", "101", marker+strings.Repeat("x", 100), marker)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Use at most 100 characters.", "the app refused the title")
	resp, _ = report(t, s, "alice", "101", marker, marker)
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	ticket := resp.Header.Get("Location")

	resp, body = editTicket(t, s, "alice", ticket, marker, marker+strings.Repeat("x", 2000))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Use at most 2000 characters.", "the app refused the details")
	resp, _ = editTicket(t, s, "alice", ticket, marker+" again", marker+" again")
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	resp, _ = editTicket(t, s, "bob", ticket, marker, marker)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, body = setStatus(t, s, "dana", ticket, marker)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, body, "Choose one of the options", "the app refused the status")
	_, body = s.get(t, "alice", ticket)
	require.Contains(t, body, marker+" again", "the ticket was saved with the marker")

	// As in the people test: once the span of the last request is in, so is everything the apps
	// logged and printed while serving the requests before it.
	require.Eventually(t, func() bool {
		spans, err := s.spansSince(t.Context(), since)
		return err == nil && slices.ContainsFunc(spans, func(sp devapi.Span) bool {
			return sp.App == s.app && sp.Attrs["http.path"] == ticket
		})
	}, 10*time.Second, 100*time.Millisecond, "the spans arrive")
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

// No page of room-maintenance, the error pages and a refused form included, breaks its
// Content-Security-Policy in Chrome.
func TestRoomsCSP(t *testing.T) {
	s := startRooms(t)
	mine := newTicket(t, s, "alice", "Squeaky door")
	theirs := newTicket(t, s, "bob", "Stuck window")
	ctx := browser(t)
	for _, v := range []struct {
		persona string
		paths   []string
	}{
		{"alice", []string{"/tickets", "/tickets/report", mine, mine + "/edit", theirs}},
		{"dana", []string{"/tickets", theirs, theirs + "/status", theirs + "000"}},
	} {
		switchPersona(t, ctx, s, v.persona)
		for _, path := range v.paths {
			require.NoError(t, chromedp.Run(ctx, chromedp.Navigate(s.url(path))))
			assert.Empty(t, cspViolations(t, ctx), path)
		}
	}

	switchPersona(t, ctx, s, "alice")
	require.NoError(t, chromedp.Run(ctx,
		chromedp.Navigate(s.url("/tickets/report")),
		chromedp.Evaluate(`document.querySelector("form").noValidate = true`, nil),
		chromedp.SendKeys("#details", "The door squeaks."),
		chromedp.Click(`button[type="submit"]`),
		chromedp.WaitVisible("#title-error"),
	))
	assert.Empty(t, cspViolations(t, ctx), "the report form with an empty title")
}
