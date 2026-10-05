package home

import (
	"context"

	"aicoded.dev/framework/web"

	"site/deps"
)

var _ RouteDataProvider = &DP{}

// DP echoes the query, so tests can check escaping.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data echoes q and link from the query.
func (p *DP) Data(_ context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	q := r.URL.Query()
	data.Q = q.Get("q")
	data.Link = q.Get("link")
	data.Page = map[string]string{"q": data.Q}
	return nil
}
