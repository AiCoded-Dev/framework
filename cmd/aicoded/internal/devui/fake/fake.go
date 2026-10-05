// Package fake is a backend of the dev UI in memory, for the tests of its pages.
package fake

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devconfig"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
)

var _ deps.Backend = (*Backend)(nil)

// Data is what a Backend answers.
type Data struct {
	Status devapi.Status
	// Logs answers every query of Logs, and Traces every query of Traces; Calls holds the
	// queries.
	Logs   []devapi.LogEntry
	Traces []devapi.TraceSummary
	// Spans holds the spans Trace finds by trace id.
	Spans []devapi.Span
	// Mail holds the messages of each app, in the order Mail lists them.
	Mail     map[string][]devapi.Mail
	Broken   devapi.Broken
	Personas []devconfig.Persona
	// Errors holds the error each method, by name, fails with.
	Errors map[string]error
}

// Call is one call a Backend got.
type Call struct {
	Method string
	App    string
	// Arg is the call's other argument: the query of Logs and Traces, the id of Trace and
	// MailGet, the message of MailReceive and the switch of SetManual.
	Arg any
}

// needsApp holds the methods that fail with E-DEV-014 for "", as the workspace's do.
var needsApp = map[string]bool{
	"Restart": true, "Mail": true, "MailGet": true, "MailReceive": true, "Stop": true, "Start": true, "SetManual": true,
}

// Backend is a deps.Backend that answers from Data and records every call but those of
// Personas and Changed. A method fails with the error Data.Errors holds for it, and with
// E-DEV-014 for an app Data.Status does not list. Set Data before the Backend is shared and
// change it only through Update after. A Backend is safe for concurrent use.
type Backend struct {
	Data Data

	mu      sync.Mutex
	calls   []Call
	changed chan struct{}
}

// Update changes the data while it holds the Backend's lock, then calls Change.
func (b *Backend) Update(f func(*Data)) {
	b.mu.Lock()
	f(&b.Data)
	b.mu.Unlock()
	b.Change()
}

// Change closes the channel Changed returned, as the workspace does at every change.
func (b *Backend) Change() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.changed != nil {
		close(b.changed)
	}
	b.changed = make(chan struct{})
}

// Calls returns the calls to the methods named, or every call when none is named, in order.
func (b *Backend) Calls(methods ...string) []Call {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Call
	for _, c := range b.calls {
		if len(methods) == 0 || slices.Contains(methods, c.Method) {
			out = append(out, c)
		}
	}
	return out
}

// answer records c and returns what v returns, unless the call fails.
func answer[T any](b *Backend, c Call, v func(d *Data) (T, error)) (T, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, c)
	var zero T
	if err := b.Data.Errors[c.Method]; err != nil {
		return zero, err
	}
	if c.App != "" || needsApp[c.Method] {
		if !slices.ContainsFunc(b.Data.Status.Apps, func(a devapi.App) bool { return a.Name == c.App }) {
			return zero, workspace.UnknownApp(c.App)
		}
	}
	return v(&b.Data)
}

// none answers a call that returns only an error.
func none(*Data) (struct{}, error) { return struct{}{}, nil }

// Status returns Data.Status.
func (b *Backend) Status(context.Context) (devapi.Status, error) {
	return answer(b, Call{Method: "Status"}, func(d *Data) (devapi.Status, error) {
		st := d.Status
		st.Apps = slices.Clone(st.Apps)
		return st, nil
	})
}

// Restart returns the status of app.
func (b *Backend) Restart(_ context.Context, app string) (devapi.App, error) {
	return answer(b, Call{Method: "Restart", App: app}, func(d *Data) (devapi.App, error) {
		i := slices.IndexFunc(d.Status.Apps, func(a devapi.App) bool { return a.Name == app })
		return d.Status.Apps[i], nil
	})
}

// Logs returns Data.Logs.
func (b *Backend) Logs(_ context.Context, q devapi.LogQuery) ([]devapi.LogEntry, error) {
	return answer(b, Call{Method: "Logs", App: q.App, Arg: q}, func(d *Data) ([]devapi.LogEntry, error) {
		return slices.Clone(d.Logs), nil
	})
}

