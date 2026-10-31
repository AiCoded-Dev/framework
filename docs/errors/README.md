# Error catalogue

Every error the framework, the generator, the CLI or a runner reports on purpose has a code
`E-<AREA>-<NNN>`, a one-line fix and a page here named after the code.

| Area | Reported by |
|---|---|
| GEN | generator and templates |
| WEB | web runtime |
| RPC | calls between apps: the generator, `aicoded rpc add`, the runner and the called app |
| MAN | aicoded.yaml |
| RUN | app↔runner protocol, what a runner refuses to serve, and a runner's enrolment and connection to the platform |
| DEV | `aicoded dev`, its control socket and its dev UI, and the apps of a workspace in `aicoded check`, `aicoded describe` and `aicoded mcp` |
| CHK | `aicoded check` and the Go build it runs, also in `aicoded dev` |
| LINT | the rules for app code that `aicoded check` applies after `go vet`, listed in [the rules guide](../guides/rules.md) |
| CLI | `aicoded init`, `aicoded explain`, `aicoded login`, `logout` and `whoami`, the sign-in that `aicoded publish` and `status` use, and the `app_create` and `howto` tools of `aicoded mcp` |
| PUB | `aicoded publish`, `aicoded status` and `aicoded preview`, and the `publish`, `release_status`, `preview_publish` and `preview_end` tools of `aicoded mcp` |
| GATE | the security checks of the platform's delivery pipeline, in the problems and notes of a publish |
| REL | the platform's release of a publish to production, the approval of an approval card in the console, and an app's preview |
| DATA | notes on where the app's data goes, in `aicoded check` and the delivery pipeline: they never stop a check or a publish |
| MIG | schema changes: the draft `.aicoded/migration.sql` in `aicoded check`, `aicoded dev` and the delivery pipeline, and the migrations applied to a database |
| SQL | the app's database connection (mysql.sock) |
| FILE | file stores |
| MAIL | mail rules, the mailbox where a target keeps none, the messages a runner takes at once, a preview's messages, and the operator's mail domains |
| OUT | the requests an app sends to outside services with `outbound.Client()`, which the runner checks against `egress` |
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

The permission list uses these codes for what its fields may hold:

- E-MAN-008: a data source with no data class, or a class other than `public`, `internal`,
  `confidential` and `personal`.
- E-MAN-024: a permission list that is *Not allowed* whatever the app type: an audience that
  lists no one, or a data source with the class `health`, `hr` or `card`.
- E-MAN-025: a key bound to a host in `egress` with a secret that is not declared, a bad header
  or a bad prefix.

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
- E-PUB-015 to E-PUB-021: a read of an app's logs and traces on the platform that it refuses: a
  range over 7 days, over 1000 rows, a value that is not valid, a store that cannot be read right
  now, an app that is not on the platform, an answer too large, and rows that cannot be read.
- E-GATE-001 to E-GATE-008: `go.mod`, `go.sum` and the modules an app uses.
- E-GATE-009 and E-GATE-010: the files of the commit.
- E-GATE-011 and E-GATE-027: the commit's permission list, which must name the app and be
  readable.
- E-GATE-012 and E-GATE-028: a step that ran out of time, memory or disk, and tests that stopped
  the checks.
- E-GATE-000: a finding of a check that has no code of its own.
- E-GATE-013 to E-GATE-017: what staticcheck, gosec, errcheck, bodyclose and sqlclosecheck find.
- E-GATE-018 and E-GATE-019: a secret in the commit, in the draft of schema changes the publish
  carried, or in its history.
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
- E-GATE-039 to E-GATE-048 are notes, which do not stop a publish, that say why its release
  *Needs approval*: an audience group, a data class, a mail domain or an outside service that the
  app type does not allow, people outside the company, a destructive schema change, data that
  reaches mail in a new way, a page open to more people than before, a function of `rpc/` open
  to more callers than before, and a key bound to an outside service that is new or changed.

Schema changes use these codes:

- E-MIG-001: a draft too large, not UTF-8, with no statement, not a file in the app's folder,
  committed to git, or sent with a publish that has not ended.
- E-MIG-002: a statement that a schema change may not hold, such as one that changes data, an
  executable comment, `DELIMITER` or a name with a database in front.
- E-MIG-003: a note on a destructive statement, such as `DROP TABLE` or `ALTER TABLE … DROP
  COLUMN`: the release needs an approval, and the app stops briefly while it is applied.
- E-MIG-004: migrations that differ from those the database ran, and on the platform a release
  of a publish whose schema changes were rejected after a step failed on production data, or a
  publish made on top of the schema changes of a publish whose approval card was declined.
- E-MIG-005: a step that failed while it was applied; on the platform, the schema changes after
  a step that MySQL refused on production data are rejected, and a step that could not finish is
  tried again with the next release.
- E-MIG-006: a draft while the permission list declares no `sqldb`.
- E-MIG-007: a rollback to a release before a destructive schema change that ran in production.

E-LINT-013 refuses a schema change in app code, which goes in the draft, and SQL in app code that
names `aicoded_migrations`, the platform's journal of migrations.

