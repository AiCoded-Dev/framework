package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"aicoded.dev/framework/cmd/aicoded/internal/check"
	"aicoded.dev/framework/cmd/aicoded/internal/describe"
	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/problem"
	"aicoded.dev/framework/cmd/aicoded/internal/publish"
	"aicoded.dev/framework/cmd/aicoded/internal/scaffold"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/manifest"
)

type appCreateIn struct {
	Name string `json:"name" jsonschema:"the new app's name: 2 to 63 lowercase letters, digits and dashes, starting with a letter"`
}

type appCreateOut struct {
	Dir  string `json:"dir"`
	Next string `json:"next"`
}

type checkIn struct {
	App    string `json:"app,omitempty" jsonschema:"check only this app; leave it out to check every app"`
	Frozen bool   `json:"frozen,omitempty" jsonschema:"write nothing, and report every generated file that is out of date"`
}

type appIn struct {
	App string `json:"app" jsonschema:"the app's name, from the app: line of its aicoded.yaml"`
}

type anyAppIn struct {
	App string `json:"app,omitempty" jsonschema:"only this app; leave it out for every app"`
}

type logsIn struct {
	App      string `json:"app,omitempty"`
	Level    string `json:"level,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	Contains string `json:"contains,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type logsOut struct {
	Entries []devapi.LogEntry `json:"entries"`
}

type tracesIn struct {
	App        string `json:"app,omitempty"`
	ErrorsOnly bool   `json:"errors_only,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type tracesOut struct {
	Traces []devapi.TraceSummary `json:"traces"`
}

type traceIn struct {
	TraceID string `json:"trace_id"`
}

type mailListOut struct {
	Mail []devapi.MailSummary `json:"mail"`
}

type mailGetIn struct {
	App string `json:"app" jsonschema:"the app's name, from the app: line of its aicoded.yaml"`
	ID  string `json:"id" jsonschema:"the message's id from mail_list"`
}

type mailReceiveIn struct {
	App     string `json:"app" jsonschema:"the app's name, from the app: line of its aicoded.yaml"`
	From    string `json:"from" jsonschema:"the sender's address"`
	Subject string `json:"subject"`
	Text    string `json:"text" jsonschema:"the plain-text body"`
}

type idOut struct {
	ID string `json:"id"`
}

type publishIn struct {
	App     string `json:"app"`
	Summary string `json:"summary"`
}

type publishOut struct {
	ID         string `json:"id"`
	App        string `json:"app"`
	SHA        string `json:"sha"`
	CreatedApp bool   `json:"created_app"`
	Next       string `json:"next"`
}

type releaseStatusIn struct {
	ID string `json:"id"`
}

// The schemas of the tools whose inputs have limits the Go types cannot state.
var (
	logsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "app": {"type": "string", "description": "only this app; leave it out for every app"},
    "level": {"type": "string", "enum": ["DEBUG", "INFO", "WARN", "ERROR"], "description": "the lowest level shown; lines that are not JSON count as INFO"},
    "trace_id": {"type": "string", "pattern": "^[0-9a-f]{32}$", "description": "only the lines of this trace"},
    "contains": {"type": "string", "description": "only lines whose message contains this text, in any case"},
    "limit": {"type": "integer", "minimum": 1, "maximum": 1000, "description": "the newest lines to return; 100 when left out"}
  },
  "additionalProperties": false
}`)
	tracesSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "app": {"type": "string", "description": "only traces this app has spans in; leave it out for every app"},
    "errors_only": {"type": "boolean", "description": "only traces with a failed span"},
    "limit": {"type": "integer", "minimum": 1, "maximum": 200, "description": "the newest traces to return; 20 when left out"}
  },
  "additionalProperties": false
}`)
	traceSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "trace_id": {"type": "string", "pattern": "^[0-9a-f]{32}$", "description": "a trace id from traces or logs"}
  },
  "required": ["trace_id"],
  "additionalProperties": false
}`)
	publishSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "app": {"type": "string", "description": "the app's name, from the app: line of its aicoded.yaml"},
    "summary": {"type": "string", "minLength": 1, "description": "what changed and why, in a sentence of the person's words, for the change record; at most 1000 characters are kept, fewer of emoji"}
  },
  "required": ["app", "summary"],
  "additionalProperties": false
}`)
	releaseStatusSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "id": {"type": "string", "pattern": "^pub_[a-z2-7]{26}$", "description": "the id that publish returned"}
  },
  "required": ["id"],
  "additionalProperties": false
}`)
)

