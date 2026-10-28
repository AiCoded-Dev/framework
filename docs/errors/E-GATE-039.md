# E-GATE-039: an audience the app type does not allow

This is a note: it does not stop the publish. It is a reason the release *Needs approval*: the publish passed, but it goes live only once an approver approved it, and `aicoded publish` and `aicoded status` list it under `Needs approval:`. Until the console exists, the platform's operator releases such a publish when your organisation asks. The app's permission list names, in `audience.internal`, a group that its app type does not allow. The app type, which security defines, lists who its apps may serve: the Internal tool allows `everyone` and every group, and another type may allow only some groups. See [app types](../guides/app-types.md).

```yaml
class: finance
audience:
  internal: [group:finance-team, group:sales]   # finance allows group:finance-team only
```

**Fix:** an approver sees the group as new on the approval card; to stay Pre-approved, list only groups the app type allows, or ask your security admin for an app type that fits
