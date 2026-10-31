# Publishing: sending an app to the platform

`aicoded publish` sends a commit of an app to the platform's delivery pipeline, which checks it
and writes a change record for it, and `aicoded status` shows the outcome. This guide covers
signing in, the clean commit, what is sent, the waiting, reading the outcome, what the delivery
pipeline checks, previews on the platform and the codes.

## Signing in

Publishing uses your sign-in to the platform: run `aicoded login --org <organisation>` once, as
[the tools](tools.md) explain. Without a sign-in, or with one that has ended, `aicoded publish`
and `aicoded status` fail with E-CLI-004. Signing in needs a person and a browser, so an AI
assistant asks you to run `aicoded login`.

The first publish of an app's name creates the app in your organisation, with you as its owner.
Only its owner publishes it again: another builder's app is E-PUB-005, and an app that an
administrator archived is E-PUB-006. A builder creates at most 10 apps a day (E-PUB-008).

## The clean commit

A publish sends a commit, never the files as they are on disk. Before anything leaves your
computer, `aicoded publish` refuses:

- an app folder, the one that holds `aicoded.yaml`, that is not the top level of a git
  repository of its own with a commit (E-PUB-001). An app in a folder of a bigger repository is
  refused, since the history of the whole repository would be sent: run `git init` in the app's
  folder and commit the app there;
- a change that is not committed, untracked files included (E-PUB-002): `git status` must list
  nothing. Files that `.gitignore` names do not count, and are never sent; neither do the draft
  of schema changes, `.aicoded/migration.sql`, and `.aicoded/migrations.json`;
- a commit or an index that holds the draft (E-MIG-001): run
  `git rm --cached .aicoded/migration.sql`. While the publish that carried the draft has not
  ended, a new publish that still carries a draft is refused the same way;
- with `--app <name>`, an app of another name (E-PUB-003);
- any problem that `aicoded check --frozen --no-tests` finds (E-PUB-004). `aicoded publish`
  prints them as `aicoded check` does and sends nothing. It checks as a released `aicoded` does,
  so it refuses a `replace` line (E-LINT-011) even from an `aicoded` installed from a framework
  checkout. The tests run on the platform;
- a commit the app has published already (E-PUB-007): every publish needs a new commit;
- a git older than 2.31, or none on `PATH` (E-PUB-012).

Run `aicoded check --frozen` first: it runs the tests too, and reports every generated file
that is out of date (E-CHK-001). Run `aicoded generate`, or `aicoded check` without `--frozen`,
and commit what it writes.

## What the commit holds

- The app's own files: `go.mod` and `go.sum`, the permission list, `main.go`, `deps/`, the pages
  with their data providers, scripts, styles and images, and `rpc/`.
- Every generated file, current: `*_gen.go`, `*_gen.ts`, `pages/assets_gen/`, `services/`,
  `.aicoded/` but the draft and the cache of migrations, and the `access` and `services`
  sections of `aicoded.yaml`. The delivery pipeline
  generates the app again and refuses it when a generated file differs (E-CHK-001), so generated
  files are never edited by hand.
- A `go.mod` that requires a released framework version that the platform builds (E-GATE-007),
  with no `replace` (E-GATE-001), `exclude` (E-GATE-002) or `toolchain` line (E-GATE-003), a
  `go` line no newer than the platform's Go (E-GATE-004), and a tidy `go.sum` (E-GATE-005). An
  `aicoded` installed from a checkout with `make install` writes a `replace` line to that
  checkout, which only that `aicoded` accepts: require a released version before you publish.
- Only the third-party modules that `modules:` in `aicoded.yaml` declares, and what they and the
  framework depend on (E-GATE-006), each matching the Go checksum database (E-GATE-008).
- Plain files and folders only, no symbolic link (E-GATE-009), at most 20,000 files and 256 MiB
  (E-GATE-010), and no `go.work` file, `vendor` folder or GOFLAGS entry outside the short list
  that E-LINT-011 allows.

## What is sent

- A git bundle of the commit that `HEAD` names, with its history: all of it on the first publish
  of an app, and afterwards only the history since the app's base, the last publish that the
  platform has verified. When the commit does not descend from the base, because the history was
  rewritten with `git commit --amend` or a rebase, or when the history since the base merges
  older history, all of it is sent again, and the change record says when history was rewritten.
