package exposed

import (
	"context"

	"clients/deps"
)

// DP reaches Exposed through an exported field.
type DP struct{ D *deps.Exposed }

// Data does nothing.
func (p *DP) Data(context.Context) error { return nil }
