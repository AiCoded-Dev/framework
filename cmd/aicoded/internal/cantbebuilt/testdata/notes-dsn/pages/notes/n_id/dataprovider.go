package n_id

import (
	"context"
	"errors"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"notes/deps"
)

var _ RouteDataProvider = &DP{}

// DP shows one note to the person who wrote it.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard lets only the note's owner open it. Anyone else gets 404, as for a note that does not
// exist, so they cannot tell which notes exist.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	n, err := p.d.Note(ctx, r.URLParamInt("id"))
	switch {
	case errors.Is(err, deps.ErrNoNote):
		return web.NotFound()
	case err != nil:
		return err
	case n.Owner != auth.Viewer(ctx).Subject:
		return web.NotFound()
	}
	return nil
}

// Data shows the note.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	n, err := p.d.Note(ctx, r.URLParamInt("id"))
	data.Note = n
	return err
}
