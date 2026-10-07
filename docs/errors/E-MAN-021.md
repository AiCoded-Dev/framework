# E-MAN-021: invalid resources

`resources` in the permission list (`aicoded.yaml`) is the app's runtime limit: `small`, `medium` or `large`, in lower case. The budget owner sets it; leave it out unless the person or their administrator gives it.

```yaml
resources: S       # wrong: that is a value of size
resources: small   # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** use `small`, `medium` or `large`.
