# Changelog for AI assistants

Changes to the API that AI assistants build apps with. Newest first.

## Unreleased

- `aicoded check` reports what `go vet` finds (E-CHK-003) with Go 1.26 too, which prints it on
  stdout; with Go 1.26 it used to pass over those findings. When `go vet` prints anything else it
  cannot read, the check fails with E-CHK-002, which quotes what `go vet` printed. It runs `go vet`
  with `-v=false -x=false`, so a GOFLAGS of `-v` or `-x` adds nothing to that output.
- A publish has notes: findings in the shape of problems that do not stop it. `aicoded publish` and
  `aicoded status` print each after the problems, on a line that starts with `note:`;
  `aicoded status --json` and the `release_status` tool of `aicoded mcp` return them as `notes`,
  after `problems`. The last line of a failed publish still counts the problems only. A platform
  that sends no notes shows none. `checks` gains the value `all-but-l7`, printed as
  `checks: the security checks of layers L1 to L5 ran; simulated attacks (L7) come later`, when the
  delivery pipeline ran every step: `checkout`, `modules`, `check`, `lint` (golangci-lint with
  staticcheck, gosec, errcheck, bodyclose and sqlclosecheck), `secrets` (gitleaks over the commit
  and its history), `vulnerabilities` (OSV-Scanner and govulncheck), `licences` (go-licenses),
  `sbom` (Syft), `capabilities` (capslock), `build` and `tests`; `partial` keeps its text. The steps
  from `check` to `capabilities` all run, whatever one finds. `//nolint`, `#nosec` and
  `//gosec:disable` are honoured for now, but each is a note that security sees: fix what the check
  reports instead. E-GATE-011 now means only an `aicoded.yaml` that names another app; one that is
  missing or cannot be read is E-GATE-027. The guide `guides/publishing` lists the steps and their
  layers. New codes: E-GATE-000 (a finding with no code of its own), E-GATE-013 to E-GATE-017
  (staticcheck, gosec, errcheck, bodyclose and sqlclosecheck), E-GATE-018 and E-GATE-019 (a secret
  in the code or in its history), E-GATE-020 (a critical known weakness the app calls), E-GATE-026
  (a `godebug` line in `go.mod`), E-GATE-027 (an `aicoded.yaml` that is missing or cannot be read),
  E-GATE-028 (tests that stopped the checks), and the notes E-GATE-021 to E-GATE-025 (a known
  weakness that is not critical or that the app does not call, a licence that is forbidden or not
  recognised, a capability reached without a building block, and a directive that silences a
  check).
- `aicoded describe` and the `describe` tool of `aicoded mcp` show the new fields of the
  permission list, each only when the app has it. The text gains the lines `class`, `owner`,
  `audience` and `external audience` after the `app` line, `connectors` after `files`, `egress`
  and one `schedule <job>  cron <expression>` for each job after the mail, and `size`,
  `resources` and `ttl` after `modules`. The JSON gains `class`, `owner`, `audience` (with
  `internal` and `external`), `connectors`, `egress`, `schedule` (with `job` and `cron`),
  `size`, `resources` and `ttl`. The surface counts each scheduled job as an entry point, and
  each connector and each `egress` host as an effect.
- The permission list, `aicoded.yaml`, takes every field: besides `app`, `data`, `access`,
  `services`, `email`, `settings`, `secrets` and `modules`, the optional `class` (the app type),
  `owner` (`group:<name>`), `audience` (`internal`: `group:<name>` or `everyone`; `external`:
  `idp:<name>` or `magic-link`), `egress` (host names, such as `api.partner.example`), `schedule`
  (`{cron: "0 6 * * *", job: daily-summary}`, five numeric cron fields in UTC), `size` (`S`, `M`
  or `L`), `resources` (`small`, `medium` or `large`) and `ttl` (`1d` to `3650d`). A field left
  out or empty is not stated. Write `owner` and `audience` when the person says who owns and
  uses the app; leave `class`, `size`, `resources` and `ttl` out unless the person or their
  administrator gives them. The platform records these fields and shows their changes in the
  change record; nothing holds the app to them yet. An unknown key, at the top level or in a
  section with fixed keys, is now an error (E-MAN-014), and so are anchors, aliases and merge
  keys, `&name`, `*name` and `<<` (E-MAN-003). Every command that reads the permission list
  reports all its problems at once, in line order, instead of the first: fix them all, then run
  `aicoded check` again. The guide `guides/permission-list` covers every field. The permission
  list's schema is now the public package `aicoded.dev/framework/manifest`, with `Load` and
  `Parse`, for runners and the platform; app code may not import it (E-LINT-001). The package
  also has `Diff`, which lists what changed between two permission lists field by field, as the
  platform's change records show it. New codes: E-MAN-014 (an unknown key), E-MAN-015
  (`class`), E-MAN-016 (`owner`), E-MAN-017 (`audience`), E-MAN-018 (`egress`), E-MAN-019
  (`schedule`), E-MAN-020 (`size`), E-MAN-021 (`resources`) and E-MAN-022 (`ttl`).
- New commands publish an app to the platform: `aicoded publish [--app <name>] [-m <summary>]
  [--no-wait] [dir]` sends the commit `HEAD` names, of an app folder that is the top level of its
  own git repository, to the platform's delivery pipeline, once nothing is left uncommitted and
  `aicoded check --frozen --no-tests` finds no problem, and waits for the outcome; `aicoded status
  [--json] <id>` shows a publish's steps, problems and status. The first publish of a name creates
  the app, owned by the builder. A publish sends a git bundle of the history since the app's last
  verified publish, or all of it, at most 32 MiB, and needs git 2.31 or later. `aicoded mcp` gains
  the tools `publish`, which takes an app and a summary and returns a publish id at once, and
  `release_status`, which takes that id. Publish only when the person asks, commit first, report
  the outcome, and fix its problems as those of `aicoded check`; on E-CLI-004, ask the person to
  run `aicoded login`. When the platform does not accept the sign-in's token, or lacks an app's
  scopes in it, both commands and both tools refresh the sign-in once and ask again; only a token
  it still refuses is E-CLI-004. The guide "Releasing" is now "Publishing", `guides/publishing`.
  New codes: E-PUB-001 to E-PUB-014 (publishing) and E-GATE-001 to E-GATE-012 (the checks of the
  delivery pipeline).