- Nothing outside the app's repository, and from it nothing but that history: other branches,
  tags, stashes, ignored files and the repository's configuration stay on your computer.
- The app's name, the commit, and a summary for the change record: `-m <summary>`, or else the
  commit's subject. Control characters are removed, line breaks and tabs become spaces, and the
  first 1000 characters are kept, or fewer when they take more than 3900 bytes, as emoji do.
- The draft of schema changes, `.aicoded/migration.sql`, when the app has one, beside the
  bundle. Once the publish passed, `aicoded publish` deletes it, and a publish that failed leaves
  it. [Schema changes](schema-changes.md) tells what happens to it then.

The bundle may be at most 32 MiB (E-PUB-009). A committed file stays in the history after you
delete it, so keep large files out of the repository. `aicoded` runs git without your `GIT_`
environment variables, so that none of them points it at another repository, and passes every
commit to git on its standard input.

## The waiting

`aicoded publish` prints `Published <app> at <commit> as <id>.`, then asks the platform for the
outcome every 3 seconds, for up to 35 minutes, and prints each status as it changes:
`queued ...`, then `running ...`. Ctrl-C stops the waiting, not the publish, and prints
`the publish goes on: aicoded status <id>`, as a wait that runs out of time does. `--no-wait`
prints the id and returns at once.

`aicoded status <id>` prints what the end of the waiting prints, without waiting, and `--json`
prints the publish as JSON. An id is `pub_` and 26 lowercase letters and digits. Another shape is
a command line that is not valid. An id that the platform does not know in your organisation is
E-PUB-013, and a publish of another builder's app is E-PUB-005.

## Reading the outcome

```text
Published rooms at 1a2b3c4 as pub_hoskmcer6l2grf5cxrfq3ilioe.
queued ...
running ...
publish pub_hoskmcer6l2grf5cxrfq3ilioe of rooms at 1a2b3c4: Add the list of rooms
  checkout        passed
  modules         passed
  check           passed
  lint            passed
  secrets         passed
  vulnerabilities passed
  licences        passed
  sbom            passed
  capabilities    passed
  migrations      passed
  data            passed
  build           passed
  tests           passed
  attacks         failed
  personal-data   passed
  rooms: pages/rooms/n_id/index.html:1: E-GATE-032: POST /rooms/7 (form close) as another viewer with the roles staff: answered 303, expected 403 or 404
    fix: check ownership in the page's Guard, which runs before every form, not in Data or Process
    docs: https://aicoded.dev/docs/errors/E-GATE-032
  note: rooms: deps/rooms.go:31: E-GATE-025: //nolint:errcheck silences a check
    fix: remove the directive and fix what the check reports
    docs: https://aicoded.dev/docs/errors/E-GATE-025
  checks: the security checks of layers L1 to L5, L8 and L9, and the simulated attacks (L7), ran
  change record 7
failed: 1 problem
```

The last line is the status: `queued`, `running`, or how the publish ended. For a failed publish
it counts the problems; notes do not count.

| Status | What it means | Exit code |
|---|---|---|
| `passed` | every step passed | 0 |
| `failed` | a step found problems | 1 |
| `refused` | the platform could not read the commit from what was sent (E-PUB-014) | 1 |
| `error` | the delivery pipeline could not finish, through a fault of its own (E-PUB-011) | 1 |

Each problem has its code, `file:line`, fix and docs link, as in `aicoded check`: fix it as you
would a problem of `aicoded check`, commit, and publish again. Each note follows the problems, on
a line that starts with `note:`, with the same parts. After `refused` or `error`, publish a new
commit, even an empty one made with `git commit --allow-empty`.

Every publish gets a change record when it ends, whatever its outcome: who asked, with the
summary, what changed, and which checks ran, each with its layers, its outcome and how many
problems and notes it found. When the simulated attacks ran, it also counts the app's pages and
those the attacks reached, and lists those they did not. Its number is on the line
`change record`.

## What the delivery pipeline checks

The delivery pipeline runs these steps in this order. Each scanner, the build, the tests and the
app under attack run in a sandbox of their own, with no network. The layers are those of the
security checks, which the change record names too.

