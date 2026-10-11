package cantbebuilt

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/web"
)

// A table anyone can read: the notes app lists every note on a page that says nothing about who
// may open it (testdata/open-notes). aicoded check refuses the page.
func TestListWithNoAccessRule(t *testing.T) {
	assert.Equal(t, []refusal{
		{"E-GEN-030", "pages/notes/index.html:1", "/notes has no access rule on its path"},
	}, runCheck(t, copyApp(t, "open-notes")))
}

// The list is open only to staff and shows each viewer their own notes, but a new page opens any
// note by the id in its URL and says nothing about whose notes it shows (testdata/any-note).
// aicoded check refuses that page. The same app whose page has guard="true" and a Guard that
// lets in only the note's owner passes (testdata/own-notes).
func TestNoteOpenedByAnyID(t *testing.T) {
	assert.Equal(t, []refusal{
		{"E-GEN-054", "pages/notes/n_id/index.html:1", "/notes/n_id has a parameter in its URL but does not say whose records it shows"},
	}, runCheck(t, copyApp(t, "any-note")))

	assert.Empty(t, runCheck(t, copyApp(t, "own-notes")), "the control passes")
}

// aicoded generate is skipped: a page written by hand lists every note and has no access rule
// anywhere on its path. The framework serves it, and refuses it on its own: a signed-in viewer
// gets 403 and the app logs E-WEB-003. The same page with an access rule shows the notes to the
// same viewer.
func TestPageServedWithoutAccessRule(t *testing.T) {
	logs := captureLog(t)

	closed := web.New(map[string]web.Route{"/notes": listPage{web.Access{Roles: []string{"staff"}}}}, web.Options{})
	code, body := getAsStaff(t, closed, "/notes")
	assert.Equal(t, http.StatusOK, code, "the control")
	assert.Equal(t, everyNote, body)
	assert.NotContains(t, logs.String(), "E-WEB-003")

	open := web.New(map[string]web.Route{"/notes": listPage{}}, web.Options{})
	code, body = getAsStaff(t, open, "/notes")
	assert.Equal(t, http.StatusForbidden, code)
	assert.NotContains(t, body, everyNote)
	assert.Contains(t, logs.String(), "E-WEB-003: no route on this path has an access rule")
}

const everyNote = "<ul><li>ana: plans for March</li><li>ben: call the bank</li></ul>"

// listPage is the notes list written by hand instead of by aicoded generate, which never writes a
// page without an access rule (E-GEN-030). Its rule is access; the zero Access is none.
type listPage struct{ access web.Access }

func (p listPage) Access() web.Access     { return p.access }
func (listPage) Layout() bool             { return false }
func (listPage) NewState() web.RouteState { return &listState{} }

// listState serves one request for listPage: it has no Guard, form or data, and writes every
// note.
type listState struct{ web.Frame }

func (*listState) Guard(context.Context, *web.Request) error                         { return nil }
func (*listState) InitForms(context.Context, *web.Request, web.ResponseWriter) error { return nil }
func (*listState) Data(context.Context, *web.Request, web.ResponseWriter) error      { return nil }
func (*listState) DefaultRoute(context.Context, *web.Request) (string, error)        { return "", nil }

func (*listState) SubmitForm(context.Context, *web.Request, web.ResponseWriter, string) (bool, error) {
	return false, nil
}

func (*listState) Write(w io.Writer) error {
	_, err := io.WriteString(w, everyNote)
	return err
}

// getAsStaff sends a GET of path to the pages handler h as the app does once it has checked the
// viewer token the runner signed: with the app's session, and the viewer ana, of the role staff,
// in the request's context. It returns the status and the body.
func getAsStaff(t *testing.T, h http.Handler, path string) (int, string) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://notes.test"+path, nil)
	ctx := runner.With(r.Context(), &runner.Session{App: "notes", CSRFKey: make([]byte, 32)})
	ctx = identity.With(ctx, identity.Identity{Subject: "ana", Name: "Ana", Roles: []string{"staff"}})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(ctx))
	return w.Code, w.Body.String()
}

// captureLog sends what the app logs into the returned buffer until the test ends. It replaces
// slog's default logger for the whole process, so tests in this package must not use t.Parallel.
func captureLog(t *testing.T) *bytes.Buffer {
	var b bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&b, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &b
}
