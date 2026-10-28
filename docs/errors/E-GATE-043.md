# E-GATE-043: people outside the company

This is a note: it does not stop the publish. It is a reason the release *Needs approval*: the publish passed, but it goes live only once an approver approved it, and `aicoded publish` and `aicoded status` list it under `Needs approval:`. Until the console exists, the platform's operator releases such a publish when your organisation asks. The app's permission list names, in `audience.external`, people outside the company, such as `magic-link`. Whatever the app type allows, an app that outside people may use always needs an approver. For now no release with an external audience goes live at all (E-REL-004). See [app types](../guides/app-types.md).

```yaml
audience:
  internal: [group:hotel-staff]
  external: [magic-link]
```

**Fix:** an approver sees the outside people on the approval card; to stay Pre-approved, leave audience.external empty
