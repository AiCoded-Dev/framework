package s_id

import (
	"context"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// maxDepth is the deepest indent the page's styles draw.
const maxDepth = 8

var traceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// DP provides one trace.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the trace the URL names. A trace id that is not 32 lowercase hex digits, or a
// trace aicoded dev does not keep, is not found.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	id := r.URLParam("id")
	if !traceID.MatchString(id) {
		return web.NotFound()
	}
	t, err := p.d.Backend.Trace(ctx, id)
	if err != nil {
		return web.NotFound()
	}
	data.Trace = view(t)
	return nil
}

// view returns t as the page shows it.
func view(t devapi.Trace) Trace {
	out := Trace{ID: t.TraceID}
	if len(t.Spans) == 0 {
		return out
	}
	start, end := t.Spans[0].Start, t.Spans[0].Start
	var apps []string
	for _, sp := range t.Spans {
		if sp.Start.Before(start) {
			start = sp.Start
		}
		if e := sp.Start.Add(ms(sp.DurationMS)); e.After(end) {
			end = e
		}
		if !slices.Contains(apps, sp.App) {
			apps = append(apps, sp.App)
		}
	}
	slices.Sort(apps)
	out.Apps = strings.Join(apps, ", ")
	out.Total = float64(end.Sub(start)) / float64(time.Millisecond)
	out.Duration = duration(end.Sub(start))
	for _, i := range tree(t.Spans) {
		sp := t.Spans[i.index]
		v := Span{
			Name: sp.Name, App: sp.App, Depth: min(i.depth, maxDepth), Offset: duration(sp.Start.Sub(start)),
			Duration: duration(ms(sp.DurationMS)), Length: sp.DurationMS, Error: sp.Error,
		}
		for _, k := range slices.Sorted(maps.Keys(sp.Attrs)) {
			v.Attrs = append(v.Attrs, Attr{Key: k, Value: sp.Attrs[k]})
		}
		out.Spans = append(out.Spans, v)
	}
	return out
}

// placed is a span, by its index, and its depth in the tree.
type placed struct{ index, depth int }

// tree orders spans, which go by start, as their tree: each span follows its parent, and
// siblings keep their order. A span whose parent is not in spans is a root. Spans that no root
// reaches, as in a loop of parents, come last: the earliest of them is a root, with the spans
// below it.
func tree(spans []devapi.Span) []placed {
	ids := map[string]bool{}
	for _, sp := range spans {
		ids[sp.SpanID] = true
	}
	children := map[string][]int{}
	var roots []int
	for i, sp := range spans {
		if sp.ParentID != "" && sp.ParentID != sp.SpanID && ids[sp.ParentID] {
			children[sp.ParentID] = append(children[sp.ParentID], i)
		} else {
			roots = append(roots, i)
		}
	}
	seen := make([]bool, len(spans))
	var out []placed
	var visit func(i, depth int)
	visit = func(i, depth int) {
		if seen[i] {
			return
		}
		seen[i] = true
		out = append(out, placed{index: i, depth: depth})
		for _, c := range children[spans[i].SpanID] {
			visit(c, depth+1)
		}
	}
	for _, i := range roots {
		visit(i, 0)
	}
	for i := range spans {
		visit(i, 0)
	}
	return out
}

func ms(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }

// duration writes d to the microsecond, as in 1.5ms or 2.25s.
func duration(d time.Duration) string { return d.Round(time.Microsecond).String() }
