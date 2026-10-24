package platform

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
)

// MaxRows is the most rows a read of logs or spans answers.
const MaxRows = 1000

// maxTelemetry is the largest answer of logs or spans read from the platform: 64 MiB.
const maxTelemetry = 64 << 20

// Envs are the environments of an app on the platform whose logs and traces can be read.
var Envs = []string{"preview", "production"}

// TraceID matches a trace id: 32 of 0-9 and a-f.
var TraceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

var spanID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// TelemetryQuery selects what Logs, Spans and Trace read of the app App, by its name, in the
// environment Env, preview or production. Since and Until bound the time read: zero for the
// platform's own, the last hour up to now; at most 7 days apart (E-PUB-015). Limit is the most
// rows, at most 1000 (E-PUB-016), or 0 for 100. Level, the lowest level shown, and Trace, a trace
// id, keep only some log lines; Trace is the trace that Trace reads.
type TelemetryQuery struct {
	App, Env     string
	Since, Until time.Time
	Level, Trace string
	Limit        int
}

// LogLine is a line of an app's log on the platform, with its text cleaned for a terminal.
type LogLine struct {
	Time       time.Time         `json:"time"`
	Level      string            `json:"level"`
	Body       string            `json:"body"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
	Release    string            `json:"release"`
	Env        string            `json:"env"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Span is a span of an app on the platform, with its text cleaned for a terminal: Time is its
// start, and DurationNS its length in nanoseconds.
type Span struct {
	Time          time.Time         `json:"time"`
	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	Name          string            `json:"name"`
	Kind          string            `json:"kind"`
	DurationNS    int64             `json:"duration_ns"`
	Status        string            `json:"status"`
	StatusMessage string            `json:"status_message,omitempty"`
	Release       string            `json:"release"`
	Env           string            `json:"env"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// When reads the time that name, such as --since, gives in s: in RFC 3339, or a while before now,
// such as 90s, 30m, 6h or 2d. An empty s is the zero time.
func When(name, s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	d, err := time.ParseDuration(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, aerr := strconv.Atoi(days)
		d, err = time.Duration(n)*24*time.Hour, aerr
	}
	if err != nil || d < 0 {
		return time.Time{}, errs.New("E-PUB-017", fmt.Sprintf("%s %.40q is neither a time in RFC 3339 nor a while ago, such as 30m, 6h or 2d", name, s),
			Fix("E-PUB-017"))
	}
	return now.Add(-d), nil
}

// Logs returns the log lines of the app that q selects, newest first.
func (c *Client) Logs(ctx context.Context, q TelemetryQuery) ([]LogLine, error) {
	lines, err := readTelemetry[LogLine](ctx, c, q, "logs", url.PathEscape(q.App)+"/logs", true)
	for i := range lines {
		l := &lines[i]
		l.Level, l.Body, l.Release, l.Env = clean(l.Level, 16), clean(l.Body, 16<<10), clean(l.Release, 64), clean(l.Env, 16)
		l.TraceID, l.SpanID = hexID(TraceID, l.TraceID), hexID(spanID, l.SpanID)
		l.Attributes = cleanAttributes(l.Attributes)
	}
	return lines, err
}

// Spans returns the spans of the app that q selects, newest first. It takes no Level or Trace.
func (c *Client) Spans(ctx context.Context, q TelemetryQuery) ([]Span, error) {
	if q.Level != "" || q.Trace != "" {
		return nil, errs.New("E-PUB-017", "a level or a trace keeps only some log lines, not spans", Fix("E-PUB-017"))
	}
	return spans(readTelemetry[Span](ctx, c, q, "traces", url.PathEscape(q.App)+"/spans", true))
}

// Trace returns the spans of the trace q.Trace of the app, newest first, at most 1000. It takes
// no Level or Limit.
func (c *Client) Trace(ctx context.Context, q TelemetryQuery) ([]Span, error) {
	if !TraceID.MatchString(q.Trace) {
		return nil, errs.New("E-PUB-017", "a trace id is 32 of 0-9 and a-f", Fix("E-PUB-017"))
	}
	if q.Level != "" || q.Limit != 0 {
		return nil, errs.New("E-PUB-017", "a trace has no level or limit", Fix("E-PUB-017"))
	}
	return spans(readTelemetry[Span](ctx, c, q, "traces", url.PathEscape(q.App)+"/traces/"+q.Trace, false))
}

func spans(out []Span, err error) ([]Span, error) {
	for i := range out {
		s := &out[i]
		s.TraceID, s.SpanID, s.ParentSpanID = hexID(TraceID, s.TraceID), hexID(spanID, s.SpanID), hexID(spanID, s.ParentSpanID)
		s.Name, s.Kind, s.Status, s.StatusMessage = clean(s.Name, 300), clean(s.Kind, 32), clean(s.Status, 32), clean(s.StatusMessage, 1000)
		s.Release, s.Env = clean(s.Release, 64), clean(s.Env, 16)
		s.Attributes = cleanAttributes(s.Attributes)
	}
	return out, err
}

// readTelemetry reads the list of T at /v1/apps/<path> for q; filters is whether the path takes
// q's level, trace and limit. what names what is read, for errors.
func readTelemetry[T any](ctx context.Context, c *Client, q TelemetryQuery, what, path string, filters bool) ([]T, error) {
	if !manifest.ValidApp(q.App) {
		return nil, errs.New("E-PUB-017", "an app's name is 2 to 63 lowercase letters, digits and dashes, starting with a letter",
			Fix("E-PUB-017"))
	}
	if !slices.Contains(Envs, q.Env) {
		return nil, errs.New("E-PUB-017", "the environment is preview or production", Fix("E-PUB-017"))
	}
	v := url.Values{"env": {q.Env}}
	if !q.Since.IsZero() {
		v.Set("since", q.Since.UTC().Format(time.RFC3339Nano))
	}
	if !q.Until.IsZero() {
		v.Set("until", q.Until.UTC().Format(time.RFC3339Nano))
	}
	if filters && q.Level != "" {
		v.Set("level", q.Level)
	}
	if filters && q.Trace != "" {
		v.Set("trace", q.Trace)
	}
	if filters && q.Limit != 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	status, answer, err := c.call(ctx, c.HTTP, http.MethodGet, "/v1/apps/"+path+"?"+v.Encode(), "", nil, maxTelemetry)
	if err != nil {
		return nil, err
	}
	e := parseError(answer)
	switch {
	case status == http.StatusOK:
	case status == http.StatusForbidden && (e.Code == "" || e.Code == "E-PUB-005"):
		return nil, errs.New("E-PUB-005", "the platform refused to show the "+what+" of "+q.App+": "+clean(cmp.Or(e.Description,
			"the app belongs to another builder"), 300), "read the "+what+" of an app you own: ask its owner, or your administrator to transfer it to you")
	case pubCode.MatchString(e.Code):
		return nil, refusal(e, e.Code, "the platform refused to show the "+what+" of "+q.App)
	case status == http.StatusNotFound:
		return nil, refusal(e, "E-PUB-019", "your organisation has no app "+q.App+" on the platform")
	case status == http.StatusServiceUnavailable:
		return nil, refusal(e, "E-PUB-018", "the platform cannot read logs and traces right now")
	default:
		return nil, unexpected(status, answer)
	}
	var out []T
	if err := json.Unmarshal(answer, &out); err != nil {
		return nil, errors.New("the platform's answer about the " + what + " of " + q.App + " is not valid")
	}
	if out == nil {
		out = []T{}
	}
	return out, nil
}

// hexID returns s when it matches id, and "" otherwise.
func hexID(id *regexp.Regexp, s string) string {
	if id.MatchString(s) {
		return s
	}
	return ""
}

// cleanAttributes returns the attributes with their keys and values fit to print.
func cleanAttributes(attrs map[string]string) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		out[clean(k, 128)] = clean(v, 1000)
	}
	return out
}
