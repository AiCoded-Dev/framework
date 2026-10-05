// Package rpc holds the functions contacts serves to other apps.
package rpc

import (
	"context"
	"errors"

	"aicoded.dev/framework/auth"
	frameworkrpc "aicoded.dev/framework/rpc"

	"contacts/deps"
)

// ContactsIn names a person by login.
type ContactsIn struct {
	Login string
}

// ContactsOut holds a person's phone numbers and email addresses.
type ContactsOut struct {
	Phones []string
	Emails []string
}

// Contacts returns the contacts of a person to that person and to hr. The people app checks the
// same rule in its Guard; this app checks it again, since it holds the data.
//
//ssr:access caller=people role=staff
func Contacts(ctx context.Context, in ContactsIn) (ContactsOut, error) {
	if !allowed(auth.Viewer(ctx), in.Login) {
		return ContactsOut{}, frameworkrpc.Error(frameworkrpc.PermissionDenied, "only the person and hr may read these contacts")
	}
	phones, emails, err := deps.Contacts(ctx, in.Login)
	if errors.Is(err, deps.ErrNoPerson) {
		return ContactsOut{}, frameworkrpc.Error(frameworkrpc.NotFound, "no such person")
	}
	if err != nil {
		return ContactsOut{}, err
	}
	return ContactsOut{Phones: phones, Emails: emails}, nil
}

// allowed reports whether viewer may read the contacts of login: they are that person, or they
// have the hr role.
func allowed(viewer auth.Identity, login string) bool {
	return viewer.Authenticated() && (viewer.Subject == login || viewer.HasRole("hr"))
}
