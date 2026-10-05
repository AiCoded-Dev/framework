package n_id

import (
	"context"

	"aicoded.dev/framework/web"

	"example.com/app/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides one note.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard lets through only note 1.
func (p *DP) Guard(_ context.Context, r *web.Request) error {
	if r.URLParamInt("id") != 1 {
		return web.NotFound()
	}
	return nil
}

// Data shows the note.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Note = Note{ID: 1, Title: "First"}
	return nil
}

// InitRename needs nothing.
func (p *DP) InitRename(context.Context, *web.Request, web.ResponseWriter, *FormRenameValues) error {
	return nil
}

// ProcessRename reloads the page.
func (p *DP) ProcessRename(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormRenameValues) error {
	return web.Redirect(r.URL.Path)
}
