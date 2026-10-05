package auth

import (
	"context"

	"aicoded.dev/framework/internal/identity"
)

// Identity is a viewer verified by the company login. The zero Identity has no roles or groups.
type Identity = identity.Identity

// Viewer returns the viewer of the request ctx belongs to, or the zero Identity outside a request.
func Viewer(ctx context.Context) Identity {
	return identity.From(ctx)
}
