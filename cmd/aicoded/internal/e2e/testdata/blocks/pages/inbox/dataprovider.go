package inbox

import (
	"context"

	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/web"

	"blocks/deps"
)

var _ RouteDataProvider = &DP{}

// DP lists the app's inbox.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists the newest messages in the inbox.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	mail, _, err := mailer.List(ctx, mailer.Inbox, 0, 50)
	if err != nil {
		return err
	}
	data.Mail = mail
	return nil
}
