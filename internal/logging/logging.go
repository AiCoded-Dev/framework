// Package logging builds the framework's logger: JSON lines on the given writer, with the
// trace and span IDs of the context added at the top level of every record, also when the
// logger has groups.
package logging

import (
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"slices"

	"aicoded.dev/framework/internal/tracectx"
)

// New returns a logger writing JSON to w.
func New(w io.Writer) *slog.Logger {
	return slog.New(handler{base: slog.NewJSONHandler(w, nil)})
}

// handler keeps groups itself instead of passing them to base, so the trace IDs it adds
// stay at the top level of the record.
type handler struct {
	base   slog.Handler
	groups []string      // open groups, outermost first
	attrs  [][]slog.Attr // attributes added inside each open group
}

func (h handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.base.Enabled(ctx, l)
}

func (h handler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	var own []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		own = append(own, a)
		return true
	})
	if len(h.groups) == 0 {
		out.AddAttrs(own...)
	} else {
		out.AddAttrs(h.nest(own))
	}
	if sc, ok := tracectx.From(ctx); ok {
		out.AddAttrs(
			slog.String("trace_id", hex.EncodeToString(sc.TraceID[:])),
			slog.String("span_id", hex.EncodeToString(sc.SpanID[:])),
		)
	}
	return h.base.Handle(ctx, out)
}

// nest wraps attrs in the open groups, innermost first, each with its own attributes.
func (h handler) nest(attrs []slog.Attr) slog.Attr {
	last := len(h.groups) - 1
	g := slog.Attr{Key: h.groups[last], Value: slog.GroupValue(append(slices.Clone(h.attrs[last]), attrs...)...)}
	for i := last - 1; i >= 0; i-- {
		g = slog.Attr{Key: h.groups[i], Value: slog.GroupValue(append(slices.Clone(h.attrs[i]), g)...)}
	}
	return g
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.groups) == 0 {
		return handler{base: h.base.WithAttrs(attrs)}
	}
	h.attrs = slices.Clone(h.attrs)
	last := len(h.attrs) - 1
	h.attrs[last] = append(slices.Clone(h.attrs[last]), attrs...)
	return h
}

func (h handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h.groups = append(slices.Clone(h.groups), name)
	h.attrs = append(slices.Clone(h.attrs), nil)
	return h
}
