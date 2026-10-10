package s_id

import (
	"context"
	"strings"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
)

var _ RouteDataProvider = &DP{}

// DP provides one message.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard answers 404 for a message the app's mail does not have, before the page reaches the
// rest of the backend.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	if _, err := p.d.Backend.MailGet(ctx, r.URLParam("app"), r.URLParam("id")); err != nil {
		return web.NotFound()
	}
	return nil
}

// Data shows the message the URL names; an app or a message the workspace does not have is
// not found.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	app := r.URLParam("app")
	m, err := p.d.Backend.MailGet(ctx, app, r.URLParam("id"))
	if err != nil {
		return web.NotFound()
	}
	data.M = Message{
		App: app, ID: m.ID, Folder: m.Folder, Stamp: m.Time.Format(time.RFC3339), Time: m.Time.Local().Format(time.DateTime),
		From: m.From, To: strings.Join(m.To, ", "), Cc: strings.Join(m.Cc, ", "), Bcc: strings.Join(m.Bcc, ", "),
		ReplyTo: m.ReplyTo, Subject: m.Subject, Text: m.Text, HTML: m.HTML != "", Attachments: m.Attachments,
	}
	return nil
}
