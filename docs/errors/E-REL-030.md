# E-REL-030: a setting or secret with no value for the preview

The permission list of the publish declares a setting or a secret that has no value set for the
preview. A preview reads only its own values, never production's, so a value set for production
does not count. Rather than start the preview without it, the platform refuses the start, and the
message lists the names that have no value. Nothing was started, and a preview that runs keeps its
version. The values are set in the console by the app's owner or a security admin, never by the
app, its builder or its AI assistant; the values in `dev.yaml` are for `aicoded dev` only. E-REL-022
is the same check for production. See [settings and secrets](../guides/settings-and-secrets.md).

```yaml
settings: [report_day]             # each name needs a value for the preview in the console
secrets: [partner_api_key]
```

**Fix:** ask the app's owner or a security admin to set the names the message lists for the preview in the console