- `aicoded check --no-tests` runs every step of `check` but the tests, and does not probe
  `-race`; the report has `"no_tests": true` in JSON, and its text ends with
  `tests did not run (--no-tests)`. The `check` tool of `aicoded mcp` does not take it: run the
  full check. Commands that use the sign-in to the platform at the same time now take turns
  through `credentials.lock`, next to `credentials.json`, so that only one refreshes it. When
  the platform cannot refresh the sign-in right now (no connection, no answer in time, a 5xx
  status, `server_error` or `temporarily_unavailable`), the sign-in is kept and the command fails
  with E-CLI-009: try again in a minute. `aicoded login` with an organisation the platform does
  not know fails with E-CLI-006, whose fix says to check the name with your administrator; Ctrl-C
  during `aicoded login` prints `sign-in cancelled`. `AICODED_PLATFORM` may end in `:443`, which
  names the same platform as the address without it. New codes: E-CLI-009 (a sign-in the
  platform could not refresh right now).
- New commands sign a builder in to the platform: `aicoded login [--org <organisation>]
  [--device]` signs in, at an address it prints for a browser on this computer, or, with
  `--device` on a computer without a browser, with a code entered in a browser on any device,
  and never opens a browser itself; `aicoded logout` revokes the sign-in and deletes it;
  `aicoded whoami [--json]` prints the organisation, email and scopes of the sign-in. The first login needs `--org`, and later ones
  remember it. `AICODED_PLATFORM` names the platform, `https://api.aicoded.cloud` by default. The
  sign-in needs a person and a browser, so no MCP tool offers it: when a command says you are not
  signed in (E-CLI-004), ask the person to run `aicoded login`. New codes: E-CLI-004 (not signed
  in), E-CLI-005 (an address in `AICODED_PLATFORM` that is not valid), E-CLI-006 (a sign-in the
  platform refused), E-CLI-007 (a credentials file that others can read) and E-CLI-008 (a sign-in
  not finished in time).
- The fixes of E-LINT-010 and E-MAN-007 put code in code spans, such as `filestore:<name>`.
- The framework's module path is now `aicoded.dev/framework` (it was
  `gopkg.aicoded.cloud/framework`). Imports, the `require` and `replace` lines of an app's
  `go.mod` and `go doc` use the new path, such as `aicoded.dev/framework/web/form`, and the CLI's
  module is `aicoded.dev/framework/cmd/aicoded`. Docs links are under `https://aicoded.dev/docs/`,
  such as the `docs:` line of every error, `https://aicoded.dev/docs/errors/<code>`.
- `aicoded generate` refuses an `<ssr:access>` that adds nothing to a rule above it on the path
  (E-GEN-053): a list of two or more different roles that includes every role of that rule, such
  as `role="staff,people"` below `role="staff"`, with or without `guard="true"`, since a list
  admits anyone with one of its roles; and `role="*"` without `guard="true"` below a rule that is
  not `*`. A single role repeated from above, `*` below `*`, and a single role or `*` with
  `guard="true"` are allowed. The error names the nearest such rule, and its fix lists only the
  roles the page adds. AGENTS.md and the access guide say that a list admits anyone with one of
  its roles, and that two roles are required by putting them on templates at different levels
  of the path.
- The `Init<Form>` stub that `aicoded generate` writes says that the hook runs every time the
  page shows or receives the form, on a GET too, and must not change data: `Process<Form>` does.
  AGENTS.md, the forms guide and `tasks/add-form` say so too. AGENTS.md lists the framework's
  packages for app code, among them `aicoded.dev/framework/web/form` for the types of form
  fields.
- A new recipe, `tasks/list-by-role`, lists a viewer's own records, or every record for a role,
  with one constant statement for each case, from the room-maintenance example. E-LINT-007's
  page and fix point to it. `tasks/ownership-check` says that the framework calls a page's
  `Guard` only with `guard="true"` in its own `<ssr:access>` (E-GEN-032).
- `aicoded check` and `aicoded dev` name the framework's package when an app imports a path
  under `aicoded.dev/framework` that the framework does not have. The E-CHK-002 of the
  import adds the packages for app code whose last path element is the same, and its fix imports
  them instead: an import of `aicoded.dev/framework/form` gets
  `import "aicoded.dev/framework/web/form" instead`. The packages named are the ones
  that `aicoded explain E-LINT-001` lists.
- `dev.yaml` takes `env:` per workspace: `dev`, the default, or `preview`, which runs the
  workspace's apps as a preview. Under `preview`, `app.Env` returns `preview`, each app's
  database and user are `ac-preview-<app>`, and the framework records errors and panics by their
  kinds and codes only, as outside `aicoded dev`. Any other value is E-DEV-020.
- The framework has a third example app, `examples/room-maintenance`, the reference for records
  with an owner: hotel staff report problems in rooms and see and edit only their own tickets,
  and `hotel-ops` see every ticket and change its status, which shows live in every open list
  that holds the ticket. Every page has an access rule, a `Guard` on each ticket answers
  `web.Forbidden()`, the SQL filters by the author too, and a live list wakes only when a ticket
  its viewer may open changes, through a `reactive.Topic` keyed by author, and reads its tickets
  again with the viewer's own rights. Its `README.md` shows where each check is. The examples'
  personas gain dana, with the roles `staff` and `hotel-ops`.
