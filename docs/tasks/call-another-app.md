# Call another app

Let one app call Go functions of another, on behalf of the viewer, with the runner and both apps
checking each call. The example is the contacts tab of the people example, which asks the
contacts app for a user's phones and emails.

## In the app that serves the call

1. Put the function in the app's `rpc/` folder, package `rpc`, with the shape
   `func Name(ctx context.Context, in In) (Out, error)`, where `In` and `Out` are structs of
   `rpc/`, and an `//ssr:access` line in its doc comment:

   <!-- code: examples/contacts/rpc/rpc.go ContactsIn -->
   ```go
   // ContactsIn names a person by login.
   type ContactsIn struct {
   	Login string
   }
   ```

   <!-- code: examples/contacts/rpc/rpc.go ContactsOut -->
   ```go
   // ContactsOut holds a person's phone numbers and email addresses.
   type ContactsOut struct {
   	Phones []string
   	Emails []string
   }
   ```

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

   - `caller=people` names the apps that may call the function, by the `app:` line of their
     permission lists.
   - `role=staff` is the role a viewer needs for a call made on their behalf.
   - Return `rpc.Error` with a code, such as `rpc.PermissionDenied` or `rpc.NotFound`, to tell
     the caller why a call failed. Any other error reaches the caller as `internal error`.
   - The folder's own package is `rpc`, so the framework's `rpc` package is imported under
     another name.
2. Serve the functions from `main.go`:

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

3. Run `aicoded generate`. It writes the server, `rpc/server_gen.go`, the snapshot of the
   functions, `.aicoded/rpc.json`, and the `services` section of the permission list. Never edit
   them.

## In the app that calls

1. In the calling app's folder, run `aicoded rpc add contacts`. It finds the app named contacts
   in the folder that holds this app's folder, or below it, and copies its snapshot to
   `.aicoded/services/contacts.json`. Then it generates this app, which writes the client,
   `services/contacts/client_gen.go`, and prints
   `aicoded rpc add: call the functions of contacts through the package in services/contacts`.
   Run it again to pick up new functions.
2. Call the client's function of the same name with the context of the request, so that the call
   goes on behalf of the viewer, and turn the codes into answers of the page:

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

3. `aicoded generate` adds the function to the `services` section of the calling app's
   permission list, as `"contacts": ["Contacts"]` under `calls`. The runner lets through only
   the calls that both permission lists name.

## Check it

- `aicoded check` in the folder that holds both apps generates, builds, vets, lints and tests
  them, and checks the snapshot people holds against contacts' functions.
- `aicoded dev` in that folder starts contacts first, since people calls it, and prints
  `aicoded dev: contacts (calls only)`.
- Open `http://people.localhost:8080/users/alice/contacts` as alice, a persona with the `staff`
  role. The dev UI's Traces page shows the request as one trace across both apps, with people's
  `rpc.call` span.

## See also

- [Calls between apps](../guides/calls-between-apps.md)
- [The permission list](../guides/permission-list.md)
- [aicoded dev](../guides/dev.md)
