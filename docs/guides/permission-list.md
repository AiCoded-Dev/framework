# The permission list: aicoded.yaml

`aicoded.yaml` declares who owns and uses the app and everything it reaches: its data, the outside
services it calls, the address it mails from and the domains it mails to, its scheduled jobs, its
settings and secrets, who may open each page, and the functions it calls and serves. Security reads
it instead of the code, and the app itself never holds a password or a key.

## An example

```yaml
# aicoded.yaml
app: room-maintenance
class: internal-tool               # the app type, given by security
owner: group:hotel-ops-leads       # the group that owns the app
audience:
  internal: [group:hotel-staff]    # people of the company who use it
  external: []                     # outside people, such as [idp:partners, magic-link]
data:
  - source: sqldb                  # the app's own SQL database
    classes: [internal]
  - source: filestore:photos       # the app's file store named photos
    classes: [internal]
egress: [api.partner.example]      # outside services it calls, over HTTPS
email:
  from: rooms@acme.example         # the one address the app sends from
  to_domains: [acme.example]       # the only domains it may send to
schedule:
  - {cron: "0 6 * * *", job: daily-summary}   # 06:00 UTC every day
settings: [report_day]             # read with config.String
secrets: [partner_api_key]         # read with secrets.Get
modules: [golang.org/x/text]       # third-party modules the code uses
size: S                            # the size limit, given by the budget owner
resources: small                   # the runtime limit, given by the budget owner
ttl: 180d                          # days until the app is reviewed
# access is written by aicoded generate from <ssr:access>; do not edit it
access:
  "/rooms":
    require: ["hotel-staff"]
# services is written by aicoded generate from rpc/ and the clients in services/; do not edit it
services:
  calls:
    "inventory": ["ReserveParts"]
```

You or your AI assistant write every section but `access` and `services`, which `aicoded
generate` adds and keeps current. Only `app` is required: a section left out, or left empty, is
not stated.

## The sections you write

- `app` is the app's name: 2 to 63 lowercase letters, digits and dashes, starting with a letter
  and not ending with a dash (E-MAN-004). It gives the app its address in `aicoded dev`, and other
  apps, `dev.yaml` and the `aicoded mcp` tools name the app by it.
- `class` names the app type the app belongs to, which security defines: lower-case letters,
  digits and dashes, starting with a letter, such as `internal-tool` (E-MAN-015).
- `owner` is the group of the company that owns the app, `group:<name>`, such as
  `group:hr-leads`. A group's name is lower-case letters, digits, dots, dashes and underscores,
  starting and ending with a letter or digit (E-MAN-016).
- `audience` says who may use the app, each entry once (E-MAN-017):
  - `internal` lists people of the company: `group:<name>` for a group, or `everyone`;
  - `external` lists outside people: `idp:<name>` for the users of a partner's identity
    provider, with a lower-case name, or `magic-link` for known outside users who sign in with
    a link sent to their email.
- `data` lists the sources of data the app reaches, each with the `classes` of data it holds,
  such as `internal`: at least one, each a lower-case name (E-MAN-008).
  - `sqldb` is the app's own SQL database; see [the SQL database](sql-database.md).
  - `filestore:<name>` is one of its file stores; see [file stores](file-stores.md).
  - `connector:<name>` names a company system the app reads with each viewer's own access. No
    building block reads one yet.

  Names are lower-case letters, digits and dashes, and each source is listed once (E-MAN-007).
- `egress` lists the outside services the app calls, by host name, over HTTPS on port 443: a
  lower-case name such as `api.partner.example`, with no scheme, port, path, wildcard or IP
  address, and none that ends in `localhost`, `local`, `internal` or `arpa`, each once
  (E-MAN-018). No building block calls one yet.
- `email` has `from`, one plain lower-case address, and `to_domains`, the lower-case domains the
  app may send to, with no wildcards and no non-ASCII names (E-MAN-009). See [mail](mail.md).
- `schedule` lists the app's scheduled jobs, each as `{cron: "<expression>", job: <name>}`
  (E-MAN-019). `job` is a lower-case name of letters, digits and dashes, each job once. `cron`
  is five fields in UTC separated by spaces: minute (0-59), hour (0-23), day of the month
  (1-31), month (1-12) and weekday (0-6, with 0 for Sunday). A field is `*`, a number, a range
  `a-b`, a step `*/n` or `a-b/n`, or a comma list of them; names such as `MON`, `?`, `L`, `W`,
  `#` and `@daily` are not accepted. No building block runs a job yet.
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
- `size` is the app's size limit, `S`, `M` or `L` (E-MAN-020), and `resources` its runtime
  limit, `small`, `medium` or `large` (E-MAN-021).
- `ttl` is how long the app may live before it is reviewed: a number of days from 1 to 3650
  followed by `d`, such as `180d` (E-MAN-022).

An AI assistant writes `owner` and `audience` when the person says who owns and uses the app. It
leaves `class`, `size`, `resources` and `ttl` out unless the person or their administrator gives
them: security and the budget owner set them.

An unknown key is an error (E-MAN-014): at the top level, in `audience`, in each entry of `data`
and `schedule`, in `email`, and in `access` and `services`, so that a misspelt key never goes
unseen. The file may not use an anchor, an alias or a merge key, `&name`, `*name` or `<<`: write
every value out in full (E-MAN-003). `aicoded check` reports every problem of the file at once, in
line order.

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

## What the platform does with it

- `data`, `email`, `settings`, `secrets`, `modules`, `access` and `services` are enforced as
  above, by the runner, `aicoded check` and the delivery pipeline.
- In this version the platform records the other fields, `class`, `owner`, `audience`, `egress`,
  `schedule`, `size`, `resources` and `ttl`, and shows each change to the permission list in the
  app's change record.
- Holding the app to them comes later, with app types, limits and production.

## See also

- [Access](access.md): the rules the `access` section records.
- [Calls between apps](calls-between-apps.md): the `services` section.
- [Publishing](publishing.md): the change record of each publish.
- [people/aicoded.yaml](../../examples/people/aicoded.yaml),
  [contacts/aicoded.yaml](../../examples/contacts/aicoded.yaml) and
  [room-maintenance/aicoded.yaml](../../examples/room-maintenance/aicoded.yaml): the permission
  lists of the examples.
