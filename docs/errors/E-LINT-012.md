# E-LINT-012: cookie not __Host-

Every app runs on a host of its own, beside other apps under the same domain. A cookie
whose name starts with `__Host-` and that has `Secure` and no `Domain` stays on the app's own host
and on HTTPS: the browser refuses to let any other host set or change it, and never sends it to
another host. Any other cookie can reach, or be planted by, the apps beside it. So app code may
set only `__Host-` cookies with `Secure: true` and no `Domain`. Browsers also need `Path: "/"` on
a `__Host-` cookie, or they drop it.

Lint reads, in hand-written and generated files:

- every `http.Cookie` composite literal, and every literal of a type of the app with the fields
  of `http.Cookie`: its `Name` must be a constant that starts with `__Host-`, its `Secure` the
  constant `true`, and its `Domain` left out or the constant `""`;
- every change of the `Name`, `Secure` or `Domain` of a cookie: only `=` with a constant that
  the literal could hold, one field at a time, and never `&c.Secure` or the like;
- an empty cookie made with `new(http.Cookie)` or `var c http.Cookie`;
- every call of `Add` or `Set` of an `http.Header` whose key is the constant `Set-Cookie`, in any
  case: its value must be the `String()` of a cookie, which the rules above check where it is
  made, or a constant `__Host-` cookie with `Secure` and no `Domain`;
- a `Set-Cookie` key written into a header map, by index or in a composite literal.

`http.SetCookie` is E-LINT-009. A header key that is not a constant is not read. On the
platform, the runner also takes `Domain` off every cookie an app sets and refuses one without
`Secure`.

```go
c := &http.Cookie{Name: "__Host-theme", Value: theme, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
w.Header().Add("Set-Cookie", c.String())
```

**Fix:** name the cookie with a constant that starts with __Host-, set Path: "/" and Secure: true, leave out Domain, and set it with w.Header().Add("Set-Cookie", c.String())
