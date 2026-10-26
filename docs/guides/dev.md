# aicoded dev: run apps on your computer

`aicoded dev` is the runner on your computer. It generates, builds and starts every app in a
folder, signs viewers in as test personas, gives the apps their database, file stores and caught
mail, rebuilds an app when its files change, and serves a dev UI with the apps' logs, traces and
mail.

## Running it

```sh
aicoded dev [dir]
aicoded dev --manual <app> [dir]
```

`dir` is the workspace, the current folder by default. `aicoded dev` takes every app below it:

- It finds every folder below it that has an `aicoded.yaml`, without following symbolic links
  below it. It does not look inside an app's folder, or in folders named `vendor`,
  `node_modules` or `testdata` or starting with `.` or `_`. Two apps with one name are refused
  (E-DEV-010).
- It runs `aicoded generate` in every app.
- It checks every snapshot an app holds against the called app's current one. An app whose held
  snapshot no longer matches fails alone (E-RPC-013). A snapshot of an app outside the folder
  only prints a warning.
- It starts each app after the apps it calls and carries the calls between them, so an app may
  call them in `OnStart`. Of apps that call each other, directly or through other apps, the one
  whose folder comes first starts first: never call, in `OnStart`, an app that calls this app too
  (E-RPC-011).
- It serves every app a viewer can open at `http://<app>.localhost:8080/`, and prints
  `aicoded dev: contacts (calls only)` for an app that has no pages and serves calls.

Open the apps in Chrome. Firefox should work but is not tested. Safari does not work: it drops
the persona cookie on plain http, and macOS does not resolve `*.localhost`.

## dev.yaml

Local values and test personas live outside the repository, in `dev.yaml` in your user
configuration folder: `~/.config/aicoded/dev.yaml` on Linux (`$XDG_CONFIG_HOME/aicoded/dev.yaml`
if that is set) and `~/Library/Application Support/aicoded/dev.yaml` on macOS. It holds local
secrets, so it must have mode 600 (E-DEV-001). Without the file, `aicoded dev` uses its defaults.

```yaml
workspaces:
  /home/me/apps:              # absolute path of the folder you run aicoded dev in
    port: 8080                # the port of the apps and the dev UI; 8080 by default
    env: dev                  # the apps' environment: dev by default, or preview
    mysql: root:<password>@unix(/var/run/mysqld/mysqld.sock)/   # a local MySQL 8, for apps with sqldb
    personas:
      - {name: alice, roles: [staff], groups: [lisbon]}
      - {name: bob, roles: [staff, hr]}
    apps:
      people:
        settings: {team_address: team@acme.example}
        secrets: {partner_api_key: local-test-key}
```

- A workspace is the folder you run `aicoded dev` in, and holds the local values of all its apps
  under `apps:`, one entry per app name.
- A field `aicoded dev` does not know, or a syntax error, is E-DEV-002.
- `env` is the environment the runner tells the workspace's apps they run in: `dev`, the
  default, or `preview`, which runs them as a preview, as they run outside `aicoded dev`. Under
  `preview`, `app.Env` returns `preview`, each app's database and user are `ac-preview-<app>`,
  apart from those of `dev`, and the framework records errors and panics by their kinds and
  codes only, as outside `aicoded dev`. Any other value is E-DEV-020. `aicoded dev` reads `env`
  when it starts.
- An app whose settings or secrets have no value here fails with E-DEV-003. See
  [settings and secrets](settings-and-secrets.md).
- `dev.yaml` is read again at every start of an app, so after adding a value, change a file of the
  app, press Restart on the dev UI or call the `preview` tool.

## Personas

On your computer, personas stand in for the company login. Choose one at
`http://<app>.localhost:8080/_aicoded/persona`; a cookie keeps the choice. A persona's name is
both the viewer's `Subject` and `Name`, with the persona's roles and groups, so it is 1 to 512
bytes, as a `Subject` is (E-DEV-021).

Without `personas` in `dev.yaml`, there are two: `all-roles`, with every role that the `access`
and `services` sections of the workspace's apps name, and `no-roles`. The first persona of the
list is the one a new browser gets.

## Building blocks

- An app that declares `sqldb` needs `mysql:` in its workspace: the admin DSN of a MySQL 8 server
  on this machine, over a Unix socket, `127.0.0.1` or `::1` (E-DEV-006, E-DEV-007). `aicoded dev`
  creates the app's own database there, `ac-dev-<app>` (`ac-preview-<app>` under `env: preview`),
  and two users, each with a new password on every start: the data user `ac-dev-<app>`, which
  may only read and write rows (`SELECT`, `INSERT`, `UPDATE`, `DELETE`) and which the app's
  `mysql.sock` logs in as, and the owner `ao-dev-<app>`, which only the migrations run as. A
  `CREATE TABLE` or any other schema change from the app gets MySQL's access-denied error. See
  [the SQL database](sql-database.md).
