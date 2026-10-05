package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/web/reactive"
)

// Caller is implemented by the state of a route with <ssr:call> functions.
type Caller interface {
	RouteKey() string
	// Call decodes args and runs the route's Call<Name> hook.
	Call(ctx context.Context, r *Request, name string, args json.RawMessage) (any, error)
}

// callTimeout bounds one page call.
var callTimeout = 30 * time.Second

const maxCallsInFlight = 4

var errCallRedirect = errs.New("E-WEB-005", "a page call returned web.Redirect",
	"return data from the call and navigate in index.ts with location.assign")

// DecodeArgs reads the arguments of a page call into v. Unknown fields and trailing data are
// refused, so a page cannot send more than the call declares.
//
// Generated code only.
func DecodeArgs(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errBadArgs()
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errBadArgs()
	}
	return nil
}

func errBadArgs() error { return Error(http.StatusBadRequest, "The call has the wrong arguments.") }

// runCall checks the access rules and guards of the whole path again, runs one page call and
// sends its result or failure.
func runCall(ctx context.Context, r *Request, conn *reactive.Conn, routes []Route, msg reactive.CallMsg) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	_ = conn.Send(context.WithoutCancel(ctx), answerCall(ctx, r, routes, msg))
}

// answerCall runs one page call and returns its result or fail frame. A panic in app code,
// including in the result's encoding, fails the call and leaves the connection open.
func answerCall(ctx context.Context, r *Request, routes []Route, msg reactive.CallMsg) (reply any) {
	defer func() {
		if p := recover(); p != nil {
			slog.ErrorContext(ctx, "panic in page call", "call", msg.Name, "value", redact.ValueFor(ctx, p), "stack", string(debug.Stack()))
			reply = reactive.NewFail(msg.ID, http.StatusInternalServerError, "Something went wrong.")
		}
	}()
	value, err := dispatchCall(ctx, r, routes, msg)
	if err == nil {
		var b []byte
		if b, err = json.Marshal(value); err == nil {
			return reactive.NewResult(msg.ID, b)
		}
	}
	status, text := callFailure(ctx, msg.Name, err)
	return reactive.NewFail(msg.ID, status, text)
}

func dispatchCall(ctx context.Context, r *Request, routes []Route, msg reactive.CallMsg) (any, error) {
	cr := &Request{Request: r.WithContext(ctx), params: r.params, session: r.session, viewer: r.viewer}
	states, err := admit(ctx, cr, routes)
	if err != nil {
		return nil, err
	}
	for _, s := range page(routes, states) {
		if c, ok := s.(Caller); ok && c.RouteKey() == msg.RouteKey {
			return c.Call(ctx, cr, msg.Name, msg.Args)
		}
	}
	return nil, Error(http.StatusNotFound, "This call is not on this page.")
}

// callFailure turns a call's error into what the page sees; details stay in the log.
func callFailure(ctx context.Context, name string, err error) (int, string) {
	var he *HTTPError
	var rd *redirect
	switch {
	case errors.As(err, &he) && he.Status >= 400 && he.Status <= 599:
		return he.Status, he.Message
	case errors.Is(err, context.DeadlineExceeded):
		slog.WarnContext(ctx, "page call timed out", "call", name)
		return http.StatusGatewayTimeout, "The call took too long."
	case errors.As(err, &rd):
		err = errCallRedirect
	case he != nil:
		err = errBadStatus(he.Status)
	}
	slog.ErrorContext(ctx, "page call failed", "call", name, "err", redact.For(ctx, err))
	return http.StatusInternalServerError, "Something went wrong."
}