- E-LINT-008 follows outside data through the setters of form fields, such as `SetError`,
  `SetValue` and `SetOptions`, so logging a field's error that quotes outside data is refused,
  and through `WriteHtml` of a select field's options; through the `String`, `Error`, `Format`
  and `MarshalJSON` methods that `fmt`, `log/slog` and `encoding/json` call on a value and on what
  it holds through pointers, elements, exported fields and embedded structs, and a context held
  there, which prints its values; and through more functions of the standard library that write
  into an argument, such as `Sum` of a hash, `slices.Insert`, `slices.Replace`, `io.CopyBuffer`
  and `ExpandString` of a `regexp.Regexp`. A call of a function value that may record through
  several functions is refused once, naming them all.
- E-LINT-008 follows outside data along more paths. Into a context: through any value whose
  type is or can become a context, such as a struct that embeds `context.Context` and holds the
  viewer's name, or an interface type of the app with the methods of `context.Context`, also
  when the value is written after it became a context; through the key and the value of
  `context.WithValue`; and through every call of a function value of type `func(error)`, such
  as the cancel function of `context.WithCancelCause`. Out of a context: through `Value`,
  `context.Cause`, a type assertion, the app's own methods, or the context itself in a log line.
  What one context of the app holds counts for all of them. And through the `Append` functions,
  such as `fmt.Appendf`, `strconv.AppendQuote` and `AppendEncode` of `base64.StdEncoding`, into
  the slice they are given.
- E-LINT-008 also checks the code that `aicoded generate` writes from templates, and points to
  the template's line: with a `*slog.Logger` in `log`, `{{ log.With('q', form.Q.GetValue()) }}`
  is refused like the same call in Go. The people example returns the errors of its file store
  as they are, since the framework records them without the file names outside `aicoded dev`,
  and `nameless` is gone. AGENTS.md names E-LINT-008, and the guides list it among the checks.
- E-LINT-008: lint refuses outside data in a log line, a print or a span: a form value, a URL
  parameter, the request, the viewer's identity, a value from the database, a mail from the
  inbox, a stored file, a secret, the input of a page call, `Validate` hook or RPC function, or
  another app's answer, or any string, error or struct made from one, that reaches `log/slog`,
  `fmt.Print`, `fmt.Printf`, `fmt.Println`, `print`, `println`, the name of `telemetry.Start` or
  `SetAttr` of a span. Numbers and booleans may be recorded; returned errors and `RecordError`
  are not checked.
- The framework records no error text outside `aicoded dev`. Wherever it logs or traces an
  error, it keeps only the error's kinds and codes, such as `E-FILE-001`,
  `MySQL error 1062 (23000)`, `fs.PathError write: fs.ErrNotExist`, `HTTP 403`, `rpc not_found`
  or `*errors.errorString`, and a panic's value only by its Go type: for an error a hook returns
  that answers 500, a refused live value, a failed subscription, page call or function, a panic,
  `Span.RecordError` and the error `app.Main` prints. Under `aicoded dev` it records the whole
  text and value. Generated live pages log through `web.LogError`, which is for generated code
  only (E-LINT-005), and `Snapshot` of `web.Reactive` takes the connection's context.
- E-LINT-004 holds `main` to one form: the options of `app.Main` and `app.Run` are an
  `app.Options` literal written in the call, such as
  `app.Main(app.Options{Handler: pages.NewHandler(d), OnStart: d.Start})`. A conversion such as
  `app.Options(o)`, a value of an unnamed struct type with the same fields, a variable, and
  options that a function returns, also one of a declared module, are refused, and so is any
  use of `app.Main` or `app.Run` other than a direct call, such as `f := app.Main`.
- In a declared module, E-LINT-003 also refuses `slog.SetDefault`, `slog.SetLogLoggerLevel` and
  `slog.NewLogLogger`, and a change of a package-level variable of the standard library or the
  framework, such as `time.Local = loc`, `*time.Local = ...`, `crc32.IEEETable[0]++`, a `range`
  that assigns `rand.Reader`, or `&time.Local`.
- E-LINT-007 also refuses constant SQL that holds more than one statement, such as
  `DELETE FROM drafts; DROP TABLE notes` (a `;` may only end the statement), and SQL that reads or
  writes a file of the database server with `INTO OUTFILE`, `INTO DUMPFILE` or `LOAD_FILE`.
- `aicoded check` runs lint's precheck of GOFLAGS and the `vendor` folder for every app before
  any other go command, so a GOFLAGS entry such as `-toolexec` never reaches `go build`; an app
  it refuses (E-LINT-011) gets no other step. `aicoded describe` warns that the surface counts no
  writes when the precheck stops lint. Package `lint` exports `Precheck`, `Result.Stopped` and
  `Framework`, the framework's module path.
- E-LINT-001 refuses the import of a package of the app that the checks do not read, in
  `testdata`, in a folder whose name starts with `_` or `.`, or in a folder that an `ignore`
  directive of `go.mod` names, and says so; and a path under `aicoded.dev/framework` that
  is not a package of the framework module. E-LINT-010 says when a Go file is in a folder that
  `go.mod` ignores. A finding or error that quotes the go command gives the first line of what it
  printed that is not a `go: downloading` notice, writes the module cache as `$GOMODCACHE`, and a
  file of a module as `module@version/path`. When `go env` itself cannot run, such as for a
  `go.mod` that asks for a Go that `GOTOOLCHAIN` cannot download, E-LINT-011 says to fix what it
  reports, such as the `go` line of `go.mod` or `GOTOOLCHAIN`.
