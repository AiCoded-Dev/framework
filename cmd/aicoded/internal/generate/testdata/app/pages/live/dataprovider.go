package live

import (
	"context"
	"errors"
	"net/http"

	"aicoded.dev/framework/web"

	"example.com/app/deps"
)

var _ RouteDataProvider = &DP{}

// DP drives the live page.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data sets the starting values.
func (p *DP) Data(_ context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.Count = 1
	data.Items = []string{"a"}
	return nil
}

// Subscribe pushes nothing.
func (p *DP) Subscribe(context.Context, *web.Request, *ReactiveState) error { return nil }

// ValidateCount refuses negative counts, with a message for the viewer, and counts it cannot
// store.
func (p *DP) ValidateCount(_ context.Context, _ *web.Request, val int) (int, error) {
	switch {
	case val < 0:
		return 0, web.Error(http.StatusBadRequest, "The count must not be negative.")
	case val > 1000:
		return 0, errors.New("the store takes counts up to 1000")
	}
	return val, nil
}
