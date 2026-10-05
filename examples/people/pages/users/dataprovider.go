package users

import (
	"context"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/web"

	"people/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides the list of users, the layout of the user pages.
type DP struct{ d *deps.Deps }

// NewDP returns the layout's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// DefaultRoute opens the first user.
func (p *DP) DefaultRoute(ctx context.Context, _ *web.Request) (string, error) {
	users, err := p.d.Users(ctx)
	if err != nil || len(users) == 0 {
		return "", err
	}
	return users[0].Login, nil
}

// Data lists the users, marks the one open, and counts them.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	users, err := p.d.Users(ctx)
	if err != nil {
		return err
	}
	data.Users = users
	data.Current = r.URLParam("login")
	data.CanAdd = auth.Viewer(ctx).HasRole("hr")
	data.UserCount = len(users)
	return nil
}

// Subscribe counts the users again whenever one is added.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	sub := p.d.UsersAdded.Subscribe()
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.Updates():
			n, err := p.d.CountUsers(ctx)
			if err != nil {
				return err
			}
			state.SetUserCount(n)
		}
	}
}
