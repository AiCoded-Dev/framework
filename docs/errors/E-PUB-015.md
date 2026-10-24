# E-PUB-015: a time range longer than 7 days

`aicoded logs` and `aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp`
with `env` set to `preview` or `production`, read an app's logs and traces on the platform. One
read covers at most 7 days, from `since` to `until`; this one asked for more, so the platform read
nothing. The platform keeps logs and traces for 30 days.

**Fix:** ask for 7 days or less: move `--since` and `--until` closer together, and read a longer time in several reads.
