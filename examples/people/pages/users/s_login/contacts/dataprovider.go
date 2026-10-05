package contacts

import (
	"context"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/rpc"
	"aicoded.dev/framework/web"

	"people/deps"
	"people/services/contacts"
)

var _ RouteDataProvider = &DP{}

// DP provides a user's contacts, which the contacts app holds.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Guard lets a viewer see their own contacts, and a viewer with the hr role anyone's. The
// contacts app checks the same rule again when it serves the call.
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	viewer := auth.Viewer(ctx)
	if viewer.Subject != r.URLParam("login") && !viewer.HasRole("hr") {
		return web.Forbidden()
	}
	return nil
}

// Data asks the contacts app for the user's phones and emails, on behalf of the viewer.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	out, err := contacts.Contacts(ctx, contacts.ContactsIn{Login: r.URLParam("login")})
	switch rpc.CodeOf(err) {
	case 0:
	case rpc.PermissionDenied:
		return web.Forbidden()
	case rpc.NotFound:
		return web.NotFound()
	default:
		return err
	}
	data.Phones, data.Emails = out.Phones, out.Emails
	return nil
}
