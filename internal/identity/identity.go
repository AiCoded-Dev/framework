// Package identity carries the verified viewer of a request in its context, with the viewer's
// token and, while serving a call from another app, that app.
package identity

import (
	"context"
	"slices"
)

// Identity is a viewer verified by the company login. The zero Identity has no roles or groups.
type Identity struct {
	Subject string
	Name    string
	Groups  []string
	Roles   []string
}

// HasRole reports whether the viewer has role.
func (i Identity) HasRole(role string) bool { return slices.Contains(i.Roles, role) }

// InGroup reports whether the viewer is in group.
func (i Identity) InGroup(group string) bool { return slices.Contains(i.Groups, group) }

// Authenticated reports whether this is a verified viewer rather than the zero Identity.
func (i Identity) Authenticated() bool { return i.Subject != "" }

func (i Identity) clone() Identity {
	i.Groups, i.Roles = slices.Clone(i.Groups), slices.Clone(i.Roles)
	return i
}

type (
	ctxKey    struct{}
	tokenKey  struct{}
	callerKey struct{}
)

// With returns a copy of ctx carrying a copy of id.
func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id.clone())
}

// From returns a copy of the identity carried by ctx, or the zero Identity. Changing the copy
// does not change what ctx carries.
func From(ctx context.Context) Identity {
	id, _ := ctx.Value(ctxKey{}).(Identity)
	return id.clone()
}

// WithToken returns a copy of ctx carrying the viewer token of the request being served.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenKey{}, token)
}

// Token returns the viewer token ctx carries, or "".
func Token(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey{}).(string)
	return t
}

// WithCaller returns a copy of ctx carrying the app whose call is being served.
func WithCaller(ctx context.Context, app string) context.Context {
	return context.WithValue(ctx, callerKey{}, app)
}

// Caller returns the app whose call ctx serves, or "".
func Caller(ctx context.Context) string {
	app, _ := ctx.Value(callerKey{}).(string)
	return app
}