- E-GEN-042 also reserves the package-level names that a page's generated files declare, in
  expressions, loops, `<ssr:var>` names and the types of `<ssr:var>` and `<ssr:call>`:
  `routeKey`, `route`, `state`, `NewRoute`, `RouteData`, `RouteDataProvider`, `ReactiveState`,
  `NewHandler`, `assets`, `assetsDir`, names starting with `renderBlock_`, and a form's
  `Form<Name>Values`.
- E-LINT-004 refuses a composite literal whose type is a type parameter or a pointer to one,
  such as `T{h, ...}` or each `{...}` in `[]T{{...}}`, whatever the constraint, and a field named
  `Handler` or `RPC` whose type mentions a type parameter, wherever it is set, such as
  `g.Handler = h` in a generic struct: use the concrete type. A literal that only holds values of
  a type parameter, such as `[]T{a, b}`, is allowed, and a keyed field such as `Handler:` is
  checked in every literal. E-LINT-009 refuses a conversion to or from a pointer to a type
  parameter, or a type parameter whose type set holds a pointer type or cannot be listed, such as
  `(*logger)(p)` for a `p` of type `P *slog.Logger`; `int(v)` for a type parameter constrained to
  number types, and a conversion to an interface type that is not a type parameter, such as
  `any(v)`, stay allowed.
- E-LINT-009 now also refuses writing through a pointer whose pointee is a type parameter
  (`*p = v` in a generic function), and converting a pointer to a standard-library or framework
  named type into a different pointer type (`(*myLogger)(slog.Default())`); both could change a
  value the app may not. Writing through your own pointer to such a type, such as
  `*out = time.Now()` for an `out *time.Time`, is refused for the same reason: return the value
  instead.
- E-LINT-011 is now "replace, go.work or vendor not allowed": besides a `replace` line and a
  `go.work` file, `aicoded check` refuses a `vendor` folder at the app root, a module cache that
  `go mod verify` does not pass (fix: `go clean -modcache`), and every GOFLAGS entry outside a
  short list. Before lint runs a go command of its own, it reads GOFLAGS from the environment
  and then from `go env -json`, and splits each at spaces, tabs and line breaks; a quote, a
  backslash or a control character is refused, and only `-mod=mod`, `-mod=readonly`,
  `-modcacherw`, `-trimpath`, `-buildvcs=true`, `-buildvcs=false`, `-buildvcs=auto`, `-v` and
  `-x` are allowed. A `go env` that fails or prints anything else, as a GOFLAGS of `-u` or `-w`
  makes it do, is refused too.
  The one allowed `replace` is still the framework pointed at the checkout this `aicoded` was
  installed from.
- E-LINT-007 now accepts only statements whose first keyword is `SELECT`, `INSERT`, `UPDATE`,
  `DELETE`, `REPLACE`, `WITH`, `CREATE TABLE`, `CREATE INDEX`, `ALTER TABLE`, `DROP TABLE` or
  `DROP INDEX`; `SET`, `PREPARE`, `EXECUTE`, `CALL`, `TRUNCATE` and the like are refused, because
  they can run a string as SQL or change how SQL is lexed. Its parser follows MySQL's lexing, so a
  `--` comment needs a space after it and a doubled backquote inside a name is one backquote. In a
  declared module, E-LINT-003 also refuses `reflect.NewAt`, `reflect.Value`'s `Pointer`,
  `UnsafeAddr` and `UnsafePointer`, and every method of the `database/sql/driver` connection,
  statement, transaction and driver interfaces.
- E-LINT-004 also reads a literal whose type is left out, such as each element of
  `[]*options{{h, ...}}`. E-LINT-009 also refuses an assignment through `*` to a value of a
  named type of the standard library or the framework, such as `*slog.Default() = ...`;
  `sql.ColumnType.ScanType`; and any method or field of package `reflect` used on a
  `reflect.Value` or `reflect.Type` the app gets from a value it is given, such as
  `e.Type.MethodByName(...)` for an `e *json.UnmarshalTypeError` (reading the `Type` field itself
  is allowed).
- E-LINT-011: `aicoded check` refuses every `replace` line in an app's `go.mod`, and a `go.work`
  file that the go command reads for the app. The one exception is the framework's `replace`
  that `aicoded init` writes from a framework checkout, accepted only by the `aicoded` installed
  from that checkout. In a declared module, E-LINT-003 also refuses a Go file that the checks
  cannot parse.
- E-LINT-007 also refuses SQL that comes from a call with several results, such as
  `db.QueryContext(statement())`, and SQL that holds an executable comment, `/*!` or `/*M!`. A
  method of an interface, or of a type parameter's constraint, is checked as a method of
  `database/sql` that takes SQL when its name and signature match once each type parameter in it
  stands for any type, such as `QueryContext(ctx context.Context, query string, args ...any) (R,
  error)`; one with such a name whose signature has a type parameter and does not match is
  refused. E-LINT-005, E-LINT-006 and E-LINT-009 match interface methods the same way. The
  surface counts the write that follows a `WITH` clause. In a declared module, E-LINT-003 refuses
  `reflect.Value`'s `Call`, `CallSlice`, `Method` and `MethodByName` and `reflect.Type`'s
  `Method` and `MethodByName`.
- Lint also reads the code that `aicoded generate` writes from templates: E-LINT-006, E-LINT-007
  and E-LINT-009 apply to it, and a problem in it points to the template's line, such as
  `pages/index.html:12`. So `{{ store.DB.QueryRow('SELECT ' + q) }}` is refused like the same
  call in Go, and the tables a template's SQL writes count in the surface. App code may use
  `sql.Named` and `sql.ErrConnDone` (E-LINT-009).