| Step | Layers | What it does | Stops the publish on |
|---|---|---|---|
| `checkout` | none | verifies the bundle and takes the commit out of it | a bundle it cannot read (`refused`), a symbolic link (E-GATE-009), a commit too large (E-GATE-010) |
| `modules` | L1 Approved parts only | checks `go.mod`, `go.sum` and the permission list, and fetches the modules the app may use | E-GATE-001 to E-GATE-008, E-GATE-011, E-GATE-026 and E-GATE-027 |
| `check` | L1 Approved parts only, L5 Every page locked | runs `aicoded check --frozen --no-tests` of the framework version the app requires | its problems |
| `lint` | L1 Approved parts only | runs golangci-lint with staticcheck (its checks of bugs and style, not its simplifications or quick fixes), gosec, errcheck, bodyclose and sqlclosecheck, and reads every Go file for directives | any finding (E-GATE-013 to E-GATE-017), a `//go:debug` comment (E-GATE-026) |
| `secrets` | L4 Leaked passwords | runs gitleaks over the commit and its history | any secret (E-GATE-018 and E-GATE-019) |
| `vulnerabilities` | L3 Known weaknesses | runs OSV-Scanner, then govulncheck, over the modules the app uses | a critical weakness the app calls (E-GATE-020) |
| `licences` | L3 Known weaknesses | reads the licence of each module with go-licenses | nothing |
| `sbom` | L3 Known weaknesses | lists the app's modules with Syft, as an SBOM | nothing |
| `capabilities` | L2 Stays in its limits | follows the app's calls to the network, files and other capabilities with capslock | nothing |
| `migrations` | L9 Safe data changes | checks the draft of schema changes that the publish carried, `.aicoded/migration.sql`, with the rules of `aicoded check`, and refuses a commit that holds the draft; a destructive change is the note E-MIG-003 | E-MIG-001, E-MIG-002 and E-MIG-006 |
| `data` | L8 Stays in its limits | notes the data that may reach the app's mail, as `aicoded check` finds it, and each data class that the permission list adds | nothing |
| `build` | L1 Approved parts only | builds the app's program | a build that fails |
| `tests` | L1 Approved parts only | runs the tests with `-race` | a failing test (E-CHK-004) |
| `attacks` | L7 Simulated attacks | starts the app it built and attacks it: requests without a session or a role, another viewer on each guarded page, and markup in every field | E-GATE-029 to E-GATE-034 and E-GATE-036 |
| `personal-data` | L7 Simulated attacks | searches what the app wrote during the attacks, its output and its spans, for what was entered in its forms | E-GATE-035 and E-GATE-037 |
| `decision` | L10 Approval if needed | decides on the release against the app type: *Pre-approved* or *Needs approval*, with each reason a note (E-GATE-039 to E-GATE-047); see [app types](app-types.md) | an app type, a group or a role that the organisation does not have, or data its source registry does not allow (E-REL-019 to E-REL-021), and a permission list that the platform's rules refuse whatever the app's framework version, such as a data class outside the four (E-MAN-008) or E-MAN-024 |

`checkout` and `modules` come first, and a problem in either skips the rest. The steps from
`check` to `data` run no code of the app, and they run whatever one of them finds, so one
publish shows every problem they find, with two exceptions: when `check` finds Go that does not
compile (E-CHK-002) or `go mod tidy` fails (E-CHK-006), the steps that read the app's packages,
`lint`, `vulnerabilities`, `licences` and `capabilities`, are `skipped`, while `secrets`, `sbom`
and `migrations` still run; and `data`, which takes its notes from `check`, is `skipped` when
`check` stopped before it read the app's code. The draft's problems that `check` finds show in
`migrations`, and `secrets` scans the draft too: a secret there is E-GATE-018 at
`.aicoded/migration.sql:<line>`. `build` and `tests` run only when all of them passed.
`attacks` and `personal-data` run only when `tests` passed, on one run of the app, and
`personal-data` is skipped when `attacks` ended in `error` or the app did not start
(E-GATE-036). `decision` comes last, only when every other step passed.
[Simulated attacks](simulated-attacks.md) says what each attack does and how to let the attacks
reach every page. A step that runs out of time, memory or disk is E-GATE-012, and tests that stop
the checks, or the go command that runs them (E-CHK-008), are E-GATE-028. A finding of a check
that has no code of its own is E-GATE-000.

