package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
)

var traceID = strings.Repeat("ab", 16)

// demoDir returns a folder that holds the permission list of demo.
func demoDir(t *testing.T) string {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aicoded.yaml"), []byte("app: demo\n"), 0o600))
	return dir
}

func TestRunLogs(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.Telemetry("/v1/apps/demo/logs",
		map[string]any{"time": "2026-10-24T12:00:01Z", "level": "ERROR", "body": "failed\x1b[2J", "trace_id": traceID, "env": "production"},
		map[string]any{"time": "2026-10-24T12:00:00.25Z", "level": "INFO", "body": "started", "env": "production"})

	code, out, errOut := aicoded("logs", "--env", "production", "--level", "info", "--since", "2d", "--limit", "2", demoDir(t))
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, "2026-10-24T12:00:00.250Z INFO  started\n2026-10-24T12:00:01.000Z ERROR failed?[2J  trace "+traceID+"\n", out,
		"oldest first, cleaned for a terminal")
	reads := p.Reads()
	require.Len(t, reads, 1)
	assert.Regexp(t, `^/v1/apps/demo/logs\?env=production&level=INFO&limit=2&since=[0-9-]+T[0-9%A.]+Z$`, reads[0])

	code, out, errOut = aicoded("logs", "--env", "preview", "--app", "demo", "--json")
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, "[]\n", out)
}

func TestRunTraces(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	root := map[string]any{"time": "2026-10-24T12:00:00Z", "trace_id": traceID, "span_id": "1111111111111111", "name": "HTTP GET",
		"duration_ns": 3000000, "status": "Error", "status_message": "E-SQL-001", "env": "production"}
	child := map[string]any{"time": "2026-10-24T12:00:00.001Z", "trace_id": traceID, "span_id": "2222222222222222",
		"parent_span_id": "1111111111111111", "name": "sql.query", "duration_ns": 1500000, "status": "Ok", "env": "production"}
	p.Telemetry("/v1/apps/demo/traces/"+traceID, child, root)
	p.Telemetry("/v1/apps/demo/spans", child, root)

	code, out, errOut := aicoded("traces", "--env", "production", "--app", "demo", "--trace", traceID)
	require.Equal(t, 0, code, errOut)
	assert.Equal(t, "2026-10-24T12:00:00.000Z HTTP GET  3ms  failed: E-SQL-001\n2026-10-24T12:00:00.001Z   sql.query  1.5ms\n", out)
	code, out, errOut = aicoded("traces", "--env", "production", "--until", "2026-10-24T13:00:00+01:00", demoDir(t))
	require.Equal(t, 0, code, errOut)
	assert.Contains(t, out, "HTTP GET  3ms  failed: E-SQL-001  trace "+traceID+"\n")
	assert.Equal(t, []string{"/v1/apps/demo/traces/" + traceID + "?env=production",
		"/v1/apps/demo/spans?env=production&until=2026-10-24T12%3A00%3A00Z"}, p.Reads())
}

func TestRunLogsRefuses(t *testing.T) {
	p, aicoded := signedIn(t)
	p.AddApp("payroll", "bo@acme.example")

	code, _, errOut := aicoded("logs", "--app", "demo")
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "give --env preview or --env production")

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"logs", "--env", "production", "--app", "demo"}, "E-PUB-019: "},
		{[]string{"logs", "--env", "production", "--app", "payroll"}, "E-PUB-005: "},
		{[]string{"logs", "--env", "staging", "--app", "payroll"}, "E-PUB-017: "},
		{[]string{"logs", "--env", "production", "--app", "demo", "--since", "yesterday"}, "E-PUB-017: --since \"yesterday\""},
		{[]string{"traces", "--env", "production", "--app", "demo", "--trace", "not-a-trace"}, "E-PUB-017: "},
	} {
		code, _, errOut := aicoded(tc.args...)
		assert.Equal(t, 1, code, tc.args)
		assert.True(t, strings.HasPrefix(errOut, tc.want), "%v: %s", tc.args, errOut)
		assert.Contains(t, errOut, "\n  fix: ")
	}
	p.AddApp("demo", platformtest.Email)
	p.TelemetryDown(true)
	code, _, errOut = aicoded("traces", "--env", "production", "--app", "demo")
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut, "E-PUB-018: ")
	assert.Empty(t, p.Reads())
}
