package outbound

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// deadline is the runner's bound on a whole request, and backstop how much longer the client
// waits for the runner's answer, so that it gives up only on a runner that hangs.
var (
	deadline = runnerproto.OutboundDeadline
	backstop = 5 * time.Second
)

// Client returns a client whose requests go to the runner, which sends them to the outside
// service. Each request must carry the context of the request or job it serves, as
// http.NewRequestWithContext gives it. The client follows no redirect: a 3xx response comes back
// with its Location. The runner ends a request after 30 s with E-OUT-006; the client gives up
// 5 s later, on a runner that does not answer.
func Client() *http.Client {
	return &http.Client{
		Transport:     transport{},
		Timeout:       deadline + backstop,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// transport hands each request to the runner of the request's context.
type transport struct{}

func (transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer func() { _ = req.Body.Close() }()
	}
	s := runner.From(req.Context())
	if s == nil || s.Outbound == nil {
		return nil, runner.ErrNoRunner
	}
	body, err := readBody(req)
	if err != nil {
		return nil, err
	}
	msg := &runnerv1.DoRequest{Method: req.Method, Url: req.URL.String(), Body: body}
	for _, name := range slices.Sorted(maps.Keys(req.Header)) {
		for _, v := range req.Header[name] {
			msg.Headers = append(msg.Headers, &runnerv1.Header{Name: name, Value: v})
		}
	}
	call := connect.NewRequest(msg)
	if sc, ok := tracectx.From(req.Context()); ok {
		call.Header().Set("traceparent", sc.Traceparent())
	}
	resp, err := s.Outbound.Do(req.Context(), call)
	if err != nil {
		var ce *connect.Error
		if errors.As(err, &ce) {
			if e, ok := errs.Parse(ce.Message()); ok {
				return nil, e
			}
		}
		// The context's own error reports Timeout, which the client's error then reports too.
		if cerr := req.Context().Err(); cerr != nil {
			return nil, cerr
		}
		return nil, fmt.Errorf("outbound: %w", err)
	}
	return response(req, resp.Msg), nil
}

// readBody reads the body of req. It refuses a body over runnerproto.MaxOutboundBody.
func readBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	tooLarge := errs.New("E-OUT-004", "the request body is over 10 MiB", "send at most 10 MiB; keep large files in a file store")
	if req.ContentLength > runnerproto.MaxOutboundBody {
		return nil, tooLarge
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, runnerproto.MaxOutboundBody+1))
	if err != nil {
		return nil, fmt.Errorf("outbound: read the request body: %w", err)
	}
	if len(body) > runnerproto.MaxOutboundBody {
		return nil, tooLarge
	}
	return body, nil
}

func response(req *http.Request, m *runnerv1.DoResponse) *http.Response {
	h := http.Header{}
	for _, f := range m.GetHeaders() {
		h.Add(f.GetName(), f.GetValue())
	}
	code := int(m.GetStatus())
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", code, http.StatusText(code)),
		StatusCode: code,
		Proto:      "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(m.GetBody())),
		ContentLength: int64(len(m.GetBody())),
		Request:       req,
	}
}
