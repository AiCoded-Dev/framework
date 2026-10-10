package l7runner

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/viewer"
)

// Protocol is the version of the control API.
const Protocol = 1

const (
	// otherApp is the audience of a token of kind audience.
	otherApp = "l7-other-app"
	// expiredAge is how long ago a token of kind expired was issued: it expired a lifetime ago.
	expiredAge = 2 * viewer.MaxLifetime
	maxSubject = 64
	maxRoles   = 64
	maxRequest = 16 << 10
)

// roleName is the shape of a role in an access rule.
var roleName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// control serves the control API: the app's state, viewer tokens and the app's spans.
type control struct {
	app    string
	other  string // the audience of a token for another app
	roles  []string
	signer *dev.Signer
	runner *dev.Runner
}

func newControl(m manifest.Manifest, signer *dev.Signer, r *dev.Runner) *control {
	other := otherApp
	if m.App == otherApp {
		other = "l7-another-app"
	}
	return &control{app: m.App, other: other, roles: m.Roles(), signer: signer, runner: r}
}

// newControlServer returns an HTTP/1.1 server of c that logs nothing.
func newControlServer(c *control) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", c.status)
	mux.HandleFunc("POST /v1/token", c.token)
	mux.HandleFunc("GET /v1/spans", c.spans)
	return &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
}

type statusResponse struct {
	Protocol int    `json:"protocol"`
	App      string `json:"app"`
	State    string `json:"state"`
}

func (c *control) status(w http.ResponseWriter, _ *http.Request) {
	state := "waiting"
	select {
	case <-c.runner.Ready():
		state = "ready"
	default:
	}
	reply(w, http.StatusOK, statusResponse{Protocol: Protocol, App: c.app, State: state})
}

type tokenRequest struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
	Kind    string   `json:"kind"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (c *control) token(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequest))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || !errors.Is(dec.Decode(&json.RawMessage{}), io.EOF) {
		reply(w, http.StatusBadRequest, errorResponse{"the body must be one JSON object with subject, roles and kind"})
		return
	}
	token, err := c.mint(req)
	if err != nil {
		reply(w, http.StatusBadRequest, errorResponse{err.Error()})
		return
	}
	reply(w, http.StatusOK, tokenResponse{token})
}

// mint returns a viewer token of req's kind for its subject with its roles, or why it cannot.
// Its errors never quote a value of req.
func (c *control) mint(req tokenRequest) (string, error) {
	switch {
	case req.Subject == "" || len(req.Subject) > maxSubject:
		return "", errors.New("the subject must be 1 to 64 bytes long")
	case len(req.Roles) > maxRoles:
		return "", errors.New("a token holds at most 64 roles")
	}
	for _, role := range req.Roles {
		if !roleName.MatchString(role) || !slices.Contains(c.roles, role) {
			return "", errors.New("a role is not one the app's permission list names")
		}
	}
	p, now := devconfig.Persona{Name: req.Subject, Roles: req.Roles}, time.Now()
	switch req.Kind {
	case "valid":
		return c.signer.Mint(p, c.app, now)
	case "expired":
		return c.signer.Mint(p, c.app, now.Add(-expiredAge))
	case "audience":
		return c.signer.Mint(p, c.other, now)
	}
	return "", errors.New("the kind must be valid, expired or audience")
}

type spansResponse struct {
	Spans   []span `json:"spans"`
	Dropped uint64 `json:"dropped"`
}

// span is a span the app exported. The runner protocol carries no events, so Events is empty;
// Status is the span's error, empty when it did not fail.
type span struct {
	Name       string            `json:"name"`
	TraceID    string            `json:"trace_id"`
	Attributes map[string]string `json:"attributes"`
	Events     []event           `json:"events"`
	Status     string            `json:"status"`
}

type event struct {
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes"`
}

func (c *control) spans(w http.ResponseWriter, _ *http.Request) {
	kept, dropped := c.runner.Spans()
	out := spansResponse{Spans: make([]span, 0, len(kept)), Dropped: dropped}
	for _, sp := range kept {
		attrs := sp.Attrs
		if attrs == nil {
			attrs = map[string]string{}
		}
		out.Spans = append(out.Spans, span{Name: sp.Name, TraceID: sp.TraceID, Attributes: attrs, Events: []event{}, Status: sp.Error})
	}
	reply(w, http.StatusOK, out)
}

// reply writes v as JSON with status code.
func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
