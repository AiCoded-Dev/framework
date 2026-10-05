package tracectx_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/tracectx"
)

const valid = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestParse(t *testing.T) {
	sc, ok := tracectx.Parse(valid)
	require.True(t, ok)
	assert.True(t, sc.Sampled)
	assert.Equal(t, valid, sc.Traceparent())

	for _, h := range []string{
		"",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
		"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e473z-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"00_4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	} {
		_, ok := tracectx.Parse(h)
		assert.False(t, ok, "input %q", h)
	}
}

func TestContext(t *testing.T) {
	_, ok := tracectx.From(context.Background())
	assert.False(t, ok)

	sc := tracectx.SpanContext{TraceID: tracectx.NewTraceID(), SpanID: tracectx.NewSpanID()}
	require.True(t, sc.Valid())
	got, ok := tracectx.From(tracectx.With(context.Background(), sc))
	require.True(t, ok)
	assert.Equal(t, sc, got)
}
