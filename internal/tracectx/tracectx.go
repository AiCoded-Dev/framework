// Package tracectx holds W3C trace context: it parses and formats traceparent headers
// and carries the current span in a context.
package tracectx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// SpanContext identifies a span within a trace.
type SpanContext struct {
	TraceID [16]byte
	SpanID  [8]byte
	Sampled bool
}

// Valid reports whether both IDs are non-zero.
func (sc SpanContext) Valid() bool {
	return sc.TraceID != [16]byte{} && sc.SpanID != [8]byte{}
}

// Traceparent formats sc as a version-00 traceparent header.
func (sc SpanContext) Traceparent() string {
	flags := "00"
	if sc.Sampled {
		flags = "01"
	}
	return "00-" + hex.EncodeToString(sc.TraceID[:]) + "-" + hex.EncodeToString(sc.SpanID[:]) + "-" + flags
}

// Parse parses a version-00 traceparent header. It returns false for anything else,
// so a malformed header starts a new trace instead of failing the request.
func Parse(h string) (SpanContext, bool) {
	var sc SpanContext
	if len(h) != 55 || h[:3] != "00-" || h[35] != '-' || h[52] != '-' {
		return SpanContext{}, false
	}
	var flags [1]byte
	if !decodeLowerHex(sc.TraceID[:], h[3:35]) || !decodeLowerHex(sc.SpanID[:], h[36:52]) || !decodeLowerHex(flags[:], h[53:]) {
		return SpanContext{}, false
	}
	sc.Sampled = flags[0]&1 == 1
	if !sc.Valid() {
		return SpanContext{}, false
	}
	return sc, true
}

func decodeLowerHex(dst []byte, s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	_, err := hex.Decode(dst, []byte(s))
	return err == nil
}

// NewTraceID returns a random trace ID.
func NewTraceID() (id [16]byte) {
	_, _ = rand.Read(id[:])
	return id
}

// NewSpanID returns a random span ID.
func NewSpanID() (id [8]byte) {
	_, _ = rand.Read(id[:])
	return id
}

type ctxKey struct{}

// With returns a copy of ctx carrying sc as the current span.
func With(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, sc)
}

// From returns the current span of ctx.
func From(ctx context.Context) (SpanContext, bool) {
	sc, ok := ctx.Value(ctxKey{}).(SpanContext)
	return sc, ok
}
