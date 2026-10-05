package rpc

import (
	"cmp"
	"context"
	"errors"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/identity"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/telemetry"
)

// Call calls method of app with the encoded input in and returns the encoded output. The
// generated client functions call it; app code calls those instead. The call goes as the viewer
// of the request ctx belongs to, or as this app when ctx has no viewer. A call whose context has
// no deadline gets runnerproto.DefaultCallDeadline; the runner cuts any call at
// runnerproto.MaxCallDeadline.
//
// A failed call returns an error whose message is the callee's or the runner's. Only a refusal
// the runner marks with runnerproto.OriginHeader reaches the caller as a coded error, whose text
// starts with the code and carries its fix line; a called app's own refusal reaches the caller as
// plain text, whatever it looks like. CodeOf reads the status of every error, and errors.Is
// matches context.Canceled or context.DeadlineExceeded when the call's context ended first.
//
// Generated code only.
func Call(ctx context.Context, app, method string, in []byte) ([]byte, error) {
	s := runner.From(ctx)
	if s == nil || s.Rpc == nil {
		return nil, runner.ErrNoRunner
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runnerproto.DefaultCallDeadline)
		defer cancel()
	}
	ctx, span := telemetry.Start(ctx, "rpc.call")
	defer span.End()
	span.SetAttr("rpc.app", app)
	span.SetAttr("rpc.method", method)

	req := connect.NewRequest(&runnerv1.CallRequest{App: app, Method: method, Payload: in, Viewer: identity.Token(ctx)})
	if sc, ok := tracectx.From(ctx); ok {
		req.Header().Set("traceparent", sc.Traceparent())
	}
	resp, err := s.Rpc.Call(ctx, req)
	if err != nil {
		e := failed(err)
		span.RecordError(e)
		return nil, e
	}
	return resp.Msg.GetPayload(), nil
}

// failed turns the error of a call to the runner into the error the caller sees. It reads a
// coded error only from a refusal the runner marked as its own; the text of the called app's
// error stays plain text, whatever it looks like.
func failed(err error) *callError {
	e := &callError{code: Internal, msg: err.Error()}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return e
	}
	e.msg = ce.Message()
	switch ce.Code() {
	case connect.CodeCanceled:
		e.ctx = context.Canceled
		e.msg = cmp.Or(e.msg, "the call was canceled")
	case connect.CodeDeadlineExceeded:
		e.ctx = context.DeadlineExceeded
		e.msg = cmp.Or(e.msg, "the call ran out of time")
	default:
		e.code = codeOf(ce.Code())
	}
	if ce.Meta().Get(runnerproto.OriginHeader) == runnerproto.OriginRunner {
		if coded, ok := errs.Parse(e.msg); ok {
			e.coded = coded
		}
	}
	return e
}
