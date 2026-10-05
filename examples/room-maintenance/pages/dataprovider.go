package pages

import (
	"context"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the layout of every page.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data names the viewer.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Viewer = auth.Viewer(ctx).Name
	return nil
}
