package notes

import (
	"context"
	"fmt"
	"log/slog"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"
	"aicoded.dev/framework/web/form"

	"site/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the note list.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the viewer's notes.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Notes = p.d.Owned(auth.Viewer(ctx).Subject)
	return nil
}

// InitAdd offers the priorities.
func (p *DP) InitAdd(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	f.Priority.SetOptions([]form.SelectOptionElement[int]{
		form.SelectOption[int]{Value: 1, Label: "Low"},
		form.SelectOption[int]{Value: 2, Label: "High"},
	})
	return nil
}

// ProcessAdd stores a valid note.
func (p *DP) ProcessAdd(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	slog.InfoContext(ctx, "processed", "form", "add", "by", auth.Viewer(ctx).Subject)
	id := p.d.Add(deps.Note{Title: f.Title.GetValue(), Priority: f.Priority.GetValue(), Owner: auth.Viewer(ctx).Subject})
	return web.Redirect(fmt.Sprintf("/notes/%d", id))
}
