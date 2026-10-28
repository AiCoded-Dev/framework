# E-REL-019: an app type the organisation does not have

The permission list's `class` names the app's type, which security defines for the organisation, and this organisation has no app type of that name. Without its app type the platform cannot tell what the app may do without an approver, so it decides nothing: the publish fails, with this problem in its step `decision`, and a release of an earlier publish whose `class` names no type is refused the same way. An app without `class` is an Internal tool, `internal-tool`, which every organisation has. The message lists the organisation's app types. See [app types](../guides/app-types.md).

```yaml
class: internal-tool               # or leave class out
```

**Fix:** set class in aicoded.yaml to one of the organisation's app types that the message lists, or leave it out for internal-tool, commit, and publish again
