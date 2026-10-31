package pages

import (
	"context"

	"clients/deps"
)

// DP reaches Deps through an exported field.
type DP struct{ D *deps.Deps }

// Data reads today's weather.
func (p *DP) Data(ctx context.Context) error {
	_, err := p.D.Today(ctx)
	return err
}
