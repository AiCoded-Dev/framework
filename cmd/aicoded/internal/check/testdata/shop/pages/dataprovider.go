package pages

import (
	"context"

	"aicoded.dev/framework/web"
)

var _ RouteDataProvider = &DP{}

// DP provides the data of this page.
type DP struct{}

// NewDP returns the page's data provider.
func NewDP() *DP { return &DP{} }

// Data fills the variables the template declares.
func (p *DP) Data(ctx context.Context, r *web.Request, w web.ResponseWriter, data *RouteData) error {
	return nil
}
