# Tools: the aicoded commands

`aicoded` creates apps, generates their code, checks them, describes them, explains its errors
and docs, connects apps that call each other, signs you in to the platform and publishes apps
to it. Every problem it reports has a code, a `file:line` where there is one, a one-line fix and
a docs link.

## The commands

```text
aicoded init <name>
aicoded generate [dir]
aicoded check [--frozen] [--no-tests] [--json] [--app <name>] [dir]
aicoded describe [--json] [--app <name>] [dir]
aicoded explain [topic]
aicoded rpc add <app>
aicoded dev [--manual <app>] [dir]
aicoded mcp [dir]
aicoded login [--org <organisation>] [--device]
aicoded logout
aicoded whoami [--json]
aicoded publish [--app <name>] [-m <summary>] [--no-wait] [dir]
aicoded status [--json] <id>
aicoded version
```

`dir` is the current folder by default. `aicoded` exits with 0 when it succeeds, 1 when it
reports a problem and 2 when the command line is not valid. `aicoded dev` has its own guide,
[aicoded dev](dev.md), and `aicoded mcp` too, [MCP](mcp.md).

## aicoded init

`aicoded init <name>` creates an app in a new folder `<name>`: `go.mod`, a permission list with
`app: <name>`, `main.go`, one page and `AGENTS.md`, the rules for AI assistants that build the
app. It then runs `aicoded generate` and `go mod tidy`.

- The name is 2 to 63 lowercase letters, digits and dashes, starting with a letter and ending
  with a letter or digit, and no folder there may have it (E-CLI-002).
- `go.mod` requires the framework version this `aicoded` was built with. An `aicoded` installed
  with `make install` in a framework checkout writes a `replace` line to that checkout instead and
  says so. `aicoded check` accepts that line only from the `aicoded` installed from that very
  checkout, and refuses every other `replace`, any `go.work` file, a `vendor` folder, a module
  cache that `go mod verify` does not pass and every GOFLAGS entry outside the short list it
  allows (E-LINT-011); the delivery pipeline refuses a `replace` line too (E-GATE-001), so
  require a released version before you publish.
  An `aicoded` that knows neither refuses (E-CLI-003).
- When `go mod tidy` fails, often for want of a network, `aicoded init` removes the new folder
  (E-CHK-006).

## aicoded generate

`aicoded generate [dir]` writes the generated files of the app in `dir`: the code of its pages, a
`dataprovider.go` in a page folder that has none, the built assets, the server and snapshot of its
`rpc/` functions, the clients of the apps it calls, and the `access` and `services` sections of
its permission list. `aicoded dev`, `aicoded check` and `aicoded rpc add` run it themselves. It
needs `go.mod` next to `aicoded.yaml` (E-GEN-037).

## aicoded check

`aicoded check [--frozen] [--no-tests] [--json] [--app <name>] [dir]` checks every app under the
folder, or the one named.

- It first runs lint's precheck for every app, before any other go command: GOFLAGS and a
  `vendor` folder decide what the go command builds and runs, so an app whose GOFLAGS or
  `vendor` folder lint refuses (E-LINT-011) gets no other step.
- For each app it runs `aicoded generate` and lists the files it wrote, checks the snapshots the
  app holds, and runs `go build`, `go vet`, lint and `go test`; a step that fails skips the later
  steps of that app. Every problem has its code, `file:line`, fix and docs link (E-CHK-002 to
  E-CHK-006, and the E-LINT codes of lint).
- Lint applies the rules for app code to every file but the tests, and nothing in the code can
  turn a finding off. [Rules for app code](rules.md) lists each rule, what to do instead and its
  E-LINT code. A file counts as generated, with what generated files may import and call, only
  when `aicoded generate` wrote it. When the go command is a newer Go than the one `aicoded` was
  built with, lint cannot read the code, and every app gets E-CHK-007.
- Once lint has read an app, the report gives its surface, as `aicoded describe` does.
- With `--frozen` it writes nothing and reports every generated file that is out of date
  (E-CHK-001), as the security checks will.
- The tests run with `-race` when cgo works (`go env CGO_ENABLED` is `1` and its C compiler is on
  `PATH`, as the first app that passes the precheck sees them); otherwise they run without it and
  `check` says so, since the delivery pipeline will run them with `-race`.
- With `--no-tests` it skips the tests and does not probe `-race`; every other step runs. The
  report says so: `no_tests` is `true` in JSON, and the text ends with
  `tests did not run (--no-tests)`. The `check` tool of `aicoded mcp` always runs the tests.