Notes on where data goes, which never stop a check or a publish, use these codes:

- E-DATA-001: data from the app's database, its files, its inbox or a secret that a call of
  `mailer.Send` may send.
- E-DATA-002: a data class that the permission list adds.

Releasing uses these codes, each a release that the platform refuses before anything runs:

- E-REL-001: a publish that did not pass the delivery pipeline, has no built app, or has a
  permission list that the platform cannot read; and a rollback to a release that never went live
  or is live already.
- E-REL-002: an organisation that the platform's operator has not placed on a server.
- E-REL-003: an app built with a framework older than v0.6.0.
- E-REL-004, E-REL-007 and E-REL-017: what the platform does not run yet: people outside the
  company, calls between apps, schedules, and outside services of an app built with a framework
  older than v0.10.0.
- E-REL-005: secrets where the target of the organisation's server does not keep them.
- E-REL-006, E-REL-008 and E-REL-013: an `email.from` address in a domain that the organisation
  has not verified, that another app of the organisation has, or of which the hosting makes no
  mailbox.
- E-REL-014: an `email.from` other than the address the app sends from already.
- E-REL-010: an app that has a release pending already.
- E-REL-011: a building block that the target of the organisation's server does not offer.
- E-REL-018: an app whose resources were made at another target than the one of its organisation's
  server.
- E-REL-012: an app whose name, or whose organisation's slug, cannot be a label of its host.
- E-REL-015: an app that the platform's operator archived.
- E-REL-019 to E-REL-021: an app type that the organisation does not have, a group or a role
  that its group map does not have, and a data source that its source registry marks *Not
  allowed*. Each also fails a publish when the platform decides on its release.
- E-REL-022: a setting or a secret that the permission list declares and that has no value set
  in the console.
- E-REL-023: a release or a rollback that *Needs approval*, asked for by the platform's operator.

Approving an approval card in the console uses these codes, each an approval that the platform
refuses, so that nothing is released. The card still waits, unless E-REL-026 refuses it because
it was approved or declined already:

- E-REL-024: an approver who built the publish.
- E-REL-025: an approver who signed in more than five minutes ago.
- E-REL-026: a card that changed since the approver opened it: a newer publish, another
  permission list, built app or decision, or a card already approved or declined.
- E-REL-027: a person who is not in the app type's approver group.

Starting a preview with `aicoded preview` or the `preview_publish` tool uses these codes, each a
start that the platform refuses, so that a preview that runs keeps its version:

- E-REL-029: a publish that has not passed its checks, or whose approval card was declined.
- E-REL-030: a setting or a secret that the permission list declares and that has no value set
  for the preview in the console.

E-REL-031 refuses the app's owner a sixth person to see the app's preview, in the console.

E-REL-009 refuses the operator's purge of a publish that a live or pending release, or the app's
running preview, runs.

E-REL-016 refuses the operator's renaming or archiving of an app that has a release live or
pending: such an app keeps its name and stays active until releases can be stopped.

E-REL-028 refuses the operator's setting or deleting a secret of an app of another organisation:
the app's owner or a security admin sets it in the console.

E-MAIL-009 refuses the operator's adding or verifying a mail domain of an organisation that has no
target to register it at: one placed on no server, or whose server's target the platform cannot
reach.

E-MAIL-010 refuses, on the platform, a message with a new idempotency key while 10 000 of the app's
messages are being sent.

E-MAIL-011 refuses, in a preview on the platform, a message over 1 MiB: a preview sends no mail,
and keeps each message for the app's owner to read in the console.

E-RUN-005 refuses an app's call to a building block the runner does not provide; none on the
platform today.

E-RUN-006 is the page a runner shows to a signed-in person whom the app's audience does not name,
and the page the platform shows at an app's preview to a person the preview does not admit.

E-RUN-009 is the reason a runner reports for a release whose grants it cannot tell the resources
of: a grant of a kind it does not know, or two of one kind.

E-RUN-011 refuses, on the platform, an app's read of a declared secret that has no value where the
app runs.

E-RUN-012 refuses, on the platform, an app's read of a secret past the runner's bound of its reads:
100 at once, then 20 a second, and 8 under way at once.

E-RUN-014 refuses the start of an app built with a framework older than v0.10.0 whose permission
list declares `egress`, and E-RUN-015 an app's read of a secret that `egress` binds to a host.

E-RUN-013 is the page a runner shows, with status 503, for an app whose new version did not start
after a destructive schema change, which the earlier version cannot run on.

E-RUN-007, E-RUN-008 and E-RUN-010 are what the platform answers a server's runner that enrols or
connects: a refused enrolment token or certificate request, a runner certificate that is missing,
expired, revoked or of another server, and a runner of another protocol version.

E-LOGIN-001 refuses to sign a person in to an app when the sign-in provider gives them an issuer or
a subject of more than 255 characters, or an identity of more than 512 bytes joined.

`aicoded explain <CODE>` prints the page.

Retired codes are never reused:

- E-GEN-044 refused a page with pages below it and no `<ssr:content/>`. Such a page is now a gate: its access rule and `Guard` cover the pages below it, which are not shown inside it.
