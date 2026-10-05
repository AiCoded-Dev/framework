package dev

import (
	"context"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto/viewer"
)

const (
	personaCookie = "aicoded_dev_persona"
	personaPath   = "/_aicoded/persona"
	pageCSP       = "default-src 'none'; form-action 'self'; frame-ancestors 'none'"
)

var pages = template.Must(template.New("").Parse(`
{{define "persona"}}<!doctype html><title>View as</title><h1>View the app as</h1>
{{range .}}<form method="post"><input type="hidden" name="name" value="{{.Name}}"><button>{{.Name}}</button></form>{{end}}{{end}}
{{define "starting"}}<!doctype html><title>{{.}} is starting</title><h1>{{.}} is starting</h1>
<p>aicoded dev is building the app. This page reloads by itself.</p>{{end}}
{{define "stopped"}}<!doctype html><title>{{.App}} is stopped</title><h1>{{.App}} is stopped</h1>
<p>{{.App}} is stopped. Start it on the dev UI at <a href="{{.UI}}">{{.UI}}</a>.</p>{{end}}
{{define "manual"}}<!doctype html><title>{{.App}} is not running</title><h1>{{.App}} is not running</h1>
<p>{{.App}} runs by hand and is not running now. Start it with:</p><pre>{{.Command}}</pre>{{end}}
{{define "unanswered"}}<!doctype html><title>{{.}} did not answer</title><h1>{{.}} did not answer</h1>
<p>aicoded dev could not reach the app. Its log lines say why.</p>{{end}}
{{define "failed"}}<!doctype html><title>{{.App}} is not running</title><h1>{{.App}} is not running</h1>
<p>aicoded dev could not build or start the app:</p>
<ul>{{range .Problems}}<li><p>{{with .Code}}<b>{{.}}</b> {{end}}{{with .Pos}}at <code>{{.}}</code>{{end}}</p><pre>{{.Message}}</pre>
{{with .Fix}}<p>Fix: {{.}}</p>{{end}}{{with .Docs}}<p><a href="{{.}}">{{.}}</a></p>{{end}}</li>{{end}}</ul>{{end}}`))

// Gateway fronts apps at http://<app>.localhost:<port>, and serves the dev UI at
// http://localhost:<port>. It stands in for the company login: every request reaches the app
// with a fresh viewer token for the persona chosen on /_aicoded/persona, and any token the
// client sent is replaced. An app that is starting, failed or stopped gets a page that says so
// instead.
type Gateway struct {
	port     string
	signer   *Signer
	ui       http.Handler
	mu       sync.RWMutex
	personas []devconfig.Persona
	apps     map[string]*gatewayApp
}

// gatewayApp is what the gateway serves at an app's host: the app through proxy, the starting
// or stopped page, or, with none of them, the failed page with problems.
type gatewayApp struct {
	proxy     *httputil.ReverseProxy
	transport *http.Transport
	starting  bool
	stopped   bool
	problems  []problem.Problem
}

type tokenKey struct{}

// NewGateway returns a gateway for apps served on port, which serves ui at the bare host. With
// no personas, a viewer is the one persona no-roles.
func NewGateway(port int, personas []devconfig.Persona, signer *Signer, ui http.Handler) *Gateway {
	g := &Gateway{port: strconv.Itoa(port), signer: signer, ui: ui, apps: map[string]*gatewayApp{}}
	g.setPersonas(personas)
	return g
}

// setPersonas sets the personas a viewer can choose from; with none, the one persona no-roles.
func (g *Gateway) setPersonas(personas []devconfig.Persona) {
	if len(personas) == 0 {
		personas = []devconfig.Persona{{Name: "no-roles"}}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.personas = personas
}

// Personas returns the personas a viewer can choose from.
func (g *Gateway) Personas() []devconfig.Persona {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return slices.Clone(g.personas)
}

// Add routes <name>.localhost to the app listening on socketPath. When the app does not answer,
// the host answers 502 with a page, and st keeps and prints a runner ERROR entry that says why.
// The proxy's other errors, such as an answer cut off on the way, st keeps as runner WARN
// entries without printing them.
func (g *Gateway) Add(name, socketPath string, st *store) {
	g.put(name, newProxy(socketPath, log.New(entryLog{st}, "gateway: ", 0), func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Context().Err() == nil {
			st.add(sourceRunner, "ERROR", "gateway: the app did not answer: "+err.Error(), true)
		}
		page(w, http.StatusBadGateway, "unanswered", name)
	}))
}

// AddManual routes <name>.localhost to the app the developer runs by hand, on socketPath. While
// the app does not answer, the host answers 503 with a page that shows command.
func (g *Gateway) AddManual(name, socketPath, command string) {
	g.put(name, newProxy(socketPath, log.New(io.Discard, "", 0), func(w http.ResponseWriter, _ *http.Request, _ error) {
		page(w, http.StatusServiceUnavailable, "manual", struct{ App, Command string }{name, command})
	}))
}

