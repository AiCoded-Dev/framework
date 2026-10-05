# Overview: how an app works

An app is a Go module with a permission list, `aicoded.yaml`, and a `main` that calls `app.Main`.
Its pages are HTML templates that `aicoded generate` turns into type-checked Go code. It runs
under a runner, which signs in its viewers and gives it its building blocks.

## The parts of an app

```text
notes/
  go.mod
  aicoded.yaml     the permission list: what the app may reach, and who may open each page
  main.go          app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})
  deps/deps.go     optional: type Deps, what every page shares, such as the database
  pages/           the pages, one folder each
  rpc/             optional: the functions other apps may call
```

- The app holds no database password and no DSN. It reaches its database, files, mail and other
  apps only through its runner, which gives it only what its permission list declares. On your
  computer, `aicoded dev` is that runner, with test personas in place of the company login.
- The building blocks are packages of the framework: `auth` (who is viewing), `config`
  (settings), `secrets`, `sqldb` (the app's own MySQL database), `filestore`, `mailer`,
  `telemetry` and `rpc` (calls between apps).
- You or your AI assistant write `aicoded.yaml`, except its `access` and `services` sections,
  which `aicoded generate` writes.

## The pages folder

Each folder under `pages/` that holds an `index.html` is a page at that path. A page with pages
below it is their layout or their gate:

```text
pages/
  index.html                  /                layout of every page: the nav and <ssr:content default="notes"/>
  notes/index.html            /notes           gate: the viewer's notes, each linking to its page
  notes/n_id/index.html       /notes/42        one note
  admin/index.html            /admin           layout of the admin pages: <ssr:content default="settings"/>
  admin/settings/index.html   /admin/settings
```

- A **layout** holds `<ssr:content/>`, where the page below it is shown. Opening the layout itself
  sends the viewer to its default page.
- A **gate** has no `<ssr:content/>`. Its access rule and `Guard` cover every page below it, but
  its markup, forms and data stay on its own page.
- A folder named `n_id` captures a whole number from the URL, and `s_name` captures any one part.
  The data provider reads them with `r.URLParamInt("id")` and `r.URLParam("name")`.
- Every page needs an access rule on its path: `<ssr:access role="…"/>` in its own template or a
  parent's (E-GEN-030). The viewer needs one of the roles of every rule on the path, and
  `role="*"` admits every viewer the company login lets in.
- A template declares every value it shows with a Go type, such as
  `<ssr:var name="notes" type="[]Note"/>`. The page's data provider, `DP` in its
  `dataprovider.go`, fills it in `Data`, and the Go compiler checks the names and the types.

## Generated files

`aicoded dev` and `aicoded check` run `aicoded generate` first. Never edit what it writes: change
the templates, the permission list or the `rpc/` package, and generate again.

| File | Written for | What it holds |
|---|---|---|
| `route_gen.go` in a page's folder | every page | the page's rendering, the `RouteData` struct of its values, its form types, its live state, and the `RouteDataProvider` interface of its hooks |
| `reactive_gen.ts` in a page's folder | a page with live values or page calls | the typed `ssr` object that the page's `index.ts` imports |
| `pages/handler_gen.go` | an app with pages | `NewHandler`, which serves every page |
| `pages/assets_gen.go`, `pages/assets_gen/` | an app with pages | the pages' scripts, styles and images, built and embedded in the app |
| `rpc/server_gen.go`, `.aicoded/rpc.json` | an app that serves calls | the server of its functions, and the snapshot of their types |
| `services/<app>/client_gen.go` | an app that calls `<app>` | the typed client of `<app>`'s functions |
| the `access` section of `aicoded.yaml` | every app | the access rules of every page |
| the `services` section of `aicoded.yaml` | an app that calls or serves | the calls the app makes and serves |

`aicoded generate` also writes a `dataprovider.go` in a page folder that has none. That file is
yours to edit. It implements every hook the template needs, and the hooks that guard something
fail closed until you write them:

- a `Guard` refuses every viewer (403);
- a `DefaultRoute` names no page (404);
- a `Validate…` of a value the page writes refuses every value;
- a page call refuses every call (403).

The stub holds `var _ RouteDataProvider = &DP{}`, so when a template needs a new hook, such as
one for a new form, the Go compiler names the method that is missing.

## What happens on a request

The runner forwards each request with a short-lived viewer token that it signs, and the app
answers only requests that carry a valid one. Then the app's pages:

1. Match the path folder by folder. A path that matches no page answers 404.
2. Check the access rule of every page on the path, root first. A path with no rule is refused.
3. Run the `Guard` of every page on the path that declares one, root first.
4. For a POST, check the form token, and refuse a form without a valid one (403).
5. Run `Init<Form>` for the forms of the layouts and the page, root first. For a POST, then run
   `Process<Form>` for the posted form, only when every field is valid.
6. Run `Data` for each layout on the path and for the page, root first.
7. Render the page into a buffer, and send it only when it renders whole.

So a check in a layout or a gate always runs before anything a page below it does. A page with
live values or page calls also opens one live connection. It runs the same access rules and
`Guard`s first, and every page call runs them again.

## Safe by default

- `{{ expr }}` escapes each value for where it lands: text, an attribute or a URL. A value where
  escaping cannot make it safe, such as in a tag name, an unquoted attribute or an attribute that
  loads code, is refused when the page is generated.
- Pages have no inline scripts or styles. Code goes in `index.ts` and styles in `index.css` next
  to the template.
- Every form carries a token, and a POST from another site is refused.
- Every error has a code, a `file:line` where there is one, a one-line fix and a page.
  `aicoded explain <code>` prints the page.

## The `aicoded` command

| Command | What it does |
|---|---|
| `aicoded init <name>` | creates an app in a new folder |
| `aicoded dev [dir]` | runs every app in the folder on a local runner, and rebuilds an app when its files change |
| `aicoded generate [dir]` | writes the generated files of one app |
| `aicoded check [dir]` | generates, builds, vets, lints and tests every app, and lists every problem with its fix |
| `aicoded describe [dir]` | summarises what every app serves, calls and reaches, and its size |
| `aicoded explain [topic]` | prints a page of the docs, such as this guide or the page of an error code, or lists them |
| `aicoded rpc add <app>` | lets the app in this folder call the functions of `<app>` |
| `aicoded mcp [dir]` | gives your AI assistant these tools over MCP |
| `aicoded version` | prints the version of `aicoded` |

## Pages

- [Quick start](quickstart.md): create an app, run it as a test persona and change its page.
- [Project structure](project-structure.md): the files of an app and who writes each.
- [Template syntax](template-syntax.md): the `ssr:` tags and attributes, and what the generator
  refuses.
- [Expressions](expressions.md): the `{{ }}` language and how each value is escaped.
- [Routing](routing.md): folders, parameters, layouts, gates, errors and redirects.
- [Access](access.md): access rules, Guards, the viewer and the order of checks.
- [Forms](forms.md): fields, the `Init` and `Process` hooks, files and the form token.
- [Live values](live-values.md): values the server updates and the page writes.
- [Page calls](page-calls.md): functions a page's script calls on its server.
- [Assets](assets.md): scripts, styles and images, and how they are built and served.
- [TypeScript API](typescript-api.md): the `ssr` object a page's script uses.
- [The web API](web-api.md): what a data provider uses from `web`, `web/form` and
  `web/reactive`.
- [The request pipeline](request-pipeline.md): what runs, in which order, for each kind of
  request.

## Building blocks, calls and tools

- [The permission list](permission-list.md): what `aicoded.yaml` declares, and the sections
  `aicoded generate` writes.
- [The SQL database](sql-database.md): the app's own MySQL database, with no DSN.
- [File stores](file-stores.md): reading and writing the app's files.
- [Mail](mail.md): sending mail with an idempotency key, and reading the mailbox.
- [Settings and secrets](settings-and-secrets.md): configured values and values nobody may see.
- [Telemetry](telemetry.md): logs and trace spans, with no personal data.
- [Calls between apps](calls-between-apps.md): serving and calling functions of other apps.
- [aicoded dev](dev.md): running the apps on your computer, `dev.yaml`, personas and the dev UI.
- [Tools](tools.md): `init`, `generate`, `check`, `describe`, `explain` and `rpc add`.
- [Rules for app code](rules.md): what `aicoded check` refuses in app code, what to write
  instead, and the error code of each rule.
- [MCP](mcp.md): the tools your AI assistant uses.
- [Releasing](releasing.md): what a commit must hold before you publish it.

## Tasks

Recipes for common changes, with the code of the example apps:

- [Start a new app](../tasks/new-app.md): `aicoded init`, the permission list, `deps` and
  `aicoded dev`.
- [Add a page](../tasks/add-page.md): a folder, a template, an access rule and `Data`.
- [Add a form](../tasks/add-form.md): fields the server checks, `Init` and `Process`, and a
  redirect.
- [Add a live value](../tasks/add-live-value.md): values the server sends while the page is
  open, and one the page writes back.
- [List rows from the database](../tasks/list-from-database.md): a table, a query with
  placeholders and a list.
- [List own records, or all of them for a role](../tasks/list-by-role.md): one constant
  statement for each case, chosen by the viewer's role.
- [Check who owns a record](../tasks/ownership-check.md): a `Guard` on the page, and the same
  check in the app that holds the data.
- [Send mail after a write](../tasks/email-after-a-write.md): commit, then mail with an
  idempotency key.
- [Take a file upload](../tasks/upload-a-file.md): file fields, their checks and a file store.
- [Call another app](../tasks/call-another-app.md): a function in `rpc/`, `aicoded rpc add` and
  the call.
