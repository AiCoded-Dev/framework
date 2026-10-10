# Access: rules, Guards and the order of checks

Every page is closed until an access rule on its path opens it. Access rules check the viewer's
roles; a Guard checks what only the data can tell, such as who owns a record. Both run, root
first, before anything else a page does. A page with an id in its URL also says whose records it
shows.

## Access rules

```html
<ssr:access role="staff"/>
<ssr:access role="staff,hr"/>
<ssr:access role="*"/>
<ssr:access role="staff" guard="true"/>
<ssr:access role="staff" shared="true"/>
```

- `role` lists roles separated by commas, and a list admits anyone with one of its roles. A
  role is lower-case letters, digits and `-`, starting with a letter, at most 63 characters.
- `role="*"` admits every viewer the company login lets into the app. It stands alone.
- A template has at most one `<ssr:access>`, and it cannot be inside `ssr:if`, `ssr:for` or
  `<ssr:form>`: the rule always applies (E-GEN-031).
- The rule can sit in the page's own template or in any template above it, and every rule on
  the path applies: the viewer needs a role from each. In the people example the root admits
  `*`, `pages/users/` asks for `staff` and `pages/users/add/` for `hr`, so adding a user takes
  both `staff` and `hr`.
- To require two roles, put them on templates at different levels of the path, as the people
  example does. A rule that adds nothing to a rule above it is refused by `aicoded generate`
  (E-GEN-053): a list of two or more different roles that includes every role of the rule
  above, such as `role="staff,hr"` below `role="staff"`, which admits every member of `staff`;
  and `role="*"` without `guard="true"` or `shared="true"` below a rule that is not `*`, which
  does not open the page to every viewer. A single role repeated from above, such as `role="staff"` below
  `role="staff"`, is allowed, and so are `role="*"` below `role="*"` and a single role or `*`
  with `guard="true"` or `shared="true"`.
- A page with no rule on its path is refused by `aicoded generate` (E-GEN-030). Pages are closed
  by default: a page nobody may open is safer than one everybody may open by mistake.

`aicoded generate` writes every page's rules into the `access` section of
[the permission list](permission-list.md), so security sees who may open what.

## The viewer

`auth.Viewer(ctx)` returns the viewer of the request: the person the company login verified,
or, on your computer, the persona you chose.

| Field or method | What it is |
|---|---|
| `Subject` | the stable id of the person; compare it to decide ownership |
| `Name` | the name to show |
| `Roles`, `HasRole("hr")` | the roles that access rules check |
| `Groups`, `InGroup("lisbon")` | the groups the person is in |
| `Authenticated()` | false for the zero viewer, which a context outside a request has |

Decide ownership by `Subject`, never by `Name`, which two people can share.

## Guards

`guard="true"` declares the page's `Guard`, which `aicoded generate` adds to its data provider:

```go
Guard(ctx context.Context, r *web.Request) error
```

- It runs after the access rules of every page on the path, root first, and before the forms,
  `Data` and the render of the page. It also runs before the page's live connection opens and
  before every page call.
- A Guard on a layout or a gate covers every page below it.
- It returns `nil` to let the viewer in, or an error that ends the request: `web.Forbidden()`
  (403), or `web.NotFound()` (404) to hide that the record exists.
- The stub `aicoded generate` writes refuses every viewer with `web.Forbidden()` until you write
  the check.
- A `Guard` method on a page whose `<ssr:access>` has no `guard="true"` would never run, so
  `aicoded generate` refuses it (E-GEN-032), and so does the app when it starts (E-WEB-007).

The contacts tab of the people example lets a viewer see their own contacts, and a viewer with
`hr` anyone's:

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

When the data lives in another app, that app checks the same rule again, since it holds the
data and cannot trust its caller to have checked; see [calls between apps](calls-between-apps.md).

## Whose records a page shows

