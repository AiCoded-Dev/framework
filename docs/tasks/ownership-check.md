# Check who owns a record

Let a viewer open only their own records, and a viewer with a stronger role anyone's. The access
rule admits the role, a `Guard` on the page checks the owner, and the app that holds the data
checks again. The example is the contacts tab of a user in the people example, at
`/users/<login>/contacts`, whose data the contacts app holds.

## Steps

1. Ask for a `Guard` in the page's access rule with `guard="true"`. A page with an id in its URL
   says whose records it shows: `guard="true"` says that its `Guard` decides, and `shared="true"`
   that everyone the rule admits may see every record. `aicoded generate` refuses such a page
   with neither (E-GEN-054). The framework calls a page's `Guard` only when its own
   `<ssr:access>` has `guard="true"`, so `aicoded generate` also refuses a `Guard` method on a
   page without it (E-GEN-032):

   <!-- code: examples/people/pages/users/s_login/contacts/index.html -->
   ```html
   <ssr:access role="staff" guard="true"/>
   <ssr:var name="phones" type="[]string"/>
   <ssr:var name="emails" type="[]string"/>
   <h2>Phones</h2>
   <ul id="phones"><li ssr:for="phone in phones">{{ phone }}</li></ul>
   <h2>Emails</h2>
   <ul id="emails"><li ssr:for="email in emails">{{ email }}</li></ul>
   ```

2. Run `aicoded generate`, or save under `aicoded dev`. The page gets a `Guard` hook, which
   refuses every viewer (403) until you write it. The `Guard` runs after the access rules of
   the whole path and before any form, `Data`, live value or page call of the page. On a layout
   or a gate, it covers every page below it. A page below needs a `Guard` of its own only for a
   check of its own, such as "only the author edits", and then `guard="true"` in its own
   `<ssr:access>` too. Here the user card above, `pages/users/s_login/`, declares
   `<ssr:access role="staff" shared="true"/>`, since every member of `staff` sees every
   profile, and the contacts tab narrows it with its own `Guard`.
3. Write the rule: the viewer owns the record, or has the role that sees everyone's.

   <!-- code: examples/people/pages/users/s_login/contacts/dataprovider.go DP.Guard -->
   ```go
   // Guard lets a viewer see their own contacts, and a viewer with the hr role anyone's. The
   // contacts app checks the same rule again when it serves the call.
   func (p *DP) Guard(ctx context.Context, r *web.Request) error {
   	viewer := auth.Viewer(ctx)
   	if viewer.Subject != r.URLParam("login") && !viewer.HasRole("hr") {
   		return web.Forbidden()
   	}
   	return nil
   }
   ```

   `auth.Viewer(ctx)` is the viewer the company login verified, and `aicoded dev` uses the
   persona you chose. `Subject` is the viewer's id, here a login, and `HasRole` checks a role.
   Return `web.NotFound()` instead of `web.Forbidden()` when a viewer must not learn that the
   record exists.
4. Check the same rule again where the data is. contacts holds the contacts, so its function
   checks the viewer before it reads anything, and before it can tell whether the login exists:

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

   <!-- code: examples/contacts/rpc/rpc.go allowed -->
   ```go
   // allowed reports whether viewer may read the contacts of login: they are that person, or they
   // have the hr role.
   func allowed(viewer auth.Identity, login string) bool {
   	return viewer.Authenticated() && (viewer.Subject == login || viewer.HasRole("hr"))
   }
   ```

   The page calls contacts on behalf of the viewer, so contacts sees the same viewer:
   [Call another app](call-another-app.md) shows the call.

Compare owners exactly. Both apps keep logins in columns with `COLLATE utf8mb4_0900_bin`, which
has no case folding and no trailing-space padding, so that the database, like the `Guard`, tells
`Alice` and `alice ` from `alice`.

## Check it

- `aicoded check` generates, builds, vets, lints and tests the apps. contacts tests `allowed` as a
  plain function, with no runner, and calls `Contacts` with no viewer and no database open, so the
  test fails if the function ever reads before it checks.
- `aicoded generate` records `guard: true` for `"/users/{login}/contacts"` and `shared: true` for
  `"/users/{login}/info"` in the `access` section of people's `aicoded.yaml`, and
  `aicoded describe` shows `guard` and `shared` next to the pages.
- With the personas alice (`staff`) and bob (`staff` and `hr`): alice opens
  `/users/alice/contacts` and gets 403 on `/users/bob/contacts`, and bob opens both.

## See also

- [Access rules](../guides/access.md)
- [E-GEN-032](../errors/E-GEN-032.md): a `Guard` that no `guard="true"` asks for.
- [E-GEN-054](../errors/E-GEN-054.md): a page with an id in its URL that does not say whose
  records it shows.
- [List own records, or all of them for a role](list-by-role.md): the list that goes with the
  check, with one constant statement for each case.
- [The request pipeline](../guides/request-pipeline.md)
- [Calls between apps](../guides/calls-between-apps.md)
