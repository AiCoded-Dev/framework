package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"math"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// outboundProtocol is the first version of the runner protocol with outbound.
const outboundProtocol = 4

// hopByHop are the headers the runner never passes on, by their canonical names.
var hopByHop = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Proxy-Connection",
	"Te", "Trailer", "Transfer-Encoding", "Upgrade"}

// dropped are the request headers the runner never passes on: the hop-by-hop ones, and Host and
// Content-Length, which follow from the URL and the body.
var dropped = append(slices.Clip(hopByHop), "Host", "Content-Length")

// methods are the request methods the runner sends.
var methods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}

// outboundCodes are the connect codes of the outbound refusals.
var outboundCodes = map[string]connect.Code{
	"E-OUT-001": connect.CodePermissionDenied,
	"E-OUT-002": connect.CodeInvalidArgument,
	"E-OUT-003": connect.CodePermissionDenied,
	"E-OUT-004": connect.CodeResourceExhausted,
	"E-OUT-005": connect.CodeResourceExhausted,
	"E-OUT-006": connect.CodeDeadlineExceeded,
	"E-OUT-007": connect.CodeResourceExhausted,
	"E-OUT-008": connect.CodeUnavailable,
	"E-OUT-009": connect.CodeInvalidArgument,
}

// outboundService checks the app's requests to outside services against egress, adds the key
// egress binds to the host, and hands what passes to Providers.Outbound.
type outboundService struct {
	s *Server
	// egress holds the entries that apply: the manifest's, or Options.Egress.
	egress manifest.Manifest
	// replaced is whether egress holds Options.Egress, the entries a preview shares with the
	// app's live release.
	replaced bool
	// slots holds a token for each request under way.
	slots chan struct{}
	// keys keeps the values of the bound keys.
	keys *keyCache
}

func newOutboundService(s *Server, egress []manifest.Egress, replaced bool) *outboundService {
	return &outboundService{s: s, egress: manifest.Manifest{Egress: egress}, replaced: replaced,
		slots: make(chan struct{}, runnerproto.MaxOutboundRequests), keys: newKeyCache()}
}

// outboundCall is what the span of one request records.
type outboundCall struct {
	start         time.Time
	host, method  string
	status        int
	reqBytes      int
	respBytes     int
	refused, fail string
}

func (o *outboundService) Do(ctx context.Context, req *connect.Request[runnerv1.DoRequest]) (*connect.Response[runnerv1.DoResponse], error) {
	method := req.Msg.GetMethod()
	if method == "" {
		method = http.MethodGet
	}
	call := &outboundCall{start: time.Now(), method: method, reqBytes: len(req.Msg.GetBody())}
	resp, err := o.do(ctx, req.Msg, call)
	if err != nil {
		call.fail = "internal error"
		if code := errs.Code(err); code != "" {
			call.refused, call.fail = code, code
		}
	}
	o.span(ctx, req.Header().Get("traceparent"), call)
	if err != nil {
		return nil, appError(err)
	}
	return connect.NewResponse(resp), nil
}

// appError returns err as the app gets it: a *connect.Error as it is, and an *errs.Error as a
// connect error of its code.
func appError(err error) error {
	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}
	c, ok := outboundCodes[errs.Code(err)]
	if !ok {
		c = connect.CodeUnavailable
	}
	return connect.NewError(c, err)
}

