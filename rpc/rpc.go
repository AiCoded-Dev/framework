package rpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/redact"
)

// Code is the status of a failed call.
type Code int

// The codes a function may return with Error. Internal is what the caller sees for any other
// error.
const (
	InvalidArgument Code = iota + 1
	NotFound
	AlreadyExists
	PermissionDenied
	FailedPrecondition
	Unavailable
	Internal
)

var connectCodes = [...]connect.Code{
	InvalidArgument:    connect.CodeInvalidArgument,
	NotFound:           connect.CodeNotFound,
	AlreadyExists:      connect.CodeAlreadyExists,
	PermissionDenied:   connect.CodePermissionDenied,
	FailedPrecondition: connect.CodeFailedPrecondition,
	Unavailable:        connect.CodeUnavailable,
	Internal:           connect.CodeInternal,
}

func (c Code) String() string {
	if c < InvalidArgument || c > Internal {
		return fmt.Sprintf("code(%d)", int(c))
	}
	return connectCodes[c].String()
}

// codeOf returns the Code of a connect code, or Internal.
func codeOf(c connect.Code) Code {
	for code := InvalidArgument; code < Internal; code++ {
		if connectCodes[code] == c {
			return code
		}
	}
	return Internal
}

// statusError is an outcome a function returns for its caller.
type statusError struct {
	code Code
	msg  string
}

func (e *statusError) Error() string { return e.msg }

// Error returns an error that reaches the caller with code and msg. A code other than
// InvalidArgument, NotFound, AlreadyExists, PermissionDenied, FailedPrecondition or Unavailable
// reaches the caller as Internal with the message "internal error", like any other error.
func Error(code Code, msg string) error {
	if code < InvalidArgument || code > Internal {
		code = Internal
	}
	return &statusError{code: code, msg: msg}
}

// Errorf is Error with a formatted message.
func Errorf(code Code, format string, args ...any) error {
	return Error(code, fmt.Sprintf(format, args...))
}

// callError is a failed call to another app.
type callError struct {
	code  Code
	msg   string
	coded *errs.Error // the runner's refusal, when the message is a coded error
	ctx   error       // context.Canceled or context.DeadlineExceeded
}

func (e *callError) Error() string { return e.msg }

func (e *callError) Unwrap() []error {
	var out []error
	if e.coded != nil {
		out = append(out, e.coded)
	}
	if e.ctx != nil {
		out = append(out, e.ctx)
	}
	return out
}

func init() {
	redact.Kind(func(e *statusError) string { return "rpc " + e.code.String() })
	redact.Kind(func(e *callError) string { return "rpc " + e.code.String() })
}

// CodeOf returns the code of err: 0 for nil, the code of an error from Error, Errorf or a call,
// and Internal for any other error.
func CodeOf(err error) Code {
	if err == nil {
		return 0
	}
	var se *statusError
	if errors.As(err, &se) {
		return se.code
	}
	var ce *callError
	if errors.As(err, &ce) {
		return ce.code
	}
	return Internal
}

// Caller returns the app whose call ctx serves, or "" outside a call.
func Caller(ctx context.Context) string {
	return identity.Caller(ctx)
}
