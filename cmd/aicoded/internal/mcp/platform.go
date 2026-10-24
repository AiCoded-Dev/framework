package mcp

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/platform"
)

// platform reports whether the fields read an app on the platform.
func (o onPlatform) platform() bool { return slices.Contains(platform.Envs, o.Env) }

// dev refuses the fields of the platform, and an env that is none, in aicoded dev.
func (o onPlatform) dev() error {
	switch {
	case o.Env != "" && o.Env != "dev":
		return errors.New("env is dev, preview or production")
	case o.Since != "" || o.Until != "":
		return errors.New("since and until read the platform, with env preview or production; aicoded dev keeps the newest rows")
	}
	return nil
}

// query returns the query of the app on the platform, with the times of o.
func (o onPlatform) query(app string) (platform.TelemetryQuery, error) {
	if err := validApp(app, false); err != nil {
		return platform.TelemetryQuery{}, err
	}
	now := time.Now()
	since, err := platform.When("since", o.Since, now)
	if err != nil {
		return platform.TelemetryQuery{}, err
	}
	until, err := platform.When("until", o.Until, now)
	return platform.TelemetryQuery{App: app, Env: o.Env, Since: since, Until: until}, err
}

// platformLogs returns the log lines of an app on the platform, oldest first.
func (s *server) platformLogs(ctx context.Context, in logsIn) ([]devapi.LogEntry, error) {
	if in.Contains != "" {
		return nil, errors.New("contains works in aicoded dev only; on the platform, narrow the lines with level, trace_id, since and until")
	}
	q, err := in.query(in.App)
	if err != nil {
		return nil, err
	}
	q.Level, q.Trace, q.Limit = in.Level, in.TraceID, in.Limit
	c, err := s.platformClient()
	if err != nil {
		return nil, err
	}
	lines, err := c.Logs(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]devapi.LogEntry, 0, len(lines))
	for _, l := range slices.Backward(lines) {
		e := devapi.LogEntry{Time: l.Time, App: q.App, Source: "app", Level: l.Level, Message: l.Body, TraceID: l.TraceID}
		if len(l.Attributes) > 0 {
			e.Attrs = map[string]any{}
			for k, v := range l.Attributes {
				e.Attrs[k] = v
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// platformTraces returns the newest traces of an app on the platform, newest first, from its
// newest 1000 spans.
func (s *server) platformTraces(ctx context.Context, in tracesIn) ([]devapi.TraceSummary, error) {
	if in.Limit < 0 || in.Limit > 200 {
		return nil, errors.New("limit is 1 to 200 traces")
	}
	q, err := in.query(in.App)
	if err != nil {
		return nil, err
	}
	q.Limit = platform.MaxRows
	c, err := s.platformClient()
	if err != nil {
		return nil, err
	}
	spans, err := c.Spans(ctx, q)
	if err != nil {
		return nil, err
	}
	byTrace := map[string][]devapi.Span{}
	for _, sp := range spans {
		byTrace[sp.TraceID] = append(byTrace[sp.TraceID], devSpan(q.App, sp))
	}
	out := []devapi.TraceSummary{}
	for id, spans := range byTrace {
		if sum := devapi.Summarize(id, spans); sum.Error || !in.ErrorsOnly {
			out = append(out, sum)
		}
	}
	slices.SortFunc(out, func(a, b devapi.TraceSummary) int {
		return cmp.Or(b.Start.Compare(a.Start), strings.Compare(a.TraceID, b.TraceID))
	})
	return out[:min(cmp.Or(in.Limit, 20), len(out))], nil
}

// platformTrace returns the spans of an app on the platform in one trace, in start order.
func (s *server) platformTrace(ctx context.Context, in traceIn) (devapi.Trace, error) {
	q, err := in.query(in.App)
	if err != nil {
		return devapi.Trace{}, err
	}
	q.Trace = in.TraceID
	c, err := s.platformClient()
	if err != nil {
		return devapi.Trace{}, err
	}
	spans, err := c.Trace(ctx, q)
	if err != nil {
		return devapi.Trace{}, err
	}
	out := devapi.Trace{TraceID: in.TraceID, Spans: []devapi.Span{}}
	for _, sp := range slices.Backward(spans) {
		out.Spans = append(out.Spans, devSpan(q.App, sp))
	}
	slices.SortStableFunc(out.Spans, func(a, b devapi.Span) int { return a.Start.Compare(b.Start) })
	return out, nil
}

// devSpan is sp, a span of app on the platform, as aicoded dev shows spans: a failed span's Error
// is its status message, or "failed".
func devSpan(app string, sp platform.Span) devapi.Span {
	out := devapi.Span{TraceID: sp.TraceID, SpanID: sp.SpanID, ParentID: sp.ParentSpanID, App: app, Name: sp.Name, Start: sp.Time,
		DurationMS: float64(sp.DurationNS) / float64(time.Millisecond), Attrs: sp.Attributes}
	if sp.Status == "Error" {
		out.Error = cmp.Or(sp.StatusMessage, "failed")
	}
	return out
}
