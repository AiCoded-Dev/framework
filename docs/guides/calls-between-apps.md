# Calls between apps

An app can call Go functions of another app as if they were its own. The called app declares who
may call each function; `aicoded generate` writes the code on both sides; and the runner carries
each call, checks it against the permission lists of both apps, and passes the viewer on.

## Serving calls

The called app puts its functions in its `rpc/` folder, `package rpc`. Each exported function has
the shape `func Name(ctx context.Context, in In) (Out, error)`, where `In` and `Out` are exported
structs declared in `rpc/`, and an `//ssr:access` line in its doc comment. The contacts example
serves one:

<!-- code: examples/contacts/rpc/rpc.go Contacts -->
```go
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
```

The framework's `rpc` package is imported under another name, `frameworkrpc`, because the
folder's own package is `rpc` too.

- `caller=` names the apps that may call the function, by the `app:` names of their permission
  lists, separated by commas. It is required.
- `role=` is the rule for calls made on behalf of a viewer: roles joined by `|`, one of which the
  viewer needs, or `*` for every viewer.
- `apps=true` lets the listed apps call the function on their own, with no viewer.
- A function needs `role=`, `apps=true` or both. `aicoded generate` refuses:
  - a function without the line (E-RPC-001);
  - a line it cannot read, a line anywhere but right above an exported function, and a line
    written `// ssr:access`, with a space after `//` (E-RPC-002);
  - a function of another shape, or a name that Go or `rpc/server_gen.go` already uses, such as
    a type `string` or a function `Server` (E-RPC-003);
  - a field of a type it cannot send (E-RPC-004).

The rule in `//ssr:access` is checked before the function runs. Ownership is the function's own
check: `Contacts` refuses a viewer who is neither the person nor `hr`, even though the calling
page checked the same rule, because the app that holds the data never trusts its caller to have
checked.

### The types

The fields of `In`, `Out` and the structs they use are exported and named, not embedded. Each
field has one of these types:

- `bool`, `int32`, `int64`, `uint32`, `uint64`, `float32`, `float64`, `string`, `[]byte`,
  `time.Time`, or another exported struct of `rpc/`;
- a slice of one of those;
- a pointer to one of those other than `[]byte`, for an optional value.

`int`, `uint`, the 8- and 16-bit integers, maps, arrays, interfaces, types of other packages,
slices of pointers and pointers to slices are refused; use `int64` for whole numbers.

### The generated files

`aicoded generate` writes two files; never edit either:

- `rpc/server_gen.go`, the code that decodes calls and encodes answers;
- `.aicoded/rpc.json`, the snapshot of the functions, which pins the number of every field.

A field keeps its number, and the number of a removed field is never used again in its struct. A
struct that no function reaches any more, or that is renamed, loses its numbers: when it comes
back, or under its new name, its fields are numbered afresh. Removing or renaming a function or a
field, or changing a field's type or number, breaks the apps that call it. Before it writes
anything, `aicoded generate` checks the change against every snapshot of the app that another app
of the workspace holds, and refuses one that breaks a snapshot (E-RPC-013); in `aicoded dev`, the
called app then fails alone. Add a new field or function instead. A refused change leaves
`.aicoded/rpc.json` as it was, so undoing the change in `rpc/` is enough; to keep it, run
`aicoded rpc add <app>` in each calling app and update its calls. A snapshot edited by hand is
refused (E-RPC-005).

`main` passes the generated server to `app.Main` as `RPC`, next to `Handler` when the app has
pages. An app with no pages passes only `RPC`:

<!-- code: examples/contacts/main.go -->
```go
// Contacts serves the phone numbers and email addresses of people to the people app.
package main

import (
	"aicoded.dev/framework/app"

	"contacts/deps"
	"contacts/rpc"
)

func main() {
	app.Main(app.Options{RPC: rpc.Server(), OnStart: deps.Start})
}
```

## Calling

In the calling app's folder, run `aicoded rpc add contacts`. It looks for the app named
`contacts` in the folder that holds the calling app's folder and below it, generates it, and
copies its snapshot to `.aicoded/services/contacts.json`. Then it generates the calling app, which
writes the client `services/contacts/client_gen.go`, package `contacts`: the app's name without
dashes, so `billing-eu` gets `services/billingeu/`. Run it again to pick up the called app's new
functions. The folder of a client holds only the generated client (E-RPC-014).

Before it writes anything in the calling app, `aicoded rpc add` refuses:

- a name that no app there has or two apps have, the calling app itself, and an app with no
  exported function in `rpc/` (E-RPC-007);