func (s *server) addTools(srv *mcpsdk.Server) {
	no := false
	readOnly := &mcpsdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &no}
	rerun := &mcpsdk.ToolAnnotations{IdempotentHint: true, DestructiveHint: &no, OpenWorldHint: &no}
	adds := &mcpsdk.ToolAnnotations{DestructiveHint: &no, OpenWorldHint: &no}
	yes := true

	tool(srv, &mcpsdk.Tool{Name: "app_create", Annotations: adds,
		Description: "Create a new app in its own folder of this workspace, with a permission list (aicoded.yaml), a main.go, one page and AGENTS.md, the rules for AI assistants that build the app."},
		s.appCreate)
	tool(srv, &mcpsdk.Tool{Name: "check", Annotations: rerun,
		Description: "Run aicoded generate, go build, go vet, the lint rules for app code and the tests for every app, or one, and list each problem with its code, file:line and fix. It rewrites generated files unless frozen."},
		s.check)
	tool(srv, &mcpsdk.Tool{Name: "preview", Annotations: rerun,
		Description: "Build and start, or restart, an app in aicoded dev and return its state and local address, or the problems that stop it."},
		func(ctx context.Context, in appIn) (devapi.App, error) {
			if err := validApp(in.App, false); err != nil {
				return devapi.App{}, err
			}
			return withDev(ctx, s, func(b devapi.Backend) (devapi.App, error) { return b.Restart(ctx, in.App) })
		})
	tool(srv, &mcpsdk.Tool{Name: "what_broke", Annotations: readOnly,
		Description: "List what went wrong in aicoded dev: the problems of failed apps, the last error log lines and the last failed spans."},
		func(ctx context.Context, in anyAppIn) (devapi.Broken, error) {
			if err := validApp(in.App, true); err != nil {
				return devapi.Broken{}, err
			}
			return withDev(ctx, s, func(b devapi.Backend) (devapi.Broken, error) { return b.WhatBroke(ctx, in.App) })
		})
	tool(srv, &mcpsdk.Tool{Name: "describe", Annotations: readOnly,
		Description: "Summarise the apps: app type, owner and audience, pages and their access rules, functions served and called, data, mail, outside services, scheduled jobs, settings, secret names, third-party modules, size and resource limits, the tables their SQL writes, their surface, and generated files that are out of date."},
		func(ctx context.Context, in anyAppIn) (describe.Summary, error) {
			if err := validApp(in.App, true); err != nil {
				return describe.Summary{}, err
			}
			return describe.Describe(ctx, s.dir, in.App)
		})
	tool(srv, &mcpsdk.Tool{Name: "howto", Annotations: readOnly,
		Description: "Show a page of the docs: a guide such as guides/overview, a task recipe such as tasks/add-form, the changelog for AI assistants, or the page of an error code such as E-DEV-001. Without a topic, list every page with its summary."},
		howto)
	tool(srv, &mcpsdk.Tool{Name: "logs", Annotations: readOnly, InputSchema: logsSchema,
		Description: "Show the newest log lines of the apps in aicoded dev, oldest first. Secret values are hidden."},
		func(ctx context.Context, in logsIn) (logsOut, error) {
			if err := validApp(in.App, true); err != nil {
				return logsOut{}, err
			}
			q := devapi.LogQuery{App: in.App, Level: in.Level, TraceID: in.TraceID, Contains: in.Contains, Limit: in.Limit}
			es, err := withDev(ctx, s, func(b devapi.Backend) ([]devapi.LogEntry, error) { return b.Logs(ctx, q) })
			return logsOut{Entries: es}, err
		})
	tool(srv, &mcpsdk.Tool{Name: "traces", Annotations: readOnly, InputSchema: tracesSchema,
		Description: "List the newest traces in aicoded dev: each request or call across the apps, with its root span, apps, span count and duration."},
		func(ctx context.Context, in tracesIn) (tracesOut, error) {
			if err := validApp(in.App, true); err != nil {
				return tracesOut{}, err
			}
			q := devapi.TraceQuery{App: in.App, ErrorsOnly: in.ErrorsOnly, Limit: in.Limit}
			ts, err := withDev(ctx, s, func(b devapi.Backend) ([]devapi.TraceSummary, error) { return b.Traces(ctx, q) })
			return tracesOut{Traces: ts}, err
		})
	tool(srv, &mcpsdk.Tool{Name: "trace", Annotations: readOnly, InputSchema: traceSchema,
		Description: "Show every span of one trace, in start order."},
		func(ctx context.Context, in traceIn) (devapi.Trace, error) {
			return withDev(ctx, s, func(b devapi.Backend) (devapi.Trace, error) { return b.Trace(ctx, in.TraceID) })
		})
	tool(srv, &mcpsdk.Tool{Name: "mail_list", Annotations: readOnly,
		Description: "List the mail an app sent, which aicoded dev catches instead of sending, and its mailbox, newest first."},
		func(ctx context.Context, in appIn) (mailListOut, error) {
			if err := validApp(in.App, false); err != nil {
				return mailListOut{}, err
			}
			ms, err := withDev(ctx, s, func(b devapi.Backend) ([]devapi.MailSummary, error) { return b.Mail(ctx, in.App) })
			return mailListOut{Mail: ms}, err
		})
	tool(srv, &mcpsdk.Tool{Name: "mail_get", Annotations: readOnly,
		Description: "Show one message from mail_list."},
		func(ctx context.Context, in mailGetIn) (devapi.Mail, error) {
			if err := validApp(in.App, false); err != nil {
				return devapi.Mail{}, err
			}
			return withDev(ctx, s, func(b devapi.Backend) (devapi.Mail, error) { return b.MailGet(ctx, in.App, in.ID) })
		})
	tool(srv, &mcpsdk.Tool{Name: "mail_receive", Annotations: adds,
		Description: "Deliver a test message to an app's inbox, as if it had arrived by mail."},
		func(ctx context.Context, in mailReceiveIn) (idOut, error) {
			if err := validApp(in.App, false); err != nil {
				return idOut{}, err
			}
			m := devapi.InboundMail{From: in.From, Subject: in.Subject, Text: in.Text}
			id, err := withDev(ctx, s, func(b devapi.Backend) (string, error) { return b.MailReceive(ctx, in.App, m) })
			return idOut{ID: id}, err
		})
	tool(srv, &mcpsdk.Tool{Name: "publish", Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: &no, OpenWorldHint: &yes},
		InputSchema: publishSchema,
		Description: "Send the committed app to the platform's delivery pipeline, only when the person asks: it refuses changes that are not committed, runs aicoded check --frozen without the tests, sends the commit and returns the publish's id at once. The first publish of a name creates the app."},
		s.publish)
	tool(srv, &mcpsdk.Tool{Name: "release_status", Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &yes},
		InputSchema: releaseStatusSchema,
		Description: "Show a publish: its status (queued, running, passed, failed, refused or error), each step with its outcome, the problems with their code, file:line and fix, and the notes, in the same shape, which do not stop the publish."},
		s.releaseStatus)
}

