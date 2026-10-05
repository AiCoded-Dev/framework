package info

import (
	"context"
	"errors"
	"time"

	"aicoded.dev/framework/web"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides a user's details.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data shows the user's details and when they last opened a page.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	u, err := p.d.User(ctx, r.URLParam("login"))
	if errors.Is(err, deps.ErrNoUser) {
		return web.NotFound()
	}
	if err != nil {
		return err
	}
	data.User = u
	data.LastSeen = seen(p.d.Seen.At(u.Login))
	return nil
}

// Subscribe shows the time of every page the user opens while this page is open.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	sub := p.d.Seen.Follow(r.URLParam("login"))
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case at := <-sub.Updates():
			state.SetLastSeen(seen(at, true))
		}
	}
}

// seen tells when a user last opened a page: at, when ok.
func seen(at time.Time, ok bool) string {
	if !ok {
		return "not since the app started"
	}
	return at.UTC().Format("2006-01-02 15:04:05 UTC")
}