- The guide `guides/rules` lists every rule that lint applies to app code: what not to do, what
  to do instead, and the error code. AGENTS.md points to it, and its "Never" bullets name
  imports and calls that lint refuses, such as `os`, `log`, `sql.Open`, `web.New` and
  `http.Get`.
- E-LINT-007: the SQL passed to `Query`, `QueryContext`, `QueryRow`, `QueryRowContext`, `Exec`,
  `ExecContext`, `Prepare` and `PrepareContext` of `*sql.DB`, `*sql.Tx` and `*sql.Conn`, or of an
  interface with such a method, is a constant expression, and these methods are only called
  directly. A list for `IN` goes in as one JSON array read with `JSON_TABLE`, as
  `guides/sql-database` shows. A declared module runs no SQL at all (E-LINT-003).
- `aicoded describe`, the `describe` tool and `aicoded check` show each app's surface: the larger
  of its entry points (pages, page calls for each page that offers them, functions served) and
  its effects (each table its constant SQL writes, once per kind of write, and each mail
  domain). `describe` lists the writes as `writes INSERT users`, and its JSON has `surface` with
  `size`, `entry_points`, `effects` and `writes`; `check`'s JSON has `surface` for every app
  that lint has read. `describe` now runs lint to read the SQL, and warns when it cannot.
- The permission list has a hand-written `modules:` section: the third-party Go modules the app's
  code uses, by module path, such as `golang.org/x/text`, each once and without a version
  (E-MAN-013). Lint refuses a third-party module that the app, or a declared module, imports
  without declaring it (E-LINT-002); the framework, the modules only it imports and the modules
  only tests use need no entry. It checks every package of a declared module that the app's
  build uses (E-LINT-003): no import of `os/exec`, `net`, `syscall`, `plugin`, `unsafe`, `C`,
  `crypto/tls`, `log/syslog`, `go/build`, `go/importer`, the framework or any `net/...` package
  other than `net/http`, `net/url`, `net/mail` and `net/netip`; no `os.StartProcess`,
  `os.FindProcess` or `os.Process`; no `//go:linkname`; no assembly or other source that is not
  Go; only the names of `net/http` app code may use; and no way to open a database or reach its
  driver. `aicoded describe` and the `describe` tool list the declared modules.
- Lint has four more rules for hand-written code. E-LINT-004: `app.Options.Handler` is a direct
  call of the generated `pages.NewHandler(...)` and `app.Options.RPC` of the generated
  `rpc.Server()`, set in the `app.Options` literal, never a raw mux or a wrapped handler.
  E-LINT-005: functions for generated code, whose go doc now says "Generated code only.", such
  as `rpc.Call`, `web.New`, `r.CSRFToken()`, a form field's `Process` and
  `reactive.HTMLBinding`, are not called, and an unexported name of a generated file is not
  used. E-LINT-006: `sql.Open`, `sql.OpenDB`, `sql.Register` and `db.Driver()` are refused; the
  database comes from `sqldb.Open`. E-LINT-009: from `net/http` only the request, response,
  header and cookie types, `StatusText`, `DetectContentType` and the constants are used, from
  `database/sql` its types, isolation levels, `ErrNoRows` and `ErrTxDone`, and `conn.Raw`,
  `slog.SetDefault`, `slog.SetLogLoggerLevel` and `slog.NewLogLogger` are refused; nor does app
  code change a variable of the standard library or the framework, such as `rand.Reader` or
  `time.Local`.
- `aicoded check` and the `check` tool lint every app that builds and vets cleanly, before its
  tests. Lint applies the rules for app code, whose problems have E-LINT codes; test files are
  not linted, and nothing in the code, such as `//nolint`, turns a finding off. E-LINT-001: an
  app imports only its own packages, the building blocks and the standard packages that its page
  lists, so never `os`, `net`, `reflect`, `unsafe` or `runnerproto`; `embed`, `rpc/wire`,
  `web/render` and `protowire` are for generated files, and a file counts as generated only when
  `aicoded generate` wrote it. E-LINT-010: an app holds only plain Go files built on every
  machine, with no assembly, C or object files, no cgo, and no `//go:build` line or `_<os>` or
  `_<arch>` file name that leaves a file out of this build. When the go command is a newer Go than the one `aicoded` was built with,
  lint cannot run and every app gets E-CHK-007, whose fix names both versions.
- `aicoded init <name>` and the `app_create` tool also write `AGENTS.md`: the rules for AI
  assistants that build the app, what they must never do, the loop of `aicoded dev` and
  `aicoded check`, and where to read more. The framework repository has the same file at its
  root. The `next` of `app_create` says to read it first.
- Every package an app imports documents itself: `go doc aicoded.dev/framework/<package>`
  prints what the package is for, its rules and the guide that covers it, and the package's
  `Example` functions show it in use.
- The framework has two example apps. `examples/people` has pages, forms with every field type,
  live values, a database, a file store, mail and a call to `examples/contacts`, which serves
  that call from its own database and checks ownership again. Neither puts personal data in its
  logs or spans. `examples/README.md` says how to run them with `aicoded dev`.
- The docs are built into `aicoded`: guides, task recipes, the changelog and the error pages.
  `aicoded explain <topic>` and the `howto` tool print a page by its path, such as
  `guides/overview`, `tasks/add-form`, `changelog` or `E-DEV-001`. Without a topic, both list
  the guides, the tasks and `changelog`, then the error codes, and `howto` gives each page its
  `summary`. A topic that names no page is E-CLI-001, now titled "no page for this topic or
  error code". The pages are also the resources `aicoded://docs/<path>`, such as
  `aicoded://docs/guides/forms`, and the MCP server's instructions start at
  `howto guides/overview`. The task recipes take their code from the example apps, and a test
  keeps it the same. `llms.txt` at the framework's root lists every guide and task, and a test
  refuses hidden Unicode in the repository.
