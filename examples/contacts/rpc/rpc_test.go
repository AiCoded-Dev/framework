package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/auth"
	frameworkrpc "aicoded.dev/framework/rpc"
)

func TestAllowed(t *testing.T) {
	alice := auth.Identity{Subject: "alice", Roles: []string{"staff"}}
	bob := auth.Identity{Subject: "bob", Roles: []string{"staff", "hr"}}

	assert.True(t, allowed(alice, "alice"), "a person reads their own contacts")
	assert.False(t, allowed(alice, "bob"), "staff never read someone else's")
	assert.False(t, allowed(alice, "Alice"), "a login matches exactly")
	assert.True(t, allowed(bob, "alice"), "hr reads anyone's")
	assert.False(t, allowed(auth.Identity{}, ""), "a call with no viewer reads nothing")
}

// TestContactsChecksTheRuleFirst runs without deps.Start, so a lookup before the rule would
// panic on the nil database.
func TestContactsChecksTheRuleFirst(t *testing.T) {
	_, err := Contacts(context.Background(), ContactsIn{Login: "alice"})
	assert.Equal(t, frameworkrpc.PermissionDenied, frameworkrpc.CodeOf(err))
}
