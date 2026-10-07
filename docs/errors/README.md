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
| CLI | `aicoded init`, `aicoded explain`, `aicoded login`, `logout` and `whoami`, and the `app_create` and `howto` tools of `aicoded mcp` |
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

`aicoded explain <CODE>` prints the page.

Retired codes are never reused:

- E-GEN-044 refused a page with pages below it and no `<ssr:content/>`. Such a page is now a gate: its access rule and `Guard` cover the pages below it, which are not shown inside it.
