package admin

import (
	"context"

	"aicoded.dev/framework/web"

	"site/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the admin layout.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data needs nothing.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// DefaultRoute opens the live page.
func (p *DP) DefaultRoute(context.Context, *web.Request) (string, error) { return "/live", nil }
