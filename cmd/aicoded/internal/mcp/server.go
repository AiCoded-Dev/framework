// Package mcp is aicoded mcp: the local MCP server through which an AI assistant creates,
// checks, previews, debugs and publishes the apps of one workspace. It has no shell tool, no
// tool takes a path, and nothing it returns holds a secret value.
package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"path/filepath"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"aicoded.dev/framework/cmd/aicoded/internal/control"
	"aicoded.dev/framework/cmd/aicoded/internal/dev"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
)

// Version is the aicoded version the server reports to its clients.
var Version = "devel"

const instructions = `This server builds and runs apps made with the aicoded framework in one folder.
Start with howto guides/overview, which explains how an app works. howto with no topic lists
every page of the docs with its summary. The pages are also the resources aicoded://docs/<path>,
aicoded://changelog and aicoded://errors/<CODE>.
Work in this loop:
1. app_create makes a new app. Then edit its files.
2. check generates the code, builds the apps and runs go vet, the lint rules for app code and the tests.
   Fix every problem it lists.
3. preview builds and starts an app in aicoded dev and returns its local address.
4. When something fails, what_broke lists the failed apps, error log lines and failed spans.
5. Only when the person asks to publish an app: commit everything in its git repository, then
   call publish with the app and a summary of the change. It sends the commit to the platform's
   delivery pipeline and returns a publish id at once. Call release_status with that id until
   the status is passed, failed, refused or error, tell the person the outcome, and fix its
   problems as you fix check's. On E-CLI-004, ask the person to run aicoded login.
aicoded.yaml is the app's permission list: write app, owner, data, email, settings, secrets
and modules by hand.
aicoded generate writes its access and services sections, every *_gen.go and *_gen.ts file,
pages/assets_gen/ and .aicoded/. Never edit them.
describe summarises the apps, and howto with an error code such as E-DEV-001 explains it.
logs, traces, trace, mail_list and mail_get show what aicoded dev saw; mail_receive delivers a test mail.`

// Serve runs the MCP server for the workspace dir over t until the client leaves or ctx ends.
// An aicoded dev it hosts itself writes its lines to logs.
func Serve(ctx context.Context, dir string, t mcpsdk.Transport, logs io.Writer, fw scaffold.Framework) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	s := &server{session: ctx, dir: root, logs: logs, fw: fw}
	defer s.close()
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "aicoded", Version: Version},
		&mcpsdk.ServerOptions{Instructions: instructions})
	s.addTools(srv)
	if err := addDocs(srv); err != nil {
		return err
	}
	if err := srv.Run(ctx, t); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

type server struct {
	// session bounds the life of an aicoded dev the server hosts.
	session context.Context
	dir     string
	logs    io.Writer
	fw      scaffold.Framework

	mu     sync.Mutex
	hosted *dev.Workspace
	client *control.Client
}

// backend returns the aicoded dev of the workspace: the one the server hosts, a running one it
// reaches through its control socket, or, when none runs, a new one it hosts until the session
// ends.
func (s *server) backend(ctx context.Context) (devapi.Backend, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.hosted != nil:
		return s.hosted, nil
	case s.client != nil:
		return s.client, nil
	}
	if _, sock, ok := dev.FindRunning(ctx, s.dir); ok {
		c := control.Dial(sock)
		if err := c.Ping(ctx); err != nil {
			return nil, err
		}
		s.client = c
		return c, nil
	}
	w, err := dev.Host(s.session, s.dir, s.logs, dev.Options{})
	if err != nil {
		return nil, err
	}
	s.hosted = w
	return w, nil
}

// forget drops the client b when err shows that its aicoded dev is gone, so the next call finds
// or hosts another.
func (s *server) forget(b devapi.Backend, err error) {
	var op *net.OpError
	if !errors.As(err, &op) || op.Op != "dial" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := b.(*control.Client); ok && c == s.client {
		s.client = nil
	}
}

// close stops the aicoded dev the server hosts.
func (s *server) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hosted != nil {
		s.hosted.Close()
		s.hosted = nil
	}
}

// withDev runs f on the aicoded dev of the workspace.
func withDev[T any](ctx context.Context, s *server, f func(devapi.Backend) (T, error)) (T, error) {
	b, err := s.backend(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	v, err := f(b)
	if err != nil {
		s.forget(b, err)
	}
	return v, err
}
