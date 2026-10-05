// Package problem is the form in which aicoded check, describe, dev and mcp show coded errors,
// in text and in JSON.
package problem

import (
	"errors"

	"aicoded.dev/framework/internal/errs"
)

// Problem is one coded error of an app, or an error without a code, which has only Message.
type Problem struct {
	App     string `json:"app,omitempty"`
	Code    string `json:"code,omitempty"`
	Pos     string `json:"pos,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
	Docs    string `json:"docs,omitempty"`
}

// From returns one problem per *errs.Error in err's tree (through Unwrap() error and
// Unwrap() []error), or one problem with only Message when there is none.
func From(app string, err error) []Problem {
	if err == nil {
		return nil
	}
	var es []*errs.Error
	collect(err, &es)
	if len(es) == 0 {
		return []Problem{{App: app, Message: err.Error()}}
	}
	return FromErrs(app, es)
}

// FromErrs returns one problem per coded error.
func FromErrs(app string, es []*errs.Error) []Problem {
	out := make([]Problem, len(es))
	for i, e := range es {
		out[i] = Problem{App: app, Code: e.Code, Pos: e.Pos, Message: e.Msg, Fix: e.Fix, Docs: errs.DocsBase + e.Code}
	}
	return out
}

// collect appends the coded errors of err's tree to es, depth first. The first node with
// Unwrap() []error that errors.As finds lies on the chain of Unwrap() error from err, and a coded
// error, which wraps nothing, can only end that chain.
func collect(err error, es *[]*errs.Error) {
	var multi interface{ Unwrap() []error }
	if errors.As(err, &multi) {
		for _, err := range multi.Unwrap() {
			collect(err, es)
		}
		return
	}
	var e *errs.Error
	if errors.As(err, &e) {
		*es = append(*es, e)
	}
}

// String returns the problem as errs.Error prints it, after "<app>: " when it has an app.
func (p Problem) String() string {
	s := p.Message
	if p.Code != "" {
		s = (&errs.Error{Code: p.Code, Pos: p.Pos, Msg: p.Message, Fix: p.Fix}).Error()
	}
	if p.App != "" {
		s = p.App + ": " + s
	}
	return s
}
