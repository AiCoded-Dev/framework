package tickets

import (
	"context"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the list of tickets: the viewer's own, or every ticket to hotel-ops.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the tickets the viewer may open.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	viewer := auth.Viewer(ctx)
	tickets, err := p.d.TicketsFor(ctx, viewer)
	if err != nil {
		return err
	}
	data.Tickets = tickets
	data.Everyone = viewer.HasRole(deps.HotelOps)
	return nil
}

// Subscribe follows the tickets the viewer may open and lists them again whenever one of them is
// reported, edited or changes its status. It lists them once as soon as it follows them, so that
// a change made since Data is not missed.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	viewer := auth.Viewer(ctx)
	changes := p.d.FollowTickets(viewer)
	defer changes.Close()
	for {
		tickets, err := p.d.TicketsFor(ctx, viewer)
		if err != nil {
			return err
		}
		state.SetTickets(tickets)
		select {
		case <-ctx.Done():
			return nil
		case <-changes.Updates():
		}
	}
}
