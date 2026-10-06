package platform_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

// newClient returns a client of p, in a home of its own, whose output is out. No browser opens
// unless the test sets Open, and no token may appear in out.
func newClient(t *testing.T, p *platformtest.Platform) (*platform.Client, *bytes.Buffer) {
	testhome.Set(t)
	out := &bytes.Buffer{}
	c := platform.New(p.URL, out)
	c.Open = func(context.Context, string) error {
		t.Error("a browser opened")
		return errors.New("no browser")
	}
	t.Cleanup(func() { assert.NotRegexp(t, `aicoded_(at|rt|ac|dc)_`, out.String(), "a token was printed") })
	return c, out
}

// signIn signs in to p in the browser.
func signIn(t *testing.T, c *platform.Client) {
	open, pages := platformtest.Browser()
	c.Open = open
	_, err := c.Login(t.Context(), "acme", false)
	require.NoError(t, err)
	<-pages
}

func TestLoginInTheBrowser(t *testing.T) {
	p := platformtest.New(t)
	c, out := newClient(t, p)
	open, pages := platformtest.Browser()
	c.Open = open
	who, err := c.Login(t.Context(), "acme", false)
	require.NoError(t, err)
	assert.Equal(t, ana, who)
	assert.Equal(t, "You can close this window and go back to the terminal.\n", <-pages)
	assert.Contains(t, out.String(), "\n    "+p.URL+"/oauth/authorize?")

	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, "acme", cr.Org)
	assert.Regexp(t, `^aicoded_at_`, cr.AccessToken)
	assert.Regexp(t, `^aicoded_rt_`, cr.RefreshToken)
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), cr.Expires, time.Minute)
}

func TestLoginAgainRevokesTheOldSignIn(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	signIn(t, c)
	old, err := platform.Load(p.URL)
	require.NoError(t, err)
	signIn(t, c)
	assert.Equal(t, []string{old.RefreshToken}, p.Revoked())
}

func TestLoginRefusesAWrongState(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	c.Timeout = 500 * time.Millisecond
	statuses := make(chan []int, 1)
	c.Open = func(_ context.Context, address string) error {
		go func() { statuses <- forge(address) }()
		return nil
	}
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
		c, _ := newClient(t, p)
		p.Refuse(tc.code, tc.description)
		open, pages := platformtest.Browser()
		c.Open = open
		_, err := c.Login(t.Context(), "acme", false)
		assert.Equal(t, "E-CLI-006", errs.Code(err), tc.code)
		require.ErrorContains(t, err, tc.want)
		require.ErrorContains(t, err, "fix: "+tc.fix)
		assert.Equal(t, "The sign-in was refused. Go back to the terminal to see why; you can close this window.\n", <-pages)
		assert.Zero(t, p.TokenRequests())
	}
}

func TestLoginWithACode(t *testing.T) {
	p := platformtest.New(t)
	c, out := newClient(t, p)
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

func TestLoginWithACodeWhenNoBrowserOpens(t *testing.T) {
	p := platformtest.New(t)
	c, out := newClient(t, p)
	c.Open = func(context.Context, string) error {
		return errors.New(`exec: "xdg-open": executable file not found in $PATH`)
	}
	who, err := c.Login(t.Context(), "acme", false)
	require.NoError(t, err)
	assert.Equal(t, ana, who)
	assert.Contains(t, out.String(), "\nNo browser opened (exec: \"xdg-open\": executable file not found in $PATH), "+
		"so sign in with a code instead.\n\nTo sign in to acme, open\n")
}

func TestLoginWithACodeEnds(t *testing.T) {
	for code, want := range map[string]string{"expired_token": "E-CLI-008", "access_denied": "E-CLI-006"} {
		p := platformtest.New(t)
		c, _ := newClient(t, p)
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
		c, out := newClient(t, p)
		p.DeviceAnswer(edit)
		_, err := c.Login(t.Context(), "acme", true)
		require.ErrorContains(t, err, "not valid", name)
		assert.Empty(t, out.String(), name)
		assert.Zero(t, p.TokenRequests(), name)
	}
}

func TestLoginOrganisation(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	_, err := c.Login(t.Context(), "", true)
	require.ErrorIs(t, err, platform.ErrNoOrg, "the first login needs one")
	_, err = c.Login(t.Context(), "Acme Corp", true)
	require.ErrorContains(t, err, "not an organisation's name")

	signIn(t, c)
	open, pages := platformtest.Browser()
	c.Open = open
	who, err := c.Login(t.Context(), "", false)
	require.NoError(t, err, "it is remembered")
	assert.Equal(t, ana, who)
	<-pages
}

func TestWhoamiRefreshes(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	p.ExpiresIn(20)
	signIn(t, c)
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
	c, _ := newClient(t, p)
	_, err := c.Whoami(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "fix: run aicoded login --org <your organisation>")
	_, err = c.Token(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
}

func TestRefreshRefused(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_old",
		RefreshToken: "aicoded_rt_revoked", Expires: time.Now()}))
	_, err := c.Token(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
	require.ErrorContains(t, err, "the refresh token is not valid")
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Equal(t, platform.Credentials{Org: "acme"}, cr, "the tokens are gone, the organisation is remembered")
}

func TestWhoamiRefusedToken(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	require.NoError(t, platform.Save(p.URL, platform.Credentials{Org: "acme", AccessToken: "aicoded_at_revoked",
		RefreshToken: "aicoded_rt_revoked", Expires: time.Now().Add(time.Hour)}))
	_, err := c.Whoami(t.Context())
	assert.Equal(t, "E-CLI-004", errs.Code(err))
}

func TestLogout(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	signIn(t, c)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)

	signedIn, revoked, err := c.Logout(t.Context())
	require.NoError(t, err)
	assert.True(t, signedIn)
	assert.True(t, revoked)
	assert.Equal(t, []string{cr.RefreshToken}, p.Revoked())
	cr, err = platform.Load(p.URL)
	require.NoError(t, err)
	assert.Empty(t, cr)

	signedIn, _, err = c.Logout(t.Context())
	require.NoError(t, err)
	assert.False(t, signedIn)
}

func TestLogoutWithoutThePlatform(t *testing.T) {
	p := platformtest.New(t)
	c, _ := newClient(t, p)
	signIn(t, c)
	p.Close()
	signedIn, revoked, err := c.Logout(t.Context())
	require.NoError(t, err)
	assert.True(t, signedIn)
	assert.False(t, revoked)
	cr, err := platform.Load(p.URL)
	require.NoError(t, err)
	assert.Empty(t, cr, "the sign-in is deleted all the same")
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

func TestOpenBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake browser is a shell script")
	}
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho \"$@\" >>"+ran+"\n"), 0o700))
	t.Setenv("PATH", dir)
	for _, address := range []string{"file:///etc/passwd", "http://evil.example/", "https://ana@evil.example/", "-a", "javascript:alert(1)"} {
		require.Error(t, platform.OpenBrowser(t.Context(), address), address)
	}
	require.NoError(t, platform.OpenBrowser(t.Context(), "https://api.aicoded.cloud/oauth/authorize?a=b&c=d"))
	b, err := os.ReadFile(ran)
	require.NoError(t, err)
	assert.Equal(t, "https://api.aicoded.cloud/oauth/authorize?a=b&c=d\n", string(b), "only the https address opened")
}
