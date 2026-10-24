# Find why a published app fails

Read a published app's logs and traces on the platform with `aicoded logs` and `aicoded traces`,
or the `logs`, `traces` and `trace` tools of `aicoded mcp`, follow a failed request through its
trace, and fix the code. Read production only when the developer asks: its logs hold what the
app logged while people used it.

## Steps

1. Ask the developer what failed and when, such as "saving a ticket failed at about 9:10". The
   platform keeps 30 days, and one read covers at most 7 days and 1000 rows (E-PUB-015,
   E-PUB-016), and an answer of at most 16 MiB (E-PUB-020).
2. Read the errors of that time, in the app's folder or with `--app`. `--env` is `preview` or
   `production` and must be given:

   ```sh
   aicoded logs --env production --since 2h --level ERROR
   ```

   With MCP, call `logs` with `app`, `env` `production`, `since` `2h` and `level` `ERROR`. A line
   that belongs to a request ends with `trace <id>`.
3. Read that request's spans, indented below the span that holds them, with the error codes of
   the spans that failed:

   ```sh
   aicoded traces --env production --trace 4bf92f3577b34da6a3ce929d0e0e4736
   ```

   With MCP, call `trace` with the `app`, `env` and `trace_id`. Without `--trace`, `aicoded
   traces` lists the newest spans, and the `traces` tool the newest traces, `errors_only` for
   those with a failed span.
4. Read the page of the error code a span or a line shows, such as `aicoded explain E-SQL-001`,
   find the code that made it, write a test in `aicoded dev` that fails the same way, and fix it.
   Logs and spans hold the error's kinds and codes, never its text, outside `aicoded dev`; see
   [telemetry](../guides/telemetry.md).
5. Commit, run `aicoded check`, and publish only when the developer asks.

## Rules

- Never copy log lines or span attributes into code, tests, commits or anything else: they can
  hold what the app's users entered.
- Your sign-in reads only apps you own. Another builder's app is E-PUB-005, and an app never
  published E-PUB-019. When the platform cannot read logs right now (E-PUB-018), try again later.
- Every read is written to the platform's audit log, with the app, the range and the filters.

## See also

- [The tools](../guides/tools.md): `aicoded logs` and `aicoded traces`.
- [MCP](../guides/mcp.md): the `logs`, `traces` and `trace` tools.
- [Telemetry](../guides/telemetry.md): what an app logs and traces, and what it must not.
