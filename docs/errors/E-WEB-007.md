# E-WEB-007: Guard is never called

The route's data provider has a method named `Guard`, but the route's `<ssr:access>` has no
`guard="true"`, so the framework would never call it and the page would open without the check.
The method may come from a type the data provider embeds, which aicoded generate cannot see in
the route's folder (see E-GEN-032), and it counts whatever its signature. The generated
`NewRoute` refuses such a data provider, so the app stops at start instead of serving the page.

**Fix:** add `guard="true"` to this route's `<ssr:access>`, or remove the Guard method