// tool adds a tool whose error reaches the client as a tool error with the text of its problems.
func tool[In, Out any](srv *mcpsdk.Server, t *mcpsdk.Tool, f func(context.Context, In) (Out, error)) {
	mcpsdk.AddTool(srv, t, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in In) (*mcpsdk.CallToolResult, Out, error) {
		out, err := f(ctx, in)
		if err != nil {
			var zero Out
			return nil, zero, toolError(err)
		}
		return nil, out, nil
	})
}

// toolError is err as a tool shows it: the text of each problem in it, with its fix and docs.
func toolError(err error) error {
	var lines []string
	for _, p := range problem.From("", err) {
		lines = append(lines, p.String())
	}
	return errors.New(strings.Join(lines, "\n"))
}

func (s *server) appCreate(ctx context.Context, in appCreateIn) (appCreateOut, error) {
	dir, err := scaffold.Create(ctx, s.dir, in.Name, s.fw, s.logs)
	if err != nil {
		return appCreateOut{}, err
	}
	return appCreateOut{Dir: dir, Next: fmt.Sprintf("read its AGENTS.md, then call preview with app %q to build and open it, edit its pages and call check", in.Name)}, nil
}

func (s *server) check(ctx context.Context, in checkIn) (check.Report, error) {
	if err := validApp(in.App, true); err != nil {
		return check.Report{}, err
	}
	return check.Run(ctx, s.dir, check.Options{Frozen: in.Frozen, App: in.App, FrameworkDir: s.fw.Dir})
}

