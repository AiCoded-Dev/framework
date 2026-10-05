package web

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
)

var testSession = &runner.Session{App: "site", CSRFKey: bytes.Repeat([]byte{7}, 32)}

// withViewer puts the runner session and a viewer in the context, as app.Run and the
// viewer check do in a real app.
func withViewer(ctx context.Context, subject string, roles ...string) context.Context {
	return identity.With(runner.With(ctx, testSession), identity.Identity{Subject: subject, Name: subject, Roles: roles})
}

func request(method, target, subject string, form url.Values, roles ...string) *http.Request {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequestWithContext(context.Background(), method, "http://site.test"+target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return r.WithContext(withViewer(r.Context(), subject, roles...))
}

func token(subject string) string {
	return mintCSRF(testSession.CSRFKey, testSession.App, subject, time.Now())
}

// fakeRoute records the hooks the pipeline calls, in order, in a shared log.
type fakeRoute struct {
	name     string
	access   Access
	log      *callLog
	guard    error
	guardFn  func() error // when set, replaces guard
	data     error
	invalid  bool
	children string
	param    string   // URL parameter the guard logs, if set
	paths    bool     // Guard and Data log the request's path and URI
	layout   bool     // the template has <ssr:content/>
	assets   []string // tags <ssr:assets/> writes for this route
}

type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, s)
}

func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

func (f *fakeRoute) Access() Access { return f.access }
func (f *fakeRoute) Layout() bool   { return f.layout }
func (f *fakeRoute) NewState() RouteState {
	s := &fakeState{route: f}
	s.SetAssets(f.assets)
	return s
}

type fakeState struct {
	Frame
	route *fakeRoute
}

func (s *fakeState) Guard(_ context.Context, r *Request) error {
	s.route.log.add(s.route.name + ".guard")
	if s.route.param != "" {
		s.route.log.add(s.route.name + ".param=" + r.URLParam(s.route.param))
	}
	s.logPath("guard", r)
	if s.route.guardFn != nil {
		return s.route.guardFn()
	}
	return s.route.guard
}

func (s *fakeState) InitForms(context.Context, *Request, ResponseWriter) error {
	s.route.log.add(s.route.name + ".init")
	return nil
}

func (s *fakeState) SubmitForm(_ context.Context, _ *Request, _ ResponseWriter, id string) (bool, error) {
	if id != s.route.name+".form" {
		return false, nil
	}
	if s.route.invalid {
		s.route.log.add(s.route.name + ".invalid")
		return true, nil
	}
	s.route.log.add(s.route.name + ".process")
	return true, nil
}

func (s *fakeState) logPath(hook string, r *Request) {
	if s.route.paths {
		s.route.log.add(s.route.name + "." + hook + " " + r.URL.Path + " " + r.RequestURI)
	}
}

func (s *fakeState) Data(_ context.Context, r *Request, w ResponseWriter) error {
	s.route.log.add(s.route.name + ".data")
	s.logPath("data", r)
	w.Header().Set("Content-Security-Policy", "default-src *")
	w.Header().Set("X-Route", s.route.name)
	return s.route.data
}

func (s *fakeState) DefaultRoute(context.Context, *Request) (string, error) {
	return s.route.children, nil
}

func (s *fakeState) Write(w io.Writer) error {
	if s.route.name == "broken" {
		return errors.New("render failed")
	}
	_, _ = io.WriteString(w, "<"+s.route.name+">")
	if err := s.WriteAssets(w); err != nil {
		return err
	}
	if err := s.WriteChild(w); err != nil {
		return err
	}
	_, err := io.WriteString(w, "</"+s.route.name+">")
	return err
}