- `aicoded dev --manual <app> [dir]` runs an app by hand, as under a debugger: `aicoded dev`
  keeps its runner up in a run folder that stays the same, never builds or starts the app, and
  prints the command that starts it:
  `cd <dir> && AICODED_RUNNER_DIR=<run folder> go run .`. The `preview` tool reports such an app
  with `state: "manual"` and the fields `command` and `runner_dir`, which only that state has;
  `preview` generates it and keeps its runner, unless its permission list or `dev.yaml` values
  changed. A `--manual` name that no app has is E-DEV-014. E-RUN-001's fix now names manual mode.
- `aicoded dev` serves a dev UI for the developer at `http://localhost:<port>`, behind a login
  link it prints; that address no longer lists the apps, so take an app's address from `preview`.
  The developer can stop an app there: its address then answers 503 and it is not rebuilt when
  its files change, until `preview` or the developer starts it again. An `aicoded dev` for a
  folder above a running one is E-DEV-012 too. A token file of the dev UI that is not valid is
  E-DEV-018, and a request to the dev UI without logging in is E-DEV-019. No tool result holds
  the dev UI's token.
- `aicoded init <name>` creates an app: `go.mod`, `aicoded.yaml`, `main.go` and `pages/index.html`.
  A bad or taken name is E-CLI-002; an `aicoded` that knows no framework version is E-CLI-003.
- `aicoded check [--frozen] [--json] [--app <name>] [dir]` runs `aicoded generate`, the held
  snapshot check, `go build`, `go vet` and `go test` for every app, and lists each problem with
  its code, `file:line` and fix: E-CHK-002 (build), E-CHK-003 (vet), E-CHK-004 (test), E-CHK-005
  (no `go`) and E-CHK-006 (`go mod tidy`). `--frozen` writes nothing and reports each generated
  file that is out of date as E-CHK-001. Tests run with `-race` when cgo works, and otherwise
  without it, with a note.
- `aicoded describe [--json] [--app <name>] [dir]` summarises the apps and never writes;
  `aicoded explain [CODE]` prints an error page or lists them.
- `aicoded mcp [dir]` serves the tools `app_create`, `check`, `preview`, `what_broke`, `describe`,
  `howto`, `logs`, `traces`, `trace`, `mail_list`, `mail_get` and `mail_receive` over stdio, and
  the resources `aicoded://errors/<CODE>` and `aicoded://changelog`. Tools take app names, never
  paths; a name no app of the workspace has is E-DEV-014. It uses the `aicoded dev` that runs for
  the folder, or runs one itself for the session.
- `aicoded dev` watches the apps and rebuilds one when its files change. An app that fails to
  generate, holds a broken snapshot, fails to build or start, lacks `dev.yaml` values, exits or
  gets a new name on its `app:` line (E-DEV-015) stops alone, and its address answers 503 with
  its problems until a change builds. `dev.yaml` values are read at every start of an app. A port
  in use is E-DEV-011; a second `aicoded dev` for the same workspace is E-DEV-012, and one of
  another version is E-DEV-013. A control socket path that holds something else is E-DEV-016,
  and one too long for a Unix socket is E-DEV-017. Logs, spans and caught mail survive restarts,
  with secret values of 4 bytes or more shown as `[secret <name>]`.
- `aicoded generate` checks every snapshot the workspace's apps hold of this app before it writes
  anything, so a breaking change to `rpc/` fails with E-RPC-013 and leaves `.aicoded/rpc.json` as
  it was. A file other than `client_gen.go` in `services/<app>/` is E-RPC-014. It prints esbuild
  warnings.
- `aicoded dev` carries calls between the apps it runs and checks each one against both
  permission lists. It refuses a call that `services.calls` does not name (E-RPC-008), a call to
  an app that is not running (E-RPC-011), and a viewer token that the runner did not issue to the
  calling app, that is an app's own, or that it issued more than 10 minutes ago (E-RPC-012): call
  with the context of the request or call being served. An app whose held snapshot a called app
  broke fails alone (E-RPC-013); a snapshot of an app outside the workspace only prints a warning.
- `aicoded dev` run in a folder above apps runs all of them. It generates each app, starts each
  one after the apps it calls, and keeps the others running when one fails. Of apps that call
  each other, the first in folder order starts first, so never call, in `OnStart`, an app that
  calls this app too (E-RPC-011). Two apps with one name are refused (E-DEV-010). The folder may
  be reached through a symbolic link; links below it are not followed. An app with no pages that
  serves calls has no address. Without `personas`, `all-roles` has the roles of every app's pages
  and functions.
- `aicoded rpc add <app>`, run in the calling app, finds `<app>` in the folder above it and
  copies its snapshot to `.aicoded/services/<app>.json`. `aicoded generate` then writes the client
  `services/<package>/client_gen.go`, whose package is `<app>` without dashes, and
  `services.calls`. Before it writes anything in the calling app, `rpc add` refuses a name that no
  app or two apps there have, the calling app itself and an app with no function in `rpc/`
  (E-RPC-007), and a package name that is a Go keyword, is predeclared or is taken (E-RPC-006).
- `services.calls` lists the client functions the app's code names, found without
  type-checking: a local variable named like a client package can add a function the app never
  calls, and a client imported with `.` is not seen, so its calls are refused (E-RPC-008). Keep
  the client's name for the client alone.
- For an app with `rpc/`, `aicoded generate` writes `rpc/server_gen.go`, the snapshot
  `.aicoded/rpc.json` and `services.serves`; never edit them. An app with no pages gets
  `access: {}`.
