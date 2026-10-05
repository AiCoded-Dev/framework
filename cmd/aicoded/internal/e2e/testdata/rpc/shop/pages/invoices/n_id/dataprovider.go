package n_id

import (
	"context"
	"strconv"
	"time"

	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/web"

	"shop/deps"
	"shop/services/billing"
)

var _ RouteDataProvider = &DP{}

// DP shows one invoice of billing.
type DP struct{}

// NewDP returns the page's data provider.
func NewDP(*deps.Deps) *DP { return &DP{} }

// Data asks billing for the invoice on behalf of the viewer.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	inv, err := billing.GetInvoice(ctx, billing.InvoiceID{ID: r.URLParamInt("id")})
	switch rpc.CodeOf(err) {
	case 0:
	case rpc.PermissionDenied:
		return web.Forbidden()
	case rpc.NotFound:
		return web.NotFound()
	default:
		return err
	}
	data.Invoice = inv
	data.Due = inv.Due.Format(time.DateOnly)
	data.Paid = "unknown"
	if inv.Paid != nil {
		data.Paid = strconv.FormatBool(*inv.Paid)
	}
	return nil
}
