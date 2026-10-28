# E-MAN-008: invalid data classes

Every data source in the permission list (`aicoded.yaml`) says which classes of data it holds. A class is one of four, from the least sensitive to the most: `public`, `internal`, `confidential` and `personal`. The app type that security defines sets the highest class an app of that type may hold, and a class above it needs approval. This entry has no classes, or a class that is not one of the four. The classes `health`, `hr` and `card` are not allowed at all (E-MAN-024).

```yaml
data:
  - source: sqldb
    classes: [secret]              # wrong: not a class
  - source: filestore:invoices
    classes: [internal, personal]  # right
```

See [the permission list](../guides/permission-list.md).

**Fix:** use public, internal, confidential or personal, such as `classes: [internal]`, for the data the source holds.
