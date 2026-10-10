# E-GEN-054: a page with an id in its URL does not say whose records it shows

The template's URL has a parameter, such as `/tickets/{id}` from the folder
`pages/tickets/n_id/`, so the page shows one record of many, but nothing says who may see which
record. Every template at or below a parameter folder, page or layout, needs a declaration on
its own `<ssr:access>` or on that of a template between it and its deepest parameter folder:

- `guard="true"` when a viewer may see only some of the records, such as their own: the
  page's `Guard` loads the record from the id in the URL and returns `web.Forbidden()` unless the
  viewer may see it;
- `shared="true"` when everyone the access rules admit may see every record, such as a staff
  directory that every member of `staff` reads.

A declaration on a layout covers the pages below it. Only a declaration from the deepest
parameter folder down counts, since that parameter names the record the page shows: a `Guard` on
`pages/tickets/` runs for `/tickets/{id}` too, but does not say whose ticket the page shows.

```html
<!-- pages/tickets/n_id/index.html: a viewer sees their own tickets -->
<ssr:access role="staff" guard="true"/>
<!-- pages/users/s_login/index.html: every member of staff sees every profile -->
<ssr:access role="staff" shared="true"/>
```

The error is at the template's `<ssr:access>`, or at line 1 when it has none, and names the
route. See [access](../guides/access.md).

**Fix:** add `guard="true"` to the page's `<ssr:access>` and check the viewer in its `Guard`, or
`shared="true"` when everyone the rule admits may see every record
