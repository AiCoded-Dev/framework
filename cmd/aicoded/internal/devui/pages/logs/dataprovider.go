package logs

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// shown is how many entries the page shows.
const shown = 200

var traceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// DP provides the log entries that match the page's query.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the apps for the filter and the entries that match the query.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	for _, a := range st.Apps {
		data.Names = append(data.Names, a.Name)
	}
	data.Query = filter(r)
	data.Logs = p.logs(ctx, data.Query)
	return nil
}

// Subscribe lists the entries again at every change of the workspace.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	q := filter(r)
	return deps.Follow(ctx, p.d.Backend, func(ctx context.Context) {
		state.SetLogs(p.logs(ctx, q))
	})
}

// filter returns the query of the page's URL.
func filter(r *web.Request) Query {
	v := r.URL.Query()
	return Query{App: v.Get("app"), Level: v.Get("level"), TraceID: v.Get("trace"), Contains: v.Get("q"), Limit: shown}
}

// logs returns the entries that match q, and the apps of q that run by hand.
func (p *DP) logs(ctx context.Context, q Query) Logs {
	var out Logs
	if st, err := p.d.Backend.Status(ctx); err == nil {
		for _, a := range st.Apps {
			if a.State == devapi.Manual && (q.App == "" || q.App == a.Name) {
				out.Manual = append(out.Manual, a.Name)
			}
		}
	}
	entries, err := p.d.Backend.Logs(ctx, q)
	if err != nil {
		out.Problem = err.Error()
		return out
	}
	for _, e := range slices.Backward(entries) {
		out.Entries = append(out.Entries, entry(e))
	}
	return out
}

func entry(e devapi.LogEntry) Entry {
	out := Entry{
		Stamp: e.Time.Format(time.RFC3339Nano), Time: e.Time.Local().Format("15:04:05.000"),
		App: e.App, Source: e.Source, Level: e.Level, Message: e.Message, Attrs: attrs(e.Attrs),
	}
	switch e.Level {
	case "DEBUG", "INFO", "WARN", "ERROR":
		out.Class = "level-" + e.Level
	}
	if traceID.MatchString(e.TraceID) {
		out.Trace = e.TraceID
	}
	return out
}

// attrs writes m as key=value pairs by key: a string value quoted, any other value as JSON.
func attrs(m map[string]any) string {
	var pairs []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		var v string
		if s, ok := m[k].(string); ok {
			v = strconv.Quote(s)
		} else if b, err := json.Marshal(m[k]); err == nil {
			v = string(b)
		}
		pairs = append(pairs, k+"="+v)
	}
	return strings.Join(pairs, " ")
}