Each check runs with the platform's own configuration, so nothing in the app's repository, such
as a `.golangci.yml`, a `.gitleaksignore`, a `.gitleaks.toml`, a `gitleaks:allow` comment, an
`osv-scanner.toml` or a `.syft.yaml`, changes what it finds. The checks read generated files too,
so a `Code generated ... DO NOT EDIT` line hides no file. The vulnerability databases are the
platform's own copies; when they are more than a day old, the `vulnerabilities` step ends in
`error`, and you publish a new commit later, as after any `error`.

A step reports problems, which fail the publish, and notes, which do not stop a publish. The
change record counts the notes, and security sees them, so fix them when you can:

- E-GATE-021: the app calls code with a known weakness that is not critical, with a score below
  9.0 or no score yet, or that is in the Go standard library;
- E-GATE-022: a module has a known weakness that the app does not call;
- E-GATE-023: a module's licence is forbidden or not recognised. Licences stop nothing: the
  list of every module's licence and the SBOM are kept with the change record;
- E-GATE-024: the app reaches a capability, such as the network or files, without a building
  block;
- E-GATE-025: a directive silences a check;
- E-GATE-038: a page the simulated attacks did not reach, which the change record lists too;
- E-GATE-039 to E-GATE-047: why the release *Needs approval*, as [app types](app-types.md)
  explains;
- E-MIG-003: a destructive schema change, which makes the release *Needs approval* and stops the
  app briefly while it is applied;
- E-DATA-001: data from the app's database, its files, its inbox or a secret that the app's mail
  may send;
- E-DATA-002: a data class that the permission list adds, which the app did not have before.

A known weakness that the app calls stops the publish when its score is 9.0 or more: the highest
CVSS score of the weakness and its aliases, such as its CVE id, in the OSV database (E-GATE-020).
A weakness of the Go standard library never stops it, since the platform chooses the Go that apps
build with: it is a note, and your administrator updates Go.

`//nolint` and `#nosec` are honoured for now: golangci-lint skips the finding that a `//nolint`
comment silences, and gosec the one that `#nosec` or `//gosec:disable` silences. But each such
directive is a note (E-GATE-025) at its `file:line`, which the change record counts, and
security sees them. Fix what the check reports instead of silencing it.

A `godebug` setting in `go.mod` and a `//go:debug` comment in a Go file change how Go behaves at
run time, and some turn off its security defaults, so the delivery pipeline refuses both
(E-GATE-026). Leave Go's defaults as they are. The `go` line of `go.mod` selects Go's defaults
too, so the delivery pipeline also refuses a `go` line older than the release of the Go it builds
with, such as `go 1.25.0` when it builds with Go 1.26 (E-GATE-026): `go mod edit -go=1.26.0`
raises it.

A publish whose `attacks` step ran says
`checks: the security checks of layers L1 to L5, L8 and L9, and the simulated attacks (L7), ran`.
A publish whose `lint`, `migrations` and `data` steps ran but not its attacks, such as one whose
tests failed or one to a platform that does not run the attacks yet, says
`checks: the security checks of layers L1 to L5, L8 and L9 ran; simulated attacks (L7) come later`.
Any other publish says `partial checks: not every security check ran; the steps above say which`,
such as one that has not ended, one that a step before `lint` stopped, or one whose `check` found
Go that does not compile; an older platform that runs only `aicoded check` and the tests says it
of every publish.

## Going live

A publish that passes is decided on at once, against its app type, and the end of
`aicoded publish` and `aicoded status` says what happens next, before the status line:

```text
  change record 8
  Pre-approved by app type internal-tool: going live.
passed
```

- `Pre-approved by app type <type>: going live.`: the platform releases it on its own, and the
  line says `live` once the new version serves.
- `Needs approval:`, then a line for each reason, then
  `An approver must approve it before it goes live.`: the permission list asks for more than the
  app type allows, or the release holds a change that always needs an approver, such as a
  destructive schema change.
- `Declined by <email>: <reason>.`, then `Publish again to ask for a new approval.` and a line
  about schema changes: an approver declined the approval card, which is final, and the command
  exits 1. Change what the reason asks for, commit, and publish again: a new publish makes a new
  approval card. The declined publish's schema changes left the app's migrations, and the draft
  was cleared when it passed, so put the ones you still want back in `.aicoded/migration.sql`; a
  publish made on top of them meanwhile fails with E-MIG-004.
