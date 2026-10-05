package tools

import (
	"context"

	"aicoded.dev/framework/web"

	"example.com/app/deps"
)

var _ RouteDataProvider = &DP{}

// DP adds numbers for the page.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data needs nothing.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// CallAdd adds the two numbers.
func (p *DP) CallAdd(_ context.Context, _ *web.Request, in AddIn) (AddOut, error) {
	return AddOut{Sum: in.A + in.B}, nil
}