- an app whose client package name Go reserves or another of this app's clients already has
  (E-RPC-006).

The client has a function of the same name and types for every function of the called app. Call
it with the context of the request being served, and turn the codes you expect into answers:

<!-- code: examples/people/pages/users/s_login/contacts/dataprovider.go DP.Data -->
```go
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
```

### Viewers and apps

- A call made with the context of a viewer's request, or of a call made on a viewer's behalf,
  goes on behalf of that viewer. The called function sees the same viewer in `auth.Viewer(ctx)`,
  and `rpc.Caller(ctx)` names the calling app.
- A call made with a context that has no viewer goes as the calling app itself. Examples are the
  context `OnStart` gets, and one serving a call an app made on its own. The function needs
  `apps=true` (E-RPC-010), and there `auth.Viewer(ctx)` is the zero viewer.
- A function with `apps=true` and no `role=` serves only calls the listed apps make on their own.
  A call on a viewer's behalf gets `rpc.PermissionDenied`.
- Call with the context of the request or call being served, never with one kept from an earlier
  request: the runner accepts a forwarded viewer token that is at most 10 minutes old, the
  longest a live page connection lasts (E-RPC-012).

### Errors

- To tell the caller why a call failed, a function returns `rpc.Error(code, msg)` or
  `rpc.Errorf(code, format, …)` with `rpc.InvalidArgument`, `NotFound`, `AlreadyExists`,
  `PermissionDenied`, `FailedPrecondition` or `Unavailable`. The caller reads the code with
  `rpc.CodeOf(err)`, which is 0 for no error, and the message, at most 1 KiB, in `err.Error()`.
- Any other error, and a panic, reach the caller as `rpc.Internal` with the message
  `internal error`, and the called app logs the real one. Never put anything in a message that
  the calling app must not see.
- A viewer without the function's role gets `rpc.PermissionDenied`.
- A call the runner refuses fails with a coded error in `err.Error()`, E-RPC-008 to E-RPC-012.
  `rpc.CodeOf` gives `PermissionDenied` for E-RPC-008 to E-RPC-010, `Unavailable` for E-RPC-011
  and `Internal` for E-RPC-012. The called app checks E-RPC-009 and E-RPC-010 again, and its
  refusal reaches the caller as `rpc.PermissionDenied`, with a text that names the same code.

### Deadlines

A call follows the deadline of its context, or gets 10 seconds when the context has none. The
runner cuts every call at 30 seconds. A call cut short fails with an error that `errors.Is`
matches with `context.DeadlineExceeded`, or `context.Canceled` when its context was cancelled.

## The services section

`aicoded generate` writes the `services` section of the permission list, as it writes `access`.
Never edit it. In the examples it is:

```yaml
# people/aicoded.yaml
services:
  calls:
    "contacts": ["Contacts"]
```

```yaml
# contacts/aicoded.yaml
access: {}
services:
  serves:
    "Contacts":
      callers: ["people"]
      require: ["staff"]
```

- `calls` names, for each app this app calls, the functions of its generated client that this
  app's code refers to. The runner lets through only these (E-RPC-008). `aicoded generate` finds
  them by name, without type-checking: in a file that imports a client, `contacts.Contacts`
  counts even where `contacts` is a local variable, and the calls of a client imported with `.`
  are not found. Keep a client's package name for the client alone, and never dot-import it.
- `serves` names every function of `rpc/` with:
  - `callers`, the apps that may call it;
  - `require`, the rules a viewer must meet, written as in `access` (left out without `role=`,
    so only app calls are served);
  - `apps: true` when apps may call it with no viewer.

  The runner and the called app both check them (E-RPC-009, E-RPC-010).
- An app with no pages gets `access: {}`.

## Several apps in aicoded dev

Run `aicoded dev` in a folder above the apps, such as `examples/`, which holds `people/` and
`contacts/`. It finds and generates every app, checks the snapshots each app holds, and starts
each app after the apps it calls, so an app may call them in `OnStart`. It prints
`aicoded dev: contacts (calls only)` for an app that has no pages and serves calls. See
[aicoded dev](dev.md).

## See also

- [The permission list](permission-list.md): the `services` section next to the others.
- [Access](access.md): the page's own checks before it calls.
- [contacts/rpc/rpc.go](../../examples/contacts/rpc/rpc.go) and
  [people/pages/users/s_login/contacts](../../examples/people/pages/users/s_login/contacts): both
  sides of a call.
- [Call another app](../tasks/call-another-app.md): a function in `rpc/`, `aicoded rpc add` and
  the call.
