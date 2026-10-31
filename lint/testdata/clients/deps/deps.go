// Package deps holds what the pages share.
package deps

import (
	"context"
	"io"
	"net/http"

	"aicoded.dev/framework/outbound"
)

// Deps keeps its client in an unexported field, as the guide shows.
type Deps struct {
	weather *http.Client
}

// New returns Deps with its client.
func New() *Deps { return &Deps{weather: outbound.Client()} }

// Today reads today's weather.
func (d *Deps) Today(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.weather.example/v1/today", http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := d.weather.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// Exposed keeps its client in an exported field.
type Exposed struct {
	Weather *http.Client
}

// NewExposed returns Exposed with its client.
func NewExposed() *Exposed { return &Exposed{Weather: outbound.Client()} }
