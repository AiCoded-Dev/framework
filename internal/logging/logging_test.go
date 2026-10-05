package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/logging"
	"aicoded.dev/framework/internal/tracectx"
)

func TestLoggerAddsTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	log := logging.New(&buf)
	sc, ok := tracectx.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.True(t, ok)

	log.With("k", "v").InfoContext(tracectx.With(context.Background(), sc), "traced")
	log.InfoContext(context.Background(), "plain")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2)
	var traced, plain map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &traced))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &plain))
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", traced["trace_id"])
	assert.Equal(t, "00f067aa0ba902b7", traced["span_id"])
	assert.Equal(t, "v", traced["k"])
	assert.NotContains(t, plain, "trace_id")
}

func TestLoggerKeepsTraceIDsTopLevelInGroups(t *testing.T) {
	var buf bytes.Buffer
	sc, ok := tracectx.Parse("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.True(t, ok)

	logging.New(&buf).With("app", "rooms").WithGroup("http").With("method", "GET").WithGroup("req").
		InfoContext(tracectx.With(context.Background(), sc), "grouped", "path", "/x")

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", got["trace_id"])
	assert.Equal(t, "00f067aa0ba902b7", got["span_id"])
	assert.Equal(t, "rooms", got["app"])
	assert.Equal(t, map[string]any{"method": "GET", "req": map[string]any{"path": "/x"}}, got["http"])
}
