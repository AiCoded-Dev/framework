package report

import (
	"context"
	"strconv"
	"strings"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"room-maintenance/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the form that reports a problem.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data needs nothing: the page is its form.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// InitReport offers the rooms, floor by floor.
func (p *DP) InitReport(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormReportValues) error {
	rooms, err := p.d.Rooms(ctx)
	if err != nil {
		return err
	}
	f.Room.SetOptions(roomOptions(rooms))
	return nil
}

// ProcessReport adds the ticket as the viewer's, tells the open lists, and opens the ticket.
func (p *DP) ProcessReport(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormReportValues) error {
	title := strings.TrimSpace(f.Title.GetValue())
	details := strings.TrimSpace(f.Details.GetValue())
	titleProblem, detailsProblem := deps.TextProblems(title, details)
	f.Title.SetError(titleProblem)
	f.Details.SetError(detailsProblem)
	if f.HasError() {
		return nil
	}
	author := auth.Viewer(ctx).Subject
	id, err := p.d.AddTicket(ctx, author, f.Room.GetValue(), title, details)
	if err != nil {
		return err
	}
	p.d.TicketChanged(author)
	return web.Redirect("/tickets/" + strconv.FormatInt(id, 10))
}

// roomOptions offers rooms, sorted by floor, in a group per floor.
func roomOptions(rooms []deps.Room) []form.SelectOptionElement[int] {
	var groups []form.SelectOptionElement[int]
	var floor []form.SelectOptionElement[int]
	for i, r := range rooms {
		floor = append(floor, form.SelectOption[int]{Value: r.Number, Label: strconv.Itoa(r.Number)})
		if i == len(rooms)-1 || rooms[i+1].Floor != r.Floor {
			groups = append(groups, form.SelectOptionGroup[int]{Label: "Floor " + strconv.Itoa(r.Floor), Options: floor})
			floor = nil
		}
	}
	return groups
}
