# E-GEN-032: Guard is never called

The route's folder has a method named `Guard`, but the route's template has no `<ssr:access>`
with `guard="true"`. The framework calls a route's Guard only when its template declares it, so
this check would never run and the page would open without it. With `guard="true"`, `Guard`
becomes part of the route's data provider interface, so a missing or misspelt Guard does not
compile. Files ending in `_gen.go` or `_test.go` are not checked.

**Fix:** add `guard="true"` to this route's `<ssr:access>`, or remove the Guard method
