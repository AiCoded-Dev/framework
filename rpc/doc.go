// Package rpc lets one app call functions of another. The called app declares them in its own
// rpc/ folder, package rpc, as plain Go functions with an //ssr:access line:
//
//	// Contacts returns the phones and email addresses of a user.
//	//
//	//ssr:access caller=people role=staff
//	func Contacts(ctx context.Context, in ContactsIn) (ContactsOut, error)
//
// caller= names the apps that may call the function. role= is the rule for calls made on a
// viewer's behalf: roles joined by |, or * for every viewer. apps=true lets the listed apps call
// with no viewer. Inside rpc/, import this package under another name, such as frameworkrpc.
//
// aicoded generate writes the server, rpc/server_gen.go, whose rpc.Server() main passes to
// app.Main as RPC, and the snapshot .aicoded/rpc.json. In the calling app, aicoded rpc add <app>
// copies the snapshot and generates a typed client in services/<app>/. Call it with the context
// of the request being served, so that the call goes on behalf of its viewer. The runner and the
// called app both check every call against the permission lists of the two apps.
//
// A function tells its caller why it failed with [Error] or [Errorf] and one of six codes:
// [InvalidArgument], [NotFound], [AlreadyExists], [PermissionDenied], [FailedPrecondition] and
// [Unavailable]. The caller reads the code with [CodeOf]. Any other error, and a panic, reach the
// caller as [Internal] with the message "internal error", so the called app's details stay in its
// own logs. A function that returns a viewer's records checks ownership itself, with
// auth.Viewer(ctx): the calling app's checks are not its own.
//
// Read more in the guide docs/guides/calls-between-apps.md and the task
// docs/tasks/call-another-app.md, which aicoded explain and the MCP tool howto print as
// guides/calls-between-apps and tasks/call-another-app.
package rpc