// Traces returns Data.Traces.
func (b *Backend) Traces(_ context.Context, q devapi.TraceQuery) ([]devapi.TraceSummary, error) {
	return answer(b, Call{Method: "Traces", App: q.App, Arg: q}, func(d *Data) ([]devapi.TraceSummary, error) {
		return slices.Clone(d.Traces), nil
	})
}

// Trace returns the spans of Data.Spans whose trace id is id.
func (b *Backend) Trace(_ context.Context, id string) (devapi.Trace, error) {
	return answer(b, Call{Method: "Trace", Arg: id}, func(d *Data) (devapi.Trace, error) {
		spans := slices.DeleteFunc(slices.Clone(d.Spans), func(sp devapi.Span) bool { return sp.TraceID != id })
		if len(spans) == 0 {
			return devapi.Trace{}, fmt.Errorf("no trace %s", id)
		}
		return devapi.Trace{TraceID: id, Spans: spans}, nil
	})
}

// Mail returns the summaries of the messages of app.
func (b *Backend) Mail(_ context.Context, app string) ([]devapi.MailSummary, error) {
	return answer(b, Call{Method: "Mail", App: app}, func(d *Data) ([]devapi.MailSummary, error) {
		out := []devapi.MailSummary{}
		for _, m := range d.Mail[app] {
			out = append(out, m.MailSummary)
		}
		return out, nil
	})
}

// MailGet returns the message id of app.
func (b *Backend) MailGet(_ context.Context, app, id string) (devapi.Mail, error) {
	return answer(b, Call{Method: "MailGet", App: app, Arg: id}, func(d *Data) (devapi.Mail, error) {
		i := slices.IndexFunc(d.Mail[app], func(m devapi.Mail) bool { return m.ID == id })
		if i < 0 {
			return devapi.Mail{}, fmt.Errorf("%s has no message %q", app, id)
		}
		return d.Mail[app][i], nil
	})
}

// MailReceive adds m to the end of the messages of app, in the folder "inbox", and calls
// Change.
func (b *Backend) MailReceive(_ context.Context, app string, m devapi.InboundMail) (string, error) {
	id, err := answer(b, Call{Method: "MailReceive", App: app, Arg: m}, func(d *Data) (string, error) {
		if d.Mail == nil {
			d.Mail = map[string][]devapi.Mail{}
		}
		id := fmt.Sprintf("inbox-%d", len(d.Mail[app])+1)
		d.Mail[app] = append(d.Mail[app], devapi.Mail{
			MailSummary: devapi.MailSummary{ID: id, Folder: "inbox", From: m.From, Subject: m.Subject, Time: time.Now().UTC()},
			Text:        m.Text,
		})
		return id, nil
	})
	if err == nil {
		b.Change()
	}
	return id, err
}

// WhatBroke returns Data.Broken.
func (b *Backend) WhatBroke(_ context.Context, app string) (devapi.Broken, error) {
	return answer(b, Call{Method: "WhatBroke", App: app}, func(d *Data) (devapi.Broken, error) {
		return d.Broken, nil
	})
}

// Stop records the call.
func (b *Backend) Stop(app string) error {
	_, err := answer(b, Call{Method: "Stop", App: app}, none)
	return err
}

// Start records the call.
func (b *Backend) Start(app string) error {
	_, err := answer(b, Call{Method: "Start", App: app}, none)
	return err
}

// SetManual records the call.
func (b *Backend) SetManual(app string, on bool) error {
	_, err := answer(b, Call{Method: "SetManual", App: app, Arg: on}, none)
	return err
}

// Personas returns Data.Personas.
func (b *Backend) Personas() []devconfig.Persona {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.Data.Personas)
}

// Changed returns a channel that Change closes.
func (b *Backend) Changed() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.changed == nil {
		b.changed = make(chan struct{})
	}
	return b.changed
}
