// Package errs formats coded errors: every "must not" in the framework is reported
// with a code, an optional file:line, a one-line fix and a link to its docs page.
package errs

import (
	"errors"
	"strings"
)

// DocsBase is the address of the error catalogue; a code's page is DocsBase + code.
const DocsBase = "https://aicoded.dev/docs/errors/"

// Error is a coded error.
type Error struct {
	Code string // E-AREA-NNN
	Pos  string // file:line; empty when there is no source position
	Msg  string
	Fix  string
}

// New returns a coded error without a source position.
func New(code, msg, fix string) *Error {
	return &Error{Code: code, Msg: msg, Fix: fix}
}

// At returns a coded error at a source position (file:line).
func At(pos, code, msg, fix string) *Error {
	return &Error{Code: code, Pos: pos, Msg: msg, Fix: fix}
}

func (e *Error) Error() string {
	var b strings.Builder
	if e.Pos != "" {
		b.WriteString(e.Pos)
		b.WriteString(": ")
	}
	b.WriteString(e.Code)
	b.WriteString(": ")
	b.WriteString(e.Msg)
	b.WriteString("\n  fix: ")
	b.WriteString(e.Fix)
	b.WriteString("\n  docs: ")
	b.WriteString(DocsBase)
	b.WriteString(e.Code)
	return b.String()
}

// Code returns the code of the first coded error in err's chain, or "".
func Code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
