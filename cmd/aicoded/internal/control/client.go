package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/internal/errs"
)

// Client reaches an aicoded dev through its control socket.
type Client struct {
	path string
	hc   *http.Client
}

var _ devapi.Backend = (*Client)(nil)

// Dial returns a client of the control socket at path. It connects for each request.
func Dial(path string) *Client {
	return &Client{path: path, hc: &http.Client{Transport: &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}}}
}

// Ping asks for the status and fails with E-DEV-013 when the aicoded dev there speaks another
// protocol.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Status(ctx)
	return err
}

func (c *Client) Status(ctx context.Context) (devapi.Status, error) {
	var st devapi.Status
	return st, c.do(ctx, http.MethodGet, "/v1/status", nil, &st)
}

func (c *Client) Restart(ctx context.Context, app string) (devapi.App, error) {
	var a devapi.App
	return a, c.do(ctx, http.MethodPost, "/v1/restart", appRequest{App: app}, &a)
}

func (c *Client) Logs(ctx context.Context, q devapi.LogQuery) ([]devapi.LogEntry, error) {
	var es []devapi.LogEntry
	return es, c.do(ctx, http.MethodPost, "/v1/logs", q, &es)
}

func (c *Client) Traces(ctx context.Context, q devapi.TraceQuery) ([]devapi.TraceSummary, error) {
	var ts []devapi.TraceSummary
	return ts, c.do(ctx, http.MethodPost, "/v1/traces", q, &ts)
}

func (c *Client) Trace(ctx context.Context, id string) (devapi.Trace, error) {
	var t devapi.Trace
	return t, c.do(ctx, http.MethodPost, "/v1/trace", traceRequest{ID: id}, &t)
}

func (c *Client) Mail(ctx context.Context, app string) ([]devapi.MailSummary, error) {
	var ms []devapi.MailSummary
	return ms, c.do(ctx, http.MethodPost, "/v1/mail", appRequest{App: app}, &ms)
}

func (c *Client) MailGet(ctx context.Context, app, id string) (devapi.Mail, error) {
	var m devapi.Mail
	return m, c.do(ctx, http.MethodPost, "/v1/mail/get", mailGetRequest{App: app, ID: id}, &m)
}

func (c *Client) MailReceive(ctx context.Context, app string, m devapi.InboundMail) (string, error) {
	var r idReply
	return r.ID, c.do(ctx, http.MethodPost, "/v1/mail/receive", mailReceiveRequest{App: app, Mail: m}, &r)
}

func (c *Client) WhatBroke(ctx context.Context, app string) (devapi.Broken, error) {
	var b devapi.Broken
	return b, c.do(ctx, http.MethodPost, "/v1/what-broke", appRequest{App: app}, &b)
}

// do sends in as JSON, or nothing when in is nil, and reads the answer into out. An error the
// aicoded dev answers comes back as an *errs.Error when it has a code.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://aicoded"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set(header, Protocol)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if got := resp.Header.Get(header); got != Protocol {
		return mismatch("the aicoded dev at "+c.path, got)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply+1))
	if err != nil {
		return err
	}
	if len(data) > maxReply {
		return fmt.Errorf("the aicoded dev at %s answered more than %d MiB", c.path, maxReply>>20)
	}
	if resp.StatusCode != http.StatusOK {
		var e errorReply
		if json.Unmarshal(data, &e) != nil || e.Error.Msg == "" {
			return fmt.Errorf("the aicoded dev at %s answered %s", c.path, resp.Status)
		}
		if e.Error.Code == "" {
			return errors.New(e.Error.Msg)
		}
		return &errs.Error{Code: e.Error.Code, Pos: e.Error.Pos, Msg: e.Error.Msg, Fix: e.Error.Fix}
	}
	return json.Unmarshal(data, out)
}
