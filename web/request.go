package web

import (
	"net/http"
	"strconv"
	"time"

	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
)

// Request is a request as the hooks of a page see it.
type Request struct {
	*http.Request
	params  map[string]string
	session *runner.Session
	viewer  identity.Identity
	token   string
}

// URLParam returns the part of the URL that the folder s_name or n_name matched, or "".
func (r *Request) URLParam(name string) string { return r.params[name] }

// URLParamInt returns the number the folder n_name matched. It returns 0 when the path has no
// such parameter or its value is not a number.
func (r *Request) URLParamInt(name string) int64 {
	n, err := strconv.ParseInt(r.params[name], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// CSRFToken returns the token the forms on this page carry. Generated code writes it.
//
// Generated code only.
func (r *Request) CSRFToken() string {
	if r.token == "" {
		r.token = mintCSRF(r.session.CSRFKey, r.session.App, r.viewer.Subject, time.Now())
	}
	return r.token
}

// ResponseWriter lets hooks set response headers; the framework writes the page. Set a cookie
// with w.Header().Add("Set-Cookie", c.String()). On a live connection, Data gets a
// ResponseWriter whose headers are never sent.
type ResponseWriter interface {
	Header() http.Header
}

type headerWriter struct{ h http.Header }

func (w headerWriter) Header() http.Header { return w.h }
