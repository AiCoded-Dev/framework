# E-PUB-021: rows the platform cannot read

`aicoded logs` and `aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp`
with `env` set to `preview` or `production`, read an app's logs and traces on the platform. A row of
the time asked for is not one the platform can read, so it answered none of them. This is the
platform's fault, not the app's, and asking again gives the same answer.

**Fix:** read another time with --since and --until, and give your administrator the app and the time that fails.
