# E-GATE-042: an outside service the app type does not allow

This is a note: it does not stop the publish. It is a reason its release *Needs approval*, and `aicoded publish` lists it under `Needs approval:`. The app's permission list names, in `egress`, a host that its app type does not allow the app to call. The Internal tool allows no outside service. See [app types](../guides/app-types.md).

```yaml
egress: [api.partner.example]
```

**Fix:** an approver sees the outside service as new on the approval card; to stay Pre-approved, remove the host from egress, or ask your security admin for an app type that allows it
