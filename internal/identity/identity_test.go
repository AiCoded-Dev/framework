package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/internal/identity"
)

func TestIdentityIsCopied(t *testing.T) {
	roles := []string{"editor"}
	ctx := identity.With(context.Background(), identity.Identity{Subject: "alice", Roles: roles, Groups: []string{"lisbon"}})
	roles[0] = "admin"
	id := identity.From(ctx)
	id.Roles[0], id.Groups[0] = "admin", "porto"
	assert.Equal(t, identity.Identity{Subject: "alice", Roles: []string{"editor"}, Groups: []string{"lisbon"}}, identity.From(ctx))
}

func TestTokenAndCaller(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, identity.Token(ctx))
	assert.Empty(t, identity.Caller(ctx))
	ctx = identity.WithCaller(identity.WithToken(ctx, "v1.token"), "shop")
	assert.Equal(t, "v1.token", identity.Token(ctx))
	assert.Equal(t, "shop", identity.Caller(ctx))
}
