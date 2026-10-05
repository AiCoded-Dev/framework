package status

import (
	"context"

	"aicoded.dev/framework/web"

	"shop/deps"
)

var _ RouteDataProvider = &DP{}

// DP shows what billing answered to shop itself when it started.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data copies the answers.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Pong, data.Secret = p.d.Pong, p.d.Secret
	return nil
}