// do checks msg, sends it, and records in call what the span shows. Its refusals are
// *errs.Error; any other error is meant for the app as it is.
func (o *outboundService) do(ctx context.Context, msg *runnerv1.DoRequest, call *outboundCall) (*runnerv1.DoResponse, error) {
	u, err := o.target(msg.GetUrl(), call)
	if err != nil {
		return nil, err
	}
	if !token(call.method) {
		return nil, badHeader("the method is not an HTTP token")
	}
	if !slices.Contains(methods, call.method) {
		return nil, errs.New("E-OUT-002", "the runner sends only GET, HEAD, POST, PUT, PATCH, DELETE and OPTIONS",
			"send the request with GET, HEAD, POST, PUT, PATCH, DELETE or OPTIONS")
	}
	h, err := requestHeader(msg.GetHeaders())
	if err != nil {
		return nil, err
	}
	if len(msg.GetBody()) > runnerproto.MaxOutboundBody {
		return nil, errs.New("E-OUT-004", "the request body is over 10 MiB", "send at most 10 MiB; keep large files in a file store")
	}
	select {
	case o.slots <- struct{}{}:
		defer func() { <-o.slots }()
	default:
		return nil, errs.New("E-OUT-007", strconv.Itoa(runnerproto.MaxOutboundRequests)+" requests to outside services are already under way",
			"wait for a request to finish before starting another")
	}
	var key string
	if b, ok := o.egress.Binding(u.Host); ok {
		v, err := o.keys.get(ctx, b.Secret, o.s.providers.Secrets.Secret)
		if err != nil {
			return nil, providerError(err)
		}
		if !fieldValue(b.Prefix + v) {
			return nil, badHeader("the value of the secret " + quote(b.Secret) + " cannot go in a header: it holds a line break or another control character")
		}
		key = v
		h.Set(b.Header, b.Prefix+v)
	}
	out, err := http.NewRequestWithContext(ctx, call.method, u.String(), bytes.NewReader(msg.GetBody()))
	if err != nil {
		return nil, badHeader("the method is not an HTTP token")
	}
	out.Header = h
	resp, err := o.s.providers.Outbound.Do(ctx, out)
	if err != nil || resp == nil || resp.Body == nil {
		return nil, unreachable(u.Host, err, key)
	}
	defer func() { _ = resp.Body.Close() }()
	call.status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, runnerproto.MaxOutboundBody+1))
	if err != nil {
		return nil, unreachable(u.Host, err, key)
	}
	if len(body) > runnerproto.MaxOutboundBody {
		return nil, errs.New("E-OUT-005", "the response body of "+quote(u.Host)+" is over 10 MiB",
			"ask the service for less, such as one page at a time")
	}
	call.respBytes = len(body)
	return &runnerv1.DoResponse{Status: int32(min(max(resp.StatusCode, 0), math.MaxInt32)), Headers: responseHeader(resp.Header), Body: body}, nil
}

// target returns the URL of raw with the canonical host of an egress entry and no port, and
// records the host in call. It refuses any other URL.
func (o *outboundService) target(raw string, call *outboundCall) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Opaque != "" || u.Host == "" {
		return nil, badURL("the URL is not of the form https://<host>/<path>")
	}
	host, ascii := canonicalHost(u.Hostname())
	if ascii {
		call.host = host
	}
	switch {
	case u.Scheme != "https":
		return nil, badURL("the URL's scheme is " + quote(u.Scheme) + ", not https")
	case u.User != nil:
		return nil, badURL("the URL holds a user name or password before its host")
	case strings.HasSuffix(u.Host, ":") || (u.Port() != "" && u.Port() != "443"):
		return nil, badURL("the URL's port is " + quote(u.Port()) + ", not 443")
	case !ascii:
		return nil, o.notListed(u.Hostname(), "; a host is written in ASCII, an international name in its xn-- form")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return nil, o.notListed(host, "; call the service by its host name")
	}
	if !slices.Contains(o.egress.EgressHosts(), host) {
		return nil, o.notListed(host, "")
	}
	if manifest.PlatformHost(host) {
		return nil, errs.New("E-OUT-003", quote(host)+" is a host of the platform, which no app may call",
			"call a public service; the platform's own hosts and private networks are never reachable")
	}
	u.Host = host
	return u, nil
}

// canonicalHost returns host in lower case without one dot at its end, and whether it is
// printable ASCII. A host that is not stays as it was.
func canonicalHost(host string) (string, bool) {
	if strings.ContainsFunc(host, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return host, false
	}
	return strings.TrimSuffix(strings.ToLower(host), "."), true
}

// notListed refuses a request to host, which egress does not list, with more said after.
func (o *outboundService) notListed(host, more string) error {
	msg := quote(host) + " is not in egress in aicoded.yaml"
	if o.replaced {
		msg = quote(host) + " is not a host of the app's live release: a preview reaches only the hosts of the app's live release"
	}
	return errs.New("E-OUT-001", msg+more, "add the host to egress in aicoded.yaml; a host the app type does not allow needs approval")
}

