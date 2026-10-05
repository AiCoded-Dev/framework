package apps

import (
	"context"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the apps of the workspace.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// rows returns the rows of the apps table.
func rows(apps []devapi.App) []App {
	out := make([]App, len(apps))
	now := time.Now()
	for i, a := range apps {
		out[i] = App{Name: a.Name, State: string(a.State), Since: deps.LocalTime(a.Since, now), Stamp: a.Since.Format(time.RFC3339), URL: a.URL}
		if len(a.Problems) > 0 {
			out[i].Problem = a.Problems[0].Code
		}
	}
	return out
}

// Data lists the apps.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	data.Apps = rows(st.Apps)
	return nil
}

// Subscribe lists the apps again at every change of the workspace.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	return deps.Follow(ctx, p.d.Backend, func(ctx context.Context) {
		if st, err := p.d.Backend.Status(ctx); err == nil {
			state.SetApps(rows(st.Apps))
		}
	})
}
