package pages

import (
	"context"

	"aicoded.dev/framework/web"

	"example.com/app/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the layout's data.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data sets the page title.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Title = "Notes"
	return nil
}
