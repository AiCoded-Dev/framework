# E-WEB-005: redirect from a page call

A page call returned `web.Redirect`. A call answers a script on the page, not a navigation, so
the framework logs this error and the call fails with status 500.

**Fix:** return data from the call and navigate in index.ts with `location.assign("/path")`.
