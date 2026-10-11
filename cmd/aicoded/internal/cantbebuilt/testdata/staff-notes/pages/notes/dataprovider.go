package notes

import (
	"context"

	"aicoded.dev/framework/web"

	"notes/deps"
)

var _ RouteDataProvider = &DP{}

// DP shows the notes, live, and adds new ones.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the notes added so far.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Notes = p.d.Notes()
	return nil
}

// Subscribe pushes nothing: the live list holds the notes as they were when the page opened.
func (p *DP) Subscribe(context.Context, *web.Request, *ReactiveState) error { return nil }

// InitAdd needs nothing.
func (p *DP) InitAdd(context.Context, *web.Request, web.ResponseWriter, *FormAddValues) error {
	return nil
}

// ProcessAdd stores the note and shows the list again.
func (p *DP) ProcessAdd(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	p.d.Add(f.Note.GetValue())
	return web.Redirect("/notes")
}
