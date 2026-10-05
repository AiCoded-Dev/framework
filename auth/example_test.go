package auth_test

import (
	"context"
	"database/sql"
	"fmt"

	"aicoded.dev/framework/auth"
)

var db *sql.DB // the app's database, from sqldb.Open

// Store the viewer's Subject, a stable id, as the owner of what they create. The context of a hook
// or a served call carries the viewer.
func ExampleViewer() {
	process := func(ctx context.Context) error {
		_, err := db.ExecContext(ctx, "INSERT INTO notes (owner, title) VALUES (?, ?)", auth.Viewer(ctx).Subject, "Quarterly review")
		return err
	}
	_ = process
}

// An ownership rule: a viewer reads their own contacts, and one with the role hr reads anyone's.
// A page's Guard applies it to auth.Viewer(ctx), and a function of rpc/ checks it again. The
// zero Identity of a call another app makes as itself has the Subject "", so the rule admits
// only an authenticated viewer.
func ExampleIdentity_HasRole() {
	mayRead := func(viewer auth.Identity, owner string) bool {
		return viewer.Authenticated() && (viewer.Subject == owner || viewer.HasRole("hr"))
	}
	alice := auth.Identity{Subject: "alice", Name: "Alice", Roles: []string{"staff"}}
	bob := auth.Identity{Subject: "bob", Name: "Bob", Roles: []string{"staff", "hr"}}
	fmt.Println(mayRead(alice, "alice"), mayRead(alice, "bob"), mayRead(bob, "alice"), mayRead(auth.Identity{}, ""))
	// Output: true false true false
}
