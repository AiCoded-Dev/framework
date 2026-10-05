# The request pipeline: what runs where

A page talks to its server in four ways: the page loads, a form posts, the live connection opens
and a script makes a page call. Each passes the runner first, then the same checks in the same
order. This guide shows what runs for each, and what runs in the browser.

## Before the app

```text
browser ──► company login ──► runner ──► the app's pages
            (aicoded dev's personas on your computer)
```

- The runner forwards each request with a viewer token it signs for this app, valid for at most
  60 seconds. The app answers a request without a valid token with 401 and never runs a hook for
  it. A token sent by the browser never reaches the app as its own.
- Each request is one span, `HTTP <method>` with the attribute `http.path`, and a panic in a
  hook answers 500 and is logged while the app keeps serving.
- Every answer of the pages carries the security headers: the Content-Security-Policy,
  `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`,
  `Cross-Origin-Opener-Policy: same-origin` and `Cross-Origin-Resource-Policy: same-origin`.
  Pages, redirects and error pages also carry `Cache-Control: no-store`.

## Loading a page

```text
GET /users/alice/info
  match the path: /, /users, /users/s_login, /users/s_login/info       404 if none
  access rules of every page on the path, root first                    403
  Guard of every page on the path that has one, root first              its error
  a layout opened itself: DefaultRoute, then redirect                   302, or 404
  Init<Form> of the forms of the layouts and the page
  Data of each layout and of the page, root first                       its error
  render into a buffer, root layout first, each page inside the one above
  200 text/html
```

A gate on the path runs its access rule and `Guard` here, but not its forms or `Data`: those
belong to its own page. Each page has its own `RouteData`; a layout never sees the page's
values, or the page the layout's.

## Posting a form

```text
POST /contact
  match the path                                                         404
  from another site?                                                     403
  access rules, then Guards                                              403 or their error
  read the body: at most 32 MiB                                          413, 400
  check the form token                                                   403
  Init<Form> of every form of the layouts and the page
  read the posted form's fields; required, types and options
  Process<Form> of the posted form, only when every field is valid       its error or redirect
  Data of each layout and of the page
  render: the page again, with the values and errors                     200
```

A `POST` to a layout itself answers 405. See [forms](forms.md).

## The live connection

A page with live values or page calls opens one WebSocket at its address plus `/__ws`, with the
page's query:

```text
GET /home/__ws   (Upgrade: websocket)
  match the path                                                         404
  from another site, or from an opaque origin?                           403
  access rules, then Guards                                              403 or their error
  Data of each layout and of the page                                    its error
  more than 8 open for this viewer?                                      429
  upgrade; send every live value
  Subscribe of each page with live values, each on its own goroutine
  until the page closes, a hook fails, or 10 minutes pass
```

Over it:

| From | Frame | When |
|---|---|---|
| server | `init` | the current value of every live part, once per connection |
| server | `patch` | a live part changed |
| page | `write` | an input with `ssr:bind` or `ssr.set` wrote a value |
| server | `ack` or `err` | `Validate<Name>` accepted or refused it |
| page | `call` | `ssr.call` ran a page call |
| server | `result` or `fail` | the call's result or its error |

The frames are the framework's own and may change with it; use the `ssr` object, never the
frames. See [live values](live-values.md).

## A page call

```text
call "star" over the live connection
  more than 4 calls of this page at once?                                429
  access rules, then Guards, of every page on the path                   403 or their error
  decode the arguments into the call's in type                           400
  Call<Name>, at most 30 seconds                                          its error, 504
  send the result as JSON
```

`Data` does not run for a call. See [page calls](page-calls.md).

## Assets

Scripts, styles and images are served at `/_aicoded/assets/` to every viewer the runner lets in,
with no access rule of a page: they are the same for everyone and hold no data. See
[assets](assets.md).

## What runs where

| | On the server | In the browser |
|---|---|---|
| rendering HTML | always | never; a live update is text or markup the server rendered |
| routing | every request | none; links load pages |
| access | access rules and `Guard`s, for every request, connection and call | nothing to trust |
| checking input | `required`, types, options, `Process`, `Validate`, `Call` | optional, for the viewer's comfort |
| live values | in the connection, while the page is open | the last text shown |
| business logic | the data providers and `deps` | none needed |

## See also

- [Access](access.md): the checks in detail.
- [Routing](routing.md): errors and redirects.
- [Overview](overview.md): how the parts of an app fit together.
