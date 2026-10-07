package platformtest

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
)

// maxBundle is the size of the largest bundle the platform takes.
const maxBundle = 32 << 20

var (
	appName   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
	commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Publish is a publish as /v1/publishes/get answers it.
type Publish struct {
	ID               string    `json:"id"`
	App              string    `json:"app"`
	SHA              string    `json:"sha"`
	Summary          string    `json:"summary"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason,omitempty"`
	Created          string    `json:"created"`
	Ended            string    `json:"ended,omitempty"`
	Parent           *Parent   `json:"parent,omitempty"`
	HistoryRewritten bool      `json:"history_rewritten"`
	Checks           string    `json:"checks"`
	Steps            []Step    `json:"steps"`
	Problems         []Problem `json:"problems"`
	Record           int       `json:"record,omitempty"`
}

// Parent is the publish before another.
type Parent struct {
	ID  string `json:"id"`
	SHA string `json:"sha"`
}

// Step is a step of the delivery pipeline and its outcome.
type Step struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
}

// Problem is a problem the delivery pipeline found.
type Problem struct {
	Code    string `json:"code"`
	Pos     string `json:"pos"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
	Docs    string `json:"docs"`
}

// Steps returns the four steps, each with its outcome.
func Steps(checkout, modules, check, tests string) []Step {
	return []Step{{"checkout", checkout}, {"modules", modules}, {"check", check}, {"tests", tests}}
}

// Passed is the answer about a publish that passed.
func Passed() Publish {
	return Publish{Status: "passed", Checks: "partial", Steps: Steps("passed", "passed", "passed", "passed"), Record: 1}
}

// Failed is the answer about a publish that failed with n problems, each with a message of size
// letters.
func Failed(n, size int) Publish {
	problems := make([]Problem, n)
	for i := range problems {
		problems[i] = Problem{Code: "E-CHK-004", Pos: fmt.Sprintf("demo_test.go:%d", i+1), Message: strings.Repeat("x", size),
			Fix: "fix the code or the test until go test passes"}
	}
	return Publish{Status: "failed", Checks: "partial", Steps: Steps("passed", "passed", "passed", "failed"), Problems: problems}
}

// Upload is a publish the fake took.
type Upload struct {
	App, SHA, Summary string
	Bundle            []byte
}

// builder is the state of the fake builder API.
type builder struct {
	apps     []*fakeApp
	pubs     []*fakePublish
	answers  []Publish
	refused  *refusal
	newApps  int
	uploads  []Upload
	requests map[string]int
}

type fakeApp struct {
	id       int
	name     string
	owner    string
	archived bool
	base     string
	stale    []string
	created  time.Time
}

type fakePublish struct {
	id, sha, summary string
	app              *fakeApp
	created          time.Time
	gets             int
}

type refusal struct {
	status          int
	errorCode, code string
}

// uploadMeta is the meta part of an upload.
type uploadMeta struct {
	App     string `json:"app"`
	SHA     string `json:"sha"`
	Summary string `json:"summary"`
}

func newBuilder() builder {
	return builder{newApps: 10, answers: []Publish{Passed()}, requests: map[string]int{}}
}

// SignIn keeps a sign-in to the fake in the test's home, as aicoded login does.
func (p *Platform) SignIn(t testing.TB) {
	t.Helper()
	p.mu.Lock()
	access, refresh := p.issue()
	expires := p.access[access].expires
	p.mu.Unlock()
	if err := platform.Save(p.URL, platform.Credentials{Org: Org, AccessToken: access, RefreshToken: refresh, Expires: expires}); err != nil {
		t.Fatal(err)
	}
}

// AddApp adds the app name, which the person with the email owner owns.
func (p *Platform) AddApp(name, owner string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.apps = append(p.apps, &fakeApp{id: len(p.apps) + 1, name: name, owner: owner, created: time.Now()})
}

// Archive archives the app name.
func (p *Platform) Archive(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.app(name).archived = true
}

// StaleBase makes the next answers of /v1/apps/get about the app name give the bases, one
// each, in place of its base.
func (p *Platform) StaleBase(name string, bases ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.app(name).stale = bases
}

// NewAppLimit sets how many apps a person may create a day; 10 by default.
func (p *Platform) NewAppLimit(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.newApps = n
}

// Answers sets the answers of /v1/publishes/get: about each publish, the first get gets the
// first answer, the next the next, and the last answer repeats. Each answer gets the publish's
// id, app, commit, summary and time. By default every publish has Passed.
func (p *Platform) Answers(answers ...Publish) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answers = answers
}

// RefuseUpload refuses the next upload, once it is read, with status, the error errorCode and,
// unless it is "", the framework's code.
func (p *Platform) RefuseUpload(status int, errorCode, code string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refused = &refusal{status, errorCode, code}
}

// Uploads returns the publishes the fake took, oldest first.
func (p *Platform) Uploads() []Upload {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.uploads)
}

// Requests returns how many requests the builder API path, such as /v1/publishes/create, got.
func (p *Platform) Requests(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[path]
}

// scopes returns the scopes of a token issued now. The caller holds p.mu.
func (p *Platform) scopes() []string {
	scopes := []string{"app:create"}
	for _, a := range p.apps {
		if a.owner == Email && !a.archived {
			scopes = append(scopes, fmt.Sprintf("app:%d:release:request", a.id), fmt.Sprintf("app:%d:status", a.id))
		}
	}
	return scopes
}

// app returns the app name, or nil. The caller holds p.mu.
func (p *Platform) app(name string) *fakeApp {
	for _, a := range p.apps {
		if a.name == name {
			return a
		}
	}
	return nil
}

// answer returns the answer about pub; next moves it on to the next answer. The caller holds
// p.mu.
func (p *Platform) answer(pub *fakePublish, next bool) Publish {
	out := p.answers[min(pub.gets, len(p.answers)-1)]
	if next {
		pub.gets++
	}
	out.ID, out.App, out.SHA, out.Summary = pub.id, pub.app.name, pub.sha, pub.summary
	out.Created = pub.created.UTC().Format(time.RFC3339)
	if out.Steps == nil {
		out.Steps = []Step{}
	}
	if out.Problems == nil {
		out.Problems = []Problem{}
	}
	return out
}

// builderCall checks the access token of a request to the builder API, counts the request and
// reads its JSON body into in, refusing unknown fields. It answers the request and returns
// false when it is refused. The caller holds p.mu.
func (p *Platform) builderCall(w http.ResponseWriter, r *http.Request, in any) (session, bool) {
	p.requests[r.URL.Path]++
	s, ok := p.session(w, r)
	if !ok {
		return session{}, false
	}
	if !slices.Contains(s.scopes, "app:create") {
		apiError(w, http.StatusForbidden, "insufficient_scope", "", "the token lacks the scope app:create")
		return session{}, false
	}
	if in == nil {
		return s, true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(in) != nil || dec.Decode(&struct{}{}) != io.EOF {
		apiError(w, http.StatusBadRequest, "invalid_request", "", "the body is not valid")
		return session{}, false
	}
	return s, true
}

func (p *Platform) getApp(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var in struct {
		Name string `json:"name"`
	}
	s, ok := p.builderCall(w, r, &in)
	if !ok {
		return
	}
	a := p.app(in.Name)
	switch {
	case a == nil:
		apiError(w, http.StatusNotFound, "not_found", "", "no app has that name")
		return
	case !may(s, a, "status"):
		notYours(w, a)
		return
	}
	base := a.base
	if len(a.stale) > 0 {
		base, a.stale = a.stale[0], a.stale[1:]
	}
	var last any
	for _, pub := range p.pubs {
		if pub.app == a {
			last = map[string]string{"id": pub.id, "sha": pub.sha, "status": p.answer(pub, false).Status}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": a.id, "name": a.name, "archived": a.archived, "base": base, "last": last})
}

func (p *Platform) getPublish(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var in struct {
		ID string `json:"id"`
	}
	s, ok := p.builderCall(w, r, &in)
	if !ok {
		return
	}
	for _, pub := range p.pubs {
		if pub.id != in.ID {
			continue
		}
		if !may(s, pub.app, "status") {
			notYours(w, pub.app)
			return
		}
		writeJSON(w, http.StatusOK, p.answer(pub, true))
		return
	}
	apiError(w, http.StatusNotFound, "not_found", "E-PUB-013", "no publish has that id")
}

// may reports whether the session may use the app with the scope app:<id>:<scope>: the person owns
// it, and the token holds the scope.
func may(s session, a *fakeApp, scope string) bool {
	return a.owner == Email && slices.Contains(s.scopes, fmt.Sprintf("app:%d:%s", a.id, scope))
}

// notYours answers as the platform does for an app the token may not use: E-PUB-006 when the
// person owns the app and it is archived, and E-PUB-005 otherwise.
func notYours(w http.ResponseWriter, a *fakeApp) {
	if a.owner == Email && a.archived {
		apiError(w, http.StatusForbidden, "insufficient_scope", "E-PUB-006", "the app is archived and takes no more publishes")
		return
	}
	apiError(w, http.StatusForbidden, "insufficient_scope", "E-PUB-005", "the app belongs to another builder, or your sign-in does not carry it yet")
}

// createPublish takes a publish as the platform does: the parts meta, then bundle, and nothing
// else; a v2 bundle with one ref, the claimed commit, and no prerequisite or the app's base; and
// the refusals of the builder API, in the platform's order. The publish it takes is its app's
// base at once, as if the worker had verified its bundle.
func (p *Platform) createPublish(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s, ok := p.builderCall(w, r, nil)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBundle+64<<10)
	var meta uploadMeta
	bundle, err := readUpload(r, &meta)
	var tooLarge *http.MaxBytesError
	switch f := p.refused; {
	case errors.As(err, &tooLarge) || len(bundle) > maxBundle:
		apiError(w, http.StatusRequestEntityTooLarge, "too_large", "E-PUB-009", "the upload is larger than 32 MiB and 64 KiB")
		return
	case err != nil:
		apiError(w, http.StatusBadRequest, "invalid_request", "", err.Error())
		return
	case f != nil:
		p.refused = nil
		apiError(w, f.status, f.errorCode, f.code, "refused as the test asked")
		return
	}
	prerequisites, refs, ok := bundleHeader(bundle)
	if !ok {
		apiError(w, http.StatusBadRequest, "invalid_request", "E-PUB-010", "the bundle's header is not a v2 git bundle's")
		return
	}
	a := p.app(meta.App)
	created := a == nil
	switch {
	case !created && a.owner == Email && a.archived:
		apiError(w, http.StatusConflict, "conflict", "E-PUB-006", "the app is archived and takes no more publishes")
		return
	case !created && !may(s, a, "release:request"):
		apiError(w, http.StatusForbidden, "insufficient_scope", "E-PUB-005", "the app belongs to another builder, or your sign-in does not carry it yet")
		return
	case len(refs) != 1 || refs[0] != meta.SHA || len(prerequisites) > 1:
		apiError(w, http.StatusBadRequest, "invalid_request", "E-PUB-010", "the bundle must have exactly one ref, at the commit given in meta, and one prerequisite at most")
		return
	case created && p.createdToday() >= p.newApps:
		apiError(w, http.StatusTooManyRequests, "too_many_requests", "E-PUB-008", "you created 10 apps in the last 24 hours")
		return
	case created:
		a = &fakeApp{id: len(p.apps) + 1, name: meta.App, owner: Email, created: time.Now()}
	case slices.ContainsFunc(p.pubs, func(pub *fakePublish) bool { return pub.app == a && pub.sha == meta.SHA }):
		apiError(w, http.StatusConflict, "conflict", "E-PUB-007", "this commit is already published to the app: publish a new commit")
		return
	}
	if len(prerequisites) == 1 && prerequisites[0] != a.base {
		apiError(w, http.StatusConflict, "base_changed", "E-PUB-010", "the bundle's prerequisite is not the app's base: get the base again")
		return
	}
	if created {
		p.apps = append(p.apps, a)
	}
	pub := &fakePublish{id: publishID(), sha: meta.SHA, summary: meta.Summary, app: a, created: time.Now()}
	p.pubs = append(p.pubs, pub)
	p.uploads = append(p.uploads, Upload{App: meta.App, SHA: meta.SHA, Summary: meta.Summary, Bundle: bundle})
	a.base = meta.SHA
	writeJSON(w, http.StatusCreated, map[string]any{"id": pub.id, "app_id": a.id, "created_app": created})
}

// readUpload reads the parts of an upload: meta, a JSON object of at most 4 KiB that it decodes
// into meta, refusing unknown fields, then bundle, which it returns, cut after 32 MiB and a byte.
func readUpload(r *http.Request, meta *uploadMeta) ([]byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "meta" {
		return nil, errors.New("the first part is not meta")
	}
	dec := json.NewDecoder(io.LimitReader(part, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(meta); err != nil {
		return nil, fmt.Errorf("meta is not valid: %w", err)
	}
	if !appName.MatchString(meta.App) || !commitSHA.MatchString(meta.SHA) || len([]rune(meta.Summary)) > 1000 ||
		strings.ContainsFunc(meta.Summary, unicode.IsControl) {
		return nil, errors.New("meta is not valid")
	}
	part, err = mr.NextPart()
	if err != nil || part.FormName() != "bundle" {
		return nil, errors.New("the second part is not bundle")
	}
	bundle, err := io.ReadAll(io.LimitReader(part, maxBundle+1))
	if err != nil || len(bundle) > maxBundle {
		return bundle, err
	}
	if _, err := mr.NextPart(); err != io.EOF {
		return nil, errors.New("there is a part after bundle")
	}
	return bundle, nil
}

// createdToday returns how many apps the person created in the last 24 hours. The caller holds
// p.mu.
func (p *Platform) createdToday() int {
	n := 0
	for _, a := range p.apps {
		if a.owner == Email && time.Since(a.created) < 24*time.Hour {
			n++
		}
	}
	return n
}

// bundleHeader returns the prerequisites and the commits of the refs of a v2 git bundle whose
// header, of at most 16 lines, ends with a blank line followed by a pack.
func bundleHeader(b []byte) (prerequisites, refs []string, ok bool) {
	rest, ok := bytes.CutPrefix(b, []byte("# v2 git bundle\n"))
	for n := 0; ok; n++ {
		var line []byte
		line, rest, ok = bytes.Cut(rest, []byte("\n"))
		switch {
		case !ok || n == 16:
			return nil, nil, false
		case len(line) == 0:
			return prerequisites, refs, bytes.HasPrefix(rest, []byte("PACK"))
		case line[0] == '-':
			sha, _, _ := strings.Cut(string(line[1:]), " ")
			prerequisites = append(prerequisites, sha)
			ok = commitSHA.MatchString(sha)
		default:
			sha, name, _ := strings.Cut(string(line), " ")
			refs = append(refs, sha)
			ok = commitSHA.MatchString(sha) && name != ""
		}
	}
	return nil, nil, false
}

func publishID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "pub_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// apiError answers with the error errorCode of the builder API, with the framework's code unless
// it is "".
func apiError(w http.ResponseWriter, status int, errorCode, code, description string) {
	body := map[string]string{"error": errorCode, "error_description": description}
	if code != "" {
		body["code"] = code
	}
	writeJSON(w, status, body)
}
