// Package platformtest fakes the platform on 127.0.0.1, for the tests of aicoded: its sign-in,
// token, revocation and whoami endpoints, and the builder API that aicoded publish and status
// call.
package platformtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The organisation of the fake, and the person who signs in to it.
const (
	Org   = "acme"
	Email = "ana@acme.example"
)

// Platform is a fake platform. Everyone who signs in gets the scope app:create, and the scopes
// app:<id>:release:request and app:<id>:status of each app they own that is not archived, as they
// are when the token is issued.
type Platform struct {
	// URL is its address, http://127.0.0.1:<port>.
	URL string
	srv *httptest.Server

	mu            sync.Mutex
	refusal       []string
	refreshError  *failure
	deviceError   string
	pending       int
	expiresIn     int
	deviceAnswer  func(map[string]any)
	codes         map[string]grant
	access        map[string]session
	refresh       map[string]bool
	devices       map[string]int
	revoked       []string
	tokenRequests int
	builder
}

// session is what an access token grants until it expires.
type session struct {
	expires time.Time
	scopes  []string
}

type grant struct{ challenge, redirect string }

type failure struct {
	status int
	code   string
}

// New starts a fake platform, closed when the test ends.
func New(t testing.TB) *Platform {
	p := &Platform{expiresIn: 900, codes: map[string]grant{}, access: map[string]session{}, refresh: map[string]bool{},
		devices: map[string]int{}, builder: newBuilder()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth/authorize", p.authorize)
	mux.HandleFunc("POST /oauth/token", p.token)
	mux.HandleFunc("POST /oauth/device", p.device)
	mux.HandleFunc("POST /oauth/revoke", p.revoke)
	mux.HandleFunc("GET /v1/whoami", p.whoami)
	mux.HandleFunc("POST /v1/apps/get", p.getApp)
	mux.HandleFunc("POST /v1/publishes/create", p.createPublish)
	mux.HandleFunc("POST /v1/publishes/get", p.getPublish)
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	p.URL = p.srv.URL
	return p
}

// Close stops the platform, so that it can no longer be reached.
func (p *Platform) Close() { p.srv.Close() }

// Refuse ends every sign-in in the browser with an RFC 6749 error, as the platform does when it
// refuses the person.
func (p *Platform) Refuse(code, description string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refusal = []string{code, description}
}

// RefreshError answers every refresh with status and the RFC 6749 error code, or with a body
// that is not JSON when code is empty, and keeps the refresh token usable.
func (p *Platform) RefreshError(status int, code string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshError = &failure{status, code}
}

// DeviceError answers every poll of a device sign-in with the error code.
func (p *Platform) DeviceError(code string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deviceError = code
}

// Pending makes a device sign-in answer n polls with authorization_pending before it succeeds.
func (p *Platform) Pending(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending = n
}

// ExpiresIn sets the lifetime of the access tokens it issues, in seconds.
func (p *Platform) ExpiresIn(seconds int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expiresIn = seconds
}

// DeviceAnswer lets edit change the answer of /oauth/device before it is sent.
func (p *Platform) DeviceAnswer(edit func(map[string]any)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deviceAnswer = edit
}

// Revoked returns the tokens it was asked to revoke.
func (p *Platform) Revoked() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.revoked...)
}

// TokenRequests returns how many requests its token endpoint got.
func (p *Platform) TokenRequests() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokenRequests
}

// Browser returns what a person does with the address of a sign-in in a browser: in the
// background, open it and follow the sign-in back to aicoded. The text of the last page it got,
// or its error, goes to pages.
func Browser() (browse func(address string), pages <-chan string) {
	ch := make(chan string, 4)
	return func(address string) { go func() { ch <- get(address) }() }, ch
}

// Watch returns a writer for aicoded's output that goes on to out and calls act with the address
// of each sign-in in a browser that aicoded prints on a line of its own.
func Watch(out io.Writer, act func(address string)) io.Writer {
	return &watcher{out: out, act: act}
}

type watcher struct {
	out     io.Writer
	act     func(string)
	text    []byte
	scanned int
}

