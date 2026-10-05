package web

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/web/form"
)

const (
	assetsPrefix  = "/_aicoded/assets/"
	liveSuffix    = "/__ws"
	maxBodyBytes  = 32 << 20
	maxFormMemory = 8 << 20
)

var (
	errNoRunner = errs.New("E-WEB-001", "pages are served without the app's runner",
		"serve pages through app.Main(app.Options{Handler: pages.NewHandler(...)})")
	errNoAccessRule = errs.New("E-WEB-003", "no route on this path has an access rule",
		`add <ssr:access role="…"/> to the page or a parent and run aicoded generate`)
)

// Options configure the pages handler.
type Options struct {
	// Assets holds the scripts, styles and images served under /_aicoded/assets/.
	Assets fs.FS
}

type node struct {
	route Route
	// param is the name of the parameter folder below this node, folder that folder's name,
	// such as "id" and "n_id", and numeric whether it takes only a decimal integer.
	param    string
	folder   string
	numeric  bool
	children map[string]*node
}

type mux struct {
	root   *node
	assets *assetServer
	live   *liveConns
}

// New returns the handler for an app's pages. routes maps paths such as "/notes/n_id" to
// their generated routes; generated code calls it. New panics if a folder has two parameter
// folders.
//
// Generated code only.
func New(routes map[string]Route, opts Options) http.Handler {
	m := &mux{root: &node{children: map[string]*node{}}, assets: newAssetServer(opts.Assets), live: newLiveConns()}
	for p, rt := range routes {
		n := m.root
		segs := segments(p)
		for i, seg := range segs {
			child := n.children[seg]
			if child == nil {
				child = &node{children: map[string]*node{}}
				n.children[seg] = child
			}
			if name, numeric, ok := paramFolder(seg); ok {
				if n.folder != "" && n.folder != seg {
					a, b := min(n.folder, seg), max(n.folder, seg)
					panic(errs.New("E-WEB-006",
						fmt.Sprintf("/%s has two parameter folders, %s and %s", strings.Join(segs[:i], "/"), a, b),
						"keep one parameter folder per folder; run aicoded generate, which refuses this layout"))
				}
				n.param, n.folder, n.numeric = name, seg, numeric
			}
			n = child
		}
		n.route = rt
	}
	return m
}

func segments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// paramFolder returns the parameter name of a folder named s_name or n_name, and whether it
// takes only a decimal integer (n_).
func paramFolder(seg string) (name string, numeric, ok bool) {
	if len(seg) > 2 && (seg[:2] == "s_" || seg[:2] == "n_") {
		return seg[2:], seg[0] == 'n', true
	}
	return "", false, false
}