- Before every start and restart of such an app, `aicoded dev` applies its migrations to its
  database as the owner (see [Migrations](#migrations)).
- File stores live in a private state folder, readable by you only:
  `~/.local/state/aicoded/<hash>/files/<app>/<store>` (E-DEV-009).
- Mail is caught instead of sent. It is still checked against the mail rules and the permission
  list. See [mail](mail.md).

## Migrations

An app's tables come from its migrations, never from its code. Before it starts an app that
declares `sqldb`, `aicoded dev` applies to the app's database, as the owner:

1. the migrations the app published, which it asks the platform for with your sign-in and keeps
   in `.aicoded/migrations.json`. It asks at the app's first start, and again at a Start or
   Restart on the dev UI or the `preview` tool once its last answer is a minute old; a rebuild
   after a change takes the kept ones. When you are not signed in or the platform cannot be
   reached, it applies the ones kept there and prints one warning line, or, with none kept, only
   the draft. A `.aicoded/migrations.json` it cannot read is a warning line too, and only the
   draft is applied;
2. then the statements of the draft, `.aicoded/migration.sql`, the next schema change.

The database's journal, the table `aicoded_migrations`, holds what ran. A draft that grows is
applied on top. When the draft changes otherwise, or the journal holds steps that the published
migrations and the draft do not start with, as after a publish, `aicoded dev` drops the app's dev
database, makes it again and applies everything: its data is test data. A list of steps that
`aicoded dev` cannot apply at all, such as one over 2000 steps or 1 MiB, stops the app with
E-MIG-004 and leaves the database as it is. It never touches another
app's database. A draft that `aicoded check` refuses (E-MIG-001 to E-MIG-003, E-MIG-006) or a step
that fails (E-MIG-005) stops the app, and its problems show at its address, on the dev UI and in
`what_broke`.

The draft and `.aicoded/migrations.json` stay out of git: a new app's `.gitignore` lists both, and
`aicoded publish` refuses a commit that holds the draft (E-MIG-001). `aicoded publish` sends the
draft with the commit, and once that publish passed, deletes it and refreshes
`.aicoded/migrations.json`; a publish that failed leaves the draft as it was. When you publish with
`--no-wait`, or with the `publish` tool, `aicoded status`, `release_status` or the next publish
does that once the publish has ended; `release_status` and the `publish` tool say what became of
the draft in `draft` (`sent`, `settled` or `none`) and `draft_note`. A draft you changed after the
publish was sent stays, to be cut down to what is still to publish. While the publish that carried
the draft has not ended, `aicoded publish` and the `publish` tool refuse to send the draft again
(E-MIG-001): wait for its outcome first.

`aicoded check`, when the workspace's `dev.yaml` (or that of a folder above it) names `mysql:`,
makes a new test database for each app that declares `sqldb`; it reads `dev.yaml` only for the
tests of such an app, and a `dev.yaml` it cannot use stops the whole check. It applies the same
steps, runs the app's tests with `AICODED_TEST_DB` set to the data user's DSN, and drops the
database and its users after. A test opens it with `sqltest.Open(t)` from `aicoded.dev/framework/sqldb/sqltest`,
which skips the test when there is no test database.

## Watching

`aicoded dev` watches the files of every app. When they change, it generates, builds and restarts
that app; the files it writes itself start nothing. The running app keeps serving until the new
one is built. A new app folder starts, and a removed one stops. An app you stopped on the dev UI
is not rebuilt, and an app you run by hand is only generated.

## When an app fails

An app that fails stops alone, and its address shows its problems, with code, `file:line` and
fix, until a change builds or you start it again: Start or Restart on the dev UI, or the
`preview` tool. It fails when it does not generate, holds a broken snapshot, does not build,
lacks values in `dev.yaml` (E-DEV-003), has a draft that is refused or a migration that fails
(E-MIG-001 to E-MIG-006), exits or is not ready within 30 seconds of starting (E-DEV-004), or
gets a new name on its `app:` line while `aicoded dev` runs (E-DEV-015).

Only problems of the whole workspace stop `aicoded dev`:

- no app (E-MAN-006);
- two apps with one name when it starts (E-DEV-010);
- a port in use (E-DEV-011);
- a `dev.yaml` that is not valid (E-DEV-001, E-DEV-002, E-DEV-020, E-DEV-021);
- a state folder that others can read (E-DEV-009);
- another `aicoded dev` for the same workspace (E-DEV-012);
- a `--manual` name that no app of the workspace has (E-DEV-014);
- a token file of the dev UI that is not valid (E-DEV-018);
- a control socket it cannot make: something other than a socket at its path (E-DEV-016), or a
  path too long for a Unix socket (E-DEV-017).

## What it keeps

Each app keeps its last 5000 log lines, 10000 spans and 1000 caught mails across restarts.
Before it keeps or prints any of them, `aicoded dev` hides every value of the app's secrets that
is 4 bytes or longer as `[secret <name>]`; it prints a warning for a shorter one.

The apps' data under `aicoded dev` is made up, so in environment `dev` the framework logs and
traces each error with its text and each panic with its value, and an app that fails to start
prints its whole error. Under `env: preview` and anywhere else it records only their kinds and
codes, such as `MySQL error 1062 (23000)`; see [telemetry](telemetry.md).

One `aicoded dev` runs per workspace. A second one for the same folder, a folder below it or a
folder above it refuses and names the running one's address (E-DEV-012), then prints
`aicoded dev: log in to the running one at <link>`, the login link of the running one's dev UI.
See [tools](tools.md) for the control socket they talk through.

## The dev UI

`aicoded dev` serves a web UI for you, the developer, at `http://localhost:8080` (the workspace's
port, without an app name). Once it listens, before the apps build, it prints the login link:
`aicoded dev: the dev UI is at http://localhost:8080/_aicoded/login?token=<64 hex characters>`.

- Open the link from the terminal or paste it into the address bar. It gives your browser a
  cookie, and the dev UI answers nothing without that cookie (E-DEV-019). The browser sends the
  cookie only with visits that start at the dev UI itself or outside the browser. A login link
  clicked on another web page still sets the cookie, but the visit it starts lands on the
  E-DEV-019 page; the next time you type the dev UI's address into the address bar, you are
  logged in.
- The token is kept in `~/.local/state/aicoded/<hash>/ui-token`, the workspace's private state
  folder, with mode 600. It stays the same when `aicoded dev` restarts, so the link does too.
  Delete the file to get a new token at the next start. A file that is not a regular file of
  mode 600 holding 64 lowercase hex characters stops `aicoded dev` (E-DEV-018).
- **Apps** lists every app with its state (`starting`, `running`, `failed`, `stopped` or
  `manual`), since when, its address and the code of its first problem. An app's page shows its
  problems with `file:line`, fix and docs link, and has the buttons Start, Restart, Stop, "Run
  it yourself" and "Let aicoded dev run it".
- **Logs** shows the newest 200 log lines, newest first, by app, lowest level, text and trace.
- **Traces** lists the newest 100 traces, by app or with an error only. A trace's page shows its
  spans as a tree, with their timing, attributes and errors, and links to its log lines.
- **Mail** shows the mail each app sent and has in its mailbox. A message's page shows its
  headers, its text and its attachments' names, and its HTML part in a sandboxed frame that runs
  no script and loads nothing from the network. "Simulate inbound mail" puts a message in the
  app's inbox.
- **Personas** lists the personas with their roles and groups, and links to each app's persona
  page.
- **MCP setup** shows the commands that add `aicoded mcp` for this workspace to Claude Code,
  Codex and Cursor.
- The lists and an app's state follow the workspace live. The dev UI updates at most 8 open pages
  at a time, across all your tabs and browsers; a ninth shows its page but does not update it.
- Secret values show as `[secret <name>]`, as in the logs, and the dev UI shows no setting
  values.

Stop keeps an app stopped, also when its files change, until you press Start or Restart or call
`preview`. Its address then answers 503: `<app> is stopped. Start it on the dev UI at
http://localhost:8080/apps/<app>`, and the terminal shows `aicoded dev: <app> stopped`. Start
restarts an app that runs. A new `aicoded dev` starts every app.

## Running an app by hand

To run an app yourself, for example under a debugger, start `aicoded dev --manual <app> [dir]`,
with the flag before the folder and once for each such app, or press "Run it yourself" on the
app's page. `aicoded dev` then keeps the app's runner up (its sockets, database, file stores and
mail) in a run folder of mode 700, and never builds or starts the app. It prints the app's address
line the first time, then the command that starts the app:
`aicoded dev: hello runs by hand; start it with: cd /path/to/hello && AICODED_RUNNER_DIR=<run folder> go run .`.

- Run that command as often as you like; each time the app is ready, `aicoded dev` prints
  `aicoded dev: hello is ready (run by hand)`. In a debugger, run the app in its folder with
  `AICODED_RUNNER_DIR` set to the run folder the app's page shows. An app started without it
  stops with E-RUN-001.
- The run folder stays the same while the app runs by hand. A change to the app's files and the
  `preview` tool only generate it and keep its runner, unless its permission list or its
  `dev.yaml` values changed: then `aicoded dev` brings up a new runner in the same folder, and you
  start the app again. Start and Restart on the dev UI always bring up a new runner.
- While the app is not running, its address answers 503 with the command, and calls from other
  apps fail with "internal error".
- The app's log lines go to the terminal you run it in, not to the dev UI. Its spans and mail
  still show there.
- "Let aicoded dev run it" builds and runs the app again and removes the run folder; stopping
  `aicoded dev` removes it too. A `--manual` name that no app of the workspace has is E-DEV-014,
  and `aicoded dev` does not start.

## See also

- [Quick start](quickstart.md): the first run of a new app.
- [Tools](tools.md): the other `aicoded` commands.
- [MCP](mcp.md): the same workspace for your AI assistant.
- [The examples' README](../../examples/README.md): a `dev.yaml` for the example apps.
