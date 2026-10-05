# Telemetry: logs and traces

An app logs with Go's `log/slog` and records trace spans with `telemetry.Start`. The framework
already records a span for every request, statement, file, mail and call, and ties each log line
to its trace. None of it may hold personal data.

## Logs

`app.Main` makes the default `slog` logger write JSON lines to standard output, with the
`trace_id` and `span_id` of the context on every line that has one. Log with the context:

```go
slog.InfoContext(ctx, "user added")
slog.InfoContext(ctx, "report sent", "rows", len(rows))
```

- The framework logs what goes wrong on its side: an error a hook returned that answers 500, a
  refused form token, a page call that failed or a panic. Outside `aicoded dev` it logs only the
  error's kinds and codes, never its text; see "No personal data" below. So return an error
  instead of logging it.
- `aicoded dev` keeps each app's last 5000 log lines and shows them on the Logs page of the dev UI
  and through the `logs` tool of `aicoded mcp`. A line that is not JSON counts as `INFO`.

## Spans

The framework records these spans:

| Span | Attributes | For |
|---|---|---|
| `HTTP GET`, `HTTP POST`, … | `http.path` | every request the app serves, including its live connection |
| `sql.query`, `sql.exec`, `sql.begin` | `db.operation` | every statement; see [the SQL database](sql-database.md) |
| `filestore.<operation>` | `store` | every file store operation; see [file stores](file-stores.md) |
| `mailer.send`, `mailer.list`, … | | every mail call; see [mail](mail.md) |
| `rpc.call` | `rpc.app`, `rpc.method` | every call to another app; see [calls between apps](calls-between-apps.md) |

A call to another app carries the trace, so the called app's spans join the caller's trace.

To time your own work, start a span with the context, and end it:

```go
ctx, span := telemetry.Start(ctx, "users.add")
span.SetAttr("photos", strconv.Itoa(len(photos)))
defer span.End()
```

- `telemetry.Start` begins a child of the context's current span, or a new trace, and returns the
  context that carries the new span: pass it on, so the work inside becomes children of it.
- `SetAttr(key, value)` records a string attribute, and `RecordError(err)` marks the span as
  failed: with the error's text under `aicoded dev` in environment `dev`, and with its kinds and
  codes anywhere else.
- `End` hands the span to the runner. Calls after `End` do nothing.

`addUser` in the people example, which `Deps.AddUser` calls, records the error that ends it:

```go
defer func() {
	if err != nil {
		span.RecordError(err)
	}
	span.End()
}()
```

`aicoded dev` keeps each app's last 10000 spans and shows them on the Traces page of the dev UI,
as a tree with their timing, attributes and errors, and through the `traces` and `trace` tools of
`aicoded mcp`. Exporting the spans to the company's tools comes later, and so do metrics.

## No personal data

Logs and spans reach more tools and more people than the app's data does. Never put personal
data in them: no form values, names, logins, email addresses, uploaded file names or message
texts, and no secret.

- Log that something happened and its outcome, never what a viewer wrote. Count instead of
  listing: the people example's span holds the number of photos, not their names.
- An error's text can quote a value, such as the duplicate entry a UNIQUE index refuses or the
  name of a stored file. So wherever the framework records an error, in its log lines and in
  `RecordError`, it keeps only the error's kinds and codes outside `aicoded dev`: E-codes, MySQL
  error numbers, status codes, the sentinels of `io/fs`, `database/sql` and `context`, the
  operation of a file error, and the Go type of any other error. An error of a file store reads
  like `fs.PathError write: fs.ErrNotExist`, a duplicate entry `MySQL error 1062 (23000)`, and an
  error from `errors.New` `*errors.errorString`. A panic's value is recorded by its Go type.
- Under `aicoded dev` in environment `dev`, whose data is made up, the framework records the
  whole text and the panic's value, so that you and your AI assistant see what went wrong.
- The error the app gets back still holds the text. Return it from the hook, page call or
  function, or pass it to `RecordError`, and the framework records it as above; never log it
  or put it in a span attribute.
- `aicoded check` refuses a log line, a print or a span that records a value from outside the
  app (E-LINT-008): a form value, a URL parameter, the request, the viewer's identity, a value
  from the database, a mail from the inbox, a stored file, a secret, the input of a page call, of
  a `Validate` hook or of a function the app serves to other apps, or another app's answer, and
  any text, error or struct made from one, a hash included. The code of a template's expressions
  is checked too. Numbers and booleans carry nothing, so ids, counts, lengths and flags may be
  logged.
- A request span records the path, so a value in a URL, such as a login in `/users/alice`, is in
  the span. Keep data that must not be traced out of URLs.

## See also

- [aicoded dev](dev.md): the Logs and Traces pages.
- [MCP](mcp.md): the `logs`, `traces` and `trace` tools.
- [people/deps/users.go](../../examples/people/deps/users.go): a span of its own, with no personal
  data.
