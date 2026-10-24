# E-PUB-019: no app of that name on the platform

`aicoded logs` and `aicoded traces`, and the `logs`, `traces` and `trace` tools of `aicoded mcp`
with `env` set to `preview` or `production`, read an app of your organisation on the platform by
its name, the `app:` line of its `aicoded.yaml`. Your organisation has no app of this name: the
name is mistyped, or the app was never published, since the first publish of a name creates the
app.

**Fix:** check the app's name on the app: line of its aicoded.yaml: an app is on the platform once it is published.