func (s *server) publish(ctx context.Context, in publishIn) (publishOut, error) {
	if err := validApp(in.App, false); err != nil {
		return publishOut{}, err
	}
	apps, err := workspace.Apps(s.dir)
	if err != nil {
		return publishOut{}, err
	}
	app, err := workspace.Select(apps, in.App)
	if err != nil {
		return publishOut{}, err
	}
	c, err := s.platformClient()
	if err != nil {
		return publishOut{}, err
	}
	cm, err := publish.Prepare(ctx, app[0].Dir, publish.Options{Summary: in.Summary})
	if ce := (*publish.CheckError)(nil); errors.As(err, &ce) {
		return publishOut{}, checkFailed(ce)
	}
	if err != nil {
		return publishOut{}, err
	}
	sent, err := publish.Send(ctx, c, cm)
	if err != nil {
		return publishOut{}, err
	}
	return publishOut{ID: sent.ID, App: sent.App, SHA: sent.SHA, CreatedApp: sent.CreatedApp,
		Next: fmt.Sprintf("call release_status with id %q about every 30 seconds until its status is passed, failed, refused or error, then tell the person the outcome; the delivery pipeline takes a few minutes", sent.ID)}, nil
}

// checkFailed is the tool error of a publish that aicoded check refused: each problem, then
// E-PUB-004.
func checkFailed(ce *publish.CheckError) error {
	var lines []string
	for _, a := range ce.Report.Apps {
		for _, p := range a.Problems {
			lines = append(lines, p.String())
		}
	}
	return errors.New(strings.Join(append(lines, ce.Error()), "\n"))
}

func (s *server) releaseStatus(ctx context.Context, in releaseStatusIn) (platform.Publish, error) {
	if !platform.PublishID.MatchString(in.ID) {
		return platform.Publish{}, errors.New("the id of a publish is pub_ and 26 lowercase letters and digits")
	}
	c, err := s.platformClient()
	if err != nil {
		return platform.Publish{}, err
	}
	return c.Publish(ctx, in.ID)
}

// platformClient returns a client of the platform that AICODED_PLATFORM names.
func (s *server) platformClient() (*platform.Client, error) {
	address, err := platform.Address()
	if err != nil {
		return nil, err
	}
	return platform.New(address, s.logs), nil
}

// validApp refuses, with E-DEV-014, a name that no app can have. An empty name passes when the
// tool takes every app.
func validApp(app string, every bool) error {
	if (every && app == "") || manifest.ValidApp(app) {
		return nil
	}
	return workspace.UnknownApp(app)
}
