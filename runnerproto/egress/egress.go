// Package egress sends a runner's checked requests to outside services. It resolves each host
// for every connection, refuses the request when any address of the answer is denied, and dials
// only an address that passed that check, over TLS with the host's name. It follows no redirect
// and bounds each request in time and its response body in size.
//
// A *Client is the runner's server.Outbound provider.
package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto"
)

// dnsTimeout bounds resolving a host.
const dnsTimeout = 5 * time.Second

// Resolver resolves host names; *net.Resolver is one.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Options configure a Client.
type Options struct {
	// Deny reports more addresses no app may reach, beyond those Denied reports. It gets an
	// IPv4-mapped IPv6 address as its IPv4 address, and an address without its zone. It is
	// asked when a connection is made: a connection already open keeps the peer it was checked
	// for, so a caller that changes what Deny refuses calls Client.CloseIdleConnections.
	Deny func(netip.Addr) bool
	// Resolver resolves hosts; nil is net.DefaultResolver.
	Resolver Resolver
	// DialContext connects to an address that passed the check, as "ip:443". It exists for
	// tests; nil is a net.Dialer that checks the address again just before it connects.
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
	// RootCAs are the certificate authorities a service's certificate must chain to. It exists
	// for tests; nil is the system's roots.
	RootCAs *x509.CertPool
}

// Client sends requests to outside services. It is safe for concurrent use.
type Client struct {
	deny      func(netip.Addr) bool
	resolver  Resolver
	dial      func(ctx context.Context, network, address string) (net.Conn, error)
	transport *http.Transport
	// deadline bounds a whole request, until its response body is closed.
	deadline time.Duration
}

// New returns a Client of o.
func New(o Options) *Client {
	c := &Client{deny: o.Deny, resolver: o.Resolver, dial: o.DialContext, deadline: runnerproto.OutboundDeadline}
	if c.resolver == nil {
		c.resolver = net.DefaultResolver
	}
	if c.dial == nil {
		c.dial = (&net.Dialer{Timeout: runnerproto.OutboundDeadline, Control: c.control}).DialContext
	}
	c.transport = &http.Transport{
		Proxy:                  nil,
		DialContext:            c.dialChecked,
		ForceAttemptHTTP2:      true,
		TLSClientConfig:        &tls.Config{RootCAs: o.RootCAs, MinVersion: tls.VersionTLS12},
		DisableCompression:     true,
		MaxResponseHeaderBytes: 1 << 20,
		MaxIdleConns:           100,
		MaxIdleConnsPerHost:    runnerproto.MaxOutboundRequests,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    runnerproto.OutboundDeadline,
	}
	return c
}

// Do sends req, an https request to port 443, and returns the response as the service sent it:
// a redirect is not followed. The whole request, until the response body is closed, has
// runnerproto.OutboundDeadline, and the body yields at most runnerproto.MaxOutboundBody bytes.
// A refusal or failure, of Do or of a read of the body, is an *errs.Error with an E-OUT code.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	host := req.URL.Hostname()
	if req.URL.Scheme != "https" || (req.URL.Port() != "" && req.URL.Port() != "443") {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errs.New("E-OUT-002", "the request to "+strconv.Quote(host)+" is not https to port 443",
			"call the service over https on port 443")
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.deadline)
	resp, err := c.transport.RoundTrip(req.WithContext(reqCtx))
	if err != nil {
		err = c.failure(ctx, reqCtx, host, err)
		cancel()
		return nil, err
	}
	resp.Body = &body{rc: resp.Body, parent: ctx, ctx: reqCtx, cancel: cancel, c: c, host: host}
	return resp, nil
}

// CloseIdleConnections closes the connections c keeps open for later requests, so that the
// next request to each host resolves and checks it again.
func (c *Client) CloseIdleConnections() {
	c.transport.CloseIdleConnections()
}

