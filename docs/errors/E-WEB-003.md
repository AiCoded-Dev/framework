# E-WEB-003: no access rule on the path

No route from the root to the requested page has an `<ssr:access>` rule, so the page is closed
to everyone. aicoded generate refuses such pages (E-GEN-030); this error means the generated code
is out of date.

**Fix:** add `<ssr:access role="…"/>` to the page or to a parent folder's template and run
`aicoded generate`.
