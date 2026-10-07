# E-MAN-020: invalid size

`size` in the permission list (`aicoded.yaml`) is the app's size limit: `S`, `M` or `L`, in capitals. The budget owner sets it; leave it out unless the person or their administrator gives it.

```yaml
size: small   # wrong: that is a value of resources
size: S       # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** use `S`, `M` or `L`.
