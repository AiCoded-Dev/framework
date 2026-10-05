// Package devapi holds what aicoded dev tells about the apps it runs, on its control socket and
// to aicoded mcp.
package devapi

import (
	"context"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/problem"
)

// State is where an app of the workspace is in its cycle of generate, build and start.
type State string

// The states of an app.
const (
	Starting State = "starting"
	Running  State = "running"
	Failed   State = "failed"
	Stopped  State = "stopped"
	// Manual is an app the developer runs by hand: aicoded dev keeps its runner up and never
	// builds or starts it.
	Manual State = "manual"
)

// App is the status of one app of the workspace.
type App struct {
	Name      string            `json:"name"`
	Dir       string            `json:"dir"`
	State     State             `json:"state"`
	URL       string            `json:"url,omitempty"` // "" for an app with no pages
	CallsOnly bool              `json:"calls_only,omitempty"`
	Problems  []problem.Problem `json:"problems,omitempty"` // why it failed
	Since     time.Time         `json:"since"`              // when it entered State
	// Command runs the app by hand, and RunnerDir is its AICODED_RUNNER_DIR; both only in
	// state manual.
	Command   string `json:"command,omitempty"`
	RunnerDir string `json:"runner_dir,omitempty"`
}

// Status is the status of the workspace.
type Status struct {
	Root    string `json:"root"`
	Gateway string `json:"gateway"` // http://localhost:<port>/
	Apps    []App  `json:"apps"`    // in folder order
}

// LogEntry is one line an app wrote, or one aicoded dev wrote about it.
type LogEntry struct {
	Time    time.Time      `json:"time"`
	App     string         `json:"app"`
	Source  string         `json:"source"`          // "app", "build" or "runner"
	Level   string         `json:"level,omitempty"` // DEBUG, INFO, WARN, ERROR; "" for a plain line
	Message string         `json:"message"`
	TraceID string         `json:"trace_id,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// LogQuery selects log entries.
type LogQuery struct {
	App, Level, TraceID, Contains string // Level is the lowest level shown
	Limit                         int    // default 100, at most 1000
}

// Span is one span an app exported.
type Span struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentID   string            `json:"parent_id,omitempty"`
	App        string            `json:"app"`
	Name       string            `json:"name"`
	Start      time.Time         `json:"start"`
	DurationMS float64           `json:"duration_ms"`
	Attrs      map[string]string `json:"attrs,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// TraceSummary sums up the spans of one trace.
type TraceSummary struct {
	TraceID    string    `json:"trace_id"`
	Root       string    `json:"root"`
	Apps       []string  `json:"apps"`
	Spans      int       `json:"spans"`
	Start      time.Time `json:"start"`
	DurationMS float64   `json:"duration_ms"`
	Error      bool      `json:"error,omitempty"`
}

// TraceQuery selects traces.
type TraceQuery struct {
	App        string
	ErrorsOnly bool
	Limit      int // default 20, at most 200
}

// Trace is every span of one trace, by start.
type Trace struct {
	TraceID string `json:"trace_id"`
	Spans   []Span `json:"spans"`
}

// MailSummary sums up one message an app sent or has in its mailbox.
type MailSummary struct {
	ID      string    `json:"id"`
	Folder  string    `json:"folder"` // "sent", or the inbox folder
	From    string    `json:"from"`
	To      []string  `json:"to"`
	Subject string    `json:"subject"`
	Time    time.Time `json:"time"`
}

// Mail is one message.
type Mail struct {
	MailSummary
	Cc          []string     `json:"cc,omitempty"`
	Bcc         []string     `json:"bcc,omitempty"`
	ReplyTo     string       `json:"reply_to,omitempty"`
	Text        string       `json:"text,omitempty"`
	HTML        string       `json:"html,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment describes an attachment; its content is never shown.
type Attachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// InboundMail is a message that arrives in an app's inbox.
type InboundMail struct{ From, Subject, Text string }

// Broken is what went wrong in the workspace lately.
type Broken struct {
	Problems []problem.Problem `json:"problems"`     // failed apps
	Errors   []LogEntry        `json:"errors"`       // last 20 ERROR entries
	Failed   []Span            `json:"failed_spans"` // last 20 spans with an error
}

// Backend answers the questions about a running aicoded dev.
type Backend interface {
	Status(ctx context.Context) (Status, error)
	Restart(ctx context.Context, app string) (App, error)
	Logs(ctx context.Context, q LogQuery) ([]LogEntry, error)
	Traces(ctx context.Context, q TraceQuery) ([]TraceSummary, error)
	Trace(ctx context.Context, id string) (Trace, error)
	Mail(ctx context.Context, app string) ([]MailSummary, error)
	MailGet(ctx context.Context, app, id string) (Mail, error)
	MailReceive(ctx context.Context, app string, m InboundMail) (string, error)
	WhatBroke(ctx context.Context, app string) (Broken, error)
}
