package server

import (
	"maps"
	"math"
	"slices"

	"aicoded.dev/framework/runnerproto/runnerv1"
)

// The bounds of the spans an app exports.
const (
	maxSpans      = 1024
	maxSpanText   = 4 << 10
	maxAttributes = 64
	maxAttrKey    = 256
)

// boundSpans returns the spans that keep to the bounds Telemetry describes, with their text cut
// and their extra attributes left out, and how many it dropped: those past maxSpans, and those
// with an id of the wrong size.
func boundSpans(spans []*runnerv1.Span) ([]*runnerv1.Span, uint64) {
	var out []*runnerv1.Span
	var dropped uint64
	for _, sp := range spans {
		if len(out) == maxSpans || !idSize(sp.GetTraceId(), 16) || !idSize(sp.GetSpanId(), 8) || !idSize(sp.GetParentSpanId(), 8) {
			dropped++
			continue
		}
		sp.Name, sp.Error = cut(sp.GetName(), maxSpanText), cut(sp.GetError(), maxSpanText)
		sp.Attributes = boundAttributes(sp.GetAttributes())
		out = append(out, sp)
	}
	return out, dropped
}

// idSize reports whether id is empty or n bytes.
func idSize(id []byte, n int) bool { return len(id) == 0 || len(id) == n }

// boundAttributes returns the first maxAttributes of attrs by key, leaving out keys over
// maxAttrKey bytes, with each value cut at maxSpanText bytes.
func boundAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return attrs
	}
	out := make(map[string]string, min(len(attrs), maxAttributes))
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		if len(out) == maxAttributes {
			break
		}
		if len(k) <= maxAttrKey {
			out[k] = cut(attrs[k], maxSpanText)
		}
	}
	return out
}

// addSaturating returns a+b, or the largest uint64 when the sum does not fit.
func addSaturating(a, b uint64) uint64 {
	if a > math.MaxUint64-b {
		return math.MaxUint64
	}
	return a + b
}
