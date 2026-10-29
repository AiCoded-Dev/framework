# E-REL-022: a setting or secret with no value

The permission list declares a setting or a secret that has no value set for production, where the
app runs. Rather than start the app without it, so that `config.String` reads it empty or `secrets.Get`
fails at its first read, the platform refuses the release, and the message lists the names that have no
value. Nothing was released, and the app keeps running the release it had. A *Pre-approved* publish
waits, and goes live by itself about a minute after the last value is set. The values are set in
the console by the app's owner or a security admin, never by the app, its builder or its AI
assistant; the values in `dev.yaml` are for `aicoded dev` only. See
[settings and secrets](../guides/settings-and-secrets.md).

The check is made at each release, a rollback too: a rollback to a release whose permission list
declares a value deleted since is refused the same way. A release that runs already is not
stopped when a value is deleted: a deleted setting reaches the app as an empty string at its next
start, and a deleted secret fails with E-RUN-011 at its next read.

```yaml
settings: [report_day]             # each name needs a value in the console
secrets: [partner_api_key]
```

**Fix:** ask the app's owner or a security admin to set the names the message lists in the console
