# Routing: folders, layouts, gates and parameters

The addresses of an app are the folders under `pages/`: there is no route table. A folder can
capture part of the URL, a page with pages below it is their layout or their gate, and a hook
ends a request with an error page or a redirect by returning an error.

## Folders are pages

A folder that holds an `index.html` is a page at its path:

```text
pages/index.html                         /
pages/home/index.html                    /home
pages/users/index.html                   /users
pages/users/add/index.html               /users/add
pages/users/s_login/index.html           /users/<login>
pages/users/s_login/info/index.html      /users/<login>/info
```

- A trailing slash is ignored: `/users/` is `/users`. A path with an empty part, such as
  `/users//add`, and a path that matches no page answer 404.
- Pages take `GET`, `HEAD` and `POST`. Any other method answers 405.

## Parameters

A folder named `s_<name>` takes any one part of the URL as the string parameter `<name>`, and a
folder named `n_<name>` takes only a whole number of digits that fits an `int64`; a part that is
not one answers 404.

```go
login := r.URLParam("login")  // from s_login
id := r.URLParamInt("id")     // from n_id, as an int64
```

- A fixed folder wins over a parameter: with `users/add/` and `users/s_login/`, `/users/add` is
  the add page. A user with the login `add` could never be opened, so refuse such names.
- A folder holds at most one parameter folder (E-GEN-041). A page that needs another one goes
  under a fixed folder, such as `notes/n_id` and `notes/by-slug/s_slug`.
- Every hook of every page on the path sees all the parameters of the URL: the layout
  `pages/users/` reads `login` to mark the open user in its list.
- `URLParam` returns `""` and `URLParamInt` returns 0 for a name the path does not have.

## Layouts

A layout has `<ssr:content/>` where the page below it is shown:

```html
<!-- pages/index.html -->
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><ssr:access role="*"/><title>People</title><ssr:assets/></head>
<body>
<nav>…</nav>
<main><ssr:content default="home"/></main>
</body>
</html>
```

- Each layout and the page have their own `RouteData` and data provider; they never share
  variables. Layouts nest: `/users/alice/info` is the info page inside the user's card, inside the
  users list, inside the root layout.
- Opening a layout itself redirects (302) to its default page. `default` names a page below the
  layout by its folders, relative to the layout, such as `default="info"` in
  `pages/users/s_login/` (E-GEN-049). It cannot name a parameter folder, whose value is not known
  when the pages are generated.
- To choose the page when the request comes, leave `default` out and return the page from the
  data provider's `DefaultRoute`. Its result is joined to the layout's path, and `""` answers
  404:

<!-- code: examples/people/pages/users/dataprovider.go DP.DefaultRoute -->
```go
// DefaultRoute opens the first user.
func (p *DP) DefaultRoute(ctx context.Context, _ *web.Request) (string, error) {
	users, err := p.d.Users(ctx)
	if err != nil || len(users) == 0 {
		return "", err
	}
	return users[0].Login, nil
}
```

- In a layout without `default`, the stub `DefaultRoute` that `aicoded generate` writes returns
  `""`, so opening the layout answers 404 until you write it.
- A POST to a layout answers 405: forms belong to the pages shown inside it.

## Gates

A page with pages below it and no `<ssr:content/>` is a gate. Its access rule and `Guard` apply
to every page below it, but its forms, `Data`, live values, calls, markup, scripts and styles
stay on its own page, which opening it shows. A list at `/notes`, with each note at
`/notes/42`, is a gate with a page below it:

```text
pages/notes/index.html        /notes       the list; its Guard covers every note
pages/notes/n_id/index.html   /notes/42    one note, which writes its own document
```

A page below a gate is not shown inside it, so it writes its whole document, `<!doctype html>`,
`<html>` and a `<head>` with `<ssr:assets/>`, unless a layout above the gate does. The root page
follows the same rule: when it is a gate, every page below it writes its whole document.

## Errors

A hook ends the request by returning an error:

| Return | Answer |
|---|---|
| `web.NotFound()` | 404 "This page does not exist." |
| `web.Forbidden()` | 403 "You do not have access to this page." |
| `web.Error(status, message)` | `status`, from 400 to 599, with `message` shown to the viewer |
| `web.Redirect(path)` | 303 to `path` |
| any other error | 500 "Something went wrong.", and the error is logged |

- The error page is the framework's own: a short HTML page with the status and the message,
  escaped. There is no custom error page.
- Never put internal details in the message of `web.Error`. A status outside 400 to 599 is
  logged as E-WEB-004 and answers 500.
- A request without a viewer, which the runner never sends, answers 401 "Sign in through the
  company login."

## Redirects

`web.Redirect("/users/" + login)` answers 303, so a form that redirects after it succeeds is not
sent again when the viewer reloads. The target must be a path on this site that starts with a
single `/` (E-WEB-002): a full URL or a path starting with `//` is refused, logged and answered
with 500, so a page can never send viewers to another site.

## Serving the pages

`main` hands the generated handler to `app.Main`:

```go
app.Main(app.Options{Handler: pages.NewHandler(d)})
```

The pages handler answers only requests that come through the app's runner (E-WEB-001). Only
the generated pages handler checks access rules, `Guard`s and form tokens and sets the security
headers, so `aicoded check` refuses any other `Handler`, such as a raw mux or a wrapped handler
(E-LINT-004). A page
that needs data from the browser without a reload uses a page call rather than a separate API;
see [page calls](page-calls.md). An app with no pages passes only `RPC`; see
[calls between apps](calls-between-apps.md).

## See also

- [Access](access.md): rules and Guards on the path.
- [The request pipeline](request-pipeline.md): what runs, in which order.
- [people/pages/users](../../examples/people/pages/users): a layout with a parameter folder and
  `DefaultRoute`.
- [Add a page](../tasks/add-page.md): a folder, a template, an access rule and `Data`.
