// Package devui is the dev UI of aicoded dev: its pages, behind a front that lets in only the
// browser that opened the login link aicoded dev printed.
package devui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/pages"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
)

// LoginPath is the path of the login link: LoginPath?token=<token>.
const LoginPath = "/_aicoded/login"

const (
	cookieName = "__Host-aicoded_dev_ui"
	mailPrefix = "/_aicoded/mail/"
	// mailCSP is the policy of a caught mail's HTML part: a frame of the dev UI may show it, with
	// its inline styles and data: images, and it can run and load nothing.
	mailCSP  = "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:; frame-ancestors 'self'"
	frontCSP = "default-src 'none'; frame-ancestors 'none'"
	fixLogin = "open the login link that aicoded dev printed when it started"
)

var (
	errNoLogin  = errs.New("E-DEV-019", "the dev UI shows nothing before you log in", fixLogin)
	errBadLogin = errs.New("E-DEV-019", "this login link does not hold the dev UI's token", fixLogin)
)

var tokenText = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidToken reports whether s has the form of an access token: 64 lowercase hex characters.
func ValidToken(s string) bool { return tokenText.MatchString(s) }

// developer is the one viewer of the dev UI.
var developer = identity.Identity{Subject: "developer", Name: "Developer", Roles: []string{"developer"}}

var frontPages = template.Must(template.New("").Parse(`
{{define "refused"}}<!doctype html><title>Log in to aicoded dev</title><h1>Log in to aicoded dev</h1>
<p><b>{{.Code}}</b> {{.Msg}}</p><p>Fix: {{.Fix}}</p><p><a href="{{.Docs}}">{{.Docs}}</a></p>{{end}}
{{define "failed"}}<!doctype html><title>aicoded dev failed</title><h1>aicoded dev failed</h1>
<p>aicoded dev could not show this page.</p>{{end}}`))

// front serves the dev UI to the browser that logged in with the token.
type front struct {
	token   string
	cookie  string
	session *runner.Session
	backend deps.Backend
	pages   http.Handler
	out     io.Writer
}

// New returns the dev UI over b, for the access token token and the aicoded at executable.
// Its login link, LoginPath?token=<token>, gives the browser a cookie that holds a MAC of the
// token; any other request without that cookie is refused with E-DEV-019. It also serves the
// HTML part of a caught mail at /_aicoded/mail/<app>/<id>, which can run and load nothing.
// A page that panics answers 500 with a plain page, and the panic's value and stack go to out,
// the terminal. token must be 64 lowercase hex characters.
//
// The dev UI accepts any Host, so the caller must pass it only requests whose Host it checked,
// as the gateway does: localhost on its own port, which stops DNS rebinding.
func New(b deps.Backend, token, executable string, out io.Writer) http.Handler {
	if !ValidToken(token) {
		panic("devui: the token is not 64 lowercase hex characters")
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("aicoded dev ui"))
	return &front{
		token:   token,
		cookie:  hex.EncodeToString(mac.Sum(nil)),
		session: &runner.Session{App: "aicoded-dev", Env: "dev", Version: "dev", CSRFKey: key},
		backend: b,
		pages:   pages.NewHandler(deps.New(b, executable)),
		out:     out,
	}
}

func (f *front) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
			panic(v)
		}
		fmt.Fprintf(f.out, "aicoded dev: the dev UI failed on %s %q: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
		page(w, http.StatusInternalServerError, "failed", nil)
	}()
	if r.URL.Path == LoginPath {
		f.login(w, r)
		return
	}
	c, err := r.Cookie(cookieName)
	if err != nil || !hmac.Equal([]byte(c.Value), []byte(f.cookie)) {
		refuse(w, http.StatusUnauthorized, errNoLogin)
		return
	}
	if rest, ok := strings.CutPrefix(r.URL.Path, mailPrefix); ok {
		f.mail(w, r, rest)
		return
	}
	f.pages.ServeHTTP(w, r.WithContext(identity.With(runner.With(r.Context(), f.session), developer)))
}

// login sets the cookie when the query holds the token and nothing else, and sends the browser
// on to the apps.
func (f *front) login(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	given := q["token"]
	if len(q) != 1 || len(given) != 1 || subtle.ConstantTimeCompare([]byte(given[0]), []byte(f.token)) != 1 {
		refuse(w, http.StatusForbidden, errBadLogin)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: f.cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// mail serves the HTML part of the message of the path rest, <app>/<id>.
func (f *front) mail(w http.ResponseWriter, r *http.Request, rest string) {
	app, id, ok := strings.Cut(rest, "/")
	if !ok || app == "" || id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	m, err := f.backend.MailGet(r.Context(), app, id)
	if err != nil || m.HTML == "" {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", mailCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	_, _ = io.WriteString(w, m.HTML)
}

// refuse answers status with the page of the E-DEV-019 error e.
func refuse(w http.ResponseWriter, status int, e *errs.Error) {
	page(w, status, "refused", struct{ Code, Msg, Fix, Docs string }{e.Code, e.Msg, e.Fix, errs.DocsBase + e.Code})
}

func page(w http.ResponseWriter, status int, name string, data any) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", frontCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_ = frontPages.ExecuteTemplate(w, name, data)
}