func badURL(msg string) error {
	return errs.New("E-OUT-002", msg, "call the service over https on port 443")
}

func badHeader(msg string) error {
	return errs.New("E-OUT-009", msg, "use header names made of letters, digits and dashes, and values without control characters such as CR, LF or NUL")
}

// unreachable returns what the app gets for err, an error of Outbound for a request to host
// that carried key: an E-OUT error by its code and message, unless either quotes key, and any
// other error as E-OUT-008 with a fixed message.
func unreachable(host string, err error, key string) error {
	var e *errs.Error
	if errors.As(err, &e) && strings.HasPrefix(e.Code, "E-OUT-") && (key == "" || !strings.Contains(e.Msg+e.Fix, key)) {
		return errs.New(e.Code, e.Msg, e.Fix)
	}
	return errs.New("E-OUT-008", quote(host)+" could not be reached", "check the host name, or try again later")
}

// requestHeader returns hs as a header, without those in dropped. It refuses a name that is not
// an HTTP token and a value with a control character other than a tab.
func requestHeader(hs []*runnerv1.Header) (http.Header, error) {
	h := http.Header{}
	for _, f := range hs {
		if !token(f.GetName()) {
			return nil, badHeader("the header name " + quote(f.GetName()) + " is not an HTTP token")
		}
		if !fieldValue(f.GetValue()) {
			return nil, badHeader("the value of the header " + quote(f.GetName()) + " holds a line break or another control character")
		}
		h.Add(f.GetName(), f.GetValue())
	}
	for _, n := range dropped {
		h.Del(n)
	}
	return h, nil
}

// responseHeader returns h without the hop-by-hop headers, by name and in the order of each
// name's values.
func responseHeader(h http.Header) []*runnerv1.Header {
	var out []*runnerv1.Header
	for _, n := range slices.Sorted(maps.Keys(h)) {
		if slices.Contains(hopByHop, http.CanonicalHeaderKey(n)) {
			continue
		}
		for _, v := range h[n] {
			out = append(out, &runnerv1.Header{Name: n, Value: v})
		}
	}
	return out
}

// token reports whether s is an HTTP token (RFC 9110, section 5.6.2).
func token(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return r > '~' || r <= ' ' || strings.ContainsRune(`"(),/:;<=>?@[\]{}`, r)
	})
}

// fieldValue reports whether s may be the value of a header: no control character but a tab.
func fieldValue(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return (r < ' ' && r != '\t') || r == 0x7f })
}

// span hands Telemetry the span of call, in the trace of traceparent when it names one.
func (o *outboundService) span(ctx context.Context, traceparent string, call *outboundCall) {
	tel := o.s.providers.Telemetry
	if tel == nil {
		return
	}
	attrs := map[string]string{
		"http.request.method":             "_OTHER",
		"aicoded.outbound.request_bytes":  strconv.Itoa(call.reqBytes),
		"aicoded.outbound.response_bytes": strconv.Itoa(call.respBytes),
	}
	if token(call.method) {
		attrs["http.request.method"] = cut(call.method, maxQuote)
	}
	if call.host != "" {
		attrs["server.address"] = cut(call.host, 253)
	}
	if call.refused != "" {
		attrs["aicoded.outbound.refused"] = call.refused
	} else if call.status != 0 {
		attrs["http.response.status_code"] = strconv.Itoa(call.status)
	}
	id := tracectx.NewSpanID()
	sp := &runnerv1.Span{SpanId: id[:], Name: "outbound", StartUnixNano: call.start.UnixNano(),
		EndUnixNano: call.start.Add(time.Since(call.start)).UnixNano(), Attributes: attrs, Error: call.fail}
	if sc, ok := tracectx.Parse(traceparent); ok {
		sp.TraceId, sp.ParentSpanId = sc.TraceID[:], sc.SpanID[:]
	} else {
		trace := tracectx.NewTraceID()
		sp.TraceId = trace[:]
	}
	tel.Export(context.WithoutCancel(ctx), []*runnerv1.Span{sp}, 0)
}
