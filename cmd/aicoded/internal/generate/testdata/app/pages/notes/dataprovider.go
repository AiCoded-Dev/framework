package notes

import (
	"context"

	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"example.com/app/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the note list.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the notes.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Notes = []Note{{ID: 1, Title: "First"}}
	return nil
}

// DefaultRoute opens the first note.
func (p *DP) DefaultRoute(context.Context, *web.Request) (string, error) { return "1", nil }

// InitAdd offers the priorities.
func (p *DP) InitAdd(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	f.Priority.SetOptions([]form.SelectOptionElement[int]{
		form.SelectOption[int]{Value: 1, Label: "Low"},
		form.SelectOption[int]{Value: 2, Label: "High"},
	})
	return nil
}

// ProcessAdd goes back to the list.
func (p *DP) ProcessAdd(context.Context, *web.Request, web.ResponseWriter, *FormAddValues) error {
	return web.Redirect("/notes")
}
