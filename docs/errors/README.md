# Error catalogue

Every error the framework, the generator, the CLI or a runner reports on purpose has a code
`E-<AREA>-<NNN>`, a one-line fix and a page here named after the code.

| Area | Reported by |
|---|---|
| GEN | generator and templates |
| WEB | web runtime |
| RPC | calls between apps: the generator, `aicoded rpc add`, the runner and the called app |
| MAN | aicoded.yaml |
| RUN | app↔runner protocol |
| DEV | `aicoded dev`, its control socket and its dev UI, and the apps of a workspace in `aicoded check`, `aicoded describe` and `aicoded mcp` |
| CHK | `aicoded check` and the Go build it runs, also in `aicoded dev` |
| LINT | the rules for app code that `aicoded check` applies after `go vet`, listed in [the rules guide](../guides/rules.md) |
| CLI | `aicoded init`, `aicoded explain`, `aicoded login`, `logout` and `whoami`, the sign-in that `aicoded publish` and `status` use, and the `app_create` and `howto` tools of `aicoded mcp` |
| PUB | `aicoded publish` and `aicoded status`, and the `publish` and `release_status` tools of `aicoded mcp` |
| GATE | the security checks of the platform's delivery pipeline, in the problems and notes of a publish |
| SQL | the app's database connection (mysql.sock) |
| FILE | file stores |
| MAIL | mail rules |

Calls between apps use these codes:

- E-RPC-001 to E-RPC-004: the functions in `rpc/`.
- E-RPC-005: snapshot files.
- E-RPC-006 and E-RPC-007: `aicoded rpc add` and the generated clients.
- E-RPC-008 to E-RPC-012: a call the runner or the called app refuses.
- E-RPC-013: a held snapshot that the called app broke.
- E-RPC-014: a file in the folder of a generated client.

Workspaces use these codes:

- E-DEV-010: two apps with one name in a workspace, in `aicoded dev`, `aicoded check`,
  `aicoded describe`, `aicoded generate` or `aicoded rpc add`.
- E-DEV-015: an app whose `app:` line changed while `aicoded dev` runs.
- E-DEV-016 and E-DEV-017: a control socket that `aicoded dev` cannot make, because something
  else is at its path or the path is too long.
- E-DEV-018 and E-DEV-019: the dev UI's token file, and a request to the dev UI from a browser
  that did not log in.

Signing in to the platform uses these codes:

- E-CLI-004: no sign-in to the platform, or one that has ended.
- E-CLI-005: an address in `AICODED_PLATFORM` that is not valid.
- E-CLI-006: a sign-in that the platform refused.
- E-CLI-007: a credentials file or folder that others can read.
- E-CLI-008: a sign-in that was not finished in time.
- E-CLI-009: a sign-in that the platform could not refresh right now.

Publishing uses these codes:

- E-PUB-001 to E-PUB-004 and E-PUB-012: a commit that `aicoded publish` refuses before it sends
  anything: not the top level of a git repository with a commit, changes not committed, an
  `--app` of another name, problems that `aicoded check` found, and git missing or too old.
- E-PUB-005 to E-PUB-010: a publish that the platform refuses: another builder's app, an
  archived app, a commit already published, too many new apps, a bundle too large, and a bundle
  not as claimed.
- E-PUB-011 and E-PUB-014: a publish that ended in `error` or `refused`.
- E-PUB-013: a publish id that the platform does not know in your organisation.
- E-GATE-001 to E-GATE-008: `go.mod`, `go.sum` and the modules an app uses.
- E-GATE-026: a `godebug` setting in `go.mod` or a `//go:debug` comment in a Go file, which
  change Go's security defaults.
- E-GATE-009 and E-GATE-010: the files of the commit.
- E-GATE-011 and E-GATE-027: the commit's permission list, which must name the app and be
  readable.
- E-GATE-012 and E-GATE-028: a step that ran out of time, memory or disk, and tests that stopped
  the checks.
- E-GATE-000: a finding of a check that has no code of its own.
- E-GATE-013 to E-GATE-017: what staticcheck, gosec, errcheck, bodyclose and sqlclosecheck find.
- E-GATE-018 and E-GATE-019: a secret in the commit or in its history.
- E-GATE-020: a critical known weakness that the app calls.
- E-GATE-021 to E-GATE-025 are notes, which do not stop a publish: known weaknesses that the app
  calls but that are not critical or are in the Go standard library, known weaknesses that the app
  does not call, licences, capabilities reached without a building block, and directives that
  silence a check.

`aicoded explain <CODE>` prints the page.

Retired codes are never reused:

- E-GEN-044 refused a page with pages below it and no `<ssr:content/>`. Such a page is now a gate: its access rule and `Guard` cover the pages below it, which are not shown inside it.
