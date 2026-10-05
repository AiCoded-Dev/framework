# E-GEN-002: `<ssr:var>` has no name

An `<ssr:var>` declares a variable the page's data provider fills. Without a `name` the
generator cannot create the field or let the template refer to it.

**Fix:** add `name="…"`
