package n_id

import (
	"context"
	"errors"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides a ticket to its author and to hotel-ops.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard lets in the ticket's author and hotel-ops, here and on the pages below. Anyone else gets
// 403, also for a ticket that does not exist, so that they cannot tell which tickets exist.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	viewer := auth.Viewer(ctx)
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	switch {
	case errors.Is(err, deps.ErrNoTicket) && viewer.HasRole(deps.HotelOps):
		return web.NotFound()
	case errors.Is(err, deps.ErrNoTicket):
		return web.Forbidden()
	case err != nil:
		return err
	case !deps.CanOpen(viewer, t):
		return web.Forbidden()
	}
	return nil
}

// Data shows the ticket, and the links to what the viewer may change.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	viewer := auth.Viewer(ctx)
	data.Ticket = t
	data.CanEdit = deps.CanEdit(viewer, t)
	data.CanSetStatus = viewer.HasRole(deps.HotelOps)
	return nil
}
