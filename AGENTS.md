# AGENTS.md

Rules for AI assistants that build apps with the aicoded framework,
`aicoded.dev/framework`. Follow them in every change. They are for building apps, not
for changing the framework itself.

## What an app is

- An app is a Go module with a permission list, `aicoded.yaml`, and a `main.go` that calls
  `app.Main`. The permission list names the app and everything it reaches: its SQL database,
  file stores, mail, settings, secrets and the apps it calls. The runner holds the app to it.
- Pages are folders under `pages/`. Each holds an `index.html` template and a `dataprovider.go`
  whose hooks fill it: `pages/notes/n_id/` is the page `/notes/42`. `aicoded generate` turns
  the pages into Go code.
- App code imports the framework's packages `app`, `auth`, `config`, `filestore`, `mailer`,
  `rpc`, `secrets`, `sqldb`, `telemetry`, `web`, `web/form` and `web/reactive`: each is
  `aicoded.dev/framework/<name>`, so the types of form fields, such as
  `form.SelectOption`, come from `aicoded.dev/framework/web/form`.
- What every page shares, such as the database, is `type Deps` in `deps/`. Its `Start` method
  opens the building blocks, and `main` passes it to `app.Main` as `OnStart`.
- Functions that other apps call are in `rpc/`. The client of another app is in
  `services/<app>/`, made by `aicoded rpc add <app>`.
- Viewers sign in through the company login before a request reaches the app, so an app has no
  login code. `auth.Viewer(ctx)` tells who is viewing.

## Always

- Put `<ssr:access role="..."/>` on the path of every page. Roles are lower-case names joined by
  commas, and a list admits anyone with one of its roles; `role="*"` admits every viewer. Every
  rule on the path applies, so require two roles by putting them on templates at different
  levels of the path, such as `staff` on a layout and `hr` on the page below it.
- Check ownership in a `Guard`: write `guard="true"` in the page's `<ssr:access>`, compare the
  record's owner with `auth.Viewer(ctx).Subject`, and return `web.Forbidden()` when the viewer may
  not see the record, or `web.NotFound()` only when even its existence must stay hidden.
- Put an `//ssr:access caller=<apps> role=<roles>` line right above every function of `rpc/`.
  Apps are joined by commas and roles by `|`; `apps=true`, in place of or next to `role=`, lets
  the apps call with no viewer.
- Check ownership again in every function of `rpc/` that returns a viewer's records, and refuse
  anyone else with the code `PermissionDenied`.
- Store `auth.Viewer(ctx).Subject` as a record's owner, never the viewer's name.
- Pass every value in SQL with a `?` placeholder.
- Give every mail an `IdempotencyKey` that names what it is about, such as
  `invoice-42-reminder`, so that a retried request never mails twice. Send it after the data it
  reports is committed.
- Declare in `aicoded.yaml` every data source, mail address, setting, secret and third-party
  module the app uses.
- Open the database and file stores once, in `OnStart`. Use the context of the request or call
  being served for everything else.
- To end a request from a hook, return `web.NotFound()`, `web.Forbidden()`,
  `web.Error(status, message)` or `web.Redirect("/path")`; otherwise return nil, or the error.
  The viewer sees only these messages; any other error shows "Something went wrong." and is
  logged.
- Put page scripts in `index.ts` and styles in `index.css` next to the template, and pass data to
  scripts with `<ssr:json>`.
- Run `aicoded check` after every change and fix every problem it lists. Commit the generated
  files with the change.

## Never

- Never write a password, key, token or DSN into code, `aicoded.yaml`, tests or docs. Read
  settings with `config.String` and secrets with `secrets.Get`, and call `Reveal()` only where
  the value is used.
- Never import `os`, `os/exec`, `net`, `syscall`, `reflect`, `unsafe` or `log`, a cloud SDK or
  a database driver: the building blocks reach everything through the runner, and `log/slog`
  logs. App code imports only its own packages, the building blocks, the standard packages that
  `aicoded explain E-LINT-001` lists, and the modules that `modules:` in `aicoded.yaml` declares.
  This is a rule for app code: until a test kit exists, a test may open the MySQL server that
  `AICODED_TEST_MYSQL_DSN` names, as the examples' tests do.
- Never call `sql.Open`, `sql.OpenDB`, `rpc.Call`, `web.New`, `http.Get`, `http.Post`,
  `http.ListenAndServe` or `http.NewServeMux`: get the database from `sqldb.Open`, call another
  app through its generated client in `services/<app>/`, and call `app.Main` with an
  `app.Options` literal written in the call, whose `Handler` and `RPC` are only
  `pages.NewHandler(...)` and `rpc.Server()`, which `aicoded generate` writes.
