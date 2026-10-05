package pages

import (
	"context"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the data of the layout around every page; the layout shows no values.
type DP struct{}

// NewDP returns the layout's data provider.
func NewDP(*deps.Deps) *DP { return &DP{} }

// Data has nothing to set.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }
