package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/internal/errs"
)

// MaxBundle is the size of the largest bundle the platform takes: 32 MiB.
const MaxBundle = 32 << 20

// uploadTimeout bounds the sending of a bundle, which takes longer than other requests.
const uploadTimeout = 10 * time.Minute

// maxPublish is the largest answer about a publish read from the platform, which holds its
// problems: 8 MiB.
const maxPublish = 8 << 20

var (
	// PublishID matches the id of a publish.
	PublishID = regexp.MustCompile(`^pub_[a-z2-7]{26}$`)
	commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
	codeName  = regexp.MustCompile(`^E-[A-Z]+-[0-9]{3}$`)
	pubCode   = regexp.MustCompile(`^E-PUB-[0-9]{3}$`)
)

// fixes are the fixes of the codes of publishing.
var fixes = map[string]string{
	"E-PUB-001": "make the app's folder a git repository of its own, with git init and git commit in it",
	"E-PUB-002": "commit them, or remove them, then run aicoded publish again",
	"E-PUB-003": "leave --app out, or run aicoded publish in the folder of the app it names",
	"E-PUB-004": "fix every problem it lists, commit, then run aicoded publish again",
	"E-PUB-005": "publish under a name of your own: change app: in aicoded.yaml, or ask your administrator to transfer the app to you",
	"E-PUB-006": "publish under a new name: change app: in aicoded.yaml, or ask your administrator about the archived app",
	"E-PUB-007": "commit a change, then run aicoded publish again",
	"E-PUB-008": "publish to an app you already have, or create the new app tomorrow",
	"E-PUB-009": "remove large files from the repository and its history, or publish from a new repository that holds only the app",
	"E-PUB-010": "run aicoded publish again",
	"E-PUB-011": "commit again, even with git commit --allow-empty, then run aicoded publish; if it fails again, give your administrator the publish's id",
	"E-PUB-012": "install git 2.31 or later, and put it on PATH",
	"E-PUB-013": "copy the id from what aicoded publish or the publish tool printed",
	"E-PUB-014": "make sure git fsck finds nothing wrong in the app's repository, commit again, then run aicoded publish",
}

// Fix returns the fix of the code of publishing, or the advice to read its docs page.
func Fix(code string) string {
	if fix, ok := fixes[code]; ok {
		return fix
	}
	return "read the docs page below"
}

// ErrBaseChanged is the error of CreatePublish when another publish of the app changed its base
// after App returned it: the bundle must be made again from the new base.
var ErrBaseChanged = errs.New("E-PUB-010", "another publish of the app changed the commit whose history the platform holds while this one was sent",
	Fix("E-PUB-010"))

// App is an app of the builder's organisation.
type App struct {
	Name     string
	Archived bool
	// Base is the commit whose history the platform holds, which a publish need not send again,
	// or "".
	Base string
	// Last is the app's latest publish, or nil.
	Last *Last
}

// Last is an app's latest publish.
type Last struct{ ID, SHA, Status string }

// Upload is a commit of an app to publish, with the git bundle that carries it.
type Upload struct {
	App, SHA, Summary string
	Bundle            []byte
}

// Created is the platform's answer to an upload.
type Created struct {
	ID         string
	CreatedApp bool
}

// Publish is a publish as the platform reports it, with its text cleaned for a terminal.
type Publish struct {
	ID      string `json:"id"`
	App     string `json:"app"`
	SHA     string `json:"sha"`
	Summary string `json:"summary"`
	// Status is queued, running, passed, failed, refused or error.
	Status string `json:"status"`
	// Reason says why the publish was refused or ended in error.
	Reason  string    `json:"reason,omitempty"`
	Created time.Time `json:"created"`
	Ended   time.Time `json:"ended,omitzero"`
	// Parent is the publish whose history this one's bundle continues.
	Parent           *Parent `json:"parent,omitempty"`
	HistoryRewritten bool    `json:"history_rewritten"`
	// Checks is "partial" while only the checks of aicoded check and the tests run.
	Checks   string            `json:"checks"`
	Steps    []Step            `json:"steps"`
	Problems []problem.Problem `json:"problems"`
	// Record is the number of the publish's change record, once it is written.
	Record int64 `json:"record,omitempty"`
}

// Parent is the publish before another.
type Parent struct {
	ID  string `json:"id"`
	SHA string `json:"sha"`
}

// Step is a step of the delivery pipeline, such as check, and its outcome: passed, failed,
// skipped or error.
type Step struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
}