// failure returns the error the app gets for err, of a request to host under ctx, which Do
// derived from the caller's parent.
func (c *Client) failure(parent, ctx context.Context, host string, err error) error {
	var e *errs.Error
	switch {
	case errors.As(err, &e):
		return e
	case errors.Is(parent.Err(), context.DeadlineExceeded):
		return errs.New("E-OUT-006", "the request to "+strconv.Quote(host)+" ran out of the time its caller gave it",
			"ask the service for less, or try again later")
	case parent.Err() != nil:
		return errs.New("E-OUT-008", "the request to "+strconv.Quote(host)+" was cancelled by its caller",
			"check the host name, or try again later")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return errs.New("E-OUT-006", "the request to "+strconv.Quote(host)+" took over "+c.deadline.String(),
			"ask the service for less, or try again later")
	default:
		return unreachable(host)
	}
}

func unreachable(host string) error {
	return errs.New("E-OUT-008", strconv.Quote(host)+" could not be reached", "check the host name, or try again later")
}

func refused(host string) error {
	return errs.New("E-OUT-003", strconv.Quote(host)+" resolves to an address no app may reach",
		"call a public service; the platform's own hosts and private networks are never reachable")
}

// denied reports whether a is on the base list or Options.Deny's.
func (c *Client) denied(a netip.Addr) bool {
	return Denied(a) || (c.deny != nil && c.deny(a.Unmap().WithZone("")))
}

// dialChecked resolves the host of address, refuses it when any address of the answer is
// denied, and dials the checked addresses on port 443 in turn until one connects.
func (c *Client) dialChecked(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errs.New("E-OUT-002", "the request is not https to port 443", "call the service over https on port 443")
	}
	rctx, cancel := context.WithTimeout(ctx, dnsTimeout)
	addrs, err := c.resolver.LookupNetIP(rctx, "ip", absolute(host))
	cancel()
	if err != nil && ctx.Err() != nil {
		return nil, err
	}
	if err != nil || len(addrs) == 0 {
		return nil, unreachable(host)
	}
	for _, a := range addrs {
		if c.denied(a) {
			return nil, refused(host)
		}
	}
	err = unreachable(host)
	for _, a := range addrs {
		conn, derr := c.dial(ctx, network, netip.AddrPortFrom(a.Unmap(), 443).String())
		if derr == nil {
			return conn, nil
		}
		var e *errs.Error
		if errors.As(derr, &e) {
			err = e
		}
		if ctx.Err() != nil {
			return nil, derr
		}
	}
	return nil, err
}

// absolute returns host as an absolute name, with a dot at its end, so that a resolver never
// tries it under a search domain. An IP address stays as it is.
func absolute(host string) string {
	if _, err := netip.ParseAddr(host); err == nil || strings.HasSuffix(host, ".") {
		return host
	}
	return host + "."
}

// control is the default dialer's last check of address, an "ip:port", before it connects.
func (c *Client) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || c.denied(ap.Addr()) {
		return errs.New("E-OUT-003", "the address of the service is one no app may reach",
			"call a public service; the platform's own hosts and private networks are never reachable")
	}
	return nil
}

// body is a response body that yields at most runnerproto.MaxOutboundBody bytes and ends its
// request's deadline when closed.
type body struct {
	rc     io.ReadCloser
	parent context.Context
	ctx    context.Context
	cancel context.CancelFunc
	c      *Client
	host   string
	n      int64
	err    error
}

func (b *body) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	if left := runnerproto.MaxOutboundBody + 1 - b.n; int64(len(p)) > left {
		p = p[:left]
	}
	n, err := b.rc.Read(p)
	b.n += int64(n)
	if b.n > runnerproto.MaxOutboundBody {
		b.err = errs.New("E-OUT-005", "the response body of "+strconv.Quote(b.host)+" is over 10 MiB",
			"ask the service for less, such as one page at a time")
		return n - int(b.n-runnerproto.MaxOutboundBody), b.err
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.err = b.c.failure(b.parent, b.ctx, b.host, err)
		return n, b.err
	}
	return n, err
}

func (b *body) Close() error {
	defer b.cancel()
	return b.rc.Close()
}
