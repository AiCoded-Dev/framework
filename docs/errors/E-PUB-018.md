# E-PUB-018: logs and traces cannot be read right now

The platform keeps apps' logs and traces in a store of its own, which `aicoded logs`,
`aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp` with `env` set to
`preview` or `production`, read through it. The platform cannot reach that store right now: it is
starting, or the store is down for a while. Nothing is lost: the apps' runners keep sending their
logs and traces, and the platform reads them again once the store is back.

**Fix:** try again in a few minutes: the platform cannot read logs and traces right now.
