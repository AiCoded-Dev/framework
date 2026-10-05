package web

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/redact"
)

// HTTPError ends a request with Status and shows Message to the viewer.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string { return strconv.Itoa(e.Status) + " " + e.Message }

// Error ends the request with status, from 400 to 599, and shows message to the viewer. Never
// put internal details in message.
func Error(status int, message string) error { return &HTTPError{Status: status, Message: message} }

// NotFound ends the request with 404.
func NotFound() error { return Error(http.StatusNotFound, "This page does not exist.") }

// Forbidden ends the request with 403.
func Forbidden() error { return Error(http.StatusForbidden, "You do not have access to this page.") }

type redirect struct{ path string }

func (r *redirect) Error() string { return "redirect to " + r.path }

// Redirect sends the viewer to path, a path on this site such as "/notes/42".
func Redirect(path string) error {
	if !localPath(path) {
		return errs.New("E-WEB-002", "web.Redirect got a target that is not a path on this site",
			`redirect to a path that starts with a single "/", such as "/notes/42"`)
	}
	return &redirect{path: path}
}

func localPath(p string) bool {
	if p == "" || p[0] != '/' || len(p) > 1 && (p[1] == '/' || p[1] == '\\') {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f || p[i] == '\\' {
			return false
		}
	}
	u, err := url.Parse(p)
	return err == nil && u.Scheme == "" && u.Host == "" && u.User == nil
}

func init() {
	redact.Kind(func(e *HTTPError) string { return "HTTP " + strconv.Itoa(e.Status) })
	redact.Kind(func(*redirect) string { return "web.Redirect" })
}

// fail answers a request that a check or a hook ended with err. Only messages meant for the
// viewer reach the page; anything else is logged, with its text only under aicoded dev in
// environment `dev`, and answered with 500.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var he *HTTPError
	var rd *redirect
	switch {
	case errors.As(err, &rd):
		setHeaders(w.Header())
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, rd.path, http.StatusSeeOther)
	case errors.As(err, &he) && he.Status >= 400 && he.Status <= 599:
		errorPage(w, he.Status, he.Message)
	default:
		if he != nil {
			err = errBadStatus(he.Status)
		}
		slog.ErrorContext(r.Context(), "request failed", "err", redact.For(r.Context(), err))
		errorPage(w, http.StatusInternalServerError, "Something went wrong.")
	}
}

func errBadStatus(status int) error {
	return errs.New("E-WEB-004", fmt.Sprintf("web.Error got status %d", status), "pass a status from 400 to 599 to web.Error")
}

func errorPage(w http.ResponseWriter, status int, message string) {
	h := w.Header()
	setHeaders(h)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	text := html.EscapeString(http.StatusText(status))
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8"><title>%d %s</title><h1>%s</h1><p>%s</p></html>`,
		status, text, text, html.EscapeString(message))
}
