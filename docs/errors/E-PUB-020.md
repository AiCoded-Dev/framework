# E-PUB-020: more rows than one answer holds

`aicoded logs` and `aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp`
with `env` set to `preview` or `production`, read an app's logs and traces on the platform. One
answer holds at most 16 MiB of rows, and the rows this read asked for are more: an app that logs
long lines can pass it before 1000 rows. The platform answered none of them. Asking again gives the
same answer.

**Fix:** ask for fewer rows with --limit, or for a shorter time with --since and --until.
