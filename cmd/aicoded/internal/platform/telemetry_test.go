package platform_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/platform"
	"aicoded.dev/framework/cmd/aicoded/internal/platform/platformtest"
	"aicoded.dev/framework/internal/errs"
)

var trace = strings.Repeat("ab", 16)

func TestLogs(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.Telemetry("/v1/apps/demo/logs", map[string]any{"time": "2026-10-24T12:00:00.5Z", "level": "ERROR",
		"body": "\x1b[31mfailed\u202e", "trace_id": trace, "span_id": "not hex", "release": "rel_1", "env": "production",
		"attributes": map[string]string{"k\n": "v\x07"}})
	since := time.Date(2026, 10, 24, 13, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	lines, err := c.Logs(t.Context(), platform.TelemetryQuery{App: "demo", Env: "production", Since: since, Level: "WARN", Trace: trace,
		Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, []platform.LogLine{{Time: time.Date(2026, 10, 24, 12, 0, 0, 5e8, time.UTC), Level: "ERROR", Body: "?[31mfailed?",
		TraceID: trace, Release: "rel_1", Env: "production", Attributes: map[string]string{"k?": "v?"}}}, lines, "cleaned for a terminal")
	assert.Equal(t, []string{"/v1/apps/demo/logs?env=production&level=WARN&limit=5&since=2026-10-24T11%3A00%3A00Z&trace=" + trace}, p.Reads())
}

func TestSpansAndATrace(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	span := map[string]any{"time": "2026-10-24T12:00:00Z", "trace_id": trace, "span_id": "0123456789abcdef", "name": "HTTP GET",
		"kind": "Server", "duration_ns": 1500, "status": "Error", "release": "rel_1", "env": "preview"}
	p.Telemetry("/v1/apps/demo/spans", span)
	p.Telemetry("/v1/apps/demo/traces/"+trace, span)
	want := []platform.Span{{Time: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), TraceID: trace, SpanID: "0123456789abcdef",
		Name: "HTTP GET", Kind: "Server", DurationNS: 1500, Status: "Error", Release: "rel_1", Env: "preview"}}

	spans, err := c.Spans(t.Context(), platform.TelemetryQuery{App: "demo", Env: "preview", Limit: 1000})
	require.NoError(t, err)
	assert.Equal(t, want, spans)
	spans, err = c.Trace(t.Context(), platform.TelemetryQuery{App: "demo", Env: "preview", Trace: trace})
	require.NoError(t, err)
	assert.Equal(t, want, spans)
	none, err := c.Logs(t.Context(), platform.TelemetryQuery{App: "demo", Env: "preview"})
	require.NoError(t, err)
	assert.Equal(t, []platform.LogLine{}, none)
	assert.Equal(t, []string{"/v1/apps/demo/spans?env=preview&limit=1000", "/v1/apps/demo/traces/" + trace + "?env=preview",
		"/v1/apps/demo/logs?env=preview"}, p.Reads())
}

func TestTelemetryRefused(t *testing.T) {
	p, c := signedIn(t)
	p.AddApp("demo", platformtest.Email)
	p.AddApp("payroll", "bo@acme.example")
	for name, tc := range map[string]struct {
		q    platform.TelemetryQuery
		code string
	}{
		"an unknown app":         {platform.TelemetryQuery{App: "nope", Env: "production"}, "E-PUB-019"},
		"another builder's app":  {platform.TelemetryQuery{App: "payroll", Env: "production"}, "E-PUB-005"},
		"no env":                 {platform.TelemetryQuery{App: "demo"}, "E-PUB-017"},
		"an app that is no name": {platform.TelemetryQuery{App: "demo/../payroll", Env: "production"}, "E-PUB-017"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := c.Logs(t.Context(), tc.q)
			assert.Equal(t, tc.code, errs.Code(err), "%v", err)
		})
	}
	_, err := c.Trace(t.Context(), platform.TelemetryQuery{App: "demo", Env: "production", Trace: "../../logs"})
	assert.Equal(t, "E-PUB-017", errs.Code(err))
	_, err = c.Spans(t.Context(), platform.TelemetryQuery{App: "demo", Env: "production", Level: "ERROR"})
	assert.Equal(t, "E-PUB-017", errs.Code(err))
	assert.Equal(t, 1, p.Requests("/v1/apps/nope/logs")+p.Requests("/v1/apps/demo/logs"), "only the unknown app reached the platform")

	p.TelemetryDown(true)
	_, err = c.Spans(t.Context(), platform.TelemetryQuery{App: "demo", Env: "production"})
	assert.Equal(t, "E-PUB-018", errs.Code(err))
	assert.Contains(t, err.Error(), "fix: try again in a few minutes")
	assert.Empty(t, p.Reads())
}

func TestWhen(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for s, want := range map[string]time.Time{"": {}, "90s": now.Add(-90 * time.Second), "6h": now.Add(-6 * time.Hour),
		"2d": now.AddDate(0, 0, -2), "2026-10-24T09:00:00+02:00": time.Date(2026, 10, 24, 7, 0, 0, 0, time.UTC)} {
		got, err := platform.When("--since", s, now)
		require.NoError(t, err, s)
		assert.True(t, want.Equal(got), "%s: %s", s, got)
	}
	for _, s := range []string{"-1h", "-2d", "d", "2w", "2026-10-24"} {
		_, err := platform.When("--since", s, now)
		assert.Error(t, err, s)
	}
}
