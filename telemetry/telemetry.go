package telemetry

import (
	"context"
	"sync"
	"time"

	"aicoded.dev/framework/internal/redact"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/internal/tracectx"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// Span is one timed operation. End it once; SetAttr and RecordError after End do nothing,
// and a second End does nothing.
type Span struct {
	sc     tracectx.SpanContext
	parent [8]byte
	name   string
	start  time.Time
	export func(*runnerv1.Span)
	dev    bool

	mu    sync.Mutex
	attrs map[string]string
	err   string
	ended bool
}

// Start begins a span named name as a child of the current span of ctx, or as a new trace.
// The returned context carries the new span.
func Start(ctx context.Context, name string) (context.Context, *Span) {
	s := &Span{name: name, start: time.Now(), dev: redact.Dev(ctx)}
	if parent, ok := tracectx.From(ctx); ok {
		s.sc.TraceID, s.parent, s.sc.Sampled = parent.TraceID, parent.SpanID, parent.Sampled
	} else {
		s.sc.TraceID, s.sc.Sampled = tracectx.NewTraceID(), true
	}
	s.sc.SpanID = tracectx.NewSpanID()
	if rs := runner.From(ctx); rs != nil {
		s.export = rs.Export
	}
	return tracectx.With(ctx, s.sc), s
}

// SetAttr records an attribute. It does nothing after End.
func (s *Span) SetAttr(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	if s.attrs == nil {
		s.attrs = map[string]string{}
	}
	s.attrs[key] = value
}

// RecordError marks the span as failed; a nil err is ignored. It does nothing after End. The
// span keeps the error's text under aicoded dev in environment `dev`, and anywhere else only its
// kinds and codes, such as E-codes, MySQL error numbers and status codes, since the text can quote
// personal data.
func (s *Span) RecordError(err error) {
	if err == nil {
		return
	}
	var text string
	if s.dev {
		text = err.Error()
	} else {
		text = redact.Error(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.err = text
}

// End finishes the span and hands it to the runner.
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	traceID, spanID, parent := s.sc.TraceID, s.sc.SpanID, s.parent
	pb := &runnerv1.Span{
		TraceId:       traceID[:],
		SpanId:        spanID[:],
		Name:          s.name,
		StartUnixNano: s.start.UnixNano(),
		EndUnixNano:   s.start.Add(time.Since(s.start)).UnixNano(),
		Attributes:    s.attrs,
		Error:         s.err,
	}
	if parent != ([8]byte{}) {
		pb.ParentSpanId = parent[:]
	}
	s.mu.Unlock()
	if s.export != nil {
		s.export(pb)
	}
}
