# The permission list: aicoded.yaml

`aicoded.yaml` declares everything the app reaches: its data, the address it mails from and the
domains it mails to, its settings and secrets, who may open each page, and the functions it calls
and serves. The runner holds the app to it, and the app itself never holds a password or a key.

## An example

```yaml
# aicoded.yaml
app: items
data:
  - source: sqldb              # the app's own SQL database
    classes: [internal]
  - source: filestore:docs     # the app's file store named docs
    classes: [internal]
email:
  from: items@acme.example     # the one address the app sends from
  to_domains: [acme.example]   # the only domains it may send to
settings: [report_day]         # read with config.String
secrets: [partner_api_key]     # read with secrets.Get
modules: [golang.org/x/text]   # third-party modules the code uses
```

You or your AI assistant write these sections. `aicoded generate` adds `access` and `services`
below them and keeps them current.

## The sections you write

- `app` is the app's name: 2 to 63 lowercase letters, digits and dashes, starting with a letter
  and not ending with a dash (E-MAN-004). It gives the app its address in `aicoded dev`, and other
  apps, `dev.yaml` and the `aicoded mcp` tools name the app by it.
- `data` lists the sources of data the app reaches, each with the `classes` of data it holds,
  such as `internal`: at least one, each a lower-case name (E-MAN-008).
  - `sqldb` is the app's own SQL database; see [the SQL database](sql-database.md).
  - `filestore:<name>` is one of its file stores; see [file stores](file-stores.md).
  - `connector:<name>` names a company system the app reads with each viewer's own access. No
    building block reads one yet.

  Names are lower-case letters, digits and dashes, and each source is listed once (E-MAN-007).
- `email` has `from`, one plain lower-case address, and `to_domains`, the lower-case domains the
  app may send to, with no wildcards and no non-ASCII names (E-MAN-009). See [mail](mail.md).
- `settings` and `secrets` name the values the app reads: lowercase letters, digits and `_`,
  starting with a letter, each once (E-MAN-005). Their values are not in the file: on your
  computer they come from `dev.yaml`. See [settings and secrets](settings-and-secrets.md).
- `modules` lists the third-party Go modules the app's code uses, by module path, such as
  `golang.org/x/text`: each once, without a version, which comes from `go.mod` (E-MAN-013).
  `aicoded check` refuses a third-party module that the app, or a module it lists, imports
  without an entry (E-LINT-002), and checks the code of every module listed: it may not run
  programs, open network connections, make system calls, use `unsafe`, `//go:linkname` or
  assembly, call methods through `reflect`, import the framework, open a database, run SQL,
  replace the default logger or change a variable of the standard library or the framework
  (E-LINT-003). The framework and the modules only it imports need no entry, nor does a module
  only tests use.
- `owner` names the group that owns the app, such as `group:hr-leads`. `aicoded` keeps it as
  written, and nothing checks it yet.

`aicoded` leaves any other section as it is.

## The sections aicoded generate writes

`aicoded generate`, which `aicoded dev` and `aicoded check` run first, writes the `access`
section from the pages' `<ssr:access>`: one entry per page a viewer can open, keyed by its URL
pattern. A layout with pages below it has no entry of its own.

```yaml
# access is written by aicoded generate from <ssr:access>; do not edit it
access:
  "/users/add":
    require: ["staff", "hr"]
  "/users/{login}/contacts":
    require: ["staff"]
    guard: true
  "/notes/{id}":
    require: ["*"]
    calls: ["star"]
```

- `require` lists the access rules on the page's path that do not admit every viewer, root
  first, with the roles of one rule joined by `|`; `["*"]` means every viewer the company login
  lets in.
- `guard: true` says that a `Guard` on the page's path runs for it.
- `calls` names the page calls of the page and of the layouts it is shown in.

It writes the `services` section from `rpc/` and the clients in `services/`: the functions the
app calls and the ones it serves; see [calls between apps](calls-between-apps.md). An app with
no pages gets `access: {}`.

Never edit either section: `aicoded generate` rewrites them from the code and leaves the rest of
the file as it was. It refuses a file whose sections it cannot replace without touching the rest:
keep `aicoded.yaml` one YAML document in block style, with every top-level key at the start of a
line and one `access` and one `services` section (E-MAN-003).

## How the app is held to it

The runner gives the app only what the permission list declares, and the framework names the
section to add when the app reaches for more:

| The app | Without | Fails with |
|---|---|---|
| opens its database with `sqldb.Open` | `- source: sqldb` | E-MAN-010 |
| opens a store with `filestore.Open(ctx, "photos")` | `- source: filestore:photos` | E-MAN-011 |
| sends or reads mail | an `email` section | E-MAN-012 |
| mails a domain | the domain in `to_domains` | E-MAIL-003 |
| reads a setting with `config.String` | its name in `settings` | E-MAN-001 |
| reads a secret with `secrets.Get` | its name in `secrets` | E-MAN-002 |
| calls a function of another app | the call in `services.calls` | E-RPC-008 |

Every page needs an access rule on its path, so every page is in `access`. Together the sections
say, in a form security can read, who uses the app, what data it reaches and where its mail goes,
without reading its code.

## See also

- [Access](access.md): the rules the `access` section records.
- [Calls between apps](calls-between-apps.md): the `services` section.
- [people/aicoded.yaml](../../examples/people/aicoded.yaml),
  [contacts/aicoded.yaml](../../examples/contacts/aicoded.yaml) and
  [room-maintenance/aicoded.yaml](../../examples/room-maintenance/aicoded.yaml): the permission
  lists of the examples.
