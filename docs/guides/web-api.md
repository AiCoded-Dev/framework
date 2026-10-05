# The web API: what a data provider uses

A page's data provider uses a small part of three packages: `web` for the request, errors,
redirects and safe markup, `web/form` for form fields, and `web/reactive` for the hubs that wake
live pages. The rest of these packages is there for the generated code.

## Package web

```go
import "aicoded.dev/framework/web"
```

### The request

`*web.Request` embeds `*http.Request`, so `r.Context()`, `r.URL.Query()`, `r.Header` and
`r.Cookie` work as usual. It adds:

| Method | Returns |
|---|---|
| `r.URLParam(name)` | the part of the URL that the folder `s_<name>` or `n_<name>` matched, or `""` |
| `r.URLParamInt(name)` | the number that the folder `n_<name>` matched, as an `int64`, or 0 |

Use the context of the request, `ctx` or `r.Context()`, for everything the app calls: it carries
the viewer, the runner and the trace. A building block called with `context.Background()`
fails (E-RUN-004).

### The response

`web.ResponseWriter` has only `Header()`: hooks set headers, and the framework writes the page.
Set a cookie with `w.Header().Add("Set-Cookie", c.String())`. The framework sets the security
headers itself and replaces any a hook set. On a page's live connection, `Data` gets a
`ResponseWriter` whose headers are never sent.

### Errors and redirects

| Function | Answer |
|---|---|
| `web.NotFound()` | 404 "This page does not exist." |
| `web.Forbidden()` | 403 "You do not have access to this page." |
| `web.Error(status, message)` | `status`, 400 to 599, showing `message` (E-WEB-004 outside that range) |
| `web.Redirect(path)` | 303 to a path on this site (E-WEB-002 for any other target) |

They return an `error`; `*web.HTTPError` holds the `Status` and `Message` of `web.Error`. Any
other error answers 500 and is logged. See [routing](routing.md).

### Safe markup

`web.SafeHTML` is markup that `{{$ }}` writes as it is:

| Function | Makes |
|---|---|
| `web.HTMLConst("<strong>")` | markup from a string constant; a variable does not compile |
| `web.EscapeHTML(text)` | markup that shows exactly `text` |
| `web.JoinHTML(a, b, …)` | the markups one after the other |

`h.String()` returns the markup. See [expressions](expressions.md).

### For generated code

`web.New`, `web.Options`, `web.Route`, `web.RouteState`, `web.Frame`, `web.Access`,
`web.Reactive`, `web.Caller`, `web.DecodeArgs`, `web.CheckNoGuard`, `web.LogError`,
`web.CSRFField`, `web.FormField` and `r.CSRFToken()` are used by the code `aicoded generate`
writes. Apps do not call them: `aicoded check` refuses, in hand-written code, a call of `web.New`,
`web.DecodeArgs`, `web.CheckNoGuard`, `web.LogError`, `r.CSRFToken()` and every other function
whose go doc says "Generated code only." (E-LINT-005).

## Package web/form

```go
import "aicoded.dev/framework/web/form"
```

| Type | A field that | Value |
|---|---|---|
| `form.Input[T]` | takes one value | `GetValue() T`, `SetValue(v)`, `GetFormValue()` |
| `form.InputMultiple[T]` | takes several, as checkboxes sharing a name | `GetValue() map[T]struct{}`, `SetValue(m)` |
| `form.Select[T]` | chooses one option | `GetValue() T`, `SetValue(v)`, `SetOptions`, `GetOptions` |
| `form.SelectMultiple[T]` | chooses several | `GetValue() map[T]struct{}`, `SetValue(m)`, `SetOptions`, `GetOptions` |
| `form.Textarea` | holds text | `GetValue() string`, `SetValue(s)` |
| `form.File` | uploads one file | `GetValue() *form.FileHeader` |
| `form.FileMultiple` | uploads several | `GetValue() []*form.FileHeader` |

- `T` is `string`, `bool`, an integer type or a float type.
- Every field has `SetError(msg)`, `HasError()`, `GetError()` and `IsNotNull()`.
- Options are `form.SelectOption[T]{Value, Label, Disabled}` and
  `form.SelectOptionGroup[T]{Label, Disabled, Options}`, both of type
  `form.SelectOptionElement[T]`.
- `form.FileHeader` embeds `*multipart.FileHeader`: `Filename`, `Size`, `Header` and `Open()`.
- `form.BaseFormValues`, which every form's values embed, has `HasError()`, `GetError()`,
  `SetError(msg)` and `IsValidated()`.
- `form.MessageRequiredField` is "This field is required" and `form.MessageInvalidOption`
  "Choose one of the options".

See [forms](forms.md).

## Package web/reactive

```go
import "aicoded.dev/framework/web/reactive"
```

| Type | Use |
|---|---|
| `reactive.NewBroadcast[V]()` | a hub that sends every value to every subscriber |
| `reactive.NewTopic[K, V]()` | a hub that sends a value to the subscribers of one key |

- `b.Publish(v)` and `t.Publish(key, v)` never block.
- `b.Subscribe()` and `t.Subscribe(key)` return a subscription: read `sub.Updates()` in a
  `select` with `ctx.Done()`, and `defer sub.Close()`.
- `b.TotalSubs()`, `t.TotalSubs()` and `t.Len()` count the subscriptions and the keys.

The connection types and frames of the package are used by the generated code and the framework,
and `reactive.Accept`, `reactive.Decode`, `reactive.TextBinding` and `reactive.HTMLBinding` are
for generated code only (E-LINT-005). See [live values](live-values.md).

## Package auth

`auth.Viewer(ctx)` returns the viewer as an `auth.Identity`, with `Subject`, `Name`, `Roles`
and `Groups`, and the methods `HasRole`, `InGroup` and `Authenticated`. See [access](access.md).

## See also

- [The request pipeline](request-pipeline.md): when each hook runs.
- `go doc aicoded.dev/framework/web` and the other packages, for every exported name.
