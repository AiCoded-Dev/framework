package pages

import (
	"context"

	"aicoded.dev/framework/web"

	"shop/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the layout.
type DP struct{}

// NewDP returns the layout's data provider.
func NewDP(*deps.Deps) *DP { return &DP{} }

// Data needs nothing.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }
