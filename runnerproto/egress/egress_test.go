package egress_test

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto"
	"aicoded.dev/framework/runnerproto/egress"
	"aicoded.dev/framework/runnerproto/server"
)

var _ server.Outbound = (*egress.Client)(nil)

var public = netip.MustParseAddr("93.184.216.34")

// resolver answers with its answers in turn, the last one again once they run out.
type resolver struct {
	mu      sync.Mutex
	answers [][]netip.Addr
	err     error
	calls   int
	names   []string
}

func answer(addrs ...string) *resolver {
	r := &resolver{answers: [][]netip.Addr{nil}}
	for _, a := range addrs {
		r.answers[0] = append(r.answers[0], netip.MustParseAddr(a))
	}
	return r
}

func (r *resolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() { r.calls++ }()
	r.names = append(r.names, host)
	if r.err != nil {
		return nil, r.err
	}
	return r.answers[min(r.calls, len(r.answers)-1)], nil
}

// dials records the addresses a client dials; a dial of refuse fails.
type dials struct {
	mu     sync.Mutex
	addrs  []string
	refuse string
}

func (d *dials) list() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.addrs
}

// service starts a TLS service for h, over HTTP/2, and returns a client of o whose every dial
// reaches it. The service's certificate is valid for example.com and its subdomains.
func service(t *testing.T, h http.HandlerFunc, o egress.Options) (*egress.Client, *dials) {
	t.Helper()
	return serve(t, h, o, true)
}

// serve is service over HTTP/2 when h2 is true, and over HTTP/1.1 otherwise.
func serve(t *testing.T, h http.HandlerFunc, o egress.Options, h2 bool) (*egress.Client, *dials) {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = h2
	srv.StartTLS()
	t.Cleanup(srv.Close)
	d := &dials{}
	o.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		d.mu.Lock()
		d.addrs = append(d.addrs, address)
		refuse := d.refuse == address
		d.mu.Unlock()
		if refuse {
			return nil, errors.New("connection refused")
		}
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	o.RootCAs = x509.NewCertPool()
	o.RootCAs.AddCert(srv.Certificate())
	return egress.New(o), d
}

func ok(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }

func get(t *testing.T, c *egress.Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	return c.Do(req.Context(), req)
}

// refusal returns the error of a GET of url with c.
func refusal(t *testing.T, c *egress.Client, url string) error {
	t.Helper()
	resp, err := get(t, c, url)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

func requireCode(t *testing.T, code string, err error) {
	t.Helper()
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, code, e.Code, e.Error())
}

func TestDenied(t *testing.T) {
	for _, a := range []string{
		"0.0.0.0", "::", "127.0.0.1", "127.255.255.254", "::1",
		"10.0.0.5", "172.16.0.1", "192.168.1.1", "169.254.169.254", "fe80::1", "fe80::1%eth0",
		"224.0.0.1", "ff02::1", "239.1.1.1", "ff0e::1",
		"0.1.2.3", "100.64.0.0", "100.64.1.1", "100.127.255.255", "192.0.0.8", "192.0.2.1",
		"198.18.0.0", "198.19.255.255", "198.51.100.7", "203.0.113.9", "240.0.0.1", "250.1.2.3",
		"255.255.255.255",
		"64:ff9b::7f00:1", "64:ff9b:1::1", "100::1", "2001::1", "2001:0:ffff::1", "2001:db8::1",
		"2001:db8::1%eth0", "2002:7f00:1::1", "fc00::1", "fd00:ec2::254", "fdff::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.5", "::ffff:169.254.169.254", "::ffff:100.64.1.1",
		"::127.0.0.1", "::a00:5", "::8.8.8.8", "::ffff:0:7f00:1", "::ffff:0:a00:5", "::ffff:0:808:808",
	} {
		assert.True(t, egress.Denied(netip.MustParseAddr(a)), a)
	}
	assert.True(t, egress.Denied(netip.Addr{}))
	for _, a := range []string{
		"93.184.216.34", "8.8.8.8", "100.63.255.255", "100.128.0.0", "198.17.255.255", "198.20.0.0",
		"2606:4700::1111", "2001:1::1", "::ffff:93.184.216.34",
	} {
		assert.False(t, egress.Denied(netip.MustParseAddr(a)), a)
	}
}

func TestDoLooksUpTheHostAsAnAbsoluteName(t *testing.T) {
	r := answer("93.184.216.34")
	c, _ := service(t, ok, egress.Options{Resolver: r})
	for _, url := range []string{"https://api.example.com/", "https://www.example.com./"} {
		resp, err := get(t, c, url)
		require.NoError(t, err, "the certificate is checked for the name without the dot: %s", url)
		require.NoError(t, resp.Body.Close())
		c.CloseIdleConnections()
	}
	assert.Equal(t, []string{"api.example.com.", "www.example.com."}, r.names, "no search domain expands it")
}