// isInt reports whether seg is digits only and fits int64.
func isInt(seg string) bool {
	for i := 0; i < len(seg); i++ {
		if seg[i] < '0' || seg[i] > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(seg, 10, 64)
	return err == nil
}

// match returns the routes on the path to p, root first, the URL parameters, and whether the
// last route is a layout with routes below it, which opening sends to its default page.
func (m *mux) match(p string) (routes []Route, params map[string]string, parent, ok bool) {
	n := m.root
	params = map[string]string{}
	if n.route != nil {
		routes = append(routes, n.route)
	}
	for _, seg := range segments(p) {
		if seg == "" {
			return nil, nil, false, false
		}
		var child *node
		if _, _, isParam := paramFolder(seg); !isParam {
			child = n.children[seg]
		}
		if child == nil && n.folder != "" && (!n.numeric || isInt(seg)) {
			params[n.param] = seg
			child = n.children[n.folder]
		}
		if child == nil {
			return nil, nil, false, false
		}
		n = child
		if n.route != nil {
			routes = append(routes, n.route)
		}
	}
	if n.route == nil {
		return nil, nil, false, false
	}
	return routes, params, n.route.Layout() && len(n.children) > 0, true
}

func (m *mux) ServeHTTP(w http.ResponseWriter, hr *http.Request) {
	h := w.Header()
	setHeaders(h)
	h.Set("Cache-Control", "no-store")
	if name, ok := strings.CutPrefix(hr.URL.Path, assetsPrefix); ok {
		m.assets.serve(w, hr, name)
		return
	}
	s := runner.From(hr.Context())
	if s == nil || len(s.CSRFKey) == 0 {
		fail(w, hr, errNoRunner)
		return
	}
	v := identity.From(hr.Context())
	if !v.Authenticated() {
		errorPage(w, http.StatusUnauthorized, "Sign in through the company login.")
		return
	}
	p, live := strings.CutSuffix(hr.URL.Path, liveSuffix)
	routes, params, parent, ok := m.match(p)
	if !ok {
		fail(w, hr, NotFound())
		return
	}
	r := &Request{Request: hr, params: params, session: s, viewer: v}
	switch {
	case live:
		r.Request = pageRequest(hr, cmp.Or(p, "/"))
		m.serveLive(w, r, routes, parent)
	case hr.Method == http.MethodGet, hr.Method == http.MethodHead, hr.Method == http.MethodPost:
		m.servePage(w, r, routes, parent)
	default:
		h.Set("Allow", "GET, HEAD, POST")
		fail(w, hr, Error(http.StatusMethodNotAllowed, "This page does not take this kind of request."))
	}
}

// pageRequest returns a copy of the live connection's request hr whose URL names the page at
// p, so the page's hooks see the path they see when the page loads.
func pageRequest(hr *http.Request, p string) *http.Request {
	u := *hr.URL
	u.Path = p
	if raw, ok := strings.CutSuffix(u.RawPath, liveSuffix); ok {
		u.RawPath = raw
	} else {
		u.RawPath = ""
	}
	pr := hr.WithContext(hr.Context())
	pr.URL = &u
	pr.RequestURI = u.RequestURI()
	return pr
}

func (m *mux) servePage(w http.ResponseWriter, r *Request, routes []Route, parent bool) {
	ctx := r.Context()
	post := r.Method == http.MethodPost
	if post && crossSite(r.Request, false) {
		slog.WarnContext(ctx, "cross-site form refused", "origin", r.Header.Get("Origin"), "host", r.Host)
		fail(w, r.Request, Forbidden())
		return
	}
	states, err := admit(ctx, r, routes)
	if err != nil {
		fail(w, r.Request, err)
		return
	}
	if parent {
		if post {
			w.Header().Set("Allow", "GET, HEAD")
			fail(w, r.Request, Error(http.StatusMethodNotAllowed, "This page does not take this kind of request."))
			return
		}
		sub, err := states[len(states)-1].DefaultRoute(ctx, r)
		if err != nil {
			fail(w, r.Request, err)
			return
		}
		self := path.Join("/", r.URL.Path)
		target := url.URL{Path: path.Join(self, sub)}
		if target.Path == self {
			fail(w, r.Request, NotFound())
			return
		}
		http.Redirect(w, r.Request, target.EscapedPath(), http.StatusFound)
		return
	}
	states = page(routes, states)
	var formID string
	if post {
		formID, err = readForm(w, r)
		if mf := r.MultipartForm; mf != nil {
			defer func() { _ = mf.RemoveAll() }()
		}
		if err != nil {
			fail(w, r.Request, err)
			return
		}
	}
	hw := headerWriter{h: w.Header()}
	for _, s := range states {
		if err := s.InitForms(ctx, r, hw); err != nil {
			fail(w, r.Request, err)
			return
		}
	}
	if post {
		if err := submit(ctx, r, hw, states, formID); err != nil {
			fail(w, r.Request, err)
			return
		}
	}
	for _, s := range states {
		if err := s.Data(ctx, r, hw); err != nil {
			fail(w, r.Request, err)
			return
		}
	}
	render(w, r, states)
}

// admit checks the access rules and then the guards of every route on the path, root first,
// and returns the routes' states. A path with no rule at all is denied.
func admit(ctx context.Context, r *Request, routes []Route) ([]RouteState, error) {
	ruled := false
	for _, rt := range routes {
		a := rt.Access()
		if len(a.Roles) == 0 {
			continue
		}
		ruled = true
		if !a.allows(r.viewer) {
			return nil, Forbidden()
		}
	}
	if !ruled {
		slog.ErrorContext(ctx, "page refused", "err", errNoAccessRule)
		return nil, Forbidden()
	}
	states := make([]RouteState, len(routes))
	for i, rt := range routes {
		states[i] = rt.NewState()
	}
	for _, s := range states {
		if err := s.Guard(ctx, r); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// page returns the states of the routes that render the page at the end of the path: every
// layout on the path and the page itself. The other routes on the path are gates: admit ran
// their access rules and Guards, but their forms, Data, live values, calls and markup belong
// to their own pages.
func page(routes []Route, states []RouteState) []RouteState {
	out := make([]RouteState, 0, len(states))
	for i, rt := range routes {
		if i == len(routes)-1 || rt.Layout() {
			out = append(out, states[i])
		}
	}
	return out
}

// readForm limits and parses the body, checks the form token and returns the posted form's id.
func readForm(w http.ResponseWriter, r *Request) (string, error) {
	if r.ContentLength > maxBodyBytes {
		return "", Error(http.StatusRequestEntityTooLarge, "The form is too large.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var err error
	if form.IsMultipart(r.Request) {
		err = r.ParseMultipartForm(maxFormMemory)
	} else {
		err = r.ParseForm()
	}
	if err != nil {
		if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
			return "", Error(http.StatusRequestEntityTooLarge, "The form is too large.")
		}
		return "", Error(http.StatusBadRequest, "The form could not be read.")
	}
	if !verifyCSRF(r.session.CSRFKey, r.session.App, r.viewer.Subject, r.PostForm.Get(CSRFField), time.Now()) {
		slog.WarnContext(r.Context(), "form refused: missing or invalid form token")
		return "", Forbidden()
	}
	return r.PostForm.Get(FormField), nil
}

func submit(ctx context.Context, r *Request, w ResponseWriter, states []RouteState, id string) error {
	for _, s := range states {
		if ok, err := s.SubmitForm(ctx, r, w, id); ok || err != nil {
			return err
		}
	}
	return Error(http.StatusBadRequest, "This form is not on this page.")
}

// render writes the page into a buffer first, so a failing template never sends half a page.
func render(w http.ResponseWriter, r *Request, states []RouteState) {
	for i := 0; i+1 < len(states); i++ {
		states[i].frame().child = states[i+1]
	}
	var b bytes.Buffer
	if err := states[0].Write(&b); err != nil {
		fail(w, r.Request, err)
		return
	}
	h := w.Header()
	setHeaders(h)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b.Bytes())
}
