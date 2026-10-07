package platform_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/cmd/aicoded/internal/testhome"
	"aicoded.dev/framework/internal/errs"
)

var ana = platform.Whoami{Org: "acme", Email: "ana@acme.example", Scopes: []string{"app:create"}}

// newClient returns a client of p, in a home of its own, and its output, in which no token may
// appear. A browser signs in at each address of a sign-in that the client prints, and sends the
// text of its last page to pages.
func newClient(t *testing.T, p *platformtest.Platform) (*platform.Client, *bytes.Buffer, <-chan string) {
	browse, pages := platformtest.Browser()
	c, out := clientActing(t, p, browse)
	return c, out, pages
}

// clientActing returns a client of p, in a home of its own, and its output, in which no token
// may appear. act gets each address of a sign-in in a browser that the client prints.
func clientActing(t *testing.T, p *platformtest.Platform, act func(address string)) (*platform.Client, *bytes.Buffer) {
	testhome.Set(t)
	out := &bytes.Buffer{}
	c := platform.New(p.URL, platformtest.Watch(out, act))
	t.Cleanup(func() { assert.NotRegexp(t, `aicoded_(at|rt|ac|dc)_`, out.String(), "a token was printed") })
	return c, out
}

// signIn signs in to p in the browser.
func signIn(t *testing.T, c *platform.Client, pages <-chan string) {
	_, err := c.Login(t.Context(), "acme", false)
	require.NoError(t, err)
	<-pages
}

func TestLoginInTheBrowser(t *testing.T) {
	p := platformtest.New(t)
	c, out, pages := newClient(t, p)
	who, err := c.Login(t.Context(), "acme", false)
	require.NoError(t, err)
	assert.Equal(t, ana, who)
	assert.Equal(t, "You can close this window and go back to the terminal.\n", <-pages)
	assert.True(t, strings.HasPrefix(out.String(), "To sign in to acme, open this address in a browser on this computer, "+
		"or run aicoded login --device on a computer without one:\n\n    "+p.URL+"/oauth/authorize?"), out.String())

	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, "acme", cr.Org)
	assert.Regexp(t, `^aicoded_at_`, cr.AccessToken)
	assert.Regexp(t, `^aicoded_rt_`, cr.RefreshToken)
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), cr.Expires, time.Minute)
}

func TestLoginAgainRevokesTheOldSignIn(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	signIn(t, c, pages)
	old, err := platform.Load(p.URL)
	require.NoError(t, err)
	signIn(t, c, pages)
	assert.Equal(t, []string{old.RefreshToken}, p.Revoked())
}

func TestLoginRefusesAWrongState(t *testing.T) {
	p := platformtest.New(t)
	statuses := make(chan []int, 1)
	c, _ := clientActing(t, p, func(address string) { go func() { statuses <- forge(address) }() })
	c.Timeout = 500 * time.Millisecond
	_, err := c.Login(t.Context(), "acme", false)
	assert.Equal(t, "E-CLI-008", errs.Code(err), "a wrong state neither signs in nor ends the sign-in")
	assert.Equal(t, []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusNotFound}, <-statuses)
	assert.Zero(t, p.TokenRequests(), "the code that came with a wrong state is never used")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Empty(t, cr)
}

// forge opens the sign-in at address, and sends the loopback where the platform sends the
// browser back three answers: the platform's with a forged state, a refusal with a forged state,
// and the platform's at another path. It returns their statuses, 0 for an answer it could not
// send.
func forge(address string) []int {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil
	}
	_ = resp.Body.Close()
	back, err := resp.Location()
	if err != nil {
		return nil
	}
	forged := back.Query()
	forged.Set("state", "forged")
	refusal := url.Values{"error": {"access_denied"}, "error_description": {"forged"}, "state": {"forged"}}
	return []int{status(*back, "", forged), status(*back, "", refusal), status(*back, "/other", back.Query())}
}

