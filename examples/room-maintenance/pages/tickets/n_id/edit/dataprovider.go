package edit

import (
	"context"
	"strconv"
	"strings"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the form that changes a ticket's title and details.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard lets only the ticket's author edit it, and only while it is open. The ticket's own Guard
// has already refused everyone but its author and hotel-ops.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	if !deps.CanEdit(auth.Viewer(ctx), t) {
		return web.Forbidden()
	}
	return nil
}

// Data needs nothing: the page is its form.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// InitEdit starts with the ticket's title and details.
func (p *DP) InitEdit(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormEditValues) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	f.Title.SetValue(t.Title)
	f.Details.SetValue(t.Details)
	return nil
}

// ProcessEdit saves the title and details, tells the open lists, and opens the ticket. The
// database changes the ticket only while the viewer is its author and it is open.
func (p *DP) ProcessEdit(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormEditValues) error {
	title := strings.TrimSpace(f.Title.GetValue())
	details := strings.TrimSpace(f.Details.GetValue())
	titleProblem, detailsProblem := deps.TextProblems(title, details)
	f.Title.SetError(titleProblem)
	f.Details.SetError(detailsProblem)
	if f.HasError() {
		return nil
	}
	id, author := r.URLParamInt("id"), auth.Viewer(ctx).Subject
	if err := p.d.EditTicket(ctx, id, author, title, details); err != nil {
		return err
	}
	p.d.TicketChanged(author)
	return web.Redirect("/tickets/" + strconv.FormatInt(id, 10))
}