- `Released: replaced by a newer release.`: the release went live, and a newer release of the
  app has since taken its place.
- `Release failed:` and why: the release did not go live where the app runs.
- `Not released:` and why: a newer publish of the app came after it, or the platform refused the
  release, such as one with scheduled jobs (E-REL-017).

`aicoded status --json` and the `release_status` tool give the same in `release`: its `tier`
(`R1` for *Pre-approved*, `R2` for *Needs approval*), its `app_type`, its `reasons`, its `state`
(`due`, `waiting`, `released`, `failed`, `replaced`, `refused` or `superseded`), the `reason` of
a refusal or a failure, and whether it is `live`; a declined approval card is in `declined`, with
the approver's email as `by`, the `reason` and the time `at`. [App types](app-types.md) tells how
the platform decides.

## When a release stops at a schema change

When the platform releases a publish to production, it first applies the app's schema changes
that the production database has not run yet, as [schema changes](schema-changes.md) tells. When
one of them fails, the release stops and the version that runs keeps serving (E-MIG-005). The
message names the step, as `<publish>#<index>`, and MySQL's error number, with a fixed short text
such as `MySQL error 1062 (duplicate key)`, never MySQL's own message, which can quote the app's
data.

- **MySQL refused the statement on production data**, such as a unique key over rows that repeat
  a value. The steps that ran in production stay, and the schema changes after them are rejected:
  every publish that holds them is never released, and `aicoded status` and `release_status`
  show E-MIG-005 as a problem of each such publish, though its status stays `passed`. A release
  of one is refused with E-MIG-004. Put a corrected version of the unreleased changes in
  `.aicoded/migration.sql`, leaving out the steps that ran, and publish again. `aicoded dev`
  makes the dev database again from the app's migrations and the new draft.
- **The step could not finish**, through a lock wait, a deadlock, a time limit or a broken
  connection. Nothing is rejected: the publish shows E-MIG-005 as a note, and the next release
  tries the step again.
- **The step ran but was not recorded**: the connection broke right after the step ran, and its
  row could not be added to the journal even on a new connection. Nothing is rejected: the
  publish shows E-MIG-005 as a note, and the platform's operator records the step before the next
  release. Nothing in the app needs to change.

## Preview

A publish that passed can run on the platform as the app's preview, so that you and a few people
you pick try it before it goes live. Starting one is a step of its own, never part of publishing,
and only the app's owner takes it, with the id of the publish:

```sh
aicoded preview pub_7kq2m4x5c3v6b5n6d2f7hjrt2a
```

```text
Preview of demo at https://demo.acme.preview.aicoded.cloud/ until 2026-11-13T10:00:00Z.
Invite up to five people in the console.
```

- **Signing in.** A preview needs the scope `app:<id>:preview` of your sign-in. A sign-in made
  before previews existed lacks it, and `aicoded preview` refreshes the sign-in once to get it, so
  you do not sign in again; a sign-in that has ended is E-CLI-004, as for publishing.
- **Which publish.** One that passed its checks and whose approval card was not declined, whatever
  its app type allows: a publish that *Needs approval* can be previewed while its card waits. Any
  other is E-REL-029.
- **One per app,** at `<app>.<organisation>.preview.aicoded.cloud`, for 14 days from its first
  start, never extended. A start with a newer publish replaces the version the preview runs,
  keeps its database and runs the new schema changes on it, destructive ones too. When the new
  publish does not continue the schema changes the preview ran, such as after a declined publish,
  the preview's data is deleted and the schema changes run from the start, and
  `aicoded preview` prints
  `Its data was reset: the preview had run schema changes that this publish does not continue.`
- **Data.** A preview starts with an empty database, with the app's schema changes applied, and
  empty file stores. People enter test data of their own through the app; never put real data in
  it.
- **Who sees it.** You, as the app's owner; up to five people of the organisation who have signed
  in before, whom you pick in the console (E-REL-031 refuses a sixth); and, while an approval card
  of the app waits, whoever may approve it. The permission list's audience does not apply there,
  and nobody outside the organisation sees a preview; the app's own access rules still apply
  inside it. Anyone else of the organisation gets the page E-RUN-006, and so does a person you
  removed, or an approver once the card is decided, within 15 minutes. Your AI assistant invites
  no one.
