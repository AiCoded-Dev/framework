# Error catalogue

Every error the framework, the generator, the CLI or a runner reports on purpose has a code
`E-<AREA>-<NNN>`, a one-line fix and a page here named after the code.

| Area | Reported by |
|---|---|
| GEN | generator and templates |
| WEB | web runtime |
| RPC | calls between apps: the generator, `aicoded rpc add`, the runner and the called app |
| MAN | aicoded.yaml |
| RUN | app↔runner protocol, and what a runner refuses to serve |
| DEV | `aicoded dev`, its control socket and its dev UI, and the apps of a workspace in `aicoded check`, `aicoded describe` and `aicoded mcp` |
| CHK | `aicoded check` and the Go build it runs, also in `aicoded dev` |
| LINT | the rules for app code that `aicoded check` applies after `go vet`, listed in [the rules guide](../guides/rules.md) |
| CLI | `aicoded init`, `aicoded explain`, `aicoded login`, `logout` and `whoami`, the sign-in that `aicoded publish` and `status` use, and the `app_create` and `howto` tools of `aicoded mcp` |
| PUB | `aicoded publish` and `aicoded status`, and the `publish` and `release_status` tools of `aicoded mcp` |
| GATE | the security checks of the platform's delivery pipeline, in the problems and notes of a publish |
| REL | the platform's release of a publish to production |
| SQL | the app's database connection (mysql.sock) |
| FILE | file stores |
| MAIL | mail rules, the mailbox where a target keeps none, and the operator's mail domains |
| LOGIN | company login, which signs people in to the apps of their organisation |

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
- E-GATE-026: a `godebug` setting in `go.mod`, a `//go:debug` comment in a Go file, or a `go`
  line older than the Go of the delivery pipeline, which change Go's security defaults.
- E-GATE-029 to E-GATE-037: what the simulated attacks find in the running app: a page that
  answers a request without a valid session, or lets in a viewer without the role its access
  rule names; another viewer who saw or changed a record through a guarded page; a guarded page
  of which the attacks reached no record; what someone entered, served as markup or found in the
  app's log or telemetry; an app that did not start or stopped; and more log or telemetry than
  the checks read.
- E-GATE-038 is a note, which does not stop a publish: a page the simulated attacks did not
  reach.

Releasing uses these codes, each a release that the platform refuses before anything runs:

- E-REL-001: a publish that did not pass the delivery pipeline, has no built app, or has a
  permission list that the platform cannot read.
- E-REL-002: an organisation that the platform's operator has not placed on a server.
- E-REL-003: an app built with a framework older than v0.4.0.
- E-REL-004, E-REL-005 and E-REL-007: what the platform does not run yet: an audience other than
  everyone in the organisation, settings and secrets, and calls between apps.
- E-REL-006, E-REL-008 and E-REL-013: an `email.from` address in a domain that the organisation
  has not verified, that another app of the organisation has, or of which the hosting makes no
  mailbox.
- E-REL-014: an `email.from` other than the address the app sends from already.
- E-REL-010: an app that has a release pending already.
- E-REL-011: a control plane with no hosting for the apps' resources.
- E-REL-012: an app whose name, or whose organisation's slug, cannot be a label of its host.
- E-REL-015: an app that the platform's operator archived.

E-REL-009 refuses the operator's purge of a publish that a live or pending release runs.

E-MAIL-009 refuses the operator's adding or verifying a mail domain on a control plane with no
hosting for mail.

E-RUN-005 refuses an app's call to a building block that the runner where the app runs does not
provide.

E-RUN-006 is the page a runner shows, to everyone, for an app whose audience it cannot check.

E-LOGIN-001 refuses to sign a person in to an app when the sign-in provider gives them an identity
of more than 512 bytes.

`aicoded explain <CODE>` prints the page.

Retired codes are never reused:

- E-GEN-044 refused a page with pages below it and no `<ssr:content/>`. Such a page is now a gate: its access rule and `Guard` cover the pages below it, which are not shown inside it.
