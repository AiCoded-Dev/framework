package checks

import (
	"context"

	"aicoded.dev/framework/web"

	"shop/deps"
	"shop/services/billing"
)

var _ RouteDataProvider = &DP{}

// DP shows how two functions of billing fail.
type DP struct{}

// NewDP returns the page's data provider.
func NewDP(*deps.Deps) *DP { return &DP{} }

// Data calls both as the viewer and shows the errors shop got.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	if _, err := billing.Fail(ctx, billing.InvoiceID{ID: 1}); err != nil {
		data.Fail = err.Error()
	}
	if _, err := billing.Missing(ctx, billing.InvoiceID{ID: 7}); err != nil {
		data.Missing = err.Error()
	}
	return nil
}