- Never edit generated files: `*_gen.go`, `*_gen.ts`, `pages/assets_gen/`, `.aicoded/`, and the
  `access` and `services` sections of `aicoded.yaml`. Change the template, `rpc/` or the rest of
  the permission list, and run `aicoded generate`.
- Never put a form value, a name, an email address, a phone number or any other personal data in
  a log line, a print or a span: `aicoded check` refuses every value from outside the app that
  reaches one, such as a form value, the viewer's identity, a row of the database or a secret
  (E-LINT-008). Return errors instead of logging them: outside `aicoded dev`, the framework
  records an error without its text. Log ids, counts and codes.
- Never build SQL from values: write it as a constant, pass values with `?`, and pass a list
  for `IN` as one JSON value that `JSON_TABLE` reads. When the viewer's role decides which rows
  to read, write one constant statement for each case and choose one in Go (`tasks/list-by-role`).
- Never change data in a form's `Init<Form>` hook: it runs every time the page shows or
  receives the form, on a GET too, so opening the page would change the data. Change data in
  `Process<Form>`, which runs only when the form is posted with every field valid.
- Never write inline `<script>` code, `<style>` elements, or `on*` or `style` attributes.
- Never write markup with `{{$ }}` from anything but a `web.SafeHTML` made with
  `web.HTMLConst`, `web.EscapeHTML` or `web.JoinHTML`.
- Never add a `replace` line to `go.mod`, a `go.work` file or a `vendor` folder: `aicoded check`
  refuses them (E-LINT-011). Only the framework's `replace` that `aicoded init` writes from a
  framework checkout passes there, and the delivery pipeline will refuse it. Require released
  versions.
- Never add `//nolint` or `#nosec`, skip a test or weaken a check to make it pass. Stop and
  explain the problem to the developer.

## The loop

1. `aicoded init <name>` creates an app in a new folder, with this file in it.
2. `aicoded dev`, in the app's folder or a folder above several apps, generates, builds and runs
   every app on a local runner, and rebuilds an app when its files change. Each app is at
   `http://<app>.localhost:8080/`; switch test personas at `/_aicoded/persona`.
3. `aicoded dev` prints a login link to its dev UI at `http://localhost:8080`. The dev UI shows
   each app's state and problems, its logs, traces and caught mail: mail is caught, never sent.
4. `aicoded check` generates, builds, vets, lints and tests every app, and lists each problem with
   its code, `file:line` and fix. Lint applies the rules for app code that `guides/rules` lists
   (E-LINT codes). `aicoded check --frozen` writes nothing and reports every generated file that
   is out of date: run it before you commit.
5. `aicoded explain <topic>` prints a page of the docs: a guide such as `guides/forms`, a task
   recipe, the changelog or the page of an error code.

Local values live outside the repository, in the developer's `dev.yaml`: the values of settings
and secrets, `mysql:` for an app with a database, and the test personas. Its format is on the
page `aicoded explain E-DEV-002` prints. When an app lacks a value (E-DEV-003), ask the developer
to add it, and never copy a value from `dev.yaml` into the app.

With MCP, the same loop is a set of tools that take app names, never paths. Add the server with
`claude mcp add aicoded -- aicoded mcp` in the workspace folder, or as the dev UI's MCP setup
page shows. The tools are:

- `app_create` creates an app; `check` is `aicoded check`;
- `preview` builds and starts an app and returns its address, or the problems that stop it;
- `what_broke` lists the failed apps, error log lines and failed spans;
- `describe` summarises the apps; `howto` reads the docs;
- `logs`, `traces`, `trace`, `mail_list` and `mail_get` show what `aicoded dev` saw, and
  `mail_receive` delivers a test mail to an app's inbox.

## Where to read

The docs are built into `aicoded`. `aicoded explain <topic>` prints a page, and the MCP tool
`howto` returns the same page; without a topic, both list every page.

- `guides/overview` is the place to start: how an app works, with a link to every guide.
- `guides/rules` lists every rule that lint applies to app code: what not to do, what to do
  instead, and its error code.
- The task recipes take their code from the framework's example apps: `tasks/new-app`,
  `tasks/add-page`, `tasks/add-form`, `tasks/add-live-value`, `tasks/list-from-database`,
  `tasks/list-by-role`, `tasks/ownership-check`, `tasks/email-after-a-write`,
  `tasks/upload-a-file` and `tasks/call-another-app`.
- `changelog` lists what changed in the framework, and an error code such as `E-DEV-003`
  explains that error.
- `aicoded describe` summarises the apps: pages and their access rules, functions served and
  called, database, file stores, mail, settings, secret names, modules, the tables their SQL
  writes, their surface, and generated files that are out of date.
- `go doc aicoded.dev/framework/<package>` prints a package's overview and rules. Its
  examples are in its `example_test.go`, in the folder that
  `go list -m -f '{{.Dir}}' aicoded.dev/framework` prints.
