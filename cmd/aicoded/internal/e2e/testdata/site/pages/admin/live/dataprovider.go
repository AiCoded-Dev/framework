package live

import (
	"context"
	"net/http"
	"time"

	"aicoded.dev/framework/web"

	"site/deps"
)

// Probe is a value that turns into an image, and runs a script, if a page ever renders it as markup.
const Probe = `<img src=x onerror="document.title='pwned'">`

var _ RouteDataProvider = &DP{}

// DP drives the live page.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data starts with the probe as the message.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Message = Probe
	return nil
}

// Subscribe counts ticks while the page is open.
func (p *DP) Subscribe(ctx context.Context, _ *web.Request, state *ReactiveState) error {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			state.SetTicks(n)
		}
	}
}

// ValidateMessage refuses long messages.
func (p *DP) ValidateMessage(_ context.Context, _ *web.Request, val string) (string, error) {
	if len(val) > 200 {
		return "", web.Error(http.StatusBadRequest, "The message is too long.")
	}
	return val, nil
}
