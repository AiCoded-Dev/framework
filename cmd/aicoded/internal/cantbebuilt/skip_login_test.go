package cantbebuilt

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/runnerproto/viewer"
	"aicoded.dev/framework/web"
)

// A request that skips the login: someone who reaches the app without passing the company login
// sends it requests with no token, with a token they made, or with a real one that has expired or
// was made for another app. The notes page is one only staff may open and has a form that adds a
// note and a live list (testdata/staff-notes). Each request goes straight to the app's socket,
// and each form carries the form token of a page a member of staff opened. The app refuses every
// request with 401 before any page code runs, logs why, and adds no note. The same requests with
// a token the company login signed for that member of staff get the page, add the note and open
// the live connection.
func TestRequestThatSkipsTheLogin(t *testing.T) {
	a := startApp(t, "staff-notes")
	ana := devconfig.Persona{Name: "ana", Roles: []string{"staff"}}
	_, page := a.send(t, pageRequest(t, a.token(t, ana)))
	form := url.Values{web.FormField: {field(t, page, web.FormField)}, web.CSRFField: {field(t, page, web.CSRFField)}}

	valid := a.token(t, ana)
	otherKey, err := dev.NewSigner()
	require.NoError(t, err)
	forged, err := otherKey.Mint(ana, a.Name, time.Now())
	require.NoError(t, err)
	expired, err := a.signer.Mint(ana, a.Name, time.Now().Add(-2*viewer.MaxLifetime))
	require.NoError(t, err)
	otherApp, err := a.signer.Mint(ana, "billing", time.Now())
	require.NoError(t, err)

	form.Set("note", "sneaked in")
	var reasons []string
	for _, try := range []struct {
		name, token string
		refused     error
	}{
		{"no token", "", viewer.ErrMalformed},
		{"a malformed token: a valid one without its signature", valid[:strings.LastIndex(valid, ".")], viewer.ErrMalformed},
		{"a forged token, signed by another key", forged, viewer.ErrSignature},
		{"an expired token", expired, viewer.ErrExpired},
		{"a token for another app", otherApp, viewer.ErrAudience},
	} {
		for _, req := range []*http.Request{pageRequest(t, try.token), formRequest(t, try.token, form), liveRequest(t, try.token)} {
			what := req.Method + " " + req.URL.Path + " with " + try.name
			code, body := a.send(t, req)
			assert.Equal(t, http.StatusUnauthorized, code, what)
			assert.Equal(t, "Unauthorized\n", body, "%s: nothing of the page", what)
			reasons = append(reasons, try.refused.Error())
		}
	}
	assert.Equal(t, reasons, a.refused(len(reasons)), "the app logs why it refused each request")

	form.Set("note", "signed in")
	code, page := a.send(t, pageRequest(t, a.token(t, ana)))
	assert.Equal(t, http.StatusOK, code, "the page")
	assert.Contains(t, page, "<h1>Staff notes</h1>")
	code, _ = a.send(t, formRequest(t, a.token(t, ana), form))
	assert.Equal(t, http.StatusSeeOther, code, "the form")
	code, _ = a.send(t, liveRequest(t, a.token(t, ana)))
	assert.Equal(t, http.StatusSwitchingProtocols, code, "the live connection")

	_, page = a.send(t, pageRequest(t, a.token(t, ana)))
	assert.Equal(t, []string{"signed in"}, notes(page), "only the signed-in form added a note")
}

// pageRequest, formRequest and liveRequest return the requests of the notes page with token, or
// with no token when it is empty: the page, the form with the fields in form, and the live
// connection.
func pageRequest(t *testing.T, token string) *http.Request {
	return request(t, http.MethodGet, "/notes", nil, token)
}

func formRequest(t *testing.T, token string, form url.Values) *http.Request {
	req := request(t, http.MethodPost, "/notes", strings.NewReader(form.Encode()), token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func liveRequest(t *testing.T, token string) *http.Request {
	req := request(t, http.MethodGet, "/notes/__ws", nil, token)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", base64.StdEncoding.EncodeToString([]byte(rand.Text()[:16])))
	return req
}

// field returns the value of the hidden input name in page.
func field(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(page)
	require.NotNil(t, m, "no %s in the page", name)
	return m[1]
}

// notes returns the notes the notes page lists.
func notes(page string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`<li>([^<]*)</li>`).FindAllStringSubmatch(page, -1) {
		out = append(out, m[1])
	}
	return out
}

// refused returns the reasons the app logged for the requests it refused, in order. It waits up
// to 10 seconds for n of them.
func (a *runningApp) refused(n int) []string {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var reasons []string
		for line := range strings.Lines(a.out.String()) {
			var entry struct{ Msg, Reason string }
			_, js, _ := strings.Cut(line, " | ")
			if json.Unmarshal([]byte(js), &entry) == nil && entry.Msg == "request refused" {
				reasons = append(reasons, entry.Reason)
			}
		}
		if len(reasons) >= n || time.Now().After(deadline) {
			return reasons
		}
		time.Sleep(10 * time.Millisecond)
	}
}
