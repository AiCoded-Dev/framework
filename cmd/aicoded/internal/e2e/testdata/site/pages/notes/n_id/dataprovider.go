package n_id

import (
	"context"
	"log/slog"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"site/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides one note to its owner.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

func (p *DP) note(ctx context.Context, r *web.Request) (deps.Note, bool) {
	n, ok := p.d.Get(r.URLParamInt("id"))
	return n, ok && n.Owner == auth.Viewer(ctx).Subject
}

// Guard hides notes the viewer does not own.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	if _, ok := p.note(ctx, r); !ok {
		return web.NotFound()
	}
	return nil
}

// Data shows the note.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Note, _ = p.note(ctx, r)
	return nil
}

// InitRename needs nothing.
func (p *DP) InitRename(context.Context, *web.Request, web.ResponseWriter, *FormRenameValues) error {
	return nil
}

// ProcessRename renames the note.
func (p *DP) ProcessRename(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormRenameValues) error {
	slog.InfoContext(ctx, "processed", "form", "rename", "by", auth.Viewer(ctx).Subject)
	n, _ := p.note(ctx, r)
	p.d.Rename(n.ID, f.Title.GetValue())
	return web.Redirect(r.URL.Path)
}

// InitGive needs nothing.
func (p *DP) InitGive(context.Context, *web.Request, web.ResponseWriter, *FormGiveValues) error {
	return nil
}

// ProcessGive hands the note over to another viewer.
func (p *DP) ProcessGive(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormGiveValues) error {
	n, _ := p.note(ctx, r)
	p.d.Give(n.ID, f.To.GetValue())
	return web.Redirect("/notes")
}

// CallStar marks the note; the page's Guard has already checked the owner for this call.
func (p *DP) CallStar(ctx context.Context, r *web.Request, in StarIn) (StarOut, error) {
	slog.InfoContext(ctx, "called", "call", "star", "starred", in.Starred)
	n, _ := p.note(ctx, r)
	p.d.Star(n.ID, in.Starred)
	return StarOut{Starred: in.Starred, Title: n.Title}, nil
}
