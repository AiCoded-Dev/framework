package s_app

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"time"

	"aicoded.dev/framework/web"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/cmd/aicoded/internal/devui/deps"
	"aicoded.dev/framework/internal/errs"
)

var _ RouteDataProvider = &DP{}

// DP provides the mail of the app the URL names.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard answers 404 for a name that is not an app of the workspace, before the form or the
// live connection reaches the backend.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	st, err := p.d.Backend.Status(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(st.Apps, func(a devapi.App) bool { return a.Name == r.URLParam("app") }) {
		return web.NotFound()
	}
	return nil
}

// Data lists the mail of the app; an app the workspace does not have is not found.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	data.App = r.URLParam("app")
	mail, err := p.d.Backend.Mail(ctx, data.App)
	if errs.Code(err) == "E-DEV-014" {
		return web.NotFound()
	}
	if err != nil {
		return err
	}
	data.Mail = messages(mail)
	return nil
}

// Subscribe lists the mail again at every change of the workspace.
func (p *DP) Subscribe(ctx context.Context, r *web.Request, state *ReactiveState) error {
	app := r.URLParam("app")
	return deps.Follow(ctx, p.d.Backend, func(ctx context.Context) {
		if mail, err := p.d.Backend.Mail(ctx, app); err == nil {
			state.SetMail(messages(mail))
		}
	})
}

// InitReceive leaves the form empty.
func (p *DP) InitReceive(context.Context, *web.Request, web.ResponseWriter, *FormReceiveValues) error {
	return nil
}

// ProcessReceive puts the message into the app's inbox and shows the mail again; it shows why
// when the workspace refuses the message.
func (p *DP) ProcessReceive(ctx context.Context, r *web.Request, _ web.ResponseWriter, f *FormReceiveValues) error {
	app := r.URLParam("app")
	m := devapi.InboundMail{From: f.From.GetValue(), Subject: f.Subject.GetValue(), Text: f.Text.GetValue()}
	if _, err := p.d.Backend.MailReceive(ctx, app, m); err != nil {
		f.SetError(err.Error())
		return nil
	}
	return web.Redirect("/mail/" + url.PathEscape(app))
}

func messages(mail []devapi.MailSummary) []Message {
	var out []Message
	for _, m := range mail {
		out = append(out, Message{
			ID: m.ID, Folder: m.Folder, Stamp: m.Time.Format(time.RFC3339), Time: m.Time.Local().Format(time.DateTime),
			From: m.From, To: strings.Join(m.To, ", "), Subject: m.Subject,
		})
	}
	return out
}