// Fail answers <name>.localhost with a 503 page that lists ps.
func (g *Gateway) Fail(name string, ps []problem.Problem) {
	g.put(name, &gatewayApp{problems: slices.Clone(ps)})
}

// Starting answers <name>.localhost with a 503 page that reloads itself every second.
func (g *Gateway) Starting(name string) { g.put(name, &gatewayApp{starting: true}) }

// Stopped answers <name>.localhost with a 503 page that says the app is stopped and where to
// start it.
func (g *Gateway) Stopped(name string) { g.put(name, &gatewayApp{stopped: true}) }

// Remove answers <name>.localhost with 404, as for a name no app has.
func (g *Gateway) Remove(name string) { g.put(name, nil) }

// put serves a at the host of name, or nothing when a is nil, and closes the idle connections
// to the app served there before.
func (g *Gateway) put(name string, a *gatewayApp) {
	g.mu.Lock()
	old := g.apps[name]
	if a == nil {
		delete(g.apps, name)
	} else {
		g.apps[name] = a
	}
	g.mu.Unlock()
	if old != nil && old.transport != nil {
		old.transport.CloseIdleConnections()
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name, ok := g.route(r.Host)
	if !ok {
		http.Error(w, "unknown host; open http://localhost:"+g.port+"/", http.StatusMisdirectedRequest)
		return
	}
	if name == "" {
		g.ui.ServeHTTP(w, r)
		return
	}
	g.mu.RLock()
	a := g.apps[name]
	g.mu.RUnlock()
	if a == nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == personaPath {
		g.persona(w, r)
		return
	}
	switch {
	case a.starting:
		w.Header().Set("Refresh", "1")
		page(w, http.StatusServiceUnavailable, "starting", name)
	case a.stopped:
		page(w, http.StatusServiceUnavailable, "stopped", struct{ App, UI string }{name, "http://localhost:" + g.port + "/apps/" + name})
	case a.proxy == nil:
		page(w, http.StatusServiceUnavailable, "failed", struct {
			App      string
			Problems []problem.Problem
		}{name, a.problems})
	default:
		token, err := g.signer.Mint(g.personaFor(r), name, time.Now())
		if err != nil {
			http.Error(w, "cannot mint a viewer token", http.StatusInternalServerError)
			return
		}
		a.proxy.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tokenKey{}, token)))
	}
}

// route maps a Host header to an app name; "" is the dev UI. Only localhost
// names on the gateway's port are accepted, which also stops DNS rebinding. A Host without
// a port means port 80.
func (g *Gateway) route(hostport string) (string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil && !strings.Contains(hostport, ":") {
		host, port, err = hostport, "80", nil
	}
	if err != nil || port != g.port {
		return "", false
	}
	host = strings.ToLower(host)
	if host == "localhost" {
		return "", true
	}
	name, ok := strings.CutSuffix(host, ".localhost")
	if !ok || name == "" || strings.Contains(name, ".") {
		return "", false
	}
	return name, true
}

func (g *Gateway) persona(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	personas := g.personas
	g.mu.RUnlock()
	switch r.Method {
	case http.MethodGet:
		page(w, http.StatusOK, "persona", personas)
	case http.MethodPost:
		if r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		name := r.PostFormValue("name")
		if !slices.ContainsFunc(personas, func(p devconfig.Persona) bool { return p.Name == name }) {
			http.Error(w, "unknown persona", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: personaCookie, Value: name, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (g *Gateway) personaFor(r *http.Request) devconfig.Persona {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if c, err := r.Cookie(personaCookie); err == nil {
		for _, p := range g.personas {
			if p.Name == c.Value {
				return p
			}
		}
	}
	return g.personas[0]
}

func page(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = pages.ExecuteTemplate(w, name, data)
}

// newProxy returns the entry of the app on socketPath. When the app does not answer, unanswered
// writes the answer; the proxy's other errors go to errLog.
func newProxy(socketPath string, errLog *log.Logger, unanswered func(http.ResponseWriter, *http.Request, error)) *gatewayApp {
	t := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &gatewayApp{transport: t, proxy: &httputil.ReverseProxy{
		Transport:    t,
		ErrorLog:     errLog,
		ErrorHandler: unanswered,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: "http", Host: "app"})
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
			token, _ := pr.In.Context().Value(tokenKey{}).(string)
			pr.Out.Header.Set(viewer.Header, token)
			sc := tracectx.SpanContext{TraceID: tracectx.NewTraceID(), SpanID: tracectx.NewSpanID(), Sampled: true}
			pr.Out.Header.Set("traceparent", sc.Traceparent())
		},
	}}
}

// entryLog is the output of a logger whose lines st keeps as runner WARN entries, unprinted.
type entryLog struct{ st *store }

func (e entryLog) Write(p []byte) (int, error) {
	e.st.add(sourceRunner, "WARN", strings.TrimSuffix(string(p), "\n"), false)
	return len(p), nil
}
