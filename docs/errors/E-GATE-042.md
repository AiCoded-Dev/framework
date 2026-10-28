# E-GATE-042: an outside service the app type does not allow

This is a note: it does not stop the publish. It is a reason the release *Needs approval*: the publish passed, but it goes live only once an approver approved it, and `aicoded publish` and `aicoded status` list it under `Needs approval:`. Until the console exists, the platform's operator releases such a publish when your organisation asks. The app's permission list names, in `egress`, a host that its app type does not allow the app to call. The Internal tool allows no outside service. See [app types](../guides/app-types.md).

```yaml
egress: [api.partner.example]
```

**Fix:** an approver sees the outside service as new on the approval card; to stay Pre-approved, remove the host from egress, or ask your security admin for an app type that allows it