func TestDo(t *testing.T) {
	c, d := service(t, ok, egress.Options{Resolver: answer("93.184.216.34")})
	resp, err := get(t, c, "https://api.example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
	assert.Equal(t, 2, resp.ProtoMajor)
	assert.Equal(t, []string{"93.184.216.34:443"}, d.list())
}

func TestRefusesDeniedAddresses(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.5", "::1", "100.64.1.1", "fd00:ec2::254"} {
		c, d := service(t, ok, egress.Options{Resolver: answer(a)})
		err := refusal(t, c, "https://api.example.com/")
		requireCode(t, "E-OUT-003", err)
		assert.NotContains(t, err.Error(), a)
		assert.Empty(t, d.list(), a)
	}
}

func TestRefusesMixedAnswers(t *testing.T) {
	for _, r := range []*resolver{answer("93.184.216.34", "10.0.0.5"), answer("10.0.0.5", "93.184.216.34")} {
		c, d := service(t, ok, egress.Options{Resolver: r})
		err := refusal(t, c, "https://api.example.com/")
		requireCode(t, "E-OUT-003", err)
		assert.Empty(t, d.list())
	}
}

func TestChecksTheAnswerOfEveryDial(t *testing.T) {
	r := &resolver{answers: [][]netip.Addr{{public}, {netip.MustParseAddr("127.0.0.1")}}}
	c, d := service(t, ok, egress.Options{Resolver: r})
	for i, code := range []string{"", "E-OUT-003"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com/", nil)
		require.NoError(t, err)
		req.Close = true
		resp, err := c.Do(req.Context(), req)
		if code == "" {
			require.NoError(t, err, i)
			require.NoError(t, resp.Body.Close())
		} else {
			requireCode(t, code, err)
		}
	}
	assert.Equal(t, 2, r.calls)
	assert.Equal(t, []string{"93.184.216.34:443"}, d.list())
}

func TestControlChecksTheAddressAgain(t *testing.T) {
	c := egress.New(egress.Options{Deny: func(a netip.Addr) bool { return a == netip.MustParseAddr("8.8.8.8") }})
	for _, a := range []string{"127.0.0.1:443", "[::ffff:10.0.0.5]:443", "[fd00:ec2::254]:443", "8.8.8.8:443", "bad"} {
		requireCode(t, "E-OUT-003", egress.Control(c, a))
	}
	assert.NoError(t, egress.Control(c, "93.184.216.34:443"))
}

func TestDenyOption(t *testing.T) {
	var seen []netip.Addr
	deny := func(a netip.Addr) bool { seen = append(seen, a); return a == public }
	for _, a := range []string{"93.184.216.34", "::ffff:93.184.216.34"} {
		c, d := service(t, ok, egress.Options{Resolver: answer(a), Deny: deny})
		err := refusal(t, c, "https://api.example.com/")
		requireCode(t, "E-OUT-003", err)
		assert.Empty(t, d.list())
	}
	assert.Equal(t, []netip.Addr{public, public}, seen)
}

func TestDoesNotFollowRedirects(t *testing.T) {
	c, d := service(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://10.0.0.5/", http.StatusFound)
	}, egress.Options{Resolver: answer("93.184.216.34")})
	resp, err := get(t, c, "https://api.example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://10.0.0.5/", resp.Header.Get("Location"))
	assert.Equal(t, []string{"93.184.216.34:443"}, d.list())
}

func TestRefusesOtherSchemesAndPorts(t *testing.T) {
	c, d := service(t, ok, egress.Options{Resolver: answer("93.184.216.34")})
	for _, u := range []string{"http://api.example.com/", "http://api.example.com:443/", "https://api.example.com:8443/"} {
		err := refusal(t, c, u)
		requireCode(t, "E-OUT-002", err)
	}
	assert.Empty(t, d.list())
}

func TestResponseBodyLimit(t *testing.T) {
	for _, h2 := range []bool{true, false} {
		for size, code := range map[int]string{runnerproto.MaxOutboundBody: "", runnerproto.MaxOutboundBody + 1: "E-OUT-005"} {
			c, _ := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(bytes.Repeat([]byte("a"), size))
			}, egress.Options{Resolver: answer("93.184.216.34")}, h2)
			resp, err := get(t, c, "https://api.example.com/")
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if code == "" {
				require.NoError(t, err)
				assert.Len(t, body, size)
			} else {
				requireCode(t, code, err)
			}
		}
	}
}

// stall waits until the client goes away, or 5 s.
func stall(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

// stallInBody sends the start of a body, then stalls.
func stallInBody(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("part"))
	w.(http.Flusher).Flush()
	stall(r)
}

// readAll returns the error of a GET of url with c under ctx, from Do or from reading the body.
func readAll(t *testing.T, ctx context.Context, c *egress.Client, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	return err
}

