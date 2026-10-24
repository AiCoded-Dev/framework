# E-PUB-016: more than 1000 rows

`aicoded logs` and `aicoded traces`, and the `logs` tool of `aicoded mcp` with `env` set to
`preview` or `production`, read at most 1000 log lines or spans of an app on the platform at once,
the newest first. This read asked for more with its limit, so the platform read nothing.

**Fix:** ask for 1000 rows or fewer, and narrow the time range with `--since` and `--until` to see the others.
