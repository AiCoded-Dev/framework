package s_app

import (
	"context"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides one app of the workspace, and starts, stops and switches it.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// app returns the app the path names, or 404 when the workspace has no such app.
func (p *DP) app(ctx context.Context, r *web.Request) (devapi.App, error) {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return devapi.App{}, err
	}
	for _, a := range st.Apps {
		if a.Name == r.URLParam("app") {
			return a, nil
		}
	}
	return devapi.App{}, web.NotFound()
}

// view returns what the page shows of a.
func view(a devapi.App) App {
	return App{
		State:     string(a.State),
		Since:     deps.LocalTime(a.Since, time.Now()),
		Stamp:     a.Since.Format(time.RFC3339),
		URL:       a.URL,
		Problems:  a.Problems,
		Command:   a.Command,
		RunnerDir: a.RunnerDir,
	}
}

// Guard answers 404 for a name that is not an app of the workspace, before a form or the live
// connection reaches the backend.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	_, err := p.app(ctx, r)
	return err
}

// Data shows the app.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	a, err := p.app(ctx, r)
	if err != nil {
		return err
	}
	data.Name, data.App = a.Name, view(a)
	return nil
}

// Subscribe shows the app again at every change of the workspace.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	return deps.Follow(ctx, p.d.Backend, func(ctx context.Context) {
		if a, err := p.app(ctx, r); err == nil {
			state.SetApp(view(a))
		}
	})
}

// done answers a form: back to this page once the backend took the request.
func done(r *web.Request, err error) error {
	if err != nil {
		return err
	}
	return web.Redirect("/apps/" + r.URLParam("app"))
}

// InitStart needs nothing.
func (p *DP) InitStart(context.Context, *web.Request, web.ResponseWriter, *FormStartValues) error {
	return nil
}

// ProcessStart starts the app.
func (p *DP) ProcessStart(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormStartValues) error {
	return done(r, p.d.Backend.Start(r.URLParam("app")))
}

// InitRestart needs nothing.
func (p *DP) InitRestart(context.Context, *web.Request, web.ResponseWriter, *FormRestartValues) error {
	return nil
}

// ProcessRestart restarts the app with Start, which restarts an app that runs and returns at
// once.
func (p *DP) ProcessRestart(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormRestartValues) error {
	return done(r, p.d.Backend.Start(r.URLParam("app")))
}

// InitStop needs nothing.
func (p *DP) InitStop(context.Context, *web.Request, web.ResponseWriter, *FormStopValues) error {
	return nil
}

// ProcessStop stops the app.
func (p *DP) ProcessStop(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormStopValues) error {
	return done(r, p.d.Backend.Stop(r.URLParam("app")))
}

// InitManual needs nothing.
func (p *DP) InitManual(context.Context, *web.Request, web.ResponseWriter, *FormManualValues) error {
	return nil
}

// ProcessManual switches the app into manual mode.
func (p *DP) ProcessManual(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormManualValues) error {
	return done(r, p.d.Backend.SetManual(r.URLParam("app"), true))
}

// InitAuto needs nothing.
func (p *DP) InitAuto(context.Context, *web.Request, web.ResponseWriter, *FormAutoValues) error {
	return nil
}

// ProcessAuto switches the app out of manual mode.
func (p *DP) ProcessAuto(_ context.Context, r *web.Request, _ web.ResponseWriter, _ *FormAutoValues) error {
	return done(r, p.d.Backend.SetManual(r.URLParam("app"), false))
}
