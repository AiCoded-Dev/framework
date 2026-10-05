package auth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/auth"
	"aicoded.dev/framework/internal/identity"
)

func TestViewer(t *testing.T) {
	none := auth.Viewer(context.Background())
	assert.False(t, none.Authenticated())
	assert.False(t, none.HasRole("admin"))

	ctx := identity.With(context.Background(), identity.Identity{Subject: "alice", Roles: []string{"hotel-ops"}, Groups: []string{"lisbon"}})
	v := auth.Viewer(ctx)
	assert.True(t, v.Authenticated())
	assert.True(t, v.HasRole("hotel-ops"))
	assert.True(t, v.InGroup("lisbon"))
	assert.False(t, v.InGroup("porto"))
}