- `--json` prints the same report as JSON.
- It exits with 1 on any problem.

```text
ok  people
  surface 5 (5 entry points, 2 effects)
aicoded check: ok
```

## aicoded describe

`aicoded describe [--json] [--app <name>] [dir]` summarises each app for your AI assistant:
its app type, owner and audience, pages with their access rules, functions served and called,
database, file stores, connectors, mail, outside services, scheduled jobs, settings, the names of
its secrets, the modules it declares, its size, resources and review period, the tables its SQL
writes, its surface, and generated files that are out of date. Each line shows only when the app
has the item. It writes nothing. For the room-maintenance example it prints:

```text
app room-maintenance (/home/me/apps/room-maintenance)
  class internal-tool
  owner group:hotel-ops-leads
  audience group:hotel-staff
  page /tickets  require staff
  page /tickets/report  require staff
  page /tickets/{id}  require staff  guard
  page /tickets/{id}/edit  require staff  guard
  page /tickets/{id}/status  require staff, hotel-ops  guard
  database
  size S
  resources small
  ttl 180d
  writes INSERT rooms, INSERT tickets, UPDATE tickets
  surface 5 (5 entry points, 3 effects)
```

An app with more in its permission list gets more lines, such as `connectors crm` after
`files`, `egress api.partner.example` and `schedule daily-summary  cron 0 6 * * *` after the
mail, and `external audience magic-link` after `audience`.

The surface is the app's size: the larger of its entry points and its effects. Its entry points
are its pages, its page calls, counted once for each page that offers them, the functions it
serves other apps and its scheduled jobs. Its effects are the tables its SQL writes, counted
once for each of `INSERT`, `UPDATE`, `DELETE` and `REPLACE`, its connectors, the outside
services it may call and the domains it may mail. The writes come from lint, which reads the
SQL; when the app does not compile, or lint's precheck refuses its GOFLAGS or `vendor` folder,
`describe` counts no writes and says so in a warning. Only `INSERT`, `UPDATE`, `DELETE` and
`REPLACE` count, and a write to several tables counts its first table; a `SELECT` and a
`CREATE`, `ALTER` or `DROP` of a table or index are not writes, and statements lint refuses
outright, such as `CALL`, `TRUNCATE`, `SET`, `PREPARE` and `EXECUTE` (E-LINT-007), never reach
the surface. Nothing compares the surface with `size` yet.

## aicoded explain

`aicoded explain <topic>` prints a page of the docs built into `aicoded`: a guide such as
`guides/overview`, a task recipe such as `tasks/add-form`, the changelog for AI assistants
(`changelog`) or the page of an error code such as `E-DEV-001`. `aicoded explain` with no topic
lists them all, with their titles. The pages are the same as in the repository's `docs/` folder,
and need no network. An unknown topic is E-CLI-001.

## aicoded rpc add

`aicoded rpc add <app>`, in the folder of an app, lets it call the functions of the app named
`<app>`: it copies that app's snapshot to `.aicoded/services/<app>.json` and generates the client
`services/<app>/client_gen.go`. See [calls between apps](calls-between-apps.md).

## aicoded login

`aicoded login [--org <organisation>] [--device]` signs you in to the platform, as a builder of
your organisation. The first login needs `--org`, the organisation's name on the platform, and
later ones remember it.

- It prints the address of the sign-in: open it in a browser on this computer. You sign in with
  your organisation's company login or, in an organisation that invites its builders, with the
  account you were invited with. The browser then comes back to `aicoded`, which listens for it
  on `127.0.0.1` only, and refuses an answer that does not belong to this sign-in. It waits 5
  minutes (E-CLI-008). `aicoded` does not open the browser itself.
- On a computer without a browser, such as a remote server, use `--device`: it prints an address
  and a code instead. Open the address in a browser on any device, enter the code there, and
  confirm. The code expires after 10 minutes (E-CLI-008).
- When the platform refuses you, the error quotes its reason (E-CLI-006). When you are not in
  your organisation's builder group, or not invited, ask your administrator to add you to it.
  When no organisation has the name you gave, check the name with your administrator.
- Ctrl-C stops the sign-in, and `aicoded login` prints `sign-in cancelled`.
- It prints `Signed in to <organisation> as <email>.` A new sign-in replaces and revokes the one
  before it.

```text
To sign in to acme, open this address in a browser on this computer, or run aicoded login --device on a computer without one:

    https://api.aicoded.cloud/oauth/authorize?client_id=aicoded&...

Signed in to acme as ana@acme.example.
```

