// Package deps holds what shop learned from billing when it started.
package deps

import (
	"context"

	"shop/services/billing"
)

// Deps is filled by Start before the app serves.
type Deps struct {
	// Pong is what billing's Ping told shop itself.
	Pong string
	// Secret is how billing answered Secret to shop itself.
	Secret string
}

// Start calls billing as shop itself: the context OnStart gets has no viewer.
func (d *Deps) Start(ctx context.Context) error {
	pong, err := billing.Ping(ctx, billing.PingIn{})
	if err != nil {
		return err
	}
	d.Pong = "caller=" + pong.Caller + " viewer=" + pong.Viewer
	d.Secret = "allowed"
	if _, err := billing.Secret(ctx, billing.PingIn{}); err != nil {
		d.Secret = err.Error()
	}
	return nil
}
