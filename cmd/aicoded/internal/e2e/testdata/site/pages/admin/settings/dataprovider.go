package settings

import (
	"context"
	"log/slog"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"site/deps"
)

var _ RouteDataProvider = &DP{}

// DP saves the settings.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data needs nothing.
func (p *DP) Data(context.Context, *web.Request, web.ResponseWriter, *RouteData) error { return nil }

// InitSave needs nothing.
func (p *DP) InitSave(context.Context, *web.Request, web.ResponseWriter, *FormSaveValues) error {
	return nil
}

// ProcessSave records that it ran.
func (p *DP) ProcessSave(ctx context.Context, _ *web.Request, _ web.ResponseWriter, _ *FormSaveValues) error {
	slog.InfoContext(ctx, "processed", "form", "save", "by", auth.Viewer(ctx).Subject)
	return web.Redirect("/admin/settings")
}