// statuses maps the status of a publish to whether the publish has ended.
var statuses = map[string]bool{"queued": false, "running": false, "passed": true, "failed": true, "refused": true, "error": true}

// Done reports whether the publish has ended.
func (p Publish) Done() bool { return statuses[p.Status] }

// App returns the app of the builder's organisation named name; found is false when there is
// none. Another builder's app is E-PUB-005, and an app the builder owns that is archived may be
// E-PUB-006, as the platform's answer says.
func (c *Client) App(ctx context.Context, name string) (app App, found bool, err error) {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return App{}, false, err
	}
	status, answer, err := c.call(ctx, c.HTTP, "/v1/apps/get", "application/json", body, maxAnswer)
	if err != nil {
		return App{}, false, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusOK:
	case pubCode.MatchString(e.Code):
		return App{}, false, refusal(e, e.Code, "the platform refused the app "+name)
	case status == http.StatusNotFound && e.Error == "not_found":
		return App{}, false, nil
	case status == http.StatusForbidden && e.Error == "insufficient_scope":
		return App{}, false, refusal(e, "E-PUB-005", "the app "+name+" belongs to another builder")
	default:
		return App{}, false, unexpected(status, answer)
	}
	var a struct {
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
		Base     string `json:"base"`
		Last     *Last  `json:"last"`
	}
	if json.Unmarshal(answer, &a) != nil || a.Name != name || a.Base != "" && !commitSHA.MatchString(a.Base) ||
		a.Last != nil && (!PublishID.MatchString(a.Last.ID) || !commitSHA.MatchString(a.Last.SHA)) {
		return App{}, false, errors.New("the platform's answer about the app " + name + " is not valid")
	}
	if a.Last != nil {
		a.Last.Status = clean(a.Last.Status, 32)
	}
	return App{Name: a.Name, Archived: a.Archived, Base: a.Base, Last: a.Last}, true, nil
}

// CreatePublish sends the upload to the delivery pipeline and returns the new publish. The first
// publish of a name creates the app. A bundle whose prerequisite is no longer the app's base is
// ErrBaseChanged; the other refusals have the code the platform gives, E-PUB-005 to E-PUB-010.
func (c *Client) CreatePublish(ctx context.Context, u Upload) (Created, error) {
	meta, err := json.Marshal(map[string]string{"app": u.App, "sha": u.SHA, "summary": u.Summary})
	if err != nil {
		return Created{}, err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, p := range []struct {
		disposition, contentType string
		data                     []byte
	}{
		{`form-data; name="meta"`, "application/json", meta},
		{`form-data; name="bundle"; filename="app.bundle"`, "application/octet-stream", u.Bundle},
	} {
		w, err := mw.CreatePart(textproto.MIMEHeader{"Content-Disposition": {p.disposition}, "Content-Type": {p.contentType}})
		if err != nil {
			return Created{}, err
		}
		if _, err := w.Write(p.data); err != nil {
			return Created{}, err
		}
	}
	if err := mw.Close(); err != nil {
		return Created{}, err
	}
	upload := *c.HTTP
	upload.Timeout = uploadTimeout
	status, answer, err := c.call(ctx, &upload, "/v1/publishes/create", mw.FormDataContentType(), body.Bytes(), maxAnswer)
	if err != nil {
		return Created{}, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusCreated:
	case status == http.StatusConflict && e.Error == "base_changed":
		return Created{}, ErrBaseChanged
	case pubCode.MatchString(e.Code):
		return Created{}, refusal(e, e.Code, "the platform refused the publish")
	case status == http.StatusForbidden:
		return Created{}, refusal(e, "E-PUB-005", "the app "+u.App+" belongs to another builder")
	case status == http.StatusRequestEntityTooLarge:
		return Created{}, refusal(e, "E-PUB-009", "the bundle of "+u.App+" is larger than the platform takes")
	default:
		return Created{}, unexpected(status, answer)
	}
	var a struct {
		ID         string `json:"id"`
		CreatedApp bool   `json:"created_app"`
	}
	if json.Unmarshal(answer, &a) != nil || !PublishID.MatchString(a.ID) {
		return Created{}, errors.New("the platform's answer to the publish is not valid")
	}
	return Created{ID: a.ID, CreatedApp: a.CreatedApp}, nil
}

// Publish returns the publish id, which must match PublishID. It reads an answer of up to 8 MiB,
// since a publish may have many problems. An id the platform does not know is E-PUB-013, and a
// publish of an app the builder does not own E-PUB-005, as the platform's answer says.
func (c *Client) Publish(ctx context.Context, id string) (Publish, error) {
	body, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		return Publish{}, err
	}
	status, answer, err := c.call(ctx, c.HTTP, "/v1/publishes/get", "application/json", body, maxPublish)
	if err != nil {
		return Publish{}, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusOK:
	case pubCode.MatchString(e.Code):
		return Publish{}, refusal(e, e.Code, "the platform refused to show the publish "+id)
	case status == http.StatusNotFound:
		return Publish{}, refusal(e, "E-PUB-013", "the platform knows no publish "+id)
	case status == http.StatusForbidden:
		return Publish{}, refusal(e, "E-PUB-005", "the publish "+id+" is of another builder's app")
	default:
		return Publish{}, unexpected(status, answer)
	}
	var p Publish
	if json.Unmarshal(answer, &p) != nil || p.ID != id || !p.valid() {
		return Publish{}, errors.New("the platform's answer about the publish " + id + " is not valid")
	}
	p.clean()
	return p, nil
}