- **Values.** A preview reads its own setting and secret values, which the app's owner or a
  security admin sets for the preview in the console, never the production values. A value the
  permission list declares and the preview lacks is E-REL-030.
- **Mail.** A preview sends no mail: each message the app sends is kept for the app's owner to
  read in the console, the newest 200, each of up to 1 MiB (E-MAIL-011).
- **Logs and traces.** `aicoded logs --env preview` and `aicoded traces --env preview` read the
  preview's.
- **The end.** `aicoded preview end` in the app's folder, or the console, ends the preview at
  once, and it ends by itself after 14 days; a security admin may end it in the console too. Its
  address stops answering at once, then its tables and files are deleted. A start after the end
  begins a new preview, with empty data and 14 days of its own.
- **Records.** Each start, new version, reset of the data, person added or removed, and end is in
  a change record of the app.

With `aicoded mcp`, the tools `preview_publish` and `preview_end` do the same, and your AI
assistant calls them only when you ask.

## What security sees

Security will approve what the app may do, not its code. The permission list is that statement, so
read it before you publish:

- `class`, `owner` and `audience` name its app type, who owns it and who may use it.
- `data` names everything the app stores or reads, with its classes.
- `egress` names the outside services it calls, `email` where its mail can go, and `schedule`
  the jobs it runs on its own.
- `settings` and `secrets` name what it is configured with, and `modules` the third-party code
  it uses.
- `size`, `resources` and `ttl` name its limits and when it is reviewed.
- `access`, which `aicoded generate` writes, names who may open each page, and `services` which
  apps it calls and which may call it.

A page with no access rule does not generate at all. [The permission list](permission-list.md)
says which sections are enforced today, and which the platform only records for now.

## The codes

- E-PUB-001 to E-PUB-004 and E-PUB-012: a commit that `aicoded publish` refuses before it sends
  anything.
- E-PUB-005 to E-PUB-010: a publish that the platform refuses; `aicoded publish` refuses
  E-PUB-006, E-PUB-007 and E-PUB-009 before sending when it can tell.
- E-PUB-011 and E-PUB-014: a publish that ended in `error` or `refused`.
- E-PUB-013: a publish id that the platform does not know in your organisation.
- E-GATE-001 to E-GATE-012 and E-GATE-026 to E-GATE-028: what the delivery pipeline finds in
  the commit, its modules, its permission list and its steps.
- E-GATE-000 and E-GATE-013 to E-GATE-020: what the security checks find in the code, its
  history and its modules.
- E-GATE-029 to E-GATE-037: what the simulated attacks find in the running app.
- E-MIG-001 to E-MIG-007: the draft of schema changes, and the migrations a release applies.
- E-REL-019 to E-REL-021: a publish that fails when the platform decides on its release.
- E-REL-029 and E-REL-030: a preview that the platform does not start.
- E-GATE-021 to E-GATE-025, E-GATE-038 to E-GATE-047, E-MIG-003, E-DATA-001 and E-DATA-002:
  notes, which do not stop a publish.

`aicoded explain <code>` prints the page of each, as does the `howto` tool of `aicoded mcp`.

## What the framework does not check yet

Some rules are yours to keep, since nothing checks them yet:

- A value a viewer sends is checked, beyond its type and options, in the hook that uses it; see
  [forms](forms.md).
- A mail's idempotency key names the event, never the attempt; see [mail](mail.md).

## Running without the platform

An app runs only under a runner: on your computer that is `aicoded dev`. A minimal runner for
running an app in production without the platform, `aicoded run`, comes later.

## See also

- [Tools](tools.md): `aicoded publish`, `aicoded status`, `aicoded preview`, `aicoded check` and
  `aicoded login`.
- [MCP](mcp.md): the `publish`, `release_status`, `preview_publish` and `preview_end` tools.
- [Simulated attacks](simulated-attacks.md): what the `attacks` and `personal-data` steps do.
- [App types](app-types.md): what goes live on its own and what needs approval.
- [Schema changes](schema-changes.md): the draft that a publish carries.
- [Project structure](project-structure.md): which files are yours and which are generated.
- [The error catalogue](../errors/README.md): every code with its page.
