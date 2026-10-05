package traces

import (
	"context"
	"strings"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// shown is how many traces the page shows.
const shown = 100

// DP provides the traces that match the page's query.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the apps for the filter and the traces that match the query.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	for _, a := range st.Apps {
		data.Names = append(data.Names, a.Name)
	}
	data.Query = filter(r)
	data.Traces = p.traces(ctx, data.Query)
	return nil
}

// Subscribe lists the traces again at every change of the workspace.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	q := filter(r)
	return deps.Follow(ctx, p.d.Backend, func(ctx context.Context) {
		state.SetTraces(p.traces(ctx, q))
	})
}

// filter returns the query of the page's URL.
func filter(r *web.Request) Query {
	v := r.URL.Query()
	return Query{App: v.Get("app"), ErrorsOnly: v.Get("errors") == "1", Limit: shown}
}

// traces returns the traces that match q.
func (p *DP) traces(ctx context.Context, q Query) Traces {
	ts, err := p.d.Backend.Traces(ctx, q)
	if err != nil {
		return Traces{Problem: err.Error()}
	}
	var out Traces
	for _, t := range ts {
		out.Traces = append(out.Traces, Trace{
			ID: t.TraceID, Stamp: t.Start.Format(time.RFC3339Nano), Start: t.Start.Local().Format("15:04:05.000"),
			Root: t.Root, Apps: strings.Join(t.Apps, ", "), Spans: t.Spans, Duration: duration(t.DurationMS), Error: t.Error,
		})
	}
	return out
}

// duration writes ms milliseconds to the microsecond, as in 1.5ms or 2.25s.
func duration(ms float64) string {
	return time.Duration(ms * float64(time.Millisecond)).Round(time.Microsecond).String()
}