// valid reports whether the ids, commits and status of p are well formed.
func (p *Publish) valid() bool {
	_, known := statuses[p.Status]
	return known && commitSHA.MatchString(p.SHA) &&
		(p.Parent == nil || PublishID.MatchString(p.Parent.ID) && commitSHA.MatchString(p.Parent.SHA))
}

// clean makes every text of p fit to print, and gives every problem the docs link of its code.
func (p *Publish) clean() {
	p.App, p.Summary, p.Reason, p.Checks = clean(p.App, 63), clean(p.Summary, 1000), clean(p.Reason, 500), clean(p.Checks, 64)
	steps := make([]Step, 0, len(p.Steps))
	for _, s := range p.Steps {
		steps = append(steps, Step{Name: clean(s.Name, 32), Outcome: clean(s.Outcome, 32)})
	}
	p.Steps = steps
	problems := make([]problem.Problem, 0, len(p.Problems))
	for _, q := range p.Problems {
		out := problem.Problem{Pos: clean(q.Pos, 300), Message: cleanLines(q.Message, 4000), Fix: clean(q.Fix, 500)}
		if codeName.MatchString(q.Code) {
			out.Code, out.Docs = q.Code, errs.DocsBase+q.Code
		}
		problems = append(problems, out)
	}
	p.Problems = problems
	p.Record = max(p.Record, 0)
}

// call posts body, of type contentType, to the builder API at path with hc and the access token
// of the sign-in, and returns the answer's status and body, of at most limit bytes. A token the
// platform no longer accepts is E-CLI-004. When the platform refuses the token for want of a
// scope, call refreshes the sign-in once, which brings the scopes of the apps the builder owns
// now, and posts again.
func (c *Client) call(ctx context.Context, hc *http.Client, path, contentType string, body []byte, limit int64) (int, []byte, error) {
	tok, err := c.Token(ctx)
	if err != nil {
		return 0, nil, err
	}
	for refreshed := false; ; refreshed = true {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.address+path, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
		answer, status, err := send(hc, req, limit)
		switch {
		case err != nil:
			return 0, nil, err
		case status == http.StatusUnauthorized:
			return 0, nil, signedOut("the platform at " + c.address + " no longer accepts your sign-in")
		case status != http.StatusForbidden || refreshed || parseError(answer).Error != "insufficient_scope":
			return status, answer, nil
		}
		if tok, err = c.Refresh(ctx, tok); err != nil {
			return 0, nil, err
		}
	}
}

// apiError is the answer of the builder API to a request it refused.
type apiError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
	Code        string `json:"code"`
}

func parseError(body []byte) apiError {
	var e apiError
	_ = json.Unmarshal(body, &e)
	return e
}

// refusal returns the error of a refusal with code, and e's description, or else msg.
func refusal(e apiError, code, msg string) error {
	if e.Description != "" {
		msg = clean(e.Description, 300)
	}
	return errs.New(code, msg, Fix(code))
}

// unexpected is the error of an answer the builder API does not give.
func unexpected(status int, body []byte) error {
	return fmt.Errorf("the platform answered %d%s", status, reason(body))
}

// cleanLines is clean for text of several lines: it keeps the line breaks and cleans each line.
func cleanLines(s string, limit int) string {
	if utf8.RuneCountInString(s) > limit {
		s = string([]rune(s)[:limit]) + "..."
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = clean(strings.TrimSuffix(l, "\r"), limit)
	}
	return strings.Join(lines, "\n")
}
