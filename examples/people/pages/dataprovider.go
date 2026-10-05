package pages

import (
	"context"
	"strings"
	"time"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the layout of every page.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data marks the section of the address in the menu, names the viewer, offers the users to
// staff, and records that the viewer opened a page.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	viewer := auth.Viewer(ctx)
	p.d.Seen.Record(viewer.Subject, time.Now())
	data.Section, _, _ = strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	data.Viewer = viewer.Name
	data.Staff = viewer.HasRole("staff")
	return nil
}
