# Publishing: sending an app to the platform

`aicoded publish` sends a commit of an app to the platform's delivery pipeline, which checks it
and writes a change record for it, and `aicoded status` shows the outcome. This guide covers
signing in, the clean commit, what is sent, the waiting, reading the outcome, what the delivery
pipeline checks and the codes.

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
  nothing. Files that `.gitignore` names do not count, and are never sent;
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
  `.aicoded/`, and the `access` and `services` sections of `aicoded.yaml`. The delivery pipeline
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
  lint            failed
  secrets         passed
  vulnerabilities passed
  licences        passed
  sbom            passed
  capabilities    passed
  build           skipped
  tests           skipped
  rooms: deps/rooms.go:14: E-GATE-015: Error return value of `db.ExecContext` is not checked
    fix: handle the error the call returns, or return it
    docs: https://aicoded.dev/docs/errors/E-GATE-015
  note: rooms: deps/rooms.go:31: E-GATE-025: //nolint:errcheck silences a check
    fix: remove the directive and fix what the check reports
    docs: https://aicoded.dev/docs/errors/E-GATE-025
  checks: the security checks of layers L1 to L5 ran; simulated attacks (L7) come later
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
problems and notes it found. Its number is on the line `change record`.

## What the delivery pipeline checks

The delivery pipeline runs these steps in this order. Each scanner, the build and the tests run
in a sandbox of their own, with no network. The layers are those of the security checks, which
the change record names too.

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
| `build` | L1 Approved parts only | builds the app's program | a build that fails |
| `tests` | L1 Approved parts only | runs the tests with `-race` | a failing test (E-CHK-004) |

`checkout` and `modules` come first, and a problem in either skips the rest. The steps from
`check` to `capabilities` run no code of the app, and they run whatever one of them finds, so one
publish shows every problem they find, with one exception: when `check` finds Go that does not
compile (E-CHK-002) or `go mod tidy` fails (E-CHK-006), the steps that read the app's packages,
`lint`, `vulnerabilities`, `licences` and `capabilities`, are `skipped`, while `secrets` and
`sbom` still run. `build` and `tests` run only when all of them passed. A step that runs out
of time, memory or disk is E-GATE-012, and tests that stop the checks, or the go command that
runs them (E-CHK-008), are E-GATE-028. A finding of a check that has no code of its own is
E-GATE-000.

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
- E-GATE-025: a directive silences a check.

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

The simulated attacks (L7) do not run yet, so a publish whose `lint` step ran says
`checks: the security checks of layers L1 to L5 ran; simulated attacks (L7) come later`. Any
other publish says `partial checks: not every security check ran; the steps above say which`,
such as one that has not ended, one that a step before `lint` stopped, or one whose `check` found
Go that does not compile; an older platform that runs only `aicoded check` and the tests says it
of every publish. Approvals and releases to production come later too, so a publish that passes
is checked and recorded, but runs nowhere yet.

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
- E-GATE-021 to E-GATE-025: notes, which do not stop a publish.

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

- [Tools](tools.md): `aicoded publish`, `aicoded status`, `aicoded check` and `aicoded login`.
- [MCP](mcp.md): the `publish` and `release_status` tools.
- [Project structure](project-structure.md): which files are yours and which are generated.
- [The error catalogue](../errors/README.md): every code with its page.
