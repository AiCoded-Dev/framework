package notes

import (
	"context"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"notes/deps"
)

var _ RouteDataProvider = &DP{}

// DP lists the viewer's notes and adds new ones.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the viewer's notes.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	notes, err := p.d.NotesOf(ctx, auth.Viewer(ctx).Subject)
	data.Notes = notes
	return err
}

// InitAdd needs nothing.
func (p *DP) InitAdd(context.Context, *web.Request, web.ResponseWriter, *FormAddValues) error {
	return nil
}

// ProcessAdd stores the note as the viewer's and shows the list again.
func (p *DP) ProcessAdd(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	if err := p.d.Add(ctx, auth.Viewer(ctx).Subject, f.Title.GetValue(), f.Body.GetValue()); err != nil {
		return err
	}
	return web.Redirect("/notes")
}