- Calls between apps: exported functions in `rpc/` (`package rpc`) of the shape
  `func Name(ctx context.Context, in In) (Out, error)`, each with
  `//ssr:access caller=<apps> role=<a|b> apps=true`. `caller=` and one of `role=` and
  `apps=true` are required. A function without the line is E-RPC-001; a malformed line, a line on
  anything but an exported function and a line written `// ssr:access` are E-RPC-002; another
  shape, or a name that Go or `rpc/server_gen.go` uses, is E-RPC-003. A function with
  `apps=true` and no `role=` takes no viewer's calls. Fields are `bool`, `int32`, `int64`,
  `uint32`, `uint64`, `float32`, `float64`, `string`, `[]byte`, `time.Time`, exported structs of
  `rpc/`, slices of these, and pointers for optional values; `int`, maps, other packages' types,
  slices of pointers and pointers to slices are refused (E-RPC-004).
- `.aicoded/rpc.json` pins every field's number, and a removed field's number is never used
  again in its struct. A struct that no function reaches any more, or that is renamed, loses its
  numbers, and its fields are numbered afresh. Renaming or removing a function or field, or
  changing a field's type or number, breaks its callers. A snapshot file that is not valid is
  E-RPC-005.
- `rpc.Error(code, msg)` and `rpc.Errorf` with `rpc.InvalidArgument`, `NotFound`,
  `AlreadyExists`, `PermissionDenied`, `FailedPrecondition` or `Unavailable` reach the caller
  with their message, cut at 1 KiB. Any other error or a panic reaches it as `rpc.Internal`
  "internal error". `rpc.CodeOf(err)` reads the code and `rpc.Caller(ctx)` names the calling
  app. A call gets 10 s when its context has no deadline, and at most 30 s; one cut short
  matches `context.DeadlineExceeded` or `context.Canceled` with `errors.Is`.
  `app.Options.RPC` serves the generated `rpc.Server()`, and an app with only `RPC` needs no
  `Handler`.
- Runner protocol 3 adds calls between apps: `RpcService.Call` on `runner.sock` and
  `AppService.Serve` on `app.sock`. `runnerproto` adds `DefaultCallDeadline` (10 s),
  `MaxCallDeadline` (30 s) and `MaxForwardedTokenAge` (10 min), and `OriginHeader` and
  `OriginRunner`: the runner marks its own refusals with the error metadata
  `Aicoded-Origin: runner`. Only a refusal with that mark reaches the caller as a coded error,
  whose text starts with the code and carries its fix line; a called app's own refusal reaches the
  caller as plain text, whatever it looks like. `rpc.CodeOf` reads the status of every error.
  Viewer tokens gain `Claims.App`, for a call an app makes as itself, and
  `viewer.VerifyIssued` checks a token an app hands back to the runner. An app built with this
  framework needs an `aicoded` that speaks protocol 3; the runner refuses a mismatch when the app
  starts (E-RUN-002).
- `sqldb.Open(ctx)` returns the app's own MySQL database as a `*sql.DB`, with no DSN or password;
  it needs `- source: sqldb` under `data` (E-MAN-010). Open it once in `OnStart` and pass values
  with `?` placeholders; one statement runs per call, and times are in UTC.
- `mailer.Send(ctx, mailer.Message{…})` sends from `email.from` to `email.to_domains` only
  (E-MAIL-003); `IdempotencyKey` is required, and a repeated key sends nothing. `mailer.List`,
  `Get`, `Delete` and `Move` read the app's one mailbox: `mailer.Inbox`, `Archive` and `Trash`.
- `filestore.Open(ctx, name)` returns `filestore:<name>` as an `fs.FS` with `Create`, `WriteFile`,
  `Mkdir`, `Remove` and `Rename`; a file appears only when `Close` succeeds. Names are paths inside
  the store (E-FILE-001), a file is at most 1 GiB (E-FILE-002), an undeclared store is E-MAN-011.
- `aicoded dev` offers the personas `all-roles`, with every role the `access` section names, and
  `no-roles` when `dev.yaml` names none.
- `aicoded generate` writes the `access` section of `aicoded.yaml`: one entry per page a viewer can
  open, keyed by URL pattern, with `require`, `guard` and `calls`. Never edit it. The file must be
  one YAML document in block style, or generate refuses it (E-MAN-003).
- Runner protocol 2 adds the file and mail services. An app built with this framework needs an
  `aicoded` that speaks protocol 2; the runner refuses a mismatch when the app starts (E-RUN-002).
- `<ssr:assets/>`, `<ssr:content/>` and `<html>` inside `ssr:if`, `ssr:else-if`, `ssr:else` or
  `ssr:for` are refused (E-GEN-051), and so is a page that renders without an `<html>` element
  (E-GEN-052).
- The permission list (`aicoded.yaml`) gains `data`, entries of `source` (`sqldb`,
  `filestore:<name>`, `connector:<name>`) with their `classes`, and `email` with `from` and
  `to_domains` (E-MAN-007..009). Using an undeclared database, store or mail is E-MAN-010..012.
- Pages: folders under `pages/` with an `index.html` are pages; `aicoded generate` writes their
  code. Every page path needs `<ssr:access role="a,b"/>`, with lower-case role names; rules along a
  path are combined, so the viewer needs a role from each, and `role="*"` admits every viewer.
- Parameter folders: `s_name` captures one URL part as a string and `n_name` a whole number made
  of digits; a part that is not one answers 404. Read both with `r.URLParam("name")`;
  `r.URLParamInt("name")` returns an `n_` value as an `int64`. Folder names are ASCII letters,
  digits and `_`, starting with a letter, and not `main` or a Go keyword. No folder under `pages/`
  starts with `_` or is named `testdata`.