// status gets u, with path instead of its own when set and with the query q, and returns the
// answer's status.
func status(u url.URL, path string, q url.Values) int {
	if path != "" {
		u.Path = path
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return 0
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestLoginRefused(t *testing.T) {
	for _, tc := range []struct{ code, description, want, fix string }{
		{"access_denied", "not in the builder group of acme", "access_denied: not in the builder group of acme",
			"ask your administrator to add you to the builder group"},
		{"access_denied", "not invited to acme", "access_denied: not invited to acme",
			"ask your administrator to add you to the builder group"},
		{"server_error", "the sign-in failed\x1b[2J on the server", "server_error: the sign-in failed?[2J on the server",
			"run aicoded login again"},
	} {
		p := platformtest.New(t)
		c, _, pages := newClient(t, p)
		p.Refuse(tc.code, tc.description)
		_, err := c.Login(t.Context(), "acme", false)
		assert.Equal(t, "E-CLI-006", errs.Code(err), tc.code)
		require.ErrorContains(t, err, tc.want)
		require.ErrorContains(t, err, "fix: "+tc.fix)
		assert.Equal(t, "The sign-in was refused. Go back to the terminal to see why; you can close this window.\n", <-pages)
		assert.Zero(t, p.TokenRequests())
	}
}

func TestLoginUnknownOrg(t *testing.T) {
	for _, device := range []bool{false, true} {
		p := platformtest.New(t)
		c, _, _ := newClient(t, p)
		_, err := c.Login(t.Context(), "nobody", device)
		assert.Equal(t, "E-CLI-006", errs.Code(err), "device %v", device)
		require.ErrorContains(t, err, "invalid_request: no organisation has that name")
		require.ErrorContains(t, err, "fix: check the organisation's name with your administrator, then run aicoded login --org <organisation>")
	}
}

func TestLoginCancelled(t *testing.T) {
	for _, device := range []bool{false, true} {
		p := platformtest.New(t)
		c, _ := clientActing(t, p, func(string) {})
		p.Pending(100)
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(100*time.Millisecond, cancel)
		_, err := c.Login(ctx, "acme", device)
		require.ErrorIs(t, err, platform.ErrCancelled, "device %v", device)
		assert.Equal(t, "sign-in cancelled", err.Error())
	}
}

func TestLoginWithACode(t *testing.T) {
	p := platformtest.New(t)
	c, out, _ := newClient(t, p)
	p.Pending(1)
	who, err := c.Login(t.Context(), "acme", true)
	require.NoError(t, err)
	assert.Equal(t, ana, who)
	assert.Equal(t, "To sign in to acme, open\n\n    "+p.URL+"/device\n\nin a browser on any device, and enter the code BCDF-GHJK\n",
		out.String())
	assert.Equal(t, 2, p.TokenRequests(), "it polls until the sign-in ends")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, "acme", cr.Org)
}

func TestLoginWithACodeEnds(t *testing.T) {
	for code, want := range map[string]string{"expired_token": "E-CLI-008", "access_denied": "E-CLI-006"} {
		p := platformtest.New(t)
		c, _, _ := newClient(t, p)
		p.DeviceError(code)
		_, err := c.Login(t.Context(), "acme", true)
		assert.Equal(t, want, errs.Code(err), code)
		cr, err := platform.Load(p.URL)
		require.NoError(t, err)
		assert.Empty(t, cr, code)
	}
}

func TestLoginChecksTheCodeAnswer(t *testing.T) {
	for name, edit := range map[string]func(map[string]any){
		"another site": func(a map[string]any) { a["verification_uri"] = "https://evil.example/device" },
		"a lookalike": func(a map[string]any) {
			a["verification_uri"] = strings.TrimSuffix(a["verification_uri"].(string), "/device") + ".evil.example/device"
		},
		"control":         func(a map[string]any) { a["verification_uri"] = a["verification_uri"].(string) + "\x1b[2J" },
		"a long code":     func(a map[string]any) { a["user_code"] = "BCDF-GHJK-LMNP" },
		"a code to steer": func(a map[string]any) { a["user_code"] = "BCDF\x1b[2J" },
		"no device code":  func(a map[string]any) { a["device_code"] = "" },
		"a bad interval":  func(a map[string]any) { a["interval"] = -1 },
	} {
		p := platformtest.New(t)
		c, out, _ := newClient(t, p)
		p.DeviceAnswer(edit)
		_, err := c.Login(t.Context(), "acme", true)
		require.ErrorContains(t, err, "not valid", name)
		assert.Empty(t, out.String(), name)
		assert.Zero(t, p.TokenRequests(), name)
	}
}

func TestLoginOrganisation(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	_, err := c.Login(t.Context(), "", true)
	require.ErrorIs(t, err, platform.ErrNoOrg, "the first login needs one")
	_, err = c.Login(t.Context(), "Acme Corp", true)
	require.ErrorContains(t, err, "not an organisation's name")

	signIn(t, c, pages)
	who, err := c.Login(t.Context(), "", false)
	require.NoError(t, err, "it is remembered")
	assert.Equal(t, ana, who)
	<-pages
}

func TestWhoamiRefreshes(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	p.ExpiresIn(20)
	signIn(t, c, pages)
	before, err := platform.Load(p.URL)
	require.NoError(t, err)

	p.ExpiresIn(900)
	who, err := c.Whoami(t.Context())
	require.NoError(t, err)
	assert.Equal(t, ana, who)
	after, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.NotEqual(t, before.AccessToken, after.AccessToken, "a token that expires within 30 s is refreshed")
	assert.NotEqual(t, before.RefreshToken, after.RefreshToken)
	assert.Equal(t, "acme", after.Org)

	requests := p.TokenRequests()
	_, err = c.Whoami(t.Context())
	require.NoError(t, err)
	assert.Equal(t, requests, p.TokenRequests(), "a fresh token is used as it is")
}

func TestWhoamiSignedOut(t *testing.T) {
	p := platformtest.New(t)
	c, _, _ := newClient(t, p)
	_, err := c.Whoami(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "fix: run aicoded login --org <your organisation>")
	_, err = c.Token(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
}

func TestRefreshRefused(t *testing.T) {
	p := platformtest.New(t)
	c, _, _ := newClient(t, p)
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_old",
		RefreshToken: "aicoded_rt_revoked", Expires: time.Now()}))
	_, err := c.Token(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "the refresh token is not valid")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr, "the tokens are gone, the organisation is remembered")
}