The sign-in lives in `credentials.json` in your user configuration folder, next to `dev.yaml`,
with one entry per platform address. It works like a password: the file must have mode 600 and
its folder mode 700, and `aicoded` refuses to use them while others can read them (E-CLI-007).
Commands that use the sign-in at the same time take turns through `credentials.lock`, a file
next to it, so that only one of them refreshes the sign-in and the others use the result; a
command waits at most 30 seconds for its turn, and then fails with an error that names the file.
`AICODED_PLATFORM` names the platform, `https://api.aicoded.cloud` by default; it must be an
`https` address, with or without `:443`, or `http://127.0.0.1:<port>` for a platform on this
computer (E-CLI-005).

There is no MCP tool for signing in: it needs a person and a browser.

## aicoded logout

`aicoded logout` deletes your sign-in from this computer and asks the platform to revoke it. It
deletes it even when the platform cannot be reached, and then says that the sign-in stays valid
on the platform until it expires, within 30 days. It keeps the name of your organisation, so the
next `aicoded login` needs no `--org`.

## aicoded whoami

`aicoded whoami [--json]` prints the organisation, the email address and the scopes of your
sign-in, as the platform sees them. It refreshes the sign-in first when it is about to expire;
when the platform cannot refresh it right now, it fails with E-CLI-009 and keeps the sign-in.
Without a sign-in, or with one the platform no longer accepts, it fails with E-CLI-004.

```text
org: acme
email: ana@acme.example
scopes: app:create
```

## aicoded publish

`aicoded publish [--app <name>] [-m <summary>] [--no-wait] [dir]` sends the commit `HEAD` names,
of the app in `dir`, to the platform's delivery pipeline, which checks it and writes its change
record. [Publishing](publishing.md) tells the whole story.

- `dir` must hold `aicoded.yaml` and be the top level of a git repository with a commit
  (E-PUB-001), with nothing left uncommitted (E-PUB-002). `--app` refuses an app of another name
  (E-PUB-003). It needs git 2.31 or later (E-PUB-012).
- It runs `aicoded check --frozen --no-tests` as a released `aicoded` does, and prints its
  problems and sends nothing when it finds any (E-PUB-004).
- It sends a git bundle of the commit with its history since the app's last verified publish,
  or all of it on the first publish, at most 32 MiB (E-PUB-009), with the app's name and a
  summary: `-m`, or the commit's subject. The first publish of a name creates the app, owned by
  you.
- It prints `Published <app> at <commit> as <id>.`, then waits for the outcome, asking every 3
  seconds for up to 35 minutes, and prints the steps, the problems as `aicoded check` prints
  them, and the status. It exits with 0 when the publish passed and 1 otherwise. Ctrl-C stops the
  waiting, not the publish. `--no-wait` returns at once.
- When the platform refuses the sign-in for want of an app's scopes, or does not accept its
  token, as when this computer's clock is off, it refreshes the sign-in once and asks again. That
  is how a sign-in gets the scopes of the app its first publish created. A token the platform
  still does not accept is E-CLI-004. `aicoded status` does the same.

## aicoded status

`aicoded status [--json] <id>` prints a publish as the end of `aicoded publish` does, at once:
its steps, its problems and its status, `queued`, `running`, `passed`, `failed`, `refused` or
`error`. `--json` prints the publish as JSON. It exits with 1 when the publish failed, was
refused or ended in error. An id is `pub_` and 26 lowercase letters and digits; another shape
exits with 2. An id the platform does not know in your organisation is E-PUB-013, a publish of
another builder's app E-PUB-005, and one of an app of yours that was archived E-PUB-006.

## The control socket

One `aicoded dev` runs per workspace. It serves a control socket, `control.sock` with mode 600,
in the workspace's private state folder under `~/.local/state/aicoded`. `aicoded mcp` and a
second `aicoded dev` talk to the running one through it: the second one refuses (E-DEV-012) and
prints the login link of the running one's dev UI, and an `aicoded` of another version gets
E-DEV-013. A file there that is not a socket (E-DEV-016), or a path too long for a Unix socket
(E-DEV-017), stops `aicoded dev`.

## Installing aicoded

`make install`, in a framework checkout, installs `aicoded` from it, with the path of the
checkout built in for `aicoded init`.

## See also

- [aicoded dev](dev.md): running the apps.
- [MCP](mcp.md): these tools for your AI assistant.
- [Publishing](publishing.md): sending an app to the platform, and reading the outcome.
- [The error catalogue](../errors/README.md): every code with its page.