- A page with pages below it is their layout when it contains `<ssr:content/>`, and their gate
  when it does not. A layout shows the page below where `<ssr:content/>` stands. Opening it
  sends the viewer to its default page, or answers 404 without one. Name the default by the
  folders of a page below the layout, relative to it, as `<ssr:content default="settings"/>` in
  `pages/admin/index.html` for `/admin/settings`. `aicoded generate` refuses a default that
  names no such page, or a parameter folder (E-GEN-049); otherwise leave `default` out and
  return the page from `DefaultRoute`. A gate's access rule and `Guard` apply to every page
  below it, but its forms, `Data`, live values, calls, markup and assets stay on its own page,
  which opening it shows: put a list at `pages/notes` and its items at `pages/notes/n_id`. When
  the root page is a gate, each page below it writes its whole document: `<!doctype html>`,
  `<html>`, a `<head>` with `<ssr:assets/>`, and `<body>`.
- `<ssr:assets/>` writes the scripts and styles of its page and of every page shown inside it;
  a page whose scripts or styles would never load is refused (E-GEN-050).
- `<ssr:access … guard="true"/>` adds `Guard(ctx, r) error` to the page's data provider; return
  `web.Forbidden()` for records the viewer may not see, or `web.NotFound()` only when even their
  existence must stay hidden.
- Hooks run in this order: the access rules of every page on the path, then their `Guard`s, root
  first, then the form token (on a post), and `Init<Form>`, `Process<Form>` (only the posted
  form, and only when it is valid) and `Data` for the layouts on the path and the page itself.
  Hooks get `*web.Request` (`r.URLParam(name)`) and `web.ResponseWriter`, which has only
  `Header()`: set a cookie with `w.Header().Add("Set-Cookie", c.String())`.
- End a request with `web.NotFound()`, `web.Forbidden()`, `web.Error(status, message)` or
  `web.Redirect("/path")`; redirects go only to paths on the same site.
- Templates escape `{{ }}` for where the value lands (text, attribute, URL). In
  `href="/{{ slug }}"` a leading `/` of the value is encoded and an empty value becomes `.`, so the
  link never leads to another site. Inline `<script>` code, `srcdoc`, `<style>`, `on*` and `style`
  attributes are errors: put code in `index.ts` and styles in `index.css` next to `index.html`.
  Pass data to scripts with `<ssr:json name="id" value="expr"/>`, one name per template.
- `{{$ expr }}` writes markup, only in element text, and takes only `web.SafeHTML`
  (`web.HTMLConst`, `web.EscapeHTML`, `web.JoinHTML`). Declare such a variable with
  `<ssr:var name="intro" type="web.SafeHTML"/>`: a variable's type may be an exported type of
  package `web`, but of no other package.
- A variable the template declares but never reads, or only binds to an input, builds.
- Whitespace between elements shows as one space, as in the browser: `<b>a</b> <i>b</i>` shows
  "a b". Text in `<pre>`, `<textarea>` and raw-text elements such as `<noscript>` keeps its
  whitespace as written.
- Page scripts import only files of the app, by a path such as `./label`; packages and
  `node_modules` are refused. Images in `<img src>`, next to the template or as `/assets/<file>`,
  are embedded in the app and served under `/_aicoded/assets/`. `pages/assets_gen/` holds no
  source maps.
- Select fields accept only the options `Init<Form>` set.
- Live values: `<ssr:var … reactive="true"/>` with `Subscribe(ctx, r, state)` and
  `state.Set<Name>(v)`; `client-writable="true"` adds `Validate<Name>`. Page scripts import
  `ssr` from `./reactive_gen`. `ssr:bind` works on `<input>` (not `type="file"`, and only with a
  fixed `type`), `<select>` and `<textarea>`.
- Page calls: `<ssr:call name="star" in="StarIn" out="StarOut"/>` adds
  `CallStar(ctx, r, in StarIn) (StarOut, error)` to the page's data provider; the page script calls
  `await ssr.call("star", {...})` over the page's live connection. Access rules and `Guard` run again
  for every call; the roles are those the connection opened with, so a role change takes effect
  when the page reconnects, at the latest after 10 minutes. A failed call rejects with
  `{status, message}`; only `web.Error` messages reach the page.
- `app.Main(app.Options{Handler: h})` runs an app under its runner. Every request reaching `h`
  has a verified viewer; there is no login code in apps.
- `app.Run(ctx, opts)` does the same until `ctx` is done and returns the error instead of
  exiting; use it in tests or to embed the app. `main` calls `app.Main`.
- `app.Options.OnStart(ctx)` runs after the handshake and before the app reports ready; its `ctx`
  can read settings and secrets. An error stops the app.
- `app.Name(ctx)`, `app.Env(ctx)` and `app.Version(ctx)` return the app's name, environment (for
  example `dev`) and release version.
- `auth.Viewer(ctx)` returns the viewer: `.Subject`, the stable id to store as a record's owner,
  `.Name`, `.HasRole(r)`, `.InGroup(g)`.
- `config.String(ctx, name)` reads a setting declared under `settings:` in `aicoded.yaml`.
- `secrets.Get(ctx, name)` reads a secret declared under `secrets:`; call `.Reveal()` only where
  the value is used. Printing or logging a secret shows `[REDACTED]`.
- `telemetry.Start(ctx, name)` starts a span; `defer span.End()`. `span.SetAttr(key, value)`
  records an attribute and `span.RecordError(err)` marks the span as failed.
- `aicoded dev` runs apps locally with test personas. It runs `aicoded generate` first, so
  pages are always built from their current templates.
