package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// partnerKey is the value of partner_api_key, the key the runner adds to requests to partner.Host.
const partnerKey = "partner-key-value"

// sent is a request as it reached the fake outbound provider.
type sent struct {
	method, url, host string
	header            http.Header
	body              []byte
}

// fakeOutbound keeps the requests it gets and answers each with answer, or 200 "ok" when
// answer is nil.
type fakeOutbound struct {
	mu     sync.Mutex
	sent   []sent
	answer func(*http.Request) (*http.Response, error)
}

func (f *fakeOutbound) Do(_ context.Context, req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	f.mu.Lock()
	f.sent = append(f.sent, sent{method: req.Method, url: req.URL.String(), host: req.Host, header: req.Header.Clone(), body: body})
	answer := f.answer
	f.mu.Unlock()
	if answer == nil {
		return respond(http.StatusOK, nil, "ok"), nil
	}
	return answer(req)
}

func (f *fakeOutbound) got() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.sent...)
}

func (f *fakeOutbound) answerWith(answer func(*http.Request) (*http.Response, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answer = answer
}

func respond(status int, h http.Header, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func get(url string) *runnerv1.DoRequest {
	return &runnerv1.DoRequest{Method: http.MethodGet, Url: url}
}

func do(t *testing.T, c clients, req *runnerv1.DoRequest) (*runnerv1.DoResponse, error) {
	resp, err := c.out.Do(t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

// coded returns the coded error that err carries to the app.
func coded(t *testing.T, err error) *errs.Error {
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	e, ok := errs.Parse(ce.Message())
	require.True(t, ok, "not coded: %v", err)
	return e
}

// serveWith serves notes changed by change on fresh fakes.
func serveWith(t *testing.T, change func(*Options)) (clients, *fakes) {
	f := newFakes()
	opts := options(t, f)
	change(&opts)
	start(t, opts)
	return dial(opts.Dir), f
}

func TestOutboundSendsTheKeyInPlaceOfTheAppsHeaders(t *testing.T) {
	c, f := serve(t)
	f.outbound.answerWith(func(*http.Request) (*http.Response, error) {
		h := http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"a=1", "b=2"}, "Connection": {"close"},
			"Transfer-Encoding": {"chunked"}, "Location": {"https://api.partner.example/next"}}
		return respond(http.StatusFound, h, `{"id":7}`), nil
	})
	resp, err := do(t, c, &runnerv1.DoRequest{Method: http.MethodPost, Url: "https://api.partner.example/v1/items?q=1",
		Headers: []*runnerv1.Header{
			{Name: "authorization", Value: "Bearer the-apps-own"},
			{Name: "AUTHORIZATION", Value: "Basic twice"},
			{Name: "Host", Value: "other.example"},
			{Name: "Connection", Value: "close"},
			{Name: "Proxy-Authorization", Value: "Basic x"},
			{Name: "Content-Type", Value: "application/json"},
			{Name: "X-Request", Value: "1"},
			{Name: "x-request", Value: "2"},
		},
		Body: []byte(`{"a":1}`)})
	require.NoError(t, err)

	assert.Equal(t, []sent{{method: http.MethodPost, url: "https://api.partner.example/v1/items?q=1", host: "api.partner.example",
		header: http.Header{"Authorization": {"Bearer " + partnerKey}, "Content-Type": {"application/json"}, "X-Request": {"1", "2"}},
		body:   []byte(`{"a":1}`)}}, f.outbound.got(), "only the runner's key, and the request goes to the URL's host")
	assert.Equal(t, int32(http.StatusFound), resp.GetStatus(), "a redirect reaches the app")
	assert.Equal(t, []*runnerv1.Header{
		{Name: "Content-Type", Value: "application/json"},
		{Name: "Location", Value: "https://api.partner.example/next"},
		{Name: "Set-Cookie", Value: "a=1"},
		{Name: "Set-Cookie", Value: "b=2"},
	}, resp.GetHeaders(), "without the hop-by-hop headers")
	assert.Equal(t, []byte(`{"id":7}`), resp.GetBody())
}

func TestOutboundMatchesTheCanonicalHost(t *testing.T) {
	c, f := serve(t)
	for _, url := range []string{
		"https://API.Partner.Example./",
		"https://api.partner.example:443/",
		"HTTPS://api.partner.example/",
		"https://status.partner.example/",
		"https://xn--bcher-kva.example/",
	} {
		_, err := do(t, c, get(url))
		require.NoError(t, err, url)
	}
	var hosts []string
	for _, s := range f.outbound.got() {
		hosts = append(hosts, s.url)
	}
	assert.Equal(t, []string{"https://api.partner.example/", "https://api.partner.example/", "https://api.partner.example/",
		"https://status.partner.example/", "https://xn--bcher-kva.example/"}, hosts)
	assert.Empty(t, f.outbound.got()[3].header, "a host without a binding gets no key")
}

func TestOutboundRefusesURLs(t *testing.T) {
	c, f := serve(t)
	for url, code := range map[string]string{
		"http://api.partner.example/":                "E-OUT-002",
		"https://api.partner.example:8443/":          "E-OUT-002",
		"https://api.partner.example:/":              "E-OUT-002",
		"https://api.partner.example@evil.example/":  "E-OUT-002",
		"https://user:pw@api.partner.example/":       "E-OUT-002",
		"https://@api.partner.example/":              "E-OUT-002",
		"ftp://api.partner.example/":                 "E-OUT-002",
		"/v1/items":                                  "E-OUT-002",
		"https:api.partner.example":                  "E-OUT-002",
		"":                                           "E-OUT-002",
		"https://evil.example/":                      "E-OUT-001",
		"https://sub.api.partner.example/":           "E-OUT-001",
		"https://api.partner.example../":             "E-OUT-001",
		"https://93.184.216.34/":                     "E-OUT-001",
		"https://[2001:db8::1]/":                     "E-OUT-001",
		"https://bücher.example/":                    "E-OUT-001",
		"https://xn--bcher-Kva.example/":             "E-OUT-001",
		"https://api.partner.example%2Eevil.example": "E-OUT-002",
		"https:///v1/items":                          "E-OUT-002",
	} {
		_, err := do(t, c, get(url))
		require.Error(t, err, url)
		assert.Equal(t, code, coded(t, err).Code, url)
	}
	assert.Empty(t, f.outbound.got(), "the provider never sees a refused request")
}

func TestOutboundRefusesThePlatformsHosts(t *testing.T) {
	c, f := serveWith(t, func(o *Options) { o.Manifest.Egress = []manifest.Egress{{Host: "api.aicoded.cloud"}} })
	_, err := do(t, c, get("https://api.aicoded.cloud/"))
	assert.Equal(t, "E-OUT-003", coded(t, err).Code)
	assert.Empty(t, f.outbound.got())
}

func TestOutboundRefusesHeaders(t *testing.T) {
	c, f := serve(t)
	for name, req := range map[string]*runnerv1.DoRequest{
		"CR LF in a value":   {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X-A", Value: "a\r\nAuthorization: x"}}},
		"LF in a value":      {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X-A", Value: "a\nb"}}},
		"NUL in a value":     {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X-A", Value: "a\x00b"}}},
		"a space in a name":  {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X A", Value: "a"}}},
		"a colon in a name":  {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X:A", Value: "a"}}},
		"an empty name":      {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "", Value: "a"}}},
		"a bad method":       {Method: "GET /x HTTP/1.1\r\n", Url: "https://api.partner.example/"},
		"CR in a Host value": {Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "Host", Value: "a\rb"}}},
	} {
		_, err := do(t, c, req)
		require.Error(t, err, name)
		assert.Equal(t, "E-OUT-009", coded(t, err).Code, name)
	}
	assert.Empty(t, f.outbound.got())
}

func TestOutboundHostNotInEgress(t *testing.T) {
	c, _ := serve(t)
	_, err := do(t, c, get("https://evil.example/"))
	e := coded(t, err)
	assert.Equal(t, `"evil.example" is not in egress in aicoded.yaml`, e.Msg)
	assert.Equal(t, "add the host to egress in aicoded.yaml; a host the app type does not allow needs approval", e.Fix)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestOutboundInAPreview(t *testing.T) {
	c, f := serveWith(t, func(o *Options) {
		o.Env = EnvPreview
		o.Egress = []manifest.Egress{{Host: "status.partner.example"}}
	})
	_, err := do(t, c, get("https://status.partner.example/"))
	require.NoError(t, err)
	_, err = do(t, c, get("https://api.partner.example/"))
	e := coded(t, err)
	assert.Equal(t, "E-OUT-001", e.Code)
	assert.Contains(t, e.Msg, "a preview reaches only the hosts of the app's live release")
	assert.Len(t, f.outbound.got(), 1)

	c, f = serveWith(t, func(o *Options) { o.Env = EnvPreview; o.Egress = []manifest.Egress{} })
	_, err = do(t, c, get("https://status.partner.example/"))
	assert.Equal(t, "E-OUT-001", coded(t, err).Code, "an empty list reaches no host")
	assert.Empty(t, f.outbound.got())
}

func TestOutboundUsesTheBindingOfOptionsEgress(t *testing.T) {
	c, f := serveWith(t, func(o *Options) {
		o.Env = EnvPreview
		o.Egress = []manifest.Egress{partner}
	})
	_, err := do(t, c, get("https://api.partner.example/"))
	require.NoError(t, err)
	assert.Equal(t, http.Header{"Authorization": {"Bearer " + partnerKey}}, f.outbound.got()[0].header)
	assert.Equal(t, []string{"secret partner_api_key"}, f.secrets.seen(), "read through the instance's own Secrets")
}

func TestOutboundBoundsTheRequestBody(t *testing.T) {
	c, f := serve(t)
	_, err := do(t, c, &runnerv1.DoRequest{Method: http.MethodPut, Url: "https://api.partner.example/", Body: make([]byte, runnerproto.MaxOutboundBody)})
	require.NoError(t, err)
	_, err = do(t, c, &runnerv1.DoRequest{Method: http.MethodPut, Url: "https://api.partner.example/", Body: make([]byte, runnerproto.MaxOutboundBody+1)})
	assert.Equal(t, "E-OUT-004", coded(t, err).Code)
	assert.Len(t, f.outbound.got(), 1, "the provider never sees the large body")
}

func TestOutboundBoundsTheResponseBody(t *testing.T) {
	c, f := serve(t)
	f.outbound.answerWith(func(*http.Request) (*http.Response, error) {
		return respond(http.StatusOK, nil, strings.Repeat("x", runnerproto.MaxOutboundBody+1)), nil
	})
	_, err := do(t, c, get("https://api.partner.example/"))
	assert.Equal(t, "E-OUT-005", coded(t, err).Code)
}

func TestOutboundBoundsRequestsAtOnce(t *testing.T) {
	c, f := serve(t)
	started, release := make(chan struct{}), make(chan struct{})
	f.outbound.answerWith(func(*http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-release
		return respond(http.StatusOK, nil, ""), nil
	})
	var wg sync.WaitGroup
	for range runnerproto.MaxOutboundRequests {
		wg.Go(func() {
			_, err := do(t, c, get("https://status.partner.example/"))
			assert.NoError(t, err)
		})
		<-started
	}
	_, err := do(t, c, get("https://status.partner.example/"))
	assert.Equal(t, "E-OUT-007", coded(t, err).Code)
	close(release)
	wg.Wait()

	f.outbound.answerWith(nil)
	_, err = do(t, c, get("https://status.partner.example/"))
	require.NoError(t, err, "a finished request frees its place")
}

func TestOutboundProviderErrors(t *testing.T) {
	c, f := serve(t)
	f.outbound.answerWith(func(*http.Request) (*http.Response, error) {
		return nil, errs.New("E-OUT-003", `"api.partner.example" resolves to an address no app may reach`, "call a public service")
	})
	_, err := do(t, c, get("https://api.partner.example/"))
	e := coded(t, err)
	assert.Equal(t, "E-OUT-003", e.Code, "a coded error of the provider passes by code and message")
	assert.Equal(t, `"api.partner.example" resolves to an address no app may reach`, e.Msg)

	f.outbound.answerWith(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("get: %w", errors.New("dial tcp 10.0.0.7:443: connection refused"))
	})
	_, err = do(t, c, get("https://api.partner.example/"))
	e = coded(t, err)
	assert.Equal(t, "E-OUT-008", e.Code)
	assert.Equal(t, `"api.partner.example" could not be reached`, e.Msg)
	assert.NotContains(t, err.Error(), "10.0.0.7", "the raw error never reaches the app")
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

func TestOutboundSecretsErrors(t *testing.T) {
	c, f := serve(t)
	f.secrets.fail(errors.New("vault at 10.0.0.7 refused"))
	_, err := do(t, c, get("https://api.partner.example/"))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "10.0.0.7")
	assert.Empty(t, f.outbound.got(), "no request without its key")
}

func TestOutboundSpans(t *testing.T) {
	c, f := serve(t)
	f.outbound.answerWith(func(*http.Request) (*http.Response, error) { return respond(http.StatusCreated, nil, "done"), nil })
	before := time.Now()
	req := connect.NewRequest(&runnerv1.DoRequest{Method: http.MethodPost, Url: "https://api.partner.example/v1/secret-path?q=ana", Body: []byte("hello")})
	req.Header().Set("traceparent", "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01")
	_, err := c.out.Do(t.Context(), req)
	require.NoError(t, err)
	_, err = do(t, c, get("https://evil.example/path"))
	require.Error(t, err)

	spans, _ := f.telemetry.got()
	require.Len(t, spans, 2)
	ok, refused := spans[0], spans[1]
	assert.Equal(t, "outbound", ok.GetName())
	assert.Equal(t, map[string]string{
		"server.address":                  "api.partner.example",
		"http.request.method":             "POST",
		"http.response.status_code":       "201",
		"aicoded.outbound.request_bytes":  "5",
		"aicoded.outbound.response_bytes": "4",
	}, ok.GetAttributes())
	assert.Empty(t, ok.GetError())
	assert.Equal(t, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, ok.GetTraceId(), "in the app's trace")
	assert.Equal(t, []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}, ok.GetParentSpanId())
	assert.Len(t, ok.GetSpanId(), 8)
	assert.LessOrEqual(t, before.UnixNano(), ok.GetStartUnixNano())
	assert.LessOrEqual(t, ok.GetStartUnixNano(), ok.GetEndUnixNano())
	assert.LessOrEqual(t, ok.GetEndUnixNano(), time.Now().UnixNano())

	assert.Equal(t, "outbound", refused.GetName())
	assert.Equal(t, map[string]string{
		"server.address":                  "evil.example",
		"http.request.method":             "GET",
		"aicoded.outbound.request_bytes":  "0",
		"aicoded.outbound.response_bytes": "0",
		"aicoded.outbound.refused":        "E-OUT-001",
	}, refused.GetAttributes())
	assert.Equal(t, "E-OUT-001", refused.GetError())
	assert.Len(t, refused.GetTraceId(), 16, "a trace of its own")
	assert.Empty(t, refused.GetParentSpanId())
	for _, sp := range spans {
		assert.NotContains(t, sp.String(), "path", "never the path or the query")
		assert.NotContains(t, sp.String(), "ana")
	}
}

func TestOutboundNeverLeaksTheKey(t *testing.T) {
	var logged bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(prev) })
	c, f := serve(t)
	var appErrs []error

	f.outbound.answerWith(func(r *http.Request) (*http.Response, error) {
		key := r.Header.Get("Authorization")
		return respond(http.StatusOK, http.Header{"X-Echo": {key}}, "you sent "+key), nil
	})
	resp, err := do(t, c, get("https://api.partner.example/"))
	require.NoError(t, err)
	assert.Contains(t, string(resp.GetBody()), partnerKey, "what the service sends back is the service's business")

	f.outbound.answerWith(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp 10.0.0.7:443 with Authorization: " + r.Header.Get("Authorization"))
	})
	_, err = do(t, c, get("https://api.partner.example/"))
	assert.Equal(t, "E-OUT-008", coded(t, err).Code)
	appErrs = append(appErrs, err)

	f.outbound.answerWith(func(r *http.Request) (*http.Response, error) {
		return nil, errs.New("E-OUT-006", "the request with "+r.Header.Get("Authorization")+" took too long", "try again later")
	})
	_, err = do(t, c, get("https://api.partner.example/"))
	assert.Equal(t, "E-OUT-008", coded(t, err).Code, "a coded error that quotes the key is not passed on")
	appErrs = append(appErrs, err)

	_, err = do(t, c, &runnerv1.DoRequest{Url: "https://api.partner.example/", Headers: []*runnerv1.Header{{Name: "X-A", Value: "\r\n"}}})
	appErrs = append(appErrs, err)
	_, err = do(t, c, &runnerv1.DoRequest{Url: "https://api.partner.example/", Body: make([]byte, runnerproto.MaxOutboundBody+1)})
	appErrs = append(appErrs, err)
	_, err = do(t, c, get("https://api.partner.example:8443/"))
	appErrs = append(appErrs, err)

	f.secrets.set("partner_api_key", partnerKey+"\r\nX-Evil: 1")
	_, err = do(t, c, get("https://api.partner.example/"))
	assert.Equal(t, "E-OUT-009", coded(t, err).Code, "a key that cannot go in a header")
	appErrs = append(appErrs, err)

	for _, err := range appErrs {
		require.Error(t, err)
		assert.NotContains(t, err.Error(), partnerKey)
	}
	spans, _ := f.telemetry.got()
	require.Len(t, spans, 7)
	for _, sp := range spans {
		assert.NotContains(t, sp.String(), partnerKey)
	}
	assert.NotContains(t, logged.String(), partnerKey)
}
