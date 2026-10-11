package n_id

import (
	"context"

	"aicoded.dev/framework/web"

	"notes/deps"
)

var _ RouteDataProvider = &DP{}

// DP shows one note.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the note with the id in the URL.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	n, err := p.d.Note(ctx, r.URLParamInt("id"))
	data.Note = n
	return err
}