func TestDeadline(t *testing.T) {
	assert.Equal(t, runnerproto.OutboundDeadline, egress.Deadline(egress.New(egress.Options{})))
	assert.Equal(t, 5*time.Second, egress.DNSTimeout)

	for _, h2 := range []bool{true, false} {
		for _, h := range []http.HandlerFunc{func(_ http.ResponseWriter, r *http.Request) { stall(r) }, stallInBody} {
			c, _ := serve(t, h, egress.Options{Resolver: answer("93.184.216.34")}, h2)
			egress.SetDeadline(c, 200*time.Millisecond)
			start := time.Now()
			err := readAll(t, context.Background(), c, "https://api.example.com/")
			requireCode(t, "E-OUT-006", err)
			assert.Contains(t, err.Error(), "took over 200ms")
			assert.Less(t, time.Since(start), 3*time.Second)
		}
	}
}

func TestDeadlineCoversASentBody(t *testing.T) {
	for _, h2 := range []bool{true, false} {
		c, _ := serve(t, ok, egress.Options{Resolver: answer("93.184.216.34")}, h2)
		egress.SetDeadline(c, 200*time.Millisecond)
		resp, err := get(t, c, "https://api.example.com/")
		require.NoError(t, err)
		time.Sleep(300 * time.Millisecond)
		_, err = io.ReadAll(resp.Body)
		requireCode(t, "E-OUT-006", err)
		assert.Contains(t, err.Error(), "took over 200ms")
		require.NoError(t, resp.Body.Close())
	}
}

func TestDeadlineCutsTheConnect(t *testing.T) {
	c := egress.New(egress.Options{Resolver: answer("93.184.216.34"),
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, errors.New("connect timed out")
			}
		}})
	egress.SetDeadline(c, 200*time.Millisecond)
	start := time.Now()
	requireCode(t, "E-OUT-006", refusal(t, c, "https://api.example.com/"))
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestCallerContext(t *testing.T) {
	for _, h := range []http.HandlerFunc{func(_ http.ResponseWriter, r *http.Request) { stall(r) }, stallInBody} {
		c, _ := service(t, h, egress.Options{Resolver: answer("93.184.216.34")})

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := readAll(t, ctx, c, "https://api.example.com/")
		cancel()
		requireCode(t, "E-OUT-006", err)
		assert.NotContains(t, err.Error(), "30s")

		ctx, cancel = context.WithCancel(context.Background())
		time.AfterFunc(200*time.Millisecond, cancel)
		err = readAll(t, ctx, c, "https://api.example.com/")
		requireCode(t, "E-OUT-008", err)
		assert.Contains(t, err.Error(), "cancelled")
	}
}

func TestDialsTheNextCheckedAddress(t *testing.T) {
	c, d := service(t, ok, egress.Options{Resolver: answer("93.184.216.35", "93.184.216.34")})
	d.refuse = "93.184.216.35:443"
	resp, err := get(t, c, "https://api.example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, []string{"93.184.216.35:443", "93.184.216.34:443"}, d.list())
}

func TestDialsIPv6(t *testing.T) {
	c, d := service(t, ok, egress.Options{Resolver: answer("2606:4700::1111")})
	resp, err := get(t, c, "https://api.example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, []string{"[2606:4700::1111]:443"}, d.list())
}

func TestCloseIdleConnections(t *testing.T) {
	c, d := serve(t, ok, egress.Options{Resolver: answer("93.184.216.34")}, false)
	assert.Equal(t, runnerproto.MaxOutboundRequests, egress.Transport(c).MaxIdleConnsPerHost)
	for range 2 {
		resp, err := get(t, c, "https://api.example.com/")
		require.NoError(t, err)
		_, _ = io.Copy(io.Discard, resp.Body)
		require.NoError(t, resp.Body.Close())
	}
	assert.Len(t, d.list(), 1)
	c.CloseIdleConnections()
	resp, err := get(t, c, "https://api.example.com/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Len(t, d.list(), 2)
}

func TestUnreachable(t *testing.T) {
	notFound := &resolver{err: &net.DNSError{Err: "no such host", Name: "api.example.com", IsNotFound: true}}
	for name, o := range map[string]egress.Options{
		"unknown host": {Resolver: notFound},
		"no address":   {Resolver: answer()},
	} {
		c, d := service(t, ok, o)
		err := refusal(t, c, "https://api.example.com/")
		requireCode(t, "E-OUT-008", err)
		assert.Empty(t, d.list(), name)
	}

	c, d := service(t, ok, egress.Options{Resolver: answer("93.184.216.34")})
	err := refusal(t, c, "https://api.partner.test/")
	requireCode(t, "E-OUT-008", err)
	assert.Equal(t, []string{"93.184.216.34:443"}, d.list())
	assert.NotContains(t, err.Error(), "x509")

	c = egress.New(egress.Options{Resolver: answer("93.184.216.34"),
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("dial tcp 10.1.2.3:443: connection refused")
		}})
	err = refusal(t, c, "https://api.example.com/")
	requireCode(t, "E-OUT-008", err)
	assert.NotContains(t, err.Error(), "10.1.2.3")
}
