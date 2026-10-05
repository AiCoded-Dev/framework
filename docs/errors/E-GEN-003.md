# E-GEN-003: `<ssr:var>` has no type

An `<ssr:var>` becomes a typed Go field that the page's data provider fills. Without a `type`
the generator cannot declare the field.

**Fix:** add a Go type, for example `type="string"`
