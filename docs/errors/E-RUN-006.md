# E-RUN-006: audience not served here

The runner where the app runs serves an app only to an audience it can check: for now, every
person of the app's organisation, `audience.internal: [everyone]` with no other entry and no
`external` audience. This app's permission list names another audience, so the runner shows the
app to no one rather than to more people than the list allows, and answers every request with
this error. A release refuses such a list before it runs (E-REL-004), so the runner meets one only
when that check was missed.

```yaml
audience:
  internal: [everyone]
```

**Fix:** set `audience: {internal: [everyone]}` in aicoded.yaml, with no group and no external entry, then publish and release the app again
