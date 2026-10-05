package status

import (
	"context"
	"strconv"

	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the form that changes a ticket's status, which only hotel-ops opens.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows which ticket the form changes.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	data.Ticket = t
	return nil
}

// InitStatus offers the statuses, starting with the ticket's own.
func (p *DP) InitStatus(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormStatusValues) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	var options []form.SelectOptionElement[string]
	for _, s := range deps.Statuses {
		options = append(options, form.SelectOption[string]{Value: string(s), Label: s.Label()})
	}
	f.Status.SetOptions(options)
	f.Status.SetValue(string(t.Status))
	return nil
}

// ProcessStatus saves the status, tells the open lists of the ticket's author and of hotel-ops,
// and opens the ticket.
func (p *DP) ProcessStatus(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormStatusValues) error {
	t, err := p.d.Ticket(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	if err := p.d.SetStatus(ctx, t.ID, deps.Status(f.Status.GetValue())); err != nil {
		return err
	}
	p.d.TicketChanged(t.Author)
	return web.Redirect("/tickets/" + strconv.FormatInt(t.ID, 10))
}