A page with a parameter in its URL, such as `/users/{login}` from the folder
`pages/users/s_login/`, shows one record of many, so it says who may see which record. Every
template at or below a parameter folder, page or layout, needs a declaration on its own
`<ssr:access>` or on that of a template between it and its deepest parameter folder, and
`aicoded generate` refuses a template without one (E-GEN-054):

- `guard="true"` when a viewer may see only some of the records, such as their own: the page's
  `Guard` checks the viewer against the record the URL names, as above;
- `shared="true"` when everyone the access rules admit may see every record, such as a staff
  directory that every member of `staff` reads.

A declaration on a layout covers the pages below it, and `guard="true"` below `shared="true"`
narrows it. In the people example every member of `staff` sees every profile, so the user card,
a layout, declares `shared="true"`, which its info tab inherits; the contacts tab keeps its own
`Guard`:

```html
<!-- pages/users/s_login/index.html -->
<ssr:access role="staff" shared="true"/>
<!-- pages/users/s_login/contacts/index.html -->
<ssr:access role="staff" guard="true"/>
```

- Only a declaration from the deepest parameter folder down counts, since that parameter names
  the record the page shows. A `Guard` on `pages/users/` runs for `/users/{login}` too, but does
  not say whose profile the card shows.
- `shared="true"`, whose only value is `"true"`, is refused next to `guard="true"`, on a template
  whose URL has no parameter, and below a template with `guard="true"`, whose `Guard` still
  decides who sees each record (E-GEN-055).
- `role="*" shared="true"` keeps the rules above as they are, as `role="*" guard="true"` does.

The [permission list](permission-list.md) records `shared: true` for each page that shows every
record to everyone its rules admit, and `guard: true` for each page a `Guard` checks, so security
sees which is which. The simulated attacks of the delivery pipeline test each guarded page with
another viewer, who did not enter the record and holds only the roles that the access rules ask
for up to the page that declares the `Guard`: see [simulated attacks](simulated-attacks.md).

## The order of checks

For a page load or a form post, in this order:

1. The path is matched; a path that matches no page answers 404.
2. A `POST` that the browser marks as coming from another site is refused (403).
3. The access rule of every page on the path, root first. A viewer without a role from each
   gets 403.
4. The `Guard` of every page on the path that declares one, root first.
5. For a `POST`, the form token; a form without a valid one gets 403.
6. `Init<Form>` of the forms of the layouts and the page, then, for a `POST`, `Process<Form>` of
   the posted form when every field is valid.
7. `Data` of each layout on the path and of the page, root first.
8. The page is rendered into a buffer and sent only when it rendered whole.

So a check in a layout or a gate always runs before anything a page below it does. The live
connection of a page matches the path (step 1), then refuses any request from another site or
with an opaque origin (403), then runs steps 3 and 4 and `Data` before it opens; every page call
runs steps 3 and 4 again. See [the request pipeline](request-pipeline.md).

## Hiding is not access

A menu that hides a link from viewers without a role is a convenience. The people example's
root layout declares `<ssr:var name="staff" type="bool"/>`, which its `Data` sets with
`auth.Viewer(ctx).HasRole("staff")`, and shows the link only when it is true:

```html
<a href="/users" ssr:if="staff">Users</a>
```

The access rule of `/users` is what keeps them out. Check roles in access rules, ownership in a
`Guard`, and every value from the browser in the hook that uses it.

## See also

- [Routing](routing.md): layouts and gates, which carry rules and Guards to the pages below.
- [Forms](forms.md): the form token and checking what a form sends.
- [people/pages/users/s_login/contacts](../../examples/people/pages/users/s_login/contacts):
  a page with a `Guard`.
- [Check who owns a record](../tasks/ownership-check.md): a `Guard` on the page, and the same
  check in the app that holds the data.
- [E-GEN-054](../errors/E-GEN-054.md) and [E-GEN-055](../errors/E-GEN-055.md): a page with an id
  in its URL that does not say whose records it shows, and `shared="true"` where it means
  nothing.
