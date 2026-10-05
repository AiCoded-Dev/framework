# E-WEB-002: redirect off this site

A hook returned `web.Redirect` with a target that is not a path on this site, for example a full
URL or a path starting with `//`. The framework refuses it, logs this error and answers with 500,
so a page can never send viewers to another site.

**Fix:** redirect to a path that starts with a single `/`, such as `/notes/42`.