// saveExpired keeps a sign-in to the platform at address whose access token has expired, and
// returns the credentials file.
func saveExpired(t *testing.T, address string) []byte {
	require.NoError(t, platform.Save(address, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_old",
		RefreshToken: "aicoded_rt_old", Expires: time.Now()}))
	return credentialsFile(t)
}

func credentialsFile(t *testing.T) []byte {
	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, "aicoded", "credentials.json"))
	require.NoError(t, err)
	return b
}

func TestRefreshFails(t *testing.T) {
	for name, tc := range map[string]struct {
		fail func(*platformtest.Platform)
		want string
	}{
		"temporarily_unavailable": {func(p *platformtest.Platform) {
			p.RefreshError(http.StatusServiceUnavailable, "temporarily_unavailable")
		}, "E-CLI-009"},
		"server_error":           {func(p *platformtest.Platform) { p.RefreshError(http.StatusInternalServerError, "server_error") }, "E-CLI-009"},
		"a 502 without an error": {func(p *platformtest.Platform) { p.RefreshError(http.StatusBadGateway, "") }, "E-CLI-009"},
		"no connection":          {func(p *platformtest.Platform) { p.Close() }, "E-CLI-009"},
		"invalid_client":         {func(p *platformtest.Platform) { p.RefreshError(http.StatusUnauthorized, "invalid_client") }, "E-CLI-006"},
	} {
		p := platformtest.New(t)
		c, _, _ := newClient(t, p)
		before := saveExpired(t, p.URL)
		tc.fail(p)
		_, err := c.Token(t.Context())
		assert.Equal(t, tc.want, errs.Code(err), name)
		if tc.want == "E-CLI-009" {
			require.ErrorContains(t, err, "the platform could not refresh your sign-in right now: ", name)
			require.ErrorContains(t, err, "fix: try again in a minute: your sign-in is kept", name)
		}
		assert.NotContains(t, err.Error(), "aicoded_rt_", name)
		assert.Equal(t, string(before), string(credentialsFile(t)), "%s: the sign-in is kept", name)
	}
}

func TestRefreshTimesOut(t *testing.T) {
	testhome.Set(t)
	stop := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-stop }))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(stop) })
	c := platform.New(slow.URL, &bytes.Buffer{})
	c.HTTP.Timeout = 100 * time.Millisecond
	before := saveExpired(t, slow.URL)
	_, err := c.Token(t.Context())
	assert.Equal(t, "E-CLI-009", errs.Code(err))
	assert.Equal(t, string(before), string(credentialsFile(t)))
}

func TestWhoamiRefusedToken(t *testing.T) {
	p := platformtest.New(t)
	c, _, _ := newClient(t, p)
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_revoked",
		RefreshToken: "aicoded_rt_revoked", Expires: time.Now().Add(time.Hour)}))
	_, err := c.Whoami(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
}

func TestLogout(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	signIn(t, c, pages)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)

	signedIn, revoked, err := c.Logout(t.Context())
	require.NoError(t, err)
	assert.True(t, signedIn)
	assert.True(t, revoked)
	assert.Equal(t, []string{cr.RefreshToken}, p.Revoked())
	cr, err = platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr)

	signedIn, _, err = c.Logout(t.Context())
	require.NoError(t, err)
	assert.False(t, signedIn)
}

func TestLogoutWithoutThePlatform(t *testing.T) {
	p := platformtest.New(t)
	c, _, pages := newClient(t, p)
	signIn(t, c, pages)
	p.Close()
	signedIn, revoked, err := c.Logout(t.Context())
	require.NoError(t, err)
	assert.True(t, signedIn)
	assert.False(t, revoked)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr, "the tokens are deleted all the same")
}

func TestNoRedirects(t *testing.T) {
	testhome.Set(t)
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	t.Cleanup(elsewhere.Close)
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(moved.Close)
	c := platform.New(moved.URL, &bytes.Buffer{})
	cr := platform.Credentials{Org: "acme", AccessToken: "aicoded_at_a", RefreshToken: "aicoded_rt_a", Expires: time.Now().Add(time.Hour)}
	require.NoError(t, platform.Save(moved.URL, cr))
	_, err := c.Whoami(t.Context())
	require.ErrorContains(t, err, "307")

	cr.Expires = time.Now()
	require.NoError(t, platform.Save(moved.URL, cr))
	_, err = c.Token(t.Context())
	require.ErrorContains(t, err, "307")

	require.NoError(t, platform.Save(moved.URL, cr))
	_, revoked, err := c.Logout(t.Context())
	require.NoError(t, err)
	assert.False(t, revoked)
	assert.Zero(t, reached.Load(), "no token followed a redirect")
}