func (w *watcher) Write(b []byte) (int, error) {
	w.text = append(w.text, b...)
	for {
		i := bytes.IndexByte(w.text[w.scanned:], '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(string(w.text[w.scanned : w.scanned+i]))
		w.scanned += i + 1
		if strings.HasPrefix(line, "http") && strings.Contains(line, "/oauth/authorize?") {
			w.act(line)
		}
	}
	return w.out.Write(b)
}

func get(address string) string {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, nil)
	if err != nil {
		return err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

func (p *Platform) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || back.Scheme != "http" || back.Hostname() != "127.0.0.1" || back.Path != "/callback" {
		http.Error(w, "redirect_uri must be http://127.0.0.1:<port>/callback", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(back.Port())
	if err != nil || q.Get("client_id") != "aicoded" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		len(q.Get("code_challenge")) != 43 || q.Get("state") == "" || q.Get("scope") != "app:create" {
		http.Error(w, "this sign-in link is not valid", http.StatusBadRequest)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	answer := "state=" + url.QueryEscape(q.Get("state"))
	switch {
	case q.Get("org") != Org:
		answer += "&error=invalid_request&error_description=" + url.QueryEscape("no organisation has that name")
	case p.refusal != nil:
		answer += "&error=" + url.QueryEscape(p.refusal[0]) + "&error_description=" + url.QueryEscape(p.refusal[1])
	default:
		code := secret("aicoded_ac_")
		p.codes[code] = grant{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri")}
		answer += "&code=" + url.QueryEscape(code)
	}
	http.Redirect(w, r, "http://127.0.0.1:"+strconv.Itoa(port)+"/callback?"+answer, http.StatusFound)
}

func (p *Platform) token(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokenRequests++
	if r.Header.Get("Authorization") != "" || r.PostFormValue("client_id") != "aicoded" {
		oauthError(w, http.StatusUnauthorized, "invalid_client", "public clients send client_id in the form")
		return
	}
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		code := r.PostFormValue("code")
		g, ok := p.codes[code]
		delete(p.codes, code)
		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		if !ok || r.PostFormValue("redirect_uri") != g.redirect || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the code is not valid")
			return
		}
	case "refresh_token":
		if f := p.refreshError; f != nil {
			if f.code == "" {
				http.Error(w, http.StatusText(f.status), f.status)
			} else {
				oauthError(w, f.status, f.code, "the refresh failed")
			}
			return
		}
		rt := r.PostFormValue("refresh_token")
		if !p.refresh[rt] {
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the refresh token is not valid")
			return
		}
		delete(p.refresh, rt)
	case "urn:ietf:params:oauth:grant-type:device_code":
		dc := r.PostFormValue("device_code")
		polls, ok := p.devices[dc]
		switch {
		case !ok:
			oauthError(w, http.StatusBadRequest, "invalid_grant", "the device code is not valid")
			return
		case p.deviceError != "":
			oauthError(w, http.StatusBadRequest, p.deviceError, "the device sign-in ended")
			return
		case polls < p.pending:
			p.devices[dc]++
			oauthError(w, http.StatusBadRequest, "authorization_pending", "the sign-in is not finished yet")
			return
		}
		delete(p.devices, dc)
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "unknown grant_type")
		return
	}
	access, refresh := p.issue()
	writeJSON(w, http.StatusOK, map[string]any{"access_token": access, "token_type": "Bearer", "expires_in": p.expiresIn,
		"refresh_token": refresh, "scope": strings.Join(p.access[access].scopes, " ")})
}

// issue issues an access token and a refresh token, with the scopes of the person who signs in.
// The caller holds p.mu.
func (p *Platform) issue() (access, refresh string) {
	access, refresh = secret("aicoded_at_"), secret("aicoded_rt_")
	p.access[access] = session{expires: time.Now().Add(time.Duration(p.expiresIn) * time.Second), scopes: p.scopes()}
	p.refresh[refresh] = true
	return access, refresh
}

func (p *Platform) device(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.PostFormValue("client_id") != "aicoded" || r.PostFormValue("org") != Org || r.PostFormValue("scope") != "app:create" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "no organisation has that name")
		return
	}
	dc := secret("aicoded_dc_")
	p.devices[dc] = 0
	answer := map[string]any{"device_code": dc, "user_code": "BCDF-GHJK", "verification_uri": p.URL + "/device",
		"expires_in": 600, "interval": 1}
	if p.deviceAnswer != nil {
		p.deviceAnswer(answer)
	}
	writeJSON(w, http.StatusOK, answer)
}

func (p *Platform) revoke(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tok := r.PostFormValue("token")
	p.revoked = append(p.revoked, tok)
	delete(p.refresh, tok)
	w.WriteHeader(http.StatusOK)
}

func (p *Platform) whoami(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.session(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"org": Org, "email": Email, "client": "aicoded", "scopes": s.scopes,
		"expires": s.expires.UTC()})
}

// session returns the session of the request's access token, or answers 401 and returns false.
// The caller holds p.mu.
func (p *Platform) session(w http.ResponseWriter, r *http.Request) (session, bool) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	s, ok := p.access[tok]
	if !ok || !time.Now().Before(s.expires) {
		oauthError(w, http.StatusUnauthorized, "invalid_token", "the token is unknown, expired or revoked")
		return session{}, false
	}
	return s, true
}

func secret(prefix string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

func oauthError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
