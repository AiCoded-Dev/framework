package server

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/runnerproto/runnerv1"
)

func TestExportBoundsSpans(t *testing.T) {
	c, f := serve(t)
	trace, id := bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 8)
	attrs := map[string]string{strings.Repeat("k", maxAttrKey+1): "x", "long": strings.Repeat("v", maxSpanText+1), "zz": "x"}
	for i := range maxAttributes - 1 {
		attrs["a"+strconv.Itoa(100+i)] = "x"
	}
	long := "x" + strings.Repeat("é", maxSpanText)
	spans := []*runnerv1.Span{
		{TraceId: trace, SpanId: id, ParentSpanId: id, Name: long, Error: long, Attributes: attrs},
		{Name: "no ids"},
		{TraceId: trace[:15], SpanId: id, Name: "short trace id"},
		{TraceId: trace, SpanId: append(id, 0), Name: "long span id"},
		{TraceId: trace, SpanId: id, ParentSpanId: id[:1], Name: "short parent id"},
	}
	_, err := c.tel.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: spans, Dropped: 2}))
	require.NoError(t, err)
	got, dropped := f.telemetry.got()
	require.Len(t, got, 2)
	assert.Equal(t, uint64(2+3), dropped, "the app's drops and the server's")
	sp := got[0]
	assert.Len(t, sp.GetName(), maxSpanText-1, "cut, and never inside a character")
	assert.Len(t, sp.GetError(), maxSpanText-1)
	assert.Len(t, sp.GetAttributes(), maxAttributes)
	assert.Equal(t, "x", sp.GetAttributes()["a100"])
	assert.Len(t, sp.GetAttributes()["long"], maxSpanText)
	assert.NotContains(t, sp.GetAttributes(), strings.Repeat("k", maxAttrKey+1), "a long key is left out")
	assert.NotContains(t, sp.GetAttributes(), "zz", "the attributes past the bound by key are left out")
	assert.Equal(t, "no ids", got[1].GetName())

	many := make([]*runnerv1.Span, maxSpans+5)
	for i := range many {
		many[i] = &runnerv1.Span{Name: "s"}
	}
	_, err = c.tel.Export(t.Context(), connect.NewRequest(&runnerv1.ExportRequest{Spans: many, Dropped: 1<<64 - 1}))
	require.NoError(t, err)
	got, _ = f.telemetry.got()
	assert.Len(t, got, 2+maxSpans)
}

func TestAddSaturating(t *testing.T) {
	assert.Equal(t, uint64(5), addSaturating(2, 3))
	assert.Equal(t, uint64(1<<64-1), addSaturating(1<<64-1, 5))
}
