# E-LINT-012: cookie not __Host-

Every app runs on a host of its own, beside other apps under the same domain. A cookie whose
name starts with `__Host-` and that has `Path=/`, `Secure` and no `Domain` stays on the app's own
host and on HTTPS: the browser refuses to let any other host set or change it, never sends it to
another host, and drops a `__Host-` cookie that lacks one of these. Any other cookie can reach, or
be planted by, the apps beside it. So app code sets only `__Host-` cookies with `Path: "/"`,
`Secure: true` and no `Domain`, and reads only `__Host-` cookies, which no other app can have set.
A cookie that deletes one, with `MaxAge: -1`, follows the same rules.

Lint reads, in hand-written and generated files:

- every `http.Cookie` composite literal, and every literal of a type of the app with the fields
  of `http.Cookie`: its `Name` must be a constant that starts with `__Host-`, its `Path` the
  constant `"/"`, its `Secure` the constant `true`, and its `Domain` left out or the constant `""`;
- every change of the `Name`, `Path`, `Secure` or `Domain` of a cookie: only `=` with a constant
  that the literal could hold, for each field, and never `&c.Secure` or the like;
- an empty cookie made with `new(http.Cookie)` or `var c http.Cookie`;
- every call of `Add` or `Set` of an `http.Header` whose key is the constant `Set-Cookie`, in any
  case: its value must be the `String()` of a cookie, which the rules above check where it is
  made, or a constant `__Host-` cookie with `Path=/`, `Secure` and no `Domain`;
- a `Set-Cookie` key written into a header map, by index or in a composite literal;
- every call of `Cookie` or `CookiesNamed` of the request whose name is a constant: it must start
  with `__Host-`;
- `Add` and `Set` of an `http.Header`, and `Cookie` and `CookiesNamed` of the request, passed
  around as values instead of called, since lint could not read what they set or read.

`http.SetCookie` is E-LINT-009. Lint cannot see a header key or a cookie name that is not a
constant, a cookie value taken from the request or from decoded data and set again with
`.String()`, the cookies that `r.Cookies()` or the `Cookie` header hand out, empty cookies inside
containers (`make([]http.Cookie, n)`, arrays, struct fields), or cookies that the page's
JavaScript sets; on the platform the runner is the boundary. It takes `Domain` off every cookie an
app sets and drops one without `Secure`.

```go
c := &http.Cookie{Name: "__Host-theme", Value: theme, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
w.Header().Add("Set-Cookie", c.String())
```

**Fix:** name the cookie with a constant that starts with __Host-, set Path: "/" and Secure: true, leave out Domain, set it with w.Header().Add("Set-Cookie", c.String()), and read only cookies whose name starts with __Host-
